package database

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

func NewInventoryDB(dataSourceName string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return nil, err
	}

	if _, err := db.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
		return nil, err
	}

	if err := initInventorySchema(db); err != nil {
		return nil, err
	}

	return db, nil
}

func initInventorySchema(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS stock (
    product_id INTEGER PRIMARY KEY,
    quantity   INTEGER NOT NULL DEFAULT 0 CHECK (quantity >= 0)
);`
	_, err := db.Exec(schema)
	return err
}
