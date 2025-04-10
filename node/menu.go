package node

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	blacklist_node "github.com/adgsm/trustflow-node/blacklist-node"
	"github.com/adgsm/trustflow-node/currency"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/price"
	"github.com/adgsm/trustflow-node/resource"
	"github.com/adgsm/trustflow-node/utils"
	"github.com/manifoldco/promptui"
	"github.com/olekukonko/tablewriter"
)

type MenuManager struct {
	lm   *utils.LogsManager
	p2pm *P2PManager
	sm   *ServiceManager
	vm   *utils.ValidatorManager
	tm   *utils.TextManager
	pm   *price.PriceManager
	rm   *resource.ResourceManager
	cm   *currency.CurrencyManager
}

func NewMenuManager(p2pm *P2PManager) *MenuManager {
	return &MenuManager{
		lm:   utils.NewLogsManager(),
		p2pm: p2pm,
		sm:   NewServiceManager(p2pm),
		vm:   utils.NewValidatorManager(),
		tm:   utils.NewTextManager(),
		pm:   price.NewPriceManager(p2pm.db),
		rm:   resource.NewResourceManager(p2pm.db),
		cm:   currency.NewCurrencyManager(p2pm.db),
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
			Items: []string{"Find services", "Request services", "Configure node", "Exit"},
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
		case "Request services":
			mm.requestServices()
		case "Configure node":
			mm.configureNode()
		case "Exit":
			msg := "Exiting interactive mode..."
			fmt.Println(msg)
			mm.lm.Log("info", msg, "menu")
			return
		}
	}
}

// Print find services sub-menu
func (mm *MenuManager) findServices() error {
	frsPrompt := promptui.Prompt{
		Label:       "Search phrases: (comma-separated)",
		Default:     "",
		Validate:    mm.vm.MinLen,
		AllowEdit:   true,
		HideEntered: false,
		IsConfirm:   false,
		IsVimMode:   false,
	}
	snResult, err := frsPrompt.Run()
	if err != nil {
		msg := fmt.Sprintf("Entering search phrases failed: %s", err.Error())
		fmt.Println(msg)
		mm.lm.Log("error", msg, "menu")
		return err
	}

	fmt.Println("Select service type:")
	// Get service type
	rPrompt := promptui.Select{
		Label: "Main \U000025B6 Find services \U000025B6 Service Type",
		Items: []string{"ANY", "DATA", "DOCKER EXECUTION ENVIRONMENT", "STANDALONE EXECUTABLE"},
	}
	_, rResult, err := rPrompt.Run()
	if err != nil {
		fmt.Printf("prompt failed: %s\n", err.Error())
		return err
	}
	if rResult == "ANY" {
		rResult = ""
	}

	return mm.sm.LookupRemoteService(snResult, rResult)
}

func (mm *MenuManager) printOfferedService(service node_types.ServiceOffer) {
	fmt.Printf("\n")
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Service Id", "Name", "Type"})
	tableP := tablewriter.NewWriter(os.Stdout)
	tableP.SetHeader([]string{"Resource", "Resource Unit", "Price", "Currency"})
	row := []string{fmt.Sprintf("%s-%d", service.NodeId, service.Id), mm.tm.Shorten(service.Name, 17, 0), service.Type}
	table.Append(row)
	table.Render() // Prints the table
	fmt.Printf("---\nDescription: %s\n", service.Description)
	fmt.Print("---\nPrice model:")
	if len(service.ServicePriceModel) > 0 {
		for i, priceComponent := range service.ServicePriceModel {
			fmt.Printf("\n%d. %s\n   %.02f %s / per %s",
				i+1, priceComponent.ResourceName, priceComponent.Price, priceComponent.CurrencySymbol, priceComponent.ResourceUnit)
		}
	} else {
		fmt.Println(" FREE")
	}
	fmt.Print("\n------------\nPress any key to continue...\n")
}

func (mm *MenuManager) printServiceResponse(serviceResponse node_types.ServiceResponse) {
	fmt.Printf("\n")
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Node Id", "Service Id", "Job Id", "Message", "Accpeted"})
	row := []string{serviceResponse.NodeId, fmt.Sprintf("%d", serviceResponse.ServiceId), fmt.Sprintf("%d", serviceResponse.JobId), serviceResponse.Message, fmt.Sprintf("%t", serviceResponse.Accepted)}
	table.Append(row)
	table.Render() // Prints the table
	fmt.Print("\n------------\nPress any key to continue...\n")
}

