package currency

import (
	"context"
	"errors"
	"fmt"

	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/price"
	"github.com/adgsm/trustflow-node/utils"
)

type CurrencyManager struct {
	sm *database.SQLiteManager
	lm *utils.LogsManager
	pm *price.PriceManager
}

func NewCurrencyManager() *CurrencyManager {
	return &CurrencyManager{
		sm: database.NewSQLiteManager(),
		lm: utils.NewLogsManager(),
		pm: price.NewPriceManager(),
	}
}

// Currency already added?
func (cm *CurrencyManager) Exists(symbol string) (error, bool) {
	if symbol == "" {
		msg := "invalid currency symbol"
		cm.lm.Log("error", msg, "currencies")
		return errors.New(msg), false
	}

	// Create a database connection
	db, err := cm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		cm.lm.Log("error", msg, "currencies")
		return err, false
	}
	defer db.Close()

	// Check if currency already existing
	var currency node_types.NullString
	row := db.QueryRowContext(context.Background(), "select currency from currencies where symbol = ?;", symbol)

	err = row.Scan(&currency)
	if err != nil {
		msg := err.Error()
		cm.lm.Log("debug", msg, "currencies")
		return nil, false
	}

	return nil, true
}

// Get currency by symbol
func (cm *CurrencyManager) Get(symbol string) (node_types.Currency, error) {
	var currency node_types.Currency
	if symbol == "" {
		msg := "invalid currency symbol"
		cm.lm.Log("error", msg, "currencies")
		return currency, errors.New(msg)
	}

	// Create a database connection
	db, err := cm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		cm.lm.Log("error", msg, "currencies")
		return currency, err
	}
	defer db.Close()

	// Search for a currency
	row := db.QueryRowContext(context.Background(), "select symbol, currency from currencies where symbol = ?;", symbol)

	err = row.Scan(&currency.Symbol, &currency.Currency)
	if err != nil {
		msg := err.Error()
		cm.lm.Log("debug", msg, "currencies")
		return currency, nil
	}

	return currency, nil
}

// List currencies
func (cm *CurrencyManager) List() ([]node_types.Currency, error) {
	// Create a database connection
	db, err := cm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		cm.lm.Log("error", msg, "currency")
		return nil, err
	}
	defer db.Close()

	// Load currencies
	rows, err := db.QueryContext(context.Background(), "select symbol, currency from currencies;")
	if err != nil {
		cm.lm.Log("error", err.Error(), "currency")
		return nil, err
	}
	defer rows.Close()

	var currencies []node_types.Currency
	for rows.Next() {
		var currency node_types.Currency
		if err := rows.Scan(&currency.Symbol, &currency.Currency); err == nil {
			currencies = append(currencies, currency)
		}
	}

	return currencies, rows.Err()
}

// Add a currency
func (cm *CurrencyManager) Add(currency string, symbol string) error {
	// Check if currency is already existing
	err, existing := cm.Exists(symbol)
	if err != nil {
		msg := err.Error()
		cm.lm.Log("error", msg, "currencies")
		return err
	}

	if existing {
		err = fmt.Errorf("currency %s (%s) is already existing", currency, symbol)
		cm.lm.Log("warn", err.Error(), "currencies")
		return err
	}

	// Create a database connection
	db, err := cm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		cm.lm.Log("error", msg, "currencies")
		return err
	}
	defer db.Close()

	// Add currency
	cm.lm.Log("debug", fmt.Sprintf("add currency %s (%s)", currency, symbol), "currencies")

	_, err = db.ExecContext(context.Background(), "insert into currencies (symbol, currency) values (?, ?);",
		symbol, currency)
	if err != nil {
		cm.lm.Log("error", err.Error(), "currencies")
		return err
	}

	return nil
}

// Remove currency
func (cm *CurrencyManager) Remove(symbol string) error {
	// Check if currency is already existing
	err, existing := cm.Exists(symbol)
	if err != nil {
		msg := err.Error()
		cm.lm.Log("error", msg, "currencies")
		return err
	}

	if !existing {
		err = fmt.Errorf("currency %s is not existing in the database. Nothing to remove", symbol)
		cm.lm.Log("warn", err.Error(), "currencies")
		return err
	}

	// Create a database connection
	db, err := cm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		cm.lm.Log("error", msg, "currencies")
		return err
	}
	defer db.Close()

	// Check if there are existing prices defined using this currency
	prices, err := cm.pm.GetPricesByCurrency(symbol)
	if err != nil {
		msg := err.Error()
		cm.lm.Log("error", msg, "currencies")
		return err
	}
	if len(prices) > 0 {
		err = fmt.Errorf("currency %s is used with %d pricings defined. Please remove pricings in this currency first", symbol, len(prices))
		cm.lm.Log("warn", err.Error(), "currencies")
		return err
	}

	// Remove currency
	cm.lm.Log("debug", fmt.Sprintf("removing currency %s", symbol), "currencies")

	_, err = db.ExecContext(context.Background(), "delete from currencies where symbol = ?;", symbol)
	if err != nil {
		msg := err.Error()
		cm.lm.Log("error", msg, "currencies")
		return err
	}

	return nil
}
