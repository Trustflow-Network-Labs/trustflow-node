package workflow

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"

	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/utils"
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
		wm.lm.Log("debug", err.Error(), "workflows")
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
		wm.lm.Log("debug", msg, "workflows")
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
		wm.lm.Log("debug", msg, "workflows")
		return workflow, err
	}

	// Load workflow jobs
	rows, err := wm.db.QueryContext(context.Background(), "select id, workflow_id, node_id, job_id, status from workflow_jobs where workflow_id = ?;", id)
	if err != nil {
		wm.lm.Log("error", err.Error(), "workflow")
		return workflow, err
	}
	defer rows.Close()

	var workflowjobs []node_types.WorkflowJob
	for rows.Next() {
		var workflowjob node_types.WorkflowJob
		if err := rows.Scan(&workflowjob.Id, &workflowjob.WorkflowId, &workflowjob.NodeId,
			&workflowjob.JobId, &workflowjob.Status); err == nil {
			workflowjobs = append(workflowjobs, workflowjob)
		}
	}

	workflow.Jobs = workflowjobs

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
		inner join workflow_jobs wj
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
func (wm *WorkflowManager) Add(name string, description string, nodeId string, jobId int64) (int64, error) {
	// Add workflow
	wm.lm.Log("debug", fmt.Sprintf("add workflow %s (%s-%d)", name, nodeId, jobId), "workflows")

	result, err := wm.db.ExecContext(context.Background(), "insert into workflows (name, description) values (?, ?);",
		name, description)
	if err != nil {
		wm.lm.Log("error", err.Error(), "workflows")
		return 0, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		wm.lm.Log("error", err.Error(), "workflows")
		return 0, err
	}

	if jobId > 0 {
		_, err = wm.db.ExecContext(context.Background(), "insert into workflow_jobs (workflow_id, node_id, job_id) values (?, ?, ?);",
			id, nodeId, jobId)
		if err != nil {
			wm.lm.Log("error", err.Error(), "workflows")
			return 0, err
		}
	}

	return id, nil
}

// Add a workflow job
func (wm *WorkflowManager) AddWorkflowJob(workflowId int64, nodeId string, jobId int64) error {
	// Check if workflow exists
	if err, exists := wm.Exists(workflowId); err != nil || !exists {
		err = fmt.Errorf("workflow %d does not exist", workflowId)
		wm.lm.Log("debug", err.Error(), "workflows")
		return err
	}

	// Add workflow job
	wm.lm.Log("debug", fmt.Sprintf("add workflow job %s-%d to workflow id %d", nodeId, jobId, workflowId), "workflows")

	_, err := wm.db.ExecContext(context.Background(), "insert into workflow_jobs (workflow_id, node_id, job_id) values (?, ?, ?);",
		workflowId, nodeId, jobId)
	if err != nil {
		wm.lm.Log("error", err.Error(), "workflows")
		return err
	}

	return nil
}

// Remove a workflow job
func (wm *WorkflowManager) RemoveWorkflowJob(workflowId int64, nodeId string, jobId int64) error {
	// Check if workflow exists
	if err, exists := wm.Exists(workflowId); err != nil || !exists {
		err = fmt.Errorf("workflow %d does not exist", workflowId)
		wm.lm.Log("debug", err.Error(), "workflows")
		return err
	}

	// Check job status
	workflow, err := wm.Get(workflowId)
	if err != nil {
		wm.lm.Log("debug", err.Error(), "workflows")
		return err
	}

	var wfjob node_types.WorkflowJob
	for _, job := range workflow.Jobs {
		if job.JobId == jobId {
			wfjob = job
			break
		}
	}

	if wfjob.Status != "IDLE" {
		err = fmt.Errorf("can not remove workflow job in status %s from workflow Id %d",
			wfjob.Status, workflowId)
		wm.lm.Log("debug", err.Error(), "workflows")
		return err
	}

	// Remove workflow job
	wm.lm.Log("debug", fmt.Sprintf("remove workflow job %s-%d from workflow id %d", nodeId, jobId, workflowId), "workflows")

	_, err = wm.db.ExecContext(context.Background(), "delete from workflow_jobs where workflow_id = ? and node_id = ? and job_id = ?;",
		workflowId, nodeId, jobId)
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
		wm.lm.Log("debug", err.Error(), "workflows")
		return err
	}

	// Check job status
	workflow, err := wm.Get(workflowId)
	if err != nil {
		wm.lm.Log("debug", err.Error(), "workflows")
		return err
	}

	var exists bool = false
	for _, job := range workflow.Jobs {
		if job.JobId == jobId {
			exists = true
			break
		}
	}
	if !exists {
		err := fmt.Errorf("workflow job %d-%s-%d does not exist", workflowId, nodeId, jobId)
		wm.lm.Log("debug", err.Error(), "workflows")
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
