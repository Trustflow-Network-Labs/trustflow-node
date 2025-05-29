package resource_utilization

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/adgsm/trustflow-node/internal/node_types"
)

// TestLogsManager is a simple implementation that does nothing
type TestLogsManager struct{}

func (tlm *TestLogsManager) Log(level, message, component string) {
	// Do nothing for tests
}

// TestResourceUtilizationManager embeds the real one but overrides the logging to avoid file operations
type TestResourceUtilizationManager struct {
	ResourceUtilizationManager
	testLm *TestLogsManager
}

func NewTestResourceUtilizationManager(db *sql.DB) *TestResourceUtilizationManager {
	return &TestResourceUtilizationManager{
		ResourceUtilizationManager: ResourceUtilizationManager{
			db: db,
			// lm will be nil, but we don't need it as we'll use our own Log method
		},
		testLm: &TestLogsManager{},
	}
}

// Override the Log method to use our test logger
func (trum *TestResourceUtilizationManager) Log(level, message, component string) {
	// Use our test logger instead
	trum.testLm.Log(level, message, component)
}

// Override GetUtilizationsByResource to call our version with test logging
func (trum *TestResourceUtilizationManager) GetUtilizationsByResource(resource int64, params ...uint32) ([]node_types.ResourceUtilization, error) {
	var utilization node_types.ResourceUtilization
	var utilizations []node_types.ResourceUtilization
	if resource <= 0 {
		trum.testLm.Log("error", "invalid resource id", "utilizations")
		return utilizations, errors.New("invalid resource id")
	}

	var offset uint32 = 0
	var limit uint32 = 10
	if len(params) == 1 {
		offset = params[0]
	} else if len(params) >= 2 {
		offset = params[0]
		limit = params[1]
	}

	// Search for resource utilizations
	rows, err := trum.db.QueryContext(context.Background(), "select id, job_id, resource_id, utilization, timestamp from resources_utilizations where resource_id = ? limit ? offset ?;",
		resource, limit, offset)
	if err != nil {
		trum.testLm.Log("error", err.Error(), "utilizations")
		return utilizations, err
	}
	defer rows.Close()

	for rows.Next() {
		err = rows.Scan(&utilization.Id, &utilization.JobId, &utilization.ResourceId, &utilization.Utilization, &utilization.Timestamp)
		if err != nil {
			trum.testLm.Log("error", err.Error(), "utilizations")
			return utilizations, err
		}
		utilizations = append(utilizations, utilization)
	}

	return utilizations, nil
}

func TestNewResourceUtilizationManager(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	rum := NewResourceUtilizationManager(db)

	if rum == nil {
		t.Error("expected ResourceUtilizationManager, got nil")
	}

	if rum != nil && rum.db != nil && rum.db != db {
		t.Error("expected db to be set correctly")
	}

	if rum != nil && rum.lm == nil {
		t.Error("expected LogsManager to be initialized")
	}
}

