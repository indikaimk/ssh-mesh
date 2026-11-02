# =============================================================================
# Stage 1: Build React + Xterm.js Web Dashboard
# =============================================================================
FROM node:20-alpine AS web-builder
WORKDIR /src/web

COPY web/package.json web/package-lock.json* ./
RUN npm install

COPY web/ ./
RUN npm run build

# =============================================================================
# Stage 2: Build Control Plane Server & Cross-Platform Static Agent Binaries
# =============================================================================
FROM golang:alpine AS go-builder
WORKDIR /src

# Allow Go toolchain to auto-download exact version specified in go.mod if needed
ENV GOTOOLCHAIN=auto

COPY go.mod go.sum ./
RUN go mod download

COPY cmd/ ./cmd/
COPY internal/ ./internal/
COPY pkg/ ./pkg/
COPY scripts/ ./scripts/

# 1. Compile Control Plane Server for target container architecture
ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w" \
    -o /out/ssh-mesh-server \
    ./cmd/server

# 2. Cross-compile static Agent binaries so /api/v1/download/agent & one-line installer work out of the box
RUN mkdir -p /out/bin/dist && \
    CGO_ENABLED=0 GOOS=linux  GOARCH=amd64 go build -ldflags="-s -w" -o /out/bin/dist/ssh-mesh-agent-linux-amd64  ./cmd/agent && \
    CGO_ENABLED=0 GOOS=linux  GOARCH=arm64 go build -ldflags="-s -w" -o /out/bin/dist/ssh-mesh-agent-linux-arm64  ./cmd/agent && \
    CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o /out/bin/dist/ssh-mesh-agent-darwin-amd64 ./cmd/agent && \
    CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o /out/bin/dist/ssh-mesh-agent-darwin-arm64 ./cmd/agent

# =============================================================================
# Stage 3: Minimal Production Runtime Image
# =============================================================================
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata wget

WORKDIR /app

# Copy compiled server, cross-platform agent binaries, and frontend bundle
COPY --from=go-builder /out/ssh-mesh-server /app/bin/ssh-mesh-server
COPY --from=go-builder /out/bin/dist/ /app/bin/dist/
COPY --from=web-builder /src/web/dist/ /app/web/dist/

# Create persistent data directory for SQLite (mesh.db) and session recordings (data/recordings)
RUN mkdir -p /app/data/recordings

ENV SSH_MESH_ADDR=":8080" \
    SSH_MESH_DB_PATH="/app/data/mesh.db" \
    SSH_MESH_WEB_DIR="/app/web/dist" \
    SSH_MESH_ADMIN_USER="admin" \
    SSH_MESH_ADMIN_PASSWORD="admin" \
    SSH_MESH_TOKEN="mesh-secret-token"

VOLUME ["/app/data"]
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget -qO- http://127.0.0.1:8080/api/v1/health || exit 1

ENTRYPOINT ["/app/bin/ssh-mesh-server"]

