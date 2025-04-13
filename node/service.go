package node

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/price"
	"github.com/adgsm/trustflow-node/utils"
)

type ServiceManager struct {
	db   *sql.DB
	lm   *utils.LogsManager
	p2pm *P2PManager
}

func NewServiceManager(p2pm *P2PManager) *ServiceManager {
	return &ServiceManager{
		db:   p2pm.db,
		lm:   utils.NewLogsManager(),
		p2pm: p2pm,
	}
}

// Service already added?
func (sm *ServiceManager) Exists(id int64) (error, bool) {
	if id <= 0 {
		msg := "invalid service id"
		sm.lm.Log("error", msg, "servics")
		return errors.New(msg), false
	}

	// Check if service is already existing
	var serviceId node_types.NullInt32
	row := sm.db.QueryRowContext(context.Background(), "select id from services where id = ?;", id)

	err := row.Scan(&serviceId)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("debug", msg, "servics")
		return nil, false
	}

	return nil, true
}

// Get Service by ID
func (sm *ServiceManager) Get(id int64) (node_types.Service, error) {
	var service node_types.Service
	if id <= 0 {
		msg := "invalid service id"
		sm.lm.Log("error", msg, "servics")
		return service, errors.New(msg)
	}

	// Get service
	row := sm.db.QueryRowContext(context.Background(), "select id, name, description, service_type, active from services where id = ?;", id)

	err := row.Scan(&service.Id, &service.Name, &service.Description, &service.Type, &service.Active)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("debug", msg, "servics")
		return service, err
	}

	return service, nil
}

// Get Data Service
func (sm *ServiceManager) GetData(serviceId int64) (node_types.DataService, error) {
	var dataService node_types.DataService
	if serviceId <= 0 {
		msg := "invalid service id"
		sm.lm.Log("error", msg, "servics")
		return dataService, errors.New(msg)
	}

	// Get data service
	row := sm.db.QueryRowContext(context.Background(), "select id, service_id, path from data_service_details where service_id = ?;", serviceId)

	err := row.Scan(&dataService.Id, &dataService.ServiceId, &dataService.Path)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("debug", msg, "servics")
		return dataService, err
	}

	return dataService, nil
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

	// Search for services
	rows, err := sm.db.QueryContext(context.Background(), "select id, name, description, node_id, service_type, path, repo, active from services where node_id = ?;", nodeId)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return services, err
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&service.Id, &service.Name, &service.Description,
			&service.Type, &service.Active)
		if err != nil {
			msg := err.Error()
			sm.lm.Log("error", msg, "services")
			return nil, err
		}
		services = append(services, service)
	}

	return services, nil
}

func (sm *ServiceManager) List(params ...uint32) ([]node_types.Service, error) {
	var services []node_types.Service

	// Read configs
	configManager := utils.NewConfigManager("")
	config, err := configManager.ReadConfigs()
	if err != nil {
		msg := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
		sm.lm.Log("warn", msg, "services")
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

	sql := fmt.Sprintf("SELECT id, name, description, service_type, active FROM services limit %d offset %d;", limit, offset)

	// Search for services
	rows, err := sm.db.QueryContext(context.Background(), sql)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var service node_types.Service

		err = rows.Scan(&service.Id, &service.Name, &service.Description,
			&service.Type, &service.Active)
		if err != nil {
			msg := err.Error()
			sm.lm.Log("error", msg, "services")
			return nil, err
		}

		services = append(services, service)
	}

	return services, nil
}

// Add a service
func (sm *ServiceManager) Add(name string, description string, serviceType string, active bool) (int64, error) {
	// Add service
	sm.lm.Log("debug", fmt.Sprintf("add service %s", name), "services")

	result, err := sm.db.ExecContext(context.Background(), "insert into services (name, description, service_type, active) values (?, ?, ?, ?);",
		name, description, serviceType, active)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return 0, err
	}

	return id, nil
}

