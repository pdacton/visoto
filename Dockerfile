# Build stage
FROM golang:1.25-alpine AS builder

# Install git for fetching dependencies
RUN apk add --no-cache git

# Set working directory
WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o visoto ./cmd/visoto

# Runtime stage
FROM alpine:3.24

# Install ca-certificates for HTTPS requests to LINDAS
RUN apk add --no-cache ca-certificates

# Create non-root user for security
RUN adduser -D -g '' visoto

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/visoto .

# Copy templates, static assets and translation catalogs
COPY --from=builder /app/templates ./templates
COPY --from=builder /app/static ./static
COPY --from=builder /app/locales ./locales

# Copy default config (can be overridden with volume mount)
COPY --from=builder /app/visoto.config ./visoto.config

# Create data directory for monitoring (volume mount point must be owned by the app user)
RUN mkdir -p /app/data

# Change ownership to non-root user
RUN chown -R visoto:visoto /app

# Switch to non-root user
USER visoto

# Expose port (default 8060 from config)
EXPOSE 8060

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8060/ping || exit 1

# Run the application
CMD ["./visoto"]