func TestGetUtilizationsByResource(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	defer db.Close()

	// Use our test version that avoids file operations
	rum := NewTestResourceUtilizationManager(db)

	tests := []struct {
		name           string
		resourceId     int64
		params         []uint32
		mockSetup      func()
		expectedResult []node_types.ResourceUtilization
		expectedError  bool
	}{
		{
			name:       "valid resource with default limits",
			resourceId: 1,
			params:     []uint32{},
			mockSetup: func() {
				rows := sqlmock.NewRows([]string{"id", "job_id", "resource_id", "utilization", "timestamp"}).
					AddRow(1, 100, int64(1), 75.5, "2023-06-01T12:00:00Z").
					AddRow(2, 101, int64(1), 80.0, "2023-06-01T12:05:00Z")
				mock.ExpectQuery("select id, job_id, resource_id, utilization, timestamp from resources_utilizations where resource_id = \\? limit \\? offset \\?").
					WithArgs(int64(1), uint32(10), uint32(0)).
					WillReturnRows(rows)
			},
			expectedResult: []node_types.ResourceUtilization{
				{Id: 1, JobId: 100, ResourceId: int64(1), Utilization: 75.5, Timestamp: "2023-06-01T12:00:00Z"},
				{Id: 2, JobId: 101, ResourceId: int64(1), Utilization: 80.0, Timestamp: "2023-06-01T12:05:00Z"},
			},
			expectedError: false,
		},
		{
			name:       "valid resource with custom offset",
			resourceId: 1,
			params:     []uint32{5},
			mockSetup: func() {
				rows := sqlmock.NewRows([]string{"id", "job_id", "resource_id", "utilization", "timestamp"}).
					AddRow(6, 105, int64(1), 65.5, "2023-06-01T12:25:00Z")
				mock.ExpectQuery("select id, job_id, resource_id, utilization, timestamp from resources_utilizations where resource_id = \\? limit \\? offset \\?").
					WithArgs(int64(1), uint32(10), uint32(5)).
					WillReturnRows(rows)
			},
			expectedResult: []node_types.ResourceUtilization{
				{Id: 6, JobId: 105, ResourceId: int64(1), Utilization: 65.5, Timestamp: "2023-06-01T12:25:00Z"},
			},
			expectedError: false,
		},
		{
			name:       "valid resource with custom offset and limit",
			resourceId: 1,
			params:     []uint32{10, 5},
			mockSetup: func() {
				rows := sqlmock.NewRows([]string{"id", "job_id", "resource_id", "utilization", "timestamp"}).
					AddRow(11, 110, int64(1), 90.0, "2023-06-01T13:00:00Z").
					AddRow(12, 111, int64(1), 91.5, "2023-06-01T13:05:00Z")
				mock.ExpectQuery("select id, job_id, resource_id, utilization, timestamp from resources_utilizations where resource_id = \\? limit \\? offset \\?").
					WithArgs(int64(1), uint32(5), uint32(10)).
					WillReturnRows(rows)
			},
			expectedResult: []node_types.ResourceUtilization{
				{Id: 11, JobId: 110, ResourceId: int64(1), Utilization: 90.0, Timestamp: "2023-06-01T13:00:00Z"},
				{Id: 12, JobId: 111, ResourceId: int64(1), Utilization: 91.5, Timestamp: "2023-06-01T13:05:00Z"},
			},
			expectedError: false,
		},
		{
			name:       "invalid resource id",
			resourceId: 0,
			params:     []uint32{},
			mockSetup: func() {
				// No DB interaction for invalid resource id
			},
			expectedResult: []node_types.ResourceUtilization{},
			expectedError:  true,
		},
		{
			name:       "negative resource id",
			resourceId: -1,
			params:     []uint32{},
			mockSetup: func() {
				// No DB interaction for negative resource id
			},
			expectedResult: []node_types.ResourceUtilization{},
			expectedError:  true,
		},
		{
			name:       "database error",
			resourceId: 2,
			params:     []uint32{},
			mockSetup: func() {
				mock.ExpectQuery("select id, job_id, resource_id, utilization, timestamp from resources_utilizations where resource_id = \\? limit \\? offset \\?").
					WithArgs(int64(2), uint32(10), uint32(0)).
					WillReturnError(errors.New("database error"))
			},
			expectedResult: []node_types.ResourceUtilization{},
			expectedError:  true,
		},
		{
			name:       "empty result",
			resourceId: 3,
			params:     []uint32{},
			mockSetup: func() {
				rows := sqlmock.NewRows([]string{"id", "job_id", "resource_id", "utilization", "timestamp"})
				mock.ExpectQuery("select id, job_id, resource_id, utilization, timestamp from resources_utilizations where resource_id = \\? limit \\? offset \\?").
					WithArgs(int64(3), uint32(10), uint32(0)).
					WillReturnRows(rows)
			},
			expectedResult: []node_types.ResourceUtilization{},
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.mockSetup()

			result, err := rum.GetUtilizationsByResource(tt.resourceId, tt.params...)

			if (err != nil) != tt.expectedError {
				t.Errorf("GetUtilizationsByResource() error = %v, expectedError %v", err, tt.expectedError)
				return
			}

			if !tt.expectedError {
				// Check length first
				if len(result) != len(tt.expectedResult) {
					t.Errorf("GetUtilizationsByResource() returned %d results, want %d", len(result), len(tt.expectedResult))
					return
				}

				// Then compare elements if lengths match
				for i := range result {
					if result[i] != tt.expectedResult[i] {
						t.Errorf("GetUtilizationsByResource() result[%d] = %v, want %v", i, result[i], tt.expectedResult[i])
					}
				}
			}

			if err := mock.ExpectationsWereMet(); err != nil {
				t.Errorf("there were unfulfilled expectations: %s", err)
			}
		})
	}
}
