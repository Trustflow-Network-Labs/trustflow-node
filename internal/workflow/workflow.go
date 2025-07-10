package workflow

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/adgsm/trustflow-node/internal/node_types"
	"github.com/adgsm/trustflow-node/internal/utils"
)

type WorkflowManager struct {
	db *sql.DB
	lm *utils.LogsManager
}

func NewWorkflowManager(db *sql.DB) *WorkflowManager {
	return &WorkflowManager{
		db: db,
		lm: utils.NewLogsManager(),
	}
}

// Workflow user entry validation?
func (wm *WorkflowManager) IsRunnableWorkflow(sid string) error {
	if sid == "" {
		err := fmt.Errorf("invalid workflow")
		wm.lm.Log("error", err.Error(), "workflows")
		return err
	}

	id, err := strconv.ParseInt(sid, 10, 64)
	if err != nil {
		wm.lm.Log("error", err.Error(), "workflows")
		return err
	}

	// Check if workflow already existing
	sql := fmt.Sprintf(`select w.id from workflows w
		inner join workflow_jobs wj
		on wj.workflow_id = w.id
		where w.id = %d and status = 'IDLE';`, id)
	rows, err := wm.db.QueryContext(context.Background(), sql)
	if err != nil {
		wm.lm.Log("error", err.Error(), "workflow")
		return err
	}
	defer rows.Close()

	var counter uint32 = 0
	for rows.Next() {
		counter++
	}

	if counter == 0 {
		err := fmt.Errorf("no IDLE jobs found in workflow id %d", id)
		wm.lm.Log("error", err.Error(), "workflows")
		return err
	}

	return nil
}

// Workflow exists?
func (wm *WorkflowManager) Exists(id int64) (error, bool) {
	if id <= 0 {
		msg := "invalid workflow"
		wm.lm.Log("error", msg, "workflows")
		return errors.New(msg), false
	}

	// Check if workflow already existing
	var iddb node_types.NullInt64
	row := wm.db.QueryRowContext(context.Background(), "select id from workflows where id = ?;", id)

	err := row.Scan(&iddb)
	if err != nil {
		msg := err.Error()
		wm.lm.Log("error", msg, "workflows")
		return err, false
	}

	return nil, true
}

// Get workflow
func (wm *WorkflowManager) Get(id int64) (node_types.Workflow, error) {
	var workflow node_types.Workflow
	if err, exists := wm.Exists(id); err != nil || !exists {
		err = fmt.Errorf("workflow %d does not exist", id)
		wm.lm.Log("debug", err.Error(), "workflows")
		return workflow, err
	}

	// Search for a workflow
	row := wm.db.QueryRowContext(context.Background(), "select id, name, description from workflows where id = ?;", id)

	err := row.Scan(&workflow.Id, &workflow.Name, &workflow.Description)
	if err != nil {
		msg := err.Error()
		wm.lm.Log("error", msg, "workflows")
		return workflow, err
	}

	// Load workflow jobs
	rows, err := wm.db.QueryContext(context.Background(), "select id, workflow_id, node_id, service_id, service_name, service_description, service_type, entrypoint, commands, last_seen, job_id, status from workflow_jobs where workflow_id = ?;", id)
	if err != nil {
		wm.lm.Log("error", err.Error(), "workflow")
		return workflow, err
	}
	defer rows.Close()

	var workflowJobs []node_types.WorkflowJob
	for rows.Next() {
		var workflowJobSql node_types.WorkflowJobSql
		if err := rows.Scan(&workflowJobSql.WorkflowJobBase.Id, &workflowJobSql.WorkflowJobBase.WorkflowId, &workflowJobSql.WorkflowJobBase.NodeId,
			&workflowJobSql.WorkflowJobBase.ServiceId, &workflowJobSql.WorkflowJobBase.ServiceName, &workflowJobSql.WorkflowJobBase.ServiceDescription,
			&workflowJobSql.WorkflowJobBase.ServiceType, &workflowJobSql.Entrypoint, &workflowJobSql.Commands,
			&workflowJobSql.LastSeen, &workflowJobSql.WorkflowJobBase.JobId, &workflowJobSql.WorkflowJobBase.Status); err == nil {

			workflowJob := workflowJobSql.ToWorkflowJob()
			workflowJobs = append(workflowJobs, workflowJob)
		}
	}

	// Load workflow jobs interfaces
	for i, workflowJob := range workflowJobs {
		rows, err := wm.db.QueryContext(context.Background(), "select id, workflow_job_id, interface_type, path from workflow_job_interfaces where workflow_job_id = ?;", workflowJob.WorkflowJobBase.Id)
		if err != nil {
			wm.lm.Log("error", err.Error(), "workflow")
			return workflow, err
		}
		defer rows.Close()

		var serviceInterfaces []node_types.ServiceInterface
		for rows.Next() {
			var serviceInterface node_types.ServiceInterface
			if err := rows.Scan(&serviceInterface.InterfaceId, &serviceInterface.WorkflowJobId,
				&serviceInterface.InterfaceType, &serviceInterface.Path); err == nil {
				serviceInterface.ServiceId = workflowJob.WorkflowJobBase.ServiceId
				serviceInterfaces = append(serviceInterfaces, serviceInterface)
			}
		}

		workflowJobs[i].WorkflowJobBase.ServiceInterfaces = serviceInterfaces
	}

	// Load workflow jobs price models
	for i, workflowJob := range workflowJobs {
		rows, err := wm.db.QueryContext(context.Background(), "select resource_group, resource, resource_unit, description, price, currency_name, currency_symbol from workflow_job_pricing_model where workflow_job_id = ?;", workflowJob.WorkflowJobBase.Id)
		if err != nil {
			wm.lm.Log("error", err.Error(), "workflow")
			return workflow, err
		}
		defer rows.Close()

		var priceModels []node_types.ServiceResourcesWithPricing
		for rows.Next() {
			var priceModel node_types.ServiceResourcesWithPricing
			if err := rows.Scan(&priceModel.ResourceGroup, &priceModel.ResourceName,
				&priceModel.ResourceUnit, &priceModel.ResourceDescription,
				&priceModel.Price, &priceModel.CurrencyName, &priceModel.CurrencySymbol); err == nil {
				priceModels = append(priceModels, priceModel)
			}
		}

		workflowJobs[i].WorkflowJobBase.ServicePriceModel = priceModels
	}

	workflow.Jobs = workflowJobs

	return workflow, nil
}

