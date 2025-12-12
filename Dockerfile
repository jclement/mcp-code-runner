# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the server binary
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o mcp-sandbox-server ./cmd/server

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/mcp-sandbox-server .

# Copy static files
COPY static ./static

# Create sandbox directory
RUN mkdir -p /var/sandboxes && chmod 755 /var/sandboxes

# Expose default port
EXPOSE 8080

# Run the server
ENTRYPOINT ["/app/mcp-sandbox-server"]
