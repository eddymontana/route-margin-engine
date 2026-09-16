# ----------------------------------------------------
# Stage 1: Build binary
# ----------------------------------------------------
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o route-margin-engine .

# ----------------------------------------------------
# Stage 2: Minimal runtime image
# ----------------------------------------------------
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app

# Copy binary and static web assets from builder stage
COPY --from=builder /app/route-margin-engine .
COPY --from=builder /app/static ./static

USER appuser

EXPOSE 8080

ENTRYPOINT ["/app/route-margin-engine"]
