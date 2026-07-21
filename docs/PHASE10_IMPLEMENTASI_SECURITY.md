# Phase 10: Performance & Security — Learning Summary

**Status**: ✅ Security selesai (Performance deferred)
**Tanggal**: 21 Juli 2026
**Scope**: Rate limiting (token bucket per-IP), security headers via Caddy (HSTS, CSP, X-Frame-Options, X-Content-Type-Options, Referrer-Policy, Permissions-Policy), input validation (Gin binding tags), SQL injection audit, CSRF analysis, database index audit

---

## Apa yang Kita Pelajari?

Phase 10 adalah tentang **defense-in-depth** — keamanan bukan satu lapis, tapi berlapis-lapis. Kalau satu lapis gagal, lapis berikutnya masih melindungi. Kita tidak cuma nambahin fitur, tapi **mengaudit** apa yang sudah aman dan **memperkuat** apa yang belum.

Ini adalah real-world security mindset: bukan "apakah aplikasi ini aman?", tapi "**lapis keamanan apa saja yang sudah terpasang?**" Setiap lapis mengurangi attack surface, dan kalau attacker berhasil melewati satu lapis, masih ada lapis berikutnya.

---

## Problem: Aplikasi Tanpa Security Hardening

### ❌ Sebelum Phase 10

```go
// router.go — tidak ada rate limiting
// Siapapun bisa kirim 1000 request/detik → server overload, DB connection pool habis

// main.go — struct Invoice tanpa validasi
type Invoice struct {
    ClientName  string        `json:"client_name"`            // ❌ bisa kosong
    Date        string        `json:"date"`                   // ❌ bisa kosong
    Items       []InvoiceItem `json:"items"`                  // ❌ bisa kosong, item isinya apa aja
    TaxRate     float64       `json:"tax_rate"`               // ❌ bisa negatif
}

// InvoiceItem gak divalidasi
type InvoiceItem struct {
    Description string  `json:"description"` // ❌ bisa kosong
    Qty         float64 `json:"qty"`         // ❌ bisa 0 atau negatif
    Price       float64 `json:"price"`       // ❌ bisa negatif
}

// Caddyfile — tidak ada security headers
// Browser gak dipaksa HTTPS, bisa di-embed di iframe, XSS gak dicegah
```

**Masalah:**
1. **Gak ada rate limiting** — API bisa di-spam, server bisa crash (DoS)
2. **Input validation minimal** — JSON structure dicek, tapi konten tidak (qty negatif? tax rate -100%? client name kosong?)
3. **Gak ada security headers** — browser gak dilindungi dari XSS, clickjacking, MIME sniffing
4. **HTTPS udah ada (Caddy)** — tapi gak ada HSTS untuk memaksa browser selalu HTTPS
5. **CSRF gak dipahami** — perlu CSRF protection atau tidak? (ternyata tidak: JWT Bearer token = immune)

### ✅ Setelah Phase 10

```go
// struct dengan binding tags
type Invoice struct {
    ClientName  string        `json:"client_name" binding:"required,min=1"`
    Date        string        `json:"date" binding:"required"`
    Items       []InvoiceItem `json:"items" binding:"required,min=1,dive"`
    TaxRate     float64       `json:"tax_rate" binding:"gte=0"`
}

type InvoiceItem struct {
    Description string  `json:"description" binding:"required,min=1"`
    Qty         float64 `json:"qty" binding:"required,gt=0"`
    Price       float64 `json:"price" binding:"required,gte=0"`
}
// ↑ Gin otomatis reject (400) kalau field tidak memenuhi constraint

// Rate limiter middleware — token bucket per IP
r.Use(RateLimitMiddleware())
// ↑ 10 req/s per IP, 429 Too Many Requests kalau limit exceeded
// ↑ /api/health dan /api/metrics di-whitelist

// Caddyfile — 6 security headers di setiap response
header {
    Strict-Transport-Security "max-age=63072000; includeSubDomains; preload"
    Content-Security-Policy   "default-src 'self'; ..."
    X-Frame-Options           "DENY"
    X-Content-Type-Options    "nosniff"
    Referrer-Policy           "strict-origin-when-cross-origin"
    Permissions-Policy        "camera=(), microphone=(), geolocation=()"
}
```

