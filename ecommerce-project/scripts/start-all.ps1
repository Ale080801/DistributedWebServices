$ErrorActionPreference = "Stop"

Write-Host "Initializing Ecommerce project..." -ForegroundColor Cyan

# Torna alla cartella root del progetto rispetto alla posizione dello script
$ScriptPath = $PSScriptRoot
if ($ScriptPath -eq "") { $ScriptPath = "." }
Set-Location (Join-Path $ScriptPath "..")

Write-Host "Compiling services (to ensure there are no errors)..."
go build ./cmd/catalog-service
go build ./cmd/auth-service
go build ./cmd/inventory-service
go build ./cmd/orders-service
go build ./cmd/api-gateway

Write-Host "Starting gRPC microservices..." -ForegroundColor Green

Start-Process powershell -ArgumentList "-NoExit -Command `"title Catalog Service; echo 'Starting Catalog...'; go run ./cmd/catalog-service/main.go`""
Start-Process powershell -ArgumentList "-NoExit -Command `"title Auth Service; echo 'Starting Auth...'; go run ./cmd/auth-service/main.go`""
Start-Process powershell -ArgumentList "-NoExit -Command `"title Inventory Service; echo 'Starting Inventory...'; go run ./cmd/inventory-service/main.go`""
Start-Process powershell -ArgumentList "-NoExit -Command `"title Orders Service; echo 'Starting Orders...'; go run ./cmd/orders-service/main.go`""

Write-Host "Waiting 3 seconds for gRPC backends to start..."
Start-Sleep -Seconds 3

Write-Host "Starting API Gateway HTTP..." -ForegroundColor Green
Start-Process powershell -ArgumentList "-NoExit -Command `"title API Gateway; echo 'Starting API Gateway...'; go run ./cmd/api-gateway/main.go`""

Write-Host "Done! 5 new windows have been opened, one for each service." -ForegroundColor Cyan
Write-Host "To shut them all down at once, execute the 'stop-all.ps1' script." -ForegroundColor Yellow
