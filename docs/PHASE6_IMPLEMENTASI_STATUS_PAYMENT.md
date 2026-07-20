# Phase 6: Invoice Status & Payment Tracking — Learning Summary

**Status**: ✅ SELESAI
**Tanggal**: 20 Juli 2026
**Scope**: Invoice status lifecycle (Draft/Sent/Paid/Overdue/Cancelled) + status history audit trail + payment recording + auto-Paid trigger + status filter + analytics dashboard update

---

## Apa yang Kita Pelajari?

Phase 6 adalah tentang **membangun state machine di atas database relasional**. Invoice bukan lagi sekadar dokumen statis — sekarang punya lifecycle: dibuat (Draft), dikirim ke client (Sent), dibayar (Paid), kadaluarsa (Overdue), atau dibatalkan (Cancelled). Setiap perubahan status tercatat di audit trail. Setiap pembayaran tercatat di tabel terpisah, dan ketika total pembayaran mencapai nilai invoice, status otomatis berubah ke Paid.

Ini adalah real-world pattern yang muncul di hampir semua aplikasi bisnis: **order management, invoice tracking, ticket systems, approval workflows** — semuanya butuh state machine + audit trail + trigger otomatis.

---

## Problem: Invoice Tanpa Status

### ❌ Sebelum Phase 6

```go
type Invoice struct {
    ID          string  `json:"id"`
    ClientName  string  `json:"client_name"`
    Date        string  `json:"date"`
    Items       []InvoiceItem `json:"items"`
    TaxRate     float64 `json:"tax_rate"`
    TotalAmount float64 `json:"total_amount"`
    // ... tidak ada status, tidak ada due_date, tidak ada payment tracking
}
```

**Masalah:**
1. **Gak bisa bedain invoice yang udah dibayar vs belum** — semua invoice kelihatan sama
2. **Gak ada due date** — gak bisa tracking mana yang telat bayar
3. **Gak ada payment history** — gak tau kapan dan berapa yang udah dibayar client
4. **Dashboard Phase 5 gak lengkap** — metric Paid/Pending/Overdue sengaja di-skip karena datanya belum ada
5. **Gak ada audit trail** — gak tau siapa yang ubah status dan kapan

### ✅ Setelah Phase 6

```go
type Invoice struct {
    // ... existing fields
    DueDate     string  `json:"due_date"`  // NEW
    Status      string  `json:"status"`    // NEW — Draft/Sent/Paid/Overdue/Cancelled
}

type Payment struct {        // NEW TABLE
    ID         string  `json:"id"`
    InvoiceID  string  `json:"invoice_id"`
    Amount     float64 `json:"amount"`
    Date       string  `json:"date"`
    Method     string  `json:"method"`     // Transfer, Cash, Credit Card, dll
    RecordedBy string  `json:"recorded_by"`
}

type StatusHistoryEntry struct {  // NEW TABLE — audit trail
    ID        string  `json:"id"`
    InvoiceID string  `json:"invoice_id"`
    OldStatus *string `json:"old_status"` // nullable — null untuk entry pertama
    NewStatus string  `json:"new_status"`
    ChangedBy string  `json:"changed_by"`
    ChangedAt string  `json:"changed_at"`
}
```

---

## Arsitektur: Hybrid State Machine

### Kenapa Hybrid (Manual + Auto)?

Di dunia nyata, gak semua status transition dilakukan manual oleh user. Ada kombinasi:
- **Manual** — user mengubah status (Draft → Sent, "saya udah kirim invoice ke client")
- **Auto-computed** — sistem menghitung (Overdue = `due_date < today` AND `status NOT IN ('Paid','Cancelled')`)
- **Auto-triggered** — sistem merespons event (Paid = `SUM(payments.amount) >= total_amount`)

```
                    ┌──────────┐
                    │  Draft   │ ← default saat invoice dibuat
                    └────┬─────┘
                         │ (MANUAL: user clicks "Mark as Sent")
                    ┌────▼─────┐
                    │   Sent   │
                    └────┬─────┘
                         │
              ┌──────────┼──────────┐
              │ (AUTO)    │ (AUTO)   │ (MANUAL)
         ┌────▼───┐  ┌───▼────┐  ┌──▼───────┐
         │  Paid  │  │Overdue │  │Cancelled │
         └────────┘  └────────┘  └──────────┘
         TERMINAL     COMPUTED    TERMINAL
```

