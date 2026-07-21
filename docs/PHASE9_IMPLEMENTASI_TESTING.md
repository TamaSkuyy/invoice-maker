# Phase 9: Testing & Code Quality — Learning Summary

**Status**: ✅ SELESAI (Part 1: Backend + Part 2: Frontend)
**Tanggal**: 16-21 Juli 2026
**Scope**: Part 1 — Backend unit test + integration test Go (table-driven, httptest, dedicated DB). Part 2 — Frontend Vitest + React Testing Library, ESLint flat config, Prettier, lint-staged pre-commit hooks, CI integration.

---

## Apa yang Kita Pelajari?

Phase 9 adalah **transformasi dari project tanpa test sama sekali** (0 file `_test.go`) menjadi project dengan **8 file test, 17 fungsi test, >40 assertions, dan 67.0% coverage**. Ini bukan cuma tentang "nulis test", tapi tentang **mentality shift** — dari "kode gue jalan kok" ke "gue bisa **buktikan** kode gue jalan."

Testing di Go punya filosofi sendiri — gak kayak framework testing berat di bahasa lain. Filosofinya: **testing adalah first-class citizen bahasa**, bukan afterthought yang ditempelin library external.

---

## Problem: Kenapa Butuh Testing?

### ❌ Sebelum Phase 9

```go
// Semua testing dilakukan manual via curl/postman
// Developer harus:
// 1. Jalanin container postgres
// 2. Jalanin ./dev-local.sh
// 3. Buka terminal lain, curl register/login
// 4. Copy token, curl endpoint satu-satu
// 5. Cek response manual pakai mata

// Kalau ada perubahan kode? Ulangi semua dari step 1.
// Kalau contributor baru clone repo? Gak tau harus test apa.
// Kalau refactor kode trus ngerusak sesuatu? Gak bakal tau sampai error di production.
```

**Masalah testing manual:**
1. **Lambat** — tiap kali ubah kode harus setup ulang, curl, cek manual
2. **Gak reproducible** — urutan test dan data input beda-beda tiap orang
3. **Gak otomatis** — gak bisa diintegrasikan ke CI/CD (Phase 8 nanti)
4. **Coverage gak kelihatan** — mana yang udah dites? mana yang belum?
5. **Bug tersembunyi** — bug di `PUT /api/invoices/:id` (missing `client_id` di Scan) gak bakal ketauan tanpa integration test yang ngejalanin full flow

### ✅ Testing di Go

```go
// Satu perintah. Semua otomatis. Hasil reproducible.
// go test ./... -v
// 
// === RUN   TestInvoiceLifeCycle
// --- PASS: TestInvoiceLifeCycle (0.24s)
// === RUN   TestInvoiceIsolationBetweenUsers
// --- PASS: TestInvoiceIsolationBetweenUsers (0.16s)
// ...
// PASS
// ok      github.com/TamaSkuyy/invoice-maker/backend     2.879s
```

---

## Bagian-Bagian Penting yang Kita Pelajari

### 1. Unit Test: Table-Driven Tests (Testify `assert`/`require`)

Ini adalah **cara paling idiomatic Go** buat nulis test untuk pure function.

**Pattern**: Daripada bikin satu fungsi test per kasus, bikin **satu slice struct** berisi semua kasus (input + expected output), terus loop pakai `t.Run()`.

```go
func TestRound2(t *testing.T) {
    tests := []struct {
        name string
        in   float64
        want float64
    }{
        {"Already 2 decimals", 10.20, 10.20},
        {"Need Rounding Up",   10.506, 10.51},
        {"Need Rounding Down", 10.504, 10.50},
        {"Negative Value",     -10.506, -10.51},
        {"Zero",               0, 0},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            assert.Equal(t, tt.want, round2(tt.in))
        })
    }
}
```

**Kenapa Table-Driven?**

| Aspek | Table-Driven | Satu Fungsi per Kasus |
|-------|-------------|----------------------|
| Menambah kasus baru | Tambah 1 struct literal | Bikin fungsi baru (lebih verbose) |
| Output `go test -v` | `TestRound2/Negative_Value` — jelas | Nama fungsi aja, kurang konteks |
| Satu kasus gagal | Yang lain tetap jalan | Tergantung struktur |
| Baca semua kasus sekaligus | Satu glance di slice | Harus scroll ke bawah |

**`assert` vs `require`:**
- `assert.Equal(t, ...)` — catat gagal, **lanjutin** assertion berikutnya
- `require.Equal(t, ...)` — catat gagal, **stop test langsung** (fatal)
- Kapan pakai `require`? Saat assertion berikutnya **gak ada artinya** kalau ini gagal. Contoh: `require.Equal(t, http.StatusCreated, rec.Code)` — kalau status code aja udah salah, gak ada gunanya decode body response-nya.

---

### 2. Unit Test: Bcrypt Salt Verification

Password hashing (`bcrypt`) punya karakteristik unik yang bikin test-nya beda dari pure function biasa.

