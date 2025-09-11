package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"sync"
	"syscall"
	"time"

	"github.com/adgsm/trustflow-node/internal/dependencies"
	"github.com/adgsm/trustflow-node/internal/node"
	"github.com/adgsm/trustflow-node/internal/node_types"
	"github.com/adgsm/trustflow-node/internal/ui"
	"github.com/adgsm/trustflow-node/internal/utils"
	"github.com/adgsm/trustflow-node/internal/workflow"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx               context.Context
	p2pm              node.P2PManager
	dm                dependencies.DependencyManager
	sm                node.ServiceManager
	wm                workflow.WorkflowManager
	ntm               *utils.NodeTypeManager
	confirmFuncChan   chan bool
	confirmChanMutex  sync.Mutex // Protect confirmFuncChan access
	frontendReadyChan chan struct{}
	channelManager    *utils.ChannelManager // Centralized channel management
	gui               ui.UI
	stopChan          chan struct{} // Channel to signal node stop from StopNode()
	stopChanMutex     sync.Mutex    // Protect stopChan access
	cleanupCtx        context.Context       // Context for cleanup goroutine
	cleanupCancel     context.CancelFunc    // Cancel function for cleanup
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	// Set runtime limits to prevent goroutine leaks
	runtime.GOMAXPROCS(runtime.NumCPU()) // Use all available CPUs
	debug.SetMaxThreads(2000)            // Limit OS threads to prevent resource exhaustion
	
	a.ctx = ctx
	a.channelManager = utils.NewChannelManager(ctx)
	a.frontendReadyChan = make(chan struct{})
	
	// Create separate cancellable context for cleanup goroutine
	a.cleanupCtx, a.cleanupCancel = context.WithCancel(ctx)
	
	// Start periodic cleanup goroutine to manage memory and goroutines
	go a.startPeriodicCleanup(a.cleanupCtx)
	a.gui = ui.GUI{
		PrintFunc: func(msg string) {
			wailsruntime.EventsEmit(a.ctx, "syslog-event", msg)
		},
		ConfirmFunc: func(question string) bool {
			a.confirmChanMutex.Lock()

			// Close existing channel if it exists
			if a.confirmFuncChan != nil {
				close(a.confirmFuncChan)
			}

			// Create new channel
			a.confirmFuncChan = make(chan bool)
			currentChan := a.confirmFuncChan // Keep reference for this call
			a.confirmChanMutex.Unlock()

			// Send the prompt to frontend
			wailsruntime.EventsEmit(a.ctx, "sysconfirm-event", question)

			// Wait for frontend response (blocks until received)
			select {
			case response := <-currentChan:
				return response
			case <-a.ctx.Done():
				return false // Default to false if context is cancelled
			}
		},
		ExitFunc: func(code int) {
			var msg = ""
			switch code {
			case 1:
				msg = "Cannot continue until dependencies are installed."
			default:
				msg = fmt.Sprintf("Unknown application exit code `%d`", code)
			}
			wailsruntime.EventsEmit(a.ctx, "exitlog-event", msg)
		},
		ServiceOfferFunc: func(serviceOffer node_types.ServiceOffer) {
			wailsruntime.EventsEmit(a.ctx, "serviceofferlog-event", serviceOffer)
		},
	}

	// Configs manager
	cm := utils.NewConfigManager("")

	// P2P manager
	a.p2pm = *node.NewP2PManager(ctx, a.gui, cm)
	a.dm = *dependencies.NewDependencyManager(a.gui, cm)
	a.sm = *node.NewServiceManager(&a.p2pm)
	a.wm = *workflow.NewWorkflowManager(a.p2pm.DB, a.p2pm.Lm, cm)
	select {
	case <-a.frontendReadyChan:
		a.CheckAndInstallDependencies()
		wailsruntime.EventsEmit(a.ctx, "dependenciesready-event", true)
	case <-time.After(10 * time.Second): // Optional timeout
		fmt.Println("Timeout waiting for frontend readiness")
	}
}

func (a *App) shutdown(ctx context.Context) {
	// Stop node before closing
	if a.IsHostRunning() {
		a.StopNode()
	}

	// Clean up channels
	a.confirmChanMutex.Lock()
	if a.confirmFuncChan != nil {
		close(a.confirmFuncChan)
		a.confirmFuncChan = nil
	}
	a.confirmChanMutex.Unlock()

	// Shutdown channel manager
	if a.channelManager != nil {
		a.channelManager.Shutdown()
	}

	defer a.p2pm.Close()
}

