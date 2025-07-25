package node_types

import (
	"fmt"
	"sync"
	"time"

	"github.com/adgsm/trustflow-node/internal/utils"
	"github.com/google/shlex"
	"github.com/libp2p/go-libp2p/core/peer"
)

const timeLayout = time.RFC3339

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
	Type           uint16
	PeerId         [255]byte
	OrderingPeerId [255]byte
	WorkflowId     int64
	JobId          int64
	InterfaceId    int64
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
	Remote             string               `json:"remote"`
	Branch             string               `json:"branch"`
	Username           string               `json:"username"`
	Token              string               `json:"token"`
	RepoDockerFiles    []string             `json:"repo_docker_files"`
	RepoDockerComposes []string             `json:"repo_docker_composes"`
	Images             []DockerServiceImage `json:"images"`
}

// Declare docker image type
type DockerServiceImage struct {
	Id               int64                         `json:"id"`
	ServiceDetailsId int64                         `json:"service_details_id"`
	ImageId          string                        `json:"image_id"`
	ImageName        string                        `json:"image_name"`
	ImageEntryPoints []string                      `json:"image_entry_points"`
	ImageCommands    []string                      `json:"image_commands"`
	ImageTags        []string                      `json:"image_tags"`
	ImageDigests     []string                      `json:"image_digests"`
	ImageOs          string                        `json:"image_os"`
	Timestamp        time.Time                     `json:"timestamp"`
	Intefaces        []DockerServiceImageInterface `json:"interfaces"`
}

// Declare docker image type
type DockerServiceImageInterface struct {
	Id             int64 `json:"id"`
	ServiceImageId int64 `json:"service_image_id"`
	Interface
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
	InterfaceType string `json:"interface_type"`
	Description   string `json:"description"`
	Path          string `json:"path"`
}

// Declare request interface base struct
type RequestInterface struct {
	JobInterfacePeers []JobInterfacePeer `json:"job_interface_peers"`
	Interface
}

// Declare service request type
type ServiceRequest struct {
	NodeId                    string             `json:"node_id"`
	WorkflowId                int64              `json:"workflow_id"`
	WorkflowJobId             int64              `json:"workflow_job_id"`
	ServiceId                 int64              `json:"service_id"`
	Entrypoint                []string           `json:"entrypoint"`
	Commands                  []string           `json:"commands"`
	Interfaces                []RequestInterface `json:"interfaces"`
	ExecutionConstraint       string             `json:"execution_constraint"`
	ExecutionConstraintDetail string             `json:"execution_constraint_detail"`
}

// Declare response type for a service request
type ServiceResponse struct {
	JobId          int64  `json:"job_id"`
	Accepted       bool   `json:"accepted"`
	Message        string `json:"message"`
	OrderingNodeId string `json:"ordering_node_id"`
	ServiceRequest
}

// Declare service request cancellation type
type ServiceRequestCancellation struct {
	NodeId        string `json:"node_id"`
	JobId         int64  `json:"job_id"`
	WorkflowJobId int64  `json:"workflow_job_id"`
}

// Declare response type for a service request cancellation
type ServiceResponseCancellation struct {
	JobId          int64  `json:"job_id"`
	Accepted       bool   `json:"accepted"`
	Message        string `json:"message"`
	OrderingNodeId string `json:"ordering_node_id"`
	ServiceRequestCancellation
}

// Declare service offer type
type ServiceOffer struct {
	Service
	NodeId            string                        `json:"node_id"`
	Interfaces        []Interface                   `json:"interfaces"`
	Entrypoint        []string                      `json:"entrypoint"`
	Commands          []string                      `json:"commands"`
	ServicePriceModel []ServiceResourcesWithPricing `json:"service_price_model"`
	LastSeen          time.Time                     `json:"last_seen"`
}

type ServiceOffersCache struct {
	sync.Mutex
	ServiceOffers map[string]ServiceOffer // key = PeerID + "-" + ServiceId
}

