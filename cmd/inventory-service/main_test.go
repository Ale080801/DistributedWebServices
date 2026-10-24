package main

import (
	"context"
	"testing"

	database "ecommerce-project/internal/database/inventory"
	"ecommerce-project/internal/proto"

	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestInventoryServer implements unit test for the inventory service.
// We use an in-memory SQLite database for isolated and fast tests.
func TestInventoryServer(t *testing.T) {
	db, err := database.NewInventoryDB(":memory:")
	if err != nil {
		t.Fatalf("unable to create test database: %v", err)
	}
	defer db.Close()

	server := &InventoryServer{db: db}
	ctx := context.Background()

	const productID = int64(1)
	const initialStock = int64(10)

	// Subtest 1: initial stock setting
	t.Run("SetStock", func(t *testing.T) {
		res, err := server.SetStock(ctx, &proto.SetStockRequest{ProductId: productID, Quantity: initialStock})
		if err != nil {
			t.Fatalf("SetStock returned an error: %v", err)
		}
		if !res.Success {
			t.Error("SetStock should return Success = true")
		}
	})

	// Subtest 2: reading the stock just set
	t.Run("GetStock", func(t *testing.T) {
		res, err := server.GetStock(ctx, &proto.GetStockRequest{ProductId: productID})
		if err != nil {
			t.Fatalf("GetStock returned an error: %v", err)
		}
		if res.Quantity != initialStock {
			t.Errorf("expected quantity %d, got %d", initialStock, res.Quantity)
		}
	})

	// Subtest 3: partial stock reservation
	t.Run("ReserveStock_Success", func(t *testing.T) {
		const reserveQty = int64(3)
		res, err := server.ReserveStock(ctx, &proto.ReserveStockRequest{ProductId: productID, Quantity: reserveQty})
		if err != nil {
			t.Fatalf("ReserveStock returned an error: %v", err)
		}
		if !res.Success {
			t.Error("ReserveStock should succeed")
		}
		expected := initialStock - reserveQty
		if res.Remaining != expected {
			t.Errorf("expected remaining %d, got %d", expected, res.Remaining)
		}
	})

	// Subtest 4: impossible reservation (requesting more than available)
	t.Run("ReserveStock_Insufficient", func(t *testing.T) {
		_, err := server.ReserveStock(ctx, &proto.ReserveStockRequest{ProductId: productID, Quantity: 999})
		if err == nil {
			t.Fatal("expected error for insufficient stock, but no error received")
		}
		if status.Code(err) != codes.ResourceExhausted {
			t.Errorf("expected error code ResourceExhausted, got: %v", status.Code(err))
		}
	})

	// Subtest 5: stock release (simulates order cancellation)
	t.Run("ReleaseStock", func(t *testing.T) {
		const releaseQty = int64(3)
		res, err := server.ReleaseStock(ctx, &proto.ReleaseStockRequest{ProductId: productID, Quantity: releaseQty})
		if err != nil {
			t.Fatalf("ReleaseStock returned an error: %v", err)
		}
		if !res.Success {
			t.Error("ReleaseStock should succeed")
		}

		// Verify stock has returned to the initial value
		stockRes, _ := server.GetStock(ctx, &proto.GetStockRequest{ProductId: productID})
		if stockRes.Quantity != initialStock {
			t.Errorf("expected stock after release %d, got %d", initialStock, stockRes.Quantity)
		}
	})

	// Subtest 6: operation on non-existent product
	t.Run("GetStock_NotFound", func(t *testing.T) {
		_, err := server.GetStock(ctx, &proto.GetStockRequest{ProductId: 9999})
		if status.Code(err) != codes.NotFound {
			t.Errorf("expected error code NotFound, got: %v", status.Code(err))
		}
	})

	// Subtest 7: SetStock on existing product (UPSERT)
	t.Run("SetStock_Upsert", func(t *testing.T) {
		const newQty = int64(50)
		server.SetStock(ctx, &proto.SetStockRequest{ProductId: productID, Quantity: newQty})
		res, _ := server.GetStock(ctx, &proto.GetStockRequest{ProductId: productID})
		if res.Quantity != newQty {
			t.Errorf("expected stock after upsert %d, got %d", newQty, res.Quantity)
		}
	})
}
