# Development Stage
FROM docker.io/library/golang:1.25-bookworm AS dev

WORKDIR /app

# Install development tools (Ganti apk dengan apt-get dan build-base dengan build-essential)
RUN apt-get update && apt-get install -y \
    git \
    build-essential \
    bash \
    postgresql-client \
    && rm -rf /var/lib/apt/lists/*

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Install air for hot reload
RUN go install github.com/air-verse/air@latest

# Install migrate tool for DB management
RUN go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

EXPOSE 8080

# Run air for hot reloading during development
CMD ["air", "-c", ".air.toml"]

# ==========================================
# Stage for production build (Biarkan seperti semula)
FROM docker.io/library/golang:1.25-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

# Stage for production image
FROM docker.io/library/alpine:3.20

WORKDIR /app

COPY --from=builder /app/server .
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080

CMD ["./server"]