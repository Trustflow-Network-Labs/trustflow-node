package main

import (
	"context"
	"fmt"
	"time"

	"github.com/adgsm/trustflow-node/internal/dependencies"
	"github.com/adgsm/trustflow-node/internal/node"
	"github.com/adgsm/trustflow-node/internal/node_types"
	"github.com/adgsm/trustflow-node/internal/ui"
	"github.com/adgsm/trustflow-node/internal/workflow"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx               context.Context
	p2pm              node.P2PManager
	dm                dependencies.DependencyManager
	sm                node.ServiceManager
	wm                workflow.WorkflowManager
	confirmFuncChan   chan bool
	frontendReadyChan chan struct{}
	gui               ui.UI
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.frontendReadyChan = make(chan struct{})
	a.gui = ui.GUI{
		PrintFunc: func(msg string) {
			runtime.EventsEmit(a.ctx, "syslog-event", msg)
		},
		ConfirmFunc: func(question string) bool {
			a.confirmFuncChan = make(chan bool)

			// Send the prompt to frontend
			runtime.EventsEmit(a.ctx, "sysconfirm-event", question)

			// Wait for frontend response (blocks until received)
			response := <-a.confirmFuncChan

			return response
		},
		ExitFunc: func(code int) {
			var msg = ""
			switch code {
			case 1:
				msg = "Cannot continue until dependencies are installed."
			default:
				msg = fmt.Sprintf("Unknown application exit code `%d`", code)
			}
			runtime.EventsEmit(a.ctx, "exitlog-event", msg)
		},
		ServiceOfferFunc: func(serviceOffer node_types.ServiceOffer) {
			runtime.EventsEmit(a.ctx, "serviceofferlog-event", serviceOffer)
		},
	}

	p2pm := node.NewP2PManager(ctx, a.gui)
	a.p2pm = *p2pm
	a.dm = *dependencies.NewDependencyManager(a.gui)
	a.sm = *node.NewServiceManager(p2pm)
	a.wm = *workflow.NewWorkflowManager(p2pm.DB)
	select {
	case <-a.frontendReadyChan:
		a.CheckAndInstallDependencies()
		runtime.EventsEmit(a.ctx, "dependenciesready-event", true)
	case <-time.After(10 * time.Second): // Optional timeout
		fmt.Println("Timeout waiting for frontend readiness")
	}
}

func (a *App) shutdown(ctx context.Context) {
	// Stop node before closing
	if a.IsHostRunning() {
		a.StopNode()
	}
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
	if a.confirmFuncChan != nil {
		a.confirmFuncChan <- response
	}
}

// Is P2P host running
func (a *App) IsHostRunning() bool {
	return a.p2pm.IsHostRunning()
}

// Start P2P node
func (a *App) StartNode(port uint16) {
	a.p2pm.Start(port, true)
}

// Stop P2P node
func (a *App) StopNode() error {
	err := a.p2pm.Stop()
	if err != nil {
		return err
	}
	return nil
}

// Find services
func (a *App) FindServices(searchPhrases string, serviceTypes string) error {
	return a.sm.LookupRemoteService(searchPhrases, serviceTypes)
}

// Add workflow
type AddWorkflowResponse struct {
	WorkflowId int64 `json:"workflow_id"`
	AddWorkflowJobResponse
}

func (a *App) AddWorkflow(name string, description string, nodeId string, serviceId int64, jobId int64, expectedJobOutputs string) AddWorkflowResponse {
	var workflowJobBases []node_types.WorkflowJobBase
	if nodeId != "" && serviceId > 0 {
		workflowJobBase := node_types.WorkflowJobBase{
			NodeId:             nodeId,
			ServiceId:          serviceId,
			JobId:              jobId,
			ExpectedJobOutputs: expectedJobOutputs,
		}
		workflowJobBases = append(workflowJobBases, workflowJobBase)
	}

	workflowId, workflowJobsIds, err := a.wm.Add(name, description, workflowJobBases)
	response := AddWorkflowResponse{
		WorkflowId: workflowId,
		AddWorkflowJobResponse: AddWorkflowJobResponse{
			WorkflowJobsIds: workflowJobsIds,
		},
	}

	if err != nil {
		response.Error = err.Error()
	}

	return response
}

// Update workflow
func (a *App) UpdateWorkflow(workflowId int64, name string, description string) error {
	err := a.wm.Update(workflowId, name, description)

	return err
}

// Add workflow job
type AddWorkflowJobResponse struct {
	WorkflowJobsIds []int64 `json:"workflow_jobs_ids"`
	Error           string  `json:"error"`
}

func (a *App) AddWorkflowJob(workflowId int64, nodeId string, serviceId int64, jobId int64, expectedJobOutputs string) AddWorkflowJobResponse {
	workflowJobBase := node_types.WorkflowJobBase{
		NodeId:             nodeId,
		ServiceId:          serviceId,
		JobId:              jobId,
		ExpectedJobOutputs: expectedJobOutputs,
	}

	workflowJobsIds, err := a.wm.AddWorkflowJobs(workflowId, []node_types.WorkflowJobBase{workflowJobBase})
	response := AddWorkflowJobResponse{
		WorkflowJobsIds: workflowJobsIds,
	}

	if err != nil {
		response.Error = err.Error()
	}

	return response
}

// Remove workflow job
func (a *App) RemoveWorkflowJob(workflowJobId int64) error {
	err := a.wm.RemoveWorkflowJob(workflowJobId)

	return err
}
