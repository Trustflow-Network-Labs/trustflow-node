package node

import (
	"encoding/json"
	"fmt"
	"os"

	blacklist_node "github.com/adgsm/trustflow-node/blacklist-node"
	"github.com/adgsm/trustflow-node/currency"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/resource"
	"github.com/adgsm/trustflow-node/utils"
	"github.com/manifoldco/promptui"
	"github.com/olekukonko/tablewriter"
)

type MenuManager struct {
	lm   *utils.LogsManager
	p2pm *P2PManager
}

func NewMenuManager(p2pm *P2PManager) *MenuManager {
	return &MenuManager{
		lm:   utils.NewLogsManager(),
		p2pm: p2pm,
	}
}

// Print menu
func (mm *MenuManager) Run() {
	mm.main()
}

// Print main menu
func (mm *MenuManager) main() {
	for {
		prompt := promptui.Select{
			Label: "Main",
			Items: []string{"Find services", "Configure node", "Workflows & Jobs", "Exit"},
		}

		_, result, err := prompt.Run()
		if err != nil {
			msg := fmt.Sprintf("Prompt failed: %s", err.Error())
			fmt.Println(msg)
			mm.lm.Log("error", msg, "menu")
			return
		}

		switch result {
		case "Find services":
			mm.findServices()
		case "Configure node":
			mm.configureNode()
		case "Workflows & Jobs":
		case "Exit":
			msg := "Exiting interactive mode..."
			fmt.Println(msg)
			mm.lm.Log("info", msg, "menu")
			return
		}
	}
}

