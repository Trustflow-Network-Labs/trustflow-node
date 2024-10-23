package cmd_helpers

import (
	"context"
	"errors"
	"fmt"

	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/utils"
)

// Get jobs by service ID
func GetJobsByServiceId(serviceId int32, params ...uint32) ([]node_types.Job, error) {
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
	}

	return jobs, nil
}

// Create new job
func CreateJob(orderingNodeId int32, serviceId int32) {
	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "services")
		return
	}
	defer db.Close()

	// Create new job
	utils.Log("debug", fmt.Sprintf("create job from ordering node id %d using service id %d", orderingNodeId, serviceId), "services")

	_, err = db.ExecContext(context.Background(), "insert into jobs (ordering_node_id, service_id) values (?, ?);",
		orderingNodeId, serviceId)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "services")
		return
	}
}
