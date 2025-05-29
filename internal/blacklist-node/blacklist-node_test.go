package blacklist_node

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/adgsm/trustflow-node/internal/node_types"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/net/conngater"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock LogsManager
type MockLogsManager struct{}

func (m *MockLogsManager) Log(level string, message string, component string) {
	// Do nothing in tests
}

// Mock TextManager
type MockTextManager struct{}

func (m *MockTextManager) Shorten(text string, prefixLen int, suffixLen int) string {
	if len(text) <= prefixLen+suffixLen {
		return text
	}
	return text[:prefixLen] + "..." + text[len(text)-suffixLen:]
}

// Test implementation of BlacklistNodeManager
type TestBlacklistNodeManager struct {
	shortenedKeys map[string]string
	db            *sql.DB
	lm            *MockLogsManager
	tm            *MockTextManager
	Gater         *conngater.BasicConnectionGater
}

// Create a new TestBlacklistNodeManager
func NewTestBlacklistNodeManager(db *sql.DB) (*TestBlacklistNodeManager, error) {
	gater, err := conngater.NewBasicConnectionGater(nil)
	if err != nil {
		return nil, err
	}

	blnm := &TestBlacklistNodeManager{
		shortenedKeys: map[string]string{},
		db:            db,
		lm:            &MockLogsManager{},
		tm:            &MockTextManager{},
		Gater:         gater,
	}

	return blnm, nil
}

