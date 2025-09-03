package node

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"

	blacklist_node "github.com/adgsm/trustflow-node/internal/blacklist-node"
	"github.com/adgsm/trustflow-node/internal/currency"
	"github.com/adgsm/trustflow-node/internal/node_types"
	"github.com/adgsm/trustflow-node/internal/price"
	"github.com/adgsm/trustflow-node/internal/repo"
	"github.com/adgsm/trustflow-node/internal/resource"
	"github.com/adgsm/trustflow-node/internal/ui"
	"github.com/adgsm/trustflow-node/internal/utils"
	"github.com/adgsm/trustflow-node/internal/workflow"
	"github.com/fatih/color"
	"github.com/google/shlex"
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
	wm   *workflow.WorkflowManager
	jm   *JobManager
}

func NewMenuManager(p2pm *P2PManager) *MenuManager {
	return &MenuManager{
		lm:   p2pm.Lm,
		p2pm: p2pm,
		sm:   NewServiceManager(p2pm),
		vm:   utils.NewValidatorManager(p2pm.cm),
		tm:   utils.NewTextManager(),
		pm:   price.NewPriceManager(p2pm.DB, p2pm.Lm),
		rm:   resource.NewResourceManager(p2pm.DB, p2pm.Lm),
		cm:   currency.NewCurrencyManager(p2pm.DB, p2pm.Lm),
		wm:   workflow.NewWorkflowManager(p2pm.DB, p2pm.Lm, p2pm.cm),
		jm:   NewJobManager(p2pm),
	}
}

func (mm *MenuManager) Close() error {
	if mm.lm != nil {
		return mm.lm.Close()
	}
	return nil
}

// Clears the terminal based on the operating system.
func (mm *MenuManager) clearTerminal() {
	switch runtime.GOOS {
	case "linux", "darwin":
		cmd := exec.Command("clear")
		cmd.Stdout = os.Stdout
		cmd.Run()
	case "windows":
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		cmd.Run()
	default:
		// For unsupported OS, use a series of newlines
		fmt.Print(strings.Repeat("\n", 100))
	}
}

// Displays the TrustFlow Network ASCII art logo.
func (mm *MenuManager) drawLogo() {
	color.Set(color.FgHiBlue)
	fmt.Println(`
 _____               _   _____ _               
|_   _|             | | |  ___| |              
  | |_ __ _   _ ___| |_| |_  | | _____      __
  | | '__| | | / __| __|  _| | |/ _ \ \ /\ / /
  | | |  | |_| \__ \ |_| |   | | (_) \ V  V / 
  \_/_|   \__,_|___/\__\_|   |_|\___/ \_/\_/  
                                               
                                             `)
	color.Set(color.FgHiCyan)
	fmt.Println("           N E T W O R K          ")
	color.Unset()
}

// Print menu
func (mm *MenuManager) Run() {
	//mm.clearTerminal()
	mm.drawLogo()
	mm.main()
}

// Select prompt helper
func (mm *MenuManager) selectPromptHelper(label string, items []string, cursPos int, size int, props *promptui.Select) (int, string, error) {
	var prompt promptui.Select
	if props != nil {
		prompt = *props
	} else {
		prompt = promptui.Select{
			Label:     label,
			Items:     items,
			CursorPos: cursPos,
			Size:      size,
		}
	}

	pos, result, err := prompt.Run()
	if err != nil {
		msg := fmt.Sprintf("Prompt `%s` failed: %s", label, err.Error())
		fmt.Println(msg)
		mm.lm.Log("error", msg, "menu")
		return -1, "", err
	}

	return pos, result, nil
}

// Input prompt helper
func (mm *MenuManager) inputPromptHelper(label string, def string, validate promptui.ValidateFunc, props *promptui.Prompt) (string, error) {
	var prompt promptui.Prompt
	if props != nil {
		prompt = *props
	} else {
		prompt = promptui.Prompt{
			Label:       label,
			Default:     def,
			Validate:    validate,
			AllowEdit:   true,
			HideEntered: false,
			IsConfirm:   false,
			IsVimMode:   false,
		}
	}

	result, err := prompt.Run()
	if err != nil {
		msg := fmt.Sprintf("Entering `%s` failed: %s", label, err.Error())
		fmt.Println(msg)
		mm.lm.Log("error", msg, "menu")
		return "", err
	}

	return result, nil
}

// Confirm prompt helper
func (mm *MenuManager) confirmPromptHelper(label string) (bool, error) {
	prompt := promptui.Prompt{
		Label:     label,
		IsConfirm: true,
	}
	result, err := prompt.Run()
	if err != nil && strings.ToLower(result) != "n" && strings.ToLower(result) != "y" {
		fmt.Printf("Prompt `%s` failed %v\n", label, err)
		return false, err
	}
	return strings.ToLower(result) == "y", nil
}

