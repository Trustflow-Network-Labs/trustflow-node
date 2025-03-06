package shared

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/tfnode"
	"github.com/adgsm/trustflow-node/utils"
)

type JobManager struct {
	jobs map[int32]*node_types.Job
}

func NewJobManager() *JobManager {
	return &JobManager{
		jobs: make(map[int32]*node_types.Job),
	}
}

// Job exists?
func (jm *JobManager) JobExists(id int32) (error, bool) {
	if id <= 0 {
		msg := "invalid job id"
		utils.Log("error", msg, "jobs")
		return errors.New(msg), false
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "jobs")
		return err, false
	}
	defer db.Close()

	// Check if job is existing
	var jobId node_types.NullInt32
	row := db.QueryRowContext(context.Background(), "select id from jobs where id = ?;", id)

	err = row.Scan(&jobId)
	if err != nil {
		msg := err.Error()
		utils.Log("debug", msg, "jobs")
		return nil, false
	}

	return nil, true
}

// Get job by id
func (jm *JobManager) GetJob(id int32) (node_types.Job, error) {
	var job node_types.Job

	// Check if job exists in a queue
	err, exists := jm.JobExists(id)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "jobs")
		return job, err
	}

	if !exists {
		msg := fmt.Sprintf("Job %d does not exists in a queue", id)
		utils.Log("error", msg, "jobs")
		return job, err
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "jobs")
		return job, err
	}
	defer db.Close()

	// Get job
	row := db.QueryRowContext(context.Background(), "select id, ordering_node_id, service_id, status, started, ended from jobs where id = ?;", id)

	err = row.Scan(&job)
	if err != nil {
		msg := err.Error()
		utils.Log("debug", msg, "jobs")
		return job, err
	}

	jm.jobs[job.Id] = &job

	return job, nil
}

// Get jobs by service ID
func (jm *JobManager) GetJobsByServiceId(serviceId int32, params ...uint32) ([]node_types.Job, error) {
	var job node_types.Job
	var jobs []node_types.Job
	if serviceId <= 0 {
		msg := "invalid service ID"
		utils.Log("error", msg, "jobs")
		return jobs, errors.New(msg)
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "jobs")
		return jobs, err
	}
	defer db.Close()

	var offset uint32 = 0
	var limit uint32 = 10
	if len(params) == 1 {
		offset = params[0]
	} else if len(params) >= 2 {
		offset = params[0]
		limit = params[1]
	}

	// Search for jobs
	rows, err := db.QueryContext(context.Background(), "select id, ordering_node_id, service_id, status, datetime(started), datetime(ended) from jobs where service_id = ? limit ? offset ?;",
		serviceId, limit, offset)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "jobs")
		return jobs, err
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&job)
		if err != nil {
			msg := err.Error()
			utils.Log("error", msg, "jobs")
			return jobs, err
		}
		jobs = append(jobs, job)
		jm.jobs[job.Id] = &job
	}

	return jobs, nil
}

// Change job status
func (jm *JobManager) UpdateJobStatus(id int32, status string) error {
	// Check if job exists in a queue
	err, exists := jm.JobExists(id)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "jobs")
		return err
	}

	if !exists {
		msg := fmt.Sprintf("Job %d does not exists in a queue", id)
		utils.Log("error", msg, "jobs")
		return err
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "jobs")
		return err
	}
	defer db.Close()

	// Update job status
	_, err = db.ExecContext(context.Background(), "update jobs set status = ? where id = ?);",
		status, id)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "jobs")
		return err
	}

	if _, exists := jm.jobs[id]; exists {
		jm.jobs[id].Status = status
	}

	return nil
}

// Create new job
func (jm *JobManager) CreateJob(orderingNodeId int32, serviceId int32) {
	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "jobs")
		return
	}
	defer db.Close()

	// Create new job
	utils.Log("debug", fmt.Sprintf("create job from ordering node id %d using service id %d", orderingNodeId, serviceId), "jobs")

	result, err := db.ExecContext(context.Background(), "insert into jobs (ordering_node_id, service_id) values (?, ?);",
		orderingNodeId, serviceId)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "jobs")
		return
	}

	id, err := result.LastInsertId()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "jobs")
		return
	}

	job := node_types.Job{
		Id:             int32(id),
		OrderingNodeId: orderingNodeId,
		ServiceId:      serviceId,
		Status:         "IDLE",
	}

	jm.jobs[int32(id)] = &job
}