// Print request services sub-menu
func (mm *MenuManager) requestServices() {
	for {
		prompt := promptui.Select{
			Label: "Main \U000025B6 Request services",
			Items: []string{"Request service and start new workflow", "Request service for existing workflow", "Back"},
		}

		_, result, err := prompt.Run()
		if err != nil {
			msg := fmt.Sprintf("Prompt failed: %s", err.Error())
			fmt.Println(msg)
			mm.lm.Log("error", msg, "menu")
			continue
		}

		switch result {
		case "Request service and start new workflow":
			mm.requestServiceNewWorkflow()
		case "Request service for existing workflow":
			mm.requestServiceExistingWorkflow()
		case "Back":
			return
		}
	}
}

// Print request service with new workflow sub-menu
func (mm *MenuManager) requestServiceNewWorkflow() error {
	// Start new workflow
	// TODO, create new workflow
	sidPrompt := promptui.Prompt{
		Label:       "Service ID",
		Default:     "",
		Validate:    mm.vm.NotEmpty,
		AllowEdit:   true,
		HideEntered: false,
		IsConfirm:   false,
		IsVimMode:   false,
	}
	sidResult, err := sidPrompt.Run()
	if err != nil {
		msg := fmt.Sprintf("Entering service ID failed: %s", err.Error())
		fmt.Println(msg)
		mm.lm.Log("error", msg, "menu")
		return err
	}

	fmt.Println("Select constraint type:")
	// Get constraint type
	cPrompt := promptui.Select{
		Label: "Main \U000025B6 Request service \U000025B6 Constraint Type",
		Items: []string{"NONE", "INPUTS READY", "DATETIME", "JOBS EXECUTED", "MANUAL START"},
	}
	_, cResult, err := cPrompt.Run()
	if err != nil {
		fmt.Printf("prompt failed: %s\n", err.Error())
		return err
	}

	serviceIdPair := strings.Split(sidResult, "-")
	if len(serviceIdPair) < 2 {
		msg := fmt.Sprintf("Invalid Service ID: %s", sidResult)
		fmt.Println(msg)
		mm.lm.Log("error", msg, "menu")
		return err
	}
	peerId := serviceIdPair[0]
	sid := serviceIdPair[1]
	peer, err := mm.p2pm.GeneratePeerFromId(peerId)
	if err != nil {
		fmt.Printf("Generating peer address info from %s failed: %s\n", peerId, err.Error())
		return err
	}
	serviceId, err := strconv.ParseInt(sid, 10, 32)
	if err != nil {
		fmt.Printf("Service Id %s seems to be invalid Id: %s\n", sid, err.Error())
		return err
	}

	// TODO, collect all inputs
	err = mm.p2pm.RequestService(peer, serviceId, []string{}, []string{mm.p2pm.h.ID().String()}, cResult, "")
	if err != nil {
		fmt.Printf("Requesting service failed: %s\n", err.Error())
		return err
	}

	return nil
}

// Print request service with new workflow sub-menu
func (mm *MenuManager) requestServiceExistingWorkflow() error {
	return nil
}