// Print main menu
func (mm *MenuManager) main() {
	for {
		_, val, err := mm.selectPromptHelper(
			"Main",
			[]string{"Workflows & Jobs", "Configure node", "Exit"},
			0, 10, nil)
		if err != nil {
			return
		}

		switch val {
		case "Workflows & Jobs":
			mm.workflows()
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

// Print rworkflows sub-menu
func (mm *MenuManager) workflows() {
	for {
		_, val, err := mm.selectPromptHelper(
			"Main \U000025B6 Workflows & Jobs",
			[]string{"Find services", "Request services", "List workflows", "Run workflow", "Back"},
			0, 10, nil)
		if err != nil {
			continue
		}

		switch val {
		case "Find services":
			mm.findServices()
		case "Request services":
			mm.requestService()
		case "List workflows":
			mm.printWorkflows(mm.wm)
		case "Run workflow":
			mm.runWorkflow()
		case "Back":
			return
		}
	}
}

// Print find services sub-menu
func (mm *MenuManager) findServices() error {
	// Search phrases
	snResult, err := mm.inputPromptHelper("Search phrases: (comma-separated)", "", mm.vm.MinLen, nil)
	if err != nil {
		return err
	}

	// Get service type
	fmt.Println("Select service type:")
	_, rResult, err := mm.selectPromptHelper(
		"Main \U000025B6 Find services \U000025B6 Service Type",
		[]string{"ANY", "DATA", "DOCKER EXECUTION ENVIRONMENT", "STANDALONE EXECUTABLE"},
		0, 10, nil)
	if err != nil {
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
	table.Header([]string{"Service Id", "Name", "Type"})
	tableP := tablewriter.NewWriter(os.Stdout)
	tableP.Header([]string{"Resource", "Resource Unit", "Price", "Currency"})
	row := []string{fmt.Sprintf("%s-%d", service.NodeId, service.Id), mm.tm.Shorten(service.Name, 17, 0), service.Type}
	table.Append(row)
	table.Render() // Prints the table
	fmt.Printf("---\nDescription: %s\n", service.Description)
	if len(service.Interfaces) > 0 {
		fmt.Print("---\nService expects following inputs/outputs:")
		for i, intfce := range service.Interfaces {
			fmt.Printf("\n%d.	%s\n	%s\n	%s\n",
				i+1, intfce.InterfaceType,
				intfce.Description, intfce.Path)
		}
	} else {
		fmt.Print("---\nService does not expect any inputs or outputs.\n")
	}
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
	table.Header([]string{"Node Id", "Service Id", "Job Id", "Message", "Accpeted"})
	row := []string{serviceResponse.NodeId, fmt.Sprintf("%d", serviceResponse.ServiceId), fmt.Sprintf("%d", serviceResponse.JobId), serviceResponse.Message, fmt.Sprintf("%t", serviceResponse.Accepted)}
	table.Append(row)
	table.Render() // Prints the table
	fmt.Print("\n------------\nPress any key to continue...\n")
}

// Print request service sub-menu
func (mm *MenuManager) requestService() error {
	//	var entrypoint, commands []string

	// Service Id
	sidResult, err := mm.inputPromptHelper("Service ID", "", mm.vm.NotEmpty, nil)
	if err != nil {
		return err
	}

	// Extract node id and service id
	serviceIdPair := strings.Split(sidResult, "-")
	if len(serviceIdPair) < 2 {
		msg := fmt.Sprintf("Invalid Service ID: %s", sidResult)
		fmt.Println(msg)
		mm.lm.Log("error", msg, "menu")
		return err
	}

	// Check Service Offers Cache
	//	serviceOffer, exists := mm.p2pm.sc.ServiceOffers[sidResult]
	//	if !exists {
	//		msg := fmt.Sprintf("Could not find provided Service ID `%s` in Service Offers Cache.\nPlease use `Find Services` option first to look up for remote services.", sidResult)
	//		fmt.Println(msg)
	//		mm.lm.Log("error", msg, "menu")
	//		return err
	//	}

	peerId := serviceIdPair[0]
	sid := serviceIdPair[1]

	// Generate peer id
	peer, err := mm.p2pm.GeneratePeerAddrInfo(peerId)
	if err != nil {
		fmt.Printf("Generating peer address info from %s failed: %s\n", peerId, err.Error())
		return err
	}

	// Validate service id
	serviceId, err := strconv.ParseInt(sid, 10, 32)
	if err != nil {
		fmt.Printf("Service Id %s seems to be invalid Id: %s\n", sid, err.Error())
		return err
	}
	//	if serviceOffer.Type == "DOCKER EXECUTION ENVIRONMENT" {
	//		entrypoint, commands, err = mm.imageEntrypointCommands(serviceOffer.Entrypoint, serviceOffer.Commands)
	//		if err != nil {
	//			fmt.Printf("Collecting service entrypoint and commands failed for service ID: %s. Error: %s\n", sid, err.Error())
	//			return err
	//		}
	//	}

	// Collect job interfaces
	//	serviceRequestInterfaces, inputsRequired, err := mm.jobInterfaces(serviceOffer.Interfaces)
	if err != nil {
		fmt.Printf("Could not collect job interfaces: %s\n", err.Error())
		return err
	}

	/*
		// Get constraint type
		fmt.Println("Select constraint type:")
		_, cResult, err := mm.selectPromptHelper(
			"Main \U000025B6 Request service \U000025B6 Constraint Type",
			[]string{"NONE", "INPUTS READY", "DATETIME", "JOBS EXECUTED", "MANUAL START"},
			0, 10, nil)
		if err != nil {
			return err
		}
	*/
	//	var cResult string = "NONE"
	//	if *inputsRequired {
	//		cResult = "INPUTS READY"
	//	}

	// Use existing workflow or create a new one service prompt
	var workflowId int64
	//	var workflowJobId int64

	nwResult, err := mm.confirmPromptHelper("Should we integrate this service or job into an existing workflow")
	if err != nil {
		return err
	}
	if nwResult {
		// Add job to existing workflow
		fmt.Println("Let's add this job to an existing workflow.")
		wfidResult, err := mm.inputPromptHelper("Workflow Id", "", mm.wm.IsRunnableWorkflow, nil)
		if err != nil {
			return err
		}

		workflowId, err = strconv.ParseInt(wfidResult, 10, 64)
		if err != nil {
			mm.lm.Log("debug", err.Error(), "workflows")
			return err
		}

	} else {
		// Create new workflow
		fmt.Println("Let's create a new workflow.")
		wfnResult, err := mm.inputPromptHelper("Workflow name", "", mm.vm.NotEmpty, nil)
		if err != nil {
			return err
		}

		wfdResult, err := mm.inputPromptHelper("Workflow description", "", nil, nil)
		if err != nil {
			return err
		}

		// Add workflow and a workflow job
		// Create workflow jobs
		workflowJobBase := node_types.WorkflowJobBase{
			NodeId:             peer.ID.String(),
			ServiceId:          serviceId,
			JobId:              0,
			ExpectedJobOutputs: "",
		}

		workflowJob := node_types.WorkflowJob{
			WorkflowJobBase: workflowJobBase,
		}

		workflowId, _, err = mm.wm.Add(wfnResult, wfdResult, []node_types.WorkflowJob{workflowJob})
		if err != nil {
			fmt.Printf("Adding new workflow failed: %s\n", err.Error())
			return err
		}
	}

	// Create workflow jobs
	workflowJobBase := node_types.WorkflowJobBase{
		NodeId:             peer.ID.String(),
		ServiceId:          serviceId,
		JobId:              0,
		ExpectedJobOutputs: "",
	}

	workflowJob := node_types.WorkflowJob{
		WorkflowJobBase: workflowJobBase,
	}

	_, err = mm.wm.AddWorkflowJobs(workflowId, []node_types.WorkflowJob{workflowJob})
	if err != nil {
		fmt.Printf("Adding new workflow job failed: %s\n", err.Error())
		return err
	}

	/*
		TODO, this method should be renamed to "addWorkflowJob"
		Below code should be moved to a new method "requestService"
		This will allow 'offline' workflow creation while requesting
		paying for service should come once flow is ready to run
	*/
	/*
		err = mm.jm.RequestJob(peer, workflowId, serviceId, entrypoint, commands, serviceRequestInterfaces, cResult, "")
		if err != nil {
			fmt.Printf("Requesting service failed: %s\n", err.Error())
			return err
		}
	*/

	return nil
}

func (mm *MenuManager) jobInterfaces(serviceInterfaces []node_types.Interface) ([]node_types.RequestInterface, *bool, error) {
	// Collect job interfaces
	var serviceRequestInterfaces []node_types.RequestInterface
	var err error

	inputsRequired := new(bool)
	for _, serviceInterface := range serviceInterfaces {
		*inputsRequired = false

		fmt.Println("The following is the Interface description as defined in the Service definition:")
		fmt.Println(serviceInterface.Description)

		var interfacePeers []node_types.JobInterfacePeer
		switch serviceInterface.InterfaceType {
		case "STDIN":
			// Input providing nodes
			interfacePeers, err = mm.stdJobInterfacePeers(serviceInterface, interfacePeers)
			if err != nil {
				return nil, nil, err
			}

			// Set flag for this service
			// that we have to wait for inputs
			// to be provided before running the service
			*inputsRequired = true

		case "STDOUT":
			// Output receiving nodes
			interfacePeers, err = mm.stdJobInterfacePeers(serviceInterface, interfacePeers)
			if err != nil {
				return nil, nil, err
			}

		case "MOUNT":
			interfacePeers, err = mm.mountJobInterfacePeers(serviceInterface, interfacePeers)
			if err != nil {
				return nil, nil, err
			}

			// Set flag for this service
			// that we have to wait for inputs
			// to be provided before running the service
			*inputsRequired = true
		default:
			fmt.Printf("Unknown interfaces type %s. Skipping...\n", serviceInterface.InterfaceType)
		}

		serviceRequestInterfaces = append(serviceRequestInterfaces, node_types.RequestInterface{
			JobInterfacePeers: interfacePeers,
			Interface:         serviceInterface,
		})
	}

	return serviceRequestInterfaces, inputsRequired, nil
}

func (mm *MenuManager) stdJobInterfacePeers(
	serviceInterface node_types.Interface,
	interfacePeers []node_types.JobInterfacePeer,
) ([]node_types.JobInterfacePeer, error) {
	var jobInterfacePeer node_types.JobInterfacePeer
	var msgNode, msgPredefinedPath, msgSpecifyFilePath, msgJobId string

	switch serviceInterface.InterfaceType {
	case "STDIN":
		msgNode = "Please specify the NodeId that will provide this input"
		msgPredefinedPath = fmt.Sprintf("The predefined input file name and path for the service is as follows:\n%s\n",
			serviceInterface.Path)
		msgSpecifyFilePath = "Please specify the file name and path that this node will provide"
	case "STDOUT":
		msgNode = "Please specify the NodeID that will receive the output"
		msgPredefinedPath = fmt.Sprintf("The predefined output file name and path for the service is as follows:\n%s\n",
			serviceInterface.Path)
		msgSpecifyFilePath = "Please specify the file name and path for the node to receive"
		msgJobId = "If a job on the receiving node is the receiver of this output, please specify the job ID; otherwise, leave the remaining entry as `0`"
	default:
		err := fmt.Errorf("unknown STD interface type `%s`", serviceInterface.InterfaceType)
		return nil, err
	}

	// Input providing / Output receiving node
	nsResult, err := mm.inputPromptHelper(msgNode, mm.p2pm.h.ID().String(), mm.vm.IsPeer, nil)
	if err != nil {
		return nil, err
	}

	// Set node Id
	jobInterfacePeer.PeerNodeId = nsResult

	// Input / Output path
	if serviceInterface.Path != "" {
		fmt.Print(msgPredefinedPath)

		// Copy path to peer path
		jobInterfacePeer.PeerPath = serviceInterface.Path
	} else {
		// Collect peer path
		var validator func(path string) error
		osType := runtime.GOOS
		switch osType {
		case "linux", "darwin":
			validator = mm.vm.IsValidFileNameUnix
		case "windows":
			validator = mm.vm.IsValidFileNameWindows
		default:
			err := fmt.Errorf("unsupported OS type `%s`", osType)
			return nil, err
		}

		fnResult, err := mm.inputPromptHelper(msgSpecifyFilePath, "", validator, nil)
		if err != nil {
			return nil, err
		}

		// Copy path to peer path
		jobInterfacePeer.PeerPath = fnResult
	}

	// Output receiving job Id
	if serviceInterface.InterfaceType == "STDOUT" {
		jidResult, err := mm.inputPromptHelper(msgJobId, "0", mm.vm.IsInt64, nil)
		if err != nil {
			return nil, err
		}

		jobInterfacePeer.PeerJobId, err = mm.tm.ToInt64(jidResult)
		if err != nil {
			return nil, err
		}
	}

	interfacePeers = append(interfacePeers, jobInterfacePeer)

	// Print "add another peer for the job interface" prompt
	aaResult, err := mm.confirmPromptHelper("Add another peer to the interface")
	if err != nil {
		return nil, err
	}
	if aaResult {
		return mm.stdJobInterfacePeers(serviceInterface, interfacePeers)
	}

	return interfacePeers, nil
}

func (mm *MenuManager) mountJobInterfacePeers(
	serviceInterface node_types.Interface,
	interfacePeers []node_types.JobInterfacePeer,
) ([]node_types.JobInterfacePeer, error) {
	var jobInterfacePeer node_types.JobInterfacePeer

	if serviceInterface.InterfaceType != "MOUNT" {
		err := fmt.Errorf("provided interface type `%s` is wrong. Interface type must be `MOUNT`", serviceInterface.InterfaceType)
		return nil, err
	}

	// Specify input/output Node IDs
	fmt.Printf("The specified file system mount point within the service's environment is `%s`\n", serviceInterface.Path)

	// Determine path validator os type
	var validator func(path string) error
	osType := runtime.GOOS
	switch osType {
	case "linux", "darwin":
		validator = mm.vm.IsValidFileNameOrMountPointUnix
	case "windows":
		validator = mm.vm.IsValidFileNameOrMountPointWindows
	default:
		err := fmt.Errorf("unsupported OS type `%s`", osType)
		return nil, err
	}

	// Ask will this be a providing or receiving node
	prResult, err := mm.confirmPromptHelper("Will you add a node that provides inputs to the job at the mount point (press Y), or will you add a node that receives outputs from a job at this mount point (press N)")
	if err != nil {
		return nil, err
	}
	if prResult {
		// Input providing node
		inResult, err := mm.inputPromptHelper("Please specify the NodeId that will provide this input", mm.p2pm.h.ID().String(), mm.vm.IsPeer, nil)
		if err != nil {
			return nil, err
		}

		// Collect peer path
		fnResult, err := mm.inputPromptHelper("Please specify the file name and path that this node will provide", "", validator, nil)
		if err != nil {
			return nil, err
		}

		jobInterfacePeer = node_types.JobInterfacePeer{
			PeerNodeId:        inResult,
			PeerMountFunction: "PROVIDER",
			PeerPath:          fnResult,
		}

	} else {
		// Output receiving node
		onResult, err := mm.inputPromptHelper("Please specify the NodeID that will receive the output", mm.p2pm.h.ID().String(), mm.vm.IsPeer, nil)
		if err != nil {
			return nil, err
		}

		// Collect peer path
		fnResult, err := mm.inputPromptHelper("Please specify the file name and path for the node to receive", "", validator, nil)
		if err != nil {
			return nil, err
		}

		jobInterfacePeer = node_types.JobInterfacePeer{
			PeerNodeId:        onResult,
			PeerMountFunction: "RECEIVER",
			PeerPath:          fnResult,
			PeerJobId:         0,
		}

		jidResult, err := mm.inputPromptHelper("If a job on the receiving node is the receiver of this output, please specify the job ID; otherwise, leave the remaining entry as `0`", "0", mm.vm.IsInt64, nil)
		if err != nil {
			return nil, err
		}

		jobInterfacePeer.PeerJobId, err = mm.tm.ToInt64(jidResult)
		if err != nil {
			return nil, err
		}
	}

	interfacePeers = append(interfacePeers, jobInterfacePeer)

	// Print "add another peer for the job interface" prompt
	aaResult, err := mm.confirmPromptHelper("Add another peer to the interface")
	if err != nil {
		return nil, err
	}
	if aaResult {
		return mm.mountJobInterfacePeers(serviceInterface, interfacePeers)
	}

	return interfacePeers, nil
}

func (mm *MenuManager) serviceInterfaces(nodeId string, osType string, interfaces []node_types.Interface) ([]node_types.Interface, error) {
	var pthResult string = ""

	_, tyResult, err := mm.selectPromptHelper(
		"Interface type",
		[]string{"STDIN", "STDOUT", "MOUNT"},
		0, 10, nil)
	if err != nil {
		return nil, err
	}

	// Determine path validator os type
	var validator func(path string) error

	switch tyResult {
	case "STDIN":
		switch osType {
		case "linux", "darwin":
			validator = mm.vm.IsValidFileNameUnix
		case "windows":
			validator = mm.vm.IsValidFileNameWindows
		default:
			err := fmt.Errorf("unsupported OS type `%s`", osType)
			return nil, err
		}

		msg := "Specify the file name and path expected by the service (optional). Leave blank for a non-specific file name"
		pthResult, err = mm.inputPromptHelper(msg, "", nil, nil)
		if err != nil {
			return nil, err
		}

		if pthResult != "" {
			if err := validator(pthResult); err != nil {
				return nil, err
			}
		}

	case "STDOUT":
		switch osType {
		case "linux", "darwin":
			validator = mm.vm.IsValidFileNameUnix
		case "windows":
			validator = mm.vm.IsValidFileNameWindows
		default:
			err := fmt.Errorf("unsupported OS type `%s`", osType)
			return nil, err
		}

		msg := "Specify the file name and path produced by the service (optional). Leave blank for a non-specific file name"
		pthResult, err = mm.inputPromptHelper(msg, "", nil, nil)
		if err != nil {
			return nil, err
		}

		if pthResult != "" {
			if err := validator(pthResult); err != nil {
				return nil, err
			}
		}

	case "MOUNT":
		switch osType {
		case "linux", "darwin":
			validator = mm.vm.IsValidAbsoluteMountPointUnix
		case "windows":
			validator = mm.vm.IsValidAbsoluteMountPointWindows
		default:
			err := fmt.Errorf("unsupported OS type `%s`", osType)
			return nil, err
		}

		pthResult, err = mm.inputPromptHelper("Please specify the file system mount point within the service's environment", "", validator, nil)
		if err != nil {
			return nil, err
		}

	default:
		err := fmt.Errorf("unsupported interface type `%s`", tyResult)
		return nil, err
	}

	dResult, err := mm.inputPromptHelper("Short description", "", mm.vm.NotEmpty, nil)
	if err != nil {
		return nil, err
	}

	intrfce := node_types.Interface{
		InterfaceType: tyResult,
		Description:   dResult,
		Path:          pthResult,
	}

	interfaces = append(interfaces, intrfce)

	// Print "add another interface" prompt
	aaResult, err := mm.confirmPromptHelper("Add another interface")
	if err != nil {
		return nil, err
	}
	if aaResult {
		return mm.serviceInterfaces(nodeId, osType, interfaces)
	}

	return interfaces, nil
}

func (mm *MenuManager) printWorkflows(wm *workflow.WorkflowManager, params ...uint32) error {
	var offset uint32 = 0
	var limit uint32 = 10

	l := mm.p2pm.cm.GetConfigWithDefault("search_results", "10")
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

	workflows, err := wm.List(offset, limit)
	if err != nil {
		return err
	}

	// Draw table output
	textManager := utils.NewTextManager()
	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"ID", "Name", "Description", "Status"})
	for _, workflow := range workflows {
		row := []string{fmt.Sprintf("%d", workflow.Id), textManager.Shorten(workflow.Name, 17, 0), textManager.Shorten(workflow.Description, 17, 0), ""}
		table.Append(row)

		for i, job := range workflow.Jobs {
			row = []string{"", fmt.Sprintf("%d.%d Job ID:", workflow.Id, i+1), fmt.Sprintf("%d-%s-%d", workflow.Id, job.WorkflowJobBase.NodeId, job.WorkflowJobBase.JobId), job.WorkflowJobBase.Status}
			table.Append(row)
		}
	}
	table.Render() // Prints the table

	if len(workflows) >= int(limit) {
		// Print "load more" prompt
		lmResult, err := mm.confirmPromptHelper("Load more")
		if err != nil {
			return err
		}
		if lmResult {
			return mm.printWorkflows(wm, offset+limit, limit)
		}
	}

	return nil
}

// Print run workflow sub-menu
func (mm *MenuManager) runWorkflow() error {
	widResult, err := mm.inputPromptHelper("Workflow ID", "", mm.wm.IsRunnableWorkflow, nil)
	if err != nil {
		return err
	}

	id, err := strconv.ParseInt(widResult, 10, 32)
	if err != nil {
		msg := fmt.Sprintf("Failed casting workflow id %s to int64: %s", widResult, err.Error())
		fmt.Println(msg)
		mm.lm.Log("error", msg, "menu")
		return err
	}

	// Get workflow
	workflow, err := mm.wm.Get(id)
	if err != nil {
		msg := fmt.Sprintf("Could not obtain workflow data for workflow Id %d: %s", id, err.Error())
		fmt.Println(msg)
		mm.lm.Log("error", msg, "menu")
		return err
	}

	for _, job := range workflow.Jobs {
		peer, err := mm.p2pm.GeneratePeerAddrInfo(job.WorkflowJobBase.NodeId)
		if err != nil {
			mm.lm.Log("error", err.Error(), "menu")
			return err
		}
		err = mm.jm.RequestJobRun(peer, workflow.Id, job.WorkflowJobBase.JobId)
		if err != nil {
			mm.lm.Log("error", err.Error(), "menu")
			return err
		}
	}

	return nil
}

func (mm *MenuManager) printJobRunResponse(jobRunResponse node_types.JobRunResponse) {
	fmt.Printf("\n")
	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"Node Id", "Job Id", "Message", "Accpeted"})
	row := []string{jobRunResponse.NodeId, fmt.Sprintf("%d", jobRunResponse.JobId), jobRunResponse.Message, fmt.Sprintf("%t", jobRunResponse.Accepted)}
	table.Append(row)
	table.Render() // Prints the table
	fmt.Print("\n------------\nPress any key to continue...\n")
}

