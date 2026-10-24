package main

import (
	"context"
	"testing"

	database "ecommerce-project/internal/database/orders"
	"ecommerce-project/internal/proto"

	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Mock Auth Client
// Implements proto.AuthServiceClient returning predefined responses,
// without needing a real gRPC server listening.

type mockAuthClient struct {
	validToken string
	userID     int64
}

func (m *mockAuthClient) Register(_ context.Context, _ *proto.RegisterRequest, _ ...grpc.CallOption) (*proto.RegisterResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not used in tests")
}
func (m *mockAuthClient) Login(_ context.Context, _ *proto.LoginRequest, _ ...grpc.CallOption) (*proto.LoginResponse, error) {
	return nil, status.Error(codes.Unimplemented, "not used in tests")
}
func (m *mockAuthClient) ValidateToken(_ context.Context, req *proto.ValidateTokenRequest, _ ...grpc.CallOption) (*proto.ValidateTokenResponse, error) {
	if req.Token == m.validToken {
		return &proto.ValidateTokenResponse{Valid: true, UserId: m.userID}, nil
	}
	return &proto.ValidateTokenResponse{Valid: false}, nil
}

// Mock Inventory Client
// Simulates the behavior of the Inventory Service in memory,
// maintaining an internal state of the stock map.

type mockInventoryClient struct {
	stock map[int64]int64 // product_id → quantity available
}

func (m *mockInventoryClient) GetStock(_ context.Context, req *proto.GetStockRequest, _ ...grpc.CallOption) (*proto.GetStockResponse, error) {
	qty, ok := m.stock[req.ProductId]
	if !ok {
		return nil, status.Error(codes.NotFound, "product not found")
	}
	return &proto.GetStockResponse{ProductId: req.ProductId, Quantity: qty}, nil
}
func (m *mockInventoryClient) ReserveStock(_ context.Context, req *proto.ReserveStockRequest, _ ...grpc.CallOption) (*proto.ReserveStockResponse, error) {
	qty, ok := m.stock[req.ProductId]
	if !ok {
		return nil, status.Error(codes.NotFound, "product not found")
	}
	if qty < req.Quantity {
		return nil, status.Errorf(codes.ResourceExhausted, "stock insufficient: available %d, requested %d", qty, req.Quantity)
	}
	m.stock[req.ProductId] -= req.Quantity
	return &proto.ReserveStockResponse{Success: true, Remaining: m.stock[req.ProductId]}, nil
}
func (m *mockInventoryClient) ReleaseStock(_ context.Context, req *proto.ReleaseStockRequest, _ ...grpc.CallOption) (*proto.ReleaseStockResponse, error) {
	m.stock[req.ProductId] += req.Quantity
	return &proto.ReleaseStockResponse{Success: true}, nil
}
func (m *mockInventoryClient) SetStock(_ context.Context, req *proto.SetStockRequest, _ ...grpc.CallOption) (*proto.SetStockResponse, error) {
	m.stock[req.ProductId] = req.Quantity
	return &proto.SetStockResponse{Success: true}, nil
}

// =============================================================================
// Test
// =============================================================================

func TestOrdersServer(t *testing.T) {
	db, err := database.NewOrdersDB(":memory:")
	if err != nil {
		t.Fatalf("impossible to create test database: %v", err)
	}
	defer db.Close()

	const validToken = "valid-test-token"
	const userID = int64(42)

	authMock := &mockAuthClient{validToken: validToken, userID: userID}
	invMock := &mockInventoryClient{stock: map[int64]int64{
		1: 10,
		2: 5,
	}}

	server := &OrdersServer{
		db:         db,
		authClient: authMock,
		invClient:  invMock,
	}
	ctx := context.Background()

	var createdOrderID int64

	// Subtest 1: successful order creation
	t.Run("CreateOrder_Success", func(t *testing.T) {
		res, err := server.CreateOrder(ctx, &proto.CreateOrderRequest{
			Token: validToken,
			Items: []*proto.OrderItem{
				{ProductId: 1, Quantity: 3, PriceCents: 1000},
				{ProductId: 2, Quantity: 2, PriceCents: 500},
			},
		})
		if err != nil {
			t.Fatalf("CreateOrder returned an error: %v", err)
		}
		if res.OrderId <= 0 {
			t.Errorf("invalid order ID: %d", res.OrderId)
		}
		createdOrderID = res.OrderId

		// Verify that stock has been correctly scaled from the mock
		if invMock.stock[1] != 7 {
			t.Errorf("expected stock 1: 7, received: %d", invMock.stock[1])
		}
		if invMock.stock[2] != 3 {
			t.Errorf("expected stock 2: 3, received: %d", invMock.stock[2])
		}
	})

	// Subtest 2: invalid token → Unauthenticated
	t.Run("CreateOrder_InvalidToken", func(t *testing.T) {
		_, err := server.CreateOrder(ctx, &proto.CreateOrderRequest{
			Token: "token-sbagliato",
			Items: []*proto.OrderItem{{ProductId: 1, Quantity: 1, PriceCents: 1000}},
		})
		if status.Code(err) != codes.Unauthenticated {
			t.Errorf("expected Unauthenticated, received: %v", status.Code(err))
		}
	})

	// Subtest 3: insufficient stock with compensation
	// The second item fails → the first item must be released (stock unchanged)
	t.Run("CreateOrder_InsufficientStock_WithCompensation", func(t *testing.T) {
		stockProduct1Before := invMock.stock[1]

		_, err := server.CreateOrder(ctx, &proto.CreateOrderRequest{
			Token: validToken,
			Items: []*proto.OrderItem{
				{ProductId: 1, Quantity: 1, PriceCents: 1000},  // succeeds
				{ProductId: 2, Quantity: 999, PriceCents: 500}, // fails
			},
		})
		if err == nil {
			t.Fatal("expected error for insufficient stock")
		}
		if status.Code(err) != codes.ResourceExhausted {
			t.Errorf("expected ResourceExhausted, received: %v", status.Code(err))
		}

		// compensation check: stock product 1 must be unchanged
		if invMock.stock[1] != stockProduct1Before {
			t.Errorf("compensation failed: expected stock 1: %d, received: %d",
				stockProduct1Before, invMock.stock[1])
		}
	})

	// Subtest 4: order without items → InvalidArgument
	t.Run("CreateOrder_EmptyItems", func(t *testing.T) {
		_, err := server.CreateOrder(ctx, &proto.CreateOrderRequest{
			Token: validToken,
			Items: []*proto.OrderItem{},
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, received: %v", status.Code(err))
		}
	})

	// Subtest 5: get order with items
	t.Run("GetOrder", func(t *testing.T) {
		res, err := server.GetOrder(ctx, &proto.GetOrderRequest{OrderId: createdOrderID})
		if err != nil {
			t.Fatalf("GetOrder returned an error: %v", err)
		}
		if res.Order.UserId != userID {
			t.Errorf("expected user_id %d, received %d", userID, res.Order.UserId)
		}
		if res.Order.Status != "confirmed" {
			t.Errorf("expected status 'confirmed', received '%s'", res.Order.Status)
		}
		if len(res.Order.Items) != 2 {
			t.Errorf("expected 2 items, received %d", len(res.Order.Items))
		}
	})

	// Subtest 6: order not found → NotFound
	t.Run("GetOrder_NotFound", func(t *testing.T) {
		_, err := server.GetOrder(ctx, &proto.GetOrderRequest{OrderId: 9999})
		if status.Code(err) != codes.NotFound {
			t.Errorf("expected NotFound, received: %v", status.Code(err))
		}
	})

	// Subtest 7: list orders for user - verify that items are included
	t.Run("ListOrders", func(t *testing.T) {
		res, err := server.ListOrders(ctx, &proto.ListOrdersRequest{UserId: userID})
		if err != nil {
			t.Fatalf("ListOrders returned an error: %v", err)
		}
		if len(res.Orders) == 0 {
			t.Error("expected orders in the list, received empty list")
		}
		// verify orders belong to the correct user
		for _, o := range res.Orders {
			if o.UserId != userID {
				t.Errorf("order with wrong user_id: expected %d, received %d", userID, o.UserId)
			}
		}
		// Verify that items are loaded (fix: ListOrders now also loads order_items)
		found := false
		for _, o := range res.Orders {
			if o.OrderId == createdOrderID {
				found = true
				if len(o.Items) != 2 {
					t.Errorf("expected 2 items in order %d, received %d", createdOrderID, len(o.Items))
				}
				break
			}
		}
		if !found {
			t.Errorf("order with ID %d not found in the list", createdOrderID)
		}
	})

	// Subtest 8: list orders for user without orders
	t.Run("ListOrders_Empty", func(t *testing.T) {
		res, err := server.ListOrders(ctx, &proto.ListOrdersRequest{UserId: 9999})
		if err != nil {
			t.Fatalf("ListOrders returned an error: %v", err)
		}
		if len(res.Orders) != 0 {
			t.Errorf("expected 0 orders, received %d", len(res.Orders))
		}
	})
}
