package keystore

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/stretchr/testify/assert"
)

// Mock implementation of the LogsManager
type MockLogsManager struct{}

func (m *MockLogsManager) Log(level string, message string, component string) {
	// Do nothing in tests
}

func NewMockLogsManager() *MockLogsManager {
	return &MockLogsManager{}
}

// Custom KeyStoreManager for testing that uses mocks
type TestKeyStoreManager struct {
	db           *sql.DB
	lm           *MockLogsManager
	settingsData map[string]any
}

func NewTestKeyStoreManager(db *sql.DB) *TestKeyStoreManager {
	return &TestKeyStoreManager{
		db:           db,
		lm:           NewMockLogsManager(),
		settingsData: make(map[string]any),
	}
}

// Implement the same methods from KeyStoreManager but with our mocks

func (ksm *TestKeyStoreManager) AddKey(identifier string, algorithm string, key []byte) error {
	_, err := ksm.db.ExecContext(context.Background(), "insert into keystore (identifier, algorithm, key) values (?, ?, ?);",
		identifier, algorithm, key)
	if err != nil {
		// Using our mock logger that doesn't access the filesystem
		ksm.lm.Log("error", err.Error(), "keystore")
		return err
	}

	return nil
}

func (ksm *TestKeyStoreManager) FindKey(identifier string) (node_types.Key, error) {
	// Declarations
	var k node_types.Key

	row := ksm.db.QueryRowContext(context.Background(), "select * from keystore where identifier = ?;", identifier)
	err := row.Scan(&k.Id, &k.Identifier, &k.Algorithm, &k.Key)
	if err != nil {
		ksm.lm.Log("error", err.Error(), "keystore")
		return k, err
	}
	return k, nil
}

func (ksm *TestKeyStoreManager) ProvideKey() (crypto.PrivKey, crypto.PubKey, error) {
	// Declarations
	var priv crypto.PrivKey
	var pub crypto.PubKey

	// Check if we have a node key already - using settingsData map instead of database
	nodeId, exists := ksm.settingsData["node_identifier"]
	if !exists || nodeId.(string) == "" {
		// We don't have a key let's create it
		var err error
		priv, pub, err = crypto.GenerateKeyPair(
			crypto.ECDSA,
			-1,
		)
		if err != nil {
			ksm.lm.Log("error", "Can generate key pair. ("+err.Error()+")", "api")
			return nil, nil, err
		}

		// This would normally get the Peer ID, but we'll just use a test value
		peerId := "test-peer-id"

		// Update node identifier in our mock settings
		ksm.settingsData["node_identifier"] = peerId

		// Add key
		key, err := crypto.MarshalPrivateKey(priv)
		if err != nil {
			ksm.lm.Log("panic", err.Error(), "p2p")
			return nil, nil, err
		}

		err = ksm.AddKey(peerId, "ECDSA: secp256r1", key)
		if err != nil {
			ksm.lm.Log("panic", err.Error(), "p2p")
			return nil, nil, err
		}
	} else {
		// We already have a key created before
		key, err := ksm.FindKey(nodeId.(string))
		if err != nil && err != sql.ErrNoRows {
			ksm.lm.Log("error", err.Error(), "node")
			return nil, nil, err
		} else if err == sql.ErrNoRows {
			// but we can't find it
			ksm.lm.Log("error", "Could not find a key for provided node identifier "+nodeId.(string)+".", "api")
			return nil, nil, err
		} else {
			// Get private key
			switch key.Algorithm {
			case "ECDSA: secp256r1":
				priv, err = crypto.UnmarshalPrivateKey(key.Key)
				if err != nil {
					ksm.lm.Log("error", err.Error(), "node")
					return nil, nil, err
				}
				pub = priv.GetPublic()
			default:
				err := errors.New("unsupported algorithm")
				ksm.lm.Log("error", "Could not extract a key for provided algorithm "+key.Algorithm+".", "api")
				return nil, nil, err
			}
		}
	}

	return priv, pub, nil
}

func setupMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("an error '%s' was not expected when opening a stub database connection", err)
	}
	return db, mock
}

