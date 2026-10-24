#!/bin/bash
# generate-certs.sh
# Generates self-signed TLS certificates for the API Gateway (HTTPS)
# and the Auth Service (gRPC TLS).
#
# Output directory: certs/
# Files generated:
#   - ca-key.pem, ca-cert.pem        (local Certificate Authority)
#   - gateway-key.pem, gateway-cert.pem  (API Gateway HTTPS server)
#   - auth-key.pem, auth-cert.pem        (Auth Service gRPC server)

set -euo pipefail

CERTS_DIR="$(cd "$(dirname "$0")/.." && pwd)/certs"
DAYS=365

echo "==> Creating certificates directory: ${CERTS_DIR}"
mkdir -p "${CERTS_DIR}"

# ---- 1. Generate a local CA ----
echo "==> Generating local CA..."
openssl genrsa -out "${CERTS_DIR}/ca-key.pem" 4096 2>/dev/null

openssl req -new -x509 -key "${CERTS_DIR}/ca-key.pem" \
    -out "${CERTS_DIR}/ca-cert.pem" \
    -days ${DAYS} \
    -subj "/C=IT/ST=Dev/L=Dev/O=GoFishing/CN=GoFishing-CA"

# ---- 2. Generate API Gateway certificate (for HTTPS) ----
echo "==> Generating API Gateway certificate..."
openssl genrsa -out "${CERTS_DIR}/gateway-key.pem" 2048 2>/dev/null

openssl req -new -key "${CERTS_DIR}/gateway-key.pem" \
    -out "${CERTS_DIR}/gateway.csr" \
    -subj "/C=IT/ST=Dev/L=Dev/O=GoFishing/CN=localhost"

# SAN (Subject Alternative Name) is required for modern TLS clients
cat > "${CERTS_DIR}/gateway-ext.cnf" <<EOF
subjectAltName = DNS:localhost, IP:127.0.0.1
EOF

openssl x509 -req -in "${CERTS_DIR}/gateway.csr" \
    -CA "${CERTS_DIR}/ca-cert.pem" \
    -CAkey "${CERTS_DIR}/ca-key.pem" \
    -CAcreateserial \
    -out "${CERTS_DIR}/gateway-cert.pem" \
    -days ${DAYS} \
    -extfile "${CERTS_DIR}/gateway-ext.cnf" 2>/dev/null

# ---- 3. Generate Auth Service certificate (for gRPC TLS) ----
echo "==> Generating Auth Service certificate..."
openssl genrsa -out "${CERTS_DIR}/auth-key.pem" 2048 2>/dev/null

openssl req -new -key "${CERTS_DIR}/auth-key.pem" \
    -out "${CERTS_DIR}/auth.csr" \
    -subj "/C=IT/ST=Dev/L=Dev/O=GoFishing/CN=localhost"

cat > "${CERTS_DIR}/auth-ext.cnf" <<EOF
subjectAltName = DNS:localhost, IP:127.0.0.1
EOF

openssl x509 -req -in "${CERTS_DIR}/auth.csr" \
    -CA "${CERTS_DIR}/ca-cert.pem" \
    -CAkey "${CERTS_DIR}/ca-key.pem" \
    -CAcreateserial \
    -out "${CERTS_DIR}/auth-cert.pem" \
    -days ${DAYS} \
    -extfile "${CERTS_DIR}/auth-ext.cnf" 2>/dev/null

# ---- Cleanup temporary files ----
rm -f "${CERTS_DIR}"/*.csr "${CERTS_DIR}"/*.cnf "${CERTS_DIR}"/*.srl

echo ""
echo "==> Certificates generated successfully in ${CERTS_DIR}/"
echo "    CA:      ca-cert.pem, ca-key.pem"
echo "    Gateway: gateway-cert.pem, gateway-key.pem"
echo "    Auth:    auth-cert.pem, auth-key.pem"
