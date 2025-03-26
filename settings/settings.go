package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/adgsm/trustflow-node/database"
	"github.com/adgsm/trustflow-node/node_types"
	"github.com/adgsm/trustflow-node/utils"
)

type SettingsManager struct {
	sm *database.SQLiteManager
	lm *utils.LogsManager
}

func NewSettingsManager() *SettingsManager {
	return &SettingsManager{
		sm: database.NewSQLiteManager(),
		lm: utils.NewLogsManager(),
	}
}

// A setting exists
func (sm *SettingsManager) Exists(key string) (error, bool) {
	if key == "" {
		msg := "invalid setting key"
		sm.lm.Log("error", msg, "settings")
		return errors.New(msg), false
	}

	// Create a database connection
	db, err := sm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "settings")
		return err, false
	}
	defer db.Close()

	// Check if key exists
	var k node_types.NullString
	row := db.QueryRowContext(context.Background(), "select key from settings where key = ?;", key)

	err = row.Scan(&k)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("debug", msg, "settings")
		return nil, false
	}

	return nil, true
}

// Read a setting
func (sm *SettingsManager) Read(key string) (any, error) {
	if key == "" {
		msg := "invalid setting key"
		sm.lm.Log("error", msg, "settings")
		return nil, errors.New(msg)
	}

	// Create a database connection
	db, err := sm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "settings")
		return nil, err
	}
	defer db.Close()

	// Check if key exists
	var keyType node_types.NullString
	row := db.QueryRowContext(context.Background(), "select setting_type from settings where key = ?;", key)

	err = row.Scan(&keyType)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("debug", msg, "settings")
		return nil, err
	}

	// Read value from determined key type
	switch keyType.String {
	case "STRING":
		var val string
		// Read settings table value_string for the provided key
		row := db.QueryRowContext(context.Background(), "select value_string from settings where key = ?;", key)

		err = row.Scan(&val)
		if err != nil {
			msg := err.Error()
			sm.lm.Log("debug", msg, "settings")
			return nil, err
		}

		return val, nil
	case "JSON":
		var val string
		// Read settings table value_json for the provided key
		row := db.QueryRowContext(context.Background(), "select value_json from settings where key = ?;", key)

		err = row.Scan(&val)
		if err != nil {
			msg := err.Error()
			sm.lm.Log("debug", msg, "settings")
			return nil, err
		}
		// Check if this is valid JSON structure
		if !IsValidJSON(val) {
			msg := fmt.Sprintf("Value %v is not a valid JSON structure", val)
			sm.lm.Log("error", msg, "settings")
			return nil, err
		}

		var js interface{}
		err = json.Unmarshal([]byte(val), &js)
		if err != nil {
			msg := err.Error()
			sm.lm.Log("debug", msg, "settings")
			return nil, err
		}

		return js, nil
	case "INTEGER":
		var val int32
		// Read settings table value_integer for the provided key
		row := db.QueryRowContext(context.Background(), "select value_integer from settings where key = ?;", key)

		err = row.Scan(&val)
		if err != nil {
			msg := err.Error()
			sm.lm.Log("debug", msg, "settings")
			return nil, err
		}

		return val, nil
	case "BOOLEAN":
		var val int32
		// Read settings table value_boolean for the provided key
		row := db.QueryRowContext(context.Background(), "select value_boolean from settings where key = ?;", key)

		err = row.Scan(&val)
		if err != nil {
			msg := err.Error()
			sm.lm.Log("debug", msg, "settings")
			return nil, err
		}

		return val != 0, nil
	case "REAL":
		var val float32
		// Read settings table value_real for the provided key
		row := db.QueryRowContext(context.Background(), "select value_real from settings where key = ?;", key)

		err = row.Scan(&val)
		if err != nil {
			msg := err.Error()
			sm.lm.Log("debug", msg, "settings")
			return nil, err
		}

		return val, nil
	default:
		msg := fmt.Sprintf("Invalid key type, %s", keyType.String)
		sm.lm.Log("error", msg, "settings")
		return nil, errors.New(msg)
	}
}

