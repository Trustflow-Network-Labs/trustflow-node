package node_types

import (
	"github.com/adgsm/trustflow-node/utils"
)

// Declare node type
type Node struct {
	Id         utils.NullInt32  `json:"id"`
	NodeId     utils.NullString `json:"node_id"`
	Multiaddrs utils.NullString `json:"multiaddrs"`
	Self       utils.NullBool   `json:"self"`
}

// Declare key type
type Key struct {
	Id         utils.NullInt32  `json:"id"`
	Identifier utils.NullString `json:"identifier"`
	Algorithm  utils.NullString `json:"algorithm"`
	Key        []byte           `json:"key"`
}
