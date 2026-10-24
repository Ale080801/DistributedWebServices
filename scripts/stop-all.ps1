Write-Host "Terminating all Go processes for the project..." -ForegroundColor Yellow

# Trova tutti i processi "go" o quelli avviati con "main.exe"
# Per sicurezza, fermiamo i processi generati da "go run" 
Get-Process | Where-Object { $_.MainWindowTitle -match "(Catalog Service|Auth Service|Inventory Service|Orders Service|API Gateway)" -or $_.Name -eq "main" } | Stop-Process -Force -ErrorAction SilentlyContinue

Write-Host "All services have been terminated." -ForegroundColor Green
