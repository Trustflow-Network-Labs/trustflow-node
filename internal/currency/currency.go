package currency

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/adgsm/trustflow-node-gui-client/internal/node_types"
	"github.com/adgsm/trustflow-node-gui-client/internal/price"
	"github.com/adgsm/trustflow-node-gui-client/internal/utils"
)

type CurrencyManager struct {
	db *sql.DB
	lm *utils.LogsManager
	pm *price.PriceManager
}

func NewCurrencyManager(db *sql.DB) *CurrencyManager {
	return &CurrencyManager{
		db: db,
		lm: utils.NewLogsManager(),
		pm: price.NewPriceManager(db),
	}
}

func (cm *CurrencyManager) IsCurrency(s string) error {
	err, b := cm.Exists(s)
	if err != nil {
		return err
	}
	if !b {
		err = fmt.Errorf("%s is not an existing currency", s)
		return err
	}
	return nil
}

// Currency already added?
func (cm *CurrencyManager) Exists(symbol string) (error, bool) {
	if symbol == "" {
		msg := "invalid currency symbol"
		cm.lm.Log("error", msg, "currencies")
		return errors.New(msg), false
	}

	// Check if currency already existing
	var currency node_types.NullString
	row := cm.db.QueryRowContext(context.Background(), "select currency from currencies where symbol = ?;", symbol)

	err := row.Scan(&currency)
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

	// Search for a currency
	row := cm.db.QueryRowContext(context.Background(), "select symbol, currency from currencies where symbol = ?;", symbol)

	err := row.Scan(&currency.Symbol, &currency.Currency)
	if err != nil {
		cm.lm.Log("debug", err.Error(), "currencies")
		return currency, err
	}

	return currency, nil
}

// List currencies
func (cm *CurrencyManager) List() ([]node_types.Currency, error) {
	// Load currencies
	rows, err := cm.db.QueryContext(context.Background(), "select symbol, currency from currencies;")
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

	// Add currency
	cm.lm.Log("debug", fmt.Sprintf("add currency %s (%s)", currency, symbol), "currencies")

	_, err = cm.db.ExecContext(context.Background(), "insert into currencies (symbol, currency) values (?, ?);",
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

	_, err = cm.db.ExecContext(context.Background(), "delete from currencies where symbol = ?;", symbol)
	if err != nil {
		msg := err.Error()
		cm.lm.Log("error", msg, "currencies")
		return err
	}

	return nil
}
