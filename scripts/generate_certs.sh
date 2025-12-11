#!/bin/bash

# Generate self-signed TLS certificates for Hypasis Sync Protocol
# Usage: ./scripts/generate_certs.sh [output_dir]

set -e

OUTPUT_DIR="${1:-.}"
CERT_FILE="${OUTPUT_DIR}/cert.pem"
KEY_FILE="${OUTPUT_DIR}/key.pem"
DAYS_VALID=365

echo "Generating self-signed TLS certificates..."
echo "Output directory: ${OUTPUT_DIR}"

# Create output directory if it doesn't exist
mkdir -p "${OUTPUT_DIR}"

# Generate private key and certificate
openssl req -x509 \
  -newkey rsa:4096 \
  -keyout "${KEY_FILE}" \
  -out "${CERT_FILE}" \
  -days ${DAYS_VALID} \
  -nodes \
  -subj "/C=US/ST=State/L=City/O=Hypasis/OU=Sync/CN=hypasis-sync" \
  -addext "subjectAltName=DNS:localhost,DNS:hypasis-sync,IP:127.0.0.1"

echo ""
echo "✅ TLS certificates generated successfully!"
echo "   Certificate: ${CERT_FILE}"
echo "   Private Key: ${KEY_FILE}"
echo "   Valid for: ${DAYS_VALID} days"
echo ""
echo "⚠️  WARNING: These are self-signed certificates for testing only."
echo "   For production, use certificates from a trusted CA (Let's Encrypt, etc.)"
echo ""
echo "To use with Hypasis Sync:"
echo "  hypasis-sync --tls-cert=${CERT_FILE} --tls-key=${KEY_FILE}"
