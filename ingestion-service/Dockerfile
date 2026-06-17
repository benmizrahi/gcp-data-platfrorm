# ==============================================================================
# STAGE 1: BUILD THE BINARY
# ==============================================================================
FROM golang:1.26-alpine AS builder

# Install necessary build dependencies
RUN apk add --no-cache git ca-certificates

WORKDIR /workspace

# Copy go.work and the local modules to satisfy workspace dependencies
COPY go.work ./
COPY schema-managment ./schema-managment
COPY ingestion-service ./ingestion-service

# Compile the ingestion service binary statically
WORKDIR /workspace/ingestion-service
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /app/ingestion-service-bin .

# ==============================================================================
# STAGE 2: RUNTIME ENVIRONMENT
# ==============================================================================
# Distroless static is the gold standard for Go containers: it contains zero shell
# or package utilities, offering a virtually zero-vulnerability attack surface.
FROM gcr.io/distroless/static-debian12:latest AS runner

# Copy root CA certificates for secure TLS connections
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy the compiled statically linked binary
COPY --from=builder /app/ingestion-service-bin /ingestion-service-bin

# Expose ports for HTTP/JSON (8080) and gRPC (50051)
EXPOSE 8080
EXPOSE 50051

# Run the binary
ENTRYPOINT ["/ingestion-service-bin"]
