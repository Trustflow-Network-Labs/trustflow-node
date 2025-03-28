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
	Id     int32
	PeerId [255]byte
}

// Declare currency type
type Currency struct {
	Symbol   string `json:"symbol"`
	Currency string `json:"currency"`
}

// Declare price type
type Price struct {
	Id                    int32   `json:"id"`
	ServiceId             int32   `json:"service_id"`
	ResourceId            int32   `json:"resource_id"`
	Currency              string  `json:"currency"`
	Price                 float64 `json:"price"`
	PriceUnitNormalizator float64 `json:"price_unit_normalizator"`
	PriceInterval         float64 `json:"price_interval"`
}

// Declare resource type
type Resource struct {
	Id     int32  `json:"id"`
	Name   string `json:"name"`
	Active bool   `json:"active"`
}

// Declare resource utilization type
type ResourceUtilization struct {
	Id          int32   `json:"id"`
	JobId       int32   `json:"job_id"`
	ResourceId  int32   `json:"resource_id"`
	Utilization float64 `json:"utilization"`
	Timestamp   string  `json:"timestamp"`
}

// Declare service type
type Service struct {
	Id          int32  `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	NodeId      string `json:"node_id"`
	Type        string `json:"type"`
	Path        string `json:"path"`
	Repo        string `json:"repo"`
	Active      bool   `json:"active"`
}

// Declare search local service type
type SearchService struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	NodeId      string `json:"node_id"`
	Type        string `json:"type"`
	Repo        string `json:"repo"`
	Active      bool   `json:"active"`
}

// Declare remote service lookup type
type ServiceLookup struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	NodeId      string `json:"node_id"`
	Type        string `json:"type"`
	Repo        string `json:"repo"`
}

// Declare service price model type
type ServiceResourcesWithPricing struct {
	ResourceName          string  `json:"resource_name"`
	Price                 float64 `json:"price"`
	PriceUnitNormalizator float64 `json:"price_unit_normalizator"`
	PriceInterval         float64 `json:"price_interval"`
	CurrencyName          string  `json:"currency_name"`
	CurrencySymbol        string  `json:"currency_symbol"`
}

// Declare service offer type
type ServiceOffer struct {
	Id                int32                         `json:"id"`
	Name              string                        `json:"name"`
	Description       string                        `json:"description"`
	NodeId            string                        `json:"node_id"`
	Type              string                        `json:"type"`
	Path              string                        `json:"path"`
	Repo              string                        `json:"repo"`
	Active            bool                          `json:"active"`
	ServicePriceModel []ServiceResourcesWithPricing `json:"service_price_model"`
}

// Declare job type
type Job struct {
	Id             int32     `json:"id"`
	OrderingNodeId string    `json:"ordering_node_id"`
	ServiceId      int32     `json:"service_id"`
	Status         string    `json:"status"`
	Started        time.Time `json:"started"`
	Ended          time.Time `json:"ended"`
}
