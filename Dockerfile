# Multi-stage build for optimized Go binary
FROM golang:1.23.4-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY conference-echo-free.go .
COPY conference-webp-ssl.go .

# Build the echo-free conference server with audio feedback prevention
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o conference-webp conference-echo-free.go

# Build the SSL wrapper
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o conference-webp-ssl conference-webp-ssl.go

# Final stage - minimal image
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

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