**Transition rules di kode:**

```go
var allowedTransitions = map[string]map[string]bool{
    "Draft": {"Sent": true, "Cancelled": true},
    "Sent":  {"Cancelled": true},
    // Paid tidak ada karena auto-only
    // Overdue tidak ada karena computed (gak pernah di-set manual)
}

func isValidStatusTransition(oldStatus, newStatus string) bool {
    // Tolak kalau mencoba set Paid atau Overdue secara manual
    if newStatus == "Paid" || newStatus == "Overdue" {
        return false
    }
    allowed, ok := allowedTransitions[oldStatus]
    return ok && allowed[newStatus]
}
```

**Kenapa pake map `map[string]map[string]bool` bukan `switch`/`if-else`?** Lebih deklaratif — nambah transition baru tinggal tambah entry di map, gak usah ubah logic. Kalau ada 10 status dengan 20 transition, `switch` bakal jadi monster.

---

## Konsep 1: SQL State Machine

Status bukan cuma string di kolom — ada **business logic** yang nge-atur kapan status bisa berubah dan ke mana.

### Validasi di backend

```go
func handleSetInvoiceStatus(c *gin.Context) {
    // 1. Fetch current status DENGAN ownership check
    var currentStatus string
    err := db.QueryRow(ctx,
        `SELECT status FROM invoices WHERE id = $1 AND user_id = $2`,
        invoiceID, userID,
    ).Scan(&currentStatus)

    // 2. Validasi transition
    if !isValidStatusTransition(currentStatus, req.Status) {
        c.JSON(http.StatusBadRequest, gin.H{
            "error": fmt.Sprintf("invalid status transition: %s→%s",
                currentStatus, req.Status),
        })
        return
    }

    // 3. Update status
    db.Exec(ctx, `UPDATE invoices SET status = $1, updated_at = $2 WHERE id = $3`,
        req.Status, now, invoiceID)

    // 4. Catat di audit trail
    writeStatusHistory(ctx, invoiceID, currentStatus, req.Status, userID)

    c.JSON(http.StatusOK, gin.H{"status": req.Status, "previous_status": currentStatus})
}
```

**Kenapa `WHERE id = $1 AND user_id = $2` di SELECT awal?** Ownership check — pastiin user A gak bisa ubah status invoice milik user B. Pattern yang sama dipake di semua handler Phase 5-6.

---

## Konsep 2: Computed State (Overdue)

Overdue **gak pernah disimpan** sebagai nilai di kolom `status`. Kenapa? Karena begitu disimpan, dia bisa jadi **stale** — hari ini Overdue, besok client bayar → harusnya gak Overdue lagi. Kalau disimpan sebagai nilai statis, kita harus bikin cron job atau trigger buat update-nya.

**Pendekatan yang lebih bersih: computed at query time.**

```sql
-- GET /api/invoices?status=Overdue
SELECT ... FROM invoices
WHERE user_id = $1
  AND status NOT IN ('Paid','Cancelled')
  AND due_date < CURRENT_DATE
ORDER BY created_at DESC
```

Gak ada kolom `status = 'Overdue'` di database. Query di atas nge-filter invoice yang **harusnya** overdue berdasarkan kondisi real-time. Ini pattern yang sama dipake di sistem production — gak perlu cron job, gak perlu trigger, selalu akurat.

**Kenapa `NOT IN ('Paid','Cancelled')`?** Kalau invoice udah Paid atau Cancelled, dia gak mungkin overdue — meskipun `due_date`-nya udah lewat.

---

## Konsep 3: Aggregation Trigger (Auto-Paid)

Ketika payment direkam, backend nge-cek apakah total pembayaran udah mencapai nilai invoice. Kalau ya → status auto-berubah ke Paid.