```go
func TestHashPassword(t *testing.T) {
    hash, err := hashPassword("secret_password")
    require.NoError(t, err)
    assert.NotEmpty(t, hash)
    assert.NotEqual(t, "secret_password", hash)  // hash != plaintext

    // bcrypt MENGGARAM (salt) — hash dua kali HARUS beda
    hash2, err := hashPassword("secret_password")
    require.NoError(t, err)
    assert.NotEqual(t, hash, hash2)  // SALT BEKERJA
}
```

**Kenapa ini penting?** Bcrypt itu **sengaja lambat** (cost factor). `TestHashPassword` makan **~130ms** sendiri. Kalau di production, cost factor bisa dinaikin ke 12-14 buat keamanan lebih — tapi makin lambat. Ini trade-off keamanan vs performa yang umum.

---

### 3. Unit Test: JWT Generate + Validate + Expiry

JWT testing ada trik menarik: **gimana cara test token expired tanpa nunggu beneran?**

```go
func TestGenerateAndValidateJWT(t *testing.T) {
    t.Run("valid token round-trips claims", func(t *testing.T) {
        token, err := generateJWT("user-123", "test@example.com")
        require.NoError(t, err)
        
        claims, err := validateJWT(token)
        require.NoError(t, err)
        assert.Equal(t, "user-123", claims.UserID)
        assert.Equal(t, "test@example.com", claims.Email)
    })

    t.Run("tampered token is rejected", func(t *testing.T) {
        token, _ := generateJWT("user-123", "test@example.com")
        tampered := token[:len(token)-2] + "xx"  // rusakin 2 karakter terakhir
        _, err := validateJWT(tampered)
        assert.Error(t, err)  // HARUS ditolak — signature verification bekerja
    })

    t.Run("expired token is rejected", func(t *testing.T) {
        t.Setenv("JWT_EXPIRATION", "-1")  // ⚡ TRICK: set expiry ke -1 detik
        token, _ := generateJWT("user-123", "test@example.com")
        _, err := validateJWT(token)
        assert.Error(t, err)  // Token udah expired pas dibuat
    })
}
```

**`t.Setenv(key, value)`** — set environment variable untuk **test ini aja**. Auto-kembali ke nilai semula setelah test selesai, gak bocor ke test lain. Ini jauh lebih bersih daripada `os.Setenv` + `defer os.Unsetenv`.

---

### 4. Refactor Prasyarat: `setupRouter()` + `runMigrationsWithConn()`

Sebelum bisa nulis integration test, kita harus bikin kode production-nya **testable** dulu.

**Masalah awal**: Semua handler ditulis sebagai **closure di dalam `main()`**, langsung reference package-level var `db`:
```go
// main.go
func main() {
    // ...
    api.POST("", func(c *gin.Context) {  // closure — gak bisa dipanggil dari test
        db.QueryRow(ctx, ...)
    })
    r.Run(":8080")
}
```

**Solusi**: Extract ke `setupRouter()` di file terpisah:
```go
// router.go
func setupRouter() *gin.Engine {
    r := gin.Default()
    // ... semua routing — bisa dipanggil dari main() maupun dari test
    return r
}

// main.go
func main() {
    // ...
    r := setupRouter()
    r.Run(":8080")
}
```

Satu detail penting: **handler `analytics.go` adalah named function top-level**, bukan closure. Mereka reference `db` package-level langsung. Jadi `setupRouter()` gak boleh dibikin dengan parameter `db` (karena `handleAnalyticsOverview()` di file lain gak akan lihat parameter itu — dia cuma lihat var global). Solusi: **`setupRouter()` tanpa parameter**, test langsung assign `db = testPool` sebelum manggilnya.

**`runMigrationsWithConn(connString)`** — extract yang sama. Production pakai env var, test pakai connection string dari setup database test.

---

### 5. Integration Test Infrastructure: `TestMain` + Database Terpisah

**Rencana awal**: `testcontainers-go` — library yang otomatis spin up container Postgres per test run.

**Realita**: Environment lokal (Manjaro) pakai **Podman rootless**, bukan dockerd asli. Podman rootless pakai userspace networking (`slirp4netns`/`pasta`), bukan `iptables` DNAT rules seperti Docker. `testcontainers-go` berasumsi semantik dockerd asli → gagal bikin port mapping. **Known limitation dicatat**, fallback ke pendekatan database terpisah.

**Pendekatan final**: `TestMain` connect ke Postgres dev yang udah jalan (`localhost:5432`), bikin **database terpisah** (`invoicedb_test`) khusus test, jalanin migration ke sana, terus assign `db` package-level var ke pool test.

