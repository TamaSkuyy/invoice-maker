# Phase 9: Backend Go Testing — Design Spec

**Status**: Approved
**Tanggal**: 2026-07-16
**Scope**: Unit + integration testing untuk backend Go saja (frontend testing di-defer). Tujuan ganda: portfolio project untuk lamar kerja Go, dan latihan menulis idiomatic Go tests.
**Mode kolaborasi**: User menulis kode test sendiri, Claude guide per bagian (bukan menulis semua test-nya).

---

## Decisions Summary

| Decision | Choice | Reason |
|----------|--------|--------|
| Frontend testing | Deferred | Fokus belajar Go; frontend testing jadi phase terpisah nanti |
| Router testability | Extract `setupRouter()` dari `main()` | Handler saat ini closure di dalam `main()` — gak bisa dipanggil dari test tanpa ini |
| Handler refactor scope | Cuma pindah lokasi, bukan bongkar jadi named functions | Named-function conversion (seperti `analytics.go`) adalah improvement terpisah, di luar scope testing |
| Test database | testcontainers-go | Container Postgres otomatis nyala/mati per test run, isolated, siap dipakai di CI (Phase 8) tanpa setup manual |
| Assertion library | testify (`assert`/`require`) | Standar de-facto di industri Go, dikurangin boilerplate dibanding `if err != nil { t.Fatal(...) }` |
| Handler coverage | Representative slice per domain | Happy path + 1-2 error case per domain; bukan exhaustive semua endpoint/edge case |
| Export (PDF/Excel/CSV) testing | Smoke test saja | Assert byte content PDF/Excel gak valuable; cukup pastikan gak error & content-type benar |
| Coverage target | ~50-60%, bukan >80% blind | TODO.md lama nulis >80% tanpa konteks; realistis mengingat export code sengaja gak di-exhaustive-test |

---

## Backend Endpoints yang Ada (existing, dari `main.go`)

```
/api/auth/register          POST
/api/auth/login             POST
/api/auth/me                GET  (protected)

/api/invoices                GET, POST       (protected)
/api/invoices/:id            GET, PUT, DELETE (protected)
/api/invoices/:id/pdf        GET  (protected)
/api/invoices/:id/csv        GET  (protected)
/api/invoices/export/excel   GET  (protected)

/api/clients                 GET, POST       (protected)
/api/clients/:id             PUT, DELETE     (protected)

/api/products                GET, POST       (protected)
/api/products/:id            PUT, DELETE     (protected)

/api/analytics/overview      GET  (protected)
/api/analytics/revenue       GET  (protected)
/api/analytics/top-clients   GET  (protected)
/api/analytics/tax-summary   GET  (protected)
/api/analytics/report        GET  (protected)
```

Semua kecuali `/api/auth/register` dan `/api/auth/login` dilindungi `authenticate()` middleware (JWT).

---

## 1. Refactor Prasyarat: `setupRouter()`

**File baru**: `backend/router.go`

**Koreksi penting dari asumsi awal**: Handler di `main.go` (auth, invoices, clients, products) adalah closure yang reference package-level `var db *pgxpool.Pool` langsung. Tapi handler `analytics.go` (`handleAnalyticsOverview`, dkk.) adalah **named function terpisah** yang juga reference `db` package-level — bukan closure di dalam `main()`. Kalau `setupRouter` dikasih parameter `db *pgxpool.Pool`, parameter itu cuma akan di-capture oleh closure yang secara lexical ditulis di dalam `setupRouter` (auth/invoices/clients/products) — sedangkan `handleAnalyticsOverview` dkk. tetap pakai `db` package-level asli karena mereka fungsi top-level yang independen.

Daripada bikin dua sumber `db` yang beda (parameter lokal untuk sebagian handler, var global untuk sisanya) — pendekatan yang lebih konsisten dan sesuai pola yang sudah ada di codebase (`db` package-level dipakai di semua tempat) adalah: **`setupRouter()` tidak menerima parameter sama sekali**, cukup pakai `db` package-level yang harus sudah di-assign sebelum dipanggil.

```go
func setupRouter() *gin.Engine {
    r := gin.Default()
    // ... semua r.Use(), r.Group(), handler registration — persis seperti sekarang,
    // tetap reference `db` package-level, sama seperti analytics.go
    return r
}
```

`main()` jadi:

```go
func main() {
    if err := initDB(); err != nil {
        log.Fatalf("failed to initialize database: %v", err)
    }
    defer closeDB()

    if err := runMigrations(); err != nil {
        log.Fatalf("failed to run migrations: %v", err)
    }

    r := setupRouter()
    r.Run(":8080")
}
```