```go
func handleRecordPayment(c *gin.Context) {
    // 1. Insert payment
    db.Exec(ctx, `INSERT INTO payments (...) VALUES (...)`, ...)

    // 2. Hitung total semua pembayaran untuk invoice ini
    var totalPaid float64
    db.QueryRow(ctx,
        `SELECT COALESCE(SUM(amount), 0) FROM payments WHERE invoice_id = $1`,
        invoiceID,
    ).Scan(&totalPaid)

    // 3. Kalau lunas, auto-transition ke Paid
    if totalPaid >= inv.TotalAmount && inv.Status != "Paid" {
        db.Exec(ctx, `UPDATE invoices SET status = 'Paid', updated_at = $1 WHERE id = $2`,
            now, invoiceID)
        writeStatusHistory(ctx, invoiceID, inv.Status, "Paid", userID)
    }

    c.JSON(http.StatusCreated, gin.H{
        "payment":        payment,
        "invoice_status": newStatus,
    })
}
```

**Kenapa `>=` bukan `==`?** Overpayment — client bayar lebih (entah sengaja atau gak). Tetep dianggap lunas.

**Kenapa `COALESCE(SUM(amount), 0)` bukan `SUM(amount)` langsung?** Kalau belum ada payment sama sekali, `SUM()` return `NULL`, bukan `0`. `COALESCE` memastikan kita selalu dapet angka.

**Partial payments didukung**: kalau total baru 600 dari 1000, status tetep Sent. Baru pas payment kedua (400) mencapai 1000, status auto-Paid.

---

## Konsep 4: Audit Trail (Status History)

Setiap perubahan status dicatat di tabel `status_history`:

```sql
CREATE TABLE status_history (
    id UUID PRIMARY KEY,
    invoice_id UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    old_status VARCHAR(20),          -- nullable — entry pertama (Draft) gak punya old_status
    new_status VARCHAR(20) NOT NULL,
    changed_by UUID REFERENCES users(id),
    changed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
```

**Kenapa `old_status` nullable?** Saat invoice pertama kali dibuat, statusnya `Draft` (DEFAULT di kolom). Tapi kita gak nulis history entry pas create — baru nulis pas ada **perubahan** status. Jadi entry pertama selalu punya `old_status` yang bukan NULL (nilai sebelum transisi).

Contoh: Invoice dibuat → `status = 'Draft'` (gak ada history entry). User klik "Mark as Sent" → satu history entry dengan `old_status = 'Draft'`, `new_status = 'Sent'`.

**Pattern query audit trail:**

```go
func handleStatusHistory(c *gin.Context) {
    // Ownership check via invoice
    var ownerID string
    db.QueryRow(ctx, `SELECT user_id FROM invoices WHERE id = $1`, invoiceID).Scan(&ownerID)
    if ownerID != userID {
        c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
        return
    }

    rows, _ := db.Query(ctx,
        `SELECT id, invoice_id, old_status, new_status, changed_by, changed_at
         FROM status_history WHERE invoice_id = $1 ORDER BY changed_at ASC`,
        invoiceID,
    )
    // Scan rows...
    c.JSON(http.StatusOK, history)
}
```

---

## Konsep 5: Dynamic Query Building di Go

Status filter di GET `/api/invoices?status=X` butuh query yang dinamis — `Overdue` itu computed filter (gak cuma `WHERE status = $2`).

```go
statusFilter := c.DefaultQuery("status", "")

query := `SELECT ... FROM invoices WHERE user_id = $1`
args := []interface{}{userID}

if statusFilter == "Overdue" {
    query += ` AND status NOT IN ('Paid','Cancelled') AND due_date < CURRENT_DATE`
} else if statusFilter != "" {
    query += ` AND status = $2`
    args = append(args, statusFilter)
}
query += ` ORDER BY created_at DESC`

rows, err := db.Query(ctx, query, args...)
```

**Kenapa `args ...interface{}`?** `db.Query` nerima variadic `...interface{}` — kita build slice of args secara dinamis. Kalau Overdue, cuma 1 arg (`userID`). Kalau filter Draft, 2 args (`userID`, `"Draft"`).

**Kenapa gak string interpolation (`fmt.Sprintf`)?** SQL injection. Selalu pakai parameterized query (`$1`, `$2`) — value dari user gak pernah langsung di-concat ke query string.

**Kenapa `[]interface{}{userID}` bukan `[]string{userID}`?** `db.Query` expect `...interface{}`, jadi slice-nya harus `[]interface{}`.

---

## Konsep 6: Nullable Values di Go + PostgreSQL

`due_date` adalah kolom nullable (`DATE` tanpa `NOT NULL`). Di Go, kita harus handle konversi `NULL` ↔ Go zero value.

