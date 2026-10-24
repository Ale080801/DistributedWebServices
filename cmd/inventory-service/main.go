package main

import (
	"context"
	"database/sql"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	database "ecommerce-project/internal/database/inventory"
	"ecommerce-project/internal/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

const (
	dbPath   = "./data/inventory.db"
	grpcPort = ":50053"
)

// InventoryServer implements InventoryServiceServer defined in the proto file.
type InventoryServer struct {
	proto.UnimplementedInventoryServiceServer
	db *sql.DB
}

// GetStock returns the quantity available for a product.
func (s *InventoryServer) GetStock(ctx context.Context, req *proto.GetStockRequest) (*proto.GetStockResponse, error) {
	var quantity int64
	err := s.db.QueryRowContext(ctx, "SELECT quantity FROM stock WHERE product_id = ?", req.ProductId).Scan(&quantity)
	if err == sql.ErrNoRows {
		return nil, status.Errorf(codes.NotFound, "product %d not found in inventory", req.ProductId)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error reading stock: %v", err)
	}
	return &proto.GetStockResponse{ProductId: req.ProductId, Quantity: quantity}, nil
}

// ReserveStock atomically reserves stock.
// Uses a conditional UPDATE to ensure quantity does not drop below zero,
// avoiding race conditions without needing explicit locks at the application level.
func (s *InventoryServer) ReserveStock(ctx context.Context, req *proto.ReserveStockRequest) (*proto.ReserveStockResponse, error) {
	if req.Quantity <= 0 {
		return nil, status.Error(codes.InvalidArgument, "quantity to reserve must be greater than zero")
	}

	// This query reserves stock only if there is sufficient availability.
	// If zero rows are modified, the product either doesn't exist or stock is insufficient.
	result, err := s.db.ExecContext(ctx,
		"UPDATE stock SET quantity = quantity - ? WHERE product_id = ? AND quantity >= ?",
		req.Quantity, req.ProductId, req.Quantity,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error updating stock: %v", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error verifying update: %v", err)
	}

	if rowsAffected == 0 {
		// Distinguish "product not found" from "insufficient stock"
		var current int64
		scanErr := s.db.QueryRowContext(ctx, "SELECT quantity FROM stock WHERE product_id = ?", req.ProductId).Scan(&current)
		if scanErr == sql.ErrNoRows {
			return nil, status.Errorf(codes.NotFound, "product %d not found in inventory", req.ProductId)
		}
		return &proto.ReserveStockResponse{Success: false, Remaining: current}, status.Errorf(codes.ResourceExhausted, "stock insufficient: available %d, requested %d", current, req.Quantity)
	}

	// Read the updated value to return to the caller
	var remaining int64
	s.db.QueryRowContext(ctx, "SELECT quantity FROM stock WHERE product_id = ?", req.ProductId).Scan(&remaining)

	return &proto.ReserveStockResponse{Success: true, Remaining: remaining}, nil
}

// ReleaseStock restores stock (e.g., in case of order cancellation).
func (s *InventoryServer) ReleaseStock(ctx context.Context, req *proto.ReleaseStockRequest) (*proto.ReleaseStockResponse, error) {
	if req.Quantity <= 0 {
		return nil, status.Error(codes.InvalidArgument, "quantity to release must be greater than zero")
	}

	result, err := s.db.ExecContext(ctx,
		"UPDATE stock SET quantity = quantity + ? WHERE product_id = ?",
		req.Quantity, req.ProductId,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error releasing stock: %v", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return nil, status.Errorf(codes.NotFound, "product %d not found in inventory", req.ProductId)
	}

	return &proto.ReleaseStockResponse{Success: true}, nil
}

// SetStock sets the initial quantity for a product.
// Uses UPSERT: if the product already exists, updates the quantity.
func (s *InventoryServer) SetStock(ctx context.Context, req *proto.SetStockRequest) (*proto.SetStockResponse, error) {
	if req.Quantity < 0 {
		return nil, status.Error(codes.InvalidArgument, "quantity cannot be negative")
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO stock (product_id, quantity) VALUES (?, ?)
		 ON CONFLICT(product_id) DO UPDATE SET quantity = excluded.quantity`,
		req.ProductId, req.Quantity,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error setting stock: %v", err)
	}

	return &proto.SetStockResponse{Success: true}, nil
}

func main() {
	if err := os.MkdirAll("data", 0755); err != nil {
		log.Fatalf("failed to create data directory: %v", err)
	}

	db, err := database.NewInventoryDB(dbPath)
	if err != nil {
		log.Fatalf("failed to initialize inventory db: %v", err)
	}
	defer db.Close()

	lis, err := net.Listen("tcp", grpcPort)
	if err != nil {
		log.Fatalf("failed to listen on port %s: %v", grpcPort, err)
	}

	srv := grpc.NewServer()
	proto.RegisterInventoryServiceServer(srv, &InventoryServer{db: db})
	reflection.Register(srv)

	log.Printf("Inventory Service listening on %s", grpcPort)
	go func() {
		if err := srv.Serve(lis); err != nil {
			log.Fatalf("error during gRPC server execution: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down inventory service...")
	srv.GracefulStop()
	log.Println("Service stopped successfully")
}