---

## Arsitektur: Defense-in-Depth

```
┌─────────────────────────────────────────────────────────────────┐
│                    DEFENSE IN DEPTH                              │
│                                                                  │
│  Internet                                                        │
│     │                                                            │
│  ┌──▼──────────────────────────────────────────────────────┐    │
│  │ LAYER 1: TLS / HTTPS (Caddy auto-SSL)                    │    │
│  │ Enkripsi data in transit, cegah MITM                     │    │
│  └──┬──────────────────────────────────────────────────────┘    │
│     │                                                            │
│  ┌──▼──────────────────────────────────────────────────────┐    │
│  │ LAYER 2: Security Headers (Caddy)                         │    │
│  │ HSTS → paksa HTTPS                                       │    │
│  │ CSP  → batasi resource (anti-XSS)                         │    │
│  │ XFO  → cegah clickjacking                                 │    │
│  │ XCTO → cegah MIME sniffing                                │    │
│  └──┬──────────────────────────────────────────────────────┘    │
│     │                                                            │
│  ┌──▼──────────────────────────────────────────────────────┐    │
│  │ LAYER 3: Rate Limiting (Go middleware)                    │    │
│  │ Token bucket per-IP, 10 req/s, 429 kalau exceed          │    │
│  │ Cegah DoS, brute force, API abuse                        │    │
│  └──┬──────────────────────────────────────────────────────┘    │
│     │                                                            │
│  ┌──▼──────────────────────────────────────────────────────┐    │
│  │ LAYER 4: Input Validation (Gin binding tags)              │    │
│  │ required, min, gt, gte, email, dive                       │    │
│  │ Tolak input invalid di layer aplikasi                    │    │
│  └──┬──────────────────────────────────────────────────────┘    │
│     │                                                            │
│  ┌──▼──────────────────────────────────────────────────────┐    │
│  │ LAYER 5: Parameterized Queries (pgx)                      │    │
│  │ $1, $2 placeholders → cegah SQL injection                 │    │
│  └──┬──────────────────────────────────────────────────────┘    │
│     │                                                            │
│  ┌──▼──────────────────────────────────────────────────────┐    │
│  │ LAYER 6: Database (PostgreSQL)                             │    │
│  │ FK constraints, ON DELETE CASCADE, 11 indexes              │    │
│  └──────────────────────────────────────────────────────────┘    │
│                                                                  │
│  Kalau Layer 3 jebol (attacker bypass rate limit)...            │
│  → Layer 4 masih validasi input                                 │
│  → Layer 5 masih cegah SQL injection                            │
│  → Layer 6 masih enforce data integrity                         │
└─────────────────────────────────────────────────────────────────┘
```

**Kenapa defense-in-depth?** Single point of failure dalam keamanan = bencana. Kalau cuma ngandelin 1 lapis (misal: "kan udah HTTPS"), terus ada misconfiguration → aplikasi completely exposed. Dengan multiple layers, setiap lapis mengurangi risk independently.

---

## Konsep 1: Rate Limiting — Token Bucket Algorithm

### Kenapa Perlu Rate Limiting?

Tanpa rate limiting, satu user bisa:
- **Brute force login** — kirim 1000 request/detik ke `/api/auth/login`
- **DoS** — habisin connection pool database dengan request masif
- **Scraping** — download semua data via API
- **API abuse** — panggil endpoint mahal (generate PDF) berulang-ulang

Rate limiting adalah **pertahanan pertama** di layer aplikasi. Bukan cuma untuk production — di development pun berguna untuk simulasi real-world behavior.

### Token Bucket: Cara Kerja

```
┌──────────────────────────────────────────┐
│         TOKEN BUCKET (per IP)             │
│                                           │
│   Kapasitas: 20 token (burst)             │
│   Refill: 10 token/detik (rate)           │
│                                           │
│   ┌───┐ ┌───┐ ┌───┐ ┌───┐ ┌───┐         │
│   │ T │ │ T │ │ T │ │ T │ │ T │  ...     │
│   └───┘ └───┘ └───┘ └───┘ └───┘         │
│                                           │
│   Setiap request → consume 1 token        │
│   Token habis → 429 Too Many Requests     │
│   1 detik kemudian → token di-refill      │
└──────────────────────────────────────────┘
```

```
Analoginya: ember bocor.
  - Ember kapasitas 20 liter (BURST = max request berturut-turut)
  - Bocor 10 liter/detik (RATE = refill rate)
  - Setiap request = ambil 1 liter
  - Kalau ember kosong = request ditolak
  - Air terus diisi ulang 10 liter/detik
```

### ❌ Sebelum — Tanpa Rate Limiting

```go
// Semua request diproses tanpa batas
r.GET("/api/invoices", func(c *gin.Context) {
    // ... query database ...
})
// ↑ 1000 request/detik dari 1 IP → semua diproses → DB overload
```

### ✅ Sesudah — Token Bucket per IP

```go
// backend/ratelimit.go
type clientRateLimiter struct {
    mu      sync.RWMutex
    entries map[string]*rateLimiterEntry  // satu bucket per IP
    rate    rate.Limit                    // token/detik
    burst   int                           // kapasitas bucket
}

func RateLimitMiddleware() gin.HandlerFunc {
    limiter := &clientRateLimiter{
        entries: make(map[string]*rateLimiterEntry),
        rate:    rate.Limit(10),  // 10 request/detik
        burst:   20,              // max 20 request berturut-turut
    }

    // Cleanup goroutine — hapus entries IP yg sudah 30 menit gak aktif
    go limiter.cleanup(30 * time.Minute)

    return func(c *gin.Context) {
        // Whitelist: health check + metrics endpoint tidak di-limit
        if pathWhitelist[c.Request.URL.Path] {
            c.Next()
            return
        }

        ip := clientIP(c)
        if !limiter.getLimiter(ip).Allow() {
            c.AbortWithStatusJSON(429, gin.H{"error": "rate_limit_exceeded"})
            return
        }
        c.Next()
    }
}
```

**Kenapa per-IP, bukan global?** Rate limiter global = semua user share 1 bucket → 1 user bisa habisin jatah user lain. Per-IP = setiap user punya bucket sendiri → user A gak bisa mengganggu user B.

**Kenapa ada cleanup goroutine?** Map `entries` akan terus bertambah untuk setiap IP baru yang mengakses API. Tanpa cleanup → memory leak (lama-lama ribuan entry IP yg udah gak aktif). Cleanup jalan setiap 5 menit, hapus IP yang terakhir terlihat >30 menit lalu.

**Kenapa health/metrics di-whitelist?** Docker healthcheck panggil `/api/health` setiap 10 detik. Prometheus scrape `/api/metrics` setiap 15 detik. Kalau kena rate limit → false positive (container dianggap unhealthy, metrics hilang).

### Ekstrak IP Client: X-Real-IP vs X-Forwarded-For

```go
func clientIP(c *gin.Context) string {
    // Prioritas 1: X-Real-IP — di-set oleh Caddy, single value, terpercaya
    if ip := c.GetHeader("X-Real-IP"); ip != "" {
        return ip
    }
    // Prioritas 2: X-Forwarded-For — bisa multiple IP (client, proxy1, proxy2)
    if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
        parts := strings.Split(xff, ",")
        return strings.TrimSpace(parts[0])  // IP pertama = client asli
    }
    // Prioritas 3: RemoteAddr — direct connection
    return c.Request.RemoteAddr
}
```

**Kenapa X-Real-IP dulu?** Di belakang reverse proxy, `RemoteAddr` selalu IP reverse proxy (bukan IP client asli). `X-Real-IP` adalah single trusted value yang di-set oleh Caddy. `X-Forwarded-For` bisa di-spoof oleh client kalau gak dikonfigurasi dengan benar — tapi Caddy menimpa header ini dengan nilai yang benar.