// Signal that frontend is ready
func (a *App) NotifyFrontendReady() {
	if a.frontendReadyChan != nil {
		close(a.frontendReadyChan)
		a.frontendReadyChan = nil // Avoid multiple closes
	}
}

// Check and install node dependencies
func (a *App) CheckAndInstallDependencies() {
	a.dm.CheckAndInstallDependencies()
}

// User confirm with the response
func (a *App) SetUserConfirmation(response bool) {
	a.confirmChanMutex.Lock()
	defer a.confirmChanMutex.Unlock()

	if a.confirmFuncChan != nil {
		select {
		case a.confirmFuncChan <- response:
			// Successfully sent response
		case <-a.ctx.Done():
			// Context cancelled, don't block
		default:
			// Channel might be closed or no receiver, don't block
		}
	}
}

// Is P2P host running
func (a *App) IsHostRunning() bool {
	return a.p2pm.IsHostRunning()
}

// Get node type (public or private)
func (a *App) IsPublicNode() bool {
	// Determine node type (if node has public IP or not)
	a.ntm = utils.NewNodeTypeManager()
	nodeType, err := a.ntm.GetNodeTypeConfig([]uint16{})
	if err != nil {
		return false
	} else {
		return nodeType.Type == "public"
	}
}

// Start P2P node
func (a *App) StartNode(port uint16, relay bool) {
	var public bool = false

	// Determine node type (if node has public IP or not)
	a.ntm = utils.NewNodeTypeManager()
	nodeType, err := a.ntm.GetNodeTypeConfig([]uint16{port})
	if err != nil {
		msg := fmt.Sprintf("⚠️ Can not determine node type: %v", err)
		wailsruntime.EventsEmit(a.ctx, "syslog-event", msg)
		return
	} else {
		msg := fmt.Sprintf("Node type: %s", nodeType.Type)
		wailsruntime.EventsEmit(a.ctx, "syslog-event", msg)
		msg = fmt.Sprintf("Local IP: %s", nodeType.LocalIP)
		wailsruntime.EventsEmit(a.ctx, "syslog-event", msg)
		msg = fmt.Sprintf("External IP: %s", nodeType.ExternalIP)
		wailsruntime.EventsEmit(a.ctx, "syslog-event", msg)
		for port, open := range nodeType.Connectivity {
			if !open {
				err = fmt.Errorf("❌ Port %d is not open", port)
				wailsruntime.EventsEmit(a.ctx, "syslog-event", err.Error())
				return
			} else {
				msg = fmt.Sprintf("✅ Port %d is open", port)
				wailsruntime.EventsEmit(a.ctx, "syslog-event", msg)
			}
		}

		public = nodeType.Type == "public"
	}

	if !public && relay {
		msg := "Private node behind NAT should not be used as a relay."
		wailsruntime.EventsEmit(a.ctx, "syslog-event", msg)
		relay = false
	}

	// Initialize stop channel for this start session
	a.stopChanMutex.Lock()
	a.stopChan = make(chan struct{})
	currentStopChan := a.stopChan // Keep reference for this session
	a.stopChanMutex.Unlock()

	// Start p2p node
	err = a.p2pm.Start(a.ctx, port, false, public, relay)
	if err != nil {
		msg := fmt.Sprintf("⚠️ Can not start p2p node:\n%v\n", err)
		wailsruntime.EventsEmit(a.ctx, "syslog-event", msg)
	}

	// Wait for node to become ready
	// Try every 500ms, up to 10 times (i.e. 5 seconds total)
	running := a.p2pm.WaitForHostReady(500*time.Millisecond, 10)
	if !running {
		err := fmt.Errorf("⚠️ Host is not running")
		a.p2pm.Lm.Log("error", err.Error(), "p2p")
		wailsruntime.EventsEmit(a.ctx, "syslog-event", err.Error())
		wailsruntime.EventsEmit(a.ctx, "hostrunninglog-event", false)
		return
	}
	wailsruntime.EventsEmit(a.ctx, "hostrunninglog-event", true)

	// Add signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-a.ctx.Done():
		a.p2pm.Lm.Log("debug", "Context cancelled", "p2p")
	case <-sigChan:
		a.p2pm.Lm.Log("debug", "Received shutdown signal", "p2p")
	case <-currentStopChan:
		a.p2pm.Lm.Log("debug", "Received stop request from StopNode", "p2p")
	}
}

