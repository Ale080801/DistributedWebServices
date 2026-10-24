package database

import (
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestNewCatalogDB(t *testing.T) {
	// Use ":memory:" to ensure the database exists only during the test.
	// This verifies that the connection opening and schema creation (initCatalogSchema)
	// occur without SQL syntax errors.
	db, err := NewCatalogDB(":memory:")
	if err != nil {
		t.Fatalf("Error creating in-memory database: %v", err)
	}
	defer db.Close()

	// Execute a test query to confirm the 'products' table exists and has the correct columns,
	// including the new is_archived column added for soft-delete.
	_, err = db.Exec("SELECT id, name, description, category, price_cents, stock, is_archived FROM products LIMIT 0")
	if err != nil {
		t.Errorf("The 'products' table was not created correctly or the schema is incorrect: %v", err)
	}
}
