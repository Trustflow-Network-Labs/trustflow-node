package price

import (
	"context"
	"errors"

	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/utils"
)

type PriceManager struct {
}

func NewPriceManager() *PriceManager {
	return &PriceManager{}
}

// Get prices by currency ID
func (pm *PriceManager) GetPricesByCurrencyId(currencyId int32, params ...uint32) ([]node_types.Price, error) {
	var price node_types.Price
	var prices []node_types.Price
	logsManager := utils.NewLogsManager()

	if currencyId <= 0 {
		msg := "invalid currency ID"
		logsManager.Log("error", msg, "prices")
		return prices, errors.New(msg)
	}

	// Create a database connection
	sqlManager := database.NewSQLiteManager()
	db, err := sqlManager.CreateConnection()
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "prices")
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
	rows, err := db.QueryContext(context.Background(), "select id, service_id, resource_id, currency_id, price, price_unit_normalizator, price_interval from prices where currency_id = ? limit ? offset ?;",
		currencyId, limit, offset)
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "prices")
		return prices, err
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&price)
		if err != nil {
			msg := err.Error()
			logsManager.Log("error", msg, "prices")
			return prices, err
		}
		prices = append(prices, price)
	}

	return prices, nil
}

// Get prices by resource ID
func (pm *PriceManager) GetPricesByResourceId(resourceId int32, params ...uint32) ([]node_types.Price, error) {
	var price node_types.Price
	var prices []node_types.Price
	logsManager := utils.NewLogsManager()

	if resourceId <= 0 {
		msg := "invalid resource ID"
		logsManager.Log("error", msg, "prices")
		return prices, errors.New(msg)
	}

	// Create a database connection
	sqlManager := database.NewSQLiteManager()
	db, err := sqlManager.CreateConnection()
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "prices")
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
	rows, err := db.QueryContext(context.Background(), "select id, service_id, resource_id, currency_id, price, price_unit_normalizator, price_interval from prices where resource_id = ? limit ? offset ?;",
		resourceId, limit, offset)
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "prices")
		return prices, err
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&price)
		if err != nil {
			msg := err.Error()
			logsManager.Log("error", msg, "prices")
			return prices, err
		}
		prices = append(prices, price)
	}

	return prices, nil
}

// Get prices by service ID
func (pm *PriceManager) GetPricesByServiceId(serviceId int32, params ...uint32) ([]node_types.Price, error) {
	var price node_types.Price
	var prices []node_types.Price
	logsManager := utils.NewLogsManager()

	if serviceId <= 0 {
		msg := "invalid service ID"
		logsManager.Log("error", msg, "prices")
		return prices, errors.New(msg)
	}

	// Create a database connection
	sqlManager := database.NewSQLiteManager()
	db, err := sqlManager.CreateConnection()
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "prices")
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
	rows, err := db.QueryContext(context.Background(), "select id, service_id, resource_id, currency_id, price, price_unit_normalizator, price_interval from prices where service_id = ? limit ? offset ?;",
		serviceId, limit, offset)
	if err != nil {
		msg := err.Error()
		logsManager.Log("error", msg, "prices")
		return prices, err
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&price)
		if err != nil {
			msg := err.Error()
			logsManager.Log("error", msg, "prices")
			return prices, err
		}
		prices = append(prices, price)
	}

	return prices, nil
}
