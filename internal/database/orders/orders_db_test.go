package database

import (
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestNewOrdersDB(t *testing.T) {
	// Verify that the connection opening and schema creation occur without errors.
	db, err := NewOrdersDB(":memory:")
	if err != nil {
		t.Fatalf("Error during creation of database in memory: %v", err)
	}
	defer db.Close()

	// Verify that the table 'orders' exists with the correct columns.
	_, err = db.Exec("SELECT id, user_id, status, created_at FROM orders LIMIT 0")
	if err != nil {
		t.Errorf("The table 'orders' has not been created correctly: %v", err)
	}

	// Verify that the table 'order_items' exists with the correct columns.
	_, err = db.Exec("SELECT id, order_id, product_id, quantity, price_cents FROM order_items LIMIT 0")
	if err != nil {
		t.Errorf("The table 'order_items' has not been created correctly: %v", err)
	}

	// Verify the CHECK constraint on status: only 'confirmed' and 'cancelled' are allowed.
	_, err = db.Exec("INSERT INTO orders (user_id, status) VALUES (1, 'invalid_status')")
	if err == nil {
		t.Error("Expected error for invalid status, but the CHECK constraint did not block the insertion")
	}

	// Verify the CHECK constraint on quantity: must be > 0.
	db.Exec("INSERT INTO orders (user_id, status) VALUES (1, 'confirmed')") //nolint:errcheck
	_, err = db.Exec("INSERT INTO order_items (order_id, product_id, quantity, price_cents) VALUES (1, 1, 0, 100)")
	if err == nil {
		t.Error("Expected error for quantity = 0, but the CHECK constraint did not block the insertion")
	}
}
