package resource

import (
	"context"
	"errors"
	"fmt"

	"github.com/adgsm/trustflow-node/cmd/price"
	resource_utilization "github.com/adgsm/trustflow-node/cmd/resource-utilization"
	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/utils"
)

type ResourceManager struct {
}

func NewResourceManager() *ResourceManager {
	return &ResourceManager{}
}

// Resource already added?
func (rm *ResourceManager) ResourceExists(name string) (error, bool) {
	logsManager := utils.NewLogsManager()
	if name == "" {
		msg := "invalid resource name"
		logsManager.Log("error", msg, "resources")
		return errors.New(msg), false
	}

	// Create a database connection
	sqlManager := database.NewSQLiteManager()
	db, err := sqlManager.CreateConnection()
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "resources")
		return err, false
	}
	defer db.Close()

	// Check if resource is already existing
	var id node_types.NullInt32
	row := db.QueryRowContext(context.Background(), "select id from resources where name = ?;", name)

	err = row.Scan(&id)
	if err != nil {
		msg := err.Error()
		logsManager.Log("debug", msg, "resources")
		return nil, false
	}

	return nil, true
}

// Get resource by name
func (rm *ResourceManager) GetResourceByName(name string) (node_types.Resource, error) {
	logsManager := utils.NewLogsManager()
	var resource node_types.Resource
	if name == "" {
		msg := "invalid resource name"
		logsManager.Log("error", msg, "resources")
		return resource, errors.New(msg)
	}

	// Create a database connection
	sqlManager := database.NewSQLiteManager()
	db, err := sqlManager.CreateConnection()
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "resources")
		return resource, err
	}
	defer db.Close()

	// Search for a resource
	row := db.QueryRowContext(context.Background(), "select id, name from resources where name = ?;", name)

	err = row.Scan(&resource.Id, &resource.Name)
	if err != nil {
		msg := err.Error()
		logsManager.Log("debug", msg, "resources")
		return resource, nil
	}

	return resource, nil
}

// Add a resource
func (rm *ResourceManager) AddResource(name string) {
	logsManager := utils.NewLogsManager()
	err, existing := rm.ResourceExists(name)
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "resources")
		return
	}

	// Create a database connection
	sqlManager := database.NewSQLiteManager()
	db, err := sqlManager.CreateConnection()
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "resources")
		return
	}
	defer db.Close()

	// Check if resource is already existing
	if existing {
		msg := fmt.Sprintf("Resource %s is already existing", name)
		logsManager.Log("warn", msg, "resources")
		return
	}

	// Add resource
	logsManager.Log("debug", fmt.Sprintf("add resource %s", name), "resources")

	_, err = db.ExecContext(context.Background(), "insert into resources (name) values (?);", name)
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "resources")
		return
	}
}

// Remove resource
func (rm *ResourceManager) RemoveResource(name string) {
	logsManager := utils.NewLogsManager()
	err, existing := rm.ResourceExists(name)
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "resources")
		return
	}

	// Create a database connection
	sqlManager := database.NewSQLiteManager()
	db, err := sqlManager.CreateConnection()
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "resources")
		return
	}
	defer db.Close()

	// Check if resource is already existing
	if !existing {
		msg := fmt.Sprintf("Resource %s is not existing in the database. Nothing to remove", name)
		logsManager.Log("warn", msg, "resources")
		return
	}

	// Check if there are existing previous resource utilizations
	resource, err := rm.GetResourceByName(name)
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "resources")
		return
	}

	resourceUtilizationManager := resource_utilization.NewResourceUtilizationManager()
	utilizations, err := resourceUtilizationManager.GetUtilizationsByResourceId(resource.Id)
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "resources")
		return
	}
	if len(utilizations) > 0 {
		msg := fmt.Sprintf("Resource %s was utilized in %d job(s) previously and it can not be deleted. You can set this resource inactive if you do not want to utilize it any more",
			name, len(utilizations))
		logsManager.Log("warn", msg, "resources")
		return
	}

	// Check if there are existing prices defined using this resource
	priceManager := price.NewPriceManager()
	prices, err := priceManager.GetPricesByResourceId(resource.Id)
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "resources")
		return
	}
	if len(prices) > 0 {
		msg := fmt.Sprintf("Resource %s is used with %d pricings defined. Please remove pricings for this resource first", name, len(prices))
		logsManager.Log("warn", msg, "resources")
		return
	}

	// Remove resource
	logsManager.Log("debug", fmt.Sprintf("removing resource %s", name), "resources")

	_, err = db.ExecContext(context.Background(), "delete from resources where name = ?;", name)
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "resources")
		return
	}
}

// Set resource inactive
func (rm *ResourceManager) SetResourceInactive(name string) {
	logsManager := utils.NewLogsManager()
	err, existing := rm.ResourceExists(name)
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "resources")
		return
	}

	// Create a database connection
	sqlManager := database.NewSQLiteManager()
	db, err := sqlManager.CreateConnection()
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "resources")
		return
	}
	defer db.Close()

	// Check if resource is already existing
	if !existing {
		msg := fmt.Sprintf("Resource %s is not existing in the database. Nothing to set inactive", name)
		logsManager.Log("warn", msg, "resources")
		return
	}

	// Check if there are existing prices defined using this resource
	resource, err := rm.GetResourceByName(name)
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "resources")
		return
	}

	priceManager := price.NewPriceManager()
	prices, err := priceManager.GetPricesByResourceId(resource.Id)
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "resources")
		return
	}

	if len(prices) > 0 {
		msg := fmt.Sprintf("Resource %s is used with %d pricings defined. Please remove pricings for this resource first", name, len(prices))
		logsManager.Log("warn", msg, "resources")
		return
	}

	// Set resource inactive
	logsManager.Log("debug", fmt.Sprintf("setting resource %s inactive", name), "resources")

	_, err = db.ExecContext(context.Background(), "update resources set active = false where name = ?;", name)
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "resources")
		return
	}
}

// Set resource active
func (rm *ResourceManager) SetResourceActive(name string) {
	logsManager := utils.NewLogsManager()
	err, existing := rm.ResourceExists(name)
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "resources")
		return
	}

	// Create a database connection
	sqlManager := database.NewSQLiteManager()
	db, err := sqlManager.CreateConnection()
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "resources")
		return
	}
	defer db.Close()

	// Check if resource is already existing
	if !existing {
		msg := fmt.Sprintf("Resource %s is not existing in the database. Nothing to set active", name)
		logsManager.Log("warn", msg, "resources")
		return
	}

	// Set resource active
	logsManager.Log("debug", fmt.Sprintf("setting resource %s active", name), "resources")

	_, err = db.ExecContext(context.Background(), "update resources set active = true where name = ?;", name)
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "resources")
		return
	}
}
