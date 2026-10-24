package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"ecommerce-project/internal/proto"

	"google.golang.org/grpc"
)

// mockInventoryClient mocka l'inventory per i web_handlers
type mockInventoryClient struct {
	proto.InventoryServiceClient
}

func (m *mockInventoryClient) GetStock(ctx context.Context, in *proto.GetStockRequest, opts ...grpc.CallOption) (*proto.GetStockResponse, error) {
	return &proto.GetStockResponse{ProductId: in.ProductId, Quantity: 10}, nil
}

func TestMain(m *testing.M) {
	// Sposta la working directory alla radice del progetto per far trovare la cartella templates
	if err := os.Chdir("../../"); err != nil {
		panic("Failed to change working directory for tests: " + err.Error())
	}
	os.Exit(m.Run())
}

// test signature fixed

func TestWebHome(t *testing.T) {
	catalogMock := &mockCatalogClient{
		products: []*proto.Product{
			{Id: 1, Name: "Test Product", Category: "Test"},
		},
	}
	inventoryMock := &mockInventoryClient{}

	gateway := &Gateway{
		catalog:   catalogMock,
		inventory: inventoryMock,
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	gateway.handleWebHome(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 OK, got %v", res.StatusCode)
	}

	// Read body to verify content
	bodyBytes, _ := io.ReadAll(res.Body)
	body := string(bodyBytes)
	
	if !strings.Contains(body, "GoFishing") {
		t.Errorf("Expected body to contain shop name, got: %v", body)
	}
	if !strings.Contains(body, "Test Product") {
		t.Errorf("Expected body to contain Test Product")
	}
}

func TestWebLogin(t *testing.T) {
	gateway := &Gateway{}

	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	w := httptest.NewRecorder()

	gateway.handleWebLogin(w, req)

	res := w.Result()
	if res.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200 OK, got %v", res.StatusCode)
	}
}