// List workflows
func (wm *WorkflowManager) List(params ...uint32) ([]node_types.Workflow, error) {
	// Read configs
	configManager := utils.NewConfigManager("")
	config, err := configManager.ReadConfigs()
	if err != nil {
		msg := fmt.Sprintf("Can not read configs file. (%s)", err.Error())
		wm.lm.Log("warn", msg, "workflows")
	}

	var offset uint32 = 0
	var limit uint32 = 10
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

	// Load workflows
	sql := fmt.Sprintf(`select distinct w.id
		from workflows w
		left join workflow_jobs wj
		on wj.workflow_id = w.id
		order by case wj.status
			when 'IDLE' then 1
			when 'RUNNING' then 2
			when 'CANCELLED' then 3
			when 'ERRORED' then 4
			when 'COMPLETED' then 5
		end
		limit %d offset %d;`, limit, offset)
	rows, err := wm.db.QueryContext(context.Background(), sql)
	if err != nil {
		wm.lm.Log("error", err.Error(), "workflow")
		return nil, err
	}

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()

	var workflows []node_types.Workflow
	for _, id := range ids {
		workflow, err := wm.Get(id)
		if err != nil {
			wm.lm.Log("error", err.Error(), "workflow")
			return nil, err
		}

		workflows = append(workflows, workflow)
	}

	return workflows, nil
}

// Add a workflow
func (wm *WorkflowManager) Add(name string, description string, workflowJobs []node_types.WorkflowJob) (int64, []int64, error) {
	wm.lm.Log("debug", fmt.Sprintf("adding workflow %s", name), "workflows")

	var wjids []int64

	result, err := wm.db.ExecContext(context.Background(), "insert into workflows (name, description) values (?, ?);",
		name, description)
	if err != nil {
		wm.lm.Log("error", err.Error(), "workflows")
		return 0, nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		wm.lm.Log("error", err.Error(), "workflows")
		return 0, nil, err
	}

	if len(workflowJobs) > 0 {
		wjids, err = wm.AddWorkflowJobs(id, workflowJobs)
		if err != nil {
			wm.lm.Log("error", err.Error(), "workflows")
			return 0, nil, err
		}
	}

	return id, wjids, nil
}

