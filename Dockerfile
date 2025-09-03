# Multi-stage build for optimized Go binary
# Use debian-based image for better CGO compatibility
FROM golang:1.23.4-bookworm AS builder

# Install build dependencies (including libwebp for CGO)
RUN apt-get update && apt-get install -y git gcc g++ libwebp-dev && rm -rf /var/lib/apt/lists/*

# Build arguments for deployment info
ARG BUILD_TIME=unknown
ARG BUILD_COMMIT=unknown
ARG BUILD_BY=local
ARG BUILD_REF=unknown

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY conference-echo-free.go .
COPY conference-webp-ssl.go .

# Build the echo-free conference server with deployment info (CGO required for WebP)
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags "-X main.BuildTime=${BUILD_TIME} -X main.BuildCommit=${BUILD_COMMIT} -X main.BuildBy=${BUILD_BY} -X main.BuildRef=${BUILD_REF}" \
    -o conference-webp conference-echo-free.go

# Build the SSL wrapper
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o conference-webp-ssl conference-webp-ssl.go

# Final stage - use debian slim for CGO compatibility
FROM debian:bookworm-slim

# Install ca-certificates for HTTPS and libwebp for runtime
RUN apt-get update && apt-get install -y ca-certificates libwebp7 && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /build/conference-webp .
COPY --from=builder /build/conference-webp-ssl .

# Create directory for SSL certificates
RUN mkdir -p /app/certs

# Expose ports
# 3001 - WebP conference server (internal)
# 443 - HTTPS/WSS (public)
# 80 - HTTP (for Let's Encrypt challenges)
EXPOSE 3001 443 80

# Default to running the SSL wrapper (which starts the WebP server internally)
CMD ["./conference-webp-ssl"]