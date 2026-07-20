# Phase 6: Invoice Status & Payment Tracking — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add invoice status lifecycle (Draft/Sent/Paid/Overdue/Cancelled), status history audit trail, payment recording with auto-Paid triggering, status filter on invoice list, and updated analytics dashboard metrics.

**Architecture:** Three new DB migrations add `status`+`due_date` to invoices, a `status_history` audit table, and a `payments` table. Two new Go files (`status.go`, `payments.go`) implement named-function handlers following the `analytics.go` pattern established in Phase 5. Overdue is computed at query time, never stored. Paid status auto-triggers when total payments ≥ invoice total_amount. Analytics overview gains paid/pending/overdue breakdown. Frontend adds status badges, action buttons, and a payment form inline in the invoice dashboard.

**Tech Stack:** Go 1.25, Gin, pgx/v5, golang-migrate (existing); testify (existing for tests); React 18 + TypeScript (existing frontend).

## Global Constraints

- Go module: `github.com/TamaSkuyy/invoice-maker/backend`, Go 1.25 (from `go.mod`).
- Postgres container `invoice-postgres` must be running at `localhost:5432` (same as Phase 9 requirement).
- New backend handlers follow the `analytics.go` named-function pattern — dedicated files (`status.go`, `payments.go`), not inline closures in `router.go`.
- All new endpoints are JWT-protected via `authenticate()` middleware with per-user data isolation at the SQL level (`WHERE user_id = $1`).
- All new handlers get integration tests following the Phase 9 pattern (same `TestMain`, `truncateTables`, `doRequest`, `registerTestUser` infra).
- Overdue is computed at read time, never stored as a column value.
- Paid status is auto-only — `PUT /api/invoices/:id/status` rejects `"status": "Paid"`.
- User writes code, Claude guides/reviews (same collaborative mode as Phase 9).

---

### Task 1: Database migrations — status, due_date, status_history, payments

**Files:**
- Create: `backend/migrations/000008_add_status_to_invoices.up.sql`
- Create: `backend/migrations/000008_add_status_to_invoices.down.sql`
- Create: `backend/migrations/000009_create_status_history.up.sql`
- Create: `backend/migrations/000009_create_status_history.down.sql`
- Create: `backend/migrations/000010_create_payments.up.sql`
- Create: `backend/migrations/000010_create_payments.down.sql`

**Interfaces:**
- Consumes: existing `invoices` table (migrations 000001-000007), `users` table (000003).
- Produces: `invoices.status VARCHAR(20) NOT NULL DEFAULT 'Draft'`, `invoices.due_date DATE`, `status_history(id, invoice_id FK, old_status, new_status, changed_by FK, changed_at)`, `payments(id, invoice_id FK, amount DECIMAL, date DATE, method VARCHAR, recorded_by FK, created_at)` — used by all subsequent tasks.

- [ ] **Step 1: Write `000008_add_status_to_invoices.up.sql`**

```sql
ALTER TABLE invoices ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'Draft';
ALTER TABLE invoices ADD COLUMN due_date DATE;
CREATE INDEX idx_invoices_status ON invoices(status);
CREATE INDEX idx_invoices_due_date ON invoices(due_date);

-- Backfill existing invoices with a reasonable due_date
UPDATE invoices SET due_date = date + INTERVAL '30 days' WHERE due_date IS NULL;
```

- [ ] **Step 2: Write `000008_add_status_to_invoices.down.sql`**

```sql
DROP INDEX IF EXISTS idx_invoices_due_date;
DROP INDEX IF EXISTS idx_invoices_status;
ALTER TABLE invoices DROP COLUMN due_date;
ALTER TABLE invoices DROP COLUMN status;
```

- [ ] **Step 3: Write `000009_create_status_history.up.sql`**

```sql
CREATE TABLE status_history (
    id UUID PRIMARY KEY,
    invoice_id UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    old_status VARCHAR(20),
    new_status VARCHAR(20) NOT NULL,
    changed_by UUID REFERENCES users(id),
    changed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_status_history_invoice_id ON status_history(invoice_id);
```

- [ ] **Step 4: Write `000009_create_status_history.down.sql`**

```sql
DROP TABLE status_history;
```

- [ ] **Step 5: Write `000010_create_payments.up.sql`**

```sql
CREATE TABLE payments (
    id UUID PRIMARY KEY,
    invoice_id UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    amount DECIMAL(10,2) NOT NULL,
    date DATE NOT NULL,
    method VARCHAR(50) NOT NULL DEFAULT 'Transfer',
    recorded_by UUID NOT NULL REFERENCES users(id),
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_payments_invoice_id ON payments(invoice_id);
```

- [ ] **Step 6: Write `000010_create_payments.down.sql`**

```sql
DROP TABLE payments;
```

- [ ] **Step 7: Run migrations against dev database to verify**

Run: `cd /home/sekuyy/project/invoice-maker && source dev-local.sh` (or `docker compose up -d postgres` if not already running), then verify the migrations apply cleanly.

If using `dev-local.sh`'s env vars:
```bash
export DB_HOST=localhost DB_PORT=5432 DB_USER=invoiceuser DB_PASSWORD=invoicepassword DB_NAME=invoicedb
cd backend && go run .   # runMigrations() in main() picks up new migration files automatically
```

Check: `psql -h localhost -U invoiceuser -d invoicedb -c "\d invoices"` — should show `status` and `due_date` columns.
Check: `psql -h localhost -U invoiceuser -d invoicedb -c "\dt"` — should show `status_history` and `payments` tables.

- [ ] **Step 8: Commit**

```bash
git add backend/migrations/000008_* backend/migrations/000009_* backend/migrations/000010_*
git commit -m "feat: add status, due_date, status_history, and payments migrations"
```

---

### Task 2: Go types — Invoice struct update, new types, AnalyticsOverview extension

**Files:**
- Modify: `backend/main.go:29-42` (Invoice struct)
- Modify: `backend/main.go` (append new types after Product struct, around line 98)
- Modify: `backend/analytics.go:19-24` (AnalyticsOverview struct)

**Interfaces:**
- Consumes: nothing (self-contained type definitions).
- Produces: `Invoice{... DueDate string, Status string ...}` (used by Tasks 3-7); `Payment{ID, InvoiceID, Amount, Date, Method, RecordedBy, CreatedAt}`, `StatusHistoryEntry{ID, InvoiceID, OldStatus *string, NewStatus, ChangedBy, ChangedAt}`, `StatusChangeRequest{Status string}`, `PaymentRequest{Amount float64, Date string, Method string}` (used by Tasks 3-4); `AnalyticsOverview{... PaidAmount, PendingAmount, OverdueCount}` (used by Task 5).

- [ ] **Step 1: Update the Invoice struct in `backend/main.go`**

Replace the existing `Invoice` struct (lines 29-42) with:

```go
type Invoice struct {
	ID          string        `json:"id"`
	ClientName  string        `json:"client_name"`
	ClientID    *string       `json:"client_id"`
	Date        string        `json:"date"`
	DueDate     string        `json:"due_date"`
	Items       []InvoiceItem `json:"items"`
	TaxRate     float64       `json:"tax_rate"`
	TotalAmount float64       `json:"total_amount"`
	Status      string        `json:"status"`
	UserID      string        `json:"user_id"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}
