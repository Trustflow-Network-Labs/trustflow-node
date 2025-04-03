package resource

import (
	"context"
	"errors"
	"fmt"

	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/price"
	"github.com/adgsm/trustflow-node/utils"
)

type ResourceManager struct {
	sm *database.SQLiteManager
	lm *utils.LogsManager
}

func NewResourceManager() *ResourceManager {
	return &ResourceManager{
		sm: database.NewSQLiteManager(),
		lm: utils.NewLogsManager(),
	}
}

// Resource already added?
func (rm *ResourceManager) Exists(group, name, unit string) (error, bool) {
	if group == "" || name == "" || unit == "" {
		msg := "invalid resource"
		rm.lm.Log("error", msg, "resources")
		return errors.New(msg), false
	}

	// Create a database connection
	db, err := rm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err, false
	}
	defer db.Close()

	// Check if resource is already existing
	var r node_types.NullInt64
	row := db.QueryRowContext(context.Background(), "select id from resources where resource_group = ? and resource = ? and resource_unit = ?;",
		group, name, unit)

	err = row.Scan(&r)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("debug", msg, "resources")
		return nil, false
	}

	return nil, true
}

// Get resource
func (rm *ResourceManager) Get(id int64) (node_types.Resource, error) {
	var resource node_types.Resource
	if id <= 0 {
		msg := "invalid resource id"
		rm.lm.Log("error", msg, "resources")
		return resource, errors.New(msg)
	}

	// Create a database connection
	db, err := rm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return resource, err
	}
	defer db.Close()

	// Search for a resource
	row := db.QueryRowContext(context.Background(), "select id, resource_group, resource, resource_unit, description, active from resources where id = ?;", id)

	err = row.Scan(&resource.Id, &resource.ResourceGroup, &resource.Resource, &resource.ResourceUnit, &resource.Description, &resource.Active)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("debug", msg, "resources")
		return resource, nil
	}

	return resource, nil
}

// List resources
func (rm *ResourceManager) List() ([]node_types.Resource, error) {
	// Create a database connection
	db, err := rm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return nil, err
	}
	defer db.Close()

	// Load resources
	rows, err := db.QueryContext(context.Background(), "select id, resource_group, resource, resource_unit, description, active from resources;")
	if err != nil {
		rm.lm.Log("error", err.Error(), "resources")
		return nil, err
	}
	defer rows.Close()

	var resaources []node_types.Resource
	for rows.Next() {
		var resource node_types.Resource
		if err := rows.Scan(&resource.Id, &resource.ResourceGroup, &resource.Resource, &resource.ResourceUnit, &resource.Description, &resource.Active); err == nil {
			resaources = append(resaources, resource)
		}
	}

	return resaources, rows.Err()
}

// Add a resource
func (rm *ResourceManager) Add(group, name, unit, description string, active bool) error {
	// Check if resource is already existing
	err, existing := rm.Exists(group, name, unit)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}
	if existing {
		err = fmt.Errorf("resource %s %s is already existing", name, unit)
		rm.lm.Log("warn", err.Error(), "resources")
		return err
	}

	// Create a database connection
	db, err := rm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}
	defer db.Close()

	// Add resource
	rm.lm.Log("debug", fmt.Sprintf("add resource %s %s", name, unit), "resources")

	_, err = db.ExecContext(context.Background(), "insert into resources (resource_group, resource, resource_unit, description, active) values (?, ?, ?, ?, ?);",
		group, name, unit, description, active)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}

	return nil
}

// Remove resource
func (rm *ResourceManager) Remove(id int64) error {
	// Check if resource is already existing
	_, err := rm.Get(id)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}

	// Create a database connection
	db, err := rm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}
	defer db.Close()

	// Check if there are existing prices defined using this resource
	priceManager := price.NewPriceManager()
	prices, err := priceManager.GetPricesByResourceId(id)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}
	if len(prices) > 0 {
		err = fmt.Errorf("resource id %d is used with %d pricings defined. Please remove pricings for this resource first", id, len(prices))
		rm.lm.Log("warn", err.Error(), "resources")
		return err
	}

	// Remove resource
	rm.lm.Log("debug", fmt.Sprintf("removing resource %d", id), "resources")

	_, err = db.ExecContext(context.Background(), "delete from resources where id = ?;", id)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}

	return nil
}

// Set resource inactive
func (rm *ResourceManager) SetInactive(id int64) error {
	// Check if resource is already existing
	_, err := rm.Get(id)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}

	// Create a database connection
	db, err := rm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}
	defer db.Close()

	// Check if there are existing prices defined using this resource
	priceManager := price.NewPriceManager()
	prices, err := priceManager.GetPricesByResourceId(id)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}

	if len(prices) > 0 {
		err = fmt.Errorf("resource id %d is used with %d pricings defined. Please remove pricings for this resource first", id, len(prices))
		rm.lm.Log("warn", err.Error(), "resources")
		return err
	}

	// Set resource inactive
	rm.lm.Log("debug", fmt.Sprintf("setting resource id %d inactive", id), "resources")

	_, err = db.ExecContext(context.Background(), "update resources set active = false where id = ?;", id)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}

	return nil
}

// Set resource active
func (rm *ResourceManager) SetActive(id int64) error {
	// Check if resource is already existing
	_, err := rm.Get(id)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}

	// Create a database connection
	db, err := rm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}
	defer db.Close()

	// Set resource active
	rm.lm.Log("debug", fmt.Sprintf("setting resource id %d active", id), "resources")

	_, err = db.ExecContext(context.Background(), "update resources set active = true where id = ?;", id)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}

	return nil
}
