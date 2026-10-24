# generate-certs.ps1
# Generates self-signed TLS certificates for the API Gateway (HTTPS)
# and the Auth Service (gRPC TLS).
#
# Requires: OpenSSL must be installed and available in the system PATH.
#   - Install via: winget install ShiningLight.OpenSSL
#   - Or download from: https://slproweb.com/products/Win32OpenSSL.html
#
# Output directory: certs\

$ErrorActionPreference = "Stop"

$ScriptPath = $PSScriptRoot
if ($ScriptPath -eq "") { $ScriptPath = "." }
$ProjectRoot = (Resolve-Path (Join-Path $ScriptPath "..")).Path
$CertsDir = Join-Path $ProjectRoot "certs"
$Days = 365

Write-Host "==> Creating certificates directory: $CertsDir" -ForegroundColor Cyan
New-Item -ItemType Directory -Force -Path $CertsDir | Out-Null

# ---- 1. Generate a local CA ----
Write-Host "==> Generating local CA..." -ForegroundColor Green
openssl genrsa -out "$CertsDir\ca-key.pem" 4096 2>$null

openssl req -new -x509 -key "$CertsDir\ca-key.pem" `
    -out "$CertsDir\ca-cert.pem" `
    -days $Days `
    -subj "/C=IT/ST=Dev/L=Dev/O=GoFishing/CN=GoFishing-CA"

# ---- 2. Generate API Gateway certificate (for HTTPS) ----
Write-Host "==> Generating API Gateway certificate..." -ForegroundColor Green
openssl genrsa -out "$CertsDir\gateway-key.pem" 2048 2>$null

openssl req -new -key "$CertsDir\gateway-key.pem" `
    -out "$CertsDir\gateway.csr" `
    -subj "/C=IT/ST=Dev/L=Dev/O=GoFishing/CN=localhost"

# SAN (Subject Alternative Name) is required for modern TLS clients
$GatewayExtContent = "subjectAltName = DNS:localhost, IP:127.0.0.1"
Set-Content -Path "$CertsDir\gateway-ext.cnf" -Value $GatewayExtContent

openssl x509 -req -in "$CertsDir\gateway.csr" `
    -CA "$CertsDir\ca-cert.pem" `
    -CAkey "$CertsDir\ca-key.pem" `
    -CAcreateserial `
    -out "$CertsDir\gateway-cert.pem" `
    -days $Days `
    -extfile "$CertsDir\gateway-ext.cnf" 2>$null

# ---- 3. Generate Auth Service certificate (for gRPC TLS) ----
Write-Host "==> Generating Auth Service certificate..." -ForegroundColor Green
openssl genrsa -out "$CertsDir\auth-key.pem" 2048 2>$null

openssl req -new -key "$CertsDir\auth-key.pem" `
    -out "$CertsDir\auth.csr" `
    -subj "/C=IT/ST=Dev/L=Dev/O=GoFishing/CN=localhost"

$AuthExtContent = "subjectAltName = DNS:localhost, IP:127.0.0.1"
Set-Content -Path "$CertsDir\auth-ext.cnf" -Value $AuthExtContent

openssl x509 -req -in "$CertsDir\auth.csr" `
    -CA "$CertsDir\ca-cert.pem" `
    -CAkey "$CertsDir\ca-key.pem" `
    -CAcreateserial `
    -out "$CertsDir\auth-cert.pem" `
    -days $Days `
    -extfile "$CertsDir\auth-ext.cnf" 2>$null

# ---- Cleanup temporary files ----
Remove-Item -Force "$CertsDir\*.csr" -ErrorAction SilentlyContinue
Remove-Item -Force "$CertsDir\*.cnf" -ErrorAction SilentlyContinue
Remove-Item -Force "$CertsDir\*.srl" -ErrorAction SilentlyContinue

Write-Host ""
Write-Host "==> Certificates generated successfully in $CertsDir\" -ForegroundColor Cyan
Write-Host "    CA:      ca-cert.pem, ca-key.pem"
Write-Host "    Gateway: gateway-cert.pem, gateway-key.pem"
Write-Host "    Auth:    auth-cert.pem, auth-key.pem"
