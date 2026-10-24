package database

import (
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestNewAuthDB(t *testing.T) {
	// Use an in-memory database for a fast and isolated test.
	db, err := NewAuthDB(":memory:")
	if err != nil {
		t.Fatalf("Error while creating the in-memory database: %v", err)
	}
	defer db.Close()

	// Verify that the 'users' table exists and has the correct columns.
	_, err = db.Exec("SELECT id, email, password_hash, created_at FROM users LIMIT 0")
	if err != nil {
		t.Errorf("The 'users' table was not created correctly or the schema is wrong: %v", err)
	}

	// Verify the UNIQUE constraint on email.
	const testEmail = "test@example.com"
	_, err = db.Exec("INSERT INTO users (email, password_hash) VALUES (?, ?)", testEmail, "hash123")
	if err != nil {
		t.Fatalf("Error while inserting the first user: %v", err)
	}

	// Attempt to insert another user with the same email. Should fail.
	_, err = db.Exec("INSERT INTO users (email, password_hash) VALUES (?, ?)", testEmail, "hash456")
	if err == nil {
		t.Error("Error expected for email uniqueness violation, but it was not blocked")
	}
}
