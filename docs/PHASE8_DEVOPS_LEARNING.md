# 🐳 Phase 8: Deployment & DevOps — Learning Guide

> Panduan belajar DevOps untuk project Invoice Maker.
> Dimulai dari setup yang sudah ada, kita akan memahami **kenapa** dan **bagaimana**
> setiap konsep DevOps diterapkan.

---

## 📋 Prasyarat Belajar

Pastikan kamu sudah menyelesaikan Phase 1–6 dan paham:
- Cara aplikasi berjalan (Go backend + React frontend + PostgreSQL)
- Docker dasar (container vs image, Dockerfile, docker-compose)
- Struktur project (monorepo: `backend/` + `frontend/`)

---

## 🔍 Audit Kondisi Saat Ini

Sebelum memperbaiki sesuatu, kita harus **mengerti dulu apa yang sudah ada** dan apa masalahnya.

### ✅ Yang Sudah Ada

| Komponen | Status | File |
|----------|--------|------|
| Dockerfile backend | Multi-stage (dev + builder + prod) | `backend/Dockerfile` |
| Dockerfile frontend | Multi-stage (bun build + nginx serve) | `frontend/Dockerfile` |
| docker-compose dev | PostgreSQL + backend (air hot reload) + frontend (nginx) | `docker-compose.yml` |
| docker-compose prod | Sama seperti dev tapi pakai .env.prod | `docker-compose.prod.yml` |
| Dev container | VS Code Dev Container siap pakai | `.devcontainer/` |
| Deploy script | Script bash untuk production deploy | `deploy-production.sh` |
| Dev script | Script bash untuk local development | `dev-local.sh` |
| Health check | PostgreSQL health check ada | `docker-compose.yml` (postgres only) |
| Reverse proxy | Nginx proxy `/api` ke backend | `frontend/nginx.conf` |

### ❌ Yang Perlu Diperbaiki (Target Phase 8)

| Masalah | Dampak |
|---------|--------|
| Backend **tidak punya health check** | Docker tidak tahu apakah backend benar-benar siap |
| Frontend **tidak punya health check** | Sama seperti di atas |
| Backend image **final size besar** (~400MB+) | Waste bandwidth, slow deploy |
| Dockerfile frontend pakai `bun:1.1` (lama) | Missing security patches |
| Dockerfile backend dev stage install `build-essential` (berat) | Build lama, image size besar |
| **Tidak ada `.dockerignore`** | Semua file dikirim ke Docker context (node_modules, .git, dll) |
| Frontend docker-compose **tidak pakai multi-stage `target: dev`** | Dev environment frontend di-container dengan nginx (tidak bisa hot reload) |
| Logging masih pakai `log.Printf` (tidak terstruktur) | Sulit di-search/di-filter saat debug di production |
| **Tidak ada CI/CD pipeline** | Manual test & deploy = human error + lambat |
| **Tidak ada monitoring** (Prometheus/Grafana) | Tidak tahu kalau aplikasi lemot sampai user komplain |
| **Tidak ada error tracking** (Sentry) | Bug di production baru ketahuan kalau user lapor |
| JWT secret di dev hardcoded & pendek | Security risk walau development |
| Podman-compose punya isu networking (tertulis di TODO) | Backend mungkin gagal resolve `postgres` hostname |

---

## 📚 1. Container & Orchestration

### 1.1 Konsep: Multi-Stage Docker Build

**Kenapa perlu?** Image Docker yang kamu deploy ke production harus sekecil mungkin karena:
- Lebih cepat di-pull ke server
- Lebih sedikit attack surface (lebih sedikit package = lebih sedikit vulnerability)
- Hemat storage di container registry

Project ini **sudah pakai multi-stage**, tapi ada ruang improvement.

#### Backend Dockerfile (analisis)

```dockerfile
# backend/Dockerfile — yang sudah ada

# Stage 1: Dev (pakai golang:1.25-bookworm, ~900MB)
FROM docker.io/library/golang:1.25-bookworm AS dev
# ... install git, build-essential, postgresql-client ...

# Stage 2: Builder (pakai golang:1.25-alpine, ~300MB)
FROM docker.io/library/golang:1.25-alpine AS builder
# ... compile binary ...

# Stage 3: Production (pakai alpine:3.20, ~7MB + binary ~15MB)
FROM docker.io/library/alpine:3.20
# ... copy binary & migrations ...
```

**Flow:** `dev (besar, lengkap) → builder (compile) → prod (minimal, hanya binary)`

✅ **Yang sudah bagus:**
- Multi-stage sudah benar — dev stage pakai image besar dengan tools lengkap, production cuma alpine + binary
- Production stage hanya copy binary + migrations, tidak ada source code

⚠️ **Yang bisa di-improve:**
1. `golang:1.25-bookworm` di dev stage bisa diganti `golang:1.25-alpine` juga (lebih kecil)
2. Production stage bisa pakai `scratch` (image kosong) atau `gcr.io/distroless/static` (lebih aman)
3. Belum ada `.dockerignore`

#### Frontend Dockerfile (analisis)

```dockerfile
# frontend/Dockerfile — yang sudah ada

# Stage 1: Build pakai bun
FROM docker.io/oven/bun:1.1 AS builder
# ... npm install + build ...

# Stage 2: Serve pakai nginx:alpine
FROM docker.io/library/nginx:1.27-alpine
# ... copy dist + nginx config ...
```