Test (`TestMain`) meng-assign `db = testDB` (reassign var package-level yang sama, legal karena test file ada di package yang sama) sebelum memanggil `setupRouter()`. Ini konsisten dengan pola global-db-pool yang sudah dipakai di seluruh codebase (termasuk `analytics.go`), tidak butuh refactor 800 baris handler jadi named function/dependency injection (itu improvement terpisah, di luar scope Phase 9), dan tidak menciptakan dua sumber kebenaran untuk `db`.

---

## 2. Test Database: testcontainers-go

**Dependency baru**: `github.com/testcontainers/testcontainers-go` + `github.com/testcontainers/testcontainers-go/modules/postgres` (test-only, masuk `go.mod` biasa karena Go tidak punya konsep "dev dependency" terpisah).

**File**: `backend/main_test.go`

```go
func TestMain(m *testing.M) {
    ctx := context.Background()

    container, err := postgres.Run(ctx, "postgres:16-alpine",
        postgres.WithDatabase("invoice_test"),
        postgres.WithUsername("test"),
        postgres.WithPassword("test"),
        testcontainers.WithWaitStrategy(
            wait.ForListeningPort("5432/tcp"),
        ),
    )
    if err != nil {
        log.Fatalf("failed to start postgres container: %v", err)
    }
    defer container.Terminate(ctx)

    connStr, err := container.ConnectionString(ctx, "sslmode=disable")
    if err != nil {
        log.Fatalf("failed to get connection string: %v", err)
    }

    // Assign the package-level `db` var (declared in db.go) — every handler,
    // closure or named function, reads from this same var.
    db, err = pgxpool.New(ctx, connStr)
    if err != nil {
        log.Fatalf("failed to connect to test db: %v", err)
    }

    if err := runMigrationsWithConn(connStr); err != nil {
        log.Fatalf("failed to run migrations: %v", err)
    }

    code := m.Run()
    os.Exit(code)
}
```

**Catatan implementasi**: `runMigrations()` saat ini ( `main.go:210`) membangun connection string dari env vars langsung di dalam fungsinya, jadi tidak bisa dipanggil dengan connection string test container. Perlu extract jadi:

```go
func runMigrationsWithConn(connString string) error {
    m, err := migrate.New("file://./migrations", connString)
    if err != nil {
        return fmt.Errorf("unable to create migration instance: %w", err)
    }
    defer m.Close()
    if err := m.Up(); err != nil && err != migrate.ErrNoChange {
        return fmt.Errorf("unable to run migrations: %w", err)
    }
    return nil
}

func runMigrations() error {
    connString := fmt.Sprintf(
        "postgres://%s:%s@%s:%s/%s?sslmode=disable",
        os.Getenv("DB_USER"), os.Getenv("DB_PASSWORD"),
        os.Getenv("DB_HOST"), os.Getenv("DB_PORT"), os.Getenv("DB_NAME"),
    )
    return runMigrationsWithConn(connString)
}
```

`main()` tetap panggil `runMigrations()` (env-based), test `TestMain` panggil `runMigrationsWithConn(containerConnStr)`.

**Isolasi antar test**: tiap test function `TRUNCATE` tabel yang relevan di awal (via helper `truncateTables(t *testing.T, db *pgxpool.Pool)`), bukan transaction rollback — lebih simpel dan cukup buat ukuran project ini.

---

## 3. Struktur File Test

Mengikuti convention Go: `xxx_test.go` bersebelahan dengan `xxx.go`.

### `backend/logic_test.go` — Pure unit test (tanpa DB, tanpa network)

Table-driven tests untuk:
- `round2(v float64) float64` — pembulatan 2 desimal, termasuk edge case negative & tepat di tengah
- `calculateTotal(items []InvoiceItem, taxRate float64) float64`
- `hashPassword` / `verifyPassword` — hash beda tiap kali dipanggil (salt), verify hash yang benar/salah
- `generateJWT` / `validateJWT` — token valid, token expired, token dengan signature salah

### `backend/auth_test.go` — Integration (pakai `setupRouter(testDB)` + `httptest`)

- Register: happy path (201 + token) — Register: email sudah terdaftar (400)
- Login: happy path (200 + token) — Login: password salah (401/400)
- Akses endpoint protected tanpa `Authorization` header → 401
- Akses endpoint protected dengan token invalid → 401

### `backend/invoices_test.go` — Integration

