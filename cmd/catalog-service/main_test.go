package main

import (
	"context"
	"testing"

	database "ecommerce-project/internal/database/catalog"
	"ecommerce-project/internal/proto"

	_ "github.com/mattn/go-sqlite3"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// TestCatalogServer implements unit test for the gRPC service methods.
// We test the CatalogServer struct directly by passing an in-memory DB for isolation.
func TestCatalogServer(t *testing.T) {
	// Setup: Initialize an in-memory SQLite database.
	// This approach is ideal for microservices because it does not require an external DB instance.
	db, err := database.NewCatalogDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer db.Close()

	// Create server instance with isolated DB.
	server := &CatalogServer{db: db}
	ctx := context.Background()

	// Define a sample product for testing.
	testProduct := &proto.CreateProductRequest{
		Name:        "Test Fishing Reel",
		Description: "A high quality reel for testing",
		Category:    "reels",
		PriceCents:  4500,
		Stock:       15,
	}

	var createdID int64

	// Subtest 1: Product insertion.
	t.Run("CreateProduct", func(t *testing.T) {
		res, err := server.CreateProduct(ctx, testProduct)
		if err != nil {
			t.Fatalf("CreateProduct returned an error: %v", err)
		}

		// Verify that the returned ID is valid (SQLite starts from 1)
		if res.Id <= 0 {
			t.Errorf("Invalid product ID received: %v", res.Id)
		}
		createdID = res.Id
	})

	// Subtest 2: List verification on a different (empty) database
	// This ensures the service doesn't crash if there are no data.
	t.Run("ListProducts_EmptyDatabase", func(t *testing.T) {
		emptyDb, err := database.NewCatalogDB(":memory:")
		if err != nil {
			t.Fatalf("Failed to create temporary DB: %v", err)
		}
		defer emptyDb.Close()

		emptyServer := &CatalogServer{db: emptyDb}
		res, err := emptyServer.ListProducts(ctx, &proto.ListProductsRequest{})
		if err != nil {
			t.Fatalf("ListProducts on empty DB failed: %v", err)
		}
		if len(res.Products) != 0 {
			t.Errorf("Expected 0 products, received %d", len(res.Products))
		}
	})

	// Subtest 3: Product list retrieval (active only, excludes archived)
	t.Run("ListProducts", func(t *testing.T) {
		res, err := server.ListProducts(ctx, &proto.ListProductsRequest{})
		if err != nil {
			t.Fatalf("ListProducts returned an error: %v", err)
		}

		if len(res.Products) == 0 {
			t.Fatal("The product list is empty, at least one element was expected")
		}

		// Verify exhaustively that all fields match
		found := false
		for _, p := range res.Products {
			if p.Id == createdID {
				found = true
				if p.Name != testProduct.Name ||
					p.Description != testProduct.Description ||
					p.Category != testProduct.Category ||
					p.PriceCents != testProduct.PriceCents ||
					p.Stock != testProduct.Stock {
					t.Errorf("Corrupted product data.\nReceived: %+v\nExpected: %+v", p, testProduct)
				}
				// Verify that the is_archived flag is false by default
				if p.IsArchived {
					t.Errorf("The newly created product should not be archived")
				}
				break
			}
		}

		if !found {
			t.Errorf("The product created with ID %d is not present in the returned list", createdID)
		}
	})

	// Subtest 4: Single product retrieval
	t.Run("GetProduct", func(t *testing.T) {
		res, err := server.GetProduct(ctx, &proto.GetProductRequest{Id: createdID})
		if err != nil {
			t.Fatalf("GetProduct returned an error for a valid ID: %v", err)
		}
		if res.Id != createdID || res.Name != testProduct.Name {
			t.Errorf("Product data mismatch. Expected %s, got %s", testProduct.Name, res.Name)
		}
	})

	// Subtest 5: Non-existent product retrieval
	t.Run("GetProduct_NotFound", func(t *testing.T) {
		_, err := server.GetProduct(ctx, &proto.GetProductRequest{Id: 99999})
		if err == nil {
			t.Fatalf("Error expected for non-existent product, but none was returned")
		}
		// Verify it's a gRPC NotFound error
		st, ok := status.FromError(err)
		if !ok || st.Code() != codes.NotFound {
			t.Errorf("Expected NotFound error code, got %v", err)
		}
	})

	// Subtest 6: Product archiving
	t.Run("ArchiveProduct", func(t *testing.T) {
		res, err := server.ArchiveProduct(ctx, &proto.ArchiveProductRequest{Id: createdID})
		if err != nil {
			t.Fatalf("ArchiveProduct returned an error: %v", err)
		}
		if !res.Success {
			t.Error("ArchiveProduct should return Success = true")
		}
	})

	// Subtest 7: Archived product does not appear in public list
	t.Run("ListProducts_ExcludesArchived", func(t *testing.T) {
		res, err := server.ListProducts(ctx, &proto.ListProductsRequest{IncludeArchived: false})
		if err != nil {
			t.Fatalf("ListProducts returned an error: %v", err)
		}
		for _, p := range res.Products {
			if p.Id == createdID {
				t.Errorf("The archived product (ID %d) should not appear in the public list", createdID)
			}
		}
	})

	// Subtest 8: With include_archived=true the archived product appears in the list
	t.Run("ListProducts_IncludeArchived", func(t *testing.T) {
		res, err := server.ListProducts(ctx, &proto.ListProductsRequest{IncludeArchived: true})
		if err != nil {
			t.Fatalf("ListProducts with include_archived returned an error: %v", err)
		}
		found := false
		for _, p := range res.Products {
			if p.Id == createdID {
				found = true
				if !p.IsArchived {
					t.Errorf("The product should be archived, but IsArchived is false")
				}
				break
			}
		}
		if !found {
			t.Errorf("The archived product (ID %d) did not appear with include_archived=true", createdID)
		}
	})

	// Subtest 9: Archiving a non-existent product → NotFound
	t.Run("ArchiveProduct_NotFound", func(t *testing.T) {
		_, err := server.ArchiveProduct(ctx, &proto.ArchiveProductRequest{Id: 99999})
		if err == nil {
			t.Fatal("Error expected for non-existent product")
		}
		st, ok := status.FromError(err)
		if !ok || st.Code() != codes.NotFound {
			t.Errorf("Expected NotFound error code, got %v", err)
		}
	})
}