✅ **Yang sudah bagus:**
- Build tool (bun) tidak ikut ke production image
- Nginx alpine sudah cukup kecil (~40MB)

⚠️ **Yang bisa di-improve:**
1. **Tidak ada dev stage** untuk development dengan hot reload (docker-compose.yml mount source tapi tetep pakai nginx)
2. `bun:1.1` bisa di-upgrade

#### Improvement: Tambahkan `.dockerignore`

```dockerignore
# .dockerignore — buat file ini di root project

# Dependencies
**/node_modules
**/vendor

# Build output
frontend/dist
backend/server

# Git
.git
.gitignore

# IDE
.vscode
.idea

# Docs & misc
*.md
docs/
.superpowers/
.claude/

# Environment
.env
.env.*
!.env.example

# Tests & coverage
*_test.go
coverage.out
coverage.html

# OS files
.DS_Store
Thumbs.db
```

**Kenapa ini penting?** Tanpa `.dockerignore`, `COPY . .` akan mengirim **semua file di project** ke Docker daemon. Ini bikin `docker build` lambat karena context size besar, dan bisa bikin cache invalid.

---

### 1.2 Konsep: Docker Networking & DNS

**Masalah aktual di project ini:** TODO.md menulis "Setup Docker networking properly (fix current Podman issues)".

#### Bagaimana container saling berkomunikasi?

```
┌──────────────────────────────────────────┐
│         Docker Network: invoice_default   │
│                                          │
│  ┌─────────┐   ┌─────────┐   ┌────────┐ │
│  │frontend │──▶│backend:8080│──▶│postgres│ │
│  │  :80    │   │  :8080   │   │ :5432  │ │
│  └─────────┘   └─────────┘   └────────┘ │
│       ▲              ▲             ▲      │
│       │              │             │      │
│   Port 3000      Port 8080    Port 5432  │
│   (host)         (host)       (host)     │
└──────────────────────────────────────────┘
```

**Key points:**
1. Setiap service di `docker-compose.yml` otomatis dapat hostname sesuai **nama service**-nya
2. `frontend` bisa akses `http://backend:8080` — ini yang dipakai di `nginx.conf`
3. `backend` bisa akses `postgres:5432` — ini yang dipakai di env `DB_HOST=postgres`
4. Container **di dalam** docker network bisa saling komunikasi tanpa expose port ke host

#### Podman vs Docker Networking Issues

Podman (yang kamu pakai) punya behavior networking yang berbeda:

| Aspek | Docker | Podman |
|-------|--------|--------|
| Default network driver | `bridge` | `bridge` (tapi isolated per pod) |
| DNS resolution | Otomatis antar container | Kadang perlu config `--dns` |
| `depends_on` behavior | Wait for start (bukan ready) | Sama, tapi bisa race condition |
| Rootless networking | Pakai `slirp4netns` | Default rootless, kadang lambat |

**Fix untuk Podman networking:**

```yaml
# docker-compose.yml — tambahkan network explicit
services:
  backend:
    # ... existing config ...
    networks:
      - invoice-net

  frontend:
    # ... existing config ...
    networks:
      - invoice-net

  postgres:
    # ... existing config ...
    networks:
      - invoice-net

networks:
  invoice-net:
    driver: bridge
```

Dan gunakan health check + `depends_on` condition `service_healthy` (seperti yang sudah ada untuk postgres). Ini mencegah race condition di mana backend mencoba konek ke postgres sebelum postgres benar-benar siap menerima koneksi.

---

### 1.3 Konsep: Health Checks

**Kenapa perlu?** Docker perlu tahu apakah container kamu benar-benar "ready" atau cuma "running". Container bisa "running" tapi aplikasi di dalamnya belum siap (misal: masih connect database).

Project ini **sudah punya health check untuk PostgreSQL saja**:

```yaml
# docker-compose.yml (existing)
postgres:
  healthcheck:
    test: ["CMD-SHELL", "pg_isready -U invoiceuser -d invoicedb"]
    interval: 5s
    timeout: 5s
    retries: 5
```

#### Tambahkan Health Check untuk Backend

**Step 1: Buat endpoint health check di Go**

```go
// backend/main.go — tambahkan di setup router

// Health check endpoint — beri tahu Docker bahwa aplikasi siap
r.GET("/api/health", func(c *gin.Context) {
    // Cek apakah database bisa di-query
    var one int
    err := db.QueryRow(c.Request.Context(), "SELECT 1").Scan(&one)
    if err != nil {
        c.JSON(http.StatusServiceUnavailable, gin.H{
            "status": "unhealthy",
            "error":  "database unreachable",
        })
        return
    }
    c.JSON(http.StatusOK, gin.H{
        "status":    "healthy",
        "timestamp": time.Now().UTC(),
    })
})
```

**Step 2: Tambahkan health check di docker-compose**

```yaml
# docker-compose.yml — tambahkan di service backend
backend:
  healthcheck:
    test: ["CMD-SHELL", "wget -qO- http://localhost:8080/api/health | grep -q healthy"]
    interval: 10s
    timeout: 5s
    retries: 3
    start_period: 10s  # kasih waktu buat startup + migrasi DB
```

