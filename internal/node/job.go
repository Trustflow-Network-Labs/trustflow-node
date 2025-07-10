package node

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/adgsm/trustflow-node/internal/node_types"
	"github.com/adgsm/trustflow-node/internal/repo"
	"github.com/adgsm/trustflow-node/internal/utils"
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
	vm   *utils.ValidatorManager
}

func NewJobManager(p2pm *P2PManager) *JobManager {
	return &JobManager{
		db:   p2pm.DB,
		lm:   utils.NewLogsManager(),
		sm:   NewServiceManager(p2pm),
		wm:   NewWorkerManager(p2pm),
		dm:   repo.NewDockerManager(p2pm.UI),
		p2pm: p2pm,
		tm:   utils.NewTextManager(),
		vm:   utils.NewValidatorManager(),
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
	row := jm.db.QueryRowContext(context.Background(), "select id, workflow_id, service_id, entrypoint, commands, ordering_node_id, execution_constraint, execution_constraint_detail, status, started, ended from jobs where id = ?;", id)

	err = row.Scan(&jobSql.Id, &jobSql.WorkflowId, &jobSql.ServiceId, &jobSql.Entrypoint, &jobSql.Commands, &jobSql.OrderingNodeId, &jobSql.ExecutionConstraint, &jobSql.ExecutionConstraintDetail, &jobSql.Status, &jobSql.Started, &jobSql.Ended)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("debug", msg, "jobs")
		return job, err
	}

	job = jobSql.ToJob()

	// Get job interfaces
	rows, err := jm.db.QueryContext(context.Background(), "select id, job_id, interface_type, path from job_interfaces where job_id = ?;", jobSql.Id)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "jobs")
		return job, err
	}

	for rows.Next() {
		var jobInterface node_types.JobInterface
		err = rows.Scan(&jobInterface.InterfaceId, &jobInterface.JobId, &jobInterface.InterfaceType, &jobInterface.Path)
		if err != nil {
			msg := err.Error()
			jm.lm.Log("error", msg, "jobs")
			return job, err
		}
		jobInterface.WorkflowId = jobSql.WorkflowId
		job.JobInterfaces = append(job.JobInterfaces, jobInterface)
	}
	rows.Close()

	// Get job interface peers
	for i, intfce := range job.JobInterfaces {
		rows, err = jm.db.QueryContext(context.Background(), "select peer_node_id, peer_job_id, peer_mount_function, path, done from job_interface_peers where job_interface_id = ?;", intfce.InterfaceId)
		if err != nil {
			msg := err.Error()
			jm.lm.Log("error", msg, "jobs")
			return job, err
		}
		for rows.Next() {
			var jobInterfacePeer node_types.JobInterfacePeer
			err = rows.Scan(&jobInterfacePeer.PeerNodeId, &jobInterfacePeer.PeerJobId, &jobInterfacePeer.PeerMountFunction, &jobInterfacePeer.PeerPath, &jobInterfacePeer.PeerDuty)
			if err != nil {
				msg := err.Error()
				jm.lm.Log("error", msg, "jobs")
				return job, err
			}
			job.JobInterfaces[i].JobInterfacePeers = append(job.JobInterfaces[i].JobInterfacePeers, jobInterfacePeer)
		}
		rows.Close()
	}

	return job, nil
}

// Remove a  job
func (jm *JobManager) RemoveJob(id int64) error {
	// Get job
	job, err := jm.GetJob(id)
	if err != nil {
		jm.lm.Log("debug", err.Error(), "jobs")
		return err
	}

	if job.Status != "IDLE" {
		err = fmt.Errorf("can not remove job id %d in status %s",
			id, job.Status)
		jm.lm.Log("debug", err.Error(), "jobs")
		return err
	}

	// Remove job
	jm.lm.Log("debug", fmt.Sprintf("removing job %d", id), "jobs")

	_, err = jm.db.ExecContext(context.Background(), "delete from jobs where id = ?;", id)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	return nil
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
		rows, err := jm.db.QueryContext(context.Background(), "select id, job_id, interface_type, path from job_interfaces where job_id = ?;", jobs[i].Id)
		if err != nil {
			msg := err.Error()
			jm.lm.Log("error", msg, "jobs")
			return jobs, err
		}

		for rows.Next() {
			err = rows.Scan(&jobInterface.InterfaceId, &jobInterface.JobId, &jobInterface.InterfaceType, &jobInterface.Path)
			if err != nil {
				msg := err.Error()
				jm.lm.Log("error", msg, "jobs")
				return jobs, err
			}
			jobInterface.WorkflowId = jobSql.WorkflowId
			jobs[i].JobInterfaces = append(jobs[i].JobInterfaces, jobInterface)
		}
		rows.Close()

		// Get job interface peers
		for i, intfce := range jobs[i].JobInterfaces {
			rows, err = jm.db.QueryContext(context.Background(), "select peer_node_id, peer_job_id, peer_mount_function, path, done from job_interface_peers where job_interface_id = ?;", intfce.InterfaceId)
			if err != nil {
				msg := err.Error()
				jm.lm.Log("error", msg, "jobs")
				return jobs, err
			}
			for rows.Next() {
				var jobInterfacePeer node_types.JobInterfacePeer
				err = rows.Scan(&jobInterfacePeer.PeerNodeId, &jobInterfacePeer.PeerJobId, &jobInterfacePeer.PeerMountFunction, &jobInterfacePeer.PeerPath, &jobInterfacePeer.PeerDuty)
				if err != nil {
					jm.lm.Log("error", err.Error(), "jobs")
					return jobs, err
				}
				job.JobInterfaces[i].JobInterfacePeers = append(job.JobInterfaces[i].JobInterfacePeers, jobInterfacePeer)
			}
			rows.Close()
		}
	}

	return jobs, nil
}

