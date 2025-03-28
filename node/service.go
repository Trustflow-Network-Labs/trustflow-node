package node

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/price"
	"github.com/adgsm/trustflow-node/utils"
)

type ServiceManager struct {
	sm       *database.SQLiteManager
	lm       *utils.LogsManager
	services map[int32]*node_types.Service
	p2pm     *P2PManager
}

func NewServiceManager(p2pm *P2PManager) *ServiceManager {
	return &ServiceManager{
		sm:       database.NewSQLiteManager(),
		lm:       utils.NewLogsManager(),
		services: make(map[int32]*node_types.Service),
		p2pm:     p2pm,
	}
}

// Service already added?
func (sm *ServiceManager) ServiceExists(id int32) (error, bool) {
	if id <= 0 {
		msg := "invalid service id"
		sm.lm.Log("error", msg, "servics")
		return errors.New(msg), false
	}

	// Create a database connection
	db, err := sm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "servics")
		return err, false
	}
	defer db.Close()

	// Check if service is already existing
	var serviceId node_types.NullInt32
	row := db.QueryRowContext(context.Background(), "select id from services where id = ?;", id)

	err = row.Scan(&serviceId)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("debug", msg, "servics")
		return nil, false
	}

	return nil, true
}

// Get Service by ID
func (sm *ServiceManager) GetService(id int32) (node_types.Service, error) {
	var service node_types.Service
	if id <= 0 {
		msg := "invalid service id"
		sm.lm.Log("error", msg, "servics")
		return service, errors.New(msg)
	}

	// Create a database connection
	db, err := sm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "servics")
		return service, err
	}
	defer db.Close()

	// Get service
	row := db.QueryRowContext(context.Background(), "select id, name, description, node_id, service_type, active from services where id = ?;", id)

	err = row.Scan(&service)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("debug", msg, "servics")
		return service, err
	}

	sm.services[id] = &service

	return service, nil
}

// Get Services by node ID
func (sm *ServiceManager) GetServicesByNodeId(nodeId string) ([]node_types.Service, error) {
	var service node_types.Service
	var services []node_types.Service

	if nodeId == "" {
		msg := "invalid service id"
		sm.lm.Log("error", msg, "servics")
		return nil, errors.New(msg)
	}

	// Create a database connection
	db, err := sm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "servics")
		return nil, err
	}
	defer db.Close()

	// Search for services
	rows, err := db.QueryContext(context.Background(), "select id, name, description, node_id, service_type, path, repo, active from services where node_id = ?;", nodeId)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return services, err
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&service.Id, &service.Name, &service.Description, &service.NodeId,
			&service.Type, &service.Path, &service.Repo, &service.Active)
		if err != nil {
			msg := err.Error()
			sm.lm.Log("error", msg, "services")
			return nil, err
		}
		services = append(services, service)
		sm.services[service.Id] = &service
	}

	return services, nil
}

// Add a service
func (sm *ServiceManager) AddService(name string, description string, nodeId string, serviceType string, servicePath string, serviceRepo string, active bool) {
	// Create a database connection
	db, err := sm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return
	}
	defer db.Close()

	// Add service
	sm.lm.Log("debug", fmt.Sprintf("add service %s", name), "services")

	result, err := db.ExecContext(context.Background(), "insert into services (name, description, node_id, service_type, path, repo, active) values (?, ?, ?, ?, ?, ?, ?);",
		name, description, nodeId, serviceType, servicePath, serviceRepo, active)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return
	}

	sm.services[int32(id)] = &node_types.Service{}
}

// Remove service
func (sm *ServiceManager) RemoveService(id int32) {
	err, existing := sm.ServiceExists(id)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return
	}

	// Create a database connection
	db, err := sm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return
	}
	defer db.Close()

	// Check if service is already existing
	if !existing {
		msg := fmt.Sprintf("Service id %d is not existing in the database. Nothing to remove", id)
		sm.lm.Log("warn", msg, "services")
		return
	}

	// Check if there are jobs executed using this service
	jobManager := NewJobManager(sm.p2pm)
	jobs, err := jobManager.GetJobsByServiceId(id)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return
	}
	if len(jobs) > 0 {
		msg := fmt.Sprintf("Service id %d was used with %d jobs executed. You can not remove this service but you can set it service inactive", id, len(jobs))
		sm.lm.Log("warn", msg, "services")
		return
	}

	// Check if there are existing prices defined using this service
	priceManager := price.NewPriceManager()
	prices, err := priceManager.GetPricesByServiceId(id)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return
	}
	if len(prices) > 0 {
		msg := fmt.Sprintf("Service id %d is used with %d prices defined. Please remove prices for this service first", id, len(prices))
		sm.lm.Log("warn", msg, "services")
		return
	}

	// Remove service
	sm.lm.Log("debug", fmt.Sprintf("removing service %d", id), "services")

	_, err = db.ExecContext(context.Background(), "delete from services where id = ?;", id)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return
	}

	delete(sm.services, id)
}

