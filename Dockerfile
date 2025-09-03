# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app
COPY conference.go .
COPY go.mod .
COPY go.sum .

RUN go mod download
RUN go build -o conference conference.go

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/conference .

EXPOSE 8080

CMD ["./conference"]