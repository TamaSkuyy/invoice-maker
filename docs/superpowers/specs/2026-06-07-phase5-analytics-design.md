# Phase 5: Reporting & Analytics — Design Spec

**Status**: Approved  
**Tanggal**: 2026-06-07  
**Scope**: Dashboard analytics + financial reports (no invoice status dependency)

---

## Decisions Summary

| Decision | Choice | Reason |
|----------|--------|--------|
| Status-dependent metrics | Skipped | Invoice `status` field is Phase 6; compute only what's available now |
| Dashboard placement | Top section on main page | Single page, scroll down for legacy invoice form |
| Chart library | Recharts | React-native SVG, composable, best fit for 4-5 charts |
| Analytics data | Dedicated `/api/analytics` endpoints | SQL aggregation efficient; small network payload |
| Reports scope | Tax summary + Revenue report only | Client payment history needs Phase 6 status field |

---

## Backend: New API Endpoints

All endpoints protected by `authenticate()` middleware. Data scoped to `user_id` from JWT claims.

### `GET /api/analytics/overview`

Dashboard metric cards.

```json
{
  "total_revenue": 150000000,
  "total_invoices": 42,
  "total_clients": 15,
  "avg_invoice_value": 3571428.57
}
```

SQL:
```sql
SELECT
  COALESCE(SUM(total_amount), 0) as total_revenue,
  COUNT(*) as total_invoices,
  COUNT(DISTINCT client_id) as total_clients,
  COALESCE(AVG(total_amount), 0) as avg_value
FROM invoices WHERE user_id = $1;
```

### `GET /api/analytics/revenue?year=2026`

Monthly revenue aggregation. Returns empty `[]` if no data for that year.

```json
{
  "period": "monthly",
  "data": [
    {"label": "Jan", "total": 12000000, "count": 5}
  ]
}
```

SQL: `GROUP BY EXTRACT(MONTH FROM date), TO_CHAR(date, 'Mon')`

### `GET /api/analytics/top-clients?limit=5`

Top N clients by total invoice value. Includes both clients with `client_id` (via JOIN) and manual `client_name` (no reference).

```json
{
  "clients": [
    {"client_name": "Acme Corp", "total": 50000000, "count": 12}
  ]
}
```

SQL: `LEFT JOIN clients c ON i.client_id = c.id`, `COALESCE(c.name, i.client_name)` as display name.

### `GET /api/analytics/tax-summary?year=2026`

Tax collected per month. Tax amount computed backward from `total_amount / (1 + tax_rate/100)` to handle varying tax rates.

```json
{
  "data": [
    {"label": "Jan", "tax": 1100000, "revenue": 11000000}
  ]
}
```

### `GET /api/analytics/report?format=pdf|excel&year=2026`

Download revenue + tax summary report. Reuses existing PDF/Excel generation patterns from Phase 3.

- **PDF**: Reuse `fpdf` pattern — summary table + monthly breakdown
- **Excel**: Reuse `excelize` pattern — overview sheet + monthly sheet

---

## Frontend: New Components

### `DashboardCards.tsx`

Four stat cards in a responsive grid:

| Card | Value | Source |
|------|-------|--------|
| Total Revenue | `Rp XX.XM` | `overview.total_revenue` |
| Total Invoices | Count | `overview.total_invoices` |
| Total Clients | Count | `overview.total_clients` |
| Avg Invoice | `Rp X.XM` | `overview.avg_invoice_value` |

### `RevenueChart.tsx`

- Recharts `BarChart` — monthly bars
- Props: `data: {label, total, count}[]`, `loading: boolean`
- Year selector dropdown (default: current year)
- Empty state: "No invoices for this year"

### `TopClientsChart.tsx`

- Recharts `PieChart` — top 5 clients by revenue
- Props: `data: {client_name, total, count}[]`, `loading: boolean`
- Empty state: "No client data yet"

### `TaxSummaryCard.tsx`

- Table: Month | Revenue | Tax | Effective Rate
- Download buttons: [Download PDF] [Download Excel]
- Year selector synced with RevenueChart
- Empty state: "No tax data for this year"

### `ProtectedInvoiceDashboard.tsx` (Modified)

Add dashboard section ABOVE existing content:

```
<main>
  <DashboardSection />    // NEW — collapsible? No, always visible
  <div class="grid ...">  // EXISTING — invoice form + preview
  <SavedInvoicesList />   // EXISTING — saved invoices table
</main>
```

Dashboard section fetches analytics on mount and when year filter changes. Uses Promise.all for parallel fetch.

---

## New TypeScript Types

```typescript
// frontend/src/types/analytics.ts

export interface AnalyticsOverview {
  total_revenue: number
  total_invoices: number
  total_clients: number
  avg_invoice_value: number
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

---

## Data Flow

```
Dashboard mount
  → fetch /api/analytics/overview
  → fetch /api/analytics/revenue?year={currentYear}
  → fetch /api/analytics/top-clients?limit=5
  → fetch /api/analytics/tax-summary?year={currentYear}
  → render all charts

Year selector change
  → re-fetch revenue + tax-summary with new year
  → revenue chart + tax table re-render

Download click
  → fetch /api/analytics/report?format=pdf|excel&year={year}
  → download blob via downloadFile() utility
```

---

## Files Changed

```
backend/
  main.go                          [MODIFIED — add analytics handlers]

frontend/src/
  types/
    analytics.ts                   [NEW — analytics type definitions]
  components/
    DashboardCards.tsx             [NEW — 4 metric cards]
    RevenueChart.tsx               [NEW — bar chart, Recharts]
    TopClientsChart.tsx            [NEW — pie chart, Recharts]
    TaxSummaryCard.tsx             [NEW — tax table + download buttons]
    ProtectedInvoiceDashboard.tsx  [MODIFIED — add dashboard section]

docs/superpowers/specs/
    2026-06-07-phase5-analytics-design.md  [THIS FILE]
```

---

## What We Skip (Phase 6+ dependencies)

- Paid/pending/outstanding metrics → needs `status` field (Phase 6)
- Client payment history report → needs `status` field (Phase 6)
- Cash flow / aging reports → needs payment tracking (Phase 6)

---

## Verification Checklist

- [ ] `go build ./...` — compile tanpa error
- [ ] `GET /api/analytics/overview` → 200 dengan data benar
- [ ] `GET /api/analytics/revenue?year=2026` → 200, array bulanan
- [ ] `GET /api/analytics/top-clients?limit=5` → 200, top clients
- [ ] `GET /api/analytics/tax-summary?year=2026` → 200, tax data
- [ ] `GET /api/analytics/report?format=pdf` → 200, file PDF valid
- [ ] `GET /api/analytics/report?format=excel` → 200, file Excel valid
- [ ] Semua endpoint dilindungi JWT (401 tanpa token)
- [ ] Isolasi data per user — hanya return data user sendiri
- [ ] `npm run build` — Vite build sukses
- [ ] `tsc --noEmit` — TypeScript strict lulus
- [ ] Dashboard cards render dengan data dari API
- [ ] Revenue chart render bar bulanan
- [ ] Top clients chart render pie
- [ ] Tax summary table + download buttons berfungsi
- [ ] Year selector ganti data di revenue chart + tax table
- [ ] Empty state tampil saat tidak ada data
