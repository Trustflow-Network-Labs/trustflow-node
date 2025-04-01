package resource

import (
	"context"
	"errors"
	"fmt"

	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/price"
	resource_utilization "github.com/adgsm/trustflow-node/resource-utilization"
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

func (rm *ResourceManager) IsResource(s string) error {
	err, b := rm.Exists(s)
	if err != nil {
		return err
	}
	if !b {
		err = fmt.Errorf("%s is not an existing resource", s)
		return err
	}
	return nil
}

// Resource already added?
func (rm *ResourceManager) Exists(name string) (error, bool) {
	if name == "" {
		msg := "invalid resource name"
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
	var r node_types.NullString
	row := db.QueryRowContext(context.Background(), "select resource from resources where resource = ?;", name)

	err = row.Scan(&r)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("debug", msg, "resources")
		return nil, false
	}

	return nil, true
}

// Get resource
func (rm *ResourceManager) Get(name string) (node_types.Resource, error) {
	var resource node_types.Resource
	if name == "" {
		msg := "invalid resource name"
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
	row := db.QueryRowContext(context.Background(), "select resource, description, active from resources where resource = ?;", name)

	err = row.Scan(&resource.Resource, &resource.Description, &resource.Active)
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
	rows, err := db.QueryContext(context.Background(), "select resource, description, active from resources;")
	if err != nil {
		rm.lm.Log("error", err.Error(), "resources")
		return nil, err
	}
	defer rows.Close()

	var resaources []node_types.Resource
	for rows.Next() {
		var resource node_types.Resource
		if err := rows.Scan(&resource.Resource, &resource.Description, &resource.Active); err == nil {
			resaources = append(resaources, resource)
		}
	}

	return resaources, rows.Err()
}

// Add a resource
func (rm *ResourceManager) Add(name string, description string, active bool) error {
	// Check if resource is already existing
	err, existing := rm.Exists(name)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}
	if existing {
		err = fmt.Errorf("resource %s is already existing", name)
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
	rm.lm.Log("debug", fmt.Sprintf("add resource %s", name), "resources")

	_, err = db.ExecContext(context.Background(), "insert into resources (resource, description, active) values (?, ?, ?);",
		name, description, active)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}

	return nil
}

// Remove resource
func (rm *ResourceManager) Remove(name string) error {
	// Check if resource is already existing
	err, existing := rm.Exists(name)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}
	if !existing {
		err = fmt.Errorf("resource %s is not existing in the database. Nothing to remove", name)
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

	// Check if there are existing previous resource utilizations
	resourceUtilizationManager := resource_utilization.NewResourceUtilizationManager()
	utilizations, err := resourceUtilizationManager.GetUtilizationsByResource(name)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}
	if len(utilizations) > 0 {
		err = fmt.Errorf("resource %s was utilized in %d job(s) previously and it can not be deleted. You can set this resource inactive if you do not want to utilize it any more",
			name, len(utilizations))
		rm.lm.Log("warn", err.Error(), "resources")
		return err
	}

	// Check if there are existing prices defined using this resource
	priceManager := price.NewPriceManager()
	prices, err := priceManager.GetPricesByResource(name)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}
	if len(prices) > 0 {
		err = fmt.Errorf("resource %s is used with %d pricings defined. Please remove pricings for this resource first", name, len(prices))
		rm.lm.Log("warn", err.Error(), "resources")
		return err
	}

	// Remove resource
	rm.lm.Log("debug", fmt.Sprintf("removing resource %s", name), "resources")

	_, err = db.ExecContext(context.Background(), "delete from resources where resource = ?;", name)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}

	return nil
}

// Set resource inactive
func (rm *ResourceManager) SetInactive(name string) error {
	// Check if resource is already existing
	err, existing := rm.Exists(name)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}
	if !existing {
		err = fmt.Errorf("resource %s is not existing in the database. Nothing to set inactive", name)
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

	// Check if there are existing prices defined using this resource
	priceManager := price.NewPriceManager()
	prices, err := priceManager.GetPricesByResource(name)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}

	if len(prices) > 0 {
		err = fmt.Errorf("resource %s is used with %d pricings defined. Please remove pricings for this resource first", name, len(prices))
		rm.lm.Log("warn", err.Error(), "resources")
		return err
	}

	// Set resource inactive
	rm.lm.Log("debug", fmt.Sprintf("setting resource %s inactive", name), "resources")

	_, err = db.ExecContext(context.Background(), "update resources set active = false where resource = ?;", name)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}

	return nil
}

// Set resource active
func (rm *ResourceManager) SetActive(name string) error {
	// Check if resource is already existing
	err, existing := rm.Exists(name)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}
	if !existing {
		err = fmt.Errorf("resource %s is not existing in the database. Nothing to set active", name)
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

	// Set resource active
	rm.lm.Log("debug", fmt.Sprintf("setting resource %s active", name), "resources")

	_, err = db.ExecContext(context.Background(), "update resources set active = true where resource = ?;", name)
	if err != nil {
		msg := err.Error()
		rm.lm.Log("error", msg, "resources")
		return err
	}

	return nil
}