// Load blacklist implementation for TestBlacklistNodeManager
func (blnm *TestBlacklistNodeManager) loadBlacklist() error {
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

// IsBlacklisted implementation for TestBlacklistNodeManager
func (blnm *TestBlacklistNodeManager) IsBlacklisted(nodeId string) (bool, error) {
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

// List implementation for TestBlacklistNodeManager
func (blnm *TestBlacklistNodeManager) List() ([]node_types.Blacklist, error) {
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

// Add implementation for TestBlacklistNodeManager
func (blnm *TestBlacklistNodeManager) Add(nodeId string, reason string) error {
	blacklisted, err := blnm.IsBlacklisted(nodeId)
	if err != nil {
		blnm.lm.Log("error", err.Error(), "blacklist-node")
		return err
	}
	// Check if node is already blacklisted
	if blacklisted {
		err = errors.New("node " + nodeId + " is already blacklisted")
		blnm.lm.Log("warn", err.Error(), "blacklist-node")
		return err
	}

	// Add node to blacklist
	blnm.lm.Log("debug", "add node "+nodeId+" to blacklist", "blacklist-node")

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

// Remove implementation for TestBlacklistNodeManager
func (blnm *TestBlacklistNodeManager) Remove(nodeId string) error {
	// Check if we have shortened key provided
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
		err = errors.New("node " + nodeId + " is not blacklisted")
		blnm.lm.Log("warn", err.Error(), "blacklist-node")
		return err
	}

	// Remove node from blacklist
	blnm.lm.Log("debug", "removing node "+nodeId+" from blacklist", "blacklist-node")

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

// Helper function to setup mock DB
func setupMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	return db, mock
}

// Helper function to create a valid peer ID for testing
func createValidPeerID(t *testing.T) string {
	// This is a valid libp2p peer ID format for testing
	return "12D3KooWA1bPnCvKJbfVxKRhxZgXLRTFsKHvDKgzwKhDPAYgE2zU"
}

// Tests

func TestNewBlacklistNodeManager(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	// Setup expectations for loading the blacklist during initialization
	mock.ExpectQuery("select node_id from blacklisted_nodes").
		WillReturnRows(sqlmock.NewRows([]string{"node_id"}))

	blnm, err := NewTestBlacklistNodeManager(db)
	require.NoError(t, err)
	require.NotNil(t, blnm)
	require.NotNil(t, blnm.Gater)

	// Load blacklist
	err = blnm.loadBlacklist()
	require.NoError(t, err)

	// Verify expectations were met
	err = mock.ExpectationsWereMet()
	require.NoError(t, err)
}

func TestIsBlacklisted(t *testing.T) {
	db, _ := setupMockDB(t)
	defer db.Close()

	blnm, err := NewTestBlacklistNodeManager(db)
	require.NoError(t, err)

	// Test with empty node ID
	blacklisted, err := blnm.IsBlacklisted("")
	assert.Error(t, err)
	assert.True(t, blacklisted)
	assert.Equal(t, "invalid Node ID", err.Error())

	// Test with invalid node ID format
	blacklisted, err = blnm.IsBlacklisted("invalid-node-id")
	assert.Error(t, err)
	assert.True(t, blacklisted)

	// Test with valid node ID that is not blacklisted
	validNodeID := createValidPeerID(t)
	blacklisted, err = blnm.IsBlacklisted(validNodeID)
	assert.NoError(t, err)
	assert.False(t, blacklisted)

	// Block the peer and test again
	peerId, err := peer.Decode(validNodeID)
	require.NoError(t, err)
	err = blnm.Gater.BlockPeer(peerId)
	require.NoError(t, err)

	blacklisted, err = blnm.IsBlacklisted(validNodeID)
	assert.NoError(t, err)
	assert.True(t, blacklisted)
}

func TestList(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	blnm, err := NewTestBlacklistNodeManager(db)
	require.NoError(t, err)

	// Setup expectations for List
	timestamp := time.Now().Format(time.RFC3339)
	rows := sqlmock.NewRows([]string{"node_id", "reason", "timestamp"}).
		AddRow(createValidPeerID(t), sql.NullString{String: "test reason", Valid: true}, timestamp)

	mock.ExpectQuery("select node_id, reason, timestamp from blacklisted_nodes").
		WillReturnRows(rows)

	// Call List
	list, err := blnm.List()
	assert.NoError(t, err)
	assert.Equal(t, 1, len(list))
	assert.Equal(t, "test reason", list[0].Reason)

	// Verify time parsing worked
	expectedTime, _ := time.Parse(time.RFC3339, timestamp)
	assert.Equal(t, expectedTime, list[0].Timestamp)

	// Test error case
	mock.ExpectQuery("select node_id, reason, timestamp from blacklisted_nodes").
		WillReturnError(errors.New("database error"))

	list, err = blnm.List()
	assert.Error(t, err)
	assert.Nil(t, list)

	// Verify expectations were met
	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}

func TestAdd(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	blnm, err := NewTestBlacklistNodeManager(db)
	require.NoError(t, err)

	validNodeID := createValidPeerID(t)

	// Test adding a node
	mock.ExpectExec("insert into blacklisted_nodes").
		WithArgs(validNodeID, "test reason", sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = blnm.Add(validNodeID, "test reason")
	assert.NoError(t, err)

	// Verify the node was added to shortenedKeys
	key := blnm.tm.Shorten(validNodeID, 6, 6)
	assert.Equal(t, validNodeID, blnm.shortenedKeys[key])

	// Test adding already blacklisted node
	blacklisted, err := blnm.IsBlacklisted(validNodeID)
	assert.NoError(t, err)
	assert.True(t, blacklisted)

	err = blnm.Add(validNodeID, "test reason")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already blacklisted")

	// Test database error
	validNodeID2 := "12D3KooWG3XbzoVyAE6Y9jTpJU4TFASuQqfEkzAS8YbTYwYbS7Ev"
	mock.ExpectExec("insert into blacklisted_nodes").
		WithArgs(validNodeID2, "test reason", sqlmock.AnyArg()).
		WillReturnError(errors.New("database error"))

	err = blnm.Add(validNodeID2, "test reason")
	assert.Error(t, err)
	assert.Equal(t, "database error", err.Error())

	// Verify expectations were met
	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}

func TestRemove(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	blnm, err := NewTestBlacklistNodeManager(db)
	require.NoError(t, err)

	validNodeID := createValidPeerID(t)

	// Setup - add a node first
	peerId, err := peer.Decode(validNodeID)
	require.NoError(t, err)
	err = blnm.Gater.BlockPeer(peerId)
	require.NoError(t, err)

	key := blnm.tm.Shorten(validNodeID, 6, 6)
	blnm.shortenedKeys[key] = validNodeID

	// Test removing a node
	mock.ExpectExec("delete from blacklisted_nodes where node_id = ?").
		WithArgs(validNodeID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = blnm.Remove(validNodeID)
	assert.NoError(t, err)

	// Verify the node was removed from shortenedKeys
	_, exists := blnm.shortenedKeys[key]
	assert.False(t, exists)

	// Test removing with shortened key
	// Re-add node first
	err = blnm.Gater.BlockPeer(peerId)
	require.NoError(t, err)
	blnm.shortenedKeys[key] = validNodeID

	mock.ExpectExec("delete from blacklisted_nodes where node_id = ?").
		WithArgs(validNodeID).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err = blnm.Remove(key)
	assert.NoError(t, err)

	// Test removing non-blacklisted node
	err = blnm.Remove(validNodeID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not blacklisted")

	// Test database error
	// Re-add node first
	err = blnm.Gater.BlockPeer(peerId)
	require.NoError(t, err)
	blnm.shortenedKeys[key] = validNodeID

	mock.ExpectExec("delete from blacklisted_nodes where node_id = ?").
		WithArgs(validNodeID).
		WillReturnError(errors.New("database error"))

	err = blnm.Remove(validNodeID)
	assert.Error(t, err)
	assert.Equal(t, "database error", err.Error())

	// Verify expectations were met
	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}

func TestLoadBlacklist(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	blnm, err := NewTestBlacklistNodeManager(db)
	require.NoError(t, err)

	validNodeID := createValidPeerID(t)

	// Setup expectations for loading the blacklist
	rows := sqlmock.NewRows([]string{"node_id"}).
		AddRow(validNodeID)

	mock.ExpectQuery("select node_id from blacklisted_nodes").
		WillReturnRows(rows)

	// Load the blacklist
	err = blnm.loadBlacklist()
	assert.NoError(t, err)

	// Verify the node was added to shortenedKeys
	key := blnm.tm.Shorten(validNodeID, 6, 6)
	assert.Equal(t, validNodeID, blnm.shortenedKeys[key])

	// Verify node is blacklisted
	blacklisted, err := blnm.IsBlacklisted(validNodeID)
	assert.NoError(t, err)
	assert.True(t, blacklisted)

	// Test database error
	mock.ExpectQuery("select node_id from blacklisted_nodes").
		WillReturnError(errors.New("database error"))

	err = blnm.loadBlacklist()
	assert.Error(t, err)
	assert.Equal(t, "database error", err.Error())

	// Verify expectations were met
	err = mock.ExpectationsWereMet()
	assert.NoError(t, err)
}
