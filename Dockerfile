# Multi-stage build for optimized Go binary
FROM golang:1.23.4-alpine AS builder

# Install build dependencies (including libwebp for CGO)
RUN apk add --no-cache git gcc g++ musl-dev libwebp-dev

# Build arguments for deployment info
ARG BUILD_TIME=unknown
ARG BUILD_COMMIT=unknown
ARG BUILD_BY=local
ARG BUILD_REF=unknown

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies (with verbose to debug)
RUN go mod download -x

# Copy source code
COPY conference-echo-free.go .
COPY conference-webp-ssl.go .

# Build the echo-free conference server with deployment info (CGO required for WebP)
RUN CGO_ENABLED=1 GOOS=linux go build -a \
    -ldflags "-X main.BuildTime=${BUILD_TIME} -X main.BuildCommit=${BUILD_COMMIT} -X main.BuildBy=${BUILD_BY} -X main.BuildRef=${BUILD_REF}" \
    -o conference-webp conference-echo-free.go

# Build the SSL wrapper
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o conference-webp-ssl conference-webp-ssl.go

# Final stage - minimal image
FROM alpine:latest

# Install ca-certificates for HTTPS and libwebp for runtime
RUN apk --no-cache add ca-certificates libwebp

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