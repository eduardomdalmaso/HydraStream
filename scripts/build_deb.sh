#!/usr/bin/env bash
set -e

VERSION="1.0.0"
PACKAGE_NAME="hydrastream"
ARCH="amd64"
DIST_DIR="dist"
BUILD_ROOT="/tmp/hydrastream_deb_build"

echo "🔨 Building HydraStream binaries..."
cargo build --release --manifest-path crates/hydra-engine/Cargo.toml
mkdir -p bin
go build -o bin/hydrastream ./cmd/hydrastream

echo "📦 Creating .deb package structure..."
rm -rf "${BUILD_ROOT}"
mkdir -p "${BUILD_ROOT}/DEBIAN"
mkdir -p "${BUILD_ROOT}/opt/hydrastream/bin"
mkdir -p "${BUILD_ROOT}/opt/hydrastream/samples"
mkdir -p "${BUILD_ROOT}/opt/hydrastream/recordings"
mkdir -p "${BUILD_ROOT}/etc/systemd/system"

# Copy binaries & assets
cp bin/hydrastream "${BUILD_ROOT}/opt/hydrastream/"
cp bin/mediamtx "${BUILD_ROOT}/opt/hydrastream/bin/"
cp mediamtx.yml "${BUILD_ROOT}/opt/hydrastream/"
cp -r samples/* "${BUILD_ROOT}/opt/hydrastream/samples/" 2>/dev/null || true

# Systemd Unit File
cat << 'EOF' > "${BUILD_ROOT}/etc/systemd/system/hydrastream.service"
[Unit]
Description=HydraStream High-Performance Data Plane Engine
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/opt/hydrastream
ExecStart=/opt/hydrastream/hydrastream
Restart=always
RestartSec=3

Environment=PORT=8080
Environment=NATS_URL=nats://localhost:4222
Environment=MEDIAMTX_WHEP_URL=http://localhost:8889
Environment=JWT_SECRET=super-secret-key-that-is-at-least-32-chars-long-for-production!

LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

# DEBIAN Control File
cat << EOF > "${BUILD_ROOT}/DEBIAN/control"
Package: ${PACKAGE_NAME}
Version: ${VERSION}
Section: video
Priority: optional
Architecture: ${ARCH}
Maintainer: Hydra Vision Team <contact@hydrastream.io>
Description: High-Performance Zero-Copy RTSP Ingest, WebRTC WHEP and Data Plane Engine
 High-throughput RFC 2326 TCP video ingestion, POSIX SHM ring buffers,
 CUDA IPC hardware acceleration and WebRTC HTTP Egress Protocol gateway.
EOF

# Post-Install Script
cat << 'EOF' > "${BUILD_ROOT}/DEBIAN/postinst"
#!/bin/sh
set -e
chmod +x /opt/hydrastream/hydrastream /opt/hydrastream/bin/mediamtx
systemctl daemon-reload
systemctl enable --now hydrastream.service || true
echo "✅ HydraStream installed and started on http://localhost:8080"
EOF
chmod 755 "${BUILD_ROOT}/DEBIAN/postinst"

# Pre-Remove Script
cat << 'EOF' > "${BUILD_ROOT}/DEBIAN/prerm"
#!/bin/sh
set -e
systemctl stop hydrastream.service || true
systemctl disable hydrastream.service || true
EOF
chmod 755 "${BUILD_ROOT}/DEBIAN/prerm"

# Post-Remove Script
cat << 'EOF' > "${BUILD_ROOT}/DEBIAN/postrm"
#!/bin/sh
set -e
systemctl daemon-reload || true
EOF
chmod 755 "${BUILD_ROOT}/DEBIAN/postrm"

mkdir -p "${DIST_DIR}"
OUTPUT_DEB="${DIST_DIR}/${PACKAGE_NAME}_${VERSION}_${ARCH}.deb"

if command -v dpkg-deb >/dev/null 2>&1; then
    dpkg-deb --build "${BUILD_ROOT}" "${OUTPUT_DEB}"
else
    # Fallback to standard POSIX ar + tar builder
    echo "ℹ️ dpkg-deb not found, using universal ar/tar builder..."
    TMP_ARCHIVE="/tmp/hydrastream_ar_build"
    rm -rf "${TMP_ARCHIVE}"
    mkdir -p "${TMP_ARCHIVE}"

    echo "2.0" > "${TMP_ARCHIVE}/debian-binary"
    tar --owner=0 --group=0 -czf "${TMP_ARCHIVE}/control.tar.gz" -C "${BUILD_ROOT}/DEBIAN" .
    tar --owner=0 --group=0 -czf "${TMP_ARCHIVE}/data.tar.gz" --exclude='./DEBIAN' -C "${BUILD_ROOT}" .

    (cd "${TMP_ARCHIVE}" && ar rcs "../hydra_final.deb" debian-binary control.tar.gz data.tar.gz)
    mv /tmp/hydra_final.deb "${OUTPUT_DEB}"
    rm -rf "${TMP_ARCHIVE}"
fi

rm -rf "${BUILD_ROOT}"
echo "🎉 Successfully built package: ${OUTPUT_DEB}"
ls -lh "${OUTPUT_DEB}"
