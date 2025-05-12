package node

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/repo"
	"github.com/adgsm/trustflow-node/utils"
	"github.com/libp2p/go-libp2p/core/peer"
)

type JobManager struct {
	db   *sql.DB
	lm   *utils.LogsManager
	sm   *ServiceManager
	wm   *WorkerManager
	dm   *repo.DockerManager
	p2pm *P2PManager
	tm   *utils.TextManager
}

func NewJobManager(p2pm *P2PManager) *JobManager {
	return &JobManager{
		db:   p2pm.db,
		lm:   utils.NewLogsManager(),
		sm:   NewServiceManager(p2pm),
		wm:   NewWorkerManager(p2pm),
		dm:   repo.NewDockerManager(),
		p2pm: p2pm,
		tm:   utils.NewTextManager(),
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
	var jobInterface node_types.JobInterface

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
	row := jm.db.QueryRowContext(context.Background(), "select id, workflow_id, service_id, entrypoint, commands, ordering_node_id, execution_constraint, execution_constraint_detail, status, started, ended from jobs where id = ?;", id)

	err = row.Scan(&jobSql.Id, &jobSql.WorkflowId, &jobSql.ServiceId, &jobSql.Entrypoint, &jobSql.Commands, &jobSql.OrderingNodeId, &jobSql.ExecutionConstraint, &jobSql.ExecutionConstraintDetail, &jobSql.Status, &jobSql.Started, &jobSql.Ended)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("debug", msg, "jobs")
		return job, err
	}

	job = jobSql.ToJob()

	// Get job interfaces
	rows, err := jm.db.QueryContext(context.Background(), "select id, job_id, node_id, interface_type, functional_interface, path from job_interfaces where job_id = ?;", jobSql.Id)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "jobs")
		return job, err
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&jobInterface.InterfaceId, &jobInterface.JobId, &jobInterface.NodeId, &jobInterface.InterfaceType, &jobInterface.FunctionalInterface, &jobInterface.Path)
		if err != nil {
			msg := err.Error()
			jm.lm.Log("error", msg, "jobs")
			return job, err
		}
		jobInterface.WorkflowId = jobSql.WorkflowId
		job.JobInterfaces = append(job.JobInterfaces, jobInterface)
	}

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

	rows, err := jm.db.QueryContext(context.Background(), fmt.Sprintf("select id, workflow_id, service_id, entrypoint, commands, ordering_node_id, execution_constraint, execution_constraint_detail, status, started, ended from jobs where service_id = ? %s limit ? offset ?;", sqlPatch),
		serviceId, limit, offset)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "jobs")
		return jobs, err
	}

	for rows.Next() {
		err = rows.Scan(&jobSql.Id, &jobSql.WorkflowId, &jobSql.ServiceId, &jobSql.Entrypoint, &jobSql.Commands, &jobSql.OrderingNodeId, &jobSql.ExecutionConstraint, &jobSql.ExecutionConstraintDetail, &jobSql.Status, &jobSql.Started, &jobSql.Ended)
		if err != nil {
			msg := err.Error()
			jm.lm.Log("error", msg, "jobs")
			return jobs, err
		}
		job = jobSql.ToJob()
		jobs = append(jobs, job)
	}
	rows.Close()

	for i := range jobs {
		// Get job interfaces
		var jobInterface node_types.JobInterface
		rows, err := jm.db.QueryContext(context.Background(), "select id, job_id, node_id, interface_type, functional_interface, path from job_interfaces where job_id = ?;", jobs[i].Id)
		if err != nil {
			msg := err.Error()
			jm.lm.Log("error", msg, "jobs")
			return jobs, err
		}

		for rows.Next() {
			err = rows.Scan(&jobInterface.InterfaceId, &jobInterface.JobId, &jobInterface.NodeId, &jobInterface.InterfaceType, &jobInterface.FunctionalInterface, &jobInterface.Path)
			if err != nil {
				msg := err.Error()
				jm.lm.Log("error", msg, "jobs")
				return jobs, err
			}
			jobInterface.WorkflowId = jobSql.WorkflowId
			jobs[i].JobInterfaces = append(jobs[i].JobInterfaces, jobInterface)
		}
		rows.Close()
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

func (jm *JobManager) RequestService(
	peer peer.AddrInfo,
	workflowId int64,
	serviceId int64,
	entrypoint, commands []string,
	Interfaces []node_types.ServiceRequestInterface,
	constr, constrDet string,
) error {
	_, err := jm.p2pm.ConnectNode(peer)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "p2p")
		return err
	}

	serviceRequest := node_types.ServiceRequest{
		NodeId:                    peer.ID.String(),
		WorkflowId:                workflowId,
		ServiceId:                 serviceId,
		Entrypoint:                entrypoint,
		Commands:                  commands,
		Interfaces:                Interfaces,
		ExecutionConstraint:       constr,
		ExecutionConstraintDetail: constrDet,
	}

	err = StreamData(jm.p2pm, peer, &serviceRequest, nil, nil)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "p2p")
		return err
	}

	return nil
}

