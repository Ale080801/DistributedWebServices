package test

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"testing"

	"ecommerce-project/internal/gateway"
	"ecommerce-project/internal/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

// --- Dummy gRPC Servers ---

type dummyAuthServer struct {
	proto.UnimplementedAuthServiceServer
}

func (s *dummyAuthServer) Login(ctx context.Context, req *proto.LoginRequest) (*proto.LoginResponse, error) {
	if req.Email == "integration@test.com" && req.Password == "secret" {
		return &proto.LoginResponse{Token: "integration-token-123"}, nil
	}
	return nil, status.Error(codes.Unauthenticated, "bad login")
}

func (s *dummyAuthServer) ValidateToken(ctx context.Context, req *proto.ValidateTokenRequest) (*proto.ValidateTokenResponse, error) {
	if req.Token == "integration-token-123" {
		return &proto.ValidateTokenResponse{Valid: true, UserId: 99}, nil
	}
	return &proto.ValidateTokenResponse{Valid: false}, nil
}

type dummyOrdersServer struct {
	proto.UnimplementedOrdersServiceServer
	lastItems []*proto.OrderItem
}

func (s *dummyOrdersServer) CreateOrder(ctx context.Context, req *proto.CreateOrderRequest) (*proto.CreateOrderResponse, error) {
	if req.Token != "integration-token-123" {
		return nil, status.Error(codes.Unauthenticated, "invalid token in orders service")
	}
	s.lastItems = req.Items
	return &proto.CreateOrderResponse{OrderId: 888}, nil
}

type dummyCatalogServer struct {
	proto.UnimplementedCatalogServer
}

func (s *dummyCatalogServer) ListProducts(ctx context.Context, req *proto.ListProductsRequest) (*proto.ListProductsResponse, error) {
	products := []*proto.Product{
		{Id: 5, Name: "Integration Product", PriceCents: 2000, IsArchived: false},
		{Id: 9, Name: "Archived Product", PriceCents: 500, IsArchived: true},
	}
	if !req.IncludeArchived {
		var active []*proto.Product
		for _, p := range products {
			if !p.IsArchived {
				active = append(active, p)
			}
		}
		return &proto.ListProductsResponse{Products: active}, nil
	}
	return &proto.ListProductsResponse{Products: products}, nil
}

func (s *dummyCatalogServer) GetProduct(ctx context.Context, req *proto.GetProductRequest) (*proto.Product, error) {
	switch req.Id {
	case 5:
		return &proto.Product{Id: 5, Name: "Integration Product", PriceCents: 2000, IsArchived: false}, nil
	case 9:
		return &proto.Product{Id: 9, Name: "Archived Product", PriceCents: 500, IsArchived: true}, nil
	}
	return nil, status.Errorf(codes.NotFound, "product %d not found", req.Id)
}

// startDummyServer helper
func startDummyServer(t *testing.T, port string, registerFunc func(s *grpc.Server)) *grpc.Server {
	lis, err := net.Listen("tcp", port)
	if err != nil {
		t.Fatalf("Failed to listen on %s (close the microservices): %v", port, err)
	}
	s := grpc.NewServer()
	registerFunc(s)
	go func() {
		if err := s.Serve(lis); err != nil {
			panic(err)
		}
	}()
	return s
}

func TestGateway_Integration(t *testing.T) {
	// 1. Start Dummy gRPC servers on the scheduled ports
	authSrv := startDummyServer(t, ":50052", func(s *grpc.Server) {
		proto.RegisterAuthServiceServer(s, &dummyAuthServer{})
	})
	defer authSrv.Stop()

	ordersDummy := &dummyOrdersServer{}
	ordersSrv := startDummyServer(t, ":50054", func(s *grpc.Server) {
		proto.RegisterOrdersServiceServer(s, ordersDummy)
	})
	defer ordersSrv.Stop()

	catalogSrv := startDummyServer(t, ":50051", func(s *grpc.Server) {
		proto.RegisterCatalogServer(s, &dummyCatalogServer{})
	})
	defer catalogSrv.Stop()

	// 2. Create gRPC clients
	authConn, _ := grpc.NewClient("localhost:50052", grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer authConn.Close()
	authClient := proto.NewAuthServiceClient(authConn)

	ordersConn, _ := grpc.NewClient("localhost:50054", grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer ordersConn.Close()
	ordersClient := proto.NewOrdersServiceClient(ordersConn)

	catalogConn, _ := grpc.NewClient("localhost:50051", grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer catalogConn.Close()
	catalogClient := proto.NewCatalogClient(catalogConn)

	// 3. Start the API Gateway
	gw := gateway.NewGateway(catalogClient, authClient, nil, ordersClient)
	mux := gw.SetupRoutes()
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// --- Integration Test Execution ---
	client := ts.Client() // Http client that automatically saves cookies
	jar, _ := cookiejar.New(nil)
	client.Jar = jar

	// A) Login via Gateway
	loginBody := []byte(`{"email":"integration@test.com", "password":"secret"}`)
	resp, err := client.Post(ts.URL+"/api/auth/login", "application/json", bytes.NewBuffer(loginBody))
	if err != nil {
		t.Fatalf("Login request error: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("Login failed, expected 200 OK, got: %v", resp.Status)
	}

	// Manual cookie verification (httptest.Client() saves it for the next requests)
	cookies := ts.Client().Jar.Cookies(resp.Request.URL)
	var found bool
	for _, c := range cookies {
		if c.Name == "auth_token" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("auth_token cookie not set by Gateway")
	}

	// B) Create Order (should use the cookie saved by the client)
	orderBody := []byte(`{"items": [{"product_id": 5, "quantity": 10}]}`)
	orderReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/orders", bytes.NewBuffer(orderBody))
	orderReq.Header.Set("Content-Type", "application/json")

	// Execute the request (the cookie is added by the client.Jar)
	orderResp, err := client.Do(orderReq)
	if err != nil {
		t.Fatalf("Order request error: %v", err)
	}

	if orderResp.StatusCode != http.StatusCreated {
		t.Fatalf("Order creation failed, expected 201 Created, got: %v", orderResp.Status)
	}

	// Final check on the backend (Dummy Orders)
	if len(ordersDummy.lastItems) != 1 || ordersDummy.lastItems[0].ProductId != 5 {
		t.Fatalf("Order data not received correctly by Orders service")
	}

	// C) Attempt to order an archived product (ID=9) → should fail with 400
	archivedOrderBody := []byte(`{"items": [{"product_id": 9, "quantity": 1}]}`)
	archivedReq, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/orders", bytes.NewBuffer(archivedOrderBody))
	archivedReq.Header.Set("Content-Type", "application/json")
	archivedResp, err := client.Do(archivedReq)
	if err != nil {
		t.Fatalf("Archived product request error: %v", err)
	}
	if archivedResp.StatusCode != http.StatusBadRequest {
		t.Fatalf("Expected 400 Bad Request for archived product, got: %v", archivedResp.Status)
	}
}