- Create → Get by ID → Update → Delete → Get by ID lagi (404)
- Create dengan payload invalid (400)
- List invoices hanya return milik user yang login (isolasi multi-user)

### `backend/clients_test.go`, `backend/products_test.go` — Integration (representative)

- Create + List untuk masing-masing domain
- 1 error case (misal create tanpa field wajib → 400)

### `backend/analytics_test.go` — Integration dengan seed data

- Seed beberapa invoice dengan `total_amount` & `date` known-value
- `GET /api/analytics/overview` → assert `total_revenue`, `total_invoices` sesuai hitungan manual
- `GET /api/analytics/revenue?year=X` → assert agregasi per bulan benar

Ini bagian paling bernilai untuk belajar Go: verifikasi SQL aggregation (`SUM`, `GROUP BY`, `COALESCE`) menghasilkan angka yang benar, bukan cuma "endpoint return 200".

### `backend/export_test.go` — Smoke test

- `GET /invoices/:id/pdf` → 200, `Content-Type: application/pdf`, body tidak kosong
- `GET /invoices/export/excel` → 200, `Content-Type` benar, body tidak kosong
- `GET /analytics/report?format=pdf` → 200 tidak error

---

## 4. Tooling & Commands

```
go get github.com/stretchr/testify
go get github.com/testcontainers/testcontainers-go
go get github.com/testcontainers/testcontainers-go/modules/postgres

go test ./...           # jalanin semua test
go test ./... -v        # verbose
go test ./... -cover    # coverage report
```

**Prasyarat lokal**: Docker/Podman harus jalan (dipakai testcontainers buat spin up Postgres). Sudah konsisten dengan `dev-local.sh` yang juga butuh Docker/Podman.

---

## 5. Housekeeping: `TODO.md`

Sebagai bagian dari implementasi:
- Centang ulang Phase 5 (Reporting & Analytics) — sudah selesai tapi checkbox belum di-update sejak commit `92afa62`
- Update Phase 9 checklist sesuai item yang benar-benar dikerjakan (unit tests ✅, integration tests ✅, E2E tests tetap unchecked/dicoret karena di-defer ke frontend testing phase, coverage target diubah dari ">80%" ke catatan realistis)

---

## Files Changed

```
backend/
  main.go                [MODIFIED — extract routing ke setupRouter()]
  router.go               [NEW — setupRouter(db *pgxpool.Pool) *gin.Engine]
  main_test.go             [NEW — TestMain, testcontainers setup]
  logic_test.go            [NEW — pure unit tests]
  auth_test.go             [NEW — integration tests]
  invoices_test.go         [NEW — integration tests]
  clients_test.go          [NEW — integration tests]
  products_test.go         [NEW — integration tests]
  analytics_test.go        [NEW — integration tests + seed data]
  export_test.go           [NEW — smoke tests]
  go.mod / go.sum          [MODIFIED — testify, testcontainers-go deps]

TODO.md                   [MODIFIED — centang Phase 5, update Phase 9]

docs/superpowers/specs/
  2026-07-16-phase9-backend-testing-design.md  [THIS FILE]
```

---

## What We Skip

- Frontend testing (Vitest/RTL) — phase terpisah nanti
- E2E browser tests (Cypress/Playwright) — butuh frontend testing dulu
- Exhaustive edge-case coverage di semua handler — representative slice cukup untuk tujuan portfolio + belajar
- Byte-level assertion pada PDF/Excel output — smoke test cukup
- CI/CD integration (GitHub Actions run test otomatis) — itu Phase 8, tapi desain testcontainers-go di sini sengaja dipilih supaya kompatibel waktu Phase 8 dikerjakan

---

## Verification Checklist

- [ ] `go build ./...` — compile tanpa error setelah refactor `setupRouter()`
- [ ] `go vet ./...` — tidak ada warning
- [ ] `go test ./...` — semua test pass
- [ ] `go test ./... -cover` — coverage ter-generate, angka masuk akal (~50-60%)
- [ ] Server masih jalan normal via `go run .` / `dev-local.sh` setelah refactor (tidak ada regresi behavior)
- [ ] Unit test `logic_test.go` cover semua pure function tanpa DB
- [ ] Integration test tiap domain (auth, invoices, clients, products, analytics) minimal 1 happy path + 1 error case
- [ ] Test isolasi multi-user terverifikasi (user A tidak bisa lihat data user B)
- [ ] `TODO.md` ter-update: Phase 5 checked, Phase 9 mencerminkan scope yang benar-benar dikerjakan