// Update a workflow
func (wm *WorkflowManager) Update(workflowId int64, name string, description string) error {
	wm.lm.Log("debug", fmt.Sprintf("updating workflow %s", name), "workflows")

	// Get workflow
	_, err := wm.Get(workflowId)
	if err != nil {
		wm.lm.Log("error", err.Error(), "workflows")
		return err
	}

	_, err = wm.db.ExecContext(context.Background(), "update workflows set name = ?, description = ? where id = ?;",
		name, description, workflowId)
	if err != nil {
		wm.lm.Log("error", err.Error(), "workflows")
		return err
	}

	return nil
}

// Remove a workflow
func (wm *WorkflowManager) Remove(workflowId int64) error {
	// Get workflow
	workflow, err := wm.Get(workflowId)
	if err != nil {
		wm.lm.Log("debug", err.Error(), "workflows")
		return err
	}

	for _, workflowJob := range workflow.Jobs {
		if workflowJob.WorkflowJobBase.Status != "IDLE" {
			err = fmt.Errorf("can not remove workflow id %d because job id %d is in status %s. Only jobs in status 'IDLE' can be deleted",
				workflowId, workflowJob.WorkflowJobBase.Id, workflowJob.WorkflowJobBase.Status)
			wm.lm.Log("error", err.Error(), "workflows")
			return err
		}
	}

	// Remove workflow
	wm.lm.Log("debug", fmt.Sprintf("removing workflow %d", workflowId), "workflows")

	_, err = wm.db.ExecContext(context.Background(), "delete from workflows where id = ?;", workflowId)
	if err != nil {
		wm.lm.Log("error", err.Error(), "workflows")
		return err
	}

	return nil
}

// Add a workflow job
func (wm *WorkflowManager) AddWorkflowJobs(workflowId int64, workflowJobs []node_types.WorkflowJob) ([]int64, error) {
	// Check if workflow exists
	if err, exists := wm.Exists(workflowId); err != nil || !exists {
		err = fmt.Errorf("workflow %d does not exist", workflowId)
		wm.lm.Log("error", err.Error(), "workflows")
		return nil, err
	}

	var ids []int64
	// Add workflow jobs
	for i, workflowJob := range workflowJobs {
		wm.lm.Log("debug", fmt.Sprintf("add workflow job %s-%d (service id: %d) to workflow id %d",
			workflowJob.WorkflowJobBase.NodeId, workflowJob.WorkflowJobBase.JobId, workflowJob.WorkflowJobBase.ServiceId, workflowId), "workflows")

		worflowJobSql := workflowJob.ToWorkflowJobSql()

		result, err := wm.db.ExecContext(context.Background(), "insert into workflow_jobs (workflow_id, node_id, service_id, service_name, service_description, service_type, entrypoint, commands, last_seen, job_id, expected_job_outputs) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?);",
			workflowId, worflowJobSql.WorkflowJobBase.NodeId, worflowJobSql.WorkflowJobBase.ServiceId, worflowJobSql.WorkflowJobBase.ServiceName,
			worflowJobSql.WorkflowJobBase.ServiceDescription, worflowJobSql.WorkflowJobBase.ServiceType,
			worflowJobSql.Entrypoint, worflowJobSql.Commands, worflowJobSql.LastSeen,
			worflowJobSql.WorkflowJobBase.JobId, worflowJobSql.WorkflowJobBase.ExpectedJobOutputs)
		if err != nil {
			wm.lm.Log("error", err.Error(), "workflows")
			return nil, err
		}

		id, err := result.LastInsertId()
		if err != nil {
			wm.lm.Log("error", err.Error(), "workflows")
			return nil, err
		}
		ids = append(ids, id)

		workflowJobs[i].WorkflowJobBase.Id = id
	}

	// Add workflow jobs interfaces
	for _, workflowJob := range workflowJobs {
		for _, serviceInterface := range workflowJob.WorkflowJobBase.ServiceInterfaces {
			wm.lm.Log("debug", fmt.Sprintf("add workflow job %s interface for workflow job id %d, workflow id %d",
				serviceInterface.InterfaceType, workflowJob.WorkflowJobBase.Id, workflowId), "workflows")

			_, err := wm.db.ExecContext(context.Background(), "insert into workflow_job_interfaces (workflow_job_id, interface_type, path) values (?, ?, ?);",
				workflowJob.WorkflowJobBase.Id, serviceInterface.InterfaceType, serviceInterface.Path)
			if err != nil {
				wm.lm.Log("error", err.Error(), "workflows")
				return nil, err
			}
		}
	}

	// Add workflow jobs pricing model
	for _, workflowJob := range workflowJobs {
		for _, priceModel := range workflowJob.WorkflowJobBase.ServicePriceModel {
			wm.lm.Log("debug", fmt.Sprintf("add workflow job price model %s (%2.f per %s) for workflow job id %d, workflow id %d",
				priceModel.ResourceName, priceModel.Price, priceModel.CurrencySymbol, workflowJob.WorkflowJobBase.Id, workflowId), "workflows")

			_, err := wm.db.ExecContext(context.Background(), "insert into workflow_job_pricing_model (workflow_job_id, resource_group, resource, resource_unit, description, price, currency_name, currency_symbol) values (?, ?, ?, ?, ?, ?, ?, ?);",
				workflowJob.WorkflowJobBase.Id, priceModel.ResourceGroup, priceModel.ResourceName, priceModel.ResourceUnit, priceModel.ResourceDescription, priceModel.Price, priceModel.CurrencyName, priceModel.CurrencySymbol)
			if err != nil {
				wm.lm.Log("error", err.Error(), "workflows")
				return nil, err
			}
		}
	}

	return ids, nil
}