**Kenapa `start_period`?** Backend kamu jalanin migrasi database otomatis saat startup. `start_period` kasih grace period di mana health check failure tidak dihitung sebagai "unhealthy". Ini mencegah container di-restart sebelum migrasi selesai.

#### Konsep: Health Check Endpoint yang Benar

Health check yang baik harus cek **dependencies**, bukan cuma return 200:

```go
// ❌ Buruk — selalu return OK walau DB mati
func healthHandler(c *gin.Context) {
    c.JSON(200, gin.H{"status": "ok"})
}

// ❌ Juga buruk — terlalu berat (jalanin full query)
func healthHandler(c *gin.Context) {
    rows, _ := db.Query("SELECT * FROM invoices WHERE ...")
    // ...
}

// ✅ Baik — cek DB connection dengan query ringan
func healthHandler(c *gin.Context) {
    err := db.Ping()
    if err != nil {
        c.JSON(503, gin.H{"status": "down", "db": "unreachable"})
        return
    }
    c.JSON(200, gin.H{"status": "healthy"})
}
```

---

### 1.4 Konsep: Proper Logging

**Kenapa sekarang belum bagus?** Go `log.Printf` tidak menghasilkan structured logs. Di production, kamu mau log dalam format JSON supaya bisa di-search dan di-filter (misal: "tampilkan semua ERROR dari user X dalam 1 jam terakhir").

#### Standard Library: `log/slog` (Go 1.21+)

Go 1.21 memperkenalkan package `log/slog` untuk structured logging. Project kamu pakai Go 1.25, jadi sudah tersedia!

```go
// backend/main.go — contoh migrasi dari log ke slog

import "log/slog"

func main() {
    // Development: text format yang enak dibaca
    // Production: JSON format yang bisa diparse oleh Grafana/Sentry
    var handler slog.Handler

    if os.Getenv("ENV") == "production" {
        handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
            Level: slog.LevelInfo,
        })
    } else {
        handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
            Level: slog.LevelDebug,
        })
    }

    logger := slog.New(handler)
    slog.SetDefault(logger) // semua code yang pakai slog.Info() akan pakai logger ini

    // Contoh usage:
    slog.Info("server starting", "port", 8080, "env", os.Getenv("ENV"))
    // Output (text):   INFO server starting port=8080 env=development
    // Output (JSON):   {"time":"...","level":"INFO","msg":"server starting","port":8080,"env":"development"}

    // Error handling dengan context:
    if err != nil {
        slog.Error("database connection failed",
            "error", err,
            "host", dbHost,
            "retry_attempt", i,
        )
    }
}
```

#### Kenapa Structured Logging?

| Format | Contoh | Bisa dicari? |
|--------|--------|-------------|
| `log.Printf` | `2024/01/15 10:30:45 User sekuyy login` | ❌ Harus pakai regex |
| `slog` (JSON) | `{"time":"...","level":"INFO","msg":"user login","username":"sekuyy"}` | ✅ `jq '.username'` atau Grafana query |

Dengan JSON logs, kamu bisa query seperti: `level=ERROR` atau `username=sekuyy` di monitoring tools.

#### Logging di Frontend

Di frontend, ganti `console.log` dengan structured approach:

```typescript
// frontend/src/lib/logger.ts
type LogLevel = 'debug' | 'info' | 'warn' | 'error';

interface LogEntry {
    level: LogLevel;
    message: string;
    timestamp: string;
    context?: Record<string, unknown>;
}

function log(level: LogLevel, message: string, context?: Record<string, unknown>) {
    const entry: LogEntry = {
        level,
        message,
        timestamp: new Date().toISOString(),
        context,
    };

    // Production: kirim ke backend atau Sentry
    if (import.meta.env.PROD) {
        // Kirim ke endpoint logging atau Sentry
        fetch('/api/logs', {
            method: 'POST',
            body: JSON.stringify(entry),
            keepalive: true, // tetap kirim walau user navigasi
        });
    }

    // Development: pretty print
    if (import.meta.env.DEV) {
        const emoji = { debug: '🔍', info: 'ℹ️', warn: '⚠️', error: '❌' }[level];
        console[level](`${emoji} ${message}`, context);
    }
}

export const logger = {
    debug: (msg: string, ctx?: Record<string, unknown>) => log('debug', msg, ctx),
    info:  (msg: string, ctx?: Record<string, unknown>) => log('info', msg, ctx),
    warn:  (msg: string, ctx?: Record<string, unknown>) => log('warn', msg, ctx),
    error: (msg: string, ctx?: Record<string, unknown>) => log('error', msg, ctx),
};
```

---

### 1.5 Optimasi Docker Image Size

#### Backend: From ~400MB ke ~15MB

**Sekarang:**
```
golang:1.25-alpine (builder)  → ~300MB
alpine:3.20 (production)       → ~7MB + binary (~15MB) = ~22MB
```

Ini sudah cukup bagus. Tapi bisa di-optimalkan lagi:

```dockerfile
# backend/Dockerfile — optimized production stage

# Gunakan distroless — TIDAK ADA shell, TIDAK ADA package manager, TIDAK ADA ls/cat
# Artinya: TIDAK ADA attack surface untuk hacker
FROM gcr.io/distroless/static-debian12:nonroot AS production

WORKDIR /app

# Copy binary dari builder
COPY --from=builder /app/server .

# Copy migrations (tetap perlu, dibaca oleh golang-migrate)
COPY --from=builder /app/migrations ./migrations

# Pakai non-root user (sudah ada di image distroless)
USER nonroot:nonroot

EXPOSE 8080

CMD ["./server"]
```