---

## Konsep 2: Security Headers — Browser sebagai Defense Layer

### Kenapa Security Headers?

Security headers bukan melindungi server — tapi **melindungi user** di browser. Setiap header memberi instruksi ke browser tentang apa yang boleh dan tidak boleh dilakukan.

### 6 Header, 6 Fungsi

```
┌─────────────────────────────────────────────────────────────────┐
│                     SECURITY HEADERS                             │
│                                                                  │
│  HSTS (Strict-Transport-Security):                               │
│  "Browser, SELALU pakai HTTPS untuk domain ini, jangan pernah    │
│   coba HTTP lagi selama 2 tahun ke depan."                       │
│  Cegah: SSL stripping, downgrade attack, man-in-the-middle       │
│                                                                  │
│  CSP (Content-Security-Policy):                                  │
│  "Hanya load script/style/font dari domain sendiri ('self').     │
│   Tolak inline script. Tolak external CDN (kecuali diizinkan)."  │
│  Cegah: XSS (Cross-Site Scripting), data injection              │
│                                                                  │
│  X-Frame-Options: DENY                                          │
│  "Jangan izinkan website ini ditampilkan di dalam <iframe>."    │
│  Cegah: Clickjacking (attacker embed website di frame transparan)│
│                                                                  │
│  X-Content-Type-Options: nosniff                                │
│  "Jangan tebak-tebak MIME type, ikuti Content-Type header."     │
│  Cegah: MIME sniffing attack (attacker upload .js dengan        │
│         Content-Type: image/png, browser eksekusi sebagai JS)   │
│                                                                  │
│  Referrer-Policy: strict-origin-when-cross-origin               │
│  "Kirim Referer header hanya untuk same-origin request.         │
│   Untuk cross-origin, kirim hanya origin (tanpa path)."         │
│  Cegah: Leak URL internal ke external site via Referer header   │
│                                                                  │
│  Permissions-Policy: camera=(), microphone=()                   │
│  "Matikan akses ke camera, microphone, geolocation."            │
│  Kurangi attack surface: kalau ada XSS, attacker gak bisa       │
│  akses hardware user                                            │
└─────────────────────────────────────────────────────────────────┘
```

### ❌ Sebelum — Tanpa Headers

```caddy
# Caddyfile tanpa security headers
{$DOMAIN:localhost} {
    handle {
        reverse_proxy frontend:80
    }
    handle_path /api/* {
        reverse_proxy backend:8080
    }
}
# Browser tidak dipaksa HTTPS → user bisa akses HTTP → MITM attack
# CSP tidak diset → kalau ada XSS vulnerability, attacker bisa load script external
# X-Frame-Options tidak diset → website bisa di-embed di iframe pihak lain
```

### ✅ Sesudah — 6 Header via Caddy

```caddy
{$DOMAIN:localhost} {
    header {
        Strict-Transport-Security "max-age=63072000; includeSubDomains; preload"
        Content-Security-Policy   "default-src 'self'; script-src 'self'; ..."
        X-Frame-Options           "DENY"
        X-Content-Type-Options    "nosniff"
        Referrer-Policy           "strict-origin-when-cross-origin"
        Permissions-Policy        "camera=(), microphone=(), geolocation=()"
    }
    # ... routes ...
}
```

**Kenapa dipasang di Caddy, bukan di kode Go?** Security headers harus ada di **setiap** response — HTML, JSON, PDF, CSV, semuanya. Kalau pasang di Go handler, harus diingat satu-satu (risk: ada handler yang lupa). Di reverse proxy = sekali pasang, semua response kena — konsisten dan gak bisa lupa.

**Kenapa CSP `'unsafe-inline'` untuk style?** Tailwind CSS pakai inline style injection di development. Di production, Tailwind tree-shaking menghasilkan CSS file yang bersih — inline style minimal. Kalau strict `style-src 'self'`, beberapa Tailwind utility mungkin rusak. `'unsafe-inline'` + `'self'` = kompromi antara keamanan dan kompatibilitas.

