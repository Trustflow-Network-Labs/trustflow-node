package price

import (
	"context"
	"errors"

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
	rows, err := db.QueryContext(context.Background(), "select id, service_id, resource, currency_symbol, price, price_unit_normalizator, price_interval from prices where currency_symbol = ? limit ? offset ?;",
		symbol, limit, offset)
	if err != nil {
		msg := err.Error()
		pm.lm.Log("error", msg, "prices")
		return prices, err
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&price)
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
func (pm *PriceManager) GetPricesByResource(resource string, params ...uint32) ([]node_types.Price, error) {
	var price node_types.Price
	var prices []node_types.Price

	if resource == "" {
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
	rows, err := db.QueryContext(context.Background(), "select id, service_id, resource, currency_symbol, price, price_unit_normalizator, price_interval from prices where resource = ? limit ? offset ?;",
		resource, limit, offset)
	if err != nil {
		msg := err.Error()
		pm.lm.Log("error", msg, "prices")
		return prices, err
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&price)
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
	rows, err := db.QueryContext(context.Background(), "select id, service_id, resource, currency_symbol, price, price_unit_normalizator, price_interval from prices where service_id = ? limit ? offset ?;",
		serviceId, limit, offset)
	if err != nil {
		msg := err.Error()
		pm.lm.Log("error", msg, "prices")
		return prices, err
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&price)
		if err != nil {
			msg := err.Error()
			pm.lm.Log("error", msg, "prices")
			return prices, err
		}
		prices = append(prices, price)
	}

	return prices, nil
}