### Problem: Insert NULL ke kolom DATE

Kalau user gak set `due_date`, frontend kirim `""` (empty string). PostgreSQL nolak `""` sebagai nilai `DATE`.

**Solusi: `interface{}` sebagai bridge:**

```go
var dueDate interface{}  // nil by default (Go zero value for interface)
if input.DueDate != "" {
    dueDate = input.DueDate
}

// Di VALUES: $5 = nil (PostgreSQL NULL) atau string (PostgreSQL DATE)
db.Exec(ctx, `INSERT INTO invoices (..., due_date, ...) VALUES ($1, $2, ..., $5, ...)`,
    ..., dueDate, ...)
```

`interface{}` nilai `nil` → driver pgx kirim SQL `NULL`. Nilai `string` → driver kirim string yang di-cast ke DATE oleh PostgreSQL.

### Problem: Baca NULL dari kolom DATE

```sql
SELECT COALESCE(CAST(due_date AS TEXT), '') FROM invoices
```

`CAST(due_date AS TEXT)` dari NULL tetep NULL. pgx gak bisa scan NULL ke Go `string` → error. Solusi: `COALESCE(..., '')` memastikan selalu return string kosong, bukan NULL.

---

## Konsep 7: Multi-User Isolation yang Diperluas

Setiap resource baru (status change, payment recording) harus di-isolasi per user — user A gak boleh ubah status atau record payment untuk invoice user B.

**Pattern yang sama di semua handler:**

```go
// Ownership check melalui invoice
var ownerID string
err := db.QueryRow(ctx,
    `SELECT user_id FROM invoices WHERE id = $1`, invoiceID,
).Scan(&ownerID)

if err != nil || ownerID != userID.(string) {
    c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
    return
}
```

**Kenapa return 404, bukan 403?** Security best practice: jangan kasih tau attacker bahwa resource itu ada tapi bukan punya dia. "Not found" menyembunyikan keberadaan resource dari user yang gak berhak. Pattern yang sama dari Phase 5 (`TestInvoiceIsolationBetweenUsers`).

---

## Dashboard Update: Analytics yang Sekarang Komplit

Phase 5 sengaja skip metric Paid/Pending/Overdue karena belum ada `status`. Phase 6 melengkapi:

```sql
SELECT
    COALESCE(SUM(total_amount), 0) as total_revenue,
    COUNT(*) as total_invoices,
    COUNT(DISTINCT client_id) as total_clients,
    CASE WHEN COUNT(*) > 0 THEN SUM(total_amount) / COUNT(*) ELSE 0 END as avg_value,
    -- NEW ↓
    COALESCE(SUM(CASE WHEN status = 'Paid' THEN total_amount ELSE 0 END), 0) as paid_amount,
    COALESCE(SUM(CASE WHEN status IN ('Draft','Sent') AND (due_date IS NULL OR due_date >= CURRENT_DATE) THEN total_amount ELSE 0 END), 0) as pending_amount,
    COUNT(CASE WHEN status NOT IN ('Paid','Cancelled') AND due_date < CURRENT_DATE THEN 1 END) as overdue_count
FROM invoices WHERE user_id = $1;
```

**`CASE WHEN ... THEN ... ELSE 0 END` di dalam agregasi** — ini pattern SQL yang powerful: conditional aggregation. Mirip `SUMIF` di Excel.

**Kenapa `due_date IS NULL OR due_date >= CURRENT_DATE` di pending?** Invoice tanpa due_date dianggap belum jatuh tempo (pending, bukan overdue). Tanpa ini, invoice tanpa due_date bisa salah masuk ke overdue.

---

## File Baru & Modifikasi