func (jm *JobManager) RequestJobRun(peer peer.AddrInfo, workflowId int64, jobId int64) error {
	_, err := jm.p2pm.ConnectNode(peer)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "p2p")
		return err
	}

	jobRunRequest := node_types.JobRunRequest{
		NodeId:     peer.ID.String(),
		WorkflowId: workflowId,
		JobId:      jobId,
	}

	err = StreamData(jm.p2pm, peer, &jobRunRequest, nil, nil)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "p2p")
		return err
	}

	return nil
}

func (jm *JobManager) SendJobRunStatus(peer peer.AddrInfo, workflowId int64, jobNodeId string, jobId int64, status string) error {
	_, err := jm.p2pm.ConnectNode(peer)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "p2p")
		return err
	}

	jobRunStatusRequest := node_types.JobRunStatusRequest{
		WorkflowId: workflowId,
		NodeId:     jobNodeId,
		JobId:      jobId,
	}

	jobRunStatus := node_types.JobRunStatus{
		JobRunStatusRequest: jobRunStatusRequest,
		Status:              status,
	}

	err = StreamData(jm.p2pm, peer, &jobRunStatus, nil, nil)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "p2p")
		return err
	}

	return nil
}

func (jm *JobManager) RequestJobRunStatus(peer peer.AddrInfo, workflowId int64, jobNodeId string, jobId int64) error {
	_, err := jm.p2pm.ConnectNode(peer)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "p2p")
		return err
	}

	jobRunStatusRequest := node_types.JobRunStatusRequest{
		WorkflowId: workflowId,
		NodeId:     jobNodeId,
		JobId:      jobId,
	}

	err = StreamData(jm.p2pm, peer, &jobRunStatusRequest, nil, nil)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "p2p")
		return err
	}

	return nil
}

// Create new job
func (jm *JobManager) CreateJob(serviceRequest node_types.ServiceRequest, orderingNode string) (node_types.Job, error) {
	var job node_types.Job
	var jobInterfaces []node_types.JobInterface

	// Create new job
	jm.lm.Log("debug", fmt.Sprintf("create job from ordering node id %s using service id %d", orderingNode, serviceRequest.ServiceId), "jobs")

	entrypoint := strings.Join(serviceRequest.Entrypoint, " ")
	commands := strings.Join(serviceRequest.Commands, " ")

	result, err := jm.db.ExecContext(context.Background(), "insert into jobs (workflow_id, service_id, entrypoint, commands, ordering_node_id, execution_constraint, execution_constraint_detail) values (?, ?, ?, ?, ?, ?, ?);",
		serviceRequest.WorkflowId, serviceRequest.ServiceId, entrypoint, commands, orderingNode, serviceRequest.ExecutionConstraint, serviceRequest.ExecutionConstraintDetail)
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
		WorkflowId:                serviceRequest.WorkflowId,
		ServiceId:                 serviceRequest.ServiceId,
		OrderingNodeId:            orderingNode,
		ExecutionConstraint:       serviceRequest.ExecutionConstraint,
		ExecutionConstraintDetail: serviceRequest.ExecutionConstraintDetail,
		Status:                    "IDLE",
	}

	for _, intface := range serviceRequest.Interfaces {
		result, err = jm.db.ExecContext(context.Background(), "insert into job_interfaces (job_id, node_id, interface_type, functional_interface, path) values (?, ?, ?, ?, ?);",
			id, intface.NodeId, intface.InterfaceType, intface.FunctionalInterface, intface.Path)
		if err != nil {
			msg := err.Error()
			jm.lm.Log("error", msg, "jobs")
			return job, err
		}

		interfaceId, err := result.LastInsertId()
		if err != nil {
			msg := err.Error()
			jm.lm.Log("error", msg, "jobs")
			return job, err
		}

		jobInterface := node_types.JobInterface{
			InterfaceId:             interfaceId,
			JobId:                   id,
			WorkflowId:              serviceRequest.WorkflowId,
			ServiceRequestInterface: intface,
		}

		jobInterfaces = append(jobInterfaces, jobInterface)
	}

	job = node_types.Job{
		JobBase:       jobBase,
		Entrypoint:    serviceRequest.Entrypoint,
		Commands:      serviceRequest.Commands,
		JobInterfaces: jobInterfaces,
	}

	return job, nil
}