// Add a data service
func (sm *ServiceManager) AddData(serviceId int64, pathss string) (int64, error) {
	// Add data service
	sm.lm.Log("debug", fmt.Sprintf("add data service path(s) %s to service ID %d", pathss, serviceId), "services")

	var cids []string
	var cidss string
	paths := strings.Split(pathss, ",")
	for _, path := range paths {
		path = strings.TrimSpace(path)
		// Skip if empty string
		if path == "" {
			continue
		}

		// Add file/folder, compress it and make CID
		// Compress (File/Folder)
		rnd := "./local_storage/" + utils.RandomString(32)
		err := utils.Compress(path, rnd)
		if err != nil {
			os.RemoveAll(rnd)
			sm.lm.Log("error", err.Error(), "services")
			return 0, err
		}

		// Create CID
		cid, err := utils.HashFileToCID(rnd)
		if err != nil {
			os.RemoveAll(rnd)
			sm.lm.Log("error", err.Error(), "services")
			return 0, err
		}

		err = os.Rename(rnd, "./local_storage/"+cid)
		if err != nil {
			sm.lm.Log("error", err.Error(), "services")
			return 0, err
		}

		cids = append(cids, cid)
	}

	cidss = strings.Join(cids, ",")
	result, err := sm.db.ExecContext(context.Background(), "insert into data_service_details (service_id, path) values (?, ?);",
		serviceId, cidss)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return 0, err
	}

	return id, nil
}

// Remove service
func (sm *ServiceManager) Remove(id int64) error {
	err, existing := sm.Exists(id)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return err
	}
	// Check if service is already existing
	if !existing {
		err = fmt.Errorf("service id %d is not existing in the database. Nothing to remove", id)
		sm.lm.Log("warn", err.Error(), "services")
		return err
	}

	// Get active (IDLE, READy or RUNING) jobs based on this service
	jobsManager := NewJobManager(sm.p2pm)
	jobs, err := jobsManager.GetJobsByServiceId(id, 1)
	if err != nil {
		sm.lm.Log("error", err.Error(), "services")
		return err
	}
	if len(jobs) > 0 {
		err = fmt.Errorf("there are %d active jobs (in status 'IDLE', 'READY' or 'RUNNING') depending on this service", len(jobs))
		sm.lm.Log("error", err.Error(), "services")
		return err
	}

	// Delete prices defined for this service (if any)
	priceManager := price.NewPriceManager(sm.db)
	err = priceManager.RemoveForService(id)
	if err != nil {
		sm.lm.Log("error", err.Error(), "services")
		return err
	}

	// Remove service
	sm.lm.Log("debug", fmt.Sprintf("removing service %d", id), "services")

	// Get service
	service, err := sm.Get(id)
	if err != nil {
		sm.lm.Log("error", err.Error(), "services")
		return err
	}

	switch service.Type {
	case "DATA":
		data, err := sm.GetData(id)
		if err != nil {
			sm.lm.Log("error", err.Error(), "services")
		} else {
			err = sm.removeData(data.Id)
			if err != nil {
				sm.lm.Log("error", err.Error(), "services")
			}
		}
	case "DOCKER EXECUTION ENVIRONMENT":
	case "STANDALONE EXECUTABLE":
	}

	_, err = sm.db.ExecContext(context.Background(), "delete from services where id = ?;", id)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return err
	}

	return nil
}

// Remove data service
func (sm *ServiceManager) removeData(id int64) error {
	// Remove data service
	sm.lm.Log("debug", fmt.Sprintf("removing data service %d", id), "services")

	// Get data service
	data, err := sm.GetData(id)
	if err != nil {
		sm.lm.Log("error", err.Error(), "services")
		return err
	}

	// Split comma separated paths
	paths := strings.Split(data.Path, ",")
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}

		// Check if same data path is used by other services
		var no int64 = 0
		row := sm.db.QueryRowContext(context.Background(), "select count(id) from data_service_details where path like ?;", "%"+path+"%")

		err = row.Scan(&no)
		if err != nil {
			sm.lm.Log("debug", err.Error(), "servics")
			return err
		}

		if no == 1 {
			// Delete the data only if this is the only service using the data
			err = os.RemoveAll("./local_storage/" + path)
			if err != nil {
				sm.lm.Log("error", err.Error(), "services")
				return err
			}
		}
	}

	_, err = sm.db.ExecContext(context.Background(), "delete from data_service_details where id = ?;", id)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return err
	}

	return nil
}

// Set service inactive
func (sm *ServiceManager) SetInactive(id int64) error {
	err, existing := sm.Exists(id)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return err
	}
	// Check if service is already existing
	if !existing {
		err = fmt.Errorf("service id %d is not existing in the database. Nothing to set inactive", id)
		sm.lm.Log("warn", err.Error(), "services")
		return err
	}

	// Check if there are existing prices defined using this service
	priceManager := price.NewPriceManager(sm.db)
	prices, err := priceManager.GetPricesByServiceId(id)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return err
	}
	if len(prices) > 0 {
		err = fmt.Errorf("service id %d is used with %d prices defined. Please remove prices for this service first", id, len(prices))
		sm.lm.Log("warn", err.Error(), "services")
		return err
	}

	// Set service inactive
	sm.lm.Log("debug", fmt.Sprintf("setting service id %d inactive", id), "services")

	_, err = sm.db.ExecContext(context.Background(), "update services set active = false where id = ?;", id)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return err
	}

	return nil
}

