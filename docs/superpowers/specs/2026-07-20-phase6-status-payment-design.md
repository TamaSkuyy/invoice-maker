# Phase 6: Invoice Status & Payment Tracking — Design Spec

**Status**: Approved
**Tanggal**: 2026-07-20
**Scope**: Invoice status lifecycle (Draft/Sent/Paid/Overdue/Cancelled) + status history audit trail + payment recording + analytics dashboard update. Payment gateway (Stripe) dan email notifications di-defer ke sub-phase terpisah.

---

## Decisions Summary

| Decision | Choice | Reason |
|----------|--------|--------|
| Status model | Hybrid (manual + auto-computed) | User sets Draft→Sent/Cancelled manually; backend auto-computes Paid (payments ≥ total) and Overdue (due_date < now); more real-world than fully manual |
| Status field type | `VARCHAR(20)` with app-level enum | PostgreSQL doesn't need native ENUM — VARCHAR is simpler to migrate, and Go app logic validates transitions |
| Due date | `due_date DATE` column on invoices | User flexibility (different clients, different payment terms); not a fixed offset |
| Status history | `status_history` table — full audit trail | Every status change recorded with old_status, new_status, changed_by, changed_at — portfolio-relevant pattern |
| Payment tracking | Separate `payments` table | Normalized; supports partial payments; SUM aggregation determines Paid status |
| Payment method | Free-text `VARCHAR` | Simpler than enum; user types whatever they use (Transfer, Cash, Credit Card, etc.) |
| Overdue computation | Computed on read, not stored | `due_date < CURRENT_DATE AND status NOT IN ('Paid','Cancelled')` — avoids stale Overdue flags |
| Dashboard update | Extend `GET /api/analytics/overview` | Phase 5 explicitly skipped "Paid/Pending metrics" waiting for this; add `paid_amount`, `pending_amount`, `overdue_count` to overview response |
| Status on invoice create | Default `Draft` | Sensible default; user promotes to Sent when ready |
| Scope decomposition | Payment Gateway (Stripe) + Email → separate sub-phase | TODO.md Phase 6 has 4 subsystems; Status+Payment is the foundation; gateway/email are external-service integrations deferred to keep this phase focused |

---

## Status Lifecycle

```
 Draft ──(manual)──► Sent ──(auto: payments ≥ total)──► Paid
   │                   │
   │                   ├──(auto: due_date < today)──► Overdue
   │                   │
   └──(manual)─────────┴──► Cancelled
```