```
backend/
  main.go               [MODIFIED] +Invoice.DueDate, +Invoice.Status, +Payment struct,
                                   +StatusHistoryEntry, +StatusChangeRequest, +PaymentRequest
  analytics.go          [MODIFIED] +AnalyticsOverview.{PaidAmount,PendingAmount,OverdueCount},
                                   updated overview SQL with CASE WHEN aggregation
  router.go             [MODIFIED] status/payment routes, status filter on GET invoices,
                                   all invoice queries updated (due_date COALESCE, status column),
                                   INSERT sets status='Draft', DueDate nullable via interface{}
  status.go             [NEW]     handleSetInvoiceStatus, handleStatusHistory,
                                   isValidStatusTransition, writeStatusHistory
  payments.go           [NEW]     handleRecordPayment (+ auto-Paid trigger), handleListPayments
  status_test.go        [NEW]     4 tests: happy path, invalid transitions, not found, isolation
  payments_test.go       [NEW]     5 tests: partial, full, multi, cancelled, isolation
  invoices_test.go       [MODIFIED] +TestInvoiceStatusFilter, +TestInvoiceOverdueAppearsInFilter
  analytics_test.go      [MODIFIED] verify PaidAmount=0/PendingAmount=revenue/OverdueCount=0
  
  migrations/
    000008_add_status_to_invoices.up/down.sql
    000009_create_status_history.up/down.sql
    000010_create_payments.up/down.sql

frontend/src/
  types/
    invoice.ts           [MODIFIED] Invoice +due_date/+status, +Payment, +StatusHistoryEntry
    analytics.ts         [MODIFIED] AnalyticsOverview +paid_amount/+pending_amount/+overdue_count
  components/
    ProtectedInvoiceDashboard.tsx  [MODIFIED] StatusBadge, status filter dropdown,
                                              "Mark as Sent"/"Cancel" buttons, payment form
    DashboardCards.tsx             [MODIFIED] 3 new metric cards (Paid/Pending/Overdue)
```

---

## Masalah yang Ditemui & Diperbaiki

### 1. DueDate NULL → Scan Gagal

**Masalah**: `CAST(due_date AS TEXT)` dari NULL tetep NULL. pgx gak bisa scan NULL ke Go `string`.

**Gejala**: GET invoice setelah create return 404, padahal invoice-nya ada. Error log: `can't scan NULL into string`.

**Solusi**: Semua SELECT dibungkus `COALESCE(CAST(due_date AS TEXT), '')` — 6 tempat di `router.go`.

### 2. Missing Comma di UPDATE Query

**Masalah**: `"UPDATE invoices SET ... due_date = $3 tax_rate = $4 ..."` — koma setelah `$3` hilang.

**Gejala**: Runtime SQL error pas update invoice.

**Solusi**: Tambah koma. Ditemukan oleh `TestInvoiceLifeCycle`.

### 3. Missing `user_id` di Export Excel SELECT

**Masalah**: Query export/excel SELECT 10 kolom tapi Scan-nya 11 field (user_id gak ada di SELECT).

**Gejala**: Export Excel return 500.

**Solusi**: Tambah `user_id` ke SELECT. Ditemukan oleh `TestExportInvoicesExcel`.

### 4. `DueDate` Empty String → INSERT Gagal

**Masalah**: Frontend/test kirim `DueDate: ""`, PostgreSQL nolak empty string sebagai DATE.

**Gejala**: Create invoice return 400, error: `invalid input syntax for type date: ""`.

**Solusi**: Pakai `var dueDate interface{}` — nil kalau kosong (PostgreSQL NULL), string kalau diisi. Pattern ini dipake di INSERT dan UPDATE handler.

### 5. `inv.Status` Kosong di Response Create

**Masalah**: Handler POST return `input` object langsung. `input.Status` gak di-set (DEFAULT cuma berlaku di DB, bukan di Go struct).

**Gejala**: Test assertion `assert.Equal(t, "Draft", inv.Status)` gagal — dapet string kosong.

**Solusi**: Tambah `input.Status = "Draft"` sebelum `c.JSON(http.StatusCreated, input)`.

---

## Yang Dipelajari di Phase 6

### Database Design
- **State machine di RDBMS** — kombinasi constraints app-level (validasi Go) + constraints DB-level (DEFAULT, NOT NULL, FK)
- **Computed column vs stored column** — Overdue dihitung di query, bukan disimpan; menghindari stale data tanpa cron job
- **Audit trail table** — `status_history` dengan FK ke `invoices` dan `users`, nullable `old_status`
- **Aggregation trigger** — auto-Paid via `SUM(amount) >= total_amount` setelah setiap insert payment
- **Conditional aggregation** — `CASE WHEN ... THEN ... ELSE 0 END` di dalam `SUM()`/`COUNT()` untuk analytics breakdown
- **Table decomposition** — payments dipisah dari invoices (normalisasi), bukan sebagai kolom `paid_amount` di invoices