---

## Konsep 3: Input Validation — Gin Binding Tags

### Kenapa Input Validation?

Input dari user **selalu untrusted** — bahkan kalau dari form di frontend kamu sendiri. Attacker bisa bypass frontend dan kirim request langsung ke API (pakai curl, Postman, atau script). Input validation di **backend** adalah mandatory — frontend validation hanya untuk UX.

### ❌ Sebelum — Cuma JSON Structure

```go
type InvoiceItem struct {
    Description string  `json:"description"`  // ❌ bisa "" (kosong)
    Qty         float64 `json:"qty"`          // ❌ bisa 0, bisa -5
    Price       float64 `json:"price"`        // ❌ bisa -1000
}

type Invoice struct {
    ClientName string        `json:"client_name"`  // ❌ bisa "" 
    Items      []InvoiceItem `json:"items"`         // ❌ bisa [], bisa nil
    TaxRate    float64       `json:"tax_rate"`      // ❌ bisa -10 (%)
}

// Di handler:
if err := c.ShouldBindJSON(&input); err != nil {
    // Hanya reject kalau JSON structure salah
    // (field type mismatch, JSON syntax error)
    // TAPI Qty=0, Price=-1000, ClientName="" → LOLOS!
}
```

### ✅ Sesudah — Binding Tags

```go
type InvoiceItem struct {
    Description string  `json:"description" binding:"required,min=1"`
    Qty         float64 `json:"qty" binding:"required,gt=0"`
    Price       float64 `json:"price" binding:"required,gte=0"`
}

type Invoice struct {
    ClientName string        `json:"client_name" binding:"required,min=1"`
    Date       string        `json:"date" binding:"required"`
    Items      []InvoiceItem `json:"items" binding:"required,min=1,dive"`
    TaxRate    float64       `json:"tax_rate" binding:"gte=0"`
}

// Di handler: ShouldBindJSON sekarang otomatis validasi juga
if err := c.ShouldBindJSON(&input); err != nil {
    c.JSON(400, gin.H{"error": err.Error()})
    // ↑ Contoh error: "Key: 'Invoice.Items[0].Qty' Error:Field
    //    validation for 'Qty' failed on the 'gt' tag"
}
```

### Arti Setiap Tag

| Tag | Arti | Contoh Invalid |
|-----|------|---------------|
| `required` | Field tidak boleh nil/empty | `""`, `0`, `nil` |
| `min=1` | String minimal 1 karakter | `""` |
| `gt=0` | Greater than 0 | `0`, `-1` |
| `gte=0` | Greater than or equal 0 | `-1` |
| `email` | Format email valid | `"bukanemail"` |
| `dive` | Validasi setiap element slice/array | Items tanpa dive → isi item tidak dicek |

**Kenapa `dive`?** Tanpa `dive`, `Items []InvoiceItem binding:"required,min=1"` hanya cek "apakah Items ada dan minimal 1 element?" — tapi **isi setiap item tidak divalidasi**. Dengan `dive`, Gin masuk ke setiap element dan validasi `Description`, `Qty`, `Price` sesuai tag masing-masing.

**Kenapa `gte=0` untuk Price, bukan `gt=0`?** Invoice item bisa punya harga Rp 0 (gratis, sample, bonus). Tapi qty gak boleh 0 (gak ada gunanya item dengan quantity 0).

---

## Konsep 4: SQL Injection — Kenapa Sudah Aman

### Kenapa Gak Perlu Diperbaiki?

Project ini **sudah mengikuti best practice** sejak awal: semua query pakai **parameterized queries** (`$1`, `$2`, ...). Ini adalah **satu-satunya pertahanan yang benar** melawan SQL injection.

### Bagaimana SQL Injection Bekerja