**Trade-off distroless vs alpine:**

| Aspek | Alpine | Distroless |
|-------|--------|------------|
| Size | ~7MB | ~2MB |
| Shell access (`docker exec -it ... sh`) | ✅ Ada | ❌ Tidak ada |
| Package manager (`apk`) | ✅ Ada | ❌ Tidak ada |
| Debuggability | Mudah | Harus pakai ephemeral debug container |
| Security | Bagus | **Sangat bagus** |
| Cocok untuk | Development, staging | Production |

> **Rekomendasi:** Pakai alpine di dev/staging, distroless di production.

#### Buat `.dockerignore` (sudah dibahas di atas)

Ini wajib! Tanpa ini, `COPY . .` mengirim `node_modules/` (bisa 500MB+) ke Docker context. Build jadi sangat lambat.

---

## 🚀 2. Cloud Deployment & CI/CD

### 2.1 Konsep: CI/CD Pipeline

**CI (Continuous Integration):** Setiap kali kamu push code ke GitHub, otomatis:
1. Run tests
2. Run linter
3. Build image (cek apakah bisa di-build)
4. Beri tahu kamu kalau ada yang gagal

**CD (Continuous Deployment/Delivery):** Setelah CI sukses, otomatis:
1. Push Docker image ke registry (Docker Hub / GitHub Container Registry)
2. Deploy ke server production
3. Run database migrations

#### GitHub Actions CI/CD Pipeline

Buat file `.github/workflows/ci-cd.yml`:

```yaml
# .github/workflows/ci-cd.yml
name: CI/CD Pipeline

on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

env:
  REGISTRY: ghcr.io
  BACKEND_IMAGE: ghcr.io/${{ github.repository }}/backend
  FRONTEND_IMAGE: ghcr.io/${{ github.repository }}/frontend

jobs:
  # ── Backend Tests ─────────────────────────────────────────────────
  test-backend:
    name: Test Backend
    runs-on: ubuntu-latest

    # Service container untuk PostgreSQL test
    services:
      postgres:
        image: postgres:16-alpine
        env:
          POSTGRES_USER: invoiceuser
          POSTGRES_PASSWORD: invoicepassword
          POSTGRES_DB: invoicedb
        ports:
          - 5432:5432
        options: >-
          --health-cmd "pg_isready -U invoiceuser -d invoicedb"
          --health-interval 5s
          --health-timeout 5s
          --health-retries 5

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Setup Go
        uses: actions/setup-go@v5
        with:
          go-version: '1.25'
          cache-dependency-path: backend/go.sum

      - name: Run tests with coverage
        working-directory: backend
        run: |
          go test ./... -coverprofile=coverage.out -v
          go tool cover -func=coverage.out

      - name: Upload coverage
        uses: actions/upload-artifact@v4
        with:
          name: coverage-report
          path: backend/coverage.out

  # ── Frontend Build Check ──────────────────────────────────────────
  test-frontend:
    name: Test Frontend
    runs-on: ubuntu-latest

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Setup Bun
        uses: oven-sh/setup-bun@v1
        with:
          bun-version: '1.1'

      - name: Install dependencies
        working-directory: frontend
        run: bun install

      - name: TypeScript check
        working-directory: frontend
        run: bun x tsc --noEmit

      - name: Build check
        working-directory: frontend
        run: bun run build

  # ── Build & Push Docker Images ────────────────────────────────────
  build-and-push:
    name: Build & Push Images
    needs: [test-backend, test-frontend]  # hanya jalan kalau test sukses
    runs-on: ubuntu-latest
    if: github.event_name == 'push' && github.ref == 'refs/heads/main'
    permissions:
      contents: read
      packages: write

    steps:
      - name: Checkout code
        uses: actions/checkout@v4

      - name: Login to GitHub Container Registry
        uses: docker/login-action@v3
        with:
          registry: ghcr.io
          username: ${{ github.actor }}
          password: ${{ secrets.GITHUB_TOKEN }}

      # Build dan push backend image
      - name: Build & push backend
        uses: docker/build-push-action@v5
        with:
          context: ./backend
          push: true
          tags: |
            ${{ env.BACKEND_IMAGE }}:${{ github.sha }}
            ${{ env.BACKEND_IMAGE }}:latest
          cache-from: type=gha
          cache-to: type=gha,mode=max

      # Build dan push frontend image
      - name: Build & push frontend
        uses: docker/build-push-action@v5
        with:
          context: ./frontend
          push: true
          tags: |
            ${{ env.FRONTEND_IMAGE }}:${{ github.sha }}
            ${{ env.FRONTEND_IMAGE }}:latest
          cache-from: type=gha
          cache-to: type=gha,mode=max
```

#### Konsep Penting di CI/CD

**1. `services:` — Test dengan real database di CI**

CI/CD packages seperti GitHub Actions bisa spin up **service containers** yang berjalan barengan dengan job test. Ini penting karena:
- Unit test butuh PostgreSQL beneran (bukan mock)
- Sama persis seperti production environment

**2. Docker layer caching dengan `type=gha`**

