package main

import (
	"context"
	"testing"

	database "ecommerce-project/internal/database/auth"
	"ecommerce-project/internal/proto"

	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestAuthServer implements unit test for the authentication service.
// We use an in-memory SQLite database to ensure isolated and fast tests.
func TestAuthServer(t *testing.T) {
	// Setup: Initialize the in-memory database.
	db, err := database.NewAuthDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer db.Close()

	server := &AuthServer{db: db}
	ctx := context.Background()

	testEmail := "test@example.com"
	testPassword := "securepassword123"
	var createdUserID int64
	var authToken string

	// Subtest 1: Registration of a new user
	t.Run("Register_Success", func(t *testing.T) {
		req := &proto.RegisterRequest{
			Email:    testEmail,
			Password: testPassword,
		}
		res, err := server.Register(ctx, req)
		if err != nil {
			t.Fatalf("Register returned an unexpected error: %v", err)
		}
		if res.UserId <= 0 {
			t.Errorf("Invalid user ID received: %d", res.UserId)
		}
		createdUserID = res.UserId
	})

	// Subtest 2: Registration of a duplicate user (same email)
	t.Run("Register_DuplicateEmail", func(t *testing.T) {
		req := &proto.RegisterRequest{
			Email:    testEmail,
			Password: "anotherpassword",
		}
		_, err := server.Register(ctx, req)
		if err == nil {
			t.Fatal("Error expected for duplicate email, but nothing was returned")
		}
		if status.Code(err) != codes.AlreadyExists {
			t.Errorf("Wrong error code. Expected: %v, Received: %v", codes.AlreadyExists, status.Code(err))
		}
	})

	// Subtest 3: Login with correct credentials
	t.Run("Login_Success", func(t *testing.T) {
		req := &proto.LoginRequest{
			Email:    testEmail,
			Password: testPassword,
		}
		res, err := server.Login(ctx, req)
		if err != nil {
			t.Fatalf("Login failed: %v", err)
		}
		if res.Token == "" {
			t.Fatal("Received token is empty")
		}
		authToken = res.Token
	})

	// Subtest 4: Login with wrong password
	t.Run("Login_WrongPassword", func(t *testing.T) {
		req := &proto.LoginRequest{
			Email:    testEmail,
			Password: "wrongpassword",
		}
		_, err := server.Login(ctx, req)
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("Wrong error code. Expected: %v, Received: %v", codes.Unauthenticated, status.Code(err))
		}
	})

	// Subtest 5: Validation of a valid token
	t.Run("ValidateToken_Success", func(t *testing.T) {
		req := &proto.ValidateTokenRequest{Token: authToken}
		res, err := server.ValidateToken(ctx, req)
		if err != nil {
			t.Fatalf("ValidateToken returned error: %v", err)
		}
		if !res.Valid {
			t.Error("The token should be valid")
		}
		if res.UserId != createdUserID {
			t.Errorf("User ID in the token (%d) does not match the created one (%d)", res.UserId, createdUserID)
		}
	})

	// Subtest 6: Validation of an invalid/corrupted token
	t.Run("ValidateToken_Invalid", func(t *testing.T) {
		req := &proto.ValidateTokenRequest{Token: "not.a.valid.token"}
		res, err := server.ValidateToken(ctx, req)
		if err != nil {
			t.Fatalf("ValidateToken shouldn't return gRPC error for malformed tokens: %v", err)
		}
		if res.Valid {
			t.Error("The token should be identified as invalid")
		}
	})
}