// Set service inactive
func (sm *ServiceManager) SetServiceInactive(id int32) {
	err, existing := sm.ServiceExists(id)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return
	}

	// Create a database connection
	db, err := sm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return
	}
	defer db.Close()

	// Check if service is already existing
	if !existing {
		msg := fmt.Sprintf("Service id %d is not existing in the database. Nothing to set inactive", id)
		sm.lm.Log("warn", msg, "services")
		return
	}

	// Check if there are existing prices defined using this service
	priceManager := price.NewPriceManager()
	prices, err := priceManager.GetPricesByServiceId(id)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return
	}
	if len(prices) > 0 {
		msg := fmt.Sprintf("Service id %d is used with %d prices defined. Please remove prices for this service first", id, len(prices))
		sm.lm.Log("warn", msg, "services")
		return
	}

	// Set service inactive
	sm.lm.Log("debug", fmt.Sprintf("setting service id %d inactive", id), "services")

	_, err = db.ExecContext(context.Background(), "update services set active = false where id = ?;", id)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return
	}

	if _, exists := sm.services[id]; exists {
		sm.services[id].Active = false
	}
}

// Set service active
func (sm *ServiceManager) SetServiceActive(id int32) {
	err, existing := sm.ServiceExists(id)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return
	}

	// Create a database connection
	db, err := sm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return
	}
	defer db.Close()

	// Check if service is already existing
	if !existing {
		msg := fmt.Sprintf("Service id %d is not existing in the database. Nothing to set active", id)
		sm.lm.Log("warn", msg, "services")
		return
	}

	// Set service active
	sm.lm.Log("debug", fmt.Sprintf("setting service id %d active", id), "services")

	_, err = db.ExecContext(context.Background(), "update services set active = true where id = ?;", id)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return
	}

	if _, exists := sm.services[id]; exists {
		sm.services[id].Active = true
	}
}

func (sm *ServiceManager) LookupRemoteService(serviceName string, serviceDescription string, serviceNodeId string, serviceType string, serviceRepo string) {
	var serviceLookup node_types.ServiceLookup = node_types.ServiceLookup{
		Name:        serviceName,
		Description: serviceDescription,
		NodeId:      serviceNodeId,
		Type:        serviceType,
		Repo:        serviceRepo,
	}
	BroadcastMessage(sm.p2pm, serviceLookup)
}

