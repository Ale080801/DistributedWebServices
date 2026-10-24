package database

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

func NewCatalogDB(dataSourceName string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return nil, err
	}

	if _, err := db.Exec(`PRAGMA foreign_keys = ON;`); err != nil {
		return nil, err
	}

	if err := initCatalogSchema(db); err != nil {
		return nil, err
	}

	return db, nil
}

func initCatalogSchema(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS products (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name TEXT NOT NULL,
    description TEXT,
    category TEXT,
    price_cents INTEGER NOT NULL CHECK (price_cents > 0),
    stock INTEGER NOT NULL DEFAULT 0,
    is_archived BOOLEAN DEFAULT 0
);
`
	_, err := db.Exec(schema)
	if err != nil {
		return err
	}

	// Migration for existing databases to add is_archived column
	_, _ = db.Exec("ALTER TABLE products ADD COLUMN is_archived BOOLEAN DEFAULT 0;")

	return nil
}
