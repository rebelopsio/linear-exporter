# Multi-stage build for Linear Prometheus exporter
# Stage 1: Builder - Compiles the Go binary
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go mod files and download dependencies
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build arguments for version info
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags="-w -s -X github.com/rebelopsio/linear-exporter/internal/collector.Version=${VERSION} -X github.com/rebelopsio/linear-exporter/internal/collector.Commit=${COMMIT} -X github.com/rebelopsio/linear-exporter/internal/collector.BuildDate=${BUILD_DATE}" \
  -a -installsuffix cgo \
  -o linear-exporter ./cmd/exporter/

# Stage 2: Runtime - Minimal final image
FROM alpine:latest

# Install only runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user for security
RUN addgroup -g 1000 exporter && \
  adduser -D -u 1000 -G exporter exporter

WORKDIR /home/exporter

# Copy compiled binary from builder stage
COPY --from=builder /app/linear-exporter /usr/local/bin/

# Copy timezone data for accurate timestamp handling
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Change ownership to non-root user
RUN chown -R exporter:exporter /home/exporter

# Switch to non-root user
USER exporter

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
  CMD wget --quiet --tries=1 --spider http://localhost:8080/health || exit 1

EXPOSE 8080

# Run the exporter
CMD ["linear-exporter"]
