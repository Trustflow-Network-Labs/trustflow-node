package resource_utilization

import (
	"context"
	"errors"

	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/utils"
)

// Get utilizations by resource ID
func GetUtilizationsByResourceId(resourceId int32, params ...uint32) ([]node_types.ResourceUtilization, error) {
	var utilization node_types.ResourceUtilization
	var utilizations []node_types.ResourceUtilization
	if resourceId <= 0 {
		msg := "invalid resource ID"
		utils.Log("error", msg, "utilizations")
		return utilizations, errors.New(msg)
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "utilizations")
		return utilizations, err
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

	// Search for resource utilizations
	rows, err := db.QueryContext(context.Background(), "select id, job_id, resource_id, utilization, timestamp from resources_utilizations where resource_id = ? limit ? offset ?;",
		resourceId, limit, offset)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "utilizations")
		return utilizations, err
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&utilization)
		if err != nil {
			msg := err.Error()
			utils.Log("error", msg, "utilizations")
			return utilizations, err
		}
		utilizations = append(utilizations, utilization)
	}

	return utilizations, nil
}