// Acknowledge receipt
func (jm *JobManager) AcknowledgeReceipt(jobId int64, interfaceId int64, peer peer.ID, dir string) error {
	var foundInterface bool = false

	job, err := jm.GetJob(jobId)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	// Check if remote peer exists in job interface peers list
	for _, intface := range job.JobInterfaces {
		if intface.InterfaceId != interfaceId {
			continue
		}
		for _, interfacePeer := range intface.JobInterfacePeers {
			bDataDir := false

			switch dir {
			case "INPUT":
				bDataDir = interfacePeer.PeerMountFunction == "PROVIDER" || intface.InterfaceType == "INPUT"
			case "OUTPUT":
				bDataDir = interfacePeer.PeerMountFunction == "RECEIVER" || intface.InterfaceType == "OUTPUT"
			default:
				err := fmt.Errorf("unknown data direction `%s`", dir)
				jm.lm.Log("error", err.Error(), "jobs")
				return err
			}

			if interfacePeer.PeerNodeId == peer.String() && bDataDir {
				foundInterface = true
				err := jm.updateJobInterfacePeerDuty(intface.InterfaceId, interfacePeer.PeerNodeId, interfacePeer.PeerJobId, true)
				if err != nil {
					jm.lm.Log("error", err.Error(), "jobs")
					return err
				}
			}
		}
	}

	if !foundInterface {
		err := fmt.Errorf("provided peer interface ID %s `%s` is not found for job Id `%d`, in interface Id `%d`",
			peer.String(), dir, jobId, interfaceId)
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	return nil
}

// Update job interface peer duty flag
func (jm *JobManager) updateJobInterfacePeerDuty(interfaceId int64, peerNodeId string, peerJobId int64, duty bool) error {
	// Update job interface peer duty
	done := int64(0)
	if duty {
		done = int64(1)
	}
	_, err := jm.db.ExecContext(context.Background(), "update job_interface_peers set done = ? where job_interface_id = ? and peer_node_id = ? and peer_job_id = ?;",
		done, interfaceId, peerNodeId, peerJobId)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	return nil
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

// Change job execution constraint
func (jm *JobManager) UpdateJobExecutionConstraint(id int64, constraint string) error {
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

	// Update job execution constraint
	_, err = jm.db.ExecContext(context.Background(), "update jobs set execution_constraint = ? where id = ?;",
		constraint, id)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "jobs")
		return err
	}

	return nil
}

func (jm *JobManager) RequestJob(
	peer peer.AddrInfo,
	workflowId int64,
	workflowJobId int64,
	serviceId int64,
	entrypoint, commands []string,
	Interfaces []node_types.RequestInterface,
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
		WorkflowJobId:             workflowJobId,
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

func (jm *JobManager) RequestJobCancellation(
	peer peer.AddrInfo,
	jobId int64,
	workflowJobId int64,
) error {
	_, err := jm.p2pm.ConnectNode(peer)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "p2p")
		return err
	}

	serviceRequestCancellation := node_types.ServiceRequestCancellation{
		NodeId:        peer.ID.String(),
		JobId:         jobId,
		WorkflowJobId: workflowJobId,
	}

	err = StreamData(jm.p2pm, peer, &serviceRequestCancellation, nil, nil)
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

	// Create new job
	jm.lm.Log("debug", fmt.Sprintf("create job from ordering node id %s using service id %d", orderingNode, serviceRequest.ServiceId), "jobs")

	entrypoint := utils.ShlexJoin(serviceRequest.Entrypoint)
	commands := utils.ShlexJoin(serviceRequest.Commands)

	result, err := jm.db.ExecContext(context.Background(), "insert into jobs (workflow_id, service_id, entrypoint, commands, ordering_node_id, execution_constraint, execution_constraint_detail) values (?, ?, ?, ?, ?, ?, ?);",
		serviceRequest.WorkflowId, serviceRequest.ServiceId, entrypoint, commands, orderingNode, serviceRequest.ExecutionConstraint, serviceRequest.ExecutionConstraintDetail)
	if err != nil {
		msg := err.Error()
		jm.lm.Log("error", msg, "jobs")
		return job, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return job, err
	}

	jobInterfaces, err := jm.CreateJobInterfaces(id, serviceRequest.Interfaces)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
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

	job = node_types.Job{
		JobBase:       jobBase,
		Entrypoint:    serviceRequest.Entrypoint,
		Commands:      serviceRequest.Commands,
		JobInterfaces: jobInterfaces,
	}

	return job, nil
}

