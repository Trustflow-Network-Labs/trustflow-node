package price

import (
	"context"
	"errors"
	"fmt"

	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/utils"
)

type PriceManager struct {
	sm *database.SQLiteManager
	lm *utils.LogsManager
}

func NewPriceManager() *PriceManager {
	return &PriceManager{
		sm: database.NewSQLiteManager(),
		lm: utils.NewLogsManager(),
	}
}

// Get prices by currency
func (pm *PriceManager) GetPricesByCurrency(symbol string, params ...uint32) ([]node_types.Price, error) {
	var price node_types.Price
	var prices []node_types.Price

	if symbol == "" {
		msg := "invalid currency"
		pm.lm.Log("error", msg, "prices")
		return prices, errors.New(msg)
	}

	// Create a database connection
	db, err := pm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		pm.lm.Log("error", msg, "prices")
		return prices, err
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

	// Search for prices
	rows, err := db.QueryContext(context.Background(), "select id, service_id, resource_id, price, currency_symbol from prices where currency_symbol = ? limit ? offset ?;",
		symbol, limit, offset)
	if err != nil {
		msg := err.Error()
		pm.lm.Log("error", msg, "prices")
		return prices, err
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&price.Id, &price.ServiceId, &price.ResourceId, &price.Price, &price.Currency)
		if err != nil {
			msg := err.Error()
			pm.lm.Log("error", msg, "prices")
			return prices, err
		}
		prices = append(prices, price)
	}

	return prices, nil
}

// Get prices by resource
func (pm *PriceManager) GetPricesByResourceId(resourceId int64, params ...uint32) ([]node_types.Price, error) {
	var price node_types.Price
	var prices []node_types.Price

	if resourceId <= 0 {
		msg := "invalid resource"
		pm.lm.Log("error", msg, "prices")
		return prices, errors.New(msg)
	}

	// Create a database connection
	db, err := pm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		pm.lm.Log("error", msg, "prices")
		return prices, err
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

	// Search for prices
	rows, err := db.QueryContext(context.Background(), "select id, service_id, resource_id, price, currency_symbol from prices where resource_id = ? limit ? offset ?;",
		resourceId, limit, offset)
	if err != nil {
		msg := err.Error()
		pm.lm.Log("error", msg, "prices")
		return prices, err
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&price.Id, &price.ServiceId, &price.ResourceId, &price.Price, &price.Currency)
		if err != nil {
			msg := err.Error()
			pm.lm.Log("error", msg, "prices")
			return prices, err
		}
		prices = append(prices, price)
	}

	return prices, nil
}

// Get prices by service ID
func (pm *PriceManager) GetPricesByServiceId(serviceId int64, params ...uint32) ([]node_types.Price, error) {
	var price node_types.Price
	var prices []node_types.Price

	if serviceId <= 0 {
		msg := "invalid service ID"
		pm.lm.Log("error", msg, "prices")
		return prices, errors.New(msg)
	}

	// Create a database connection
	db, err := pm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		pm.lm.Log("error", msg, "prices")
		return prices, err
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

	// Search for prices
	rows, err := db.QueryContext(context.Background(), "select id, service_id, resource_id, price, currency_symbol from prices where service_id = ? limit ? offset ?;",
		serviceId, limit, offset)
	if err != nil {
		msg := err.Error()
		pm.lm.Log("error", msg, "prices")
		return prices, err
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&price.Id, &price.ServiceId, &price.ResourceId, &price.Price, &price.Currency)
		if err != nil {
			msg := err.Error()
			pm.lm.Log("error", msg, "prices")
			return prices, err
		}
		prices = append(prices, price)
	}

	return prices, nil
}

// Add a price
func (pm *PriceManager) Add(serviceId int64, resourceId int64, price float64, currency string) (int64, error) {
	// Create a database connection
	db, err := pm.sm.CreateConnection()
	if err != nil {
		pm.lm.Log("error", err.Error(), "prices")
		return 0, err
	}
	defer db.Close()

	// Add price
	pm.lm.Log("debug", fmt.Sprintf("add price %.02f %s for service Id %d and service resource id %d",
		price, currency, serviceId, resourceId), "prices")

	result, err := db.ExecContext(context.Background(), "insert into prices (service_id, resource_id, price, currency_symbol) values (?, ?, ?, ?);",
		serviceId, resourceId, price, currency)
	if err != nil {
		pm.lm.Log("error", err.Error(), "prices")
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		pm.lm.Log("error", err.Error(), "prices")
		return 0, err
	}

	return id, nil
}

// Remove prices defined for a service
func (pm *PriceManager) RemoveForService(serviceId int64) error {
	// Create a database connection
	db, err := pm.sm.CreateConnection()
	if err != nil {
		pm.lm.Log("error", err.Error(), "prices")
		return err
	}
	defer db.Close()

	// Remove prices
	pm.lm.Log("debug", fmt.Sprintf("remove prices for service Id %d", serviceId), "prices")

	_, err = db.ExecContext(context.Background(), "delete from prices where service_id = ?;", serviceId)
	if err != nil {
		pm.lm.Log("error", err.Error(), "prices")
		return err
	}

	return nil
}
