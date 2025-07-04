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

// Remove workflow
func (a *App) RemoveWorkflow(workflowId int64) error {
	err := a.wm.Remove(workflowId)

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
