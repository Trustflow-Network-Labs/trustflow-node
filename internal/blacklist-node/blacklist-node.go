package blacklist_node

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/adgsm/trustflow-node/internal/node_types"
	"github.com/adgsm/trustflow-node/internal/ui"
	"github.com/adgsm/trustflow-node/internal/utils"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/conngater"
)

type BlacklistNodeManager struct {
	shortenedKeys map[string]string
	db            *sql.DB
	lm            *utils.LogsManager
	tm            *utils.TextManager
	Gater         *conngater.BasicConnectionGater
	UI            ui.UI
}

func NewBlacklistNodeManager(db *sql.DB, ui ui.UI) (*BlacklistNodeManager, error) {
	gater, err := conngater.NewBasicConnectionGater(nil)
	if err != nil {
		return nil, err
	}

	blnm := &BlacklistNodeManager{
		shortenedKeys: map[string]string{},
		db:            db,
		lm:            utils.NewLogsManager(),
		tm:            utils.NewTextManager(),
		Gater:         gater,
		UI:            ui,
	}

	err = blnm.loadBlacklist()
	if err != nil {
		return nil, err
	}

	return blnm, nil
}

// Load blacklist
func (blnm *BlacklistNodeManager) loadBlacklist() error {
	// Load a blacklist
	rows, err := blnm.db.QueryContext(context.Background(), "select node_id from blacklisted_nodes;")
	if err != nil {
		blnm.lm.Log("error", err.Error(), "blacklist-node")
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var nodeId string
		if err := rows.Scan(&nodeId); err != nil {
			blnm.lm.Log("error", err.Error(), "blacklist-node")
			return err
		}
		peerId, err := peer.Decode(nodeId)
		if err != nil {
			blnm.lm.Log("error", err.Error(), "blacklist-node")
			return err
		}
		err = blnm.Gater.BlockPeer(peerId)
		if err != nil {
			blnm.lm.Log("error", err.Error(), "blacklist-node")
			return err
		}
		key := blnm.tm.Shorten(nodeId, 6, 6)
		blnm.shortenedKeys[key] = nodeId
	}

	return rows.Err()
}

// Is node blacklisted
func (blnm *BlacklistNodeManager) IsBlacklisted(nodeId string) (bool, error) {
	if nodeId == "" {
		msg := "invalid Node ID"
		blnm.lm.Log("error", msg, "blacklist-node")
		return true, errors.New(msg)
	}

	peerId, err := peer.Decode(nodeId)
	if err != nil {
		blnm.lm.Log("error", err.Error(), "blacklist-node")
		return true, err
	}

	return !blnm.Gater.InterceptPeerDial(peerId), nil
}

// List blacklisted nodes
func (blnm *BlacklistNodeManager) List() ([]node_types.Blacklist, error) {
	// Load a blacklist
	rows, err := blnm.db.QueryContext(context.Background(), "select node_id, reason, timestamp from blacklisted_nodes;")
	if err != nil {
		blnm.lm.Log("error", err.Error(), "blacklist-node")
		return nil, err
	}
	defer rows.Close()

	var nodes []node_types.Blacklist
	for rows.Next() {
		var nodeSQL node_types.BlacklistSQL
		if err := rows.Scan(&nodeSQL.NodeId, &nodeSQL.Reason, &nodeSQL.Timestamp); err == nil {
			peerId, err := peer.Decode(nodeSQL.NodeId)
			if err == nil {
				t, err := time.Parse(time.RFC3339, nodeSQL.Timestamp)
				if err != nil {
					blnm.UI.Print(fmt.Sprintf("Error parsing time: %s", err.Error()))
					continue
				}
				var node node_types.Blacklist = node_types.Blacklist{
					NodeId:    peerId,
					Reason:    nodeSQL.Reason.String,
					Timestamp: t,
				}
				nodes = append(nodes, node)
			}
		}
	}

	return nodes, rows.Err()
}

// Blacklist a node
func (blnm *BlacklistNodeManager) Add(nodeId string, reason string) error {
	blacklisted, err := blnm.IsBlacklisted(nodeId)
	if err != nil {
		blnm.lm.Log("error", err.Error(), "blacklist-node")
		return err
	}
	// Check if node is already blacklisted
	if blacklisted {
		err = fmt.Errorf("node %s is already blacklisted", nodeId)
		blnm.lm.Log("warn", err.Error(), "blacklist-node")
		return err
	}

	// Add node to blacklist
	blnm.lm.Log("debug", fmt.Sprintf("add node %s to blacklist", nodeId), "blacklist-node")

	_, err = blnm.db.ExecContext(context.Background(), `insert into blacklisted_nodes (node_id, reason, timestamp) values (?, ?, ?);`,
		nodeId, reason, time.Now().Format(time.RFC3339))
	if err != nil {
		msg := err.Error()
		blnm.lm.Log("error", msg, "blacklist-node")
		return err
	}

	peerId, err := peer.Decode(nodeId)
	if err != nil {
		blnm.lm.Log("error", err.Error(), "blacklist-node")
		return err
	}

	key := blnm.tm.Shorten(nodeId, 6, 6)
	blnm.shortenedKeys[key] = nodeId

	return blnm.Gater.BlockPeer(peerId)
}

// Remove node from blacklist
func (blnm *BlacklistNodeManager) Remove(nodeId string) error {
	// Chek if we have shortened key provided
	fullNodeId, found := blnm.shortenedKeys[nodeId]
	if found {
		nodeId = fullNodeId
	}

	blacklisted, err := blnm.IsBlacklisted(nodeId)
	if err != nil {
		msg := err.Error()
		blnm.lm.Log("error", msg, "blacklist-node")
		return err
	}

	// Check if node is already blacklisted
	if !blacklisted {
		err = fmt.Errorf("node %s is not blacklisted", nodeId)
		blnm.lm.Log("warn", err.Error(), "blacklist-node")
		return err
	}

	// Remove node from blacklist
	blnm.lm.Log("debug", fmt.Sprintf("removing node %s from blacklist", nodeId), "blacklist-node")

	_, err = blnm.db.ExecContext(context.Background(), "delete from blacklisted_nodes where node_id = ?;", nodeId)
	if err != nil {
		msg := err.Error()
		blnm.lm.Log("error", msg, "blacklist-node")
		return err
	}

	peerId, err := peer.Decode(nodeId)
	if err != nil {
		blnm.lm.Log("error", err.Error(), "blacklist-node")
		return err
	}

	key := blnm.tm.Shorten(nodeId, 6, 6)
	delete(blnm.shortenedKeys, key)

	return blnm.Gater.UnblockPeer(peerId)
}
