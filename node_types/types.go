package node_types

import "time"

// Declare node type
type Node struct {
	Id         int32  `json:"id"`
	NodeId     string `json:"node_id"`
	Multiaddrs string `json:"multiaddrs"`
	Self       bool   `json:"self"`
}

// Declare key type
type Key struct {
	Id         int32  `json:"id"`
	Identifier string `json:"identifier"`
	Algorithm  string `json:"algorithm"`
	Key        []byte `json:"key"`
}

// Declare stream data type
type StreamData struct {
	Type   uint16
	Id     int32
	PeerId [255]byte
}

// Declare currency type
type Currency struct {
	Id       int32  `json:"id"`
	Currency string `json:"currency"`
	Symbol   string `json:"symbol"`
}

// Declare price type
type Price struct {
	Id                    int32   `json:"id"`
	ServiceId             int32   `json:"service_id"`
	ResourceId            int32   `json:"resource_id"`
	CurrencyId            int32   `json:"currency_id"`
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
	NodeId      int32  `json:"node_id"`
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
	OrderingNodeId int32     `json:"ordering_node_id"`
	ServiceId      int32     `json:"service_id"`
	Status         string    `json:"status"`
	Started        time.Time `json:"started"`
	Ended          time.Time `json:"ended"`
}
