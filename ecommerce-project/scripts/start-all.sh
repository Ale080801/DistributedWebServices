#!/bin/bash
set -e

echo -e "\e[36mInitializing Ecommerce project...\e[0m"

# Torna alla cartella root del progetto rispetto alla posizione dello script
cd "$(dirname "$0")/.."

echo "Compiling services (to ensure there are no errors)..."
go build ./cmd/catalog-service
go build ./cmd/auth-service
go build ./cmd/inventory-service
go build ./cmd/orders-service
go build ./cmd/api-gateway

echo -e "\e[32mStarting gRPC microservices...\e[0m"

go run ./cmd/catalog-service/main.go &
CATALOG_PID=$!
go run ./cmd/auth-service/main.go &
AUTH_PID=$!
go run ./cmd/inventory-service/main.go &
INVENTORY_PID=$!
go run ./cmd/orders-service/main.go &
ORDERS_PID=$!

echo "Waiting 3 seconds for gRPC backends to start..."
sleep 3

echo -e "\e[32mStarting API Gateway HTTP...\e[0m"
go run ./cmd/api-gateway/main.go &
GATEWAY_PID=$!

# Salva i PID in un file per permettere a stop-all.sh di chiuderli facilmente
echo "$CATALOG_PID $AUTH_PID $INVENTORY_PID $ORDERS_PID $GATEWAY_PID" > scripts/.pids

echo -e "\e[36mDone! Services are running in the background.\e[0m"
echo -e "\e[33mTo shut them all down at once, execute the './scripts/stop-all.sh' script.\e[0m"