// Get workflow job
func (wm *WorkflowManager) GetWorkflowJob(id int64) (node_types.WorkflowJob, error) {
	var workflowJob node_types.WorkflowJob
	var workflowJobSql node_types.WorkflowJobSql

	// Load a workflow job
	row := wm.db.QueryRowContext(context.Background(), "select id, workflow_id, node_id, service_id, service_name, service_description, service_type, entrypoint, commands, last_seen, job_id, status from workflow_jobs where id = ?;", id)

	err := row.Scan(&workflowJobSql.WorkflowJobBase.Id, &workflowJobSql.WorkflowJobBase.WorkflowId, &workflowJobSql.WorkflowJobBase.NodeId,
		&workflowJobSql.WorkflowJobBase.ServiceId, &workflowJobSql.WorkflowJobBase.ServiceName, &workflowJobSql.WorkflowJobBase.ServiceDescription,
		&workflowJobSql.WorkflowJobBase.ServiceType, &workflowJobSql.Entrypoint, &workflowJobSql.Commands,
		&workflowJobSql.LastSeen, &workflowJobSql.WorkflowJobBase.JobId, &workflowJobSql.WorkflowJobBase.Status)
	if err != nil {
		msg := err.Error()
		wm.lm.Log("error", msg, "workflows")
		return workflowJob, err
	}

	workflowJob = workflowJobSql.ToWorkflowJob()

	// Load a workflow job interfaces
	rows, err := wm.db.QueryContext(context.Background(), "select id, workflow_job_id, interface_type, path from workflow_job_interfaces where workflow_job_id = ?;",
		workflowJob.WorkflowJobBase.Id)
	if err != nil {
		wm.lm.Log("error", err.Error(), "workflow")
		return workflowJob, err
	}
	for rows.Next() {
		var serviceInterface node_types.ServiceInterface
		err := rows.Scan(&serviceInterface.InterfaceId, &serviceInterface.WorkflowJobId,
			&serviceInterface.InterfaceType, &serviceInterface.InterfaceType)
		if err != nil {
			wm.lm.Log("error", err.Error(), "workflow")
			return workflowJob, err
		}

		workflowJob.WorkflowJobBase.ServiceInterfaces = append(workflowJob.WorkflowJobBase.ServiceInterfaces, serviceInterface)
	}
	rows.Close()

	// Load a workflow job price model
	rows, err = wm.db.QueryContext(context.Background(), "select resource_group, resource, resource_unit, description, price, currency_name, currency_symbol from workflow_job_pricing_model where workflow_job_id = ?;",
		workflowJob.WorkflowJobBase.Id)
	if err != nil {
		wm.lm.Log("error", err.Error(), "workflow")
		return workflowJob, err
	}
	for rows.Next() {
		var priceModel node_types.ServiceResourcesWithPricing
		err := rows.Scan(&priceModel.ResourceGroup, &priceModel.ResourceName, &priceModel.ResourceUnit,
			&priceModel.ResourceDescription, &priceModel.Price, &priceModel.CurrencyName, &priceModel.CurrencySymbol)
		if err != nil {
			wm.lm.Log("error", err.Error(), "workflow")
			return workflowJob, err
		}

		workflowJob.WorkflowJobBase.ServicePriceModel = append(workflowJob.WorkflowJobBase.ServicePriceModel, priceModel)
	}
	rows.Close()

	return workflowJob, nil
}