func (sm *SettingsManager) ReadBoolSetting(key string) bool {
	b, err := sm.Read(key)
	if err != nil {
		sm.lm.Log("error", err.Error(), "settings")
		return false
	}
	bval, ok := b.(bool)
	if ok {
		return bval
	}
	return false
}

// Modify a setting
func (sm *SettingsManager) Modify(key string, value string) {
	err, exists := sm.Exists(key)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "settings")
		return
	}

	// Create a database connection
	db, err := sm.sm.CreateConnection()
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "settings")
		return
	}
	defer db.Close()

	// Check if key is existing
	if !exists {
		msg := fmt.Sprintf("Key %s does not exist", key)
		sm.lm.Log("warn", msg, "settings")
		return
	}

	// Modify a setting
	sm.lm.Log("debug", fmt.Sprintf("modifying setting %s to %s", key, value), "settings")

	// Check the key value type
	var keyType node_types.NullString
	row := db.QueryRowContext(context.Background(), "select setting_type from settings where key = ?;", key)

	err = row.Scan(&keyType)
	if err != nil {
		msg := err.Error()
		sm.lm.Log("error", msg, "settings")
		return
	}
	if !keyType.Valid {
		msg := "Invalid key type"
		sm.lm.Log("error", msg, "settings")
		return
	}

	switch keyType.String {
	case "STRING":
		// Update settings table value_string for the provided key
		_, err = db.ExecContext(context.Background(), "update settings set value_string = ? where key = ?;",
			value, key)
		if err != nil {
			msg := err.Error()
			sm.lm.Log("error", msg, "settings")
			return
		}
	case "JSON":
		// Check if this is valid JSON structure
		if !IsValidJSON(value) {
			msg := fmt.Sprintf("Provided value %v is not a valid JSON structure", value)
			sm.lm.Log("error", msg, "settings")
			return
		}
		// Update settings table value_json for the provided key
		_, err = db.ExecContext(context.Background(), "update settings set value_json = ? where key = ?;",
			value, key)
		if err != nil {
			msg := err.Error()
			sm.lm.Log("error", msg, "settings")
			return
		}
	case "INTEGER":
		num, err := strconv.ParseInt(value, 10, 32) // Base 10, 32-bit integer
		if err != nil {
			msg := err.Error()
			sm.lm.Log("error", msg, "settings")
			return
		}
		// Update settings table value_integer for the provided key
		_, err = db.ExecContext(context.Background(), "update settings set value_integer = ? where key = ?;",
			num, key)
		if err != nil {
			msg := err.Error()
			sm.lm.Log("error", msg, "settings")
			return
		}
	case "BOOLEAN":
		var num int8 = 0
		b, err := strconv.ParseBool(value)
		if err != nil {
			msg := err.Error()
			sm.lm.Log("error", msg, "settings")
			return
		}
		if b {
			num = 1
		}
		// Update settings table value_boolean for the provided key
		_, err = db.ExecContext(context.Background(), "update settings set value_boolean = ? where key = ?;",
			num, key)
		if err != nil {
			msg := err.Error()
			sm.lm.Log("error", msg, "settings")
			return
		}
	case "REAL":
		num, err := strconv.ParseFloat(value, 32)
		if err != nil {
			msg := err.Error()
			sm.lm.Log("error", msg, "settings")
			return
		}
		// Update settings table value_real for the provided key
		_, err = db.ExecContext(context.Background(), "update settings set value_real = ? where key = ?;",
			num, key)
		if err != nil {
			msg := err.Error()
			sm.lm.Log("error", msg, "settings")
			return
		}
	default:
		msg := fmt.Sprintf("Invalid key type, %s", keyType.String)
		sm.lm.Log("error", msg, "settings")
		return
	}
}

// IsValidJSON checks if a string is a valid JSON structure
func IsValidJSON(s string) bool {
	var js any // Can be map or slice
	return json.Unmarshal([]byte(s), &js) == nil
}