// Print configure node sub-menu
func (mm *MenuManager) configureNode() {
	for {
		_, result, err := mm.selectPromptHelper(
			"Main \U000025B6 Configure node",
			[]string{"Blacklist", "Currencies", "Resources", "Services", "Settings", "Back"},
			0, 10, nil)
		if err != nil {
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
		_, result, err := mm.selectPromptHelper(
			"Main \U000025B6 Configure node \U000025B6 Blacklist",
			[]string{"List nodes", "Add node", "Remove node", "Back"},
			0, 10, nil)
		if err != nil {
			continue
		}

		switch result {
		case "List nodes":
			err := mm.listBlacklistNodes()
			if err != nil {
				continue
			}
		case "Add node":
			err := mm.addBlacklistNode()
			if err != nil {
				continue
			}
		case "Remove node":
			err := mm.removeNodeFromBlacklist()
			if err != nil {
				continue
			}
		case "Back":
			return
		}
	}
}
func (mm *MenuManager) listBlacklistNodes() error {
	blacklistManager, err := blacklist_node.NewBlacklistNodeManager(mm.p2pm.DB, mm.p2pm.UI, mm.p2pm.Lm)
	if err != nil {
		fmt.Println(err.Error())
		mm.lm.Log("error", err.Error(), "menu")
		return err
	}

	err = mm.printBlacklist(blacklistManager)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
	}

	return nil
}

