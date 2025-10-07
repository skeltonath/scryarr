# Build stage
FROM golang:1.22-alpine AS builder

# Install build dependencies
RUN apk add --no-cache gcc musl-dev sqlite-dev

WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o scryarr ./cmd/worker

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates sqlite-libs tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/scryarr /app/scryarr

# Create volume mount points
RUN mkdir -p /config /data /data/recommendations /output

# Expose API port
EXPOSE 8080

# Set default environment
ENV TZ=UTC

# Run the service
ENTRYPOINT ["/app/scryarr"]
CMD ["--config", "/config/app.yml", "--categories", "/config/categories.yml"]