// Remove a workflow job
func (wm *WorkflowManager) RemoveWorkflowJob(workflowJobId int64) error {
	// Get workflow job
	workflowJob, err := wm.GetWorkflowJob(workflowJobId)
	if err != nil {
		wm.lm.Log("error", err.Error(), "workflows")
		return err
	}

	if workflowJob.WorkflowJobBase.Status != "IDLE" {
		err = fmt.Errorf("can not remove workflow job id %d in status %s",
			workflowJobId, workflowJob.WorkflowJobBase.Status)
		wm.lm.Log("error", err.Error(), "workflows")
		return err
	}

	// Remove workflow job
	wm.lm.Log("debug", fmt.Sprintf("removing workflow job %d", workflowJobId), "workflows")

	_, err = wm.db.ExecContext(context.Background(), "delete from workflow_jobs where id = ?;", workflowJobId)
	if err != nil {
		wm.lm.Log("error", err.Error(), "workflows")
		return err
	}

	return nil
}

// Update status of a requested service / job to accepted
func (wm *WorkflowManager) RegisteredWorkflowJob(workflowId int64, workflowJobId int64, nodeId string, serviceId int64, jobId int64, expectedJobOutputs string) error {
	// Check if workflow exists
	if err, exists := wm.Exists(workflowId); err != nil || !exists {
		err = fmt.Errorf("workflow %d does not exist", workflowId)
		wm.lm.Log("error", err.Error(), "workflows")
		return err
	}

	// Accepted workflow job
	wm.lm.Log("debug", fmt.Sprintf("accepted workflow job %s-%d (service id: %d) for workflow id %d, workflow job id %d", nodeId, jobId, serviceId, workflowId, workflowJobId), "workflows")

	_, err := wm.db.ExecContext(context.Background(), "update workflow_jobs set job_id = ?, expected_job_outputs = ? where id = ?;",
		jobId, expectedJobOutputs, workflowJobId)
	if err != nil {
		wm.lm.Log("error", err.Error(), "workflows")
		return err
	}

	return nil
}

// Update workflow job status
func (wm *WorkflowManager) UpdateWorkflowJobStatus(workflowId int64, nodeId string, jobId int64, status string) error {
	// Check if workflow exists
	if err, exists := wm.Exists(workflowId); err != nil || !exists {
		err = fmt.Errorf("workflow %d does not exist", workflowId)
		wm.lm.Log("error", err.Error(), "workflows")
		return err
	}

	// Check job status
	workflow, err := wm.Get(workflowId)
	if err != nil {
		wm.lm.Log("error", err.Error(), "workflows")
		return err
	}

	var exists bool = false
	for _, job := range workflow.Jobs {
		if job.WorkflowJobBase.JobId == jobId {
			exists = true
			break
		}
	}
	if !exists {
		err := fmt.Errorf("workflow job %d-%s-%d does not exist", workflowId, nodeId, jobId)
		wm.lm.Log("error", err.Error(), "workflows")
		return err
	}

	// Update workflow job status
	wm.lm.Log("debug", fmt.Sprintf("set workflow job %d-%s-%d to %s", workflowId, nodeId, jobId, status), "workflows")

	_, err = wm.db.ExecContext(context.Background(), "update workflow_jobs set status = ? where workflow_id = ? and node_id = ? and job_id = ?;",
		status, workflowId, nodeId, jobId)
	if err != nil {
		wm.lm.Log("error", err.Error(), "workflows")
		return err
	}

	return nil
}

// Check if there are workflow jobs containing expected output
func (wm *WorkflowManager) ExpectedOutputFound(nodeId string, output string) (bool, error) {
	// Load workflow jobs matching the expected output
	sql := `select distinct w.id
		from workflows w
		inner join workflow_jobs wj
			on wj.workflow_id = w.id
		where wj.node_id = ?
		and ',' || wj.expected_job_outputs || ',' like ?`

	pattern := fmt.Sprintf("%%,%s,%%", output)
	rows, err := wm.db.QueryContext(context.Background(), sql, nodeId, pattern)
	if err != nil {
		wm.lm.Log("error", err.Error(), "workflow")
		return false, err
	}

	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err == nil {
			ids = append(ids, id)
		}
	}
	rows.Close()

	if len(ids) > 0 {
		return true, nil
	}

	return false, nil
}
