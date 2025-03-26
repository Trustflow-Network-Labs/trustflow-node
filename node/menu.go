package node

import (
	"encoding/json"
	"fmt"

	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/utils"
	"github.com/manifoldco/promptui"
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

func (mm *MenuManager) Run() {
	for {
		prompt := promptui.Select{
			Label: "Choose an option",
			Items: []string{"Find remote services", "Find local services", "Exit"},
		}

		_, result, err := prompt.Run()
		if err != nil {
			msg := fmt.Sprintf("Prompt failed: %s", err.Error())
			fmt.Println(msg)
			mm.lm.Log("error", msg, "p2p")
			return
		}

		switch result {
		case "Find remote services":
			frsPrompt := promptui.Prompt{
				Label:       "Service name",
				Default:     "New service",
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
				return
			}
			serviceManager := NewServiceManager(mm.p2pm)
			serviceManager.LookupRemoteService(snResult, "", "", "", "")
		case "Find local services":
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
		case "Exit":
			msg := "Exiting interactive mode..."
			fmt.Println(msg)
			mm.lm.Log("info", msg, "p2p")
			return
		}
	}
}
