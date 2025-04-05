package node_types

import (
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
)

// Declare key type
type Key struct {
	Id         int32  `json:"id"`
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
	Id     int64
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
	Id        int64  `json:"id"`
	ServiceId int64  `json:"service_id"`
	Repo      string `json:"repo"`
	Image     string `json:"image"`
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

// Declare job type
type Job struct {
	Id             int64     `json:"id"`
	OrderingNodeId string    `json:"ordering_node_id"`
	ServiceId      int64     `json:"service_id"`
	Status         string    `json:"status"`
	Started        time.Time `json:"started"`
	Ended          time.Time `json:"ended"`
}
