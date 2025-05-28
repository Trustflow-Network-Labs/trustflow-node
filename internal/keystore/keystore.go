package keystore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/adgsm/trustflow-node-gui-client/internal/node_types"
	"github.com/adgsm/trustflow-node-gui-client/internal/settings"
	"github.com/adgsm/trustflow-node-gui-client/internal/utils"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

type KeyStoreManager struct {
	db *sql.DB
	lm *utils.LogsManager
}

func NewKeyStoreManager(db *sql.DB) *KeyStoreManager {
	return &KeyStoreManager{
		db: db,
		lm: utils.NewLogsManager(),
	}
}

// Add node
func (ksm *KeyStoreManager) AddKey(identifier string, algorithm string, key []byte) error {
	_, err := ksm.db.ExecContext(context.Background(), "insert into keystore (identifier, algorithm, key) values (?, ?, ?);",
		identifier, algorithm, key)
	if err != nil {
		msg := err.Error()
		ksm.lm.Log("error", msg, "keystore")
		return err
	}

	return nil
}

// Find key
func (ksm *KeyStoreManager) FindKey(identifier string) (node_types.Key, error) {
	// Declarations
	var k node_types.Key

	row := ksm.db.QueryRowContext(context.Background(), "select * from keystore where identifier = ?;", identifier)
	err := row.Scan(&k.Id, &k.Identifier, &k.Algorithm, &k.Key)
	if err != nil {
		msg := err.Error()
		ksm.lm.Log("error", msg, "keystore")
		return k, err
	}
	return k, nil
}

func (ksm *KeyStoreManager) ProvideKey() (crypto.PrivKey, crypto.PubKey, error) {
	// Declarations
	var priv crypto.PrivKey
	var pub crypto.PubKey

	// Check do we have a node key already
	settingsManager := settings.NewSettingsManager(ksm.db)
	nodeId, err := settingsManager.Read("node_identifier")
	if err != nil {
		msg := err.Error()
		ksm.lm.Log("error", msg, "node")
		return nil, nil, err
	} else if nodeId.(string) == "" {
		// We don't have a key let's create it
		// Create a keypair
		//		p, err := ecdsa.GenerateKey(secp256k1.S256(), rand.Reader)
		//privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		priv, pub, err = crypto.GenerateKeyPair(
			crypto.ECDSA,
			-1,
		)
		if err != nil {
			msg := fmt.Sprintf("Can generate key pair. (%s)", err.Error())
			ksm.lm.Log("error", msg, "api")
			return nil, nil, err
		}

		// Get Peer ID from the public key
		peerId, err := peer.IDFromPublicKey(pub)
		if err != nil {
			msg := fmt.Sprintf("Can generate peer ID from public key. (%s)", err.Error())
			ksm.lm.Log("error", msg, "api")
			return nil, nil, err
		}

		// Update node identifier to settings
		settingsManager.Modify("node_identifier", peerId.String())

		// Add key
		key, err := crypto.MarshalPrivateKey(priv)
		if err != nil {
			ksm.lm.Log("panic", err.Error(), "p2p")
			panic(fmt.Sprintf("%v", err))
		}

		err = ksm.AddKey(peerId.String(), fmt.Sprintf("%s: secp256r1", priv.Type().String()), key)
		if err != nil {
			ksm.lm.Log("panic", err.Error(), "p2p")
			panic(fmt.Sprintf("%v", err))
		}
	} else {
		// We already have a key created before
		key, err := ksm.FindKey(nodeId.(string))
		if err != nil && strings.ToLower(err.Error()) != "sql: no rows in result set" {
			msg := err.Error()
			ksm.lm.Log("error", msg, "node")
			return nil, nil, err
		} else if err != nil && strings.ToLower(err.Error()) == "sql: no rows in result set" {
			// but we can't find it
			msg := fmt.Sprintf("Could not find a key for provided node identifier %s.", nodeId.(string))
			ksm.lm.Log("error", msg, "api")
			return nil, nil, err
		} else {
			// Get private key
			switch key.Algorithm {
			case "ECDSA: secp256r1":
				priv, err = crypto.UnmarshalPrivateKey(key.Key)
				if err != nil {
					msg := err.Error()
					ksm.lm.Log("error", msg, "node")
					return nil, nil, err
				}
				pub = priv.GetPublic()
			default:
				msg := fmt.Sprintf("Could not extract a key for provided algorithm %s.", key.Algorithm)
				ksm.lm.Log("error", msg, "api")
				return nil, nil, err
			}
		}
	}

	return priv, pub, nil
}