```go
func TestMain(m *testing.M) {
    gin.SetMode(gin.TestMode)
    ctx := context.Background()

    // Connect ke admin DB dulu (harus bisa CREATE/DROP database)
    adminPool, _ := pgxpool.New(ctx, "postgres://invoiceuser:invoicepassword@localhost:5432/postgres?sslmode=disable")
    
    // Terminate koneksi nyangkut dari test run sebelumnya
    adminPool.Exec(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity 
        WHERE datname = 'invoicedb_test' AND pid <> pg_backend_pid()`)
    
    // Drop + Create database test
    adminPool.Exec(ctx, `DROP DATABASE IF EXISTS invoicedb_test`)
    adminPool.Exec(ctx, `CREATE DATABASE invoicedb_test`)
    adminPool.Close()

    // Assign db package-level var — INI YANG DIPAKAI SEMUA HANDLER
    db, _ = pgxpool.New(ctx, "postgres://invoiceuser:invoicepassword@localhost:5432/invoicedb_test?sslmode=disable")
    
    // Jalanin migration ke database test
    runMigrationsWithConn(testConnStr)

    // Jalanin semua test
    code := m.Run()

    // Cleanup
    db.Close()
    os.Exit(code)
}
```

**Kenapa `DROP DATABASE IF EXISTS` + `pg_terminate_backend` dulu?** Kalau `go test` sebelumnya crash/di-Ctrl+C, koneksi ke `invoicedb_test` bisa nyangkut — Postgres nolak `DROP DATABASE` kalau masih ada yang connect. `pg_terminate_backend` maksa putus semua koneksi lama sebelum drop.

---

### 6. Integration Test via `httptest` + `*gin.Engine`

**Cara Go bikin HTTP test tanpa beneran buka port network:**

```go
func TestRegister(t *testing.T) {
    truncateTables(t)
    router := setupRouter()  // router in-memory — gak ada port yang dibuka!

    t.Run("happy path returns 201 with token", func(t *testing.T) {
        rec := doRequest(router, http.MethodPost, "/api/auth/register", 
            SignupRequest{Email: "newuser@mail.com", Password: "password123"}, "")
        
        require.Equal(t, http.StatusCreated, rec.Code)

        var resp AuthResponse
        json.Unmarshal(rec.Body.Bytes(), &resp)
        assert.NotEmpty(t, resp.Token)
    })
}
```

**Apa yang terjadi di balik layar:**
1. `httptest.NewRequest(...)` — bikin `*http.Request` object (seperti yang diterima server HTTP beneran)
2. `httptest.NewRecorder()` — bikin `*ResponseRecorder` yang implement `http.ResponseWriter` (simpan response untuk di-assert)
3. `router.ServeHTTP(rec, req)` — **Gin memproses request persis seperti production**, tapi lewat memory bukan network socket
4. `rec.Code`, `rec.Body.Bytes()`, `rec.Header()` — kita assert response-nya

Ini namanya **in-process HTTP testing** — full request/response cycle, middleware, routing, handler, semuanya jalan beneran, tapi **tanpa port network**. Lebih cepat dan lebih reliable daripada test yang beneran binding port.

---

### 7. Test Isolation: `truncateTables()` + `CASCADE`

Tiap integration test harus **independen** — gak boleh ketiban data sisa dari test sebelumnya.

```go
func truncateTables(t *testing.T) {
    t.Helper()
    ctx := context.Background()
    _, err := db.Exec(ctx, `TRUNCATE TABLE invoice_items, invoices, clients, products, users CASCADE`)
    require.NoError(t, err)
}
```

**Kenapa table order-nya spesifik?** `CASCADE` artinya kalau ada foreign key constraint, Postgres akan cascade truncate ke child table. Order: child tables dulu (`invoice_items`) → parent (`invoices`, `clients`, `products`) → paling atas (`users`). Dengan `CASCADE`, order gak terlalu kritis — tapi tetap best practice untuk eksplisit.

**Pakai `t.Helper()`** — ini nandain bahwa function ini adalah helper, bukan test function beneran. Kalau assertion di dalam helper gagal, error message bakal nunjuk ke baris di test function yang manggil helper, bukan ke baris di dalam helper. Lebih gampang debugging.

---

### 8. Multi-User Isolation Test (Portfolio Highlight)

Ini test yang paling nunjukin pemahaman tentang **multi-tenancy security**:

```go
func TestInvoiceIsolationBetweenUsers(t *testing.T) {
    truncateTables(t)
    router := setupRouter()
    tokenA := registerTestUser(t, router, "usera@mail.com", "password")
    tokenB := registerTestUser(t, router, "userb@mail.com", "password")

    // User A bikin invoice
    createRec := doRequest(router, http.MethodPost, "/api/invoices", Invoice{
        ClientName: "User A's Client", /*...*/
    }, tokenA)
    require.Equal(t, http.StatusCreated, createRec.Code)
    
    var created Invoice
    decodeJSON(createRec.Body.Bytes(), &created)

    // User B coba akses invoice User A via ID → 404 (bukan 403!)
    getRec := doRequest(router, http.MethodGet, "/api/invoices/"+created.ID, nil, tokenB)
    assert.Equal(t, http.StatusNotFound, getRec.Code)

    // User B GET list → kosong (invoice User A gak bocor)
    listRec := doRequest(router, http.MethodGet, "/api/invoices", nil, tokenB)
    var invoices []Invoice
    decodeJSON(listRec.Body.Bytes(), &invoices)
    assert.Empty(t, invoices)
}
```

**Kenapa 404, bukan 403?** Di real-world API, kalau kita return 403 ("forbidden"), attacker bisa enumerasi ID: "oh, invoice ini ada tapi bukan punya gue." Dengan return 404, semua invoice yang bukan punya user kelihatan "tidak ada" — attacker gak bisa bedain antara "invoice tidak ada" dan "invoice ada tapi bukan punya kamu". Ini security pattern yang umum di production API.

---

### 9. SQL Aggregation Verification via Seed Data

Test analytics ini unik — kita gak cuma ngecek "endpoint return 200", tapi **ngecek kebenaran hasil SQL aggregation:**

```go
func TestAnalyticsRevenueByMonth(t *testing.T) {
    truncateTables(t)
    router := setupRouter()
    token := registerTestUser(t, router, "revenueuser@mail.com", "password")

    // Seed data dengan nilai yang sudah diketahui
    seedInvoice(t, router, token, "2026-01-10", 1000, 0)  // Jan: 1000
    seedInvoice(t, router, token, "2026-01-20", 500, 0)   // Jan: 500 (total Jan: 1500)
    seedInvoice(t, router, token, "2026-02-05", 2000, 0)  // Feb: 2000

    rec := doRequest(router, http.MethodGet, "/api/analytics/revenue?year=2026", nil, token)
    require.Equal(t, http.StatusOK, rec.Code)

    var resp RevenueResponse
    decodeJSON(rec.Body.Bytes(), &resp)

    // Build map untuk lookup berdasarkan label bulan
    byLabel := map[string]RevenueDataPoint{}
    for _, dp := range resp.Data {
        byLabel[dp.Label] = dp
    }

    // Verifikasi agregasi SQL akurat
    require.Contains(t, byLabel, "Jan")
    assert.Equal(t, 1500.0, byLabel["Jan"].Total)  // 1000 + 500 = 1500
    assert.Equal(t, 2, byLabel["Jan"].Count)

    require.Contains(t, byLabel, "Feb")
    assert.Equal(t, 2000.0, byLabel["Feb"].Total)
    assert.Equal(t, 1, byLabel["Feb"].Count)
}
```

**Kenapa pakai map?** Data dari analytics endpoint urut by month (1-12), tapi kita gak bisa asumsi urutan hasil DB selalu konsisten. Map lookup by label bulan lebih robust — gak peduli urutan `resp.Data` gimana, asalkan "Jan" dan "Feb" ada.

---

### 10. Smoke Tests untuk Binary Output (PDF/Excel)

Untuk endpoint yang generate binary (PDF/Excel/CSV), assertion byte-level gak praktis dan gak valuable. Pattern-nya: **smoke test** — cek status code, content-type, dan body non-empty:

```go
rec := doRequest(router, http.MethodGet, "/api/invoices/"+id+"/pdf", nil, token)
require.Equal(t, http.StatusOK, rec.Code)
assert.Equal(t, "application/pdf", rec.Header().Get("Content-Type"))
assert.NotEmpty(t, rec.Body.Bytes())
```

Kalau generate PDF-nya crash (misal `fpdf` error), status code-nya gak mungkin 200. Kalau file corrupt/kosong, body-nya kosong. **Cukup buat confidence**, tanpa perlu nge-parse binary PDF.

---

## Masalah yang Ditemui & Diperbaiki

### 1. Bug Asli: Missing `client_id` di `PUT /api/invoices/:id` Handler

**Ditemukan oleh**: `TestInvoiceLifeCycle` — step "Update" gagal dengan error SQL: *"number of field descriptions must equal number of destinations, got 9 and 8"*

**Root cause**: Di handler `PUT /api/invoices/:id`, bagian "Fetch and return updated invoice" punya query SELECT 9 kolom tapi `Scan(...)` cuma punya 8 tujuan — `client_id` kelewatan:

```go
// Sebelum perbaikan — 9 kolom di SELECT, 8 di Scan
err = db.QueryRow(ctx, `SELECT id, client_name, client_id, CAST(date AS TEXT), 
    tax_rate, total_amount, user_id, created_at, updated_at FROM invoices WHERE id = $1`, id).
    Scan(&updatedInv.ID, &updatedInv.ClientName, &updatedInv.Date,  // ← client_id missing!
         &updatedInv.TaxRate, &updatedInv.TotalAmount, &updatedInv.UserID,
         &updatedInv.CreatedAt, &updatedInv.UpdatedAt)

