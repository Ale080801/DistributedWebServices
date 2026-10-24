#!/bin/bash

echo -e "\e[33mTerminating all Go processes for the project...\e[0m"

# Torna alla cartella root del progetto
cd "$(dirname "$0")/.."

if [ -f scripts/.pids ]; then
    kill $(cat scripts/.pids) 2>/dev/null || true
    rm scripts/.pids
fi

pkill -f "go run ./cmd/catalog-service" || true
pkill -f "go run ./cmd/auth-service" || true
pkill -f "go run ./cmd/inventory-service" || true
pkill -f "go run ./cmd/orders-service" || true
pkill -f "go run ./cmd/api-gateway" || true

echo -e "\e[32mAll services have been terminated.\e[0m"