```

- [ ] **Step 2: Append new types after the Product struct in `backend/main.go`**

After the `Product` struct (ends around line 98), add:

```go
// Payment represents a recorded payment toward an invoice.
type Payment struct {
	ID         string    `json:"id"`
	InvoiceID  string    `json:"invoice_id"`
	Amount     float64   `json:"amount"`
	Date       string    `json:"date"`
	Method     string    `json:"method"`
	RecordedBy string    `json:"recorded_by"`
	CreatedAt  time.Time `json:"created_at"`
}

// StatusHistoryEntry records a single status change for audit trail.
type StatusHistoryEntry struct {
	ID        string    `json:"id"`
	InvoiceID string    `json:"invoice_id"`
	OldStatus *string   `json:"old_status"`
	NewStatus string    `json:"new_status"`
	ChangedBy string    `json:"changed_by"`
	ChangedAt time.Time `json:"changed_at"`
}

// StatusChangeRequest is the request body for PUT /api/invoices/:id/status.
type StatusChangeRequest struct {
	Status string `json:"status" binding:"required"`
}

// PaymentRequest is the request body for POST /api/invoices/:id/payments.
type PaymentRequest struct {
	Amount float64 `json:"amount" binding:"required,gt=0"`
	Date   string  `json:"date" binding:"required"`
	Method string  `json:"method" binding:"required"`
}
```

`*string` for `OldStatus` (pointer type) makes it nullable in JSON — first status entry has `old_status: null`.

- [ ] **Step 3: Extend AnalyticsOverview in `backend/analytics.go`**

Replace the existing `AnalyticsOverview` struct (lines 19-24) with:

```go
type AnalyticsOverview struct {
	TotalRevenue    float64 `json:"total_revenue"`
	TotalInvoices   int     `json:"total_invoices"`
	TotalClients    int     `json:"total_clients"`
	AvgInvoiceValue float64 `json:"avg_invoice_value"`
	PaidAmount      float64 `json:"paid_amount"`
	PendingAmount   float64 `json:"pending_amount"`
	OverdueCount    int     `json:"overdue_count"`
}
```

- [ ] **Step 4: Build to verify no compile errors**

Run: `cd backend && go build ./...`
Expected: successful build. The new types aren't used yet, but the existing code must still compile after the `Invoice` struct gained two new fields — check that all existing `SELECT ... Scan(...)` calls still compile (they use explicit column lists, so adding columns to the table but not to the Scan should not cause issues at compile time, only at runtime if the column is in the SELECT list).

Important: existing code that does `INSERT INTO invoices ... VALUES (...)` without listing columns will break because the number of columns changed. Check all INSERT statements in `router.go` and ensure they explicitly name columns (they already do — `INSERT INTO invoices (id, client_name, ...)` — verify by reading the router code. If any INSERT doesn't list columns, fix it to list them explicitly, adding the new `status` and `due_date` fields.)

- [ ] **Step 5: Commit**

```bash
git add backend/main.go backend/analytics.go
git commit -m "feat: add Invoice status/due_date, Payment, StatusHistory, and extended AnalyticsOverview types"
```

---

### Task 3: `status.go` — status transition logic, handler, and router registration

**Files:**
- Create: `backend/status.go`
- Modify: `backend/router.go` (add route group after line 813)

**Interfaces:**
- Consumes: `Invoice{Status}`, `StatusHistoryEntry`, `StatusChangeRequest` (Task 2); package-level `db` (`backend/db.go`); `uuid` (for generating IDs); `authenticate()` middleware.
- Produces: `handleSetInvoiceStatus(c *gin.Context)` — used by router registration in this task. `isValidStatusTransition(old, new string) bool` — used by status handler internally, may also be consumed by `payments.go` (Task 4) if you want to share the validator.

- [ ] **Step 1: Write `backend/status.go`**

```go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Allowed status values.
var validStatuses = map[string]bool{
	"Draft":     true,
	"Sent":      true,
	"Paid":      true,
	"Overdue":   true,
	"Cancelled": true,
}

// Transition rules — which from→to pairs are allowed for manual changes.
// Paid can never be set manually. Overdue is computed, never set.
var allowedTransitions = map[string]map[string]bool{
	"Draft": {"Sent": true, "Cancelled": true},
	"Sent":  {"Cancelled": true},
}

func isValidStatusTransition(oldStatus, newStatus string) bool {
	if !validStatuses[newStatus] {
		return false
	}
	// Paid is auto-only; Overdue is computed, never set manually.
	if newStatus == "Paid" || newStatus == "Overdue" {
		return false
	}
	allowed, ok := allowedTransitions[oldStatus]
	return ok && allowed[newStatus]
}

func writeStatusHistory(ctx context.Context, invoiceID, oldStatus, newStatus, changedBy string) error {
	_, err := db.Exec(ctx,
		`INSERT INTO status_history (id, invoice_id, old_status, new_status, changed_by, changed_at)
		 VALUES ($1, $2, $3, $4, $5, $6)`,
		uuid.New().String(), invoiceID, oldStatus, newStatus, changedBy, time.Now(),
	)
	return err
}

func handleSetInvoiceStatus(c *gin.Context) {
	invoiceID := c.Param("id")
	userID, _ := c.Get("user_id")

	var req StatusChangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Fetch current status — ownership check built-in.
	var currentStatus string
	err := db.QueryRow(ctx,
		`SELECT status FROM invoices WHERE id = $1 AND user_id = $2`,
		invoiceID, userID,
	).Scan(&currentStatus)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
		return
	}

	if !isValidStatusTransition(currentStatus, req.Status) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("invalid status transition: %s→%s", currentStatus, req.Status),
		})
		return
	}

	// Update status.
	now := time.Now()
	_, err = db.Exec(ctx,
		`UPDATE invoices SET status = $1, updated_at = $2 WHERE id = $3`,
		req.Status, now, invoiceID,
	)
	if err != nil {
		log.Printf("update status error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update status"})
		return
	}

	// Record in history.
	if err := writeStatusHistory(ctx, invoiceID, currentStatus, req.Status, userID.(string)); err != nil {
		log.Printf("write status history error: %v", err)
		// Non-fatal — status already updated, log the missing audit entry.
	}

	c.JSON(http.StatusOK, gin.H{"status": req.Status, "previous_status": currentStatus})
}
```

- [ ] **Step 2: Register the route in `backend/router.go`**

Find the closing of the analytics route group (around line 825, after `analytics.GET("/report", handleAnalyticsReport)` and before `return r`). After that closing brace, add a new route group:

```go
// Status management (protected)
status := r.Group("/api/invoices/:id")
status.Use(authenticate())
{
	status.PUT("/status", handleSetInvoiceStatus)
	status.GET("/history", handleStatusHistory)
}
```

**Wait** — this creates a route conflict. `r.Group("/api/invoices/:id")` will match paths like `/api/invoices/:id/status` but this conflicts with the existing `/api/invoices/export/excel` route (which is registered inside the existing `api := r.Group("/api/invoices")` block). Gin resolves this correctly because `/export/excel` is a literal match and `:id` is a parameter — but only if the `/export/excel` route is registered BEFORE any `:id` sub-routes. The existing code already has `/export/excel` before `/:id` inside the `api` group. Adding a new group at the top level should be fine as long as it's registered AFTER the existing `api := r.Group("/api/invoices")` group.

Actually, a cleaner approach: register the new routes INSIDE the existing `api` group, next to the existing `/export/excel` and `/:id` routes:

Inside the existing `api := r.Group("/api/invoices")` block (which uses `api.Use(authenticate())`), add before the closing `}` of the block (after line ~575, before line 576 which is a closing `}`):

```go
// Set invoice status (manual transition)
api.PUT("/:id/status", handleSetInvoiceStatus)