```yaml
cache-from: type=gha    # Pakai GitHub Actions Cache untuk layer Docker
cache-to: type=gha,mode=max  # Simpan layer cache setelah build
```

Tanpa ini, setiap CI run akan build Docker image dari nol. Dengan ini, hanya layer yang berubah yang di-build ulang. **Drastis mengurangi build time** (dari 5 menit jadi 30 detik).

**3. Image tagging strategy**

```
ghcr.io/username/invoice-maker/backend:latest    ← selalu ke versi terbaru
ghcr.io/username/invoice-maker/backend:a1b2c3d   ← versi spesifik per commit
```

Tag `latest` buat development/staging, tag commit SHA buat production (supaya bisa rollback ke versi sebelumnya dengan tepat).

---

### 2.2 Konsep: Deploy ke Cloud

Ada beberapa opsi untuk deploy:

#### Opsi A: VPS (DigitalOcean / Linode / Vultr)

Paling sederhana & murah untuk learning.

```
┌─────────────────────────────────┐
│         VPS (Ubuntu 24.04)      │
│                                 │
│  ┌──────────┐  ┌─────────────┐  │
│  │ Docker   │  │ Docker      │  │
│  │ Compose  │──│ Networks    │  │
│  └──────────┘  └─────────────┘  │
│                                 │
│  Nginx reverse proxy            │
│  (SSL termination)              │
│  ↓                              │
│  frontend:80 → backend:8080    │
│                 ↓               │
│              postgres:5432     │
└─────────────────────────────────┘
        ▲
        │ HTTPS (port 443)
        │
    [Internet]

    [Cloudflare DNS / Domain]
```

**Setup dengan Terraform + Ansible (opsional, advanced):**
Tapi untuk sekarang, pakai script `deploy-production.sh` yang sudah ada sudah cukup.

#### Opsi B: Managed Services

| Service | Kegunaan | Contoh Provider |
|---------|----------|----------------|
| Managed PostgreSQL | Tidak perlu manage DB sendiri | AWS RDS, GCP Cloud SQL, DigitalOcean Managed DB |
| Container Registry | Simpan Docker images | GitHub Container Registry (gratis!), Docker Hub |
| Managed Kubernetes | Auto-scaling container | GKE, EKS, DigitalOcean Kubernetes |
| PaaS (Platform as a Service) | Deploy langsung dari git | Railway, Render, Fly.io |

> **Rekomendasi untuk belajar:** Mulai dari **VPS + Docker Compose**, lalu naik ke **managed services** satu per satu. Jangan langsung Kubernetes — itu overkill dan bikin bingung.

#### Setup SSL/TLS (HTTPS)

Production wajib HTTPS. Cara paling sederhana: **Caddy** atau **Traefik** sebagai reverse proxy dengan auto-SSL.

```yaml
# docker-compose.prod.yml — tambahkan Caddy sebagai reverse proxy
services:
  caddy:
    image: caddy:2-alpine
    container_name: invoice-caddy
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile
      - caddy_data:/data
    depends_on:
      - frontend
      - backend
    restart: unless-stopped

volumes:
  caddy_data:
```

```caddy
# Caddyfile
invoice.example.com {
    # Auto HTTPS dari Let's Encrypt
    reverse_proxy frontend:80

    # Backend API juga di-subdomain berbeda
}

api.example.com {
    reverse_proxy backend:8080
}
```

Caddy **otomatis mendapatkan & renew sertifikat SSL dari Let's Encrypt**. Gak perlu manual setup Certbot.

---

### 2.3 Konsep: Database Migrations in CI/CD

Project ini sudah menjalankan migrasi otomatis di `main.go` saat startup. Ini bagus untuk development, tapi **berisiko di production**:

- Gimana kalau migrasi gagal? Aplikasi mati.
- Gimana kalau 2 instance backend jalan barengan? Dua-duanya coba migrasi (race condition).
- Gimana rollback migrasi yang gagal?

#### Best Practice: Run Migrasi sebagai Job Terpisah

```yaml
# GitHub Actions — job terpisah untuk migrasi
deploy:
  runs-on: ubuntu-latest
  needs: [build-and-push]
  steps:
    # ... setup ...

    - name: Run database migrations
      run: |
        # Pakai golang-migrate CLI di CI
        docker run --rm \
          -v $PWD/backend/migrations:/migrations \
          migrate/migrate \
          -path=/migrations \
          -database="postgres://$DB_USER:$DB_PASSWORD@$DB_HOST:$DB_PORT/$DB_NAME?sslmode=require" \
          up
```

Atau lebih baik, dalam script deploy:

```bash
# deploy-production.sh — tambahkan step migrasi
run_migrations() {
  log "Running database migrations..."

  # Jalankan migrate dalam container sementara
  docker run --rm \
    --network invoice-maker-prod_default \
    -v "$PWD/backend/migrations:/migrations" \
    migrate/migrate \
    -path=/migrations \
    -database="postgres://${DB_USER}:${DB_PASSWORD}@postgres:5432/${DB_NAME}?sslmode=disable" \
    up

  if [ $? -ne 0 ]; then
    err "Migration gagal! Deployment dibatalkan."
    exit 1
  fi
  success "Migrations applied."
}
```

---

## 📊 3. Monitoring & Logging

### 3.1 Konsep: Observability (3 Pillars)