func (mm *MenuManager) addBlacklistNode() error {
	// Get node ID
	nidResult, err := mm.inputPromptHelper("Node ID", "", mm.vm.IsPeer, nil)
	if err != nil {
		return err
	}

	// Get reason for blacklisting node
	rsResult, err := mm.inputPromptHelper("Reason (optional)", "", nil, nil)
	if err != nil {
		return err
	}

	// Add node to a blacklist
	blacklistManager, err := blacklist_node.NewBlacklistNodeManager(mm.p2pm.DB, mm.p2pm.UI, mm.p2pm.Lm)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
		return err
	}

	err = blacklistManager.Add(nidResult, rsResult)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
		return err
	}
	fmt.Printf("\U00002705 Node %s is added to blacklist\n", nidResult)

	// Print updated blacklist
	err = mm.printBlacklist(blacklistManager)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
	}

	return nil
}

func (mm *MenuManager) removeNodeFromBlacklist() error {
	// Get node ID
	nidResult, err := mm.inputPromptHelper("Node ID", "", mm.vm.IsPeer, nil)
	if err != nil {
		return err
	}

	// Remove node from blacklist
	blacklistManager, err := blacklist_node.NewBlacklistNodeManager(mm.p2pm.DB, mm.p2pm.UI, mm.p2pm.Lm)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
		return err
	}

	err = blacklistManager.Remove(nidResult)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
		return err
	}
	fmt.Printf("\U00002705 Node %s is removed from blacklist\n", nidResult)

	// Print updated blacklist
	err = mm.printBlacklist(blacklistManager)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
	}

	return nil
}