func (jm *JobManager) CreateJobInterfaces(
	jobId int64,
	interfaces []node_types.RequestInterface,
) ([]node_types.JobInterface, error) {

	// Get job
	job, err := jm.GetJob(jobId)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return nil, err
	}

	// Get underlaying service
	service, err := jm.sm.Get(job.ServiceId)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return nil, err
	}

	// Check if requested interace(s) connections are matching service interface type(s)
	var allowedInterfaces []string
	switch service.Type {
	case "DATA":
		allowedInterfaces = []string{"STDOUT"}

	case "DOCKER EXECUTION ENVIRONMENT":
		allowedInterfaces = []string{"STDIN", "STDOUT", "MOUNT"}

	case "STANDALONE EXECUTABLE":
		allowedInterfaces = []string{"STDIN", "STDOUT"}

	default:
		err := fmt.Errorf("unknown service type %s", service.Type)
		jm.lm.Log("error", err.Error(), "jobs")
		return nil, err
	}

	if allowed, intfce := jm.allowedRequestInterfaces(interfaces, allowedInterfaces); !allowed {
		err := fmt.Errorf("service id `%d` does not have defined interface type %s", job.ServiceId, intfce)
		return nil, err
	}

	// Remove existing interfaces (if any)
	_, err = jm.db.ExecContext(context.Background(), "delete from job_interfaces where job_id = ?;", job.Id)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return nil, err
	}

	// Add interfaces
	for _, intface := range interfaces {
		result, err := jm.db.ExecContext(context.Background(), "insert into job_interfaces (job_id, interface_type, path) values (?, ?, ?);",
			job.Id, intface.InterfaceType, intface.Path)
		if err != nil {
			jm.lm.Log("error", err.Error(), "jobs")
			return nil, err
		}

		// Last inserted interface Id
		interfaceId, err := result.LastInsertId()
		if err != nil {
			msg := err.Error()
			jm.lm.Log("error", msg, "jobs")
			return nil, err
		}

		var jobInterfacePeers []node_types.JobInterfacePeer
		for _, interfacePeer := range intface.JobInterfacePeers {
			// If this is STDIN interface type peer job id must be the job id
			if intface.InterfaceType == "STDIN" || interfacePeer.PeerMountFunction == "PROVIDER" {
				interfacePeer.PeerJobId = job.Id
			}
			_, err = jm.db.ExecContext(context.Background(), "insert into job_interface_peers (job_interface_id, peer_node_id, peer_job_id, peer_mount_function, path) values (?, ?, ?, ?, ?);",
				interfaceId, interfacePeer.PeerNodeId, interfacePeer.PeerJobId, interfacePeer.PeerMountFunction, interfacePeer.PeerPath)
			if err != nil {
				msg := err.Error()
				jm.lm.Log("error", msg, "jobs")
				return nil, err
			}

			jobInterfacePeer := node_types.JobInterfacePeer{
				PeerJobId:         interfacePeer.PeerJobId,
				PeerNodeId:        interfacePeer.PeerNodeId,
				PeerPath:          interfacePeer.PeerPath,
				PeerMountFunction: interfacePeer.PeerMountFunction,
			}
			jobInterfacePeers = append(jobInterfacePeers, jobInterfacePeer)
		}

		jobInterface := node_types.JobInterface{
			InterfaceId:       interfaceId,
			JobId:             job.Id,
			WorkflowId:        job.WorkflowId,
			JobInterfacePeers: jobInterfacePeers,
			Interface:         intface.Interface,
		}

		job.JobInterfaces = append(job.JobInterfaces, jobInterface)
	}

	return job.JobInterfaces, nil
}

func (jm *JobManager) allowedRequestInterfaces(
	interfaces []node_types.RequestInterface,
	allowed []string,
) (bool, string) {
	for _, intface := range interfaces {
		if !utils.InSlice(intface.InterfaceType, allowed) {
			return false, intface.InterfaceType
		}
	}
	return true, ""
}

