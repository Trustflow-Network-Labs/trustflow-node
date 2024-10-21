package cmd_helpers

import (
	"context"
	"errors"

	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/utils"
)

// Get services by service type ID
func GetServicesByServiceTypeId(serviceTypeId int32, params ...uint32) ([]node_types.Service, error) {
	var service node_types.Service
	var services []node_types.Service
	if serviceTypeId <= 0 {
		msg := "invalid service type ID"
		utils.Log("error", msg, "services")
		return services, errors.New(msg)
	}

	// Create a database connection
	db, err := database.CreateConnection()
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "services")
		return services, err
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

	// Search for services
	rows, err := db.QueryContext(context.Background(), "select id, name, description, node_id, service_type_id from services where service_type_id = ? limit ? offset ?;",
		serviceTypeId, limit, offset)
	if err != nil {
		msg := err.Error()
		utils.Log("error", msg, "services")
		return services, err
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&service)
		if err != nil {
			msg := err.Error()
			utils.Log("error", msg, "services")
			return services, err
		}
		services = append(services, service)
	}

	return services, nil
}