// Setelah perbaikan — 9 kolom di SELECT, 9 di Scan
    Scan(&updatedInv.ID, &updatedInv.ClientName, &updatedInv.ClientID,  // ← fixed
         &updatedInv.Date, &updatedInv.TaxRate, &updatedInv.TotalAmount,
         &updatedInv.UserID, &updatedInv.CreatedAt, &updatedInv.UpdatedAt)
```

**Lessons:** Ini adalah **contoh nyata kenapa integration test penting**. Bug ini udah ada dari Phase 4, dan gak ketauan karena:
1. Frontend gak selalu manggil PUT lalu fetch ulang response body
2. Error hanya kelihatan di log server (yang jarang diliat developer)
3. Manual testing curl jarang banget nyentuh update flow penuh

---

### 2. Environment Issue: Podman Rootless vs testcontainers-go

**Masalah**: `testcontainers-go` gagal spin up container Postgres — error `iptables ... RULE_APPEND failed (No such file or directory): rule in chain DOCKER`

**Root cause**: Laptop (Manjaro) pakai Podman rootless dengan userspace networking (`slirp4netns`/`pasta`), bukan dockerd asli dengan `iptables` DNAT. testcontainers-go berasumsi semantik Docker — inkompatibel.

**Solusi**: Fallback ke dedicated database `invoicedb_test` di container Postgres dev yang udah jalan. Container via docker-compose gak kena masalah karena Podman compose pakai `netavark` untuk networking, bukan iptables.

**Catatan**: Issue ini udah dicatat di `TODO.md` Phase 8 ("fix current Podman issues"). Bukan di-skip permanen — nanti pas ada Docker asli atau konfigurasi Podman yang kompatibel, bisa di-consider lagi.

---

### 3. Test Bug: Email Typo (`mail.ccom` vs `mail.com`)

Error yang gampang kelewat baca manual tapi langsung ketauan pas test di-run — `registerTestUser` bikin user dengan email `"logintest@mail.ccom"` (double 'c'), tapi test login pakai `"logintest@mail.com"`. Hasilnya: login gagal, error `401` bukan `200`. Begitu test di-run, langsung kelihatan di mana letak mismatch-nya.

---

## Struktur File Test yang Dibuat

```
backend/
├── logic_test.go          [NEW] Unit test pure functions
│   ├── TestRound2              — 6 sub-test (table-driven)
│   ├── TestCalculateTotal      — 4 sub-test (table-driven)
│   ├── TestHashPassword        — bcrypt salt verification
│   ├── TestVerifyHashPassword  — verify vrai/salah/kosong
│   └── TestGenerateAndValidateJWT — valid, malformed, tampered, expired
│
├── main_test.go           [NEW] Infrastruktur test
│   ├── TestMain                — dedicated DB setup/teardown
│   ├── truncateTables()        — helper isolation
│   ├── doRequest()             — helper HTTP in-process
│   └── registerTestUser()      — helper create user + return token
│
├── auth_test.go           [NEW] Integration: autentikasi
│   ├── TestRegister            — happy path + duplicate email
│   ├── TestLogin               — happy path + wrong password
│   └── TestProtectedRouteRequiresAuth — 401 tanpa/invalid token
│
├── invoices_test.go       [NEW] Integration: full CRUD + security
│   ├── TestInvoiceLifeCycle    — create → get → update → delete → 404
│   ├── TestCreateInvoiceInvalidPayload — 400 pada payload invalid
│   └── TestInvoiceIsolationBetweenUsers — multi-user data isolation
│
├── clients_test.go        [NEW] Integration: client CR
│   ├── TestClientCreateAndList  — create + list
│   └── TestClientDeleteNotFound — 404 pada delete invalid
│
├── products_test.go       [NEW] Integration: product CR
│   ├── TestProductCreateAndList  — create + list
│   └── TestProductDeleteNotFound — 404 pada delete invalid
│
├── analytics_test.go      [NEW] Integration: SQL aggregation
│   ├── TestAnalyticsOverview          — seed → verify aggregation
│   ├── TestAnalyticsOverviewEmptyState — all zeros
│   └── TestAnalyticsRevenueByMonth    — seed → verify per-month
│
└── export_test.go         [NEW] Smoke: binary output
    ├── TestDownloadInvoicePDF     — content-type pdf + non-empty
    ├── TestDownloadInvoiceCSV     — content-type csv + non-empty
    ├── TestExportInvoicesExcel    — content-type xlsx + non-empty
    └── TestDownloadAnalyticsReport — pdf + excel report non-error