**Transition rules enforced by backend:**
1. `PUT /api/invoices/:id/status` — manual transitions only:
   - `Draft → Sent` ✅
   - `Draft → Cancelled` ✅
   - `Sent → Cancelled` ✅
   - `Cancelled → Sent` ❌ (can't revive cancelled)
   - `Cancelled → Draft` ❌
   - `Paid → anything` ❌ (paid is terminal)
   - `→ Paid` ❌ (Paid is auto-only, not settable via manual endpoint)
2. `POST /api/invoices/:id/payments` — when a payment is recorded:
   - After insert, check `SUM(payments.amount) >= invoices.total_amount`
   - If yes: auto-transition `status → Paid`, write status_history
   - If no: status unchanged
3. Overdue is **never stored** — it's computed at query time:
   ```sql
   SELECT ...,
     CASE WHEN status NOT IN ('Paid','Cancelled') AND due_date < CURRENT_DATE 
          THEN 'Overdue' ELSE status END AS effective_status
   FROM invoices
   ```

---

## Database Migrations

### Migration `000008`: Add status + due_date to invoices

**Up:**
```sql
ALTER TABLE invoices ADD COLUMN status VARCHAR(20) NOT NULL DEFAULT 'Draft';
ALTER TABLE invoices ADD COLUMN due_date DATE;
CREATE INDEX idx_invoices_status ON invoices(status);
CREATE INDEX idx_invoices_due_date ON invoices(due_date);

-- Backfill existing invoices: leave as Draft, set due_date = date + 30 days
UPDATE invoices SET due_date = date + INTERVAL '30 days' WHERE due_date IS NULL;
```

**Down:**
```sql
DROP INDEX IF EXISTS idx_invoices_due_date;
DROP INDEX IF EXISTS idx_invoices_status;
ALTER TABLE invoices DROP COLUMN due_date;
ALTER TABLE invoices DROP COLUMN status;
```

### Migration `000009`: Create status_history table

**Up:**
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

**`old_status` nullable**: first status entry (Draft on create) has `old_status = NULL`.

**Down:**
```sql
DROP TABLE status_history;
```

### Migration `000010`: Create payments table

**Up:**
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

**Down:**
```sql
DROP TABLE payments;
```

---

## Backend: New Types

### `backend/main.go` — modified Invoice struct

```go
type Invoice struct {
    ID          string        `json:"id"`
    ClientName  string        `json:"client_name"`
    ClientID    *string       `json:"client_id"`
    Date        string        `json:"date"`
    DueDate     string        `json:"due_date"`           // NEW
    Items       []InvoiceItem `json:"items"`
    TaxRate     float64       `json:"tax_rate"`
    TotalAmount float64       `json:"total_amount"`
    Status      string        `json:"status"`              // NEW
    UserID      string        `json:"user_id"`
    CreatedAt   time.Time     `json:"created_at"`
    UpdatedAt   time.Time     `json:"updated_at"`
}
```

### `backend/main.go` — new types

```go
type Payment struct {
    ID         string    `json:"id"`
    InvoiceID  string    `json:"invoice_id"`
    Amount     float64   `json:"amount"`
    Date       string    `json:"date"`
    Method     string    `json:"method"`
    RecordedBy string    `json:"recorded_by"`
    CreatedAt  time.Time `json:"created_at"`
}

type StatusHistoryEntry struct {
    ID        string    `json:"id"`
    InvoiceID string    `json:"invoice_id"`
    OldStatus *string   `json:"old_status"`  // nullable — first entry has null
    NewStatus string    `json:"new_status"`
    ChangedBy string    `json:"changed_by"`
    ChangedAt time.Time `json:"changed_at"`
}

type StatusChangeRequest struct {
    Status string `json:"status" binding:"required"`
}

type PaymentRequest struct {
    Amount float64 `json:"amount" binding:"required,gt=0"`
    Date   string  `json:"date" binding:"required"`
    Method string  `json:"method" binding:"required"`
}
```

### `backend/analytics.go` — extended AnalyticsOverview

```go
type AnalyticsOverview struct {
    TotalRevenue    float64 `json:"total_revenue"`
    TotalInvoices   int     `json:"total_invoices"`
    TotalClients    int     `json:"total_clients"`
    AvgInvoiceValue float64 `json:"avg_invoice_value"`
    PaidAmount      float64 `json:"paid_amount"`       // NEW
    PendingAmount   float64 `json:"pending_amount"`    // NEW — Sent/Draft, not overdue
    OverdueCount    int     `json:"overdue_count"`     // NEW — past due_date, not Paid/Cancelled
}
```

---

## Backend: New & Modified API Endpoints

All new endpoints protected by `authenticate()`. Data scoped to `user_id` from JWT.

### `GET /api/invoices?status=Draft` — Modified: filter by status

Existing list endpoint gains an optional `status` query param. When absent, returns all invoices (existing behavior). When present, filters by `invoices.status = $status` (or `effective_status` for Overdue — see SQL note below).

Overdue is special: it's not a stored value. When `?status=Overdue`, the query uses:
```sql
SELECT ... FROM invoices 
WHERE user_id = $1 
  AND status NOT IN ('Paid','Cancelled') 
  AND due_date < CURRENT_DATE
ORDER BY created_at DESC
```

For all other status values, simple equality filter.

### `PUT /api/invoices/:id/status` — New: set status manually

Request body: `{"status": "Sent"}`

Logic:
1. Check invoice exists and owned by user (existing EXISTS check pattern)
2. Validate transition is allowed (see transition rules above)
3. `UPDATE invoices SET status = $new, updated_at = now() WHERE id = $id`
4. `INSERT INTO status_history (...) VALUES (...)` — one INSERT, not two queries
5. Return updated invoice

Allowed transitions:
- `Draft → Sent`
- `Draft → Cancelled`
- `Sent → Cancelled`

Any other transition → 400 `{"error": "invalid status transition: Draft→Paid"}`.

### `POST /api/invoices/:id/payments` — New: record payment

Request body: `{"amount": 250000, "date": "2026-07-20", "method": "Transfer"}`

Logic:
1. Check invoice exists, owned by user, and status is not Cancelled (can't pay cancelled)
2. Insert payment row
3. Check total payments: `SELECT COALESCE(SUM(amount), 0) FROM payments WHERE invoice_id = $1`
4. If `total_paid >= invoice.total_amount`:
   - `UPDATE invoices SET status = 'Paid', updated_at = now() WHERE id = $1`
   - `INSERT INTO status_history (old_status → 'Paid')`
5. Return the created payment + current invoice status

**Partial payments allowed**: total payments can be less than total_amount — invoice stays as Sent/Overdue until full.

**Overpayment handled**: if `SUM(amount) > total_amount`, status still → Paid (lunas, even if overpaid).

### `GET /api/invoices/:id/payments` — New: list payments

Returns `[]Payment` for the given invoice, ordered by `date DESC`.

Includes ownership check via the invoice's user_id.

### `GET /api/invoices/:id/history` — New: status audit trail

Returns `[]StatusHistoryEntry` for the given invoice, ordered by `changed_at ASC`.

Includes ownership check via the invoice's user_id.

### `GET /api/analytics/overview` — Modified: add status breakdown

The existing SQL is extended to compute 3 additional fields:

```sql
SELECT
    COALESCE(SUM(total_amount), 0) as total_revenue,
    COUNT(*) as total_invoices,
    COUNT(DISTINCT client_id) as total_clients,
    CASE WHEN COUNT(*) > 0 THEN SUM(total_amount) / COUNT(*) ELSE 0 END as avg_value,
    COALESCE(SUM(CASE WHEN status = 'Paid' THEN total_amount ELSE 0 END), 0) as paid_amount,
    COALESCE(SUM(CASE WHEN status IN ('Draft','Sent') AND (due_date IS NULL OR due_date >= CURRENT_DATE) THEN total_amount ELSE 0 END), 0) as pending_amount,
    COUNT(CASE WHEN status NOT IN ('Paid','Cancelled') AND due_date < CURRENT_DATE THEN 1 END) as overdue_count
FROM invoices WHERE user_id = $1;
```

---

## Backend: File Structure

New files (following the pattern established by `analytics.go`):

```
backend/
  main.go               [MODIFIED — Invoice/Payment/StatusHistory types, PaymentRequest/StatusChangeRequest types]
  router.go              [MODIFIED — new route groups, modified GET /api/invoices handler]
  analytics.go           [MODIFIED — extended AnalyticsOverview, updated overview SQL]
  status.go              [NEW — status transition validation, PUT /api/invoices/:id/status handler]
  payments.go            [NEW — Payment type, POST+GET /api/invoices/:id/payments handlers, auto-Paid logic]
  migrations/
    000008_add_status_to_invoices.up.sql   [NEW]
    000008_add_status_to_invoices.down.sql
    000009_create_status_history.up.sql    [NEW]
    000009_create_status_history.down.sql
    000010_create_payments.up.sql          [NEW]
    000010_create_payments.down.sql
```

`status.go` and `payments.go` follow the pattern of `analytics.go` — named function handlers in dedicated files, not inline closures in `main.go`/`router.go`. This continues the code organization evolution that started in Phase 5.

---

## Frontend: Changes

### Modified: `frontend/src/types/invoice.ts`

```typescript
export interface Invoice {
  id: string
  client_name: string
  client_id: string | null
  date: string
  due_date: string               // NEW
  items: InvoiceItem[]
  tax_rate: number
  total_amount: number
  status: string                 // NEW
  user_id: string
  created_at: string
  updated_at: string
}
```

New types:
```typescript
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

### Modified: `frontend/src/types/analytics.ts`

```typescript
export interface AnalyticsOverview {
  total_revenue: number
  total_invoices: number
  total_clients: number
  avg_invoice_value: number
  paid_amount: number           // NEW
  pending_amount: number        // NEW
  overdue_count: number         // NEW
}
```

### Modified: `frontend/src/components/ProtectedInvoiceDashboard.tsx`

1. **Status filter dropdown** above the invoice list — `<select>` with options: All, Draft, Sent, Paid, Overdue, Cancelled. Filters the `GET /api/invoices?status=X` fetch.
2. **Status badge** in each invoice row — colored `span`/`div`:
   - Draft: gray background
   - Sent: blue background
   - Paid: green background
   - Overdue: red background
   - Cancelled: strikethrough text, gray
3. **"Mark as Sent" button** in each Draft invoice row → calls `PUT /api/invoices/:id/status` with `{"status": "Sent"}`
4. **"Cancel" button** in each Draft/Sent invoice row → calls `PUT /api/invoices/:id/status` with `{"status": "Cancelled"}`
5. **Record Payment form** — inline expandable section per invoice: amount input, date input, method dropdown (Transfer, Cash, Credit Card, Check). Calls `POST /api/invoices/:id/payments`. Shows existing payments list below.
6. **Payment history** — below each invoice row, collapsible: shows list of payments (date, amount, method) + status history timeline

### Modified: `frontend/src/components/DashboardCards.tsx`

Add 3 new metric cards to the existing 4, using the new `analytics.overview` fields:
- **Paid Revenue** (green) — `analytics.paid_amount`
- **Pending Revenue** (amber) — `analytics.pending_amount`
- **Overdue Invoices** (red, count) — `analytics.overdue_count`

Layout: existing 4 cards (1 row of 4) → new row of 3 cards below.

---

## Testing Strategy

All new/modified handlers get integration tests following the Phase 9 pattern — same `TestMain`, `truncateTables`, `doRequest`, `registerTestUser` infra.

### `backend/invoices_test.go` — Modified

Add to existing `TestInvoiceLifeCycle`:
- Verify newly created invoice defaults to `status: "Draft"` in the response

Add new tests:
- `TestInvoiceStatusFilter` — create 2 invoices with different statuses, verify `?status=X` filter returns correct subset
- `TestInvoiceOverdueAppearsInList` — create invoice with `due_date` in the past, verify it appears as "Overdue" or under `?status=Overdue` filter

### `backend/status_test.go` — New

- `TestSetInvoiceStatus` — happy path: Draft→Sent, verify response + status_history entry
- `TestSetInvoiceStatusInvalidTransition` — Draft→Paid rejected (400), verify error message
- `TestSetInvoiceStatusCancelledCantRevive` — Cancelled→Sent rejected (400)
- `TestSetInvoiceStatusPaidIsTerminal` — Paid→Draft rejected (400)
- `TestSetInvoiceStatusNotFound` — random UUID → 404
- `TestSetInvoiceStatusIsolation` — user B tries to change user A's invoice → 404

### `backend/payments_test.go` — New

- `TestRecordPaymentPartial` — record payment < total_amount → status stays Sent
- `TestRecordPaymentFull` — record payment = total_amount → status auto→Paid, status_history entry created
- `TestRecordPaymentOverpay` — record payment > total_amount → status auto→Paid (still valid)
- `TestRecordPaymentMultiplePartial` — two partial payments sum to total → status → Paid on second one
- `TestRecordPaymentCancelledInvoice` — try to pay cancelled invoice → 400
- `TestListPayments` — record 2 payments, GET list → both returned in order
- `TestListPaymentsIsolation` — user B can't see payments of user A's invoice

### `backend/analytics_test.go` — Modified

Update `TestAnalyticsOverview` to verify new fields:
- After seeding 1 invoice with `status='Draft'`: `paid_amount=0`, `pending_amount=seeded_value`, `overdue_count=0`
- After seeding 1 invoice with `status='Paid'`: `paid_amount=seeded_value`, `pending_amount` unchanged

---

## Files Changed (Complete)

```
backend/
  main.go               [MODIFIED — Invoice struct (+DueDate, +Status), new types, no new handler closures]
  router.go              [MODIFIED — new route groups for status/payments, status filter on GET invoices]
  analytics.go           [MODIFIED — extended AnalyticsOverview, updated SQL for paid/pending/overdue]
  status.go              [NEW — status transition logic + handler]
  payments.go            [NEW — payment handler + auto-Paid logic]
  migrations/
    000008_add_status_to_invoices.up.sql   [NEW]
    000008_add_status_to_invoices.down.sql
    000009_create_status_history.up.sql    [NEW]
    000009_create_status_history.down.sql
    000010_create_payments.up.sql          [NEW]
    000010_create_payments.down.sql

frontend/src/
  types/
    invoice.ts            [MODIFIED — Invoice +DueDate/+Status, new Payment/StatusHistoryEntry types]
    analytics.ts          [MODIFIED — AnalyticsOverview +paid_amount/+pending_amount/+overdue_count]
  components/
    ProtectedInvoiceDashboard.tsx  [MODIFIED — status filter, status badge, action buttons, payment form]
    DashboardCards.tsx             [MODIFIED — 3 new metric cards]

backend/
  invoices_test.go       [MODIFIED — status filter + overdue test cases]
  status_test.go         [NEW — status transition tests]
  payments_test.go        [NEW — payment + auto-Paid tests]
  analytics_test.go       [MODIFIED — verify new overview fields]

docs/
  superpowers/
    specs/2026-07-20-phase6-status-payment-design.md  [THIS FILE]
```

---

## What We Skip (Deferred to Sub-Phase)

- **Payment Gateway (Stripe)** — requires external API keys, webhook handling, async processing. This is its own learning module.
- **Email Notifications** — requires SMTP/Mailgun setup, email templating. Separate integration.
- **Filter by date range on invoice list** — nice-to-have, not core to status/payment tracking.
- **Partial payment / installment tracking UI** — backend supports partial payments; frontend shows them, but no dedicated installment schedule UI.

---

## Verification Checklist

### Backend
- [ ] `go build ./...` — compile tanpa error
- [ ] `GET /api/invoices?status=Draft` — filter works, returns only Draft invoices
- [ ] `GET /api/invoices?status=Overdue` — computed filter returns overdue invoices
- [ ] `PUT /api/invoices/:id/status` — valid transition succeeds, status_history written
- [ ] `PUT /api/invoices/:id/status` — invalid transition returns 400
- [ ] `POST /api/invoices/:id/payments` — full payment auto-transitions to Paid
- [ ] `POST /api/invoices/:id/payments` — partial payment does NOT change status
- [ ] `GET /api/invoices/:id/payments` — returns payment list
- [ ] `GET /api/invoices/:id/history` — returns status audit trail
- [ ] `GET /api/analytics/overview` — includes paid_amount, pending_amount, overdue_count
- [ ] All endpoints JWT-protected (401 without token)
- [ ] Multi-user isolation (status change, payment recording)
- [ ] `go test ./...` — all existing + new tests pass
- [ ] `go vet ./...` — clean

### Frontend
- [ ] `npm run build` — Vite build sukses
- [ ] `tsc --noEmit` — TypeScript strict lulus
- [ ] Status badge renders with correct color per status
- [ ] Status filter dropdown filters invoice list
- [ ] "Mark as Sent" button works (calls status API, badge updates)
- [ ] Payment form records payment, status auto-updates to Paid if full
- [ ] Payment history display shows recorded payments
- [ ] Dashboard cards show paid_amount, pending_amount, overdue_count