func (mm *MenuManager) printBlacklist(blnm *blacklist_node.BlacklistNodeManager) error {
	nodes, err := blnm.List()
	if err != nil {
		return err
	}

	// Draw table output
	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"Node ID", "Reason", "Timestamp"})
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
		_, result, err := mm.selectPromptHelper(
			"Main \U000025B6 Configure node \U000025B6 Currencies",
			[]string{"List currencies", "Add currency", "Remove currency", "Back"},
			0, 10, nil)
		if err != nil {
			continue
		}

		switch result {
		case "List currencies":
			err := mm.listCurrencies()
			if err != nil {
				continue
			}
		case "Add currency":
			err := mm.addCurrency()
			if err != nil {
				continue
			}
		case "Remove currency":
			err := mm.removeCurrency()
			if err != nil {
				continue
			}
		case "Back":
			return
		}
	}
}

func (mm *MenuManager) listCurrencies() error {
	err := mm.printCurrencies(mm.cm)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
	}
	return err
}

func (mm *MenuManager) addCurrency() error {
	// Get currency symbol
	csResult, err := mm.inputPromptHelper("Currency Symbol", "", mm.vm.NotEmpty, nil)
	if err != nil {
		return err
	}

	// Get currency name
	cnResult, err := mm.inputPromptHelper("Currency name", "", mm.vm.NotEmpty, nil)
	if err != nil {
		return err
	}

	// Add currency
	err = mm.cm.Add(cnResult, csResult)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
		return err
	}
	fmt.Printf("\U00002705 Currency %s (%s) is added\n", cnResult, csResult)

	// Print updated list of currencies
	err = mm.printCurrencies(mm.cm)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
	}

	return nil
}

func (mm *MenuManager) removeCurrency() error {
	// Get currency symbol
	csResult, err := mm.inputPromptHelper("Currency Symbol", "", mm.vm.NotEmpty, nil)
	if err != nil {
		return err
	}

	// Remove currency
	err = mm.cm.Remove(csResult)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
		return err
	}
	fmt.Printf("\U00002705 Currency %s is removed\n", csResult)

	// Print updated list of currencies
	err = mm.printCurrencies(mm.cm)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
	}

	return nil
}

func (mm *MenuManager) printCurrencies(cm *currency.CurrencyManager) error {
	currencies, err := cm.List()
	if err != nil {
		return err
	}

	// Draw table output
	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"Currency", "Symbol"})
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
		_, result, err := mm.selectPromptHelper(
			"Main \U000025B6 Configure node \U000025B6 Resources",
			[]string{"List resources", "Set resource active", "Set resource inactive", "Add resource", "Remove resource", "Back"},
			0, 10, nil)
		if err != nil {
			continue
		}

		switch result {
		case "List resources":
			err := mm.listResources()
			if err != nil {
				continue
			}
		case "Set resource active":
			err := mm.setResourceActive()
			if err != nil {
				continue
			}
		case "Set resource inactive":
			err := mm.setResourceInactive()
			if err != nil {
				continue
			}
		case "Add resource":
			err := mm.addResource()
			if err != nil {
				continue
			}
		case "Remove resource":
			err := mm.removeResource()
			if err != nil {
				continue
			}
		case "Back":
			return
		}
	}
}