### Go Patterns
- **Dynamic query building** — `[]interface{}` args + `args...` variadic untuk query dengan filter opsional
- **Nullable values ke PostgreSQL** — `interface{}` sebagai bridge: `nil` → SQL NULL, typed value → SQL value
- **`COALESCE` di SELECT** — handle NULL dari DB ke Go string/number
- **Map-based validation** — `map[string]map[string]bool` untuk transition rules, lebih deklaratif dari `switch`
- **Helper functions antar file** — `writeStatusHistory` di `status.go` dipanggil dari `payments.go` (satu package)

### Testing Patterns
- **State transition testing** — test tiap edge transition: valid, invalid (Paid manual), backward (Sent→Draft), revive (Cancelled→Sent)
- **Auto-trigger testing** — partial payment (status unchanged), full payment (auto-Paid), multiple partials (cumulative)
- **Computed state testing** — overdue filter dengan due_date masa lalu
- **Multi-user isolation extended** — status change + payment recording juga di-isolasi
- **Audit trail verification** — cek jumlah & konten history entry setelah serangkaian transisi

### Architecture Decisions
- **Hybrid state machine vs fully manual** — hybrid lebih real-world untuk business apps
- **Overdue computed vs stored** — computed lebih akurat, gak perlu cron job
- **Separate payments table vs JSON column** — table untuk queryability (`SUM`, `WHERE`, `ORDER BY`)
- **Free-text payment method vs enum** — VARCHAR lebih fleksibel (user bisa ketik metode pembayaran apapun)
- **Nullable due_date** — gak semua invoice harus punya due date, UX lebih baik

---

## Langkah Berikutnya

Setelah Phase 6, yang bisa dikerjakan:

- **Phase 6 Sub-Phase: Payment Gateway (Stripe)** — integrasi payment gateway beneran: bikin payment link, auto-update status dari webhook Stripe, handle refund. Ini project belajar payment processing & async webhook handling.

- **Phase 6 Sub-Phase: Email Notifications** — kirim invoice PDF via email ke client, payment reminder, overdue notification. Butuh setup SMTP (Mailgun/SendGrid) atau queue (Redis/RabbitMQ).

- **Phase 8: DevOps / CI-CD** — GitHub Actions auto-run test suite tiap push, badge coverage di README, deploy ke cloud (DigitalOcean/Railway).

---

## Referensi

### PostgreSQL
- [Conditional Expressions — CASE](https://www.postgresql.org/docs/current/functions-conditional.html)
- [Aggregate Functions](https://www.postgresql.org/docs/current/functions-aggregate.html)
- [COALESCE](https://www.postgresql.org/docs/current/functions-conditional.html#FUNCTIONS-COALESCE-NVL-IFNULL)

### Go
- [pgx — QueryRow, Query, Exec](https://pkg.go.dev/github.com/jackc/pgx/v5)
- [Variadic functions — `...interface{}`](https://go.dev/ref/spec#Passing_arguments_to_..._parameters)
- [Nil interface values](https://go.dev/tour/methods/12)

### Design Patterns
- [State Machine Pattern](https://refactoring.guru/design-patterns/state)
- [Audit Log Pattern](https://martinfowler.com/eaaDev/AuditLog.html)
- [Materialized View vs Computed Query](https://www.postgresql.org/docs/current/rules-materializedviews.html)

### Related Project Docs
- `docs/superpowers/specs/2026-07-20-phase6-status-payment-design.md` — full design spec
- `docs/superpowers/plans/2026-07-20-phase6-status-payment.md` — implementation plan (8 tasks)
- `docs/PHASE9_IMPLEMENTASI_TESTING.md` — Phase 9 learning doc (testing patterns)
- `docs/PHASE5_IMPLEMENTASI_ANALYTICS.md` — Phase 5 learning doc (dashboard analytics)
- `TODO.md` — full project roadmap

---

**Phase 6 Selesai** ✅
Invoice Maker kini punya complete invoice lifecycle dengan state machine, audit trail, dan payment tracking — fitur yang membedakan "project latihan" dari "aplikasi bisnis yang bisa dipakai beneran."
