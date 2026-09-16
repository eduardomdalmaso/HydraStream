#!/usr/bin/env bash
set -e

VERSION="1.0.0"
PACKAGE_NAME="hydrastream"
ARCH="amd64"
DIST_DIR="dist"
BUILD_ROOT="/tmp/hydrastream_win_build"

echo "🔨 Cross-compiling HydraStream for Windows (amd64)..."
mkdir -p bin
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o bin/hydrastream.exe ./cmd/hydrastream

echo "📦 Creating Windows distribution zip..."
rm -rf "${BUILD_ROOT}"
mkdir -p "${BUILD_ROOT}/bin"
mkdir -p "${BUILD_ROOT}/samples"
mkdir -p "${BUILD_ROOT}/recordings"

cp bin/hydrastream.exe "${BUILD_ROOT}/"
cp mediamtx.yml "${BUILD_ROOT}/"
cp README.md "${BUILD_ROOT}/"
cp -r samples/* "${BUILD_ROOT}/samples/" 2>/dev/null || true

# Helper script to launch on Windows
cat << 'EOF' > "${BUILD_ROOT}/start_hydrastream.bat"
@echo off
echo Starting HydraStream Data Plane Engine on Windows...
echo Listening on http://localhost:8080
hydrastream.exe
pause
EOF

mkdir -p "${DIST_DIR}"
OUTPUT_ZIP="$(pwd)/${DIST_DIR}/${PACKAGE_NAME}_${VERSION}_windows_${ARCH}.zip"
rm -f "${OUTPUT_ZIP}"

(cd "${BUILD_ROOT}" && zip -r -q "${OUTPUT_ZIP}" .)
rm -rf "${BUILD_ROOT}"


echo "🎉 Successfully built Windows package: ${OUTPUT_ZIP}"
ls -lh "${OUTPUT_ZIP}"
