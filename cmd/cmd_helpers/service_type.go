package cmd_helpers

import (
	"context"
	"errors"
	"fmt"

	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/utils"
)

// ServiceType already added?
func ServiceTypeExists(name string) (error, bool) {
	if name == "" {
		msg := "invalid service type name"
		utils.Log("error", msg, "service types")
		return errors.New(msg), false
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "service types")
		return err, false
	}
	defer db.Close()

	// Check if service type is already existing
	var id node_types.NullInt32
	row := db.QueryRowContext(context.Background(), "select id from service_types where name = ?;", name)

	err = row.Scan(&id)
	if err != nil {
		msg := err.Error()
		utils.Log("debug", msg, "service types")
		return nil, false
	}

	return nil, true
}

// Get service type by name
func GetServiceTypeByName(name string) (node_types.ServiceType, error) {
	var serviceType node_types.ServiceType
	if name == "" {
		msg := "invalid service type name"
		utils.Log("error", msg, "service types")
		return serviceType, errors.New(msg)
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "service types")
		return serviceType, err
	}
	defer db.Close()

	// Search for a service type
	row := db.QueryRowContext(context.Background(), "select id, name from service_types where name = ?;", name)

	err = row.Scan(&serviceType.Id, &serviceType.Name)
	if err != nil {
		msg := err.Error()
		utils.Log("debug", msg, "service types")
		return serviceType, nil
	}

	return serviceType, nil
}

// Add a service type
func AddServiceType(name string) {
	err, existing := ServiceTypeExists(name)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "service types")
		return
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "service types")
		return
	}
	defer db.Close()

	// Check if service type is already existing
	if existing {
		msg := fmt.Sprintf("Service type %s is already existing", name)
		utils.Log("warn", msg, "service types")
		return
	}

	// Add service type
	utils.Log("debug", fmt.Sprintf("add service type %s", name), "service types")

	_, err = db.ExecContext(context.Background(), "insert into service_types (name) values (?);", name)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "service types")
		return
	}
}

// Remove service type
func RemoveServiceType(name string) {
	err, existing := ServiceTypeExists(name)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "service types")
		return
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "service types")
		return
	}
	defer db.Close()

	// Check if service type is already existing
	if !existing {
		msg := fmt.Sprintf("Service type %s is not existing in the database. Nothing to remove", name)
		utils.Log("warn", msg, "service types")
		return
	}

	// Check if there are existing prices defined using this service type
	serviceType, err := GetServiceTypeByName(name)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "service types")
		return
	}

	prices, err := GetServicesByServiceTypeId(serviceType.Id.Int32)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "service types")
		return
	}
	if len(prices) > 0 {
		msg := fmt.Sprintf("Service type %s is used with %d services defined. Please remove services for this service type first", name, len(prices))
		utils.Log("warn", msg, "service types")
		return
	}

	// Remove service type
	utils.Log("debug", fmt.Sprintf("removing service type %s", name), "service types")

	_, err = db.ExecContext(context.Background(), "delete from service_types where name = ?;", name)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "service types")
		return
	}
}

// Set service type inactive
func SetServiceTypeInactive(name string) {
	err, existing := ServiceTypeExists(name)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "service types")
		return
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "service types")
		return
	}
	defer db.Close()

	// Check if service type is already existing
	if !existing {
		msg := fmt.Sprintf("Service type %s is not existing in the database. Nothing to set inactive", name)
		utils.Log("warn", msg, "service types")
		return
	}

	// Check if there are existing prices defined using this service type
	serviceType, err := GetServiceTypeByName(name)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "service types")
		return
	}

	prices, err := GetServicesByServiceTypeId(serviceType.Id.Int32)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "service types")
		return
	}

	if len(prices) > 0 {
		msg := fmt.Sprintf("Service type %s is used with %d services defined. Please remove services for this service type first", name, len(prices))
		utils.Log("warn", msg, "service types")
		return
	}

	// Set service type inactive
	utils.Log("debug", fmt.Sprintf("setting service type %s inactive", name), "service types")

	_, err = db.ExecContext(context.Background(), "update service_types set active = false where name = ?;", name)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "service types")
		return
	}
}

// Set service type active
func SetServiceTypeActive(name string) {
	err, existing := ServiceTypeExists(name)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "service types")
		return
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "service types")
		return
	}
	defer db.Close()

	// Check if service type is already existing
	if !existing {
		msg := fmt.Sprintf("Service type %s is not existing in the database. Nothing to set active", name)
		utils.Log("warn", msg, "service types")
		return
	}

	// Set service type active
	utils.Log("debug", fmt.Sprintf("setting service type %s active", name), "service types")

	_, err = db.ExecContext(context.Background(), "update service_types set active = true where name = ?;", name)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "service types")
		return
	}
}
