package main

import (
	"context"
	"crypto/x509"
	"database/sql"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	database "ecommerce-project/internal/database/orders"
	"ecommerce-project/internal/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"
)

const (
	dbPath        = "./data/orders.db"
	grpcPort      = ":50054"
	authAddr      = "localhost:50052"
	inventoryAddr = "localhost:50053"
)

// OrdersServer implements OrdersServiceServer defined in the proto file.
// Depends on Auth Service (JWT validation) and Inventory Service.
type OrdersServer struct {
	proto.UnimplementedOrdersServiceServer
	db         *sql.DB
	authClient proto.AuthServiceClient
	invClient  proto.InventoryServiceClient
}

// CreateOrder creates a new order following these steps:
//  1. Validate JWT via Auth Service
//  2. Reserve stock for each product via Inventory Service
//  3. Persist the order in the DB in an atomic transaction
//
// In case of error after partial reservations, the successful reservations
// are cancelled.
func (s *OrdersServer) CreateOrder(ctx context.Context, req *proto.CreateOrderRequest) (*proto.CreateOrderResponse, error) {
	if len(req.Items) == 0 {
		return nil, status.Error(codes.InvalidArgument, "order must contain at least one product")
	}

	// 1. Validate JWT via Auth Service
	authRes, err := s.authClient.ValidateToken(ctx, &proto.ValidateTokenRequest{Token: req.Token})
	if err != nil || !authRes.Valid {
		return nil, status.Error(codes.Unauthenticated, "invalid or expired token")
	}
	userID := authRes.UserId

	// 2. Reserve stock for each product via Inventory Service.
	// Keep track of successful reservations to be able to compensate in case of error.
	reserved := make([]*proto.ReserveStockRequest, 0, len(req.Items))
	for _, item := range req.Items {
		_, err := s.invClient.ReserveStock(ctx, &proto.ReserveStockRequest{
			ProductId: item.ProductId,
			Quantity:  item.Quantity,
		})
		if err != nil {
			// Compensation: release the reservations already made before the failure
			for _, r := range reserved {
				s.invClient.ReleaseStock(ctx, &proto.ReleaseStockRequest{ //nolint:errcheck
					ProductId: r.ProductId,
					Quantity:  r.Quantity,
				})
			}
			return nil, status.Errorf(codes.ResourceExhausted,
				"stock insufficient for product %d: %v", item.ProductId, err)
		}
		reserved = append(reserved, &proto.ReserveStockRequest{
			ProductId: item.ProductId,
			Quantity:  item.Quantity,
		})
	}

	// 3. Persist the order in an atomic SQL transaction
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.releaseAll(ctx, reserved)
		return nil, status.Errorf(codes.Internal, "error opening transaction: %v", err)
	}

	res, err := tx.ExecContext(ctx,
		"INSERT INTO orders (user_id, status) VALUES (?, 'confirmed')", userID)
	if err != nil {
		tx.Rollback() //nolint:errcheck
		s.releaseAll(ctx, reserved)
		return nil, status.Errorf(codes.Internal, "error creating order: %v", err)
	}

	orderID, _ := res.LastInsertId()

	for _, item := range req.Items {
		_, err := tx.ExecContext(ctx,
			"INSERT INTO order_items (order_id, product_id, quantity, price_cents) VALUES (?, ?, ?, ?)",
			orderID, item.ProductId, item.Quantity, item.PriceCents,
		)
		if err != nil {
			tx.Rollback()
			s.releaseAll(ctx, reserved)
			return nil, status.Errorf(codes.Internal, "error inserting order item: %v", err)
		}
	}

	if err := tx.Commit(); err != nil {
		s.releaseAll(ctx, reserved)
		return nil, status.Errorf(codes.Internal, "error committing transaction: %v", err)
	}

	return &proto.CreateOrderResponse{OrderId: orderID}, nil
}

// releaseAll is a helper for compensation: releases all listed stock reservations.
func (s *OrdersServer) releaseAll(ctx context.Context, reserved []*proto.ReserveStockRequest) {
	for _, r := range reserved {
		s.invClient.ReleaseStock(ctx, &proto.ReleaseStockRequest{ //nolint:errcheck
			ProductId: r.ProductId,
			Quantity:  r.Quantity,
		})
	}
}

