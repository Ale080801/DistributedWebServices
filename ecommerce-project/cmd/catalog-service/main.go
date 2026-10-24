// cmd/catalog-service-go/main.go
package main

import (
	"context"
	"database/sql"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	database "ecommerce-project/internal/database/catalog" // db layer
	"ecommerce-project/internal/proto"                     // catalog.proto (option go_package)

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

const (
	dbPath   = "./data/catalog.db"
	grpcPort = ":50051"
)

// CatalogServer implements Catalog service.
type CatalogServer struct {
	proto.UnimplementedCatalogServer
	db *sql.DB
}

func (s *CatalogServer) ListProducts(ctx context.Context, req *proto.ListProductsRequest) (*proto.ListProductsResponse, error) {
	query := "SELECT id, name, description, category, price_cents, stock, is_archived FROM products WHERE is_archived = 0"
	if req.IncludeArchived {
		query = "SELECT id, name, description, category, price_cents, stock, is_archived FROM products"
	}
	rows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "query products: %v", err)
	}
	defer rows.Close()

	var products []*proto.Product
	for rows.Next() {
		p := &proto.Product{}
		if err := rows.Scan(&p.Id, &p.Name, &p.Description, &p.Category, &p.PriceCents, &p.Stock, &p.IsArchived); err != nil {
			return nil, status.Errorf(codes.Internal, "scan product: %v", err)
		}
		products = append(products, p)
	}
	return &proto.ListProductsResponse{Products: products}, nil
}

func (s *CatalogServer) GetProduct(ctx context.Context, req *proto.GetProductRequest) (*proto.Product, error) {
	row := s.db.QueryRowContext(ctx, "SELECT id, name, description, category, price_cents, stock, is_archived FROM products WHERE id = ?", req.Id)

	p := &proto.Product{}
	err := row.Scan(&p.Id, &p.Name, &p.Description, &p.Category, &p.PriceCents, &p.Stock, &p.IsArchived)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, status.Errorf(codes.NotFound, "product not found")
		}
		return nil, status.Errorf(codes.Internal, "error query product: %v", err)
	}
	return p, nil
}

func (s *CatalogServer) CreateProduct(ctx context.Context, req *proto.CreateProductRequest) (*proto.CreateProductResponse, error) {
	// Price validation: must be greater than zero
	if req.PriceCents <= 0 {
		return nil, status.Error(codes.InvalidArgument, "price must be greater than zero")
	}

	res, err := s.db.ExecContext(ctx,
		"INSERT INTO products (name, description, category, price_cents, stock) VALUES (?, ?, ?, ?, ?)",
		req.Name, req.Description, req.Category, req.PriceCents, req.Stock)

	if err != nil {
		return nil, status.Errorf(codes.Internal, "insert product: %v", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get last insert id: %v", err)
	}

	return &proto.CreateProductResponse{Id: id}, nil
}

// Soft deletes products to maintain data integrity with orders
func (s *CatalogServer) ArchiveProduct(ctx context.Context, req *proto.ArchiveProductRequest) (*proto.ArchiveProductResponse, error) {
	res, err := s.db.ExecContext(ctx, "UPDATE products SET is_archived = 1 WHERE id = ?", req.Id)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update archive: %v", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return nil, status.Errorf(codes.NotFound, "product not found")
	}
	return &proto.ArchiveProductResponse{Success: true}, nil
}

func main() {
	if err := os.MkdirAll("data", 0755); err != nil {
		log.Fatalf("create data dir: %v", err)
	}

	// Initialize DB
	db, err := database.NewCatalogDB(dbPath)
	if err != nil {
		log.Fatalf("init db: %v", err)
	}
	defer db.Close()

	// Create gRPC server
	lis, err := net.Listen("tcp", grpcPort)
	if err != nil {
		log.Fatalf("listen %s: %v", grpcPort, err)
	}

	srv := grpc.NewServer()
	proto.RegisterCatalogServer(srv, &CatalogServer{db: db})
	reflection.Register(srv) // for debug with grpcurl
	// Start server in goroutine
	log.Printf("Catalog service listening on %s", grpcPort)
	go func() {
		if err := srv.Serve(lis); err != nil {
			log.Fatalf("grpc serve: %v", err)
		}
	}()

	// Graceful shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	log.Println("shutting down...")
	srv.GracefulStop()
	log.Println("server stopped")
}