// Stop P2P node
func (a *App) StopNode() error {
	// Trigger stop channel to unblock StartNode's select statement
	a.stopChanMutex.Lock()
	if a.stopChan != nil {
		select {
		case a.stopChan <- struct{}{}:
			// Successfully sent stop signal
		default:
			// Channel might be closed or no receiver, don't block
		}
	}
	a.stopChanMutex.Unlock()

	// Stop p2p manager with timeout to prevent hanging
	fmt.Printf("Stopping P2P manager...\n")
	stopDone := make(chan error, 1)
	go func() {
		stopDone <- a.p2pm.Stop()
	}()
	
	select {
	case err := <-stopDone:
		if err != nil {
			fmt.Printf("⚠️ P2P manager stop error: %v\n", err)
			return err
		}
		fmt.Printf("P2P manager stopped successfully\n")
	case <-time.After(15 * time.Second):
		fmt.Printf("⚠️ P2P manager stop timeout - force exiting\n")
		// Continue with cleanup even if stop times out
	}
	
	// Give pprof server and other HTTP connections time to close
	time.Sleep(500 * time.Millisecond)
	
	// Cancel cleanup context after p2p manager stops
	if a.cleanupCancel != nil {
		a.cleanupCancel()
	}
	return nil
}

// Host peer Id
func (a *App) PeerId() string {
	return a.p2pm.PeerId.String()
}

// Find services
func (a *App) FindServices(searchPhrases string, serviceType string) error {
	return a.sm.LookupRemoteService(searchPhrases, serviceType)
}

// Find remote node/peer services
func (a *App) FindPeerServices(searchPhrases string, serviceType string, peerId string) error {
	return a.p2pm.RequestServiceCatalogue(searchPhrases, serviceType, peerId)
}

// Search local node/peer services
func (a *App) SearchServices(searchPhrases string, serviceType string, offset uint32, limit uint32) error {
	var serviceServices node_types.SearchService = node_types.SearchService{
		Phrases: searchPhrases,
		Type:    serviceType,
		Active:  true,
	}

	services, err := a.sm.SearchServices(serviceServices, offset, limit)
	if err != nil {
		return err
	}

	for _, service := range services {
		wailsruntime.EventsEmit(a.ctx, "serviceofferlog-event", service)
	}

	return nil
}

// Get workflow grid props
// TODO, get/set grid size, service box size, etc
type WorkflowGUIProps struct {
	SnapToGrid int64  `json:"snap_to_grid"`
	Error      string `json:"error"`
}

func (a *App) GetWorkflowGUIProps(workflowId int64) WorkflowGUIProps {
	var response = WorkflowGUIProps{}

	// Look for workflow gui params
	row := a.p2pm.DB.QueryRowContext(context.Background(), "select snap_to_grid from workflows_gui where workflow_id = ?;", workflowId)

	err := row.Scan(&response.SnapToGrid)
	if err != nil {
		response.Error = err.Error()
	}

	return response
}

// Set workflow gui props
func (a *App) SetWorkflowGUIProps(workflowId int64, snapToGrid int64) error {
	var err error = nil

	// Check do we have properties set already
	workflowGUIProps := a.GetWorkflowGUIProps(workflowId)
	if workflowGUIProps.Error != "" {
		// We don't have props set yet
		_, err = a.p2pm.DB.ExecContext(context.Background(), "insert into workflows_gui (workflow_id, snap_to_grid) values (?, ?);",
			workflowId, snapToGrid)
		if err != nil {
			return err
		}
	} else {
		// We have props set before
		_, err = a.p2pm.DB.ExecContext(context.Background(), "update workflows_gui set snap_to_grid = ? where workflow_id = ?;",
			snapToGrid, workflowId)
		if err != nil {
			return err
		}
	}

	return err
}

// Get service card props
type ServiceCardGUIProps struct {
	X     int64  `json:"x"`
	Y     int64  `json:"y"`
	Error string `json:"error"`
}

func (a *App) GetServiceCardGUIProps(workflowJobId int64) ServiceCardGUIProps {
	var response = ServiceCardGUIProps{}

	// Look for workflow job gui params
	row := a.p2pm.DB.QueryRowContext(context.Background(), "select x, y from workflow_jobs_gui where workflow_job_id = ?;", workflowJobId)

	err := row.Scan(&response.X, &response.Y)
	if err != nil {
		response.Error = err.Error()
	}

	return response
}

