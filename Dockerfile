# ----------------------------------------------------
# Stage 1: Build binary
# ----------------------------------------------------
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Download dependencies first (go.sum* wildcard prevents failure if missing)
COPY go.mod go.sum* ./
RUN go mod download

# Copy source files
COPY . .

# Compile static binary optimized for container execution
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o route-margin-engine .

# ----------------------------------------------------
# Stage 2: Minimal runtime image
# ----------------------------------------------------
FROM alpine:3.20

# Install SSL certificates for outgoing HTTPS calls
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user for container security
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/route-margin-engine .

USER appuser

EXPOSE 8080

ENTRYPOINT ["/app/route-margin-engine"]