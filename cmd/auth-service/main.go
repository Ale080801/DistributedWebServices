package main

import (
	"context"
	"crypto/tls"
	"database/sql"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	database "ecommerce-project/internal/database/auth"
	"ecommerce-project/internal/proto"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

const (
	dbPath       = "./data/auth.db"
	grpcPort     = ":50052"
	tlsCertFile  = "./certs/auth-cert.pem"
	tlsKeyFile   = "./certs/auth-key.pem"
)

var jwtKey []byte

func init() {
	// Generates a random key each time the server is restarted.
	// This causes previous tokens (and thus login sessions)
	// to be invalidated each time the server is restarted.
	jwtKey = make([]byte, 32)
	for i := range jwtKey {
		jwtKey[i] = byte(time.Now().UnixNano() + int64(i)) // Simple pseudo-random
	}
}

// AuthServer implements AuthServiceServer defined in the proto file.
type AuthServer struct {
	proto.UnimplementedAuthServiceServer
	db *sql.DB
}

func (s *AuthServer) Register(ctx context.Context, req *proto.RegisterRequest) (*proto.RegisterResponse, error) {
	if req.Email == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password are required")
	}

	// Password hashing with bcrypt before saving to database
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error hashing password: %v", err)
	}

	res, err := s.db.ExecContext(ctx, "INSERT INTO users (email, password_hash) VALUES (?, ?)", req.Email, string(hash))
	if err != nil {
		// Check for specific error of unique constraint (email already exists)
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return nil, status.Error(codes.AlreadyExists, "email already exists")
		}
		return nil, status.Errorf(codes.Internal, "error during registration: %v", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "impossible to retrieve user id: %v", err)
	}

	return &proto.RegisterResponse{UserId: id}, nil
}

func (s *AuthServer) Login(ctx context.Context, req *proto.LoginRequest) (*proto.LoginResponse, error) {
	var userID int64
	var passwordHash string
	var isAdmin bool

	// 1. Search for the user in the DB
	err := s.db.QueryRowContext(ctx, "SELECT id, password_hash, is_admin FROM users WHERE email = ?", req.Email).Scan(&userID, &passwordHash, &isAdmin)
	if err == sql.ErrNoRows {
		return nil, status.Error(codes.NotFound, "user not found")
	}

	// 2. Compares the received password with the hash saved in the DB
	if err := bcrypt.CompareHashAndPassword([]byte(passwordHash), []byte(req.Password)); err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid credentials")
	}

	// 3. Generates the JWT
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  userID,
		"is_admin": isAdmin,
		"exp":      time.Now().Add(time.Hour * 24).Unix(), // expires in 24 hours
	})

	tokenString, err := token.SignedString(jwtKey)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error generating token: %v", err)
	}

	return &proto.LoginResponse{Token: tokenString}, nil
}

// ValidateToken checks if a token is valid and returns the associated user ID.
func (s *AuthServer) ValidateToken(ctx context.Context, req *proto.ValidateTokenRequest) (*proto.ValidateTokenResponse, error) {
	token, err := jwt.Parse(req.Token, func(token *jwt.Token) (interface{}, error) {
		// Checks that the algorithm is the expected one
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, status.Errorf(codes.Unauthenticated, "unexpected signing method: %v", token.Header["alg"])
		}
		return jwtKey, nil
	})

	if err != nil || !token.Valid {
		return &proto.ValidateTokenResponse{Valid: false}, nil
	}

	// Extracts the claims
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		userID := int64(claims["user_id"].(float64))
		isAdmin := false
		if val, ok := claims["is_admin"].(bool); ok {
			isAdmin = val
		}
		return &proto.ValidateTokenResponse{
			UserId:  userID,
			Valid:   true,
			IsAdmin: isAdmin,
		}, nil
	}

	return &proto.ValidateTokenResponse{Valid: false}, nil
}

func main() {
	// Make sure the data directory exists for the SQLite database
	if err := os.MkdirAll("data", 0755); err != nil {
		log.Fatalf("failed to create data directory: %v", err)
	}

	// Initialization of the database dedicated to authentication
	db, err := database.NewAuthDB(dbPath)
	if err != nil {
		log.Fatalf("failed to initialize auth db: %v", err)
	}
	defer db.Close()

	// Setup listener TCP on port 50052
	lis, err := net.Listen("tcp", grpcPort)
	if err != nil {
		log.Fatalf("failed to listen on port %s: %v", grpcPort, err)
	}

	// Load TLS certificate for encrypted gRPC transport
	cert, err := tls.LoadX509KeyPair(tlsCertFile, tlsKeyFile)
	if err != nil {
		log.Fatalf("failed to load TLS credentials: %v", err)
	}
	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}
	creds := credentials.NewTLS(tlsConfig)

	// Creation and registration of the gRPC server with TLS
	srv := grpc.NewServer(grpc.Creds(creds))
	proto.RegisterAuthServiceServer(srv, &AuthServer{db: db})
	reflection.Register(srv)

	log.Printf("Auth Service listening on %s", grpcPort)
	go func() {
		if err := srv.Serve(lis); err != nil {
			log.Fatalf("error during gRPC server execution: %v", err)
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down auth service...")
	srv.GracefulStop()
	log.Println("Service stopped successfully")
}