// Set service card props
func (a *App) SetServiceCardGUIProps(workflowJobId int64, x int64, y int64) error {
	var err error = nil

	// Check do we have properties set already
	serviceCardGUIProps := a.GetServiceCardGUIProps(workflowJobId)
	if serviceCardGUIProps.Error != "" {
		// We don't have props set yet
		_, err = a.p2pm.DB.ExecContext(context.Background(), "insert into workflow_jobs_gui (workflow_job_id, x, y) values (?, ?, ?);",
			workflowJobId, x, y)
		if err != nil {
			return err
		}
	} else {
		// We have props set before
		_, err = a.p2pm.DB.ExecContext(context.Background(), "update workflow_jobs_gui set x = ?, y = ? where workflow_job_id = ?;",
			x, y, workflowJobId)
		if err != nil {
			return err
		}
	}

	return err
}

// List workflows
type ListWorkflowsResponse struct {
	Workflows []node_types.Workflow `json:"workflows"`
	Error     string                `json:"error"`
}

func (a *App) ListWorkflows(offset, limit uint32) ListWorkflowsResponse {
	var response ListWorkflowsResponse

	var params []uint32 = []uint32{
		offset,
		limit,
	}

	workflows, err := a.wm.List(params...)
	if err != nil {
		response.Error = err.Error()
		return response
	}

	response.Workflows = workflows

	return response
}

// Add workflow
type AddWorkflowResponse struct {
	WorkflowId int64 `json:"workflow_id"`
	AddWorkflowJobResponse
}

type ServiceInterface struct {
	Description   string `json:"description"`
	InterfaceType string `json:"interface_type"`
	Path          string `json:"path"`
}

func (a *App) AddWorkflow(
	name, description, nodeId string,
	serviceId int64,
	serviceName, serviceDescription, serviceType string,
	entrypoint, commands []string,
	serviceInterfaces []ServiceInterface,
	servicePriceModel []node_types.ServiceResourcesWithPricing,
	lastSeen string, jobId int64, expectedJobOutputs string,
) AddWorkflowResponse {
	const timeLayout = time.RFC3339
	var workflowJobs []node_types.WorkflowJob
	var response AddWorkflowResponse

	if nodeId != "" && serviceId > 0 {
		var servIntfaces []node_types.ServiceInterface
		for _, serviceInterface := range serviceInterfaces {
			servIntface := node_types.ServiceInterface{
				Interface: node_types.Interface{
					InterfaceType: serviceInterface.InterfaceType,
					Description:   serviceInterface.Description,
					Path:          serviceInterface.Path,
				},
			}
			servIntfaces = append(servIntfaces, servIntface)
		}

		workflowJobBase := node_types.WorkflowJobBase{
			NodeId:             nodeId,
			ServiceId:          serviceId,
			ServiceName:        serviceName,
			ServiceDescription: serviceDescription,
			ServiceType:        serviceType,
			JobId:              jobId,
			ExpectedJobOutputs: expectedJobOutputs,
			ServiceInterfaces:  servIntfaces,
			ServicePriceModel:  servicePriceModel,
		}

		workflowJob := node_types.WorkflowJob{
			WorkflowJobBase: workflowJobBase,
			Entrypoint:      entrypoint,
			Commands:        commands,
		}

		if lastSeen != "" {
			ls, err := time.Parse(timeLayout, lastSeen)
			if err != nil {
				response.Error = err.Error()
			} else {
				workflowJob.LastSeen = ls
			}
		}

		workflowJobs = append(workflowJobs, workflowJob)
	}

	workflowId, workflowJobsIds, err := a.wm.Add(name, description, workflowJobs)
	if err != nil {
		response.Error = err.Error()
	}

	response = AddWorkflowResponse{
		WorkflowId: workflowId,
		AddWorkflowJobResponse: AddWorkflowJobResponse{
			WorkflowJobsIds: workflowJobsIds,
		},
	}

	return response
}

// Update workflow
func (a *App) UpdateWorkflow(workflowId int64, name string, description string) error {
	return a.wm.Update(workflowId, name, description)
}

// Remove workflow
func (a *App) RemoveWorkflow(workflowId int64) error {
	return a.wm.Remove(workflowId)
}

// Get workflow job
type GetWorkflowJobResponse struct {
	WorkflowJob node_types.WorkflowJob `json:"workflow_job"`
	Error       string                 `json:"error"`
}

func (a *App) GetWorkflowJob(workflowJobId int64) GetWorkflowJobResponse {
	var response GetWorkflowJobResponse
	workflowJob, err := a.wm.GetWorkflowJob(workflowJobId)
	if err != nil {
		response.Error = err.Error()
	}

	response.WorkflowJob = workflowJob

	return response
}

// Add workflow job
type AddWorkflowJobResponse struct {
	WorkflowJobsIds []int64 `json:"workflow_jobs_ids"`
	Error           string  `json:"error"`
}