```

**File di-modifikasi:**
```
backend/router.go         [MODIFIED] Fix bug PUT /api/invoices/:id (missing client_id di Scan)
backend/main.go           [MODIFIED] Extract runMigrationsWithConn(), slim main()
TODO.md                  [MODIFIED] Centang Phase 5, update Phase 9 progress
```

---

## Yang Dipelajari di Phase 9

### Testing Concepts

- **Table-driven tests** — pattern idiomatic Go, lebih maintainable dari satu-fungsi-per-kasus
- **`assert` vs `require`** — `require` fatal (stop test), `assert` non-fatal (lanjut, kumpulin semua failure)
- **`t.Helper()`** — nandain helper function biar error message nunjuk ke call site yang bener
- **`t.Setenv()`** — set env var scoped ke satu test aja, autorestored setelah selesai
- **`t.Run()`** — sub-test dengan nama deskriptif, gagal satu gak ngaruh yang lain
- **`TestMain(m *testing.M)`** — setup/teardown global sekali seumur test run
- **In-process HTTP testing** — `httptest.NewRequest` + `httptest.NewRecorder` + `router.ServeHTTP` — full stack tanpa port network
- **Test isolation** — `TRUNCATE ... CASCADE` di awal tiap test function
- **Multi-user isolation test** — verifikasi security data scoping (user A gak bisa lihat data user B)
- **SQL aggregation verification** — seed data known-value, decode response, assert hasil `SUM`/`COUNT`/`GROUP BY`
- **Smoke tests** — content-type + non-empty body untuk binary output (PDF/Excel), tanpa byte-level parsing
- **Coverage tooling** — `go test ./... -cover`, `-coverprofile`, `go tool cover -html`

### Refactoring for Testability

- **Extract `setupRouter()`** — bikin router reusable dari test tanpa binding port
- **Extract `runMigrationsWithConn()`** — bikin migration testable dengan connection string yang berbeda dari production
- **Closure vs named function di package-level** — closure di `main.go` nangkep `db` global, named function di `analytics.go` juga — dua-duanya butuh strategi yang konsisten (assign `db` var, bukan passing parameter)

### Real-World Debugging

- **Bug hunting via test failure** — `PUT /api/invoices/:id` handler Scan missing column, ketauan dari integration test yang ngejalanin full flow
- **Environment incompatibility** — Podman rootless userspace networking vs testcontainers-go iptables assumption — documented as known limitation, fallback path chosen

### Go Tooling

- **`go test ./...`** — run semua test dalam module
- **`go test -run 'Pattern'`** — filter test spesifik
- **`go test -v`** — verbose (lihat nama tiap sub-test)
- **`go test -cover`** — coverage percentage
- **`go vet ./...`** — static analysis sebelum test
- **`go mod tidy`** — bersihin unused dependencies

---

## Langkah Berikutnya

Setelah Phase 9 selesai, yang bisa dikerjakan:

- **Phase 8: DevOps / CI-CD** — Setup GitHub Actions untuk auto-run `go test ./...` + `go vet` di tiap push. Hasilnya: badge hijau di README + confidence bahwa setiap PR gak ngerusak yang udah jalan.

- **Phase 6: Invoice Status** — Tambah `status` field (Draft/Sent/Paid/Overdue) → dashboard bisa nambah widget Paid vs Pending yang diskip di Phase 5.

- **Frontend Testing** — Component test (Vitest) + E2E (Playwright) untuk React. Ini di luar scope belajar Go — bisa dikerjakan terpisah.

- **Integration test coverage lebih dalam** — Handler yang sekarang cuma "representative slice" bisa di-exhausted coverage-nya, terutama error branch yang belum tersentuh.

---

## Referensi

### Go Testing
- [Go testing package](https://pkg.go.dev/testing) — official docs
- [TestMain pattern](https://pkg.go.dev/testing#hdr-Main)
- [Table-driven tests](https://dave.cheney.net/2019/05/07/prefer-table-driven-tests) — Dave Cheney
- [httptest package](https://pkg.go.dev/net/http/httptest)

### testify
- [testify/assert](https://pkg.go.dev/github.com/stretchr/testify/assert)
- [testify/require](https://pkg.go.dev/github.com/stretchr/testify/require)

### PostgreSQL
- [TRUNCATE ... CASCADE](https://www.postgresql.org/docs/current/sql-truncate.html)
- [pg_terminate_backend](https://www.postgresql.org/docs/current/functions-admin.html#FUNCTIONS-ADMIN-SIGNAL)

### Related Project Docs
- `docs/superpowers/specs/2026-07-16-phase9-backend-testing-design.md` — full design spec (decisions, rationale, architecture)
- `docs/superpowers/plans/2026-07-16-phase9-backend-testing.md` — implementation plan (10 tasks, step-by-step)
- `docs/PHASE5_IMPLEMENTASI_ANALYTICS.md` — pola dokumentasi Phase 5 (format sama dengan dokumen ini)
- `TODO.md` — full project roadmap

---

**Part 1 (Backend) Selesai** ✅

---

## Part 2: Frontend Testing & Code Quality

**Tanggal**: 21 Juli 2026  
**Scope**: Vitest + React Testing Library setup, component smoke tests, ESLint flat config, Prettier, lint-staged pre-commit hooks, CI integration

---

### Problem: Frontend Tanpa Test & Tanpa Linter

#### ❌ Sebelum Part 2

```bash
# package.json — cuma 3 script
"scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "preview": "vite preview"
}

