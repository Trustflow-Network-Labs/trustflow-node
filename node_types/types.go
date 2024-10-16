package node_types

// Declare node type
type Node struct {
	Id         NullInt32  `json:"id"`
	NodeId     NullString `json:"node_id"`
	Multiaddrs NullString `json:"multiaddrs"`
	Self       NullBool   `json:"self"`
}

// Declare key type
type Key struct {
	Id         NullInt32  `json:"id"`
	Identifier NullString `json:"identifier"`
	Algorithm  NullString `json:"algorithm"`
	Key        []byte     `json:"key"`
}

// Declare stream data type
type StreamData struct {
	Type    uint16
	Version uint16
	PeerId  [255]byte
}

// Declare currency type
type Currency struct {
	Id       NullInt32  `json:"id"`
	Currency NullString `json:"currency"`
	Symbol   NullString `json:"symbol"`
}

// Declare price type
type Price struct {
	Id                    NullInt32   `json:"id"`
	ServiceId             NullInt32   `json:"service_id"`
	ResourceId            NullInt32   `json:"resource_id"`
	CurrencyId            NullInt32   `json:"currency_id"`
	Price                 NullFloat64 `json:"price"`
	PriceUnitNormalizator NullFloat64 `json:"price_unit_normalizator"`
	PriceInterval         NullFloat64 `json:"price_interval"`
}

// Declare resource type
type Resource struct {
	Id     NullInt32  `json:"id"`
	Name   NullString `json:"name"`
	Active NullBool   `json:"active"`
}

// Declare resource utilization type
type ResourceUtilization struct {
	Id          NullInt32   `json:"id"`
	JobId       NullInt32   `json:"job_id"`
	ResourceId  NullInt32   `json:"resource_id"`
	Utilization NullFloat64 `json:"utilization"`
	Timestamp   NullString  `json:"timestamp"`
}