func (sm *ServiceManager) SearchServices(searchService node_types.SearchService, params ...uint32) ([]node_types.ServiceOffer, error) {
	var services []node_types.ServiceOffer

	// Read configs
	configManager := utils.NewConfigManager("")
	config, err := configManager.ReadConfigs()
	if err != nil {
		message := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
		sm.lm.Log("error", message, "p2p")
		panic(err)
	}

	var offset uint32 = 0
	var limit uint32 = 10
	l := config["search_results"]
	l64, err := strconv.ParseUint(l, 10, 32)
	if err != nil {
		limit = 10
	} else {
		limit = uint32(l64)
	}

	if len(params) == 1 {
		offset = params[0]
	} else if len(params) >= 2 {
		offset = params[0]
		limit = params[1]
	}

	sql := `SELECT s.id, s.name, s.description, s.node_id, s.service_type, s.path, s.repo, s.active
		FROM services s
		WHERE 1`

	// Parse service names (comma delimited) to search for
	serviceNames := strings.Split(searchService.Name, ",")
	for i, serviceName := range serviceNames {
		serviceName = strings.TrimSpace(serviceName)
		// Skip if empty string
		if serviceName == "" {
			continue
		}
		// Query for each name provided
		if i == 0 {
			sql = sql + fmt.Sprintf(" AND (LOWER(s.name) LIKE LOWER('%s')", "%"+serviceName+"%")
		} else {
			sql = sql + fmt.Sprintf(" OR LOWER(s.name) LIKE LOWER('%s')", "%"+serviceName+"%")
		}
		if i >= len(serviceNames)-1 {
			sql = sql + ")"
		}
	}

	// Parse service descriptions (comma delimited) to search for
	serviceDescriptions := strings.Split(searchService.Description, ",")
	for i, serviceDescription := range serviceDescriptions {
		serviceDescription = strings.TrimSpace(serviceDescription)
		// Skip if empty string
		if serviceDescription == "" {
			continue
		}
		// Query for each description provided
		if i == 0 {
			sql = sql + fmt.Sprintf(" AND (LOWER(s.description) LIKE LOWER('%s')", "%"+serviceDescription+"%")
		} else {
			sql = sql + fmt.Sprintf(" OR LOWER(s.description) LIKE LOWER('%s')", "%"+serviceDescription+"%")
		}
		if i >= len(serviceDescriptions)-1 {
			sql = sql + ")"
		}
	}

	// Parse service identity nodes (comma delimited) to search for
	serviceNodeIds := strings.Split(searchService.NodeId, ",")
	for i, serviceNodeId := range serviceNodeIds {
		serviceNodeId = strings.TrimSpace(serviceNodeId)
		// Skip if empty string
		if serviceNodeId == "" {
			continue
		}
		// Query for each node ID provided
		if i == 0 {
			sql = sql + fmt.Sprintf(" AND (s.node_id = '%s'", serviceNodeId)
		} else {
			sql = sql + fmt.Sprintf(" OR s.node_id = '%s'", serviceNodeId)
		}
		if i >= len(serviceNodeIds)-1 {
			sql = sql + ")"
		}
	}

	// Parse service types (comma delimited) to search for
	serviceTypes := strings.Split(searchService.Type, ",")
	for i, serviceType := range serviceTypes {
		serviceType = strings.TrimSpace(serviceType)
		// Skip if empty string
		if serviceType == "" {
			continue
		}
		// Query for each type provided
		if i == 0 {
			sql = sql + fmt.Sprintf(" AND (s.service_type = '%s'", serviceType)
		} else {
			sql = sql + fmt.Sprintf(" OR s.service_type = '%s'", serviceType)
		}
		if i >= len(serviceTypes)-1 {
			sql = sql + ")"
		}
	}

	// Parse repos (comma delimited) to search for
	serviceRepos := strings.Split(searchService.Repo, ",")
	for i, serviceRepo := range serviceRepos {
		serviceRepo = strings.TrimSpace(serviceRepo)
		// Skip if empty string
		if serviceRepo == "" {
			continue
		}
		// Query for each repo provided
		if i == 0 {
			sql = sql + fmt.Sprintf(" AND (LOWER(s.repo) LIKE LOWER('%s')", "%"+serviceRepo+"%")
		} else {
			sql = sql + fmt.Sprintf(" OR LOWER(s.repo) LIKE LOWER('%s')", "%"+serviceRepo+"%")
		}
		if i >= len(serviceRepos)-1 {
			sql = sql + ")"
		}
	}

	// Filter service per provided active flag
	sql = sql + fmt.Sprintf(" AND s.active = %t", searchService.Active)

	// Add trailing semicolumn
	sql = sql + fmt.Sprintf(" limit %d offset %d;", limit, offset)

	// Create a database connection
	db, err := sm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return nil, err
	}
	defer db.Close()

	// Search for services
	rows, err := db.QueryContext(context.Background(), sql)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var service node_types.Service
		var serviceOffer node_types.ServiceOffer
		err = rows.Scan(&serviceOffer.Id, &serviceOffer.Name, &serviceOffer.Description, &serviceOffer.NodeId,
			&serviceOffer.Type, &serviceOffer.Path, &serviceOffer.Repo, &serviceOffer.Active)
		if err != nil {
			msg := err.Error()
			sm.lm.Log("error", msg, "services")
			return nil, err
		}

		service = node_types.Service{
			Id:          serviceOffer.Id,
			Name:        serviceOffer.Name,
			Description: serviceOffer.Description,
			NodeId:      serviceOffer.NodeId,
			Type:        serviceOffer.Type,
			Path:        serviceOffer.Path,
			Repo:        serviceOffer.Repo,
			Active:      serviceOffer.Active,
		}

		sm.services[service.Id] = &service

		// Query service resources with prices
		sql = fmt.Sprintf(`SELECT r.name, p.price, p.price_unit_normalizator, p.price_interval, c.currency, c.symbol
			FROM prices p
			INNER JOIN resources r ON p.resource_id = r.id
			INNER JOIN currencies c ON p.currency_id = c.id
			WHERE p.service_id = %d and r.active = true`, serviceOffer.Id)

		rrows, err := db.QueryContext(context.Background(), sql)
		if err != nil {
			msg := err.Error()
			sm.lm.Log("error", msg, "services")
			return nil, err
		}
		defer rrows.Close()

		var serviceResources []node_types.ServiceResourcesWithPricing

		for rrows.Next() {
			var serviceResource node_types.ServiceResourcesWithPricing
			err = rrows.Scan(&serviceResource.ResourceName, &serviceResource.Price, &serviceResource.PriceUnitNormalizator,
				&serviceResource.PriceInterval, &serviceResource.CurrencyName, &serviceResource.CurrencySymbol)
			if err != nil {
				msg := err.Error()
				sm.lm.Log("warn", msg, "services")
				continue
			}
			serviceResources = append(serviceResources, serviceResource)
		}

		serviceOffer.ServicePriceModel = serviceResources

		services = append(services, serviceOffer)
	}

	return services, nil
}
