# Phase 8: Deployment & DevOps — Learning Summary

**Status**: ✅ SELESAI
**Tanggal**: 21 Juli 2026
**Scope**: Docker optimization (.dockerignore, health checks, multi-stage build), structured logging (slog), CI/CD pipeline (GitHub Actions + GHCR), Prometheus + Grafana monitoring, Sentry error tracking (Go + React), production deployment with Caddy SSL

---

## Apa yang Kita Pelajari?

Phase 8 adalah tentang **membawa aplikasi dari "jalan di laptop" ke "jalan di internet" dengan production-grade quality**. Ini bukan cuma soal deploy — tapi soal seluruh lifecycle setelah kode ditulis: bagaimana memastikan kode selalu bisa di-build (CI/CD), bagaimana tahu kalau ada yang error (Sentry), bagaimana monitor performa (Prometheus + Grafana), bagaimana log yang bisa di-search (slog), bagaimana container yang efisien dan aman (Docker optimization), dan bagaimana production deployment dengan HTTPS (Caddy).

Ini adalah real-world skillset yang dipakai di SEMUA perusahaan teknologi: **DevOps, SRE, Platform Engineering** — semuanya dibangun di atas fondasi ini.

---

## Problem: Aplikasi Tanpa DevOps

### ❌ Sebelum Phase 8

```bash
# Development: jalan manual
cd backend && go run .
cd frontend && npm run dev

# Log: gak terstruktur
log.Printf("query error: %v", err)
# Output: 2024/01/15 10:30:45 query error: connection refused
# ↑ gak bisa di-search, gak bisa di-filter, gak bisa di-agregasi

# Deploy: manual SCP + SSH
scp binary user@vps:/app/
ssh user@vps "nohup ./app &"
# ↑ gak ada health check, gak ada auto-restart, gak ada HTTPS

# Monitoring: "user complain dulu baru tau error"
# Error tracking: cek log manual via SSH
# CI/CD: test manual di laptop sebelum deploy
```

**Masalah:**
1. **Log gak terstruktur** — `log.Printf` gak bisa di-parse tools monitoring, sulit di-search
2. **Gak ada health check** — Docker gak tahu container sehat atau enggak, gak bisa auto-restart
3. **Gak ada CI/CD** — test manual, build manual, deploy manual = human error + lambat
4. **Gak ada monitoring** — gak tahu performa aplikasi (request rate, latency, error rate)
5. **Gak ada error tracking** — error di production baru ketahuan kalau user lapor
6. **Deploy tanpa HTTPS** — port 3000 & 8080 exposed langsung, plain HTTP

### ✅ Setelah Phase 8

```bash
# Git push → otomatis test + build + push Docker image
git push origin main

# Deploy: satu perintah
ssh vps "./deploy-production.sh --update"

# Log: terstruktur, JSON di production
{"time":"...","level":"ERROR","msg":"query","error":"connection refused"}
# ↑ bisa di-query: jq 'select(.level=="ERROR")', Grafana, Sentry

# Monitoring: Prometheus + Grafana dashboard
# Error tracking: Sentry auto-capture + stack trace + session replay
# Health check: Docker auto-restart kalau container unhealthy
# HTTPS: Caddy auto-SSL dari Let's Encrypt
```

---

## Arsitektur: DevOps Pipeline