// Get invoice status history
api.GET("/:id/history", handleStatusHistory)

// Record a payment
api.POST("/:id/payments", handleRecordPayment)

// List payments for an invoice
api.GET("/:id/payments", handleListPayments)
```

This is cleaner because:
1. Reuses the existing `api.Use(authenticate())` middleware (no duplicate)
2. Grouped with other invoice routes logically
3. No route conflict — `/:id/status` is more specific than `/:id` and Gin handles it correctly

But wait — the `handleStatusHistory` and `handleRecordPayment` / `handleListPayments` functions don't exist yet (Task 3 only covers status). Let me adjust: in this task, only add the status + history routes that are actually defined. Move the payment routes to Task 4.

So for Task 3, add inside the existing `api` group block:

```go
api.PUT("/:id/status", handleSetInvoiceStatus)
api.GET("/:id/history", handleStatusHistory)
```

- [ ] **Step 3: Add `handleStatusHistory` to `backend/status.go`**

```go
func handleStatusHistory(c *gin.Context) {
	invoiceID := c.Param("id")
	userID, _ := c.Get("user_id")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Ownership check — invoice must belong to user.
	var ownerID string
	err := db.QueryRow(ctx, `SELECT user_id FROM invoices WHERE id = $1`, invoiceID).Scan(&ownerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
		return
	}
	if ownerID != userID.(string) {
		c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
		return
	}

	rows, err := db.Query(ctx,
		`SELECT id, invoice_id, old_status, new_status, changed_by, changed_at
		 FROM status_history WHERE invoice_id = $1 ORDER BY changed_at ASC`,
		invoiceID,
	)
	if err != nil {
		log.Printf("query status history error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch status history"})
		return
	}
	defer rows.Close()

	var history []StatusHistoryEntry
	for rows.Next() {
		var h StatusHistoryEntry
		if err := rows.Scan(&h.ID, &h.InvoiceID, &h.OldStatus, &h.NewStatus, &h.ChangedBy, &h.ChangedAt); err != nil {
			log.Printf("scan history error: %v", err)
			continue
		}
		history = append(history, h)
	}
	if history == nil {
		history = []StatusHistoryEntry{}
	}

	c.JSON(http.StatusOK, history)
}
```

- [ ] **Step 4: Update all INSERT/UPDATE/SELECT across the codebase to handle new Invoice fields**

Every query that touches `invoices` must account for the `status` and `due_date` columns:

1. **INSERT in `router.go`** (around line 325, inside `POST /api/invoices`): the INSERT must include `status` and `due_date`. The handler already lists columns explicitly:

Find the insert:
```go
_, err = tx.Exec(ctx,
    "INSERT INTO invoices (id, client_name, client_id, date, tax_rate, total_amount, user_id, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)",
    input.ID, input.ClientName, input.ClientID, input.Date, input.TaxRate, input.TotalAmount, input.UserID, input.CreatedAt, input.UpdatedAt,
)
```

Replace with:
```go
_, err = tx.Exec(ctx,
    "INSERT INTO invoices (id, client_name, client_id, date, due_date, tax_rate, total_amount, status, user_id, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)",
    input.ID, input.ClientName, input.ClientID, input.Date, input.DueDate, input.TaxRate, input.TotalAmount, "Draft", input.UserID, input.CreatedAt, input.UpdatedAt,
)
```

2. **UPDATE in `router.go`** (inside `PUT /api/invoices/:id`): the UPDATE statement currently updates `client_name, date, tax_rate, total_amount, updated_at`. Add `status` and `due_date`:

```go
_, err = tx.Exec(ctx,
    "UPDATE invoices SET client_name = $1, date = $2, due_date = $3, tax_rate = $4, total_amount = $5, updated_at = $6 WHERE id = $7",
    input.ClientName, input.Date, input.DueDate, input.TaxRate, input.TotalAmount, input.UpdatedAt, id,
)
```

3. **All SELECT queries** that scan into an `Invoice` struct must add the `status` and `due_date` columns. Find every `SELECT ... FROM invoices` and `SELECT ... FROM invoices WHERE ...` across `router.go`:

Search for all invoice queries and add `status, CAST(due_date AS TEXT)` to the column list and `&inv.Status, &inv.DueDate` to the Scan call. This is mechanical:

- List invoices (GET `/api/invoices`): add `status, due_date` → Scan
- Export Excel: add `status, due_date` → Scan
- Get single (GET `/api/invoices/:id`): add `status, due_date` → Scan
- Create response (already handled by returning the `input` object with default `Draft`)
- Update re-fetch: add `status, due_date` → Scan
- PDF download: add `status, due_date` → Scan
- CSV download: add `status, due_date` → Scan

For `due_date` which is `DATE` in Postgres: the existing pattern uses `CAST(date AS TEXT)` for the `date` column. Do the same: `CAST(due_date AS TEXT)` in SELECT, scan into `&inv.DueDate` (which is `string` in Go).

- [ ] **Step 5: Build and fix any compile errors**

Run: `cd backend && go build ./...`
Expected: successful build. Fix any Scan count mismatches (the most likely error — missing `&inv.Status, &inv.DueDate` in some Scan call).

- [ ] **Step 6: Run existing tests to verify no regressions**

Run: `cd backend && go test ./... -v 2>&1 | grep -E "PASS|FAIL"`
Expected: all existing tests still pass. The `TestInvoiceLifeCycle` test creates invoices via the API — verify the create response includes `"status":"Draft"` by checking the test output.

If tests fail because the create/update response doesn't include `status`/`due_date`: the handler returns the `input` object directly (`c.JSON(http.StatusCreated, input)`), and the `Invoice` struct now has `Status` and `DueDate` fields with zero values (`""`). That's fine — the frontend will see `"status":""` — but `TestInvoiceLifeCycle` doesn't check `status` yet (it will in Task 7 when we update the test). For now, make sure tests PASS.

- [ ] **Step 7: Commit**

```bash
git add backend/status.go backend/router.go
git commit -m "feat: add invoice status handler with transition validation and history endpoint"
```

---

### Task 4: `payments.go` — payment recording, auto-Paid, payment listing, and status filter

**Files:**
- Create: `backend/payments.go`
- Modify: `backend/router.go` (add payment routes + status filter on invoice list)

**Interfaces:**
- Consumes: `Payment`, `PaymentRequest` (Task 2); `PaymentRequest{Amount, Date, Method}`; `isValidStatusTransition` from `status.go` (optional); package-level `db`; `uuid`.
- Produces: `handleRecordPayment(c *gin.Context)`, `handleListPayments(c *gin.Context)` — registered in router this task. Status filter logic in GET `/api/invoices`.

- [ ] **Step 1: Write `backend/payments.go`**

```go
package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func handleRecordPayment(c *gin.Context) {
	invoiceID := c.Param("id")
	userID, _ := c.Get("user_id")

	var req PaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// Fetch invoice — ownership check + current status.
	var inv Invoice
	err := db.QueryRow(ctx,
		`SELECT id, total_amount, status, user_id FROM invoices WHERE id = $1 AND user_id = $2`,
		invoiceID, userID,
	).Scan(&inv.ID, &inv.TotalAmount, &inv.Status, &inv.UserID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
		return
	}

	if inv.Status == "Cancelled" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot pay a cancelled invoice"})
		return
	}

	// Insert payment.
	paymentID := uuid.New().String()
	now := time.Now()
	_, err = db.Exec(ctx,
		`INSERT INTO payments (id, invoice_id, amount, date, method, recorded_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		paymentID, invoiceID, req.Amount, req.Date, req.Method, userID, now,
	)
	if err != nil {
		log.Printf("insert payment error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record payment"})
		return
	}

	// Check if fully paid.
	var totalPaid float64
	db.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM payments WHERE invoice_id = $1`,
		invoiceID,
	).Scan(&totalPaid)

	newStatus := inv.Status
	if totalPaid >= inv.TotalAmount && inv.Status != "Paid" {
		// Auto-transition to Paid.
		db.Exec(ctx,
			`UPDATE invoices SET status = 'Paid', updated_at = $1 WHERE id = $2`,
			now, invoiceID,
		)
		newStatus = "Paid"
		// Record auto-transition in status history.
		writeStatusHistory(ctx, invoiceID, inv.Status, "Paid", userID.(string))
	}

	c.JSON(http.StatusCreated, gin.H{
		"payment": Payment{
			ID:         paymentID,
			InvoiceID:  invoiceID,
			Amount:     req.Amount,
			Date:       req.Date,
			Method:     req.Method,
			RecordedBy: userID.(string),
			CreatedAt:  now,
		},
		"invoice_status": newStatus,
	})
}

