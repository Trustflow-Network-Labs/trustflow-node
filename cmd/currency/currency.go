package currency

import (
	"context"
	"errors"
	"fmt"

	"github.com/adgsm/trustflow-node/cmd/price"
	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/utils"
)

// Currency already added?
func CurrencyExists(symbol string) (error, bool) {
	if symbol == "" {
		msg := "invalid currency symbol"
		utils.Log("error", msg, "currencies")
		return errors.New(msg), false
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "currencies")
		return err, false
	}
	defer db.Close()

	// Check if currency already existing
	var id node_types.NullInt32
	row := db.QueryRowContext(context.Background(), "select id from currencies where symbol = ?;", symbol)

	err = row.Scan(&id)
	if err != nil {
		msg := err.Error()
		utils.Log("debug", msg, "currencies")
		return nil, false
	}

	return nil, true
}

// Get currency by symbol
func GetCurrencyBySymbol(symbol string) (node_types.Currency, error) {
	var currency node_types.Currency
	if symbol == "" {
		msg := "invalid currency symbol"
		utils.Log("error", msg, "currencies")
		return currency, errors.New(msg)
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "currencies")
		return currency, err
	}
	defer db.Close()

	// Search for a currency
	row := db.QueryRowContext(context.Background(), "select id, currency, symbol from currencies where symbol = ?;", symbol)

	err = row.Scan(&currency.Id, &currency.Currency, &currency.Symbol)
	if err != nil {
		msg := err.Error()
		utils.Log("debug", msg, "currencies")
		return currency, nil
	}

	return currency, nil
}

// Add a currency
func AddCurrency(currency string, symbol string) {
	err, existing := CurrencyExists(symbol)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "currencies")
		return
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "currencies")
		return
	}
	defer db.Close()

	// Check if currency is already existing
	if existing {
		msg := fmt.Sprintf("Currency %s (%s) is already existing", currency, symbol)
		utils.Log("warn", msg, "currencies")
		return
	}

	// Add currency
	utils.Log("debug", fmt.Sprintf("add currency %s (%s)", currency, symbol), "currencies")

	_, err = db.ExecContext(context.Background(), "insert into currencies (currency, symbol) values (?, ?);",
		currency, symbol)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "currencies")
		return
	}
}

// Remove currency
func RemoveCurrency(symbol string) {
	err, existing := CurrencyExists(symbol)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "currencies")
		return
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "currencies")
		return
	}
	defer db.Close()

	// Check if currency is already existing
	if !existing {
		msg := fmt.Sprintf("Currency %s is not existing in the database. Nothing to remove", symbol)
		utils.Log("warn", msg, "currencies")
		return
	}

	// Check if there are existing prices defined using this currency
	currency, err := GetCurrencyBySymbol(symbol)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "currencies")
		return
	}

	prices, err := price.GetPricesByCurrencyId(currency.Id.Int32)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "currencies")
		return
	}
	if len(prices) > 0 {
		msg := fmt.Sprintf("Currency %s is used with %d pricings defined. Please remove pricings in this currency first", symbol, len(prices))
		utils.Log("warn", msg, "currencies")
		return
	}

	// Remove currency
	utils.Log("debug", fmt.Sprintf("removing currency %s", symbol), "currencies")

	_, err = db.ExecContext(context.Background(), "delete from currencies where symbol = ?;", symbol)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "currencies")
		return
	}
}
