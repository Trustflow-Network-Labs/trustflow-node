package resource_utilization

import (
	"context"
	"errors"

	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/utils"
)

type ResourceUtilizationManager struct {
	sm *database.SQLiteManager
	lm *utils.LogsManager
}

func NewResourceUtilizationManager() *ResourceUtilizationManager {
	return &ResourceUtilizationManager{
		sm: database.NewSQLiteManager(),
		lm: utils.NewLogsManager(),
	}
}

// Get utilizations by resource
func (rum *ResourceUtilizationManager) GetUtilizationsByResource(resource string, params ...uint32) ([]node_types.ResourceUtilization, error) {
	var utilization node_types.ResourceUtilization
	var utilizations []node_types.ResourceUtilization
	if resource == "" {
		msg := "invalid resource name"
		rum.lm.Log("error", msg, "utilizations")
		return utilizations, errors.New(msg)
	}

	// Create a database connection
	db, err := rum.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		rum.lm.Log("error", msg, "utilizations")
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
	rows, err := db.QueryContext(context.Background(), "select id, job_id, resource, utilization, timestamp from resources_utilizations where resource = ? limit ? offset ?;",
		resource, limit, offset)
	if err != nil {
		msg := err.Error()
		rum.lm.Log("error", msg, "utilizations")
		return utilizations, err
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&utilization)
		if err != nil {
			msg := err.Error()
			rum.lm.Log("error", msg, "utilizations")
			return utilizations, err
		}
		utilizations = append(utilizations, utilization)
	}

	return utilizations, nil
}