```go
// ❌ BAHAYA: string concatenation
clientName := c.Query("client")  // user input
query := "SELECT * FROM invoices WHERE client_name = '" + clientName + "'"
// Attacker kirim: ?client='; DROP TABLE invoices; --
// Query jadi: SELECT * FROM invoices WHERE client_name = ''; DROP TABLE invoices; --'
// Database: hapus tabel invoices!

// ✅ AMAN: parameterized query
clientName := c.Query("client")
row := db.QueryRow(ctx, "SELECT * FROM invoices WHERE client_name = $1", clientName)
// Attacker kirim: ?client='; DROP TABLE invoices; --
// Query jadi: SELECT * FROM invoices WHERE client_name = $1
// Parameter: $1 = "'; DROP TABLE invoices; --"
// Database: mencari client dgn nama "'; DROP TABLE invoices; --"
//             (tidak ada → return empty, TIDAK ADA query ke-2)
```

### Kenapa Parameterized Query Aman?

Parameterized query memisahkan **SQL structure** dari **data**:

```
SQL Structure:  SELECT * FROM invoices WHERE client_name = $1
Data:           $1 = "'; DROP TABLE invoices; --"

Database parser memproses STRUCTURE dulu → lalu masukkan DATA sebagai literal string.
Attacker gak bisa "keluar" dari string literal → input selalu dianggap nilai, bukan perintah.
```

**Kenapa bukan escape string?** `strings.Replace(input, "'", "''")` = **rawan**. Selalu ada edge case yang terlewat. Parameterized query = **database engine sendiri yang handle escaping**, 100% benar.

### Audit Project Ini

```bash
# Cek: apakah ada fmt.Sprintf untuk query?
$ grep -rn 'fmt.Sprintf.*SELECT' --include="*.go" backend/
# (no results) ✅

# Cek: semua query pakai $1, $2, ...?
$ grep -c '\$[0-9]' backend/router.go
# 39 — semua query di router pakai parameterized
```

---

## Konsep 5: CSRF — Kenapa Tidak Perlu

### Kapan CSRF Jadi Masalah?

CSRF (Cross-Site Request Forgery) terjadi kalau:
1. **Session pakai cookies** — browser otomatis kirim cookies ke domain yang sama
2. Attacker bikin website jahat yang kirim request ke API kamu
3. Browser user (yang sedang login di website kamu) otomatis attach session cookie

```
Contoh CSRF attack:
1. User login ke invoice.example.com → browser simpan session cookie
2. User buka evil-site.com (tanpa sadar)
3. evil-site.com punya <form action="https://invoice.example.com/api/invoices" method="POST">
4. Browser otomatis kirim session cookie → request dianggap sah
5. Invoice palsu terbuat tanpa sepengetahuan user
```

### Kenapa Project Ini Immune?

Invoice Maker pakai **JWT Bearer token**, bukan session cookie:

```go
// Frontend kirim token EXPLICITLY di header
fetch('/api/invoices', {
    headers: {
        'Authorization': 'Bearer eyJhbGciOi...'  // ← explicit, manual
    }
})

// BUKAN cookie yang auto-attached oleh browser
```

```
Mekanisme perlindungan:
1. Browser TIDAK PERNAH auto-attach Authorization header
2. Attacker gak bisa baca token dari localStorage/sessionStorage (cross-origin)
3. Attacker gak bisa set Authorization header dari external domain
4. → CSRF attack terhadap Bearer token API = IMPOSSIBLE
```

**Kapan CSRF perlu?** Kalau kamu ganti dari JWT ke session-based auth (cookie + `httpOnly`). Atau kalau ada endpoint yang menerima cookie-based auth selain JWT.

---

## Skill yang Dikuasai

| Skill | Yang Dipelajari | Real-World Usage |
|-------|----------------|------------------|
| Rate limiting | Token bucket algorithm, per-IP isolation, cleanup pattern | API Gateway, WAF, nginx rate limiting |
| Security headers | HSTS, CSP, XFO, XCTO, Referrer, Permissions | Every production website |
| Input validation | Gin binding tags (`required`, `gt`, `gte`, `dive`, `email`) | Every API endpoint |
| SQL injection defense | Parameterized queries vs string concat, audit pattern | Every database query |
| CSRF analysis | Cookie vs Bearer token, when CSRF is relevant | Auth architecture decisions |
| Defense-in-depth | Multi-layer security architecture | Security design for all production systems |
| Security audit | grep audit for injection, index analysis | Code review, penetration testing prep |

