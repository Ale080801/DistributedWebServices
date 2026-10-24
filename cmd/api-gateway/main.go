package main

import (
	"context"
	"crypto/x509"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"ecommerce-project/internal/gateway"
	"ecommerce-project/internal/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func getEnvOrDefault(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func main() {
	// gRPC Endpoints
	catalogAddr := getEnvOrDefault("CATALOG_ADDR", "localhost:50051")
	authAddr := getEnvOrDefault("AUTH_ADDR", "localhost:50052")
	inventoryAddr := getEnvOrDefault("INVENTORY_ADDR", "localhost:50053")
	ordersAddr := getEnvOrDefault("ORDERS_ADDR", "localhost:50054")
	httpPort := getEnvOrDefault("PORT", ":8080")

	log.Println("Starting API Gateway...")

	// Connect to Catalog Service
	catalogConn, err := grpc.NewClient(catalogAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to Catalog Service: %v", err)
	}
	defer catalogConn.Close()
	catalogClient := proto.NewCatalogClient(catalogConn)
	log.Printf("Connected to Catalog Service at %s", catalogAddr)

	// Connect to Auth Service (with TLS — carries passwords)
	authCACert, err := os.ReadFile(getEnvOrDefault("TLS_CA_CERT", "./certs/ca-cert.pem"))
	if err != nil {
		log.Fatalf("Failed to read CA certificate: %v", err)
	}
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(authCACert) {
		log.Fatalf("Failed to parse CA certificate")
	}
	authCreds := credentials.NewClientTLSFromCert(certPool, "")

	authConn, err := grpc.NewClient(authAddr, grpc.WithTransportCredentials(authCreds))
	if err != nil {
		log.Fatalf("Failed to connect to Auth Service: %v", err)
	}
	defer authConn.Close()
	authClient := proto.NewAuthServiceClient(authConn)
	log.Printf("Connected to Auth Service at %s (TLS)", authAddr)

	// Connect to Inventory Service
	inventoryConn, err := grpc.NewClient(inventoryAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to Inventory Service: %v", err)
	}
	defer inventoryConn.Close()
	inventoryClient := proto.NewInventoryServiceClient(inventoryConn)
	log.Printf("Connected to Inventory Service at %s", inventoryAddr)

	// Connect to Orders Service
	ordersConn, err := grpc.NewClient(ordersAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to Orders Service: %v", err)
	}
	defer ordersConn.Close()
	ordersClient := proto.NewOrdersServiceClient(ordersConn)
	log.Printf("Connected to Orders Service at %s", ordersAddr)

	// Setup Gateway Handlers
	gw := gateway.NewGateway(catalogClient, authClient, inventoryClient, ordersClient)
	mux := gw.SetupRoutes()

	// Setup HTTPS Server (TLS — protects credentials in transit)
	tlsCertFile := getEnvOrDefault("TLS_CERT", "./certs/gateway-cert.pem")
	tlsKeyFile := getEnvOrDefault("TLS_KEY", "./certs/gateway-key.pem")

	server := &http.Server{
		Addr:    httpPort,
		Handler: mux,
	}

	go func() {
		log.Printf("API Gateway HTTPS server listening on %s", httpPort)
		if err := server.ListenAndServeTLS(tlsCertFile, tlsKeyFile); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTPS server error: %v", err)
		}
	}()

	// Graceful shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("Shutting down API Gateway...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	}
	log.Println("API Gateway stopped.")
}