```
┌────────────────────────────────────────────────────┐
│                 OBSERVABILITY                       │
│                                                    │
│  ┌──────────┐  ┌──────────┐  ┌──────────────────┐ │
│  │  LOGS    │  │ METRICS  │  │     TRACES       │ │
│  │          │  │          │  │                  │ │
│  │ "Apa     │  │ "Berapa  │  │ "Di mana         │ │
│  │  yang    │  │  banyak/ │  │  bottleneck      │ │
│  │  terjadi?│  │  cepat?" │  │  dalam request?" │ │
│  └──────────┘  └──────────┘  └──────────────────┘ │
│       ▲              ▲               ▲            │
│       │              │               │            │
│  slog/JSON     Prometheus      (OpenTelemetry)     │
│  → Loki        → Grafana       → Jaeger/Tempo      │
└────────────────────────────────────────────────────┘
```

Untuk Phase 8, kita fokus ke LOGS dan METRICS dulu. Traces (OpenTelemetry) adalah topik advanced.

---

### 3.2 METRICS: Prometheus + Grafana

#### Apa itu Prometheus?

Prometheus adalah **time-series database** yang **menarik (scrape)** data metrics dari aplikasi kamu setiap 15 detik. Bukan aplikasi kamu yang kirim data ke Prometheus — Prometheus yang datang ke aplikasi kamu.

#### Step 1: Tambahkan metrics endpoint di backend

```go
// backend/metrics.go
import (
    "github.com/gin-gonic/gin"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
    // Counter: selalu naik (jumlah total)
    httpRequestsTotal = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "http_requests_total",
            Help: "Total HTTP requests",
        },
        []string{"method", "path", "status"},
    )

    // Histogram: distribusi nilai (latency)
    httpRequestDuration = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name:    "http_request_duration_seconds",
            Help:    "HTTP request latency",
            Buckets: prometheus.DefBuckets, // .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10
        },
        []string{"method", "path"},
    )

    // Gauge: nilai yang naik-turun (current value)
    activeConnections = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "db_connections_active",
            Help: "Active database connections",
        },
    )
)

func init() {
    prometheus.MustRegister(httpRequestsTotal, httpRequestDuration, activeConnections)
}

// Middleware untuk track metrics setiap request
func MetricsMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()

        // Process request
        c.Next()

        // Record metrics setelah request selesai
        duration := time.Since(start).Seconds()
        status := strconv.Itoa(c.Writer.Status())

        httpRequestsTotal.WithLabelValues(c.Request.Method, c.FullPath(), status).Inc()
        httpRequestDuration.WithLabelValues(c.Request.Method, c.FullPath()).Observe(duration)
    }
}

// Handler untuk Prometheus scrape
func MetricsHandler() gin.HandlerFunc {
    h := promhttp.Handler()
    return func(c *gin.Context) {
        h.ServeHTTP(c.Writer, c.Request)
    }
}
```

Lalu daftarkan di `main.go`:

```go
r.Use(MetricsMiddleware())
r.GET("/api/metrics", MetricsHandler())
```

#### Step 2: Tambahkan Prometheus + Grafana ke docker-compose

```yaml
# docker-compose.monitoring.yml
services:
  prometheus:
    image: prom/prometheus:latest
    container_name: invoice-prometheus
    volumes:
      - ./monitoring/prometheus.yml:/etc/prometheus/prometheus.yml
      - prometheus_data:/prometheus
    command:
      - '--config.file=/etc/prometheus/prometheus.yml'
      - '--storage.tsdb.path=/prometheus'
    ports:
      - "9090:9090"
    restart: unless-stopped

  grafana:
    image: grafana/grafana:latest
    container_name: invoice-grafana
    environment:
      - GF_SECURITY_ADMIN_PASSWORD=admin
    ports:
      - "3001:3000"  # 3000 sudah dipakai frontend
    volumes:
      - grafana_data:/var/lib/grafana
      - ./monitoring/grafana-dashboards:/etc/grafana/provisioning/dashboards
    restart: unless-stopped

volumes:
  prometheus_data:
  grafana_data:
```

```yaml
# monitoring/prometheus.yml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: 'backend'
    static_configs:
      - targets: ['backend:8080']
    metrics_path: '/api/metrics'
```

#### Apa yang bisa kamu lihat di Grafana?

Setelah Prometheus + Grafana jalan:
1. **Request rate:** Berapa banyak request per detik
2. **Latency:** P50, P95, P99 latency (berapa lama rata-rata request diproses)
3. **Error rate:** Berapa persen request yang return 4xx/5xx
4. **Database connections:** Berapa banyak koneksi yang aktif

Semua ini bisa dibuat jadi **dashboard** dengan grafik real-time.

---

### 3.3 Error Tracking: Sentry

**Kenapa Sentry?** Error di production bisa terjadi tanpa kamu sadari. Sentry:
- Menangkap semua panic/error beserta **full stack trace**
- Mengelompokkan error yang identik (biar gak spam)
- Memberi tahu **berapa banyak user yang kena**
- Menunjukkan **apa yang user lakukan** sebelum error terjadi

#### Setup Sentry di Backend (Go)

