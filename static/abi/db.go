package abi

import (
	"fmt"

	"github.com/blocklords/sds/db"
)

// Save the ABI in the Database
func SetInDatabase(db *db.Database, a *Abi) error {
	result, err := db.Connection.Exec(`INSERT IGNORE INTO static_abi (abi_id, body) VALUES (?, ?) `, a.Id, a.Bytes)
	if err != nil {
		return fmt.Errorf("abi setting db error: %v", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("checking insert result: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf("expected to have 1 affected rows. Got %d", affected)
	}
	return nil
}

// Get all abis from database
func GetAllFromDatabase(db *db.Database) ([]*Abi, error) {
	rows, err := db.Connection.Query("SELECT body, abi_id FROM static_abi")
	if err != nil {
		return nil, fmt.Errorf("db: %w", err)
	}

	defer rows.Close()

	abis := make([]*Abi, 0)

	// Loop through rows, using Scan to assign column data to struct fields.
	for rows.Next() {
		var bytes []byte
		var abi_id string

		if err := rows.Scan(&bytes, &abi_id); err != nil {
			return nil, fmt.Errorf("failed to scan database result: %w", err)
		}
		abi := Abi{
			Bytes: bytes,
			Id:    abi_id,
		}

		abis = append(abis, &abi)
	}
	return abis, err
}
