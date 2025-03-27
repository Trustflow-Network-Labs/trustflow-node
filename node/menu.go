package node

import (
	"encoding/json"
	"fmt"
	"os"

	blacklist_node "github.com/adgsm/trustflow-node/blacklist-node"
	"github.com/adgsm/trustflow-node/node_types"
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
			mm.lm.Log("error", msg, "p2p")
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
			mm.lm.Log("info", msg, "p2p")
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
			mm.lm.Log("error", msg, "p2p")
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
				mm.lm.Log("error", msg, "p2p")
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
				mm.lm.Log("error", err.Error(), "p2p")
				continue
			}

			serviceCatalogue, err := mm.p2pm.ServiceLookup(data, true)
			if err != nil {
				mm.lm.Log("error", err.Error(), "p2p")
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
			mm.lm.Log("error", msg, "p2p")
			continue
		}

		switch result {
		case "Blacklist":
			mm.blacklist()
		case "Currencies":
		case "Resources":
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
			mm.lm.Log("error", msg, "p2p")
			continue
		}

		switch result {
		case "List nodes":
			blacklistManager, err := blacklist_node.NewBlacklistNodeManager()
			if err != nil {
				fmt.Println(err.Error())
				mm.lm.Log("error", err.Error(), "p2p")
				continue
			}
			nodes, err := blacklistManager.List()
			if err != nil {
				fmt.Println(err.Error())
				mm.lm.Log("error", err.Error(), "p2p")
				continue
			}

			// Draw table output
			textManager := utils.NewTextManager()
			table := tablewriter.NewWriter(os.Stdout)
			table.SetHeader([]string{"Node ID", "Reason", "Timestamp"})
			for _, node := range nodes {
				row := []string{textManager.ShortenString(node.NodeId.String(), 6, 6), node.Reason, node.Timestamp.Local().Format("2006-01-02 15:04:05 MST")}
				table.Append(row)
			}
			table.Render() // Prints the table
		case "Add node":
		case "Remove node":
		case "Back":
			return
		}
	}
}
