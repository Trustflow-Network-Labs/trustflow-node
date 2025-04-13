package node

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/utils"
)

type JobManager struct {
	db   *sql.DB
	lm   *utils.LogsManager
	sm   *ServiceManager
	wm   *WorkerManager
	p2pm *P2PManager
}

func NewJobManager(p2pm *P2PManager) *JobManager {
	return &JobManager{
		db:   p2pm.db,
		lm:   utils.NewLogsManager(),
		sm:   NewServiceManager(p2pm),
		wm:   NewWorkerManager(p2pm),
		p2pm: p2pm,
	}
}

// Job exists?
func (jm *JobManager) JobExists(id int64) (error, bool) {
	if id <= 0 {
		msg := "invalid job id"
		jm.lm.Log("error", msg, "jobs")
		return errors.New(msg), false
	}

	// Check if job is existing
	var jobId node_types.NullInt32
	row := jm.db.QueryRowContext(context.Background(), "select id from jobs where id = ?;", id)

	err := row.Scan(&jobId)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("debug", msg, "jobs")
		return nil, false
	}

	return nil, true
}

// Get job by id
func (jm *JobManager) GetJob(id int64) (node_types.Job, error) {
	var job node_types.Job
	var jobSql node_types.JobSql

	// Check if job exists in a queue
	err, exists := jm.JobExists(id)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "jobs")
		return job, err
	}
	if !exists {
		msg := fmt.Sprintf("Job %d does not exists in a queue", id)
		jm.lm.Log("error", msg, "jobs")
		return job, err
	}

	// Get job
	row := jm.db.QueryRowContext(context.Background(), "select id, service_id, ordering_node_id, input_node_ids, output_node_ids, execution_constraint, execution_constraint_detail, status, started, ended from jobs where id = ?;", id)

	err = row.Scan(&jobSql.Id, &jobSql.ServiceId, &jobSql.OrderingNodeId, &jobSql.InputNodeIds, &jobSql.OutputNodeIds, &jobSql.ExecutionConstraint, &jobSql.ExecutionConstraintDetail, &jobSql.Status, &jobSql.Started, &jobSql.Ended)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("debug", msg, "jobs")
		return job, err
	}

	job = jobSql.ToJob()

	return job, nil
}

// Get jobs by service ID
func (jm *JobManager) GetJobsByServiceId(serviceId int64, params ...uint32) ([]node_types.Job, error) {
	var jobSql node_types.JobSql
	var job node_types.Job
	var jobs []node_types.Job

	if serviceId <= 0 {
		msg := "invalid service ID"
		jm.lm.Log("error", msg, "jobs")
		return jobs, errors.New(msg)
	}

	var showOnlyActiveJobs bool = false // set to true for IDLE, READY and RUNNING jobs only
	if len(params) >= 1 {
		showOnlyActiveJobs = params[0] != 0
	}
	var offset uint32 = 0
	var limit uint32 = 10
	if len(params) >= 2 {
		offset = params[1]
	} else if len(params) >= 3 {
		offset = params[1]
		limit = params[2]
	}

	// Search for jobs
	sqlPatch := ""
	if showOnlyActiveJobs {
		sqlPatch = " AND (status = 'IDLE' OR status = 'READY' OR status = 'RUNNING') "
	}

	rows, err := jm.db.QueryContext(context.Background(), fmt.Sprintf("select id, service_id, ordering_node_id, input_node_ids, output_node_ids, execution_constraint, execution_constraint_detail, status, started, ended from jobs where service_id = ? %s limit ? offset ?;", sqlPatch),
		serviceId, limit, offset)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "jobs")
		return jobs, err
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&jobSql.Id, &jobSql.ServiceId, &jobSql.OrderingNodeId, &jobSql.InputNodeIds, &jobSql.OutputNodeIds, &jobSql.ExecutionConstraint, &jobSql.ExecutionConstraintDetail, &jobSql.Status, &jobSql.Started, &jobSql.Ended)
		if err != nil {
			msg := err.Error()
			jm.lm.Log("error", msg, "jobs")
			return jobs, err
		}
		job = jobSql.ToJob()
		jobs = append(jobs, job)
	}

	return jobs, nil
}

