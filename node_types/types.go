package node_types

import (
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// Declare key type
type Key struct {
	Id         int64  `json:"id"`
	Identifier string `json:"identifier"`
	Algorithm  string `json:"algorithm"`
	Key        []byte `json:"key"`
}

// Declare blacklist type
type BlacklistSQL struct {
	NodeId    string     `json:"node_id"`
	Reason    NullString `json:"reason"`
	Timestamp string     `json:"timestamp"`
}
type Blacklist struct {
	NodeId    peer.ID   `json:"node_id"`
	Reason    string    `json:"reason"`
	Timestamp time.Time `json:"timestamp"`
}

// Declare stream data type
type StreamData struct {
	Type   uint16
	PeerId [255]byte
}

// Declare currency type
type Currency struct {
	Symbol   string `json:"symbol"`
	Currency string `json:"currency"`
}

// Declare price type
type Price struct {
	Id         int64   `json:"id"`
	ServiceId  int64   `json:"service_id"`
	ResourceId int64   `json:"resource_id"`
	Price      float64 `json:"price"`
	Currency   string  `json:"currency"`
}

// Declare resource type
type Resource struct {
	Id            int64      `json:"id"`
	ResourceGroup string     `json:"resource_group"`
	Resource      string     `json:"resource"`
	ResourceUnit  string     `json:"resource_unit"`
	Description   NullString `json:"description"`
	Active        bool       `json:"active"`
}

// Declare resource utilization type
type ResourceUtilization struct {
	Id          int64   `json:"id"`
	JobId       int64   `json:"job_id"`
	ResourceId  int64   `json:"resource_id"`
	Utilization float64 `json:"utilization"`
	Timestamp   string  `json:"timestamp"`
}

// Declare service type
type Service struct {
	Id          int64  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"`
	Active      bool   `json:"active"`
}

// Declare data service type
type DataService struct {
	Id        int64  `json:"id"`
	ServiceId int64  `json:"service_id"`
	Path      string `json:"path"`
}

// Declare docker service type
type DockerService struct {
	Id                 int64                `json:"id"`
	ServiceId          int64                `json:"service_id"`
	Repo               string               `json:"repo"`
	RepoDockerFiles    []string             `json:"repo_docker_files"`
	RepoDockerComposes []string             `json:"repo_docker_composes"`
	Images             []DockerServiceImage `json:"images"`
}

// Declare docker image type
type DockerServiceImage struct {
	Id               int64                         `json:"id"`
	ServiceDetailsId int64                         `json:"service_details_id"`
	ImageId          int64                         `json:"image_id"`
	ImageName        string                        `json:"image_name"`
	ImageTags        []string                      `json:"image_tags"`
	ImageDigests     []string                      `json:"image_digests"`
	Intefaces        []DockerServiceImageInterface `json:"interfaces"`
}

// Declare docker image type
type DockerServiceImageInterface struct {
	Id                  int64  `json:"id"`
	ServiceImageId      int64  `json:"service_image_id"`
	InterfaceType       string `json:"interface_type"`
	FunctionalInterface string `json:"functional_interface"`
	Description         string `json:"description"`
	Path                string `json:"path"`
}

// Declare executable service type
type ExecutableService struct {
	Id        int64  `json:"id"`
	ServiceId int64  `json:"service_id"`
	Path      string `json:"path"`
}

// Declare search local service type
type SearchService struct {
	Phrases string `json:"phrases"`
	Type    string `json:"type"`
	Active  bool   `json:"active"`
}

// Declare remote service lookup type
type ServiceLookup struct {
	Phrases string `json:"phrases"`
	Type    string `json:"type"`
}

// Declare service price model type
type ServiceResourcesWithPricing struct {
	ResourceGroup       string     `json:"resource_group"`
	ResourceName        string     `json:"resource_name"`
	ResourceUnit        string     `json:"resource_unit"`
	ResourceDescription NullString `json:"resource_description"`
	Price               float64    `json:"price"`
	CurrencyName        string     `json:"currency_name"`
	CurrencySymbol      string     `json:"currency_symbol"`
}

// Declare interface base struct
type Interface struct {
	NodeId              string `json:"node_id"`
	InterfaceType       string `json:"interface_type"`
	FunctionalInterface string `json:"functional_interface"`
	Description         string `json:"description"`
	Path                string `json:"path"`
}

// Declare service request type
type ServiceRequest struct {
	NodeId                    string      `json:"node_id"`
	WorkflowId                int64       `json:"workflow_id"`
	ServiceId                 int64       `json:"service_id"`
	Interfaces                []Interface `json:"interfaces"`
	ExecutionConstraint       string      `json:"execution_constraint"`
	ExecutionConstraintDetail string      `json:"execution_constraint_detail"`
}

// Declare response type for a service request
type ServiceResponse struct {
	JobId          int64  `json:"job_id"`
	Accepted       bool   `json:"accepted"`
	Message        string `json:"message"`
	OrderingNodeId string `json:"ordering_node_id"`
	ServiceRequest
}

// Declare service offer type
type ServiceOffer struct {
	Id                int64                         `json:"id"`
	Name              string                        `json:"name"`
	Description       string                        `json:"description"`
	NodeId            string                        `json:"node_id"`
	Type              string                        `json:"type"`
	Active            bool                          `json:"active"`
	ServicePriceModel []ServiceResourcesWithPricing `json:"service_price_model"`
}

// Declare workflow struct
type Workflow struct {
	Id          int64         `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Jobs        []WorkflowJob `json:"jobs"`
}

// Declare workflow job struct
type WorkflowJob struct {
	Id                 int64  `json:"id"`
	WorkflowId         int64  `json:"workflow_id"`
	NodeId             string `json:"node_id"`
	JobId              int64  `json:"job_id"`
	ExpectedJobOutputs string `json:"expected_job_outputs"`
	Status             string `json:"status"`
}

// Declare job base struct
type JobBase struct {
	Id                        int64  `json:"id"`
	WorkflowId                int64  `json:"workflow_id"`
	ServiceId                 int64  `json:"service_id"`
	OrderingNodeId            string `json:"ordering_node_id"`
	ExecutionConstraint       string `json:"execution_constraint"`
	ExecutionConstraintDetail string `json:"execution_constraint_detail"`
	Status                    string `json:"status"`
}

// Declare job type
type Job struct {
	JobBase
	JobInterfaces []JobInterface `json:"job_interfaces"`
	Started       time.Time      `json:"started"`
	Ended         time.Time      `json:"ended"`
}

// Declare job sql type
type JobSql struct {
	JobBase
	Started string `json:"started"`
	Ended   string `json:"ended"`
}

// Declare job interfaces base struct
type JobInterface struct {
	InterfaceId int64 `json:"interface_id"`
	JobId       int64 `json:"job_id"`
	Interface
}

const timeLayout = time.RFC3339

func (js *JobSql) ToJob() Job {
	var started, ended time.Time

	if js.Started != "" {
		started, _ = time.Parse(timeLayout, js.Started)
	}
	if js.Ended != "" {
		ended, _ = time.Parse(timeLayout, js.Ended)
	}

	return Job{
		JobBase: js.JobBase,
		Started: started,
		Ended:   ended,
	}
}

func (j *Job) ToJobSql() JobSql {
	var started, ended string

	if !j.Started.IsZero() {
		started = j.Started.Format(timeLayout)
	}
	if !j.Ended.IsZero() {
		ended = j.Ended.Format(timeLayout)
	}
	return JobSql{
		JobBase: j.JobBase,
		Started: started,
		Ended:   ended,
	}
}

// Declare job run request type
type JobRunRequest struct {
	WorkflowId int64  `json:"workflow_id"`
	NodeId     string `json:"node_id"`
	JobId      int64  `json:"job_id"`
}

// Declare job run response type for a job run request
type JobRunResponse struct {
	Accepted bool   `json:"accepted"`
	Message  string `json:"message"`
	JobRunRequest
}

// Declare job run status request type
type JobRunStatusRequest struct {
	WorkflowId int64  `json:"workflow_id"`
	NodeId     string `json:"node_id"`
	JobId      int64  `json:"job_id"`
}

// Declare job run status type
type JobRunStatus struct {
	JobRunStatusRequest
	Status string `json:"status"`
}

// Declare docker image type
type DockerImage struct {
	Id      string    `json:"id"`
	Name    string    `json:"name"`
	Tags    []string  `json:"tags"`
	Digests []string  `json:"digests"`
	BuiltAt time.Time `json:"built_at"`
}

// Declare docker image with Interfaces type
type DockerImageWithInterfaces struct {
	DockerImage
	Interfaces []Interface `json:"interfaces"`
}
