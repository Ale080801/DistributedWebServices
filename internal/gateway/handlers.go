package gateway

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"

	"ecommerce-project/internal/proto"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Gateway handles HTTP requests and forwards them to gRPC services.
type Gateway struct {
	catalog   proto.CatalogClient
	auth      proto.AuthServiceClient
	inventory proto.InventoryServiceClient
	orders    proto.OrdersServiceClient
}

// NewGateway creates a new Gateway instance.
func NewGateway(
	catalog proto.CatalogClient,
	auth proto.AuthServiceClient,
	inventory proto.InventoryServiceClient,
	orders proto.OrdersServiceClient,
) *Gateway {
	return &Gateway{
		catalog:   catalog,
		auth:      auth,
		inventory: inventory,
		orders:    orders,
	}
}

// SetupRoutes configures the HTTP routes.
func (g *Gateway) SetupRoutes() *http.ServeMux {
	mux := http.NewServeMux()

	// --- Web Routes (HTML) ---
	mux.HandleFunc("GET /", g.handleWebHome)
	mux.HandleFunc("GET /login", g.handleWebLogin)
	mux.HandleFunc("GET /register", g.handleWebRegister)
	mux.HandleFunc("GET /cart", g.handleWebCart)
	mux.HandleFunc("GET /product/{id}", g.handleWebProduct)
	mux.HandleFunc("GET /admin", g.handleWebAdmin)
	mux.HandleFunc("GET /orders", g.handleWebOrders)

	// --- REST API Routes (JSON) ---
	// Auth
	mux.HandleFunc("POST /api/auth/register", g.handleAuthRegister)
	mux.HandleFunc("POST /api/auth/login", g.handleAuthLogin)
	mux.HandleFunc("POST /api/auth/logout", g.handleAuthLogout)
	mux.HandleFunc("GET /api/auth/me", g.handleAuthMe)

	// Catalog
	mux.HandleFunc("GET /api/catalog", g.handleCatalogList)

	// Inventory
	mux.HandleFunc("GET /api/inventory/{product_id}", g.handleInventoryGetStock)

	// Orders
	mux.HandleFunc("POST /api/orders", g.handleOrdersCreate)
	mux.HandleFunc("GET /api/orders", g.handleOrdersList)

	// Admin API
	mux.HandleFunc("POST /api/admin/catalog", g.handleAdminCatalogCreate)
	mux.HandleFunc("POST /api/admin/inventory", g.handleAdminInventorySet)
	mux.HandleFunc("POST /api/admin/catalog/{id}/archive", g.handleAdminCatalogArchive)

	return mux
}

// --- Utility methods ---

func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

func writeError(w http.ResponseWriter, err error) {
	st, ok := status.FromError(err)
	if !ok {
		writeJSONResponse(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	statusCode := http.StatusInternalServerError
	switch st.Code() {
	case codes.InvalidArgument:
		statusCode = http.StatusBadRequest
	case codes.NotFound:
		statusCode = http.StatusNotFound
	case codes.Unauthenticated:
		statusCode = http.StatusUnauthorized
	case codes.PermissionDenied:
		statusCode = http.StatusForbidden
	}

	writeJSONResponse(w, statusCode, map[string]string{"error": st.Message()})
}

// getTokenFromCookie extracts the JWT from the auth_token cookie.
func getTokenFromCookie(r *http.Request) (string, error) {
	cookie, err := r.Cookie("auth_token")
	if err != nil {
		return "", err
	}
	return cookie.Value, nil
}

// --- Handlers ---

func (g *Gateway) handleAuthRegister(w http.ResponseWriter, r *http.Request) {
	var req proto.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONResponse(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}

	resp, err := g.auth.Register(context.Background(), &req)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSONResponse(w, http.StatusCreated, resp)
}

func (g *Gateway) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	var req proto.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONResponse(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}

	resp, err := g.auth.Login(context.Background(), &req)
	if err != nil {
		writeError(w, err)
		return
	}

	// Set the JWT in an HttpOnly cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    resp.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   false, // Set to true if using HTTPS
		SameSite: http.SameSiteLaxMode,
	})

	writeJSONResponse(w, http.StatusOK, map[string]string{"message": "Logged in successfully"})
}

func (g *Gateway) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	// Clear the cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		MaxAge:   -1,
	})

	writeJSONResponse(w, http.StatusOK, map[string]string{"message": "Logged out successfully"})
}