// Change job status
func (jm *JobManager) UpdateJobStatus(id int64, status string) error {
	// Check if job exists in a queue
	err, exists := jm.JobExists(id)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "jobs")
		return err
	}
	if !exists {
		msg := fmt.Sprintf("Job %d does not exists in a queue", id)
		jm.lm.Log("error", msg, "jobs")
		return err
	}

	// Update job status
	_, err = jm.db.ExecContext(context.Background(), "update jobs set status = ? where id = ?;",
		status, id)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "jobs")
		return err
	}

	return nil
}

// Create new job
func (jm *JobManager) CreateJob(serviceRequest node_types.ServiceRequest, orderingNode string) (node_types.Job, error) {
	var job node_types.Job

	// Create new job
	jm.lm.Log("debug", fmt.Sprintf("create job from ordering node id %s using service id %d", orderingNode, serviceRequest.ServiceId), "jobs")

	result, err := jm.db.ExecContext(context.Background(), "insert into jobs (service_id, ordering_node_id, input_node_ids, output_node_ids, execution_constraint, execution_constraint_detail) values (?, ?, ?, ?, ?, ?);",
		serviceRequest.ServiceId, orderingNode, strings.Join(serviceRequest.InputNodeIds, ","), strings.Join(serviceRequest.OutputNodeIds, ","), serviceRequest.ExecutionConstraint, serviceRequest.ExecutionConstraintDetail)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "jobs")
		return job, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "jobs")
		return job, err
	}

	jobBase := node_types.JobBase{
		Id:                        id,
		ServiceId:                 serviceRequest.ServiceId,
		OrderingNodeId:            orderingNode,
		ExecutionConstraint:       serviceRequest.ExecutionConstraint,
		ExecutionConstraintDetail: serviceRequest.ExecutionConstraintDetail,
		Status:                    "IDLE",
	}

	job = node_types.Job{
		JobBase:       jobBase,
		InputNodeIds:  serviceRequest.InputNodeIds,
		OutputNodeIds: serviceRequest.OutputNodeIds,
	}

	return job, nil
}

// Run jobs from queue
func (jm *JobManager) ProcessQueue() {
	var jobSql node_types.JobSql
	var jobs []node_types.JobSql

	// TODO, implement other cases ('NONE'/'READY' is just one case)
	rows, err := jm.db.QueryContext(context.Background(), "select id, service_id, ordering_node_id, input_node_ids, output_node_ids, execution_constraint, execution_constraint_detail, status, started, ended from jobs where execution_constraint = 'NONE' and status = 'READY';")
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return
	}

	for rows.Next() {
		err = rows.Scan(&jobSql.Id, &jobSql.ServiceId, &jobSql.OrderingNodeId, &jobSql.InputNodeIds, &jobSql.OutputNodeIds, &jobSql.ExecutionConstraint, &jobSql.ExecutionConstraintDetail, &jobSql.Status, &jobSql.Started, &jobSql.Ended)
		if err != nil {
			jm.lm.Log("error", err.Error(), "jobs")
			return
		}

		jobs = append(jobs, jobSql)
	}
	rows.Close()

	for _, job := range jobs {
		err = jm.RunJob(job.Id)
		if err != nil {
			jm.lm.Log("error", err.Error(), "jobs")
			return
		}
	}
}

