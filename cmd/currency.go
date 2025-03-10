package cmd

import (
	"github.com/adgsm/trustflow-node/cmd/currency"
	"github.com/spf13/cobra"
)

var curr string
var symbol string
var addCurrencyCmd = &cobra.Command{
	Use:     "add-currency",
	Aliases: []string{"currency-add"},
	Short:   "Add a currency",
	Long:    "Adding new currency will allow setting data/services pricing in that currency",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		currencyManager := currency.NewCurrencyManager()
		currencyManager.AddCurrency(curr, symbol)
	},
}

var removeCurrencyCmd = &cobra.Command{
	Use:     "remove-currency",
	Aliases: []string{"currency-remove"},
	Short:   "Remove a currency",
	Long:    "Removing a currency will prevent setting data/services pricing in that currency. Currency can not be removed if there is an price set in that currency",
	Args:    cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		currencyManager := currency.NewCurrencyManager()
		currencyManager.RemoveCurrency(symbol)
	},
}

func init() {
	addCurrencyCmd.Flags().StringVarP(&curr, "currency", "c", "", "Currency name to be added")
	addCurrencyCmd.MarkFlagRequired("currency")
	addCurrencyCmd.Flags().StringVarP(&symbol, "symbol", "s", "", "Currency symbol to be added")
	addCurrencyCmd.MarkFlagRequired("symbol")
	rootCmd.AddCommand(addCurrencyCmd)
	removeCurrencyCmd.Flags().StringVarP(&symbol, "symbol", "s", "", "Currency symbol to be removed")
	removeCurrencyCmd.MarkFlagRequired("symbol")
	rootCmd.AddCommand(removeCurrencyCmd)
}
