package price

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/Trustflow-Network-Labs/trustflow-node/internal/node_types"
	"github.com/Trustflow-Network-Labs/trustflow-node/internal/utils"
)

type PriceManager struct {
	db *sql.DB
	lm *utils.LogsManager
}

func NewPriceManager(db *sql.DB, lm *utils.LogsManager) *PriceManager {
	return &PriceManager{
		db: db,
		lm: lm,
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

	var offset uint32 = 0
	var limit uint32 = 10
	if len(params) == 1 {
		offset = params[0]
	} else if len(params) >= 2 {
		offset = params[0]
		limit = params[1]
	}

	// Search for prices
	rows, err := pm.db.QueryContext(context.Background(), "select id, service_id, resource_id, price, currency_symbol from prices where currency_symbol = ? limit ? offset ?;",
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

	var offset uint32 = 0
	var limit uint32 = 10
	if len(params) == 1 {
		offset = params[0]
	} else if len(params) >= 2 {
		offset = params[0]
		limit = params[1]
	}

	// Search for prices
	rows, err := pm.db.QueryContext(context.Background(), "select id, service_id, resource_id, price, currency_symbol from prices where resource_id = ? limit ? offset ?;",
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

	var offset uint32 = 0
	var limit uint32 = 10
	if len(params) == 1 {
		offset = params[0]
	} else if len(params) >= 2 {
		offset = params[0]
		limit = params[1]
	}

	// Search for prices
	rows, err := pm.db.QueryContext(context.Background(), "select id, service_id, resource_id, price, currency_symbol from prices where service_id = ? limit ? offset ?;",
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
	// Add price
	pm.lm.Log("debug", fmt.Sprintf("add price %.02f %s for service Id %d and service resource id %d",
		price, currency, serviceId, resourceId), "prices")

	result, err := pm.db.ExecContext(context.Background(), "insert into prices (service_id, resource_id, price, currency_symbol) values (?, ?, ?, ?);",
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
	// Remove prices
	pm.lm.Log("debug", fmt.Sprintf("remove prices for service Id %d", serviceId), "prices")

	_, err := pm.db.ExecContext(context.Background(), "delete from prices where service_id = ?;", serviceId)
	if err != nil {
		pm.lm.Log("error", err.Error(), "prices")
		return err
	}

	return nil
}