func (mm *MenuManager) listResources() error {
	err := mm.printResources(mm.rm)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
	}
	return err
}

func (mm *MenuManager) setResourceActive() error {
	// Get resource Id
	rnResult, err := mm.inputPromptHelper("Resource Id", "", mm.vm.IsInt64, nil)
	if err != nil {
		return err
	}

	// Set resource active
	rn, err := mm.tm.ToInt64(rnResult)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
		return err
	}
	err = mm.rm.SetActive(rn)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
		return err
	}
	fmt.Printf("\U00002705 Resource id %d is set to active\n", rn)

	// Print updated list of resources
	err = mm.listResources()

	return err
}

func (mm *MenuManager) setResourceInactive() error {
	// Get resource Id
	rnResult, err := mm.inputPromptHelper("Resource Id", "", mm.vm.IsInt64, nil)
	if err != nil {
		return err
	}

	// Set resource inactive
	rn, err := mm.tm.ToInt64(rnResult)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
		return err
	}
	err = mm.rm.SetInactive(rn)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
		return err
	}
	fmt.Printf("\U00002705 Resource id %d is set to inactive\n", rn)

	// Print updated list of resources
	err = mm.listResources()

	return err
}

func (mm *MenuManager) addResource() error {
	// Get resource group name
	rgResult, err := mm.inputPromptHelper("Resource group", "", mm.vm.NotEmpty, nil)
	if err != nil {
		return err
	}

	// Get resource name
	rnResult, err := mm.inputPromptHelper("Resource name", "", mm.vm.NotEmpty, nil)
	if err != nil {
		return err
	}

	// Get resource unit name
	ruResult, err := mm.inputPromptHelper("Resource unit", "", mm.vm.NotEmpty, nil)
	if err != nil {
		return err
	}

	// Get resource description
	rdResult, err := mm.inputPromptHelper("Resource description", "", nil, nil)
	if err != nil {
		return err
	}

	// Get resource state
	raResult, err := mm.confirmPromptHelper("Is active")
	if err != nil {
		return err
	}

	// Add resource
	err = mm.rm.Add(rgResult, rnResult, ruResult, rdResult, raResult)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
		return err
	}
	fmt.Printf("\U00002705 Resource %s is added\n", rnResult)

	// Print updated list of resources
	err = mm.listResources()

	return err
}

func (mm *MenuManager) removeResource() error {
	// Get resource id
	rnResult, err := mm.inputPromptHelper("Resource Id", "", mm.vm.IsInt64, nil)
	if err != nil {
		return err
	}

	// Remove resource
	rn, err := mm.tm.ToInt64(rnResult)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
		return err
	}
	err = mm.rm.Remove(rn)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
		return err
	}
	fmt.Printf("\U00002705 Resource id %d is removed\n", rn)

	err = mm.listResources()

	return err
}

func (mm *MenuManager) printResources(rm *resource.ResourceManager) error {
	resources, err := rm.List()
	if err != nil {
		return err
	}

	// Draw table output
	table := tablewriter.NewWriter(os.Stdout)
	table.Header([]string{"Id", "Resource Group", "Resource", "Resource Unit", "Active"})
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
		_, result, err := mm.selectPromptHelper(
			"Main \U000025B6 Configure node \U000025B6 Services",
			[]string{"List services", "Show service details", "Add service", "Set service active", "Set service inactive", "Remove service", "Back"},
			0, 10, nil)
		if err != nil {
			continue
		}

		switch result {
		case "List services":
			err := mm.listServices()
			if err != nil {
				continue
			}
		case "Show service details":
		case "Add service":
			err := mm.addService()
			if err != nil {
				continue
			}
		case "Set service active":
			err := mm.setServiceActive(true)
			if err != nil {
				continue
			}
		case "Set service inactive":
			err := mm.setServiceActive(false)
			if err != nil {
				continue
			}
		case "Remove service":
			err := mm.removeService()
			if err != nil {
				continue
			}
		case "Back":
			return
		}
	}
}
func (mm *MenuManager) listServices() error {
	err := mm.printServices(mm.sm)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
	}
	return err
}

func (mm *MenuManager) addService() error {
	// Get service name
	snResult, err := mm.inputPromptHelper("Service name", "", mm.vm.NotEmpty, nil)
	if err != nil {
		return err
	}

	// Get service description
	sdResult, err := mm.inputPromptHelper("Service description", "", nil, nil)
	if err != nil {
		return err
	}

	// Get service state
	raResult, err := mm.confirmPromptHelper("Is active")
	if err != nil {
		return err
	}

	_, stResult, err := mm.selectPromptHelper(
		"Main \U000025B6 Configure node \U000025B6 Services \U000025B6 Add Service \U000025B6 Service Type",
		[]string{"DATA", "DOCKER EXECUTION ENVIRONMENT", "STANDALONE EXECUTABLE"},
		0, 10, nil)
	if err != nil {
		return err
	}

	switch stResult {
	case "DATA":
		err := mm.addDataService(snResult, sdResult, stResult, raResult)
		if err != nil {
			return err
		}
	case "DOCKER EXECUTION ENVIRONMENT":
		err := mm.addDockerService(snResult, sdResult, stResult, raResult)
		if err != nil {
			return err
		}
	case "STANDALONE EXECUTABLE":
	}

	// Service is added
	fmt.Printf("\U00002705 Service %s is added\n", snResult)

	// Print service table
	err = mm.listServices()

	return err
}

func (mm *MenuManager) addDataService(name, description, stype string, active bool) error {
	// Get file/folder/data path
	dpResult, err := mm.inputPromptHelper("Path", "", mm.vm.NotEmpty, nil)
	if err != nil {
		return err
	}

	// Add service
	id, err := mm.sm.Add(name, description, stype, active)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
		return err
	}

	// Add data service
	_, err = mm.sm.AddData(id, dpResult)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
		mm.sm.Remove(id)
		return err
	}

	// Print service pricing prompt
	err = mm.printServicePrice(id)
	if err != nil {
		mm.lm.Log("error", err.Error(), "menu")
		mm.sm.Remove(id)
		return err
	}

	return nil
}

func (mm *MenuManager) addInterfaces(question string, osType string) ([]node_types.Interface, error) {
	// Define inputs/outputs
	var interfaces []node_types.Interface

	// Inputs
	qResult, err := mm.confirmPromptHelper(question)
	if err != nil {
		return nil, err
	}
	if qResult {
		intfcs, err := mm.serviceInterfaces(mm.p2pm.h.ID().String(), osType, interfaces)
		if err != nil {
			fmt.Printf("Collecting interfaces failed %v\n", err)
			return nil, err
		}
		interfaces = append(interfaces, intfcs...)
	}

	return interfaces, nil
}