### End-to-End Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│                        DEVOPS PIPELINE                              │
│                                                                     │
│  [Laptop]                  [GitHub Actions]          [VPS]          │
│  git push ──▶ CI/CD Pipeline ──▶ GHCR ──▶ docker pull ──▶ deploy  │
│                     │                                               │
│              ┌──────┼──────┐                                        │
│              ▼      ▼      ▼                                        │
│           Test   Build   Push                                       │
│           Go +   Docker  Image                                      │
│           tsc    Image   ke GHCR                                    │
│                                                                     │
│  [Production Stack]                                                 │
│  ┌──────────────────────────────────────────────┐                   │
│  │ Caddy (:80/:443) ← Auto SSL                  │                   │
│  │  ├── /api/* → backend:8080                   │                   │
│  │  └── /*     → frontend:80                    │                   │
│  │                                              │                   │
│  │ Backend:8080  → PostgreSQL:5432              │                   │
│  │   ├── /api/health   (Docker healthcheck)     │                   │
│  │   ├── /api/metrics  (Prometheus scrape)      │                   │
│  │   └── sentrygin     (panic → Sentry)         │                   │
│  │                                              │                   │
│  │ Monitoring:                                   │                   │
│  │   Prometheus → scrape /api/metrics setiap 15s │                   │
│  │   Grafana    → dashboard request/latency      │                   │
│  │   Sentry     → error + stack trace + replay   │                   │
│  └──────────────────────────────────────────────┘                   │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Konsep 1: Docker Health Check — Deep vs Shallow

Health check endpoint jangan cuma `return 200`. Harus cek dependency — karena aplikasi tanpa database = useless.

### ❌ Sebelum

```go
// Shallow health check — bohong!
r.GET("/api/health", func(c *gin.Context) {
    c.JSON(200, gin.H{"status": "ok"})
})
// ↑ server hidup → return 200
// ↑ database mati → TETAP return 200 (bohong: aplikasi sebenernya gak berfungsi)
```

### ✅ Sesudah

```go
// Deep health check — cek dependency
r.GET("/api/health", func(c *gin.Context) {
    ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
    defer cancel()

    if err := db.Ping(ctx); err != nil {
        c.JSON(http.StatusServiceUnavailable, gin.H{
            "status": "unhealthy",
            "db":     "unreachable",
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "status": "healthy",
        "time":   time.Now().UTC().Format(time.RFC3339),
    })
})
```

**Kenapa `db.Ping()` bukan `SELECT 1`?** `Ping()` verify koneksi masih alive tanpa overhead query execution — ini yang paling ringan. `SELECT 1` juga ok, tapi `Ping()` slightly lebih efisien.

**Kenapa `start_period: 15s` di docker-compose?** Backend jalankan migrasi database saat startup (~5-10 detik). Tanpa `start_period`, Docker langsung cek health check → belum siap → anggap unhealthy → restart container → infinite loop. `start_period` kasih grace period di mana health check failure tidak dihitung.

```yaml
# docker-compose.yml
backend:
  healthcheck:
    test: ["CMD-SHELL", "wget -qO- http://localhost:8080/api/health | grep -q healthy"]
    interval: 10s       # cek setiap 10 detik
    timeout: 5s         # timeout 5 detik
    retries: 3          # 3x gagal = unhealthy → restart
    start_period: 15s   # grace period untuk startup + migrasi
```

---

## Konsep 2: Structured Logging — `log/slog` vs `log.Printf`

### Kenapa Structured Logging?

```
Unstructured (log.Printf):
  2024/01/15 10:30:45 query error: connection refused
  ↑ text mentah, gak bisa di-filter kecuali pakai regex

Structured (slog, text):
  time=2024-01-15T10:30:45 level=ERROR msg="query" error="connection refused"

Structured (slog, JSON):
  {"time":"2024-01-15T10:30:45Z","level":"ERROR","msg":"query","error":"connection refused"}
  ↑ bisa di-query tools: jq, Grafana Loki, Sentry, Elasticsearch
```

### ❌ Sebelum

```go
// 43 instance log.Printf tersebar di 5 file
log.Printf("query error: %v", err)
log.Printf("insert invoice error: %v", err)
log.Printf("pdf generation error: %v", err)
log.Fatalf("failed to initialize database: %v", err)
```

### ✅ Sesudah

```go
// Semua diganti slog.Error dengan key-value pairs
slog.Error("query", "error", err)
slog.Error("insert invoice", "error", err)
slog.Error("pdf generation", "error", err)
slog.Error("failed to initialize database", "error", err)
os.Exit(1)
```

**Kenapa key-value pairs (`"error", err`)?** Structured logging = setiap field punya nama. Tools monitoring bisa query: `level=ERROR AND error="connection refused"`. Dengan `log.Printf`, value `err` cuma jadi string mentah dalam message.

**Kenapa `os.Exit(1)` setelah `slog.Error`?** `log.Fatalf` otomatis exit — `slog.Error` tidak. Kalau error fatal (gak bisa connect DB), kita harus explicit exit. Ini by design: slog memisahkan logging dari control flow.

### Konfigurasi via Environment

```go
// backend/logger.go
func initLogger() {
    level := parseLogLevel(os.Getenv("LOG_LEVEL"))    // DEBUG/INFO/WARN/ERROR
    format := strings.ToLower(os.Getenv("LOG_FORMAT")) // text/json

    var handler slog.Handler
    opts := &slog.HandlerOptions{Level: level}

    if format == "json" {
        handler = slog.NewJSONHandler(os.Stdout, opts)    // production
    } else {
        handler = slog.NewTextHandler(os.Stdout, opts)    // development
    }
    slog.SetDefault(slog.New(handler))
}
```

**Kenapa env variable, bukan hardcode?** 12-Factor App principle #3: config disimpan di environment variables. Development: text + debug. Production: json + info. Gak perlu ganti kode — cukup ganti env.

---

## Konsep 3: CI/CD dengan GitHub Actions

### Service Container: Test dengan Database Sungguhan

Test backend butuh PostgreSQL beneran (bukan mock). GitHub Actions bisa spin up **service container** yang hidup selama job berlangsung:

```yaml
test-backend:
  runs-on: ubuntu-latest
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
```

**Kenapa service container, bukan install PostgreSQL di runner?** Isolation! Container punya environment yang bersih, persis seperti production. Setelah job selesai → container otomatis dihapus. Gak ada sisaan state dari test sebelumnya.

**Kenapa `--health-cmd`?** Runner akan tunggu PostgreSQL bener-bener siap sebelum mulai test. Tanpa ini, `go test` bisa mulai sebelum PostgreSQL siap → connection refused.

### Action Pinning: Supply Chain Security

```yaml
# ❌ Sebelum — tag bisa di-move ke commit malicious
uses: docker/build-push-action@v6

# ✅ Sesudah — SHA immutable, gak bisa diubah
uses: docker/build-push-action@10e90e3645eae34f1e60eeb005ba3a3d33f178e8  # v6
```

**Kenapa?** Kalau attacker compromise repo `docker/build-push-action`, mereka bisa ganti tag `v6` ke commit malicious. Pipeline kamu langsung kena. Git SHA immutable — tidak bisa diubah tanpa mengganti history Git.

### Docker Layer Caching

```yaml
- name: Build & push backend
  uses: docker/build-push-action@...
  with:
    cache-from: type=gha        # baca cache dari GitHub Actions Cache
    cache-to: type=gha,mode=max # simpan cache setelah build
```

**Kenapa?** Tanpa cache, setiap CI run build Docker image dari nol (~5 menit). Dengan cache, hanya layer yang berubah yang di-build ulang (~30 detik). `type=gha` pakai GitHub Actions Cache sebagai cache storage — gak perlu setup external service.

**Kenapa butuh `docker/setup-buildx-action`?** Cache export (`type=gha`) adalah fitur BuildKit. Docker runner default di GitHub Actions pakai driver lama yang gak support. `setup-buildx-action` mengganti driver ke `docker-container` yang support BuildKit penuh.

---

## Konsep 4: Prometheus Metrics — 4 Tipe Metric

### Kenapa Perlu Tau 4 Tipe?

Prometheus bukan cuma "simpan angka". Setiap tipe metric punya behavior dan use case berbeda. Salah pilih tipe = salah interpretasi data.

```go
// backend/metrics.go

// Counter: selalu NAIK (odometer). Gak pernah turun.
// Cocok untuk: total request, total error, total bytes sent.
httpRequestsTotal = prometheus.NewCounterVec(
    prometheus.CounterOpts{
        Name: "http_requests_total",
        Help: "Total number of HTTP requests.",
    },
    []string{"method", "path", "status"},
)

// Histogram: distribusi nilai dalam BUCKET. 
// Cocok untuk: latency, response size.
// Bucket bawaan: .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10 detik
httpRequestDuration = prometheus.NewHistogramVec(
    prometheus.HistogramOpts{
        Name:    "http_request_duration_seconds",
        Help:    "HTTP request latency in seconds.",
        Buckets: prometheus.DefBuckets,
    },
    []string{"method", "path"},
)

// Gauge: nilai NAIK-TURUN (speedometer).
// Cocok untuk: active connections, memory usage, queue size.
dbConnectionsActive = prometheus.NewGauge(
    prometheus.GaugeOpts{
        Name: "db_connections_active",
        Help: "Number of active database connections.",
    },
)
```

**Kenapa `NewCounterVec` bukan `NewCounter`?** `Vec` = metric dengan **label** (dimensions). Tanpa label, semua request dihitung jadi satu. Dengan label `method`, `path`, `status`, kita bisa query: "berapa request GET /api/invoices yang gagal?", "request ke endpoint mana yang paling lambat?"

**Kenapa `FullPath()` bukan `URL.Path`?** `FullPath()` return path pattern (`/api/invoices/:id`), `URL.Path` return path aktual (`/api/invoices/abc123`). Dengan pattern, semua request ke endpoint yang sama dikelompokkan jadi satu — bisa di-agregasi. Dengan path aktual, setiap ID jadi metric terpisah → cardinality explosion.

```go
// Middleware: catat setiap request
func MetricsMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        start := time.Now()
        c.Next()
        duration := time.Since(start).Seconds()
        status := strconv.Itoa(c.Writer.Status())
        path := c.FullPath()  // pattern, bukan aktual
        if path == "" {
            path = c.Request.URL.Path  // fallback
        }
        httpRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
        httpRequestDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
    }
}
```

### Prometheus Pull Model

```
BUKAN: backend → push metrics → Prometheus
TAPI:  Prometheus → GET /api/metrics → backend (setiap 15 detik)

┌──────────┐  scrape (pull)   ┌──────────┐
│Prometheus│ ────────────────▶│ Backend  │
│  :9090   │ ◀─── metrics ───│  :8080   │
└────┬─────┘                  └──────────┘
     │
     ▼
┌──────────┐
│ Grafana  │  query Prometheus → tampilkan dashboard
│  :3001   │
└──────────┘
```

**Kenapa pull, bukan push?** Pull model lebih sederhana: Prometheus yang kontrol kapan scrape. Backend cuma perlu expose HTTP endpoint. Kalau backend mati, Prometheus tau karena scrape gagal — ini juga jadi sinyal "service down."

---

## Konsep 5: Sentry — Error Tracking vs Logging

### Kenapa Sentry + Logging, bukan salah satu?

```
Logging (slog):     "APA yang terjadi?"     — semua event, informasi umum
Sentry:             "KENAPA error terjadi?" — hanya error, dengan full context
Metrics (Prometheus): "BERAPA banyak/seberapa cepat?" — agregasi numerik

Ketiganya SALING MELENGKAPI, bukan menggantikan:
  - Log: "user login" ← normal event
  - Sentry: "panic di handler login, stack trace: ..." ← exception detail
  - Metrics: "login latency p95 = 200ms" ← performance aggregate
```

### Backend: Panic Capture dengan Middleware

```go
// backend/router.go — middleware PERTAMA setelah gin.Default()
r.Use(sentrygin.New(sentrygin.Options{
    Repanic: true,  // kirim ke Sentry, LALU re-panic
}))
```

**Kenapa `Repanic: true`?** Sentry middleware tangkap panic → kirim stack trace → re-panic. Kenapa? Karena `gin.Default()` sudah termasuk `gin.Recovery` yang handle graceful error response ke client. Kalau gak re-panic, client dapat response aneh. Kalau re-panic, flow-nya: Sentry capture → gin.Recovery tangkap → return 500 dengan proper JSON error.

**Kenapa middleware PERTAMA?** Middleware di Gin jalan secara LIFO (last-in-first-out). Middleware terluar tangkap duluan. Sentry harus jadi yang pertama supaya bisa tangkap panic dari SEMUA middleware dan handler di bawahnya.

### Frontend: ErrorBoundary + Session Replay

```tsx
// frontend/src/main.tsx
<Sentry.ErrorBoundary
  fallback={({ error, resetError }) => (
    <div>
      <h1>Something went wrong</h1>
      <p>The error has been reported to our team.</p>
      <button onClick={resetError}>Try Again</button>
    </div>
  )}
>
  <App />
</Sentry.ErrorBoundary>
```

**Kenapa ErrorBoundary, bukan try/catch?** React component tree error gak bisa di-catch dengan try/catch biasa. ErrorBoundary adalah React pattern khusus untuk tangkap error di rendering phase. Tanpa ErrorBoundary → seluruh halaman blank (React unmount seluruh tree). Dengan ErrorBoundary → hanya component di dalam boundary yang kena, sisanya tetap jalan.

**Kenapa `maskAllText: true` di replay?** Session replay ngerekam tampilan layar user. Invoice Maker menampilkan data sensitif: nama klien, email, nominal invoice. Tanpa masking → PII leak. Dengan masking → hanya struktur halaman yang terlihat, semua teks di-blur.

```typescript
Sentry.replayIntegration({
    maskAllText: true,      // blur SEMUA teks
    maskAllInputs: true,    // blur SEMUA input field
    blockAllMedia: true,    // blokir gambar/video
}),
```

### Graceful Degradation: Tanpa DSN

```go
// backend/sentry.go
func initSentry() {
    dsn := os.Getenv("SENTRY_DSN")
    if dsn == "" {
        slog.Warn("SENTRY_DSN not set — error tracking disabled")
        return  // ← aplikasi TETAP jalan normal tanpa Sentry
    }
    sentry.Init(sentry.ClientOptions{Dsn: dsn, ...})
}
```

**Kenapa disabled kalau DSN kosong?** Development tanpa Sentry harus tetap bisa jalan. Production tanpa Sentry juga (walau gak recommended). Pattern ini disebut **graceful degradation** — fitur opsional gak boleh break core functionality.

---

## Konsep 6: Production Deployment — Caddy vs Nginx

### Kenapa Caddy?

| Aspek | Nginx | Caddy |
|-------|-------|-------|
| SSL setup | Manual: install Certbot, cron job, config | **Otomatis**: detect domain, ambil cert, renew |
| Config complexity | ~50 baris untuk HTTPS + reverse proxy | ~15 baris |
| Renewal | Cron job (`certbot renew`) | Otomatis, Caddy handle sendiri |
| Perfect for | Perusahaan besar (banyak custom rule) | Project kecil-menengah, "just works" |

```
# Caddyfile — 6 baris config untuk HTTPS + reverse proxy
{$DOMAIN:localhost} {
    handle_path /api/* {
        reverse_proxy backend:8080
    }
    handle {
        reverse_proxy frontend:80
    }
}
```

**Kenapa `expose` bukan `ports` di docker-compose?** 

```yaml
# ❌ Production: semua port terbuka ke internet
backend:
  ports:
    - "8080:8080"    # bisa diakses langsung dari internet!

# ✅ Production: hanya Caddy yang exposed
backend:
  expose:
    - "8080"         # hanya container lain di network yg sama yg bisa akses
```

`ports:` = mapping port host ke container → terbuka ke internet. `expose:` = port hanya terbuka di internal Docker network. Production rule: **hanya reverse proxy yang pakai `ports:`, semua service lain pakai `expose:`**.

---

## Skill yang Dikuasai

| Skill | Tool/Pattern | Real-World Usage |
|-------|-------------|------------------|
| Container optimization | Multi-stage build, health checks, `.dockerignore` | Every Docker project |
| Structured logging | `log/slog`, JSON format, env-based config | Go 1.21+ standard |
| CI/CD | GitHub Actions, service containers, GHCR, layer caching | Every GitHub project |
| Supply chain security | SHA-pinned actions, frozen lockfiles | SOC 2 / compliance |
| Monitoring | Prometheus Counter/Gauge/Histogram, Grafana provisioning | Kubernetes, microservices |
| Error tracking | Sentry SDK, ErrorBoundary, session replay, PII masking | All production apps |
| Production deploy | Caddy auto-SSL, internal networking, IaC | VPS / cloud deploy |
| Infrastructure as Code | Docker Compose, Caddyfile, env templates | DevOps standard |

---

## Referensi

### Docker
- [Dockerfile Best Practices](https://docs.docker.com/develop/develop-images/dockerfile_best-practices/)
- [Multi-stage builds](https://docs.docker.com/build/building/multi-stage/)
- [Healthcheck](https://docs.docker.com/reference/dockerfile/#healthcheck)

### Go
- [log/slog — Structured Logging](https://go.dev/blog/slog)
- [Prometheus Go Client](https://pkg.go.dev/github.com/prometheus/client_golang)
- [Sentry Go SDK](https://docs.sentry.io/platforms/go/)

### GitHub Actions
- [Workflow Syntax](https://docs.github.com/en/actions/writing-workflows/workflow-syntax-for-github-actions)
- [Service Containers](https://docs.github.com/en/actions/using-containerized-services/about-service-containers)
- [Docker Build-Push Action](https://github.com/docker/build-push-action)
- [Security Hardening](https://docs.github.com/en/actions/security-for-github-actions/security-guides/security-hardening-for-github-actions)

### Prometheus & Grafana
- [Metric Types](https://prometheus.io/docs/concepts/metric_types/)
- [Grafana Provisioning](https://grafana.com/docs/grafana/latest/administration/provisioning/)

### Sentry
- [Sentry Go SDK](https://docs.sentry.io/platforms/go/)
- [Sentry React SDK](https://docs.sentry.io/platforms/javascript/guides/react/)
- [Session Replay Privacy](https://docs.sentry.io/platforms/javascript/session-replay/privacy/)

### Production
- [Caddy Server](https://caddyserver.com/docs/)
- [12-Factor App](https://12factor.net/)

### Related Project Docs
- `docs/DEPLOYMENT_GUIDE.md` — panduan deploy ke VPS step-by-step
- `TODO.md` — Phase 8 task list original
- `docs/PHASE9_IMPLEMENTASI_TESTING.md` — Phase 9 learning doc

---

**Phase 8 Selesai** ✅
Invoice Maker kini punya production-grade DevOps pipeline: CI/CD otomatis, monitoring real-time dengan Prometheus + Grafana, error tracking dengan Sentry, structured logging, dan production deployment dengan auto-HTTPS via Caddy. Ini adalah skillset yang membedakan "bisa coding" dari "bisa shipping software ke production."
