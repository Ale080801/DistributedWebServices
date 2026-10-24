package database

import (
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestNewInventoryDB(t *testing.T) {
	// Use ":memory:" to ensure the database exists only during the test.
	// This verifies that the connection opening and schema creation (initInventorySchema)
	// occur without SQL syntax errors.
	db, err := NewInventoryDB(":memory:")
	if err != nil {
		t.Fatalf("Error creating in-memory database: %v", err)
	}
	defer db.Close()

	// Verify that the 'stock' table exists and has the correct columns.
	_, err = db.Exec("SELECT product_id, quantity FROM stock LIMIT 0")
	if err != nil {
		t.Errorf("The 'stock' table was not created correctly or the schema is incorrect: %v", err)
	}

	// Verify the CHECK constraint: quantity >= 0 must not allow negative values.
	_, err = db.Exec("INSERT INTO stock (product_id, quantity) VALUES (1, -1)")
	if err == nil {
		t.Error("Expected error for negative quantity, but CHECK constraint did not block insertion")
	}
}