// Print configure node sub-menu
func (mm *MenuManager) configureNode() {
	for {
		prompt := promptui.Select{
			Label: "Main \U000025B6 Configure node",
			Items: []string{"Blacklist", "Currencies", "Resources", "Services", "Settings", "Back"},
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
		case "Services":
			mm.services()
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
			blacklistManager, err := blacklist_node.NewBlacklistNodeManager(mm.p2pm.db)
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
			// Get node ID
			nidPrompt := promptui.Prompt{
				Label:       "Node ID",
				Default:     "",
				Validate:    mm.vm.IsPeer,
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

			blacklistManager, err := blacklist_node.NewBlacklistNodeManager(mm.p2pm.db)
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
			// Get node ID
			nidPrompt := promptui.Prompt{
				Label:       "Node ID",
				Default:     "",
				Validate:    mm.vm.NotEmpty,
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

			blacklistManager, err := blacklist_node.NewBlacklistNodeManager(mm.p2pm.db)
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
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"Node ID", "Reason", "Timestamp"})
	for _, node := range nodes {
		row := []string{mm.tm.Shorten(node.NodeId.String(), 6, 6), node.Reason, node.Timestamp.Local().Format("2006-01-02 15:04:05 MST")}
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
			err = mm.printCurrencies(mm.cm)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Add currency":
			// Get currency symbol
			csPrompt := promptui.Prompt{
				Label:       "Currency Symbol",
				Default:     "",
				Validate:    mm.vm.NotEmpty,
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
				Validate:    mm.vm.NotEmpty,
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

			// Add currency
			err = mm.cm.Add(cnResult, csResult)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}
			fmt.Printf("\U00002705 Currency %s (%s) is added\n", cnResult, csResult)

			err = mm.printCurrencies(mm.cm)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Remove currency":
			// Get currency symbol
			csPrompt := promptui.Prompt{
				Label:       "Currency Symbol",
				Default:     "",
				Validate:    mm.vm.NotEmpty,
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

			// Remove currency
			err = mm.cm.Remove(csResult)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}
			fmt.Printf("\U00002705 Currency %s is removed\n", csResult)

			err = mm.printCurrencies(mm.cm)
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
			err = mm.printResources(mm.rm)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Set resource active":
			// Get resource Id
			rnPrompt := promptui.Prompt{
				Label:       "Resource Id",
				Default:     "",
				Validate:    mm.vm.IsInt64,
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			rnResult, err := rnPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering resource Id failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			rn, err := mm.tm.ToInt64(rnResult)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}

			// Set resource active
			err = mm.rm.SetActive(rn)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}
			fmt.Printf("\U00002705 Resource id %d is set to active\n", rn)

			err = mm.printResources(mm.rm)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Set resource inactive":
			// Get resource Id
			rnPrompt := promptui.Prompt{
				Label:       "Resource Id",
				Default:     "",
				Validate:    mm.vm.IsInt64,
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			rnResult, err := rnPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering resource id failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			rn, err := mm.tm.ToInt64(rnResult)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}

			// Set resource active
			err = mm.rm.SetInactive(rn)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}
			fmt.Printf("\U00002705 Resource id %d is set to inactive\n", rn)

			err = mm.printResources(mm.rm)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Add resource":
			// Get resource group name
			rgPrompt := promptui.Prompt{
				Label:       "Resource group",
				Default:     "",
				Validate:    mm.vm.NotEmpty,
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			rgResult, err := rgPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering resource group name failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			// Get resource name
			rnPrompt := promptui.Prompt{
				Label:       "Resource name",
				Default:     "",
				Validate:    mm.vm.NotEmpty,
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

			// Get resource unit name
			ruPrompt := promptui.Prompt{
				Label:       "Resource unit",
				Default:     "",
				Validate:    mm.vm.NotEmpty,
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			ruResult, err := ruPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering resource unit name failed: %s", err.Error())
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
				Validate:    mm.vm.IsBool,
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

			active, err := mm.tm.ToBool(raResult)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}

			// Add resource
			err = mm.rm.Add(rgResult, rnResult, ruResult, rdResult, active)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}
			fmt.Printf("\U00002705 Resource %s is added\n", rnResult)

			err = mm.printResources(mm.rm)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Remove resource":
			// Get resource id
			rnPrompt := promptui.Prompt{
				Label:       "Resource Id",
				Default:     "",
				Validate:    mm.vm.NotEmpty,
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			rnResult, err := rnPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering resource Id failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			rn, err := mm.tm.ToInt64(rnResult)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}

			// Remove resource
			err = mm.rm.Remove(rn)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}
			fmt.Printf("\U00002705 Resource id %d is removed\n", rn)

			err = mm.printResources(mm.rm)
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
	table.SetHeader([]string{"Id", "Resource Group", "Resource", "Resource Unit", "Active"})
	for _, resource := range resources {
		row := []string{strconv.FormatInt(resource.Id, 10), resource.ResourceGroup, resource.Resource, resource.ResourceUnit, fmt.Sprintf("%t", resource.Active)}
		table.Append(row)
	}
	table.Render() // Prints the table

	return nil
}

// Print services sub-menu
func (mm *MenuManager) services() {
	for {
		prompt := promptui.Select{
			Label: "Main \U000025B6 Configure node \U000025B6 Services",
			Items: []string{"List services", "Show service details", "Add service", "Set service active", "Set service inactive", "Remove service", "Back"},
		}

		_, result, err := prompt.Run()
		if err != nil {
			msg := fmt.Sprintf("Prompt failed: %s", err.Error())
			fmt.Println(msg)
			mm.lm.Log("error", msg, "menu")
			continue
		}

		switch result {
		case "List services":
			err = mm.printServices(mm.sm)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Show service details":
		case "Add service":
			// Get service name
			snPrompt := promptui.Prompt{
				Label:       "Service name",
				Default:     "",
				Validate:    mm.vm.NotEmpty,
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			snResult, err := snPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering service name failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			// Get service description
			sdPrompt := promptui.Prompt{
				Label:       "Service description",
				Default:     "",
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			sdResult, err := sdPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering service description failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			// Get service state
			raPrompt := promptui.Prompt{
				Label:       "Is active?",
				Default:     "",
				Validate:    mm.vm.IsBool,
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			raResult, err := raPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering service active flag failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			active, err := mm.tm.ToBool(raResult)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}

			stPrompt := promptui.Select{
				Label: "Main \U000025B6 Configure node \U000025B6 Services \U000025B6 Add Service \U000025B6 Service Type",
				Items: []string{"DATA", "DOCKER EXECUTION ENVIRONMENT", "STANDALONE EXECUTABLE"},
			}

			_, stResult, err := stPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("Prompt failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			// Add service
			id, err := mm.sm.Add(snResult, sdResult, stResult, active)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}

			switch stResult {
			case "DATA":
				// Get file/folder/data path
				dpPrompt := promptui.Prompt{
					Label:       "Path",
					Default:     "",
					Validate:    mm.vm.NotEmpty,
					AllowEdit:   true,
					HideEntered: false,
					IsConfirm:   false,
					IsVimMode:   false,
				}
				dpResult, err := dpPrompt.Run()
				if err != nil {
					msg := fmt.Sprintf("\U00002757 Entering service file/folder/data path failed: %s", err.Error())
					fmt.Println(msg)
					mm.lm.Log("error", msg, "menu")
					mm.sm.Remove(id)
					continue
				}

				// Add data service
				_, err = mm.sm.AddData(id, dpResult)
				if err != nil {
					fmt.Printf("\U00002757 %s\n", err.Error())
					mm.lm.Log("error", err.Error(), "menu")
					mm.sm.Remove(id)
					continue
				}
			case "DOCKER EXECUTION ENVIRONMENT":
			case "STANDALONE EXECUTABLE":
			}

			// Print free or paid service prompt
			sfcPrompt := promptui.Prompt{
				Label:     "Is this service free of charge",
				IsConfirm: true,
			}
			sfcResult, err := sfcPrompt.Run()
			if err != nil && strings.ToLower(sfcResult) != "n" && strings.ToLower(sfcResult) != "y" {
				fmt.Printf("Prompt failed %v\n", err)
				mm.lm.Log("error", err.Error(), "menu")
				mm.sm.Remove(id)
				continue
			}
			if strings.ToLower(sfcResult) == "n" {
				// Add service price
				err = mm.addServicePrice(id)
				if err != nil {
					fmt.Println(err.Error())
					mm.lm.Log("error", err.Error(), "menu")
					mm.sm.Remove(id)
					continue
				}
			}

			// Service is added
			fmt.Printf("\U00002705 Service %s is added\n", snResult)

			// Print service table
			err = mm.printServices(mm.sm)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Set service active":
			// Get service Id
			rnPrompt := promptui.Prompt{
				Label:       "Service Id",
				Default:     "",
				Validate:    mm.vm.IsInt64,
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			rnResult, err := rnPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering service Id failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			// Set service active
			id, err := mm.tm.ToInt64(rnResult)
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Service Id is not valid int64: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}
			err = mm.sm.SetActive(id)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}
			fmt.Printf("\U00002705 Service %s is set active\n", rnResult)

			err = mm.printServices(mm.sm)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Set service inactive":
			// Get service Id
			rnPrompt := promptui.Prompt{
				Label:       "Service Id",
				Default:     "",
				Validate:    mm.vm.IsInt64,
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			rnResult, err := rnPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering service Id failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			// Set service active
			id, err := mm.tm.ToInt64(rnResult)
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Service Id is not valid int64: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}
			err = mm.sm.SetInactive(id)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
				continue
			}
			fmt.Printf("\U00002705 Service %s is set inactive\n", rnResult)

			err = mm.printServices(mm.sm)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Remove service":
			// Get service Id
			rnPrompt := promptui.Prompt{
				Label:       "Service Id",
				Default:     "",
				Validate:    mm.vm.IsInt64,
				AllowEdit:   true,
				HideEntered: false,
				IsConfirm:   false,
				IsVimMode:   false,
			}
			rnResult, err := rnPrompt.Run()
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Entering service Id failed: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}

			// Remove service
			id, err := mm.tm.ToInt64(rnResult)
			if err != nil {
				msg := fmt.Sprintf("\U00002757 Service Id is not valid int64: %s", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}
			err = mm.sm.Remove(id)
			if err != nil {
				msg := fmt.Sprintf("\U00002757 %s\n", err.Error())
				fmt.Println(msg)
				mm.lm.Log("error", msg, "menu")
				continue
			}
			fmt.Printf("\U00002705 Service %s is removed\n", rnResult)

			err = mm.printServices(mm.sm)
			if err != nil {
				fmt.Printf("\U00002757 %s\n", err.Error())
				mm.lm.Log("error", err.Error(), "menu")
			}
		case "Back":
			return
		}
	}
}

func (mm *MenuManager) printServices(sm *ServiceManager, params ...uint32) error {
	var offset uint32 = 0
	var limit uint32 = 10

	// Read configs
	configManager := utils.NewConfigManager("")
	config, err := configManager.ReadConfigs()
	if err != nil {
		message := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
		sm.lm.Log("error", message, "menu")
		panic(err)
	}

	l := config["search_results"]
	l64, err := strconv.ParseUint(l, 10, 32)
	if err != nil {
		limit = 10
	} else {
		limit = uint32(l64)
	}

	if len(params) == 1 {
		offset = params[0]
	} else if len(params) >= 2 {
		offset = params[0]
		limit = params[1]
	}

	services, err := sm.List(offset, limit)
	if err != nil {
		return err
	}

	// Draw table output
	textManager := utils.NewTextManager()
	table := tablewriter.NewWriter(os.Stdout)
	table.SetHeader([]string{"ID", "Name", "Type", "Active"})
	for _, service := range services {
		row := []string{fmt.Sprintf("%d", service.Id), textManager.Shorten(service.Name, 17, 0), service.Type, fmt.Sprintf("%t", service.Active)}
		table.Append(row)
	}
	table.Render() // Prints the table

	if len(services) >= int(limit) {
		// Print "load more" prompt
		lmPrompt := promptui.Prompt{
			Label:     "Load more?",
			IsConfirm: true,
		}
		lmResult, err := lmPrompt.Run()
		if err != nil && strings.ToLower(lmResult) != "n" && strings.ToLower(lmResult) != "y" {
			fmt.Printf("Prompt failed %v\n", err)
			return err
		}
		if strings.ToLower(lmResult) == "y" {
			mm.printServices(sm, offset+limit, limit)
		}
	}

	return nil
}

func (mm *MenuManager) addServicePrice(id int64) error {
	fmt.Println("Please add service resource price")
	// Get resource
	var rItems []string
	rItemsMap := make(map[string]int64)
	resources, err := mm.rm.List()
	if err != nil {
		return err
	}
	for _, resource := range resources {
		key := fmt.Sprintf("%s [%s]", resource.Resource, resource.ResourceUnit)
		rItems = append(rItems, key)
		rItemsMap[key] = resource.Id
	}
	rPrompt := promptui.Select{
		Label: "Main \U000025B6 Configure node \U000025B6 Services \U000025B6 Add Service \U000025B6 Resource",
		Items: rItems,
	}
	_, rResult, err := rPrompt.Run()
	if err != nil {
		err = fmt.Errorf("prompt failed: %s", err.Error())
		return err
	}
	r := rItemsMap[rResult]

	// Get resource price
	rpPrompt := promptui.Prompt{
		Label:       "Service resource price",
		Default:     "1.00",
		Validate:    mm.vm.IsFloat64,
		AllowEdit:   true,
		HideEntered: false,
		IsConfirm:   false,
		IsVimMode:   false,
	}
	rpResult, err := rpPrompt.Run()
	if err != nil {
		err = fmt.Errorf("\U00002757 Entering service resource price failed: %s", err.Error())
		return err
	}
	rp, err := mm.tm.ToFloat64(rpResult)
	if err != nil {
		return err
	}

	// Get currency
	var cItems []string
	currencies, err := mm.cm.List()
	if err != nil {
		return err
	}
	for _, currency := range currencies {
		cItems = append(cItems, currency.Symbol)
	}
	rscPrompt := promptui.Select{
		Label: "Main \U000025B6 Configure node \U000025B6 Services \U000025B6 Add Service \U000025B6 Currency",
		Items: cItems,
	}
	_, rscResult, err := rscPrompt.Run()
	if err != nil {
		err = fmt.Errorf("prompt failed: %s", err.Error())
		return err
	}

	mm.pm.Add(id, r, rp, rscResult)

	// Add more resource prices prompt
	srmPrompt := promptui.Prompt{
		Label:     "Add more service resource prices?",
		IsConfirm: true,
	}
	srmResult, err := srmPrompt.Run()
	if err != nil && strings.ToLower(srmResult) != "n" && strings.ToLower(srmResult) != "y" {
		mm.lm.Log("error", err.Error(), "menu")
		return err
	}
	if strings.ToLower(srmResult) == "y" {
		// Add another service price
		return mm.addServicePrice(id)
	}

	return nil
}