// Print find services sub-menu
func (mm *MenuManager) findServices() {
	for {
		prompt := promptui.Select{
			Label: "Main \U000025B6 Find services",
			Items: []string{"Find remote services", "List local services", "Back"},
		}

		_, result, err := prompt.Run()
		if err != nil {
			msg := fmt.Sprintf("Prompt failed: %s", err.Error())
			fmt.Println(msg)
			mm.lm.Log("error", msg, "menu")
			continue
		}

		switch result {
		case "Find remote services":
			frsPrompt := promptui.Prompt{
				Label:       "Service name",
				Default:     "",
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			snResult, err := frsPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("Entering service name failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}
			serviceManager := NewServiceManager(mm.p2pm)
			serviceManager.LookupRemoteService(snResult, "", "", "", "")
		case "List local services":
			var data []byte
			var catalogueLookup node_types.ServiceLookup = node_types.ServiceLookup{
				Name:        "",
				Description: "",
				NodeId:      "",
				Type:        "",
				Repo:        "",
			}

			data, err = json.Marshal(catalogueLookup)
			if err != nil {
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}

			serviceCatalogue, err := mm.p2pm.ServiceLookup(data, true)
			if err != nil {
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}

			fmt.Printf("%v\n", serviceCatalogue)
		case "Back":
			return
		}
	}
}

// Print configure node sub-menu
func (mm *MenuManager) configureNode() {
	for {
		prompt := promptui.Select{
			Label: "Main \U000025B6 Configure node",
			Items: []string{"Blacklist", "Currencies", "Resources", "Prices", "Services", "Settings", "Back"},
		}

		_, result, err := prompt.Run()
		if err != nil {
			msg := fmt.Sprintf("Prompt failed: %s", err.Error())
			fmt.Println(msg)
			mm.lm.Log("error", msg, "menu")
			continue
		}

		switch result {
		case "Blacklist":
			mm.blacklist()
		case "Currencies":
			mm.currencies()
		case "Resources":
			mm.resources()
		case "Prices":
		case "Services":
		case "Settings":
		case "Back":
			return
		}
	}
}

// Print blacklist sub-menu
func (mm *MenuManager) blacklist() {
	for {
		prompt := promptui.Select{
			Label: "Main \U000025B6 Configure node \U000025B6 Blacklist",
			Items: []string{"List nodes", "Add node", "Remove node", "Back"},
		}

		_, result, err := prompt.Run()
		if err != nil {
			msg := fmt.Sprintf("Prompt failed: %s", err.Error())
			fmt.Println(msg)
			mm.lm.Log("error", msg, "menu")
			continue
		}

		switch result {
		case "List nodes":
			blacklistManager, err := blacklist_node.NewBlacklistNodeManager()
			if err != nil {
				fmt.Println(err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}
			err = mm.printBlacklist(blacklistManager)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Add node":
			validatorManager := utils.NewValidatorManager()
			// Get node ID
			nidPrompt := promptui.Prompt{
				Label:       "Node ID",
				Default:     "",
				Validate:    validatorManager.IsPeer,
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			nidResult, err := nidPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering Node ID failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			// Get reason for blacklisting node
			rsPrompt := promptui.Prompt{
				Label:       "Reason (optional)",
				Default:     "",
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			rsResult, err := rsPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering reason for blacklisting node failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			blacklistManager, err := blacklist_node.NewBlacklistNodeManager()
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}

			// Add node to blacklist
			err = blacklistManager.Add(nidResult, rsResult)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}
			fmt.Printf("\U00002705 Node %s is added to blacklist\n", nidResult)

			err = mm.printBlacklist(blacklistManager)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Remove node":
			validatorManager := utils.NewValidatorManager()
			// Get node ID
			nidPrompt := promptui.Prompt{
				Label:       "Node ID",
				Default:     "",
				Validate:    validatorManager.NotEmpty,
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			nidResult, err := nidPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering Node ID failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			blacklistManager, err := blacklist_node.NewBlacklistNodeManager()
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}

			// Remove node from blacklist
			err = blacklistManager.Remove(nidResult)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}
			fmt.Printf("\U00002705 Node %s is removed from blacklist\n", nidResult)

			err = mm.printBlacklist(blacklistManager)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Back":
			return
		}
	}
}

func (mm *MenuManager) printBlacklist(blnm *blacklist_node.BlacklistNodeManager) error {
	nodes, err := blnm.List()
	if err != nil {
		return err
	}

	// Draw table output
	textManager := utils.NewTextManager()
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Node ID", "Reason", "Timestamp"})
	for _, node := range nodes {
		row := []string{textManager.Shorten(node.NodeId.String(), 6, 6), node.Reason, node.Timestamp.Local().Format("2006-01-02 15:04:05 MST")}
		table.Append(row)
	}
	table.Render() // Prints the table

	return nil
}

// Print currencies sub-menu
func (mm *MenuManager) currencies() {
	for {
		prompt := promptui.Select{
			Label: "Main \U000025B6 Configure node \U000025B6 Currencies",
			Items: []string{"List currencies", "Add currency", "Remove currency", "Back"},
		}

		_, result, err := prompt.Run()
		if err != nil {
			msg := fmt.Sprintf("Prompt failed: %s", err.Error())
			fmt.Println(msg)
			mm.lm.Log("error", msg, "menu")
			continue
		}

		switch result {
		case "List currencies":
			currenciesManager := currency.NewCurrencyManager()
			err = mm.printCurrencies(currenciesManager)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Add currency":
			validatorManager := utils.NewValidatorManager()
			// Get currency symbol
			csPrompt := promptui.Prompt{
				Label:       "Currency Symbol",
				Default:     "",
				Validate:    validatorManager.NotEmpty,
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			csResult, err := csPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering currency symbol failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			// Get currency name
			cnPrompt := promptui.Prompt{
				Label:       "Currency name",
				Default:     "",
				Validate:    validatorManager.NotEmpty,
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			cnResult, err := cnPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering currency name failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			currenciesManager := currency.NewCurrencyManager()

			// Add currency
			err = currenciesManager.Add(cnResult, csResult)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}
			fmt.Printf("\U00002705 Currency %s (%s) is added\n", cnResult, csResult)

			err = mm.printCurrencies(currenciesManager)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Remove currency":
			validatorManager := utils.NewValidatorManager()
			// Get currency symbol
			csPrompt := promptui.Prompt{
				Label:       "Currency Symbol",
				Default:     "",
				Validate:    validatorManager.NotEmpty,
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			csResult, err := csPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering currency symbol failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			currenciesManager := currency.NewCurrencyManager()

			// Remove currency
			err = currenciesManager.Remove(csResult)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}
			fmt.Printf("\U00002705 Currency %s is removed\n", csResult)

			err = mm.printCurrencies(currenciesManager)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Back":
			return
		}
	}
}

func (mm *MenuManager) printCurrencies(cm *currency.CurrencyManager) error {
	currencies, err := cm.List()
	if err != nil {
		return err
	}

	// Draw table output
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Currency", "Symbol"})
	for _, currency := range currencies {
		row := []string{currency.Currency, currency.Symbol}
		table.Append(row)
	}
	table.Render() // Prints the table

	return nil
}

// Print resources sub-menu
func (mm *MenuManager) resources() {
	for {
		prompt := promptui.Select{
			Label: "Main \U000025B6 Configure node \U000025B6 Resources",
			Items: []string{"List resources", "Set resource active", "Set resource inactive", "Add resource", "Remove resource", "Back"},
		}

		_, result, err := prompt.Run()
		if err != nil {
			msg := fmt.Sprintf("Prompt failed: %s", err.Error())
			fmt.Println(msg)
			mm.lm.Log("error", msg, "menu")
			continue
		}

		switch result {
		case "List resources":
			resourcesManager := resource.NewResourceManager()
			err = mm.printResources(resourcesManager)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Set resource active":
			validatorManager := utils.NewValidatorManager()
			// Get resource name
			rnPrompt := promptui.Prompt{
				Label:       "Resource name",
				Default:     "",
				Validate:    validatorManager.NotEmpty,
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			rnResult, err := rnPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering resource name failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			// Set resource active
			resourcesManager := resource.NewResourceManager()
			err = resourcesManager.SetActive(rnResult)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}
			fmt.Printf("\U00002705 Resource %s is setr to active\n", rnResult)

			err = mm.printResources(resourcesManager)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Set resource inactive":
			validatorManager := utils.NewValidatorManager()
			// Get resource name
			rnPrompt := promptui.Prompt{
				Label:       "Resource name",
				Default:     "",
				Validate:    validatorManager.NotEmpty,
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			rnResult, err := rnPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering resource name failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			// Set resource active
			resourcesManager := resource.NewResourceManager()
			err = resourcesManager.SetInactive(rnResult)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}
			fmt.Printf("\U00002705 Resource %s is setr to inactive\n", rnResult)

			err = mm.printResources(resourcesManager)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Add resource":
			validatorManager := utils.NewValidatorManager()
			// Get resource name
			rnPrompt := promptui.Prompt{
				Label:       "Resource name",
				Default:     "",
				Validate:    validatorManager.NotEmpty,
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			rnResult, err := rnPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering resource name failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			// Get resource description
			rdPrompt := promptui.Prompt{
				Label:       "Resource description",
				Default:     "",
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			rdResult, err := rdPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering resource description failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			// Get resource state
			raPrompt := promptui.Prompt{
				Label:       "Is active?",
				Default:     "",
				Validate:    validatorManager.IsBool,
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			raResult, err := raPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering resource active flag failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			textManager := utils.NewTextManager()
			active, err := textManager.ToBool(raResult)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}

			// Add resource
			resourcesManager := resource.NewResourceManager()
			err = resourcesManager.Add(rnResult, rdResult, active)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}
			fmt.Printf("\U00002705 Resource %s is added\n", rnResult)

			err = mm.printResources(resourcesManager)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Remove resource":
			validatorManager := utils.NewValidatorManager()
			// Get resource name
			rnPrompt := promptui.Prompt{
				Label:       "Resource name",
				Default:     "",
				Validate:    validatorManager.NotEmpty,
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			rnResult, err := rnPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering resource name failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			resourcesManager := resource.NewResourceManager()

			// Remove resource
			err = resourcesManager.Remove(rnResult)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}
			fmt.Printf("\U00002705 Resource %s is removed\n", rnResult)

			err = mm.printResources(resourcesManager)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Back":
			return
		}
	}
}

func (mm *MenuManager) printResources(rm *resource.ResourceManager) error {
	resources, err := rm.List()
	if err != nil {
		return err
	}

	// Draw table output
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Resource", "Description", "Active"})
	for _, resource := range resources {
		row := []string{resource.Resource, resource.Description.String, fmt.Sprintf("%t", resource.Active)}
		table.Append(row)
	}
	table.Render() // Prints the table

	return nil
}