---

## Referensi

### Rate Limiting
- [Token Bucket Algorithm](https://en.wikipedia.org/wiki/Token_bucket)
- [golang.org/x/time/rate](https://pkg.go.dev/golang.org/x/time/rate)
- [Rate Limiting Best Practices](https://cloud.google.com/architecture/rate-limiting-strategies-techniques)

### Security Headers
- [OWASP Secure Headers Project](https://owasp.org/www-project-secure-headers/)
- [MDN: Content-Security-Policy](https://developer.mozilla.org/en-US/docs/Web/HTTP/CSP)
- [MDN: Strict-Transport-Security](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Strict-Transport-Security)
- [securityheaders.com](https://securityheaders.com/) — test header website kamu

### Input Validation
- [Gin Binding](https://pkg.go.dev/github.com/gin-gonic/gin#Binding)
- [OWASP Input Validation Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Input_Validation_Cheat_Sheet.html)

### SQL Injection
- [OWASP SQL Injection](https://owasp.org/www-community/attacks/SQL_Injection)
- [PostgreSQL Parameterized Queries](https://www.postgresql.org/docs/current/sql-prepare.html)

### OWASP Top 10
- [OWASP Top 10 (2021)](https://owasp.org/www-project-top-ten/)

### Related Project Docs
- `docs/PHASE8_IMPLEMENTASI_DEVOPS.md` — Phase 8 learning doc (Caddy, CI/CD, monitoring)
- `docs/PHASE6_IMPLEMENTASI_STATUS_PAYMENT.md` — Phase 6 learning doc (state machine, audit trail)
- `TODO.md` — full project roadmap

---

---

## Bonus: React Code Splitting — Lazy Loading

**Tanggal**: 21 Juli 2026

### Kenapa Ini Masuk Phase 10?

Phase 10 mencakup performance optimization. Salah satu quick win terbesar untuk frontend performance adalah **code splitting** — memisahkan bundle JavaScript menjadi chunk-chunk kecil yang di-download hanya saat dibutuhkan.

### Problem: Bundle Monolitik

Semua JavaScript di-download dalam satu bundle besar. Termasuk `recharts` (150KB gzip) — library charting yang hanya dipakai di tab Analytics. User yang cuma bikin invoice tetap harus download library yang tidak dipakai.

### Solusi: React.lazy() + Suspense

```tsx
// ProtectedInvoiceDashboard.tsx
import { lazy, Suspense } from "react";

// Chart components di-split ke chunk terpisah.
// Download hanya saat component pertama kali di-render.
const RevenueChart = lazy(() =>
  import("./RevenueChart").then(m => ({ default: m.RevenueChart }))
);

// Bungkus dengan Suspense + fallback skeleton
<Suspense fallback={<ChartSkeleton />}>
  <RevenueChart ... />
</Suspense>
```

**Kenapa `.then(m => ({ default: m.X }))`?** `React.lazy()` hanya support default export. RevenueChart adalah named export. `.then()` mengubahnya.

**Hasil:** Vite build menghasilkan chunk terpisah untuk `recharts` + chart components. User yang tidak buka Analytics tidak mendownload library 150KB.

**Kenapa tidak lazy-load routes?** Project belum pakai router (React Router). Navigasi pakai state. Lazy-load routes butuh implementasi router — overkill untuk benefit yang sama. Lazy-load components yang berat = 80% benefit dengan 20% effort.

---

**Phase 10 Complete** ✅  
Invoice Maker kini punya defense-in-depth: TLS + security headers + rate limiting + input validation + parameterized queries + database constraints + code splitting. Ini adalah security + performance baseline yang membedakan "project belajar" dari "aplikasi yang siap production."
