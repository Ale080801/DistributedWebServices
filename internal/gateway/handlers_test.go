package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ecommerce-project/internal/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// --- Mocks ---

type mockCatalogClient struct {
	proto.CatalogClient
	products   []*proto.Product
	productMap map[int64]*proto.Product // per GetProduct by ID
}

func (m *mockCatalogClient) ListProducts(ctx context.Context, in *proto.ListProductsRequest, opts ...grpc.CallOption) (*proto.ListProductsResponse, error) {
	return &proto.ListProductsResponse{Products: m.products}, nil
}

func (m *mockCatalogClient) GetProduct(ctx context.Context, in *proto.GetProductRequest, opts ...grpc.CallOption) (*proto.Product, error) {
	if m.productMap != nil {
		if p, ok := m.productMap[in.Id]; ok {
			return p, nil
		}
	}
	return nil, status.Error(codes.NotFound, "product not found")
}

type mockAuthClient struct {
	proto.AuthServiceClient
	validToken string
	userID     int64
}

func (m *mockAuthClient) Login(ctx context.Context, in *proto.LoginRequest, opts ...grpc.CallOption) (*proto.LoginResponse, error) {
	if in.Email == "test@test.com" && in.Password == "password" {
		return &proto.LoginResponse{Token: m.validToken}, nil
	}
	return nil, status.Error(codes.Unauthenticated, "invalid credentials")
}

func (m *mockAuthClient) ValidateToken(ctx context.Context, in *proto.ValidateTokenRequest, opts ...grpc.CallOption) (*proto.ValidateTokenResponse, error) {
	if in.Token == m.validToken {
		return &proto.ValidateTokenResponse{Valid: true, UserId: m.userID}, nil
	}
	return &proto.ValidateTokenResponse{Valid: false}, nil
}

type mockOrdersClient struct {
	proto.OrdersServiceClient
	lastOrderReq *proto.CreateOrderRequest
}

func (m *mockOrdersClient) CreateOrder(ctx context.Context, in *proto.CreateOrderRequest, opts ...grpc.CallOption) (*proto.CreateOrderResponse, error) {
	m.lastOrderReq = in
	return &proto.CreateOrderResponse{OrderId: 100}, nil
}

// --- Tests ---

func TestGateway_HandleCatalogList(t *testing.T) {
	mockCatalog := &mockCatalogClient{
		products: []*proto.Product{
			{Id: 1, Name: "Test Product"},
		},
	}
	gw := NewGateway(mockCatalog, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/catalog", nil)
	w := httptest.NewRecorder()

	gw.handleCatalogList(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	var respBody proto.ListProductsResponse
	if err := json.NewDecoder(res.Body).Decode(&respBody); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(respBody.Products) != 1 || respBody.Products[0].Name != "Test Product" {
		t.Errorf("Unexpected products in response")
	}
}

func TestGateway_HandleAuthLogin_Success(t *testing.T) {
	mockAuth := &mockAuthClient{validToken: "fake-jwt-token"}
	gw := NewGateway(nil, mockAuth, nil, nil)

	body := []byte(`{"email":"test@test.com", "password":"password"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBuffer(body))
	w := httptest.NewRecorder()

	gw.handleAuthLogin(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", res.Status)
	}

	// Verifica che il cookie sia stato impostato
	cookies := res.Cookies()
	var authToken string
	for _, c := range cookies {
		if c.Name == "auth_token" {
			authToken = c.Value
		}
	}

	if authToken != "fake-jwt-token" {
		t.Errorf("Expected cookie 'auth_token' to be 'fake-jwt-token', got '%s'", authToken)
	}
}

func TestGateway_HandleOrdersCreate_Authenticated(t *testing.T) {
	mockAuth := &mockAuthClient{validToken: "fake-jwt-token", userID: 42}
	mockOrders := &mockOrdersClient{}
	mockCatalog := &mockCatalogClient{
		productMap: map[int64]*proto.Product{
			1: {Id: 1, Name: "Test Product", PriceCents: 1000, IsArchived: false},
		},
	}
	gw := NewGateway(mockCatalog, mockAuth, nil, mockOrders)

	body := []byte(`{"items": [{"product_id": 1, "quantity": 2}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/orders", bytes.NewBuffer(body))

	// Aggiungiamo il cookie di sessione alla richiesta HTTP
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: "fake-jwt-token"})

	w := httptest.NewRecorder()
	gw.handleOrdersCreate(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusCreated {
		t.Errorf("Expected status Created, got %v", res.Status)
	}

	// Verifichiamo che il token sia stato passato correttamente al servizio ordini
	if mockOrders.lastOrderReq == nil || mockOrders.lastOrderReq.Token != "fake-jwt-token" {
		t.Errorf("Token non passato correttamente al servizio ordini")
	}
}

func TestGateway_HandleOrdersCreate_ArchivedProduct(t *testing.T) {
	mockAuth := &mockAuthClient{validToken: "fake-jwt-token", userID: 42}
	mockOrders := &mockOrdersClient{}
	mockCatalog := &mockCatalogClient{
		productMap: map[int64]*proto.Product{
			99: {Id: 99, Name: "Archived Product", PriceCents: 500, IsArchived: true},
		},
	}
	gw := NewGateway(mockCatalog, mockAuth, nil, mockOrders)

	body := []byte(`{"items": [{"product_id": 99, "quantity": 1}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/orders", bytes.NewBuffer(body))
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: "fake-jwt-token"})

	w := httptest.NewRecorder()
	gw.handleOrdersCreate(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status 400 Bad Request for archived product, got %v", res.Status)
	}

	// Verifichiamo che l'ordine non sia stato creato
	if mockOrders.lastOrderReq != nil {
		t.Errorf("L'ordine non avrebbe dovuto essere creato per un prodotto archiviato")
	}
}

func TestGateway_HandleOrdersCreate_Unauthorized(t *testing.T) {
	gw := NewGateway(nil, nil, nil, nil) // Non serve mock se fallisce prima

	body := []byte(`{"items": [{"product_id": 1, "quantity": 2}]}`)
	req := httptest.NewRequest(http.MethodPost, "/api/orders", bytes.NewBuffer(body))

	w := httptest.NewRecorder()
	gw.handleOrdersCreate(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status Unauthorized, got %v", res.Status)
	}
}