// CRON, Run jobs from queue
func (jm *JobManager) ProcessQueue() {
	var id int64
	var idsNone []int64
	var idsConstraint []int64

	// Pick jobs ready for execution
	rows, err := jm.db.QueryContext(context.Background(),
		"select id, execution_constraint from jobs where (execution_constraint = 'NONE' or execution_constraint = 'INPUTS READY') and status = 'READY';")
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return
	}

	for rows.Next() {
		var executionConstraint string
		err = rows.Scan(&id, &executionConstraint)
		if err != nil {
			jm.lm.Log("error", err.Error(), "jobs")
			return
		}

		if executionConstraint == "INPUTS READY" {
			idsConstraint = append(idsConstraint, id)
		}

		idsNone = append(idsNone, id)
	}
	rows.Close()

	for _, id := range idsConstraint {
		job, err := jm.GetJob(id)
		if err != nil {
			jm.lm.Log("error", err.Error(), "jobs")
			return
		}

		if _, _, _, err := jm.checkInterfaces(job); err != nil {
			jm.lm.Log("debug", err.Error(), "jobs")
			continue
		}

		idsNone = append(idsNone, id)
	}

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
	for _, id := range idsNone {
		go jm.doWithRetry("run job", context.Background(), id, maxRetries, backOff)
	}
}

