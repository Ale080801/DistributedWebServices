package database

import (
	"database/sql"

	_ "github.com/mattn/go-sqlite3"
)

func NewAuthDB(dataSourceName string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dataSourceName)
	if err != nil {
		return nil, err
	}

	if err := initAuthSchema(db); err != nil {
		return nil, err
	}

	return db, nil
}

func initAuthSchema(db *sql.DB) error {
	schema := `
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    is_admin BOOLEAN DEFAULT FALSE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`
	_, err := db.Exec(schema)
	return err
}
