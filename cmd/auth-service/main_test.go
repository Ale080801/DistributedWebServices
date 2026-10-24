package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	database "ecommerce-project/internal/database/auth"
	"ecommerce-project/internal/proto"

	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
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

// generateTestCerts creates a temporary CA and server certificate for testing.
func generateTestCerts(t *testing.T) (certFile, keyFile, caFile string) {
	t.Helper()
	tmpDir := t.TempDir()

	// Generate CA key and certificate
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate CA key: %v", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test CA"},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(1 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}

	caCertDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("Failed to create CA certificate: %v", err)
	}

	caFile = filepath.Join(tmpDir, "ca-cert.pem")
	writePEM(t, caFile, "CERTIFICATE", caCertDER)

	caCert, err := x509.ParseCertificate(caCertDER)
	if err != nil {
		t.Fatalf("Failed to parse CA cert: %v", err)
	}

	// Generate server key and certificate
	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate server key: %v", err)
	}

	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(1 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		DNSNames:     []string{"localhost"},
	}

	serverCertDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("Failed to create server certificate: %v", err)
	}

	certFile = filepath.Join(tmpDir, "server-cert.pem")
	writePEM(t, certFile, "CERTIFICATE", serverCertDER)

	keyFile = filepath.Join(tmpDir, "server-key.pem")
	serverKeyDER, err := x509.MarshalECPrivateKey(serverKey)
	if err != nil {
		t.Fatalf("Failed to marshal server key: %v", err)
	}
	writePEM(t, keyFile, "EC PRIVATE KEY", serverKeyDER)

	return certFile, keyFile, caFile
}

func writePEM(t *testing.T, filename, blockType string, derBytes []byte) {
	t.Helper()
	f, err := os.Create(filename)
	if err != nil {
		t.Fatalf("Failed to create %s: %v", filename, err)
	}
	defer f.Close()
	if err := pem.Encode(f, &pem.Block{Type: blockType, Bytes: derBytes}); err != nil {
		t.Fatalf("Failed to write PEM to %s: %v", filename, err)
	}
}

// TestAuthServer_gRPC_TLS verifies that the Auth Service gRPC server works
// correctly with TLS: a TLS client can connect, while an insecure client fails.
func TestAuthServer_gRPC_TLS(t *testing.T) {
	certFile, keyFile, caFile := generateTestCerts(t)

	// Setup in-memory database
	db, err := database.NewAuthDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer db.Close()

	// Load server TLS credentials
	serverCert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		t.Fatalf("Failed to load server TLS credentials: %v", err)
	}
	serverCreds := credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{serverCert}})

	// Start gRPC server with TLS on a random port
	lis, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("Failed to listen: %v", err)
	}
	grpcServer := grpc.NewServer(grpc.Creds(serverCreds))
	proto.RegisterAuthServiceServer(grpcServer, &AuthServer{db: db})
	go grpcServer.Serve(lis)
	defer grpcServer.Stop()

	serverAddr := lis.Addr().String()

	// --- Test 1: TLS client should connect and work ---
	t.Run("TLS_Client_Success", func(t *testing.T) {
		caCertPEM, err := os.ReadFile(caFile)
		if err != nil {
			t.Fatalf("Failed to read CA cert: %v", err)
		}
		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(caCertPEM) {
			t.Fatal("Failed to add CA cert to pool")
		}
		clientCreds := credentials.NewClientTLSFromCert(certPool, "")

		conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(clientCreds))
		if err != nil {
			t.Fatalf("Failed to connect with TLS: %v", err)
		}
		defer conn.Close()

		client := proto.NewAuthServiceClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Register a user via TLS connection
		resp, err := client.Register(ctx, &proto.RegisterRequest{
			Email:    "tlstest@example.com",
			Password: "securepass",
		})
		if err != nil {
			t.Fatalf("Register over TLS failed: %v", err)
		}
		if resp.UserId <= 0 {
			t.Errorf("Expected valid user ID, got %d", resp.UserId)
		}
	})

	// --- Test 2: Insecure client should fail ---
	t.Run("Insecure_Client_Rejected", func(t *testing.T) {
		conn, err := grpc.NewClient(serverAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			t.Fatalf("Failed to create insecure client: %v", err)
		}
		defer conn.Close()

		client := proto.NewAuthServiceClient(conn)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		_, err = client.Register(ctx, &proto.RegisterRequest{
			Email:    "insecure@example.com",
			Password: "pass",
		})
		if err == nil {
			t.Error("Insecure client should not be able to communicate with a TLS server")
		}
	})
}