// CRON, Run jobs from queue
func (jm *JobManager) ProcessQueue() {
	var id int64
	var ids []int64

	// TODO, implement other cases ('NONE'/'READY' is just one case)
	rows, err := jm.db.QueryContext(context.Background(),
		//		"select id from jobs where execution_constraint = 'NONE' and status = 'READY';")
		"select id from jobs where status = 'READY';")
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return
	}

	for rows.Next() {
		err = rows.Scan(&id)
		if err != nil {
			jm.lm.Log("error", err.Error(), "jobs")
			return
		}

		ids = append(ids, id)
	}
	rows.Close()

	configManager := utils.NewConfigManager("")
	configs, err := configManager.ReadConfigs()
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return
	}

	maxRetries, err := jm.tm.ToInt(configs["max_job_run_retries"])
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return
	}
	initialBackoff, err := jm.tm.ToInt(configs["job_initial_backoff"])
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return
	}
	backOff := time.Duration(initialBackoff) * time.Second
	for _, id := range ids {
		go jm.RunJobWithRetry(context.Background(), id, maxRetries, backOff)
	}
}

// CRON, Request remote jobs status update
func (jm *JobManager) RequestWorkflowJobsStatusUpdates() {
	// Load workflow jobs with status 'RUNNING'
	sql := `select distinct w.id, wj.node_id, wj.job_id
		from workflows w
		inner join workflow_jobs wj
		on wj.workflow_id = w.id
		where wj.status = 'RUNNING';`
	rows, err := jm.db.QueryContext(context.Background(), sql)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return
	}
	defer rows.Close()

	for rows.Next() {
		var workflowId int64
		var nodeId string
		var jobId int64
		if err := rows.Scan(&workflowId, &nodeId, &jobId); err == nil {
			peer, err := jm.p2pm.GeneratePeerAddrInfo(nodeId)
			if err != nil {
				jm.lm.Log("error", err.Error(), "jobs")
				return
			}
			err = jm.RequestJobRunStatus(peer, workflowId, nodeId, jobId)
			if err != nil {
				jm.lm.Log("error", err.Error(), "jobs")
				return
			}
		}
	}
}

// Run job from a queue
func (jm *JobManager) RunJob(ctx context.Context, jobId int64) error {
	// Get job from a queue
	job, err := jm.GetJob(jobId)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "jobs")
		return err
	}

	// Check job status
	status := job.Status

	if status != "READY" {
		msg := fmt.Sprintf("Job id %d is in status %s. Expected job status is 'READY'", job.Id, status)
		jm.lm.Log("error", msg, "jobs")
		return err
	}

	err = jm.wm.StartWorker(ctx, jobId, jm)
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
}