func (mm *MenuManager) addDockerService(name, description, stype string, active bool) error {
	// Do we have docker image prepared or
	// we will create it fron git repo?
	deetResult, err := mm.confirmPromptHelper("Have a pre-built Docker image? Enter 'Y' and image name to pull. Else, enter 'N' and Git repo URL with Dockerfile or docker-compose.yml")
	if err != nil {
		return err
	}
	if !deetResult {
		return mm.addDockerServiceFromGit(name, description, stype, active)
	} else {
		return mm.addDockerServiceFromRepo(name, description, stype, active)
	}
}

func (mm *MenuManager) addDockerServiceFromGit(name, description, stype string, active bool) error {
	// Pull from git and compose Docker image
	gruResult, err := mm.inputPromptHelper("Git repository URL", "", mm.vm.NotEmpty, nil)
	if err != nil {
		return err
	}

	// If this is private repo ask for credentials
	var username string = ""
	var token string = ""

	iprResult, err := mm.confirmPromptHelper("Is this private repo")
	if err != nil {
		return err
	}
	if iprResult {
		// Ask for Git credentials
		username, err = mm.inputPromptHelper("Git user", "", nil, nil)
		if err != nil {
			return err
		}
		token, err = mm.inputPromptHelper("Git token/password", "", nil, nil)
		if err != nil {
			return err
		}
	}

	// Validate repo
	gitManager := repo.NewGitManager(mm.p2pm.cm)
	err = gitManager.ValidateRepo(gruResult, username, token)
	if err != nil {
		msg := fmt.Sprintf("\U00002757 Failed to access Git repo: %v\n", err)
		fmt.Println(msg)
		mm.lm.Log("error", msg, "menu")
		return err
	}

	// Ask for Git branch
	branch, err := mm.inputPromptHelper("Set branch for pull/clone (optional, blank for default)", "", nil, nil)
	if err != nil {
		return err
	}

	// Pull/clone
	gitRoot := mm.p2pm.cm.GetConfigWithDefault("local_git_root", "./local_storage/git/")
	repoPath, err := gitManager.CloneOrPull(gitRoot, gruResult, branch, username, token)
	if err != nil {
		msg := fmt.Sprintf("\U00002757 Failed pulling/cloning repo %s: %v\n", gruResult, err)
		fmt.Println(msg)
		mm.lm.Log("error", msg, "menu")
		return err
	}

	// Check for docker files
	dockerCheckResult, err := gitManager.CheckDockerFiles(repoPath)
	if err != nil {
		msg := fmt.Sprintf("\U00002757 Failed checking repo for docker files: %v\n", err)
		fmt.Println(msg)
		mm.lm.Log("error", msg, "menu")
		return err
	}
	if !dockerCheckResult.HasDockerfile && !dockerCheckResult.HasCompose {
		msg := fmt.Sprintf("\U00002757 Repo '%s' has neither Dockerfile nor docker-compose.yml\n", gruResult)
		fmt.Println(msg)
		mm.lm.Log("error", msg, "menu")
		os.RemoveAll(repoPath)
		return err
	}

	// Run docker & build image(s)
	dockerManager := repo.NewDockerManager(ui.CLI{}, mm.lm, mm.p2pm.cm, mm.p2pm.GetGoroutineTracker())
	_, images, errors := dockerManager.Run(repoPath, nil, true, "", true, "", "", nil, nil, nil, nil, nil)
	if errors != nil {
		for _, err := range errors {
			msg := fmt.Sprintf("\U00002757 Building image(s) from repo '%s' ended with following error: %s\n", gruResult, err.Error())
			fmt.Println(msg)
			mm.lm.Log("error", msg, "menu")
		}
		os.RemoveAll(repoPath)
		return err
	}

	var imagesWithInterfaces []node_types.DockerImageWithInterfaces
	for _, img := range images {
		msg := fmt.Sprintf("\U00002705 Successfully built image: %s (%s), tags: %v, digests: %v from repo %s\n", img.Name, img.Id, img.Tags, img.Digests, gruResult)
		fmt.Println(msg)
		mm.lm.Log("debug", msg, "menu")

		// Define inputs/outputs
		interfaces, err := mm.addInterfaces(
			"Does the service's image require inputs to run, produce outputs, or need mount points to be defined",
			img.Os,
		)
		if err != nil {
			return err
		}

		entrypoint, commands, err := mm.imageEntrypointCommands(img.EntryPoints, img.Commands)
		if err != nil {
			return err
		}

		if len(entrypoint) > 0 {
			img.EntryPoints = entrypoint
		}

		if len(commands) > 0 {
			img.Commands = commands
		}

		imageWithInterfaces := node_types.DockerImageWithInterfaces{
			DockerImage: img,
			Interfaces:  interfaces,
		}

		imagesWithInterfaces = append(imagesWithInterfaces, imageWithInterfaces)
	}

	// Add service
	id, err := mm.sm.Add(name, description, stype, active)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
		return err
	}

	// Add doker service
	_, err = mm.sm.AddDocker(id, repoPath, gruResult, branch, username, token, dockerCheckResult, imagesWithInterfaces)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
		mm.sm.Remove(id)
		return err
	}

	// Print service pricing prompt
	err = mm.printServicePrice(id)
	if err != nil {
		mm.lm.Log("error", err.Error(), "menu")
		mm.sm.Remove(id)
		return err
	}

	return nil
}