```go
import (
    "github.com/getsentry/sentry-go"
    sentrygin "github.com/getsentry/sentry-go/gin"
)

func main() {
    // Init Sentry
    err := sentry.Init(sentry.ClientOptions{
        Dsn:              os.Getenv("SENTRY_DSN"),
        Environment:      os.Getenv("ENV"), // "production" / "staging"
        TracesSampleRate: 1.0,              // capture 100% (development: 1.0, production: 0.1)
        Debug:            os.Getenv("ENV") != "production",
    })
    if err != nil {
        log.Fatalf("sentry.Init: %s", err)
    }
    defer sentry.Flush(2 * time.Second)

    // Pasang Sentry middleware di Gin (tangkap panic)
    r.Use(sentrygin.New(sentrygin.Options{
        Repanic: true, // re-throw panic setelah kirim ke Sentry
    }))

    // ... setup routes ...
}
```

#### Setup Sentry di Frontend (React)

```bash
cd frontend && npm install @sentry/react
```

```typescript
// frontend/src/main.tsx
import * as Sentry from "@sentry/react";

Sentry.init({
    dsn: import.meta.env.VITE_SENTRY_DSN,
    environment: import.meta.env.MODE, // "development" / "production"
    integrations: [
        Sentry.browserTracingIntegration(),
        Sentry.replayIntegration(),
    ],
    tracesSampleRate: 0.1,  // 10% di production
    replaysSessionSampleRate: 0.1, // 10% user session di-rekam
    replaysOnErrorSampleRate: 1.0, // 100% kalau ada error
});

// Bungkus app dengan ErrorBoundary
ReactDOM.createRoot(document.getElementById('root')!).render(
    <Sentry.ErrorBoundary fallback={<p>Something went wrong</p>}>
        <App />
    </Sentry.ErrorBoundary>
);
```

---

### 3.4 Uptime Monitoring

**Kenapa perlu?** Gimana kalau server mati jam 3 pagi? Kamu perlu yang **nge-ping endpoint kamu** dan kirim notifikasi (Telegram/Discord/Email) kalau down.

#### Opsi Gratis

| Service | Free Tier | Fitur |
|---------|-----------|-------|
| **UptimeRobot** | 50 monitor, 5-min interval | HTTP ping, keyword check, Telegram/Slack notif |
| **Better Uptime** | 10 monitor, 3-min interval | HTTP ping, status page, on-call scheduling |
| **Grafana Cloud** | 10k metrics, 50GB logs | Synthetic monitoring + alerting |

#### Setup Sederhana dengan UptimeRobot