// Run job from a queue
func (jm *JobManager) RunJob(jobId int64) error {
	// Get job from a queue
	job, err := jm.GetJob(jobId)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "jobs")
		return err
	}

	// Check job status
	status := job.Status

	switch status {
	case "IDLE":
		msg := fmt.Sprintf("Job id %d is %s", job.Id, status)
		jm.lm.Log("error", msg, "jobs")
		return err
	case "READY":
		err := jm.wm.StartWorker(jobId, jm)
		if err != nil {
			// Stop worker
			serr := jm.wm.StopWorker(jobId)
			if serr != nil {
				msg := serr.Error()
				jm.lm.Log("error", msg, "jobs")
				return err
			}

			// Log error
			jm.lm.Log("error", err.Error(), "jobs")
			return err
		}

		return nil
	case "RUNNING":
		msg := fmt.Sprintf("Job id %d is %s", job.Id, status)
		jm.lm.Log("error", msg, "jobs")
		return err
	case "CANCELLED":
		msg := fmt.Sprintf("Job id %d has been already executed with status %s. Create a new job to execute it", job.Id, status)
		jm.lm.Log("error", msg, "jobs")
		return err
	case "ERRORED":
		msg := fmt.Sprintf("Job id %d has been already executed with status %s. Create a new job to execute it", job.Id, status)
		jm.lm.Log("error", msg, "jobs")
		return err
	case "COMPLETED":
		msg := fmt.Sprintf("Job id %d has been already executed with status %s. Create a new job to execute it", job.Id, status)
		jm.lm.Log("error", msg, "jobs")
		return err
	default:
		msg := fmt.Sprintf("Unknown job status %s", status)
		jm.lm.Log("error", msg, "jobs")
		return err
	}
}

func (jm *JobManager) StartJob(id int64) error {
	job, err := jm.GetJob(id)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	// Check underlaying service
	jm.lm.Log("debug", fmt.Sprintf("checking job's underlaying service id %d", job.ServiceId), "jobs")

	service, err := jm.sm.Get(job.ServiceId)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	// Check if service is active
	if !service.Active {
		msg := fmt.Sprintf("Service id %d is inactive", service.Id)
		jm.lm.Log("error", msg, "jobs")
		return err
	}

	// Determine service type
	serviceType := service.Type

	// Set job status to RUNNING
	err = jm.UpdateJobStatus(job.Id, "RUNNING")
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	switch serviceType {
	case "DATA":
		err := jm.StreamDataJob(job)
		if err != nil {
			jm.lm.Log("error", err.Error(), "jobs")

			// Set job status to ERRORED
			err1 := jm.UpdateJobStatus(job.Id, "ERRORED")
			if err1 != nil {
				jm.lm.Log("error", err1.Error(), "jobs")
			}

			return err
		}
	case "DOCKER EXECUTION ENVIRONMENT":
	case "WASM EXECUTION ENVIRONMENT":
	default:
		msg := fmt.Sprintf("Unknown service type %s", serviceType)
		jm.lm.Log("error", msg, "jobs")
		return err
	}

	jm.lm.Log("debug", fmt.Sprintf("start running job id %d", id), "jobs")

	// Set job status to COMPLETED
	err = jm.UpdateJobStatus(job.Id, "COMPLETED")
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	err = jm.wm.StopWorker(job.Id)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	return nil
}

func (jm *JobManager) StreamDataJob(job node_types.Job) error {
	// Get data source path
	service, err := jm.sm.Get(job.ServiceId)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "jobs")
		return err
	}
	dataService, err := jm.sm.GetData(service.Id)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "jobs")
		return err
	}

	paths := strings.Split(dataService.Path, ",")
	if len(paths) > 0 {
		return jm.StreamDataJobEngine(job, paths, 0)
	}

	return nil
}

func (jm *JobManager) StreamDataJobEngine(job node_types.Job, paths []string, index int) error {
	path := "./local_storage/" + strings.TrimSpace(paths[index])

	// Check if the file exists
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		err = fmt.Errorf("file %s does not exist", path)
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	} else if err != nil {
		// Handle other potential errors
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	// Open the file for reading
	file, err := os.Open(path)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}
	//	defer file.Close() // Ensure the file is closed after operations

	receivingNodes := job.OutputNodeIds
	for _, receivingNode := range receivingNodes {
		// Get peer
		p, err := jm.p2pm.GeneratePeerFromId(receivingNode)
		if err != nil {
			jm.lm.Log("error", err.Error(), "jobs")
			return err
		}

		// Connect to peer and start streaming
		err = StreamData(jm.p2pm, p, file, nil)
		if err != nil {
			msg := err.Error()
			jm.lm.Log("error", msg, "jobs")
			return err
		}
	}

	if len(paths) > index+1 {
		return jm.StreamDataJobEngine(job, paths, index+1)
	}

	return nil
}