// GetOrder retrieves the complete details of an order (header + items).
func (s *OrdersServer) GetOrder(ctx context.Context, req *proto.GetOrderRequest) (*proto.GetOrderResponse, error) {
	var order proto.Order
	err := s.db.QueryRowContext(ctx,
		"SELECT id, user_id, status, created_at FROM orders WHERE id = ?", req.OrderId,
	).Scan(&order.OrderId, &order.UserId, &order.Status, &order.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, status.Errorf(codes.NotFound, "order %d not found", req.OrderId)
	}
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error reading order: %v", err)
	}

	rows, err := s.db.QueryContext(ctx,
		"SELECT product_id, quantity, price_cents FROM order_items WHERE order_id = ?", req.OrderId,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error reading items: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		item := &proto.OrderItem{}
		if err := rows.Scan(&item.ProductId, &item.Quantity, &item.PriceCents); err != nil {
			return nil, status.Errorf(codes.Internal, "error scanning item: %v", err)
		}
		order.Items = append(order.Items, item)
	}

	return &proto.GetOrderResponse{Order: &order}, nil
}

// ListOrders returns all orders of a user, in descending order of creation.
// For each order, items are also loaded from the order_items table,
// using order_id as a foreign key.
func (s *OrdersServer) ListOrders(ctx context.Context, req *proto.ListOrdersRequest) (*proto.ListOrdersResponse, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT id, user_id, status, created_at FROM orders WHERE user_id = ? ORDER BY id DESC",
		req.UserId,
	)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "error reading orders: %v", err)
	}

	// Collect all orders and close the cursor before opening another one.
	// SQLite with a single connection does not support concurrent cursors.
	var orders []*proto.Order
	for rows.Next() {
		order := &proto.Order{}
		if err := rows.Scan(&order.OrderId, &order.UserId, &order.Status, &order.CreatedAt); err != nil {
			rows.Close()
			return nil, status.Errorf(codes.Internal, "error scanning order: %v", err)
		}
		orders = append(orders, order)
	}
	rows.Close()

	// Load items for each order using order_id as FK.
	for _, order := range orders {
		itemRows, err := s.db.QueryContext(ctx,
			"SELECT product_id, quantity, price_cents FROM order_items WHERE order_id = ?",
			order.OrderId,
		)
		if err != nil {
			return nil, status.Errorf(codes.Internal, "error reading items order %d: %v", order.OrderId, err)
		}
		for itemRows.Next() {
			item := &proto.OrderItem{}
			if err := itemRows.Scan(&item.ProductId, &item.Quantity, &item.PriceCents); err != nil {
				itemRows.Close()
				return nil, status.Errorf(codes.Internal, "error scanning item: %v", err)
			}
			order.Items = append(order.Items, item)
		}
		itemRows.Close()
	}

	return &proto.ListOrdersResponse{Orders: orders}, nil
}

func main() {
	if err := os.MkdirAll("data", 0755); err != nil {
		log.Fatalf("failed to create data directory: %v", err)
	}

	db, err := database.NewOrdersDB(dbPath)
	if err != nil {
		log.Fatalf("failed to initialize orders db: %v", err)
	}
	defer db.Close()

	// Connection to Auth Service (with TLS)
	authCACert, err := os.ReadFile("./certs/ca-cert.pem")
	if err != nil {
		log.Fatalf("failed to read CA certificate: %v", err)
	}
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(authCACert) {
		log.Fatalf("failed to parse CA certificate")
	}
	authCreds := credentials.NewClientTLSFromCert(certPool, "")

	authConn, err := grpc.NewClient(authAddr, grpc.WithTransportCredentials(authCreds))
	if err != nil {
		log.Fatalf("failed to connect to auth service: %v", err)
	}
	defer authConn.Close()

	// Connection to Inventory Service
	invConn, err := grpc.NewClient(inventoryAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("failed to connect to inventory service: %v", err)
	}
	defer invConn.Close()

	lis, err := net.Listen("tcp", grpcPort)
	if err != nil {
		log.Fatalf("failed to listen on port %s: %v", grpcPort, err)
	}

	srv := grpc.NewServer()
	proto.RegisterOrdersServiceServer(srv, &OrdersServer{
		db:         db,
		authClient: proto.NewAuthServiceClient(authConn),
		invClient:  proto.NewInventoryServiceClient(invConn),
	})
	reflection.Register(srv)

	log.Printf("Orders Service listening on %s", grpcPort)
	go func() {
		if err := srv.Serve(lis); err != nil {
			log.Fatalf("error during gRPC server execution: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down orders service...")
	srv.GracefulStop()
	log.Println("Service stopped successfully")
}