func (jm *JobManager) RunJobWithRetry(
	ctx context.Context,
	jobId int64,
	maxRetries int,
	initialBackoff time.Duration,
) error {
	backoff := initialBackoff
	var lastErr error

	for i := range maxRetries {
		// Check if context is cancelled before each attempt
		if ctx.Err() != nil {
			return ctx.Err()
		}

		err := jm.RunJob(ctx, jobId)
		if err == nil {
			return nil // success
		}

		lastErr = err
		if i < maxRetries-1 {
			// Add jitter to prevent thundering herd
			jitter := time.Duration(rand.Int63n(int64(backoff / 2)))
			sleepDuration := backoff + jitter

			select {
			case <-time.After(sleepDuration):
				backoff *= 2
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return lastErr
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

	jm.lm.Log("debug", fmt.Sprintf("started running job id %d", id), "jobs")

	switch serviceType {
	case "DATA":
		err := jm.streamDataJob(job)
		if err != nil {
			return err
		}
	case "DOCKER EXECUTION ENVIRONMENT":
		err := jm.dockerExecutionJob(job)
		if err != nil {
			return err
		}
	case "WASM EXECUTION ENVIRONMENT":
	default:
		msg := fmt.Sprintf("Unknown service type %s", serviceType)
		jm.lm.Log("error", msg, "jobs")
		return err
	}

	jm.lm.Log("debug", fmt.Sprintf("ended running job id %d", id), "jobs")

	err = jm.wm.StopWorker(job.Id)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	return nil
}

func (jm *JobManager) logAndEmitJobError(jobId int64, err error) {
	jm.lm.Log("error", err.Error(), "jobs")

	// Set job status to ERRORED
	err1 := jm.UpdateJobStatus(jobId, "ERRORED")
	if err1 != nil {
		jm.lm.Log("error", err1.Error(), "jobs")
	}

	// Send job status update to remote node
	go func() {
		err := jm.StatusUpdate(jobId, "ERRORED")
		if err != nil {
			jm.lm.Log("error", err.Error(), "jobs")
		}
	}()
}

func (jm *JobManager) StatusUpdate(jobId int64, status string) error {
	job, err := jm.GetJob(jobId)
	if err != nil {
		return err
	}
	// Send job status update to remote node
	nodeId := jm.p2pm.h.ID().String()
	peerId, err := jm.p2pm.GeneratePeerAddrInfo(job.OrderingNodeId)
	if err != nil {
		return err
	}
	return jm.SendJobRunStatus(peerId, job.WorkflowId, nodeId, job.Id, status)
}

func (jm *JobManager) streamDataJob(job node_types.Job) error {
	// Get data source path
	service, err := jm.sm.Get(job.ServiceId)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}
	dataService, err := jm.sm.GetData(service.Id)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	paths := strings.Split(dataService.Path, ",")
	if len(paths) > 0 {
		return jm.streamDataJobEngine(job, paths, 0)
	}

	return nil
}

func (jm *JobManager) streamDataJobEngine(job node_types.Job, paths []string, index int) error {
	configManager := utils.NewConfigManager("")
	configs, err := configManager.ReadConfigs()
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}
	path := configs["local_storage"] + strings.TrimSpace(paths[index])

	// Check if the file exists
	_, err = os.Stat(path)
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
	//	defer file.Close() // This should be done after streaming is finished

	host := jm.p2pm.h.ID().String()
	for _, jobInterface := range job.JobInterfaces {
		// Get peer
		if jobInterface.FunctionalInterface == "OUTPUT" {
			if jobInterface.NodeId == host {
				// It's own service / data
				fdir := configs["received_files_storage"] + host + "/" + fmt.Sprintf("%d", job.WorkflowId) + "/"
				if err = os.MkdirAll(fdir, 0755); err != nil {
					jm.lm.Log("error", err.Error(), "jobs")
					return err
				}
				dest := fdir + filepath.Base(path)
				if err = utils.FileCopy(path, dest, 48*1024); err != nil {
					jm.lm.Log("error", err.Error(), "jobs")
					return err
				}

				// Uncompress received file
				err = utils.Uncompress(dest, fdir)
				if err != nil {
					jm.lm.Log("error", err.Error(), "jobs")
					return err
				}
				err = os.RemoveAll(dest)
				if err != nil {
					jm.lm.Log("error", err.Error(), "jobs")
					return err
				}
			} else {
				// Send data to the requesting node
				p, err := jm.p2pm.GeneratePeerAddrInfo(jobInterface.NodeId)
				if err != nil {
					jm.lm.Log("error", err.Error(), "jobs")
					return err
				}

				// Connect to peer and start streaming
				err = StreamData(jm.p2pm, p, file, &job, nil)
				if err != nil {
					jm.lm.Log("error", err.Error(), "jobs")
					return err
				}
			}
		}
	}

	if len(paths) > index+1 {
		return jm.streamDataJobEngine(job, paths, index+1)
	}

	return nil
}

func (jm *JobManager) dockerExecutionJob(job node_types.Job) error {
	var inputFiles []string
	var outputFiles []string
	var mounts = make(map[string]string)
	var multiErr error

	configManager := utils.NewConfigManager("")
	configs, err := configManager.ReadConfigs()
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	// Get docker job
	service, err := jm.sm.Get(job.ServiceId)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}
	docker, err := jm.sm.GetDocker(service.Id)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	// Check are job inputs ready
	inputs, outputs, err := jm.dockerExecutionJobInterfaces(job.JobInterfaces)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	for _, input := range inputs {
		switch input.InterfaceType {
		case "STDIN/STDOUT", "FILE STREAM":
			inputFiles, err = jm.validatePaths(configs["received_files_storage"], input, inputFiles)
			if err != nil {
				jm.lm.Log("error", err.Error(), "jobs")
				return err
			}
		case "MOUNTED FILE SYSTEM":
			mounts, err = jm.validateMounts(configs["received_files_storage"], input, mounts)
			if err != nil {
				jm.lm.Log("error", err.Error(), "jobs")
				return err
			}
		default:
			err := fmt.Errorf("unknown interface type `%s`", input.InterfaceType)
			jm.lm.Log("error", err.Error(), "jobs")
			return err
		}
	}

	for _, output := range outputs {
		switch output.InterfaceType {
		case "STDIN/STDOUT", "FILE STREAM":
			outputFiles, err = jm.createPaths(configs["received_files_storage"], output, outputFiles)
			if err != nil {
				jm.lm.Log("error", err.Error(), "jobs")
				return err
			}
		case "MOUNTED FILE SYSTEM":
			mounts, err = jm.createMounts(configs["received_files_storage"], output, mounts)
			if err != nil {
				jm.lm.Log("error", err.Error(), "jobs")
				return err
			}
		default:
			err := fmt.Errorf("unknown interface type `%s`", output.InterfaceType)
			jm.lm.Log("error", err.Error(), "jobs")
			return err
		}
	}

	if len(docker.RepoDockerComposes) > 0 {
		// Run docker-compose
		for _, compose := range docker.RepoDockerComposes {
			containers, _, errs := jm.dm.Run(
				docker.Repo,
				job.Id,
				false,
				"",
				true,
				compose,
				"",
				inputFiles,
				outputFiles,
				mounts,
				job.Entrypoint,
				job.Commands,
			)
			for _, err := range errs {
				jm.lm.Log("error", err.Error(), "jobs")
				multiErr = errors.Join(multiErr, err)
			}
			if multiErr != nil {
				return fmt.Errorf("docker compose errors: %w", multiErr)
			}
			jm.lm.Log("debug", fmt.Sprintf("the following containers were running %s", strings.Join(containers, ", ")), "jobs")
		}
	} else if len(docker.RepoDockerFiles) > 0 {
		// Run dockerfiles
		containers, _, errs := jm.dm.Run(
			docker.Repo,
			job.Id,
			false,
			"",
			true,
			"",
			"",
			inputFiles,
			outputFiles,
			mounts,
			job.Entrypoint,
			job.Commands,
		)
		for _, err := range errs {
			jm.lm.Log("error", err.Error(), "jobs")
			multiErr = errors.Join(multiErr, err)
		}
		if multiErr != nil {
			return fmt.Errorf("docker compose errors: %w", multiErr)
		}
		jm.lm.Log("debug", fmt.Sprintf("the following containers were running %s", strings.Join(containers, ", ")), "jobs")
	} else if len(docker.Images) > 0 {
		// Run images
		for _, image := range docker.Images {
			if len(image.ImageTags) == 0 {
				err := fmt.Errorf("image %s has no tags", image.ImageName)
				jm.lm.Log("error", err.Error(), "jobs")
				return err
			}

			containers, _, errs := jm.dm.Run(
				docker.Repo,
				job.Id,
				false,
				image.ImageTags[0],
				true,
				"",
				"",
				inputFiles,
				outputFiles,
				mounts,
				job.Entrypoint,
				job.Commands,
			)
			for _, err := range errs {
				jm.lm.Log("error", err.Error(), "jobs")
				multiErr = errors.Join(multiErr, err)
			}
			if multiErr != nil {
				return fmt.Errorf("docker compose errors: %w", multiErr)
			}
			jm.lm.Log("debug", fmt.Sprintf("the following containers were running %s", strings.Join(containers, ", ")), "jobs")
		}
	} else {
		err := errors.New("no docker-compose.yml, Dockerfiles, or images existing")
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	return nil
}

func (jm *JobManager) dockerExecutionJobInterfaces(jobInterfaces []node_types.JobInterface) ([]node_types.JobInterface, []node_types.JobInterface, error) {
	var inputs []node_types.JobInterface
	var outputs []node_types.JobInterface
	for _, intrfce := range jobInterfaces {
		if intrfce.FunctionalInterface == "INPUT" {
			inputs = append(inputs, intrfce)
		} else if intrfce.FunctionalInterface == "OUTPUT" {
			outputs = append(inputs, intrfce)
		}
	}
	return inputs, outputs, nil
}

func (jm *JobManager) validatePaths(base string, inrfce node_types.JobInterface, files []string) ([]string, error) {
	paths := strings.SplitSeq(inrfce.Path, ",")
	for path := range paths {
		path = base + inrfce.NodeId + "/" + fmt.Sprintf("%d", inrfce.WorkflowId) + "/" + strings.TrimSpace(path)

		// Check if the path exists
		err := jm.pathExists(path)
		if err != nil {
			jm.lm.Log("error", err.Error(), "jobs")
			return nil, err
		}

		path, err = filepath.Abs(path)
		if err != nil {
			jm.lm.Log("error", err.Error(), "jobs")
			return nil, err
		}

		files = append(files, path)
	}

	return files, nil
}

func (jm *JobManager) createPaths(base string, inrfce node_types.JobInterface, files []string) ([]string, error) {
	var err error

	paths := strings.SplitSeq(inrfce.Path, ",")
	for path := range paths {
		path = base + inrfce.NodeId + "/" + fmt.Sprintf("%d", inrfce.WorkflowId) + "/" + strings.TrimSpace(path)

		// Make sure file path is created
		fdir := filepath.Dir(path)
		if err := os.MkdirAll(fdir, 0755); err != nil {
			jm.lm.Log("error", err.Error(), "jobs")
			return nil, err
		}

		path, err = filepath.Abs(path)
		if err != nil {
			jm.lm.Log("error", err.Error(), "jobs")
			return nil, err
		}

		files = append(files, path)

	}

	return files, nil
}

func (jm *JobManager) validateMounts(base string, inrfce node_types.JobInterface, mounts map[string]string) (map[string]string, error) {
	paths := strings.Split(inrfce.Path, ":")

	if len(paths) != 2 {
		err := fmt.Errorf("invalid mount path %s", inrfce.Path)
		jm.lm.Log("error", err.Error(), "jobs")
		return nil, err
	}

	path := base + inrfce.NodeId + "/" + fmt.Sprintf("%d", inrfce.WorkflowId) + "/" + strings.TrimSpace(paths[0])

	// Check if the path exists
	err := jm.pathExists(path)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return nil, err
	}

	path, err = filepath.Abs(path)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return nil, err
	}

	mounts[path] = paths[1]
	return mounts, nil
}

func (jm *JobManager) createMounts(base string, inrfce node_types.JobInterface, mounts map[string]string) (map[string]string, error) {
	var err error

	paths := strings.Split(inrfce.Path, ":")

	if len(paths) != 2 {
		err := fmt.Errorf("invalid mount path %s", inrfce.Path)
		jm.lm.Log("error", err.Error(), "jobs")
		return nil, err
	}

	path := base + inrfce.NodeId + "/" + fmt.Sprintf("%d", inrfce.WorkflowId) + "/" + strings.TrimSpace(paths[0])

	// Make sure file path is created
	//	fdir := filepath.Dir(path)
	//	if err := os.MkdirAll(fdir, 0755); err != nil {
	//		jm.lm.Log("error", err.Error(), "jobs")
	//		return nil, err
	//	}

	path, err = filepath.Abs(path)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return nil, err
	}

	mounts[path] = paths[1]

	fmt.Printf("%v\n", mounts)
	return mounts, nil
}

func (jm *JobManager) pathExists(path string) error {
	// Check if the path exists
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		err = fmt.Errorf("path %s does not exist", path)
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	} else if err != nil {
		// Handle other potential errors
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	return nil
}