func (g *Gateway) handleAdminCatalogArchive(w http.ResponseWriter, r *http.Request) {
	if !g.requireAdmin(r) {
		writeJSONResponse(w, http.StatusForbidden, map[string]string{"error": "Admin access required"})
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeJSONResponse(w, http.StatusBadRequest, map[string]string{"error": "Invalid product ID"})
		return
	}

	resp, err := g.catalog.ArchiveProduct(context.Background(), &proto.ArchiveProductRequest{
		Id: id,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSONResponse(w, http.StatusOK, resp)
}

// handleAuthMe validates the current session cookie and returns the user's info.
// This lets JS check login state without accessing the HttpOnly cookie directly.
func (g *Gateway) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	token, err := getTokenFromCookie(r)
	if err != nil {
		writeJSONResponse(w, http.StatusUnauthorized, map[string]string{"error": "not logged in"})
		return
	}

	valResp, err := g.auth.ValidateToken(context.Background(), &proto.ValidateTokenRequest{Token: token})
	if err != nil || !valResp.Valid {
		writeJSONResponse(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"user_id":   valResp.UserId,
		"logged_in": true,
		"is_admin":  valResp.IsAdmin,
	})
}

func (g *Gateway) handleCatalogList(w http.ResponseWriter, r *http.Request) {
	includeArchived := r.URL.Query().Get("include_archived") == "true"
	resp, err := g.catalog.ListProducts(context.Background(), &proto.ListProductsRequest{
		IncludeArchived: includeArchived,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSONResponse(w, http.StatusOK, resp)
}

func (g *Gateway) handleInventoryGetStock(w http.ResponseWriter, r *http.Request) {
	productIDStr := r.PathValue("product_id")
	productID, err := strconv.ParseInt(productIDStr, 10, 64)
	if err != nil {
		writeJSONResponse(w, http.StatusBadRequest, map[string]string{"error": "Invalid product ID"})
		return
	}

	resp, err := g.inventory.GetStock(context.Background(), &proto.GetStockRequest{
		ProductId: productID,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSONResponse(w, http.StatusOK, resp)
}

func (g *Gateway) handleOrdersCreate(w http.ResponseWriter, r *http.Request) {
	// Extract token from cookie
	token, err := getTokenFromCookie(r)
	if err != nil {
		writeJSONResponse(w, http.StatusUnauthorized, map[string]string{"error": "Missing or invalid auth token"})
		return
	}

	var req struct {
		Items []*proto.OrderItem `json:"items"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONResponse(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}

	// Validate against catalog (prevents ordering archived products and forging prices)
	for _, item := range req.Items {
		p, err := g.catalog.GetProduct(context.Background(), &proto.GetProductRequest{Id: item.ProductId})
		if err != nil {
			writeJSONResponse(w, http.StatusNotFound, map[string]string{"error": "Product not found"})
			return
		}
		if p.IsArchived {
			writeJSONResponse(w, http.StatusBadRequest, map[string]string{"error": "Cannot order an archived product"})
			return
		}
		// Enforce server-side price to prevent client tampering
		item.PriceCents = p.PriceCents
	}

	// Forward to Orders service
	resp, err := g.orders.CreateOrder(context.Background(), &proto.CreateOrderRequest{
		Token: token,
		Items: req.Items,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSONResponse(w, http.StatusCreated, resp)
}

func (g *Gateway) handleOrdersList(w http.ResponseWriter, r *http.Request) {
	token, err := getTokenFromCookie(r)
	if err != nil {
		writeJSONResponse(w, http.StatusUnauthorized, map[string]string{"error": "Missing or invalid auth token"})
		return
	}

	// The current orders.proto ListOrdersRequest takes a user_id, not a token.
	// We need to validate the token first to get the user_id!
	valResp, err := g.auth.ValidateToken(context.Background(), &proto.ValidateTokenRequest{
		Token: token,
	})
	if err != nil || !valResp.Valid {
		writeJSONResponse(w, http.StatusUnauthorized, map[string]string{"error": "Invalid token"})
		return
	}

	resp, err := g.orders.ListOrders(context.Background(), &proto.ListOrdersRequest{
		UserId: valResp.UserId,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSONResponse(w, http.StatusOK, resp)
}

// requireAdmin checks if the current request has a valid admin token.
func (g *Gateway) requireAdmin(r *http.Request) bool {
	token, err := getTokenFromCookie(r)
	if err != nil {
		return false
	}
	valResp, err := g.auth.ValidateToken(context.Background(), &proto.ValidateTokenRequest{
		Token: token,
	})
	if err != nil || !valResp.Valid {
		return false
	}
	return valResp.IsAdmin
}

func (g *Gateway) handleAdminCatalogCreate(w http.ResponseWriter, r *http.Request) {
	if !g.requireAdmin(r) {
		writeJSONResponse(w, http.StatusForbidden, map[string]string{"error": "Forbidden: Admin access required"})
		return
	}

	var req proto.CreateProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONResponse(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}

	// Create in catalog
	resp, err := g.catalog.CreateProduct(context.Background(), &req)
	if err != nil {
		writeError(w, err)
		return
	}

	// Set initial stock in inventory
	_, err = g.inventory.SetStock(context.Background(), &proto.SetStockRequest{
		ProductId: resp.Id,
		Quantity:  req.Stock,
	})
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSONResponse(w, http.StatusCreated, resp)
}

func (g *Gateway) handleAdminInventorySet(w http.ResponseWriter, r *http.Request) {
	if !g.requireAdmin(r) {
		writeJSONResponse(w, http.StatusForbidden, map[string]string{"error": "Forbidden: Admin access required"})
		return
	}

	var req proto.SetStockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONResponse(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}

	resp, err := g.inventory.SetStock(context.Background(), &req)
	if err != nil {
		writeError(w, err)
		return
	}

	writeJSONResponse(w, http.StatusOK, resp)
}