# Testing: 0 file, 0 test
# Linting: tidak ada ESLint, tidak ada Prettier
# CI: cuma tsc --noEmit (type check doang)
# Pre-commit: tidak ada
```

**Masalah:**
1. **Gak ada test** — refactor komponen = gambling (gak tau ada yg rusak atau enggak)
2. **Gak ada ESLint** — bug React (missing deps, setState in effect) gak kedetect
3. **Gak ada formatter** — style code tidak konsisten antar developer
4. **CI gak verifikasi** — cuma cek TypeScript types, gak cek apakah komponen bisa render

#### ✅ Setelah Part 2

```bash
"scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "preview": "vite preview",
    "lint": "eslint src/",              # ← BARU
    "lint:fix": "eslint src/ --fix",    # ← BARU
    "format": "prettier --write src/",  # ← BARU
    "format:check": "prettier --check src/", # ← BARU
    "test": "vitest run",               # ← BARU
    "test:watch": "vitest"              # ← BARU
}

# Testing: 3 file, 13 test
# CI: lint + test + type check + build
# Pre-commit: lint-staged auto-fix
```

---

### Arsitektur: Testing & Code Quality Pipeline

```
┌──────────────────────────────────────────────────────────────┐
│              FRONTEND QUALITY PIPELINE                        │
│                                                               │
│  [Kode Editor]                                                │
│       │                                                       │
│       ▼                                                       │
│  ┌─────────────────────────────────────────────────────┐     │
│  │ PRE-COMMIT (lint-staged)                             │     │
│  │  *.ts,*.tsx → eslint --fix → prettier --write        │     │
│  │  *.json,*.css,*.md → prettier --write                │     │
│  └─────────────────────────────────────────────────────┘     │
│       │                                                       │
│       ▼                                                       │
│  ┌─────────────────────────────────────────────────────┐     │
│  │ CI PIPELINE (setiap push)                            │     │
│  │  ├── ESLint check (non-blocking warning)             │     │
│  │  ├── Prettier format check                           │     │
│  │  ├── Vitest run (3 files, 13 tests)                  │     │
│  │  ├── tsc --noEmit (type check)                       │     │
│  │  └── vite build (production bundle)                  │     │
│  └─────────────────────────────────────────────────────┘     │
│                                                               │
│  [Developer Experience]                                       │
│  npm run test:watch  → auto re-run on file change            │
│  npm run lint:fix    → auto-fix linting issues               │
│  npm run format      → auto-format all files                 │
└──────────────────────────────────────────────────────────────┘
```

---

### Konsep 1: ESLint Flat Config — Kenapa Bukan `.eslintrc`?

ESLint v9+ pindah ke **flat config** (`eslint.config.js`). Format lama (`.eslintrc`) deprecated.

#### ❌ Format Lama (`.eslintrc`)

```json
// .eslintrc.json — FORMAT LAMA, deprecated di ESLint v9
{
  "extends": [
    "eslint:recommended",
    "plugin:@typescript-eslint/recommended"
  ],
  "parser": "@typescript-eslint/parser",
  "plugins": ["react-hooks"],
  "rules": {
    "react-hooks/rules-of-hooks": "error"
  }
}
```

#### ✅ Flat Config (`eslint.config.js`)

```js
// eslint.config.js — FLAT CONFIG, ESLint v9+
import js from "@eslint/js";
import tseslint from "typescript-eslint";
import reactHooks from "eslint-plugin-react-hooks";
import prettierConfig from "eslint-config-prettier";

