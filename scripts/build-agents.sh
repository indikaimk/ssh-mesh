#!/usr/bin/env bash
set -euo pipefail

# Build directory
DIST_DIR="bin/dist"
mkdir -p "${DIST_DIR}"

echo "=========================================================="
echo "  Building SSH-Mesh Agent Binaries (Cross-Platform)"
echo "=========================================================="

TARGETS=(
  "linux/amd64"
  "linux/arm64"
  "darwin/amd64"
  "darwin/arm64"
)

for target in "${TARGETS[@]}"; do
  os="${target%%/*}"
  arch="${target##*/}"
  out_name="${DIST_DIR}/ssh-mesh-agent-${os}-${arch}"
  
  echo "--> Compiling ${os}/${arch} -> ${out_name}..."
  CGO_ENABLED=0 GOOS="${os}" GOARCH="${arch}" go build \
    -ldflags="-s -w" \
    -o "${out_name}" \
    ./cmd/agent
done

# Also create the default local binary
echo "--> Compiling local agent -> bin/ssh-mesh-agent..."
go build -o bin/ssh-mesh-agent ./cmd/agent

echo "Build complete! Artifacts in ${DIST_DIR}/:"
ls -lh "${DIST_DIR}"
