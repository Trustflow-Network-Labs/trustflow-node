package cmd_helpers

import (
	"context"
	"errors"
	"fmt"

	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/utils"
)

// Service already added?
func ServiceExists(id int32) (error, bool) {
	if id <= 0 {
		msg := "invalid service id"
		utils.Log("error", msg, "servics")
		return errors.New(msg), false
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "servics")
		return err, false
	}
	defer db.Close()

	// Check if service is already existing
	var serviceId node_types.NullInt32
	row := db.QueryRowContext(context.Background(), "select id from services where id = ?;", id)

	err = row.Scan(&serviceId)
	if err != nil {
		msg := err.Error()
		utils.Log("debug", msg, "servics")
		return nil, false
	}

	return nil, true
}

// Get Service by ID
func GetService(id int32) (node_types.Service, error) {
	var service node_types.Service
	if id <= 0 {
		msg := "invalid service id"
		utils.Log("error", msg, "servics")
		return service, errors.New(msg)
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "servics")
		return service, err
	}
	defer db.Close()

	// Get service
	row := db.QueryRowContext(context.Background(), "select id, name, description, node_id, service_type_id, active from services where id = ?;", id)

	err = row.Scan(&service)
	if err != nil {
		msg := err.Error()
		utils.Log("debug", msg, "servics")
		return service, err
	}

	return service, nil
}

// Get services by service type ID
func GetServicesByServiceTypeId(serviceTypeId int32, params ...uint32) ([]node_types.Service, error) {
	var service node_types.Service
	var services []node_types.Service
	if serviceTypeId <= 0 {
		msg := "invalid service type ID"
		utils.Log("error", msg, "services")
		return services, errors.New(msg)
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "services")
		return services, err
	}
	defer db.Close()

	var offset uint32 = 0
	var limit uint32 = 10
	if len(params) == 1 {
		offset = params[0]
	} else if len(params) >= 2 {
		offset = params[0]
		limit = params[1]
	}

	// Search for services
	rows, err := db.QueryContext(context.Background(), "select id, name, description, node_id, service_type_id, active from services where service_type_id = ? limit ? offset ?;",
		serviceTypeId, limit, offset)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "services")
		return services, err
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&service)
		if err != nil {
			msg := err.Error()
			utils.Log("error", msg, "services")
			return services, err
		}
		services = append(services, service)
	}

	return services, nil
}

// Add a service
func AddService(name string, description string, node_id int32, service_type_id int32, active bool) {
	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "services")
		return
	}
	defer db.Close()

	// Add service
	utils.Log("debug", fmt.Sprintf("add service %s", name), "services")

	_, err = db.ExecContext(context.Background(), "insert into services (name, description, node_id, service_type_id, active) values (?, ?, ?, ?, ?);",
		name, description, node_id, service_type_id, active)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "services")
		return
	}
}

// Remove service
func RemoveService(id int32) {
	err, existing := ServiceExists(id)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "services")
		return
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "services")
		return
	}
	defer db.Close()

	// Check if service is already existing
	if !existing {
		msg := fmt.Sprintf("Service id %d is not existing in the database. Nothing to remove", id)
		utils.Log("warn", msg, "services")
		return
	}

	// Check if there are jobs executed using this service
	jobs, err := GetJobsByServiceId(id)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "services")
		return
	}
	if len(jobs) > 0 {
		msg := fmt.Sprintf("Service id %d was used with %d jobs executed. You can not remove this service but you can set it service inactive", id, len(jobs))
		utils.Log("warn", msg, "services")
		return
	}

	// Check if there are existing prices defined using this service
	prices, err := GetPricesByServiceId(id)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "services")
		return
	}
	if len(prices) > 0 {
		msg := fmt.Sprintf("Service id %d is used with %d prices defined. Please remove prices for this service first", id, len(prices))
		utils.Log("warn", msg, "services")
		return
	}

	// Remove service
	utils.Log("debug", fmt.Sprintf("removing service %d", id), "services")

	_, err = db.ExecContext(context.Background(), "delete from services where id = ?;", id)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "services")
		return
	}
}

// Set service inactive
func SetServiceInactive(id int32) {
	err, existing := ServiceExists(id)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "services")
		return
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "services")
		return
	}
	defer db.Close()

	// Check if service is already existing
	if !existing {
		msg := fmt.Sprintf("Service id %d is not existing in the database. Nothing to set inactive", id)
		utils.Log("warn", msg, "services")
		return
	}

	// Check if there are existing prices defined using this service
	prices, err := GetPricesByServiceId(id)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "services")
		return
	}
	if len(prices) > 0 {
		msg := fmt.Sprintf("Service id %d is used with %d prices defined. Please remove prices for this service first", id, len(prices))
		utils.Log("warn", msg, "services")
		return
	}

	// Set service inactive
	utils.Log("debug", fmt.Sprintf("setting service id %d inactive", id), "services")

	_, err = db.ExecContext(context.Background(), "update services set active = false where id = ?;", id)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "services")
		return
	}
}

// Set service active
func SetServiceActive(id int32) {
	err, existing := ServiceExists(id)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "services")
		return
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "services")
		return
	}
	defer db.Close()

	// Check if service is already existing
	if !existing {
		msg := fmt.Sprintf("Service id %d is not existing in the database. Nothing to set active", id)
		utils.Log("warn", msg, "services")
		return
	}

	// Set service active
	utils.Log("debug", fmt.Sprintf("setting service id %d active", id), "services")

	_, err = db.ExecContext(context.Background(), "update services set active = true where id = ?;", id)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "services")
		return
	}
}