export default tseslint.config(
  js.configs.recommended,              // ESLint recommended
  ...tseslint.configs.recommended,     // TypeScript rules
  {
    plugins: { "react-hooks": reactHooks },
    rules: { ...reactHooks.configs.recommended.rules },
  },
  prettierConfig,                      // MUST be last — matikan ESLint rules yg konflik dgn Prettier
  { ignores: ["dist/", "node_modules/"] },
);
```

**Kenapa flat config?** Lebih eksplisit, composable (tinggal tambah/hapus config object), dan lebih cepat. `tseslint.config()` adalah helper dari `typescript-eslint` untuk membuat config yang type-safe.

**Kenapa `prettierConfig` harus terakhir?** Config di-merge secara berurutan. Config terakhir menang. `eslint-config-prettier` matikan rules ESLint yang konflik dengan Prettier — kalau ditaruh di awal, rules ESLint lain bisa mengaktifkannya kembali.

---

### Konsep 2: Vitest — Kenapa Bukan Jest?

| Aspek | Jest | Vitest |
|-------|------|--------|
| Config | `jest.config.js` terpisah | Pakai `vite.config.ts` yang sama |
| Speed | Lebih lambat (transform manual) | Lebih cepat (ESBuild native) |
| ESM support | Ribet (perlu transform) | Native (pakai Vite) |
| TypeScript | Perlu `ts-jest` / `babel` | Native (pakai Vite) |
| Watch mode | `jest --watch` | `vitest` (default watch) |
| Compatibility | Mature, banyak plugin | Compatible dengan Jest API |

```ts
// vitest.config.ts — pakai Vite config yang sama dengan development!
import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],          // ← plugin React yang sama dengan dev
  test: {
    environment: "jsdom",       // simulate browser
    setupFiles: ["./src/test/setup.ts"],
    globals: true,              // expect, describe, it tanpa import
    css: false,                 // skip CSS = lebih cepat
  },
});
```

**Kenapa `environment: "jsdom"`?** Test component React butuh DOM API (`document.createElement`, `element.click()`). Node.js gak punya DOM — `jsdom` meng-emulasi DOM di Node.js sehingga `render(<LoginPage />)` bisa jalan.

**Kenapa `globals: true`?** Tanpa ini, setiap file test harus import `describe`, `it`, `expect` dari `vitest`. Dengan `globals: true`, API Vitest tersedia global (seperti Jest). Lebih ergonomis — tapi perlu `"types": ["vitest/globals"]` di tsconfig supaya TypeScript gak error.

**Kenapa `css: false`?** Component test gak butuh CSS. Skip parsing CSS = test ~30% lebih cepat. Visual regressions (kalau butuh) pakai Playwright/Storybook, bukan unit test.

---

### Konsep 3: Component Testing — Render + Interact + Assert

#### Pattern: Testing Component dengan Props

Komponen paling gampang di-test adalah yang **menerima semua behavior sebagai props** — tidak ada API call, tidak ada state management, tidak ada side effect.

```tsx
// LoginPage menerima SEMUA behavior sebagai props:
interface LoginPageProps {
  onLoginSuccess: () => void;           // callback → bisa di-mock
  onNavigateToRegister: () => void;     // callback → bisa di-mock
  login: (email: string, password: string) => Promise<void>; // async → bisa di-mock
  loading: boolean;                     // state → dikontrol test
  error: string | null;                 // state → dikontrol test
}
```

Dengan pattern ini, test jadi **deterministik** — gak ada API call beneran, gak ada localStorage, gak ada state yg gak terkontrol.

```tsx
// LoginPage.test.tsx
describe("LoginPage", () => {
  // 1. Render test — paling dasar
  it("should render login form", () => {
    render(<LoginPage {...defaultProps} />);
    expect(screen.getByLabelText("Email")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Login" })).toBeInTheDocument();
  });

  // 2. Interaction test — user ketik + klik
  it("should call login with email and password", async () => {
    const login = vi.fn().mockResolvedValue(undefined);
    render(<LoginPage {...defaultProps} login={login} />);
    
    await userEvent.type(screen.getByLabelText("Email"), "test@example.com");
    await userEvent.type(screen.getByLabelText("Password"), "password123");
    fireEvent.click(screen.getByRole("button", { name: "Login" }));

    expect(login).toHaveBeenCalledWith("test@example.com", "password123");
  });

  // 3. State test — loading = button disabled
  it("should show loading state", () => {
    render(<LoginPage {...defaultProps} loading={true} />);
    expect(screen.getByRole("button", { name: "Logging in..." })).toBeDisabled();
  });

  // 4. Error test — tampilkan error message
  it("should show server error", () => {
    render(<LoginPage {...defaultProps} error="Invalid credentials" />);
    expect(screen.getByText("Invalid credentials")).toBeInTheDocument();
  });
});
```

**Kenapa `vi.fn().mockResolvedValue(undefined)`?** `login` adalah async function. Kalau pakai `vi.fn()` biasa → return `undefined` → `await undefined` → crash. `mockResolvedValue` bikin mock function return Promise yang resolve ke `undefined`.

**Kenapa `userEvent.type()` bukan `fireEvent.change()`?** `userEvent` mensimulasikan interaksi user asli: ketik karakter satu-per-satu, dengan delay realistis. `fireEvent.change` cuma trigger event DOM — lebih cepat tapi kurang realistis. Rule of thumb: **pakai `userEvent` untuk interaction test, `fireEvent` untuk quick trigger**.

---

### Konsep 4: Pre-commit Hooks — Cegah Commit Kotor

`lint-staged` menjalankan linter + formatter **hanya pada file yang di-staging** (bukan seluruh project). Ini bikin pre-commit hook super cepat.

```json
// package.json
"lint-staged": {
  "*.{ts,tsx}": ["eslint --fix", "prettier --write"],
  "*.{json,css,md}": ["prettier --write"]
}
```

**Kenapa cuma staged files?** Di project besar, `eslint src/` bisa makan 30+ detik. `lint-staged` cuma proses file yang mau di-commit (~1 detik). Developer experience > completeness.

**Kenapa gak pakai husky?** Husky butuh install git hooks + setup script. `lint-staged` bisa dijalankan manual: `npx lint-staged`. Untuk project kecil, manual sudah cukup — tidak worth overhead husky.

---

### Skill yang Dikuasai

| Skill | Tool | Real-World Usage |
|-------|------|------------------|
| Component testing | Vitest + React Testing Library | Semua React project |
| Mock pattern | `vi.fn()` + props injection | Unit test di semua framework |
| Linting | ESLint flat config v10 | Semua JS/TS project |
| Formatting | Prettier | Semua project |
| Pre-commit hooks | lint-staged | Team development |
| CI quality gates | GitHub Actions lint + test step | Every CI pipeline |
| Test setup | jsdom, globals, setupFiles | Standard Vitest config |

---

### Referensi

#### Testing
- [Vitest Docs](https://vitest.dev/)
- [React Testing Library](https://testing-library.com/react)
- [Common mistakes with RTL](https://kentcdodds.com/blog/common-mistakes-with-react-testing-library)

#### Code Quality
- [ESLint Flat Config](https://eslint.org/docs/latest/use/configure/configuration-files)
- [typescript-eslint](https://typescript-eslint.io/getting-started/)
- [Prettier Docs](https://prettier.io/docs/)
- [lint-staged](https://github.com/lint-staged/lint-staged)

#### Related Project Docs
- `docs/superpowers/specs/2026-07-16-phase9-backend-testing-design.md` — Part 1 design spec (backend)
- `docs/PHASE8_IMPLEMENTASI_DEVOPS.md` — CI/CD pipeline tempat lint + test jalan
- `docs/PHASE10_IMPLEMENTASI_SECURITY.md` — security hardening Phase 10
- `TODO.md` — full project roadmap

---

**Phase 9 Complete** ✅  
Dari 0 test + 0 linter di frontend ke 3 file, 13 test, ESLint flat config, Prettier, lint-staged, dan CI integration. Backend 67% coverage + frontend testing infrastructure siap — aplikasi ini punya quality gates di setiap layer.