func TestAddKey(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	ksm := NewTestKeyStoreManager(db)

	// Test case: successful key addition
	mock.ExpectExec("insert into keystore").
		WithArgs("test-id", "ECDSA", []byte{1, 2, 3}).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := ksm.AddKey("test-id", "ECDSA", []byte{1, 2, 3})
	assert.NoError(t, err)

	// Test case: database error
	mock.ExpectExec("insert into keystore").
		WithArgs("test-id", "ECDSA", []byte{1, 2, 3}).
		WillReturnError(errors.New("database error"))

	err = ksm.AddKey("test-id", "ECDSA", []byte{1, 2, 3})
	assert.Error(t, err)
	assert.Equal(t, "database error", err.Error())

	// Make sure all expectations were met
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestFindKey(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	ksm := NewTestKeyStoreManager(db)

	// Test case: key found
	rows := sqlmock.NewRows([]string{"id", "identifier", "algorithm", "key"}).
		AddRow(1, "test-id", "ECDSA", []byte{1, 2, 3})

	mock.ExpectQuery("select \\* from keystore where identifier = \\?").
		WithArgs("test-id").
		WillReturnRows(rows)

	key, err := ksm.FindKey("test-id")
	assert.NoError(t, err)
	assert.Equal(t, int64(1), key.Id)
	assert.Equal(t, "test-id", key.Identifier)
	assert.Equal(t, "ECDSA", key.Algorithm)
	assert.Equal(t, []byte{1, 2, 3}, key.Key)

	// Test case: key not found
	mock.ExpectQuery("select \\* from keystore where identifier = \\?").
		WithArgs("non-existent").
		WillReturnError(sql.ErrNoRows)

	_, err = ksm.FindKey("non-existent")
	assert.Error(t, err)
	assert.Equal(t, sql.ErrNoRows, err)

	// Make sure all expectations were met
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestProvideKey_NewKey(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	ksm := NewTestKeyStoreManager(db)

	// Set up for creating a new key (empty node identifier)
	ksm.settingsData["node_identifier"] = ""

	// Mock the key insertion
	mock.ExpectExec("insert into keystore").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	resultPriv, resultPub, err := ksm.ProvideKey()
	assert.NoError(t, err)
	assert.NotNil(t, resultPriv)
	assert.NotNil(t, resultPub)

	// Make sure all expectations were met
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestProvideKey_ExistingKey(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	ksm := NewTestKeyStoreManager(db)

	// Create a real ECDSA key for testing
	priv, _, err := crypto.GenerateKeyPair(crypto.ECDSA, -1)
	assert.NoError(t, err)

	privBytes, err := crypto.MarshalPrivateKey(priv)
	assert.NoError(t, err)

	// Set up for using an existing key
	ksm.settingsData["node_identifier"] = "test-peer-id"

	// Mock the FindKey call
	keyRows := sqlmock.NewRows([]string{"id", "identifier", "algorithm", "key"}).
		AddRow(1, "test-peer-id", "ECDSA: secp256r1", privBytes)

	mock.ExpectQuery("select \\* from keystore where identifier = \\?").
		WithArgs("test-peer-id").
		WillReturnRows(keyRows)

	resultPriv, resultPub, err := ksm.ProvideKey()
	assert.NoError(t, err)
	assert.NotNil(t, resultPriv)
	assert.NotNil(t, resultPub)

	// Verify resultPriv is equivalent to priv
	privBytes1, err := crypto.MarshalPrivateKey(priv)
	assert.NoError(t, err)
	privBytes2, err := crypto.MarshalPrivateKey(resultPriv)
	assert.NoError(t, err)
	assert.Equal(t, privBytes1, privBytes2)

	// Make sure all expectations were met
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestProvideKey_KeyNotFound(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	ksm := NewTestKeyStoreManager(db)

	// Set up for a key that doesn't exist in the database
	ksm.settingsData["node_identifier"] = "test-peer-id"

	// Mock key not found
	mock.ExpectQuery("select \\* from keystore where identifier = \\?").
		WithArgs("test-peer-id").
		WillReturnError(sql.ErrNoRows)

	_, _, err := ksm.ProvideKey()
	assert.Error(t, err)
	assert.Equal(t, sql.ErrNoRows, err)

	// Make sure all expectations were met
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}

func TestProvideKey_UnsupportedAlgorithm(t *testing.T) {
	db, mock := setupMockDB(t)
	defer db.Close()

	ksm := NewTestKeyStoreManager(db)

	// Set up for an unsupported algorithm
	ksm.settingsData["node_identifier"] = "test-peer-id"

	// Mock FindKey with unsupported algorithm
	keyRows := sqlmock.NewRows([]string{"id", "identifier", "algorithm", "key"}).
		AddRow(1, "test-peer-id", "UNSUPPORTED", []byte{1, 2, 3})

	mock.ExpectQuery("select \\* from keystore where identifier = \\?").
		WithArgs("test-peer-id").
		WillReturnRows(keyRows)

	_, _, err := ksm.ProvideKey()
	assert.Error(t, err)

	// Make sure all expectations were met
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %s", err)
	}
}