// Run job from a queue
func (jm *JobManager) RunJob(jobId int32) error {
	// Get job from a queue
	job, err := jm.GetJob(jobId)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "jobs")
		return err
	}

	// Check job status
	status := job.Status

	switch status {
	case "IDLE":
		manager := NewWorkerManager()
		err := manager.StartWorker(jobId, job)
		if err != nil {
			// Stop worker
			serr := manager.StopWorker(jobId)
			if serr != nil {
				msg := serr.Error()
				utils.Log("error", msg, "jobs")
				return err
			}

			// Log error
			msg := err.Error()
			utils.Log("error", msg, "jobs")
			return err
		}
		// Set job status to RUNNING
		err = jm.UpdateJobStatus(jobId, "RUNNING")
		if err != nil {
			msg := err.Error()
			utils.Log("error", msg, "jobs")
			return err
		}
		return nil
	case "RUNNING":
		msg := fmt.Sprintf("Job id %d is %s", job.Id, status)
		utils.Log("error", msg, "jobs")
		return err
	case "CANCELLED":
		msg := fmt.Sprintf("Job id %d has been already executed with status %s. Create a new job to execute it", job.Id, status)
		utils.Log("error", msg, "jobs")
		return err
	case "ERRORED":
		msg := fmt.Sprintf("Job id %d has been already executed with status %s. Create a new job to execute it", job.Id, status)
		utils.Log("error", msg, "jobs")
		return err
	case "FINISHED":
		msg := fmt.Sprintf("Job id %d has been already executed with status %s. Create a new job to execute it", job.Id, status)
		utils.Log("error", msg, "jobs")
		return err
	default:
		msg := fmt.Sprintf("Unknown job status %s", status)
		utils.Log("error", msg, "jobs")
		return err
	}
}

func (jm *JobManager) StartJob(job node_types.Job) error {
	// Check underlaying service
	utils.Log("debug", fmt.Sprintf("checking job's underlaying service id %d", job.ServiceId), "jobs")

	serviceManager := NewServiceManager()
	service, err := serviceManager.GetService(job.ServiceId)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "jobs")
		return err
	}

	// Check if service is active
	if !service.Active {
		msg := fmt.Sprintf("Service id %d is inactive", service.Id)
		utils.Log("error", msg, "jobs")
		return err
	}

	// Determine service type
	serviceType := service.Type

	switch serviceType {
	case "DATA":
		err := jm.StreamDataJob(job)
		if err != nil {
			msg := err.Error()
			utils.Log("error", msg, "jobs")
			return err
		}
	case "DOCKER EXECUTION ENVIRONMENT":
	case "WASM EXECUTION ENVIRONMENT":
	default:
		msg := fmt.Sprintf("Unknown service type %s", serviceType)
		utils.Log("error", msg, "jobs")
		return err
	}

	// Run a job
	jobId := job.Id
	utils.Log("debug", fmt.Sprintf("start running job id %d", jobId), "jobs")

	return nil
}

func (jm *JobManager) StreamDataJob(job node_types.Job) error {
	// Check if node is running
	if running := IsHostRunning(); !running {
		msg := "node is not running"
		err := errors.New(msg)
		utils.Log("error", msg, "jobs")
		return err
	}

	// Get data source path
	serviceManager := NewServiceManager()
	service, err := serviceManager.GetService(job.ServiceId)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "jobs")
		return err
	}

	// Check if the file exists
	_, err = os.Stat(service.Path)
	if os.IsNotExist(err) {
		msg := "file does not exist"
		err := errors.New(msg)
		utils.Log("error", msg, "jobs")
		return err
	} else if err != nil {
		// Handle other potential errors
		utils.Log("error", err.Error(), "jobs")
		return err
	}

	// Open the file for reading
	file, err := os.Open(service.Path)
	if err != nil {
		utils.Log("error", err.Error(), "jobs")
		return err
	}
	defer file.Close() // Ensure the file is closed after operations

	// Get ordering node peer ID
	orderingNode, err := tfnode.FindNodeById(job.OrderingNodeId)
	if err != nil {
		utils.Log("error", err.Error(), "jobs")
		return err
	}

	// Get peer
	p, err := GeneratePeerFromId(orderingNode.NodeId)
	if err != nil {
		utils.Log("error", err.Error(), "jobs")
		return err
	}

	// Connect to peer and start streaming
	err = StreamData(p, file)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "jobs")
		return err
	}

	return nil
}