1. Daftar di [uptimerobot.com](https://uptimerobot.com) (gratis)
2. Tambahkan monitor baru:
   - Type: `HTTP(s)`
   - URL: `https://invoice.example.com/api/health`
   - Monitoring interval: `5 minutes`
3. Setup alert contact: Telegram bot atau email
4. Selesai! Kamu akan dapat notifikasi kalau aplikasi down.

---

## 🛠️ Hands-On: Checklist Phase 8

Ikuti checklist ini untuk implementasi bertahap:

### Step 1: Docker Optimization (1-2 jam)
- [ ] Buat `.dockerignore` di root project
- [ ] Tambahkan health check endpoint `/api/health` di backend
- [ ] Tambahkan health check di service `backend` dan `frontend` di `docker-compose.yml`
- [ ] Coba: `docker compose build` dan pastikan context size mengecil (pakai `docker history <image>`)
- [ ] Baca: `docker stats` — lihat resource usage container kamu

### Step 2: Structured Logging (1-2 jam)
- [ ] Migrasi `log.Printf` ke `log/slog` di backend
- [ ] Tambahkan ENV variable `LOG_LEVEL` dan `LOG_FORMAT` (text/json)
- [ ] Test: coba jalankan dengan `LOG_FORMAT=json`, lihat output terstruktur

### Step 3: CI/CD Pipeline (2-3 jam)
- [ ] Buat `.github/workflows/ci.yml` untuk test + build
- [ ] Setup GitHub Container Registry
- [ ] Push image pertama ke `ghcr.io/<username>/invoice-maker/backend`
- [ ] Test: push ke GitHub, lihat CI berjalan di Actions tab

### Step 4: Metrics & Monitoring (2-3 jam)
- [ ] Install package Prometheus client untuk Go
- [ ] Tambahkan `/api/metrics` endpoint
- [ ] Tambahkan Prometheus + Grafana ke docker-compose
- [ ] Buat dashboard sederhana: request rate + latency
- [ ] Baca: pahami 4 jenis metric (Counter, Gauge, Histogram, Summary)

### Step 5: Error Tracking (1 jam)
- [ ] Buat akun Sentry (sentry.io — free tier: 5K errors/month)
- [ ] Integrasikan Sentry SDK ke backend dan frontend
- [ ] Test: trigger error intentional, lihat di Sentry dashboard

### Step 6: Deploy ke Cloud (3-4 jam)
- [ ] Buat VPS (DigitalOcean $6/month atau pakai free tier provider lain)
- [ ] SCP `docker-compose.prod.yml` + `.env.prod` ke server
- [ ] Jalankan `docker compose -f docker-compose.prod.yml up -d`
- [ ] Setup domain + SSL dengan Caddy

---

## 📖 Konsep DevOps yang Perlu Kamu Pahami

### 1. Why DevOps?

> DevOps is not a role, it's a culture.

| Dulu (Traditional) | Sekarang (DevOps) |
|-------------------|-------------------|
| Developer: "It works on my machine" | Container: environment identik di mana pun |
| Deploy setiap 2 minggu (lama, stressful) | Deploy setiap hari (perubahan kecil = risiko kecil) |
| Ops team yang handle server | Developer juga paham infrastructure |
| Manual test sebelum deploy | Automated CI yang test + build |
| Kalau down, user yang kasih tau | Monitoring yang kasih tau sebelum user tau |

### 2. "Pets vs Cattle" — Filosofi Server

- **Pets (server tradisional):** Dikasih nama (`server-produksi-baru`), kalau sakit di-obatin. Mahal dan sulit direplace.
- **Cattle (container):** Gak punya nama (random ID), kalau sakit langsung diganti yang baru. Murah dan cepat.

Docker container adalah "cattle". Kalau ada yang error, kamu gak debugging di dalam containernya — kamu destroy dan bikin yang baru.

### 3. 12-Factor App

[12factor.net](https://12factor.net) — panduan untuk aplikasi yang mudah di-deploy:

1. **Codebase:** Satu codebase, banyak deploy (dev, staging, production)
2. **Dependencies:** Explicit deklarasi (go.mod, package.json)
3. **Config:** Simpan di environment variables (✅ udah benar di project ini)
4. **Backing services:** Perlakukan DB, cache sebagai attached resource
5. **Build, release, run:** Pisahkan build dan run stage
6. **Processes:** Stateless (gak simpan state di memory/filesystem)
7. **Port binding:** Self-contained, export service via port (✅ udah benar)
8. **Concurrency:** Scale via process, bukan thread
9. **Disposability:** Fast startup + graceful shutdown
10. **Dev/prod parity:** Environment semirip mungkin (Docker = parity)
11. **Logs:** Perlakukan log sebagai event stream (structured logging ✅)
12. **Admin processes:** Jalankan sebagai one-off process

> Project Invoice Maker sudah mengikuti banyak prinsip ini!

---

## 🎮 Perintah Penting untuk Diingat

```bash
# ── Docker ───────────────────────────────
docker compose up -d              # Start semua service di background
docker compose ps                  # Lihat status container
docker compose logs -f backend    # Follow logs backend
docker compose logs --tail=10      # 10 baris terakhir
docker compose down               # Stop semua
docker compose down -v            # Stop + hapus volumes (reset database!)
docker compose build --no-cache   # Rebuild dari nol
docker system prune -a            # Bersihin unused images/containers/cache
docker stats                      # Resource usage real-time
docker history <image>            # Lihat layer history sebuah image

# ── Debugging Container ──────────────────
docker exec -it invoice-backend sh         # Masuk shell container
docker inspect invoice-backend | jq        # Lihat full config container
docker logs invoice-backend --tail=50      # Log terakhir

# ── GitHub Container Registry ────────────
echo $GITHUB_TOKEN | docker login ghcr.io -u USERNAME --password-stdin
docker push ghcr.io/username/invoice-maker/backend:latest
docker pull ghcr.io/username/invoice-maker/backend:latest

# ── Monitoring ───────────────────────────
# Prometheus targets:      http://localhost:9090/targets
# Grafana dashboards:      http://localhost:3001
# Backend metrics endpoint: http://localhost:8080/api/metrics
# Backend health check:     http://localhost:8080/api/health
```

---

## 🔗 Resources

### Docker
- [Dockerfile Best Practices](https://docs.docker.com/develop/develop-images/dockerfile_best-practices/)
- [Multi-stage builds](https://docs.docker.com/build/building/multi-stage/)
- [Docker Compose spec](https://docs.docker.com/compose/compose-file/)

### CI/CD
- [GitHub Actions Docs](https://docs.github.com/en/actions)
- [GitHub Actions: Docker build-push](https://github.com/docker/build-push-action)
- [Trunk-Based Development](https://trunkbaseddevelopment.com/)

### Monitoring
- [Prometheus: Metric Types](https://prometheus.io/docs/concepts/metric_types/)
- [Grafana Dashboards](https://grafana.com/grafana/dashboards/)
- [slog Proposal (Go)](https://go.dev/blog/slog)

### Security
- [Docker Security Best Practices](https://docs.docker.com/develop/security-best-practices/)
- [Caddy Auto HTTPS](https://caddyserver.com/docs/automatic-https)
- [OWASP Docker Security Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Docker_Security_Cheat_Sheet.html)

### Tools
- [Sentry Go SDK](https://docs.sentry.io/platforms/go/)
- [Sentry React SDK](https://docs.sentry.io/platforms/javascript/guides/react/)
- [Dive (Docker image analyzer)](https://github.com/wagoodman/dive)
- [Hadolint (Dockerfile linter)](https://github.com/hadolint/hadolint)

---

## 📝 Catatan untuk Phase 9 & 10

Phase 8 (DevOps) dan Phase 9 (Testing) serta Phase 10 (Security) saling terkait:

- **Testing yang bagus** (Phase 9) membuat CI/CD pipeline (Phase 8) **berguna**: kalau test gagal, jangan deploy.
- **Security scanning** (Phase 10) bisa dimasukkan sebagai step di CI/CD pipeline (Phase 8).
- **Health checks** (Phase 8) adalah dasar untuk **zero-downtime deployment**.

---

**Happy Learning! 🐳**
