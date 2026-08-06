#!/usr/bin/env bash

set -euo pipefail

DIST_DIR="dist"
BINARY_NAME="codelab-renderer"

echo "Cleaning up dist directory..."
rm -rf "${DIST_DIR}"
mkdir -p "${DIST_DIR}"

# Target platforms in GOOS/GOARCH/OUTPUT_FILE format
PLATFORMS=(
    "darwin/amd64/${BINARY_NAME}-darwin-amd64"
    "darwin/arm64/${BINARY_NAME}-darwin-arm64"
    "linux/amd64/${BINARY_NAME}-linux-amd64"
    "windows/amd64/${BINARY_NAME}-windows-amd64.exe"
)

echo "Starting cross-compilation..."
for PLATFORM in "${PLATFORMS[@]}"; do
    IFS="/" read -r GOOS GOARCH OUTPUT_FILE <<< "${PLATFORM}"
    echo "Building for ${GOOS}/${GOARCH} -> ${DIST_DIR}/${OUTPUT_FILE}..."
    
    # Compile with stripped symbols to reduce binary size
    env GOOS="${GOOS}" GOARCH="${GOARCH}" go build -ldflags="-w -s" -o "${DIST_DIR}/${OUTPUT_FILE}" .
done

echo "Cross-compilation complete! Binaries are available in the '${DIST_DIR}/' directory:"
ls -lh "${DIST_DIR}"