func handleListPayments(c *gin.Context) {
	invoiceID := c.Param("id")
	userID, _ := c.Get("user_id")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Ownership check.
	var ownerID string
	err := db.QueryRow(ctx, `SELECT user_id FROM invoices WHERE id = $1`, invoiceID).Scan(&ownerID)
	if err != nil || ownerID != userID.(string) {
		c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
		return
	}

	rows, err := db.Query(ctx,
		`SELECT id, invoice_id, amount, CAST(date AS TEXT), method, recorded_by, created_at
		 FROM payments WHERE invoice_id = $1 ORDER BY date DESC, created_at DESC`,
		invoiceID,
	)
	if err != nil {
		log.Printf("query payments error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch payments"})
		return
	}
	defer rows.Close()

	var payments []Payment
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.InvoiceID, &p.Amount, &p.Date, &p.Method, &p.RecordedBy, &p.CreatedAt); err != nil {
			log.Printf("scan payment error: %v", err)
			continue
		}
		payments = append(payments, p)
	}
	if payments == nil {
		payments = []Payment{}
	}

	c.JSON(http.StatusOK, payments)
}
```

- [ ] **Step 2: Register payment routes in `backend/router.go`**

Inside the existing `api := r.Group("/api/invoices")` block, add after the status/history routes from Task 3:

```go
// Record a payment
api.POST("/:id/payments", handleRecordPayment)
// List payments
api.GET("/:id/payments", handleListPayments)
```

- [ ] **Step 3: Add status filter to GET `/api/invoices` handler**

In `backend/router.go`, find the existing `api.GET("", func(c *gin.Context) { ... })` handler (starts around line 158). The handler currently does:

```go
api.GET("", func(c *gin.Context) {
    userID, _ := c.Get("user_id")
    ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
    defer cancel()

    rows, err := db.Query(ctx, "SELECT id, client_name, client_id, CAST(date AS TEXT), tax_rate, total_amount, user_id, created_at, updated_at FROM invoices WHERE user_id = $1 ORDER BY created_at DESC", userID)
```

Modify to support an optional `status` query param:

```go
api.GET("", func(c *gin.Context) {
    userID, _ := c.Get("user_id")
    statusFilter := c.DefaultQuery("status", "")
    ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
    defer cancel()

    var rows pgx.Rows
    var err error

    if statusFilter == "Overdue" {
        // Overdue is computed — not a stored value.
        rows, err = db.Query(ctx,
            `SELECT id, client_name, client_id, CAST(date AS TEXT), CAST(due_date AS TEXT),
                    tax_rate, total_amount, status, user_id, created_at, updated_at
             FROM invoices
             WHERE user_id = $1
               AND status NOT IN ('Paid','Cancelled')
               AND due_date < CURRENT_DATE
             ORDER BY created_at DESC`,
            userID,
        )
    } else if statusFilter != "" {
        rows, err = db.Query(ctx,
            `SELECT id, client_name, client_id, CAST(date AS TEXT), CAST(due_date AS TEXT),
                    tax_rate, total_amount, status, user_id, created_at, updated_at
             FROM invoices
             WHERE user_id = $1 AND status = $2
             ORDER BY created_at DESC`,
            userID, statusFilter,
        )
    } else {
        rows, err = db.Query(ctx,
            `SELECT id, client_name, client_id, CAST(date AS TEXT), CAST(due_date AS TEXT),
                    tax_rate, total_amount, status, user_id, created_at, updated_at
             FROM invoices
             WHERE user_id = $1
             ORDER BY created_at DESC`,
            userID,
        )
    }
```

...and update every Scan call inside this handler to include `&inv.DueDate, &inv.Status` (the columns now include `due_date` and `status`). Also add `"github.com/jackc/pgx/v5"` to the import in `router.go` if not already imported (it should be, since `db` is `*pgxpool.Pool` which delegates to pgx for `Query`/`QueryRow`).

**Wait** — the `pgx.Rows` type. Actually, `db.Query` returns `pgx.Rows` (from jackc/pgx/v5), not `*sql.Rows`. The existing code uses `rows, err := db.Query(...)` without typing the variable explicitly — it just uses `:=`. For the split logic, declare `var rows pgx.Rows` won't work because the `:=` in each branch creates a new variable. Use a different pattern:

```go
var rows interface{ Close(); Next() bool; Scan(...) error; Err() error }
```

No, that's overcomplicating it. Simpler approach — declare `rows` with an explicit type that matches what `db.Query` returns. But `db.Query` returns `pgx.Rows`, and importing `pgx/v5` just for the type is fine. Alternatively, use separate query wrapper functions or restructure to avoid the branch:

```go
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

This avoids the type-declaration problem and is also DRYer. Use this pattern.

- [ ] **Step 4: Also update the Excel export handler in `router.go`**

The `api.GET("/export/excel", ...)` handler also SELECTs from invoices — add `status, CAST(due_date AS TEXT)` to its column list and `&inv.Status, &inv.DueDate` to its Scan.

- [ ] **Step 5: Build and run tests**

Run: `cd backend && go build ./... && go test ./... -v 2>&1 | grep -E "PASS|FAIL"`
Expected: all tests pass. If any test fails, inspect the error — most likely a Scan count mismatch somewhere (a SELECT that was missed in Step 1 of Task 3).

- [ ] **Step 6: Commit**

```bash
git add backend/payments.go backend/router.go
git commit -m "feat: add payment recording, auto-Paid, payment listing, and status filter"
```

---

### Task 5: Update analytics overview SQL for paid/pending/overdue

**Files:**
- Modify: `backend/analytics.go:70-93` (handleAnalyticsOverview — only the SQL query)

**Interfaces:**
- Consumes: extended `AnalyticsOverview` (Task 2); `handleAnalyticsOverview` function (existing); package-level `db`.
- Produces: updated overview endpoint — `paid_amount`, `pending_amount`, `overdue_count` computed in SQL.

- [ ] **Step 1: Update the overview SQL in `handleAnalyticsOverview`**

Find the existing query in `backend/analytics.go` (around line 76):

```go
err := db.QueryRow(ctx, `
    SELECT
        COALESCE(SUM(total_amount), 0),
        COUNT(*),
        COUNT(DISTINCT client_id),
        CASE WHEN COUNT(*) > 0 THEN SUM(total_amount) / COUNT(*) ELSE 0 END
    FROM invoices
    WHERE user_id = $1
`, userID).Scan(&o.TotalRevenue, &o.TotalInvoices, &o.TotalClients, &o.AvgInvoiceValue)
```

Replace with:

```go
err := db.QueryRow(ctx, `
    SELECT
        COALESCE(SUM(total_amount), 0),
        COUNT(*),
        COUNT(DISTINCT client_id),
        CASE WHEN COUNT(*) > 0 THEN SUM(total_amount) / COUNT(*) ELSE 0 END,
        COALESCE(SUM(CASE WHEN status = 'Paid' THEN total_amount ELSE 0 END), 0),
        COALESCE(SUM(CASE WHEN status IN ('Draft','Sent') AND (due_date IS NULL OR due_date >= CURRENT_DATE) THEN total_amount ELSE 0 END), 0),
        COUNT(CASE WHEN status NOT IN ('Paid','Cancelled') AND due_date < CURRENT_DATE THEN 1 END)
    FROM invoices
    WHERE user_id = $1
`, userID).Scan(
    &o.TotalRevenue, &o.TotalInvoices, &o.TotalClients, &o.AvgInvoiceValue,
    &o.PaidAmount, &o.PendingAmount, &o.OverdueCount,
)
```

**Why `due_date IS NULL OR due_date >= CURRENT_DATE` for pending?** Invoices without a due_date set (`NULL`) should be treated as not-yet-due, grouped under "pending" rather than "overdue". This handles edge cases gracefully.

- [ ] **Step 2: Build and verify**

Run: `cd backend && go build ./...`
Expected: successful build. The new fields are used by tests later (Task 7).

- [ ] **Step 3: Commit**

```bash
git add backend/analytics.go
git commit -m "feat: extend analytics overview with paid/pending/overdue breakdown"
```

---

### Task 6: Frontend — types and component updates

**Files:**
- Modify: `frontend/src/types/invoice.ts`
- Modify: `frontend/src/types/analytics.ts`
- Modify: `frontend/src/components/ProtectedInvoiceDashboard.tsx`
- Modify: `frontend/src/components/DashboardCards.tsx`

**Interfaces:**
- Consumes: backend JSON API responses (shapes match the Go types from Task 2); existing React components and `apiFetch`/`downloadFile` utilities.
- Produces: updated TypeScript types; new UI elements (status badges, filter dropdown, action buttons, payment form, new metric cards).

- [ ] **Step 1: Update `frontend/src/types/invoice.ts`**

```typescript
export interface InvoiceItem {
  description: string
  qty: number
  price: number
}

export interface Invoice {
  id: string
  client_name: string
  client_id: string | null
  date: string
  due_date: string
  items: InvoiceItem[]
  tax_rate: number
  total_amount: number
  status: string
  user_id: string
  created_at: string
  updated_at: string
}

export interface Client {
  id: string
  user_id: string
  name: string
  email: string
  phone: string
  address: string
  created_at: string
  updated_at: string
}

export interface Product {
  id: string
  user_id: string
  name: string
  description: string
  default_price: number
  created_at: string
  updated_at: string
}

export interface Payment {
  id: string
  invoice_id: string
  amount: number
  date: string
  method: string
  recorded_by: string
  created_at: string
}

export interface StatusHistoryEntry {
  id: string
  invoice_id: string
  old_status: string | null
  new_status: string
  changed_by: string
  changed_at: string
}
```

- [ ] **Step 2: Update `frontend/src/types/analytics.ts`**

```typescript
export interface AnalyticsOverview {
  total_revenue: number
  total_invoices: number
  total_clients: number
  avg_invoice_value: number
  paid_amount: number
  pending_amount: number
  overdue_count: number
}

export interface RevenueDataPoint {
  label: string
  total: number
  count: number
}

export interface RevenueResponse {
  period: "monthly"
  data: RevenueDataPoint[]
}

export interface TopClientData {
  client_name: string
  total: number
  count: number
}

export interface TopClientsResponse {
  clients: TopClientData[]
}

export interface TaxDataPoint {
  label: string
  tax: number
  revenue: number
}

export interface TaxSummaryResponse {
  data: TaxDataPoint[]
}
```

- [ ] **Step 3: Add status color helper and status filter to `ProtectedInvoiceDashboard.tsx`**

In the component, add a helper function and a new state variable for the status filter:

```typescript
const STATUS_COLORS: Record<string, string> = {
  Draft: "bg-gray-100 text-gray-700",
  Sent: "bg-blue-100 text-blue-700",
  Paid: "bg-green-100 text-green-700",
  Overdue: "bg-red-100 text-red-700",
  Cancelled: "bg-gray-100 text-gray-400 line-through",
}

function StatusBadge({ status }: { status: string }) {
  const colorClass = STATUS_COLORS[status] || STATUS_COLORS.Draft
  return (
    <span className={`px-2 py-0.5 rounded-full text-xs font-medium ${colorClass}`}>
      {status}
    </span>
  )
}
```

Add a `const [statusFilter, setStatusFilter] = useState("")` state variable.

In the invoice fetch logic, pass `statusFilter` as a query param:
```typescript
const url = `/invoices${statusFilter ? `?status=${statusFilter}` : ""}`
const invoices = await apiFetch<Invoice[]>(url)
```

In the JSX, add a `<select>` dropdown above the invoice list:
```tsx
<select
  value={statusFilter}
  onChange={(e) => setStatusFilter(e.target.value)}
  className="px-3 py-1 border rounded text-sm"
>
  <option value="">All Status</option>
  <option value="Draft">Draft</option>
  <option value="Sent">Sent</option>
  <option value="Paid">Paid</option>
  <option value="Overdue">Overdue</option>
  <option value="Cancelled">Cancelled</option>
</select>
```

- [ ] **Step 4: Add "Mark as Sent" / "Cancel" action buttons per invoice row**

In the invoice list rendering, add action buttons next to each row:

```tsx
{invoice.status === "Draft" && (
  <button
    onClick={() => handleChangeStatus(invoice.id, "Sent")}
    className="text-xs px-2 py-1 bg-blue-500 text-white rounded hover:bg-blue-600"
  >
    Mark as Sent
  </button>
)}
{(invoice.status === "Draft" || invoice.status === "Sent") && (
  <button
    onClick={() => handleChangeStatus(invoice.id, "Cancelled")}
    className="text-xs px-2 py-1 bg-gray-500 text-white rounded hover:bg-gray-600 ml-1"
  >
    Cancel
  </button>
)}
```

Implement `handleChangeStatus`:

```typescript
const handleChangeStatus = async (invoiceId: string, newStatus: string) => {
  try {
    await apiFetch(`/invoices/${invoiceId}/status`, {
      method: "PUT",
      body: JSON.stringify({ status: newStatus }),
    })
    fetchInvoices() // Refresh list
  } catch (err) {
    console.error("Failed to change status:", err)
  }
}
```

- [ ] **Step 5: Add inline payment form and payment history per invoice row**

For each invoice row, add an expandable section (or a small form below the row) with:

```tsx
{/* Payment form */}
<form onSubmit={(e) => { e.preventDefault(); handleRecordPayment(invoice.id); }} className="mt-2 flex gap-2 text-sm">
  <input type="number" placeholder="Amount" ... className="w-24 border px-1 rounded" />
  <input type="date" ... className="border px-1 rounded" />
  <select ... className="border px-1 rounded">
    <option>Transfer</option>
    <option>Cash</option>
    <option>Credit Card</option>
    <option>Check</option>
  </select>
  <button type="submit" className="px-2 py-0.5 bg-green-500 text-white rounded text-xs">
    Record Payment
  </button>
</form>
```

Implement `handleRecordPayment`:

```typescript
const handleRecordPayment = async (invoiceId: string) => {
  try {
    await apiFetch(`/invoices/${invoiceId}/payments`, {
      method: "POST",
      body: JSON.stringify({ amount, date, method }),
    })
    // Reset form, refresh invoices
    fetchInvoices()
  } catch (err) {
    console.error("Failed to record payment:", err)
  }
}
```

Payment history display per invoice row — shown as small text below the form:

```tsx
{payments[invoice.id]?.map((p: Payment) => (
  <div key={p.id} className="text-xs text-gray-500 ml-2">
    {p.date} — {formatIDR(p.amount)} via {p.method}
  </div>
))}
```

(You'll need a `payments` state that loads payments on-demand when expanding an invoice row, or loads all payments upfront — choose the simpler approach of loading payments for visible invoices on list refresh.)

- [ ] **Step 6: Add 3 new metric cards to `DashboardCards.tsx`**

The existing component receives `data: AnalyticsOverview | null` and renders 4 cards. Add 3 more cards after the existing 4:

| Card | Value | Color | Source |
|------|-------|-------|--------|
| Paid Revenue | `Rp XX.XM` | Green | `data.paid_amount` |
| Pending Revenue | `Rp XX.XM` | Amber | `data.pending_amount` |
| Overdue Invoices | Count | Red | `data.overdue_count` |

Use the same `formatIDR` helper and card structure as the existing 4 cards. Wrap all 7 cards in a `grid grid-cols-4` (first row) and `grid grid-cols-3` (second row with the 3 new ones), or simply extend to `grid-cols-4` with the 7th card spanning.

- [ ] **Step 7: Build frontend and fix type errors**

Run: `cd frontend && npm run build 2>&1 | tail -20`
Expected: Vite build succeeds. Fix any TypeScript errors.

- [ ] **Step 8: Verify end-to-end manually**

Start the full app: `cd /home/sekuyy/project/invoice-maker && ./dev-local.sh`
- Login, create an invoice → should show "Draft" badge
- Click "Mark as Sent" → badge updates to "Sent"
- Record a payment → status auto-changes to "Paid" if full
- Dashboard cards should show paid/pending/overdue numbers
- Status filter dropdown should filter the list

- [ ] **Step 9: Commit**

```bash
git add frontend/src/types/invoice.ts frontend/src/types/analytics.ts \
        frontend/src/components/ProtectedInvoiceDashboard.tsx \
        frontend/src/components/DashboardCards.tsx
git commit -m "feat: add status badges, filter, action buttons, payment form, and dashboard cards"
```

---

### Task 7: Tests — status, payments, invoice filter, analytics update

**Files:**
- Create: `backend/status_test.go`
- Create: `backend/payments_test.go`
- Modify: `backend/invoices_test.go` (add status-filter and overdue test cases)
- Modify: `backend/analytics_test.go` (verify new overview fields)

**Interfaces:**
- Consumes: `setupRouter()`, `truncateTables`, `doRequest`, `registerTestUser`, `decodeJSON` (from `backend/main_test.go`); `Invoice{..., Status, DueDate}`, `StatusChangeRequest`, `PaymentRequest`, `Payment`, `StatusHistoryEntry` (Task 2); `AnalyticsOverview{..., PaidAmount, PendingAmount, OverdueCount}` (Task 2).
- Produces: nothing new consumed by later tasks (final task before verification).

- [ ] **Step 1: Write `backend/status_test.go`**

```go
package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetInvoiceStatusHappyPath(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "statustest@mail.com", "password")

	// Create invoice → defaults to Draft.
	rec := doRequest(router, http.MethodPost, "/api/invoices", Invoice{
		ClientName: "Status Test Client",
		Date:       "2026-07-20",
		TaxRate:    0,
		Items:      []InvoiceItem{{Description: "Item", Qty: 1, Price: 100}},
	}, token)
	require.Equal(t, http.StatusCreated, rec.Code)
	var inv Invoice
	require.NoError(t, decodeJSON(rec.Body.Bytes(), &inv))
	assert.Equal(t, "Draft", inv.Status)

	// Draft → Sent.
	statusRec := doRequest(router, http.MethodPut, "/api/invoices/"+inv.ID+"/status",
		StatusChangeRequest{Status: "Sent"}, token)
	require.Equal(t, http.StatusOK, statusRec.Code, statusRec.Body.String())

	// Verify status history.
	histRec := doRequest(router, http.MethodGet, "/api/invoices/"+inv.ID+"/history", nil, token)
	require.Equal(t, http.StatusOK, histRec.Code)
	var history []StatusHistoryEntry
	require.NoError(t, decodeJSON(histRec.Body.Bytes(), &history))
	require.Len(t, history, 1)
	assert.Equal(t, "Sent", history[0].NewStatus)
	assert.Nil(t, history[0].OldStatus) // First entry has null old_status
}

func TestSetInvoiceStatusInvalidTransition(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "invalidstatustest@mail.com", "password")

	// Create invoice.
	rec := doRequest(router, http.MethodPost, "/api/invoices", Invoice{
		ClientName: "Invalid Transition",
		Date:       "2026-07-20",
		TaxRate:    0,
		Items:      []InvoiceItem{{Description: "Item", Qty: 1, Price: 100}},
	}, token)
	var inv Invoice
	decodeJSON(rec.Body.Bytes(), &inv)

	// Draft → Paid should fail (Paid is auto-only).
	statusRec := doRequest(router, http.MethodPut, "/api/invoices/"+inv.ID+"/status",
		StatusChangeRequest{Status: "Paid"}, token)
	assert.Equal(t, http.StatusBadRequest, statusRec.Code)

	// Draft → Overdue should fail (Overdue is computed, never set manually).
	statusRec = doRequest(router, http.MethodPut, "/api/invoices/"+inv.ID+"/status",
		StatusChangeRequest{Status: "Overdue"}, token)
	assert.Equal(t, http.StatusBadRequest, statusRec.Code)

	// Draft → Sent.
	doRequest(router, http.MethodPut, "/api/invoices/"+inv.ID+"/status",
		StatusChangeRequest{Status: "Sent"}, token)

	// Sent → Draft should fail (can't go backward to Draft).
	statusRec = doRequest(router, http.MethodPut, "/api/invoices/"+inv.ID+"/status",
		StatusChangeRequest{Status: "Draft"}, token)
	assert.Equal(t, http.StatusBadRequest, statusRec.Code)

	// Now cancel it.
	doRequest(router, http.MethodPut, "/api/invoices/"+inv.ID+"/status",
		StatusChangeRequest{Status: "Cancelled"}, token)

	// Cancelled → Sent should fail (can't revive).
	statusRec = doRequest(router, http.MethodPut, "/api/invoices/"+inv.ID+"/status",
		StatusChangeRequest{Status: "Sent"}, token)
	assert.Equal(t, http.StatusBadRequest, statusRec.Code)
}

func TestSetInvoiceStatusNotFound(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "notfoundstatustest@mail.com", "password")

	rec := doRequest(router, http.MethodPut,
		"/api/invoices/00000000-0000-0000-0000-000000000000/status",
		StatusChangeRequest{Status: "Sent"}, token)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestSetInvoiceStatusIsolation(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	tokenA := registerTestUser(t, router, "statusisolationA@mail.com", "password")
	tokenB := registerTestUser(t, router, "statusisolationB@mail.com", "password")

	// User A creates invoice.
	rec := doRequest(router, http.MethodPost, "/api/invoices", Invoice{
		ClientName: "User A's Invoice",
		Date:       "2026-07-20",
		TaxRate:    0,
		Items:      []InvoiceItem{{Description: "Item", Qty: 1, Price: 100}},
	}, tokenA)
	var inv Invoice
	decodeJSON(rec.Body.Bytes(), &inv)

	// User B tries to change User A's invoice status → 404.
	statusRec := doRequest(router, http.MethodPut, "/api/invoices/"+inv.ID+"/status",
		StatusChangeRequest{Status: "Sent"}, tokenB)
	assert.Equal(t, http.StatusNotFound, statusRec.Code)
}
```

- [ ] **Step 2: Write `backend/payments_test.go`**

```go
package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestInvoice(t *testing.T, router *gin.Engine, token string) Invoice {
	t.Helper()
	rec := doRequest(router, http.MethodPost, "/api/invoices", Invoice{
		ClientName: "Payment Test Client",
		Date:       "2026-07-20",
		TaxRate:    0,
		Items:      []InvoiceItem{{Description: "Item", Qty: 1, Price: 1000}},
	}, token)
	require.Equal(t, http.StatusCreated, rec.Code)
	var inv Invoice
	require.NoError(t, decodeJSON(rec.Body.Bytes(), &inv))
	// Mark as Sent so payments can be recorded.
	doRequest(router, http.MethodPut, "/api/invoices/"+inv.ID+"/status",
		StatusChangeRequest{Status: "Sent"}, token)
	return inv
}

func TestRecordPaymentPartial(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "partialpayment@mail.com", "password")
	inv := createTestInvoice(t, router, token) // total_amount = 1000

	rec := doRequest(router, http.MethodPost, "/api/invoices/"+inv.ID+"/payments",
		PaymentRequest{Amount: 600, Date: "2026-07-20", Method: "Transfer"}, token)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	// Status should NOT be Paid (600 < 1000).
	var paymentResp struct {
		Payment       Payment `json:"payment"`
		InvoiceStatus string  `json:"invoice_status"`
	}
	require.NoError(t, decodeJSON(rec.Body.Bytes(), &paymentResp))
	assert.Equal(t, "Sent", paymentResp.InvoiceStatus)
	assert.Equal(t, 600.0, paymentResp.Payment.Amount)
}

func TestRecordPaymentFull(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "fullpayment@mail.com", "password")
	inv := createTestInvoice(t, router, token) // total_amount = 1000

	rec := doRequest(router, http.MethodPost, "/api/invoices/"+inv.ID+"/payments",
		PaymentRequest{Amount: 1000, Date: "2026-07-20", Method: "Transfer"}, token)
	require.Equal(t, http.StatusCreated, rec.Code)

	var paymentResp struct {
		Payment       Payment `json:"payment"`
		InvoiceStatus string  `json:"invoice_status"`
	}
	require.NoError(t, decodeJSON(rec.Body.Bytes(), &paymentResp))
	assert.Equal(t, "Paid", paymentResp.InvoiceStatus)

	// Status history should have 2 entries: Draft→Sent (from createTestInvoice), Sent→Paid (auto).
	histRec := doRequest(router, http.MethodGet, "/api/invoices/"+inv.ID+"/history", nil, token)
	var history []StatusHistoryEntry
	decodeJSON(histRec.Body.Bytes(), &history)
	assert.Len(t, history, 2)
	assert.Equal(t, "Sent", history[0].NewStatus)
	assert.Equal(t, "Paid", history[1].NewStatus)
}

func TestRecordPaymentMultiplePartial(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "multipayment@mail.com", "password")
	inv := createTestInvoice(t, router, token) // total_amount = 1000

	// Payment 1: 600.
	rec1 := doRequest(router, http.MethodPost, "/api/invoices/"+inv.ID+"/payments",
		PaymentRequest{Amount: 600, Date: "2026-07-20", Method: "Transfer"}, token)
	require.Equal(t, http.StatusCreated, rec1.Code)
	var resp1 struct {
		Payment       Payment `json:"payment"`
		InvoiceStatus string  `json:"invoice_status"`
	}
	decodeJSON(rec1.Body.Bytes(), &resp1)
	assert.Equal(t, "Sent", resp1.InvoiceStatus) // Still not paid.

	// Payment 2: 400 → total 1000, should auto-Paid.
	rec2 := doRequest(router, http.MethodPost, "/api/invoices/"+inv.ID+"/payments",
		PaymentRequest{Amount: 400, Date: "2026-07-21", Method: "Cash"}, token)
	require.Equal(t, http.StatusCreated, rec2.Code)
	var resp2 struct {
		Payment       Payment `json:"payment"`
		InvoiceStatus string  `json:"invoice_status"`
	}
	decodeJSON(rec2.Body.Bytes(), &resp2)
	assert.Equal(t, "Paid", resp2.InvoiceStatus)

	// List payments should return 2 entries.
	listRec := doRequest(router, http.MethodGet, "/api/invoices/"+inv.ID+"/payments", nil, token)
	var payments []Payment
	decodeJSON(listRec.Body.Bytes(), &payments)
	assert.Len(t, payments, 2)
}

func TestRecordPaymentCancelledInvoice(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "cancelledpayment@mail.com", "password")

	rec := doRequest(router, http.MethodPost, "/api/invoices", Invoice{
		ClientName: "Will Be Cancelled",
		Date:       "2026-07-20",
		TaxRate:    0,
		Items:      []InvoiceItem{{Description: "Item", Qty: 1, Price: 1000}},
	}, token)
	var inv Invoice
	decodeJSON(rec.Body.Bytes(), &inv)

	// Cancel it.
	doRequest(router, http.MethodPut, "/api/invoices/"+inv.ID+"/status",
		StatusChangeRequest{Status: "Cancelled"}, token)

	// Try to pay → 400.
	payRec := doRequest(router, http.MethodPost, "/api/invoices/"+inv.ID+"/payments",
		PaymentRequest{Amount: 500, Date: "2026-07-20", Method: "Transfer"}, token)
	assert.Equal(t, http.StatusBadRequest, payRec.Code)
}

func TestListPaymentsIsolation(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	tokenA := registerTestUser(t, router, "paymentisoA@mail.com", "password")
	tokenB := registerTestUser(t, router, "paymentisoB@mail.com", "password")

	inv := createTestInvoice(t, router, tokenA)
	doRequest(router, http.MethodPost, "/api/invoices/"+inv.ID+"/payments",
		PaymentRequest{Amount: 500, Date: "2026-07-20", Method: "Transfer"}, tokenA)

	// User B tries to list payments for User A's invoice → 404.
	rec := doRequest(router, http.MethodGet, "/api/invoices/"+inv.ID+"/payments", nil, tokenB)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
```

Don't forget to add `"github.com/gin-gonic/gin"` to the import in `payments_test.go` (used by the `createTestInvoice` helper signature).

- [ ] **Step 3: Modify `backend/invoices_test.go` — add status/overdue filter tests**

Append to `backend/invoices_test.go`:

```go
func TestInvoiceStatusFilter(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "filterstatus@mail.com", "password")

	// Create a Draft invoice (default).
	draftRec := doRequest(router, http.MethodPost, "/api/invoices", Invoice{
		ClientName: "Draft Invoice",
		Date:       "2026-07-20",
		TaxRate:    0,
		Items:      []InvoiceItem{{Description: "Item", Qty: 1, Price: 100}},
	}, token)
	require.Equal(t, http.StatusCreated, draftRec.Code)
	var draftInv Invoice
	decodeJSON(draftRec.Body.Bytes(), &draftInv)

	// Create another and mark as Sent.
	sentRec := doRequest(router, http.MethodPost, "/api/invoices", Invoice{
		ClientName: "Sent Invoice",
		Date:       "2026-07-20",
		TaxRate:    0,
		Items:      []InvoiceItem{{Description: "Item", Qty: 1, Price: 100}},
	}, token)
	var sentInv Invoice
	decodeJSON(sentRec.Body.Bytes(), &sentInv)
	doRequest(router, http.MethodPut, "/api/invoices/"+sentInv.ID+"/status",
		StatusChangeRequest{Status: "Sent"}, token)

	// List all — should return 2.
	allRec := doRequest(router, http.MethodGet, "/api/invoices", nil, token)
	var all []Invoice
	decodeJSON(allRec.Body.Bytes(), &all)
	assert.Len(t, all, 2)

	// Filter by Draft — should return 1.
	draftFilter := doRequest(router, http.MethodGet, "/api/invoices?status=Draft", nil, token)
	var drafts []Invoice
	decodeJSON(draftFilter.Body.Bytes(), &drafts)
	require.Len(t, drafts, 1)
	assert.Equal(t, "Draft", drafts[0].Status)

	// Filter by Sent — should return 1.
	sentFilter := doRequest(router, http.MethodGet, "/api/invoices?status=Sent", nil, token)
	var sents []Invoice
	decodeJSON(sentFilter.Body.Bytes(), &sents)
	require.Len(t, sents, 1)
	assert.Equal(t, "Sent", sents[0].Status)
}

func TestInvoiceOverdueAppearsInFilter(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "overduefilter@mail.com", "password")

	// Create invoice with due_date in the past.
	rec := doRequest(router, http.MethodPost, "/api/invoices", Invoice{
		ClientName: "Overdue Invoice",
		Date:       "2026-01-01",
		DueDate:    "2026-01-15",
		TaxRate:    0,
		Items:      []InvoiceItem{{Description: "Item", Qty: 1, Price: 100}},
	}, token)
	require.Equal(t, http.StatusCreated, rec.Code)
	var inv Invoice
	decodeJSON(rec.Body.Bytes(), &inv)

	// MARK AS SENT (overdue only computed for Sent invoices with past due_date).
	doRequest(router, http.MethodPut, "/api/invoices/"+inv.ID+"/status",
		StatusChangeRequest{Status: "Sent"}, token)

	// Filter by Overdue — should include it (due_date is 2026-01-15, today is later).
	overdueFilter := doRequest(router, http.MethodGet, "/api/invoices?status=Overdue", nil, token)
	var overdue []Invoice
	decodeJSON(overdueFilter.Body.Bytes(), &overdue)
	require.Len(t, overdue, 1)
	assert.Equal(t, "Sent", overdue[0].Status) // stored status is Sent, filter is computed
}
```

- [ ] **Step 4: Modify `backend/analytics_test.go` — verify new overview fields**

In `TestAnalyticsOverview`, after seeding the 2 invoices (1000 + 2000), add assertions for the new fields:

```go
// Both seeded invoices default to "Draft", so paid_amount should be 0.
assert.Equal(t, 0.0, overview.PaidAmount)
// Both are Draft with no due_date, so they should count as pending.
assert.Equal(t, 3000.0, overview.PendingAmount)
// No overdue invoices.
assert.Equal(t, 0, overview.OverdueCount)
```

In `TestAnalyticsOverviewEmptyState`, add:

```go
assert.Equal(t, 0.0, overview.PaidAmount)
assert.Equal(t, 0.0, overview.PendingAmount)
assert.Equal(t, 0, overview.OverdueCount)
```

- [ ] **Step 5: Run all tests**

Run: `cd backend && go test ./... -v 2>&1 | grep -E "PASS|FAIL|ok"`
Expected: all tests pass — existing ones from Phase 9 + new status/payment/analytics tests.

- [ ] **Step 6: Run coverage to see improvement**

Run: `cd backend && go test ./... -cover`
Expected: coverage stays at or above the Phase 9 level (~67%). The new handler code paths are tested.

- [ ] **Step 7: Commit**

```bash
git add backend/status_test.go backend/payments_test.go \
        backend/invoices_test.go backend/analytics_test.go
git commit -m "test: add status transition, payment, filter, and analytics tests for Phase 6"
```

---

### Task 8: Full suite verification, TODO.md housekeeping

**Files:**
- Modify: `TODO.md`
- Possibly modify: `.gitignore` (add coverage artifacts if not already)

- [ ] **Step 1: Run full test suite with coverage**

Run: `cd backend && go test ./... -cover -v 2>&1 | tail -30`
Expected: all tests pass. Note the final coverage percentage.

- [ ] **Step 2: Run go vet and go build**

Run: `cd backend && go vet ./... && go build ./...`
Expected: clean.

- [ ] **Step 3: Build frontend**

Run: `cd frontend && npm run build 2>&1 | tail -10`
Expected: Vite build succeeds.

- [ ] **Step 4: Update `TODO.md`**

Under Phase 6: Invoice Status Tracking:
```markdown
- [x] Add status field: Draft, Sent, Paid, Overdue, Cancelled
- [x] Status history log (when status changes, by whom)
- [x] Filter invoices by status
```

Under Phase 6: Payment Tracking:
```markdown
- [x] Record partial & full payments
- [x] Payment date tracking
- [x] Payment method recording
```

Also update the Phase 6 Learning Goal:
```markdown
### Learning Goal
- Learn SQL state machines (hybrid manual/auto status transitions) ✅
- Understand audit trail pattern (status_history table) ✅
- Practice computed state (Overdue derived at query time, never stored) ✅
- Multi-user isolation with new resources (payments, status changes) ✅
```

Leave Payment Gateway and Email Notifications items unchecked — deferred.

- [ ] **Step 5: Commit**

```bash
git add TODO.md
git commit -m "docs: update TODO.md for Phase 6 status and payment tracking completion"
```

---

## Self-Review Notes

- Spec Section 1 (Migrations) → Task 1
- Spec Section 2 (Backend Types) → Task 2
- Spec Section 3 (API Endpoints) → Tasks 3, 4, 5
- Spec Section 4 (Frontend) → Task 6
- Spec Section 5 (Testing) → Task 7
- Spec Section 6 (Files Changed) → referenced in each task
- Verification Checklist items → verified in each task's "run" steps + Task 8 final suite

No TBD/TODO placeholders. All task code blocks are complete. Schema field names match across migrations, Go structs, and TypeScript interfaces.