func (mm *MenuManager) addDockerServiceFromRepo(name, description, stype string, active bool) error {
	// Pull existing Docker image
	pediResult, err := mm.inputPromptHelper("Pre-built Docker image name to pull", "", mm.vm.NotEmpty, nil)
	if err != nil {
		return err
	}

	// Validate docker image
	dockerManager := repo.NewDockerManager(ui.CLI{}, mm.lm, mm.p2pm.cm, mm.p2pm.GetGoroutineTracker())
	cmdOut, err := dockerManager.ValidateImage(pediResult)
	if err != nil {
		msg := fmt.Sprintf("\U00002757 Docker image check failed: %v\nOutput: %s", err, string(cmdOut))
		fmt.Println(msg)
		mm.lm.Log("error", msg, "menu")
		return err
	}

	// Pull image
	_, images, errors := dockerManager.Run("", nil, true, pediResult, true, "", "", nil, nil, nil, nil, nil)
	if errors != nil {
		for _, err := range errors {
			msg := fmt.Sprintf("\U00002757 Pulling image '%s' ended with following error: %s\n", pediResult, err.Error())
			fmt.Println(msg)
			mm.lm.Log("error", msg, "menu")
		}
		return err
	}

	var imagesWithInterfaces []node_types.DockerImageWithInterfaces
	for _, img := range images {
		msg := fmt.Sprintf("\U00002705 Successfully pulled image: %s (%s), tags: %v, digests: %v from repo %s\n", img.Name, img.Id, img.Tags, img.Digests, pediResult)
		fmt.Println(msg)
		mm.lm.Log("debug", msg, "menu")

		// Define inputs/outputs
		interfaces, err := mm.addInterfaces(
			"Does the service's image require inputs to run, produce outputs, or need mount points to be defined",
			img.Os,
		)
		if err != nil {
			return err
		}

		entrypoint, commands, err := mm.imageEntrypointCommands(img.EntryPoints, img.Commands)
		if err != nil {
			return err
		}

		if len(entrypoint) > 0 {
			img.EntryPoints = entrypoint
		}

		if len(commands) > 0 {
			img.Commands = commands
		}

		imageWithInterfaces := node_types.DockerImageWithInterfaces{
			DockerImage: img,
			Interfaces:  interfaces,
		}

		imagesWithInterfaces = append(imagesWithInterfaces, imageWithInterfaces)
	}

	// Add service
	id, err := mm.sm.Add(name, description, stype, active)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
		return err
	}

	// Add doker service
	_, err = mm.sm.AddDocker(id, "", "", "", "", "", repo.DockerFileCheckResult{}, imagesWithInterfaces)
	if err != nil {
		fmt.Printf("\U00002757 %s\n", err.Error())
		mm.lm.Log("error", err.Error(), "menu")
		mm.sm.Remove(id)
		return err
	}

	// Print service pricing prompt
	err = mm.printServicePrice(id)
	if err != nil {
		mm.lm.Log("error", err.Error(), "menu")
		mm.sm.Remove(id)
		return err
	}

	return nil
}

func (mm *MenuManager) imageEntrypointCommands(epoint, cmd []string) ([]string, []string, error) {
	// Extract defaults
	sentrypoint := strings.Join(epoint, " ")
	scommands := strings.Join(cmd, " ")

	// Ask for custom entrypoint
	epResult, err := mm.inputPromptHelper("Please provide the Docker image Entrypoint (optional)", sentrypoint, nil, nil)
	if err != nil {
		return nil, nil, err
	}

	entrypoint, err := shlex.Split(epResult)
	if err != nil {
		return nil, nil, err
	}

	// Ask for custom commands
	cmdResult, err := mm.inputPromptHelper("Please provide the Docker image Commands (optional)", scommands, nil, nil)
	if err != nil {
		return nil, nil, err
	}

	commands, err := shlex.Split(cmdResult)
	if err != nil {
		return nil, nil, err
	}

	return entrypoint, commands, nil
}

func (mm *MenuManager) setServiceActive(active bool) error {
	// Get service Id
	rnResult, err := mm.inputPromptHelper("Service Id", "", mm.vm.IsInt64, nil)
	if err != nil {
		return err
	}

	// Set service active or inactive
	id, err := mm.tm.ToInt64(rnResult)
	if err != nil {
		msg := fmt.Sprintf("\U00002757 Service Id is not valid int64: %s", err.Error())
		fmt.Println(msg)
		mm.lm.Log("error", msg, "menu")
		return err
	}

	if active {
		err = mm.sm.SetActive(id)
		if err != nil {
			fmt.Printf("\U00002757 %s\n", err.Error())
			mm.lm.Log("error", err.Error(), "menu")
			return err
		}
		fmt.Printf("\U00002705 Service %s is set active\n", rnResult)
	} else {
		err = mm.sm.SetInactive(id)
		if err != nil {
			fmt.Printf("\U00002757 %s\n", err.Error())
			mm.lm.Log("error", err.Error(), "menu")
			return err
		}
		fmt.Printf("\U00002705 Service %s is set inactive\n", rnResult)
	}

	err = mm.listServices()

	return err
}

func (mm *MenuManager) removeService() error {
	// Get service Id
	rnResult, err := mm.inputPromptHelper("Service Id", "", mm.vm.IsInt64, nil)
	if err != nil {
		return err
	}

	// Remove service
	id, err := mm.tm.ToInt64(rnResult)
	if err != nil {
		msg := fmt.Sprintf("\U00002757 Service Id is not valid int64: %s", err.Error())
		fmt.Println(msg)
		mm.lm.Log("error", msg, "menu")
		return err
	}
	err = mm.sm.Remove(id)
	if err != nil {
		msg := fmt.Sprintf("\U00002757 %s\n", err.Error())
		fmt.Println(msg)
		mm.lm.Log("error", msg, "menu")
		return err
	}
	fmt.Printf("\U00002705 Service %s is removed\n", rnResult)

	err = mm.listServices()

	return err
}

func (mm *MenuManager) printServices(sm *ServiceManager, params ...uint32) error {
	var offset uint32 = 0
	var limit uint32 = 10

	l := mm.p2pm.cm.GetConfigWithDefault("search_results", "10")
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
	table.Header([]string{"ID", "Name", "Type", "Active"})
	for _, service := range services {
		row := []string{fmt.Sprintf("%d", service.Id), textManager.Shorten(service.Name, 17, 0), service.Type, fmt.Sprintf("%t", service.Active)}
		table.Append(row)
	}
	table.Render() // Prints the table

	if len(services) >= int(limit) {
		// Print "load more" prompt
		lmResult, err := mm.confirmPromptHelper("Load more")
		if err != nil {
			return err
		}
		if lmResult {
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

	_, rResult, err := mm.selectPromptHelper(
		"Main \U000025B6 Configure node \U000025B6 Services \U000025B6 Add Service \U000025B6 Resource",
		rItems,
		0, 10, nil)
	if err != nil {
		return err
	}
	r := rItemsMap[rResult]

	// Get resource price
	rpResult, err := mm.inputPromptHelper("Service resource price", "1.00", mm.vm.IsFloat64, nil)
	if err != nil {
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
	_, rscResult, err := mm.selectPromptHelper(
		"Main \U000025B6 Configure node \U000025B6 Services \U000025B6 Add Service \U000025B6 Currency",
		cItems,
		0, 10, nil)
	if err != nil {
		return err
	}

	// Add price
	mm.pm.Add(id, r, rp, rscResult)

	// Add more resource prices prompt
	srmResult, err := mm.confirmPromptHelper("Add more service resource prices")
	if err != nil {
		return err
	}
	if srmResult {
		// Add another service price
		return mm.addServicePrice(id)
	}

	return nil
}

func (mm *MenuManager) printServicePrice(id int64) error {
	// Print free or paid service prompt
	sfcResult, err := mm.confirmPromptHelper("Is this service free of charge")
	if err != nil {
		return err
	}
	if !sfcResult {
		// Add service price
		err = mm.addServicePrice(id)
		if err != nil {
			fmt.Println(err.Error())
			return err
		}
	}
	return nil
}