// Check if job is waiting for input(s)
func (jm *JobManager) JobExpectingInputsFrom(jobId int64, sendingNode string) (bool, error) {
	var count int

	// Find job
	row := jm.db.QueryRowContext(context.Background(),
		"select count(j.id) from jobs j inner join job_interfaces ji on ji.job_id = j.id inner join job_interface_peers jip on jip.job_interface_id = ji.id where jip.peer_job_id = ? and (ji.interface_type = 'STDIN' or jip.peer_mount_function = 'PROVIDER') and (j.status = 'IDLE' or j.status = 'READY') and jip.peer_node_id = ?;",
		jobId, sendingNode)
	err := row.Scan(&count)
	if err != nil {
		jm.lm.Log("debug", err.Error(), "jobs")
		return false, err
	}

	return count > 0, nil
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
func (jm *JobManager) RunJob(ctx context.Context, jobId int64, retry, maxRetries int) error {
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

	err = jm.wm.StartWorker(ctx, jobId, jm, retry, maxRetries)
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

func (jm *JobManager) doWithRetry(
	doWhat string,
	ctx context.Context,
	jobId int64,
	maxRetries int,
	initialBackoff time.Duration,
) error {
	backoff := initialBackoff
	var execErr error
	var lastErr error

	for i := range maxRetries {
		// Check if context is cancelled before each attempt
		if ctx.Err() != nil {
			return ctx.Err()
		}

		switch doWhat {
		case "run job":
			execErr = jm.RunJob(ctx, jobId, i, maxRetries)
			if execErr == nil {
				return nil // success
			}
		case "send data":
			execErr = jm.streamDataJob(jobId)
			if execErr == nil {
				return nil // success
			}
		case "send docker output":
			execErr = jm.sendDockerOutput(jobId)
			if execErr == nil {
				return nil // success
			}
		default:
			execErr = fmt.Errorf("unsupported action `%s`", doWhat)
			return execErr
		}

		lastErr = execErr
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

	configManager := utils.NewConfigManager("")
	configs, err := configManager.ReadConfigs()
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}
	maxRetries, err := jm.tm.ToInt(configs["max_job_send_output_retries"])
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}
	initialBackoff, err := jm.tm.ToInt(configs["job_send_output_initial_backoff"])
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}
	backOff := time.Duration(initialBackoff) * time.Second

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
		err := jm.doWithRetry("send data", context.Background(), job.Id, maxRetries, backOff)
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

func (jm *JobManager) streamDataJob(jobId int64) error {
	// Get job
	job, err := jm.GetJob(jobId)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

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

	host := jm.p2pm.h.ID().String()
	for _, jobInterface := range job.JobInterfaces {
		if jobInterface.InterfaceType != "STDOUT" {
			continue
		}
		for _, interfacePeer := range jobInterface.JobInterfacePeers {

			// Is the DATA job input?
			var jobId int64 = job.Id
			if interfacePeer.PeerJobId != 0 {
				jobId = interfacePeer.PeerJobId
				// Override job Id
				job.Id = jobId
			}

			// Get peer
			if interfacePeer.PeerNodeId == host {
				// It's own service / data
				fdir := filepath.Join(configs["local_storage"], "workflows", job.OrderingNodeId, strconv.FormatInt(job.WorkflowId, 10), "job", strconv.FormatInt(jobId, 10), "input", host)
				if err = os.MkdirAll(fdir, 0755); err != nil {
					jm.lm.Log("error", err.Error(), "jobs")
					return err
				}
				dest := fdir + filepath.Base(path)
				if err = utils.BufferFileCopy(path, dest, 48*1024); err != nil {
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
				p, err := jm.p2pm.GeneratePeerAddrInfo(interfacePeer.PeerNodeId)
				if err != nil {
					jm.lm.Log("error", err.Error(), "jobs")
					return err
				}

				// Open the file for reading
				file, err := os.Open(path)
				if err != nil {
					jm.lm.Log("error", err.Error(), "jobs")
					return err
				}
				//	defer file.Close() // This must be done after streaming is finished

				jobReplica := jm.createPeerJobReplica(job, jobInterface, interfacePeer, false)

				// Connect to peer and start streaming
				err = StreamData(jm.p2pm, p, file, &jobReplica, nil)
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
	var multiErr error

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

	// Check are job inputs, outputs and mounts ready
	inputFiles, outputFiles, mounts, err := jm.checkInterfaces(job)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	if len(docker.RepoDockerComposes) > 0 {
		// Run docker-compose
		for _, compose := range docker.RepoDockerComposes {
			containers, _, errs := jm.dm.Run(
				docker.Repo,
				&job,
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
			&job,
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
				&job,
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

	// Send job outputs
	configManager := utils.NewConfigManager("")
	configs, err := configManager.ReadConfigs()
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}
	maxRetries, err := jm.tm.ToInt(configs["max_job_send_output_retries"])
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}
	initialBackoff, err := jm.tm.ToInt(configs["job_send_output_initial_backoff"])
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}
	backOff := time.Duration(initialBackoff) * time.Second
	go jm.doWithRetry("send docker output", context.Background(), job.Id, maxRetries, backOff)

	return nil
}

// Send docker job outputs
func (jm *JobManager) sendDockerOutput(jobId int64) error {
	configManager := utils.NewConfigManager("")
	configs, err := configManager.ReadConfigs()
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	job, err := jm.GetJob(jobId)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	for _, intrface := range job.JobInterfaces {
		base := filepath.Join(configs["local_storage"], "workflows", job.OrderingNodeId, strconv.FormatInt(job.WorkflowId, 10))
		for _, interfacePeer := range intrface.JobInterfacePeers {
			err := jm.sendDataForPeerInterface(job, intrface, interfacePeer, base, configs, "OUTPUT")
			if err != nil {
				jm.lm.Log("error", err.Error(), "jobs")
				return err
			}
		}
	}

	return nil
}

// Send data for a specific interface and peer
func (jm *JobManager) sendDataForPeerInterface(
	job node_types.Job,
	intrface node_types.JobInterface,
	interfacePeer node_types.JobInterfacePeer,
	base string,
	configs utils.Config,
	whatData string,
) error {
	paths := strings.SplitSeq(interfacePeer.PeerPath, ",")
	for path := range paths {
		path := strings.TrimSpace(path)
		isDir := strings.HasSuffix(path, string(os.PathSeparator))

		switch intrface.InterfaceType {
		case "STDIN":
			if whatData != "INPUT" {
				continue
			}
			path = filepath.Join(base, "job", strconv.FormatInt(intrface.JobId, 10), "input", interfacePeer.PeerNodeId, path)
		case "STDOUT":
			if whatData != "OUTPUT" {
				continue
			}
			path = filepath.Join(base, "job", strconv.FormatInt(intrface.JobId, 10), "output", interfacePeer.PeerNodeId, path)
		case "MOUNT":
			// Check if this peer is RECEIVER
			if interfacePeer.PeerMountFunction == "RECEIVER" && whatData == "OUTPUT" {
				var lnkDir string

				// Is mount point file or dir
				isDir = strings.HasSuffix(intrface.Path, string(os.PathSeparator))
				path = filepath.Join(base, "job", strconv.FormatInt(intrface.JobId, 10), "mounts", intrface.Path)

				// Is link file or dir
				isLnkDir := strings.HasSuffix(interfacePeer.PeerPath, string(os.PathSeparator))
				lnk := filepath.Join(base, "job", strconv.FormatInt(intrface.JobId, 10), "output", interfacePeer.PeerNodeId, interfacePeer.PeerPath)

				// Are we linking file to dir or vice-versa
				if !isDir && isLnkDir {
					lnk = filepath.Join(lnk, filepath.Base(path))
					isLnkDir = !isLnkDir
				} else if isDir && !isLnkDir {
					path = filepath.Join(path, filepath.Base(lnk))
				}

				// Create directory sub-structure for link
				lnkDir = filepath.Dir(lnk)
				if err := os.MkdirAll(lnkDir, 0755); err != nil {
					jm.lm.Log("error", err.Error(), "jobs")
					return err
				}

				// Create symb link
				err := utils.CreateSymlink(path, lnk)
				if err != nil {
					jm.lm.Log("error", err.Error(), "jobs")
					return err
				}
				path = lnk
				isDir = isLnkDir
			} else if interfacePeer.PeerMountFunction == "PROVIDER" && whatData == "INPUT" {
				// Provide job INPUT data
				isDir = strings.HasSuffix(interfacePeer.PeerPath, string(os.PathSeparator))
				path = filepath.Join(base, "job", strconv.FormatInt(intrface.JobId, 10), "input", interfacePeer.PeerNodeId, interfacePeer.PeerPath)
			} else if whatData == "STD" {
				// Provide job STD logs
				isDir = true
				path = filepath.Join(base, "job", strconv.FormatInt(intrface.JobId, 10), "logs")
			} else {
				// Skip
				continue
			}
		default:
			continue
		}
		if isDir && !strings.HasSuffix(path, string(os.PathSeparator)) {
			path += string(os.PathSeparator)
		}

		// Check if the path exists
		err := jm.pathExists(path)
		if err != nil {
			jm.lm.Log("error", err.Error(), "jobs")
			return err
		}

		// Compress output
		pathDir := filepath.Dir(path)
		rnd := filepath.Join(pathDir, utils.RandomString(32))

		err = utils.Compress(path, rnd)
		if err != nil {
			os.RemoveAll(rnd)
			jm.lm.Log("error", err.Error(), "jobs")
			return err
		}

		// Create output CID
		cid, err := utils.HashFileToCID(rnd)
		if err != nil {
			os.RemoveAll(rnd)
			jm.lm.Log("error", err.Error(), "jobs")
			return err
		}

		// Rename compressed file to CID
		cidSrcPath := filepath.Join(pathDir, cid)
		err = os.Rename(rnd, cidSrcPath)
		if err != nil {
			jm.lm.Log("error", err.Error(), "jobs")
			return err
		}

		// Send output
		host := jm.p2pm.h.ID().String()
		if interfacePeer.PeerNodeId == host {
			// Copy it to host job / local repo
			var fdir string
			if interfacePeer.PeerJobId == 0 {
				// Copy it to local repo
				fdir = filepath.Join(configs["local_storage"])
			} else {
				// Copy it to job
				fdir = filepath.Join(configs["local_storage"], "workflows", job.OrderingNodeId, strconv.FormatInt(job.WorkflowId, 10), "job", strconv.FormatInt(interfacePeer.PeerJobId, 10), "input", host)
			}
			fdir += string(os.PathSeparator)

			if err = os.MkdirAll(fdir, 0755); err != nil {
				jm.lm.Log("error", err.Error(), "jobs")
				return err
			}

			// Copy to destination folder
			cidDestPath := filepath.Join(fdir, cid)
			if err = utils.BufferFileCopy(cidSrcPath, cidDestPath, 48*1024); err != nil {
				jm.lm.Log("error", err.Error(), "jobs")
				return err
			}

			if interfacePeer.PeerJobId != 0 {
				// Uncompress received file
				err = utils.Uncompress(cidDestPath, fdir)
				if err != nil {
					jm.lm.Log("error", err.Error(), "jobs")
					return err
				}
				err = os.RemoveAll(cidDestPath)
				if err != nil {
					jm.lm.Log("error", err.Error(), "jobs")
					return err
				}
			}
		} else {
			// Send data to the requesting node
			p, err := jm.p2pm.GeneratePeerAddrInfo(interfacePeer.PeerNodeId)
			if err != nil {
				jm.lm.Log("error", err.Error(), "jobs")
				return err
			}

			// Open the file for reading
			file, err := os.Open(filepath.Join(pathDir, cid))
			if err != nil {
				jm.lm.Log("error", err.Error(), "jobs")
				return err
			}
			//	defer file.Close() // This must be done after streaming is finished

			// Set correct job id to deliver it to
			jobReplica := jm.createPeerJobReplica(job, intrface, interfacePeer, true)

			// Connect to peer and start streaming
			err = StreamData(jm.p2pm, p, file, &jobReplica, nil)
			if err != nil {
				jm.lm.Log("error", err.Error(), "jobs")
				return err
			}
		}
	}

	return nil
}

// Send job data if peer is eligible to get job data
func (jm *JobManager) SendIfPeerEligible(
	jobId int64,
	intrfaceId int64,
	peer peer.ID,
	dir string,
) error {
	var job node_types.Job
	var err error
	var eligible bool = false

	configManager := utils.NewConfigManager("")
	configs, err := configManager.ReadConfigs()
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	job, err = jm.GetJob(jobId)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return err
	}

	for _, intrface := range job.JobInterfaces {
		for _, interfacePeer := range intrface.JobInterfacePeers {
			if intrface.InterfaceId != intrfaceId {
				continue
			}
			if interfacePeer.PeerNodeId == peer.String() {
				var condition bool = false
				switch dir {
				case "INPUT":
					condition = interfacePeer.PeerMountFunction == "PROVIDER" || intrface.InterfaceType == "INPUT"
				case "OUTPUT":
					condition = interfacePeer.PeerMountFunction == "RECEIVER" || intrface.InterfaceType == "OUTPUT"
				default:
					return fmt.Errorf("unsupported job data direction `%s`", dir)
				}

				if condition {
					// Eligible
					eligible = true

					base := filepath.Join(configs["local_storage"], "workflows", job.OrderingNodeId, strconv.FormatInt(job.WorkflowId, 10))
					return jm.sendDataForPeerInterface(job, intrface, interfacePeer, base, configs, dir)
				}
			}
		}
	}

	if !eligible {
		return fmt.Errorf("interface peer `%s` is ineligible to collect job data (job Id: `%d`) from interface `%d` in direction `%s`",
			peer.String(), jobId, intrfaceId, dir)
	}

	return nil
}

// Create stripped job object replica for StreamData purposes
func (jm *JobManager) createPeerJobReplica(
	job node_types.Job,
	intrface node_types.JobInterface,
	interfacePeer node_types.JobInterfacePeer,
	usePeerJobId bool,
) node_types.Job {
	jobReplicaBase := node_types.JobBase{
		OrderingNodeId: job.OrderingNodeId,
		WorkflowId:     job.WorkflowId,
		Id:             job.Id,
	}
	if usePeerJobId && interfacePeer.PeerJobId != 0 {
		jobReplicaBase.Id = interfacePeer.PeerJobId
	}

	jobReplicaInterfaces := []node_types.JobInterface{
		{
			InterfaceId: intrface.InterfaceId,
			WorkflowId:  intrface.WorkflowId,
			JobId:       intrface.JobId,
			Interface:   intrface.Interface,
			JobInterfacePeers: []node_types.JobInterfacePeer{
				{
					PeerJobId:         interfacePeer.PeerJobId,
					PeerNodeId:        interfacePeer.PeerNodeId,
					PeerPath:          interfacePeer.PeerPath,
					PeerMountFunction: interfacePeer.PeerMountFunction,
					PeerDuty:          interfacePeer.PeerDuty,
				},
			},
		},
	}

	jobReplica := node_types.Job{
		JobBase:       jobReplicaBase,
		JobInterfaces: jobReplicaInterfaces,
	}

	return jobReplica
}

// Check are job inputs and mounts ready (also prepare output paths)
func (jm *JobManager) checkInterfaces(job node_types.Job) ([]string, []string, map[string]string, error) {
	var inputFiles []string
	var outputFiles []string
	var mounts = make(map[string]string)

	configManager := utils.NewConfigManager("")
	configs, err := configManager.ReadConfigs()
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return nil, nil, nil, err
	}

	for _, intrface := range job.JobInterfaces {
		basePath := filepath.Join(configs["local_storage"], "workflows", job.OrderingNodeId, strconv.FormatInt(job.WorkflowId, 10))
		switch intrface.InterfaceType {
		case "STDIN":
			inputFiles, err = jm.validateHostPaths(basePath, intrface, inputFiles)
			if err != nil {
				jm.lm.Log("error", err.Error(), "jobs")
				return nil, nil, nil, err
			}
		case "STDOUT":
			outputFiles, err = jm.createHostPaths(basePath, intrface, outputFiles)
			if err != nil {
				jm.lm.Log("error", err.Error(), "jobs")
				return nil, nil, nil, err
			}
		case "MOUNT":
			mounts, err = jm.createHostMountPoints(basePath, intrface, mounts)
			if err != nil {
				jm.lm.Log("error", err.Error(), "jobs")
				return nil, nil, nil, err
			}
		default:
			err := fmt.Errorf("unknown interface type `%s`", intrface.InterfaceType)
			jm.lm.Log("error", err.Error(), "jobs")
			return nil, nil, nil, err
		}
	}

	return inputFiles, outputFiles, mounts, nil
}

func (jm *JobManager) validateHostPaths(base string, inrfce node_types.JobInterface, files []string) ([]string, error) {
	var inOut string

	switch inrfce.InterfaceType {
	case "STDIN":
		inOut = "input"
	case "STDOUT":
		inOut = "output"
	default:
		err := fmt.Errorf("unsupported interface type %s", inrfce.InterfaceType)
		jm.lm.Log("error", err.Error(), "jobs")
		return nil, err
	}
	for _, interfacePeer := range inrfce.JobInterfacePeers {
		paths := strings.SplitSeq(interfacePeer.PeerPath, ",")
		for path := range paths {
			path := strings.TrimSpace(path)
			isDir := strings.HasSuffix(path, string(os.PathSeparator))

			path = filepath.Join(base, "job", strconv.FormatInt(inrfce.JobId, 10), inOut, interfacePeer.PeerNodeId, path)
			if isDir && !strings.HasSuffix(path, string(os.PathSeparator)) {
				path += string(os.PathSeparator)
			}

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
	}

	return files, nil
}

func (jm *JobManager) createHostPaths(base string, inrfce node_types.JobInterface, files []string) ([]string, error) {
	var err error
	var inOut string

	switch inrfce.InterfaceType {
	case "STDIN":
		inOut = "input"
	case "STDOUT":
		inOut = "output"
	default:
		err := fmt.Errorf("unsupported interface type %s", inrfce.InterfaceType)
		jm.lm.Log("error", err.Error(), "jobs")
		return nil, err
	}

	for _, interfacePeer := range inrfce.JobInterfacePeers {
		paths := strings.SplitSeq(interfacePeer.PeerPath, ",")
		for path := range paths {
			path := strings.TrimSpace(path)
			isDir := strings.HasSuffix(path, string(os.PathSeparator))

			path = filepath.Join(base, "job", strconv.FormatInt(inrfce.JobId, 10), inOut, interfacePeer.PeerNodeId, path)
			if isDir && !strings.HasSuffix(path, string(os.PathSeparator)) {
				path += string(os.PathSeparator)
			}

			// Make sure file path is created
			path, err = jm.createPath(path, isDir)
			if err != nil {
				return nil, err
			}

			files = append(files, path)
		}
	}

	return files, nil
}

func (jm *JobManager) createHostMountPoints(base string, inrfce node_types.JobInterface, mounts map[string]string) (map[string]string, error) {
	var err error

	// Which OS we are running host on
	osType := runtime.GOOS

	// Check if provided path is dir
	serviceMountPoint := strings.TrimSpace(inrfce.Path)
	isDir := strings.HasSuffix(serviceMountPoint, string(os.PathSeparator))

	// Make sure we have os specific path separator since we are adding this path to host's path
	switch osType {
	case "linux", "darwin":
		serviceMountPoint = filepath.ToSlash(serviceMountPoint)
	case "windows":
		serviceMountPoint = filepath.FromSlash(serviceMountPoint)
	default:
		err := fmt.Errorf("unsupported OS type `%s`", osType)
		return nil, err
	}

	// Make sure mount file path is created
	mountPath := filepath.Join(base, "job", strconv.FormatInt(inrfce.JobId, 10), "mounts", serviceMountPoint)

	if isDir && !strings.HasSuffix(mountPath, string(os.PathSeparator)) {
		mountPath += string(os.PathSeparator)
	}

	mountPath, err = jm.createPath(mountPath, isDir)
	if err != nil {
		return nil, err
	}

	// Check if mount path is file or folder
	// If it's a file and it doesn't exist at runtime
	// Docker will create it as a folder, so we must
	// create a file instead prior to execution
	var validator func(path string) error
	switch osType {
	case "linux", "darwin":
		validator = jm.vm.IsValidFileNameUnix
	case "windows":
		validator = jm.vm.IsValidFileNameWindows
	default:
		err := fmt.Errorf("unsupported OS type `%s`", osType)
		return nil, err
	}

	err = validator(mountPath)
	if err != nil {
		return nil, err
	} else {
		// Make sure path is not already a directory
		if fi, err := os.Stat(mountPath); err == nil && fi.IsDir() {
			err := fmt.Errorf("a directory exists at the file path: %s", mountPath)
			return nil, err
		}

		f, err := os.Create(mountPath)
		if err != nil {
			return nil, err
		}
		f.Close()
	}

	// Send back updated abs path
	mounts[mountPath] = inrfce.Path

	// Copy all inputs
	for _, interfacePeer := range inrfce.JobInterfacePeers {
		// Check peer's mount function
		if interfacePeer.PeerMountFunction != "PROVIDER" {
			continue
		}
		paths := strings.SplitSeq(interfacePeer.PeerPath, ",")
		for path := range paths {
			// Check if provided path is dir
			path := strings.TrimSpace(path)
			isDir := strings.HasSuffix(path, string(os.PathSeparator))

			path = filepath.Join(base, "job", strconv.FormatInt(inrfce.JobId, 10), "input", interfacePeer.PeerNodeId, path)
			if isDir && !strings.HasSuffix(path, string(os.PathSeparator)) {
				path += string(os.PathSeparator)
			}

			// Check if the path exists
			err := jm.pathExists(path)
			if err != nil {
				err = fmt.Errorf("job defined input path `%s` is missing (system error: %s)", path, err.Error())
				jm.lm.Log("error", err.Error(), "jobs")
				return nil, err
			}

			// Copy peer path to mount point
			err = utils.CopyPath(path, mountPath)
			if err != nil {
				err = fmt.Errorf("copy path `%s` to `%s` failed (system error: %s)",
					path, mountPath, err.Error())
				jm.lm.Log("error", err.Error(), "jobs")
				return nil, err
			}
		}
	}

	return mounts, nil
}

func (jm *JobManager) createPath(path string, isDir bool) (string, error) {
	// If it's a directory, ensure it ends with a slash for clarity (optional)
	if isDir && !strings.HasSuffix(path, string(os.PathSeparator)) {
		path += string(os.PathSeparator)
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return "", err
	}

	// If it's a directory, create it directly
	if isDir {
		if err := os.MkdirAll(absPath, 0755); err != nil {
			jm.lm.Log("error", err.Error(), "jobs")
			return "", err
		}
		return absPath + string(os.PathSeparator), nil
	}

	// If it's a file, create the parent directory
	dir := filepath.Dir(absPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		jm.lm.Log("error", err.Error(), "jobs")
		return "", err
	}

	return absPath, nil
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