// Set service active
func (sm *ServiceManager) SetActive(id int64) error {
	err, existing := sm.Exists(id)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return err
	}
	// Check if service is already existing
	if !existing {
		err := fmt.Errorf("service id %d is not existing in the database. Nothing to set active", id)
		sm.lm.Log("warn", err.Error(), "services")
		return err
	}

	// Set service active
	sm.lm.Log("debug", fmt.Sprintf("setting service id %d active", id), "services")

	_, err = sm.db.ExecContext(context.Background(), "update services set active = true where id = ?;", id)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return err
	}

	return nil
}

func (sm *ServiceManager) LookupRemoteService(searchPhrases string, serviceType string) error {
	var serviceLookup node_types.ServiceLookup = node_types.ServiceLookup{
		Phrases: searchPhrases,
		Type:    serviceType,
	}
	return BroadcastMessage(sm.p2pm, serviceLookup)
}

func (sm *ServiceManager) SearchServices(searchService node_types.SearchService, params ...uint32) ([]node_types.ServiceOffer, error) {
	var services []node_types.ServiceOffer

	// Read configs
	configManager := utils.NewConfigManager("")
	config, err := configManager.ReadConfigs()
	if err != nil {
		message := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
		sm.lm.Log("error", message, "services")
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

	sql := `SELECT s.id, s.name, s.description, s.service_type, s.active
		FROM services s
		WHERE 1`

	// Parse search phrases (comma delimited) to search for
	searchPhrases := strings.Split(searchService.Phrases, ",")
	for i, searchPhrase := range searchPhrases {
		searchPhrase = strings.TrimSpace(searchPhrase)
		// Skip if empty string
		if searchPhrase == "" {
			continue
		}
		// Query for each name provided
		if i == 0 {
			sql = sql + fmt.Sprintf(" AND (LOWER(s.name) LIKE LOWER('%s') OR LOWER(s.description) LIKE LOWER('%s')", "%"+searchPhrase+"%", "%"+searchPhrase+"%")
		} else {
			sql = sql + fmt.Sprintf(" OR LOWER(s.name) LIKE LOWER('%s') OR LOWER(s.description) LIKE LOWER('%s')", "%"+searchPhrase+"%", "%"+searchPhrase+"%")
		}
		if i >= len(searchPhrases)-1 {
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

	// Filter service per provided active flag
	sql = sql + fmt.Sprintf(" AND s.active = %t", searchService.Active)

	// Add trailing semicolumn
	sql = sql + fmt.Sprintf(" limit %d offset %d;", limit, offset)

	// Search for services
	rows, err := sm.db.QueryContext(context.Background(), sql)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "services")
		return nil, err
	}
	//	defer rows.Close()

	for rows.Next() {
		var serviceOffer node_types.ServiceOffer
		err = rows.Scan(&serviceOffer.Id, &serviceOffer.Name, &serviceOffer.Description,
			&serviceOffer.Type, &serviceOffer.Active)
		if err != nil {
			msg := err.Error()
			sm.lm.Log("error", msg, "services")
			return nil, err
		}

		services = append(services, serviceOffer)
	}
	rows.Close()

	for i := range services {
		// Query service resources with prices
		sql = fmt.Sprintf(`SELECT r.resource_group, r.resource, r.resource_unit, r.description, p.price, c.currency, c.symbol
			FROM prices p
			INNER JOIN resources r ON p.resource_id = r.id
			INNER JOIN currencies c ON p.currency_symbol = c.symbol
			WHERE p.service_id = %d and r.active = true`, services[i].Id)

		rows, err = sm.db.QueryContext(context.Background(), sql)
		if err != nil {
			msg := err.Error()
			sm.lm.Log("error", msg, "services")
			return nil, err
		}

		// Service host node Id
		services[i].NodeId = sm.p2pm.h.ID().String()

		for rows.Next() {
			var serviceResource node_types.ServiceResourcesWithPricing
			err = rows.Scan(&serviceResource.ResourceGroup, &serviceResource.ResourceName, &serviceResource.ResourceUnit, &serviceResource.ResourceDescription,
				&serviceResource.Price, &serviceResource.CurrencyName, &serviceResource.CurrencySymbol)
			if err != nil {
				msg := err.Error()
				sm.lm.Log("warn", msg, "services")
				continue
			}
			services[i].ServicePriceModel = append(services[i].ServicePriceModel, serviceResource)
		}
		rows.Close()
	}

	return services, nil
}