func (a *App) AddWorkflowJob(
	workflowId int64,
	nodeId string,
	serviceId int64,
	serviceName, serviceDescription, serviceType string,
	entrypoint, commands []string,
	serviceInterfaces []ServiceInterface,
	servicePriceModel []node_types.ServiceResourcesWithPricing,
	lastSeen string, jobId int64, expectedJobOutputs string,
) AddWorkflowJobResponse {
	const timeLayout = time.RFC3339
	var response AddWorkflowJobResponse

	var servIntfaces []node_types.ServiceInterface
	for _, serviceInterface := range serviceInterfaces {
		servIntface := node_types.ServiceInterface{
			Interface: node_types.Interface{
				InterfaceType: serviceInterface.InterfaceType,
				Description:   serviceInterface.Description,
				Path:          serviceInterface.Path,
			},
		}
		servIntfaces = append(servIntfaces, servIntface)
	}

	workflowJobBase := node_types.WorkflowJobBase{
		NodeId:             nodeId,
		ServiceId:          serviceId,
		ServiceName:        serviceName,
		ServiceDescription: serviceDescription,
		ServiceType:        serviceType,
		JobId:              jobId,
		ExpectedJobOutputs: expectedJobOutputs,
		ServiceInterfaces:  servIntfaces,
		ServicePriceModel:  servicePriceModel,
	}

	workflowJob := node_types.WorkflowJob{
		WorkflowJobBase: workflowJobBase,
		Entrypoint:      entrypoint,
		Commands:        commands,
	}

	if lastSeen != "" {
		ls, err := time.Parse(timeLayout, lastSeen)
		if err != nil {
			response.Error = err.Error()
		} else {
			workflowJob.LastSeen = ls
		}
	}

	workflowJobsIds, err := a.wm.AddWorkflowJobs(workflowId, []node_types.WorkflowJob{workflowJob})
	if err != nil {
		response.Error = err.Error()
	}

	response.WorkflowJobsIds = workflowJobsIds

	return response
}

// Remove workflow job
func (a *App) RemoveWorkflowJob(workflowJobId int64) error {
	return a.wm.RemoveWorkflowJob(workflowJobId)
}

// Add workflow job interface peer
type AddWorkflowJobInterfacePeerResponse struct {
	WorkflowJobInterfacePeerId int64  `json:"workflow_job_interface_peer_id"`
	Error                      string `json:"error"`
}

func (a *App) AddWorkflowJobInterfacePeer(workflowJobId int64, workflowJobInterfaceId int64, peerNodeId string, peerServiceId int64, peerMountFunction string, path string) AddWorkflowJobInterfacePeerResponse {
	var response AddWorkflowJobInterfacePeerResponse
	id, err := a.wm.AddWorkflowJobInterfacePeer(workflowJobId, workflowJobInterfaceId, peerNodeId, peerServiceId, peerMountFunction, path)
	if err != nil {
		response.Error = err.Error()
	}
	response.WorkflowJobInterfacePeerId = id

	return response
}

// Remove workflow job interface peer
func (a *App) RemoveWorkflowJobInterfacePeer(workflowJobId int64, workflowJobInterfacePeerId int64) error {
	return a.wm.RemoveWorkflowJobInterfacePeer(workflowJobId, workflowJobInterfacePeerId)
}

// startPeriodicCleanup runs periodic memory and goroutine cleanup to prevent leaks
func (a *App) startPeriodicCleanup(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute) // Run cleanup every 5 minutes
	defer ticker.Stop()
	
	var lastGoroutineCount int
	
	for {
		select {
		case <-ticker.C:
			// Force garbage collection to clean up unused memory
			runtime.GC()
			
			// Free OS memory back to the system
			debug.FreeOSMemory()
			
			// Log current goroutine count for monitoring
			numGoroutines := runtime.NumGoroutine()
			if numGoroutines > 500 { // Alert if goroutines are high
				fmt.Printf("[CLEANUP] High goroutine count detected: %d (GUI mode)\n", numGoroutines)
			}
			
			// Track goroutine growth over time
			if lastGoroutineCount > 0 && numGoroutines > lastGoroutineCount {
				growth := numGoroutines - lastGoroutineCount
				if growth > 50 { // Alert if growth is significant
					fmt.Printf("[CLEANUP] Goroutine growth detected: +%d (total: %d)\n", growth, numGoroutines)
				}
			}
			lastGoroutineCount = numGoroutines
			
		case <-ctx.Done():
			// Context cancelled, stop cleanup
			fmt.Printf("[CLEANUP] Periodic cleanup stopped\n")
			return
		}
	}
}