func NewServiceOffersCache() *ServiceOffersCache {
	return &ServiceOffersCache{
		ServiceOffers: make(map[string]ServiceOffer),
	}
}

func (sc *ServiceOffersCache) AddOrUpdate(serviceOffer ServiceOffer) ServiceOffer {
	sc.Lock()
	defer sc.Unlock()
	key := fmt.Sprintf("%s-%d", serviceOffer.NodeId, serviceOffer.Id)
	serviceOffer.LastSeen = time.Now()
	sc.ServiceOffers[key] = serviceOffer
	return serviceOffer
}

func (sc *ServiceOffersCache) PruneExpired(ttl time.Duration) {
	sc.Lock()
	defer sc.Unlock()
	now := time.Now()
	for key, service := range sc.ServiceOffers {
		if now.Sub(service.LastSeen) > ttl {
			delete(sc.ServiceOffers, key)
		}
	}
}

// Declare workflow struct
type Workflow struct {
	Id          int64         `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Jobs        []WorkflowJob `json:"jobs"`
}

// Declare workflow job base struct
type WorkflowJobBase struct {
	Id                 int64                         `json:"id"`
	WorkflowId         int64                         `json:"workflow_id"`
	NodeId             string                        `json:"node_id"`
	ServiceId          int64                         `json:"service_id"`
	ServiceName        string                        `json:"service_name"`
	ServiceDescription string                        `json:"service_description"`
	ServiceType        string                        `json:"service_type"`
	JobId              int64                         `json:"job_id"`
	ExpectedJobOutputs string                        `json:"expected_job_outputs"`
	Status             string                        `json:"status"`
	ServiceInterfaces  []ServiceInterface            `json:"service_interfaces"`
	ServicePriceModel  []ServiceResourcesWithPricing `json:"service_price_model"`
}

// Declare service interface base struct
type ServiceInterface struct {
	InterfaceId           int64                  `json:"interface_id"`
	WorkflowJobId         int64                  `json:"workflow_job_id"`
	ServiceId             int64                  `json:"service_id"`
	ServiceInterfacePeers []ServiceInterfacePeer `json:"service_interface_peers"`
	Interface
}

// Declare service interface peer struct
type ServiceInterfacePeer struct {
	PeerServiceId     int64  `json:"peer_service_id"`
	PeerNodeId        string `json:"peer_node_id"`
	PeerPath          string `json:"peer_path"`
	PeerMountFunction string `json:"peer_mount_function"`
	PeerDuty          int64  `json:"peer_duty"`
}

// Declare workflow job struct
type WorkflowJob struct {
	WorkflowJobBase WorkflowJobBase `json:"workflow_job_base"`
	Entrypoint      []string        `json:"entrypoint"`
	Commands        []string        `json:"commands"`
	LastSeen        time.Time       `json:"last_seen"`
}

// Declare workflow job sql struct
type WorkflowJobSql struct {
	WorkflowJobBase WorkflowJobBase `json:"workflow_job_base"`
	Entrypoint      string          `json:"entrypoint"`
	Commands        string          `json:"commands"`
	LastSeen        string          `json:"last_seen"`
}

func (wjs *WorkflowJobSql) ToWorkflowJob() WorkflowJob {
	var lastSeen time.Time

	entrypoint, _ := shlex.Split(wjs.Entrypoint)
	commands, _ := shlex.Split(wjs.Commands)

	if wjs.LastSeen != "" {
		lastSeen, _ = time.Parse(timeLayout, wjs.LastSeen)
	}

	return WorkflowJob{
		WorkflowJobBase: wjs.WorkflowJobBase,
		Entrypoint:      entrypoint,
		Commands:        commands,
		LastSeen:        lastSeen,
	}
}

func (wj *WorkflowJob) ToWorkflowJobSql() WorkflowJobSql {
	var lastSeen string

	entrypoint := utils.ShlexJoin(wj.Entrypoint)
	commands := utils.ShlexJoin(wj.Commands)

	if !wj.LastSeen.IsZero() {
		lastSeen = wj.LastSeen.Format(timeLayout)
	}
	return WorkflowJobSql{
		WorkflowJobBase: wj.WorkflowJobBase,
		Entrypoint:      entrypoint,
		Commands:        commands,
		LastSeen:        lastSeen,
	}
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

// Declare job interface peer
type JobInterfacePeer struct {
	PeerJobId         int64  `json:"peer_job_id"`
	PeerNodeId        string `json:"peer_node_id"`
	PeerPath          string `json:"peer_path"`
	PeerMountFunction string `json:"peer_mount_function"`
	PeerDuty          int64  `json:"peer_duty"`
}

// Declare job interfaces base struct
type JobInterface struct {
	InterfaceId       int64              `json:"interface_id"`
	WorkflowId        int64              `json:"workflow_id"`
	JobId             int64              `json:"job_id"`
	JobInterfacePeers []JobInterfacePeer `json:"job_interface_peers"`
	Interface
}

// Declare job type
type Job struct {
	JobBase
	Entrypoint    []string       `json:"entrypoint"`
	Commands      []string       `json:"commands"`
	JobInterfaces []JobInterface `json:"job_interfaces"`
	Started       time.Time      `json:"started"`
	Ended         time.Time      `json:"ended"`
}

// Declare job sql type
type JobSql struct {
	JobBase
	Entrypoint string `json:"entrypoint"`
	Commands   string `json:"commands"`
	Started    string `json:"started"`
	Ended      string `json:"ended"`
}

func (js *JobSql) ToJob() Job {
	var started, ended time.Time

	entrypoint, _ := shlex.Split(js.Entrypoint)
	commands, _ := shlex.Split(js.Commands)

	if js.Started != "" {
		started, _ = time.Parse(timeLayout, js.Started)
	}
	if js.Ended != "" {
		ended, _ = time.Parse(timeLayout, js.Ended)
	}

	return Job{
		JobBase:    js.JobBase,
		Entrypoint: entrypoint,
		Commands:   commands,
		Started:    started,
		Ended:      ended,
	}
}

func (j *Job) ToJobSql() JobSql {
	var started, ended string

	entrypoint := utils.ShlexJoin(j.Entrypoint)
	commands := utils.ShlexJoin(j.Commands)

	if !j.Started.IsZero() {
		started = j.Started.Format(timeLayout)
	}
	if !j.Ended.IsZero() {
		ended = j.Ended.Format(timeLayout)
	}
	return JobSql{
		JobBase:    j.JobBase,
		Entrypoint: entrypoint,
		Commands:   commands,
		Started:    started,
		Ended:      ended,
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

// Declare job data receipt acknowledgement type
type JobDataReceiptAcknowledgement struct {
	JobRunStatusRequest
	InterfaceId     int64  `json:"interface_id"`
	OrderingNodeId  string `json:"ordering_node_id"`
	ReceivingNodeId string `json:"receiving_node_id"`
}

// Declare job data request type
type JobDataRequest struct {
	JobRunStatusRequest
	InterfaceId int64  `json:"interface_id"`
	WhatData    string `json:"what_data"`
}

// Declare docker image type
type DockerImage struct {
	Id          string    `json:"id"`
	Name        string    `json:"name"`
	EntryPoints []string  `json:"entry_points"`
	Commands    []string  `json:"commands"`
	Tags        []string  `json:"tags"`
	Digests     []string  `json:"digests"`
	Os          string    `json:"os"`
	BuiltAt     time.Time `json:"built_at"`
}

// Declare docker image with Interfaces type
type DockerImageWithInterfaces struct {
	DockerImage
	Interfaces []Interface `json:"interfaces"`
}
