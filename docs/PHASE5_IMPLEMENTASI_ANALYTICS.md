# Phase 5: Reporting & Analytics — Ringkasan Implementasi

**Status**: ✅ SELESAI (Dashboard + Reports)
**Tanggal**: 9 Juni 2026
**Diimplementasikan Oleh**: Claude Code

---

## Ringkasan

Phase 5 menambahkan dashboard analytics dan financial reports ke Invoice Maker. User kini bisa melihat metrik bisnis (total revenue, invoice count, top clients) dan mendownload laporan keuangan dalam format PDF/Excel — semua tanpa harus keluar dari halaman utama.

**Pencapaian Utama**: Dashboard dengan 4 metric cards, bar chart revenue bulanan, pie chart top clients, tabel tax summary, dan downloadable financial reports.

**Scope yang diimplementasikan:**
- ✅ Dashboard Cards — 4 metrik (total revenue, invoices, clients, avg invoice)
- ✅ Revenue Chart — bar chart bulanan dengan year selector
- ✅ Top Clients Chart — pie chart donut top 5 clients
- ✅ Tax Summary — tabel tax per bulan + download PDF/Excel
- ⏭️ Paid/Pending/Overdue metrics — skip (butuh field `status` dari Phase 6)
- ⏭️ Client Payment History report — skip (butuh Phase 6)

---

## Keputusan Arsitektur

### 1. Dedicated `/api/analytics` Endpoint (bukan compute di frontend)

Data analytics dihitung di database via SQL aggregation (`SUM`, `GROUP BY`, `COUNT`, `date_trunc`), bukan di frontend.

**Alasan:**
- Query aggregation jauh lebih efisien di PostgreSQL dibanding JavaScript
- Network payload kecil — hanya kirim data agregat (beberapa KB), bukan seluruh invoice + items (bisa MB)
- Backend bisa reuse query yang sama untuk dashboard display dan report generation
- Frontend tidak perlu fetch semua invoice + items hanya untuk menghitung total

**Trade-off:** 5 endpoint baru di backend. Tapi masing-masing < 20 baris handler code — pattern yang sama diulang.

### 2. Tax Dihitung Mundur dari `total_amount`

Karena `tax_rate` bisa berbeda per invoice, tax dihitung dengan reverse formula:

```sql
total_amount - total_amount / (1 + tax_rate / 100)
```

**Mengapa tidak simpan `tax_amount` sebagai kolom?** Karena itu duplikasi data yang bisa di-derive. Tapi kalau performa jadi masalah di scale besar, bisa ditambah generated column.

### 3. Year Selector di Parent, Bukan per Component

State `selectedYear` di-lift ke `ProtectedInvoiceDashboard`, bukan di masing-masing component.

**Alasan:**
- Revenue chart dan tax table harus selalu pakai tahun yang sama
- Satu year selector (di RevenueChart) mengontrol dua komponen sekaligus
- Menghindari duplicate state dan out-of-sync bug

### 4. Dashboard di Atas, Bukan Tab Terpisah

Dashboard section ditempatkan di atas invoice form dalam satu halaman yang sama.

**Alasan:**
- User bisa lihat analytics dan tetap bikin invoice tanpa pindah halaman
- Tidak butuh router/navigation baru
- Satu halaman, scroll vertical — simple UX

### 5. File `analytics.go` Terpisah dari `main.go`

Handler analytics di file sendiri, bukan inline di `main.go` (yang sudah 1048 baris).

**Alasan:**
- `main.go` sudah terlalu besar — pattern ini mulai memecah handler ke dedicated files
- Analytics adalah domain terpisah dari invoices/clients/products
- Memudahkan navigasi dan testing ke depannya

---

## Implementasi Backend

### File Baru

```
backend/analytics.go
```

### Tipe Data Analytics

```go
type AnalyticsOverview struct {
    TotalRevenue    float64 `json:"total_revenue"`
    TotalInvoices   int     `json:"total_invoices"`
    TotalClients    int     `json:"total_clients"`
    AvgInvoiceValue float64 `json:"avg_invoice_value"`
}

type RevenueDataPoint struct {
    Label string  `json:"label"`  // "Jan", "Feb", ...
    Total float64 `json:"total"`
    Count int     `json:"count"`
}

type TopClientData struct {
    ClientName string  `json:"client_name"`
    Total      float64 `json:"total"`
    Count      int     `json:"count"`
}

type TaxDataPoint struct {
    Label   string  `json:"label"`
    Tax     float64 `json:"tax"`
    Revenue float64 `json:"revenue"`
}
```

### Endpoint API

Semua endpoint di-protect oleh middleware `authenticate()`. Isolasi data per user di-enforce di query level (`WHERE user_id = $1`).

#### `GET /api/analytics/overview`

Dashboard metric cards. Satu query, 4 nilai.

```bash
curl -X GET http://localhost:8080/api/analytics/overview \
  -H "Authorization: Bearer $TOKEN"
```

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
  COALESCE(SUM(total_amount), 0),
  COUNT(*),
  COUNT(DISTINCT client_id),
  CASE WHEN COUNT(*) > 0 THEN SUM(total_amount) / COUNT(*) ELSE 0 END
FROM invoices WHERE user_id = $1;
```

**Note:** `COUNT(DISTINCT client_id)` menghitung client unik dari invoice yang punya `client_id` (nullable). Client dengan `client_id = NULL` (input manual tanpa simpan client) tidak dihitung.

#### `GET /api/analytics/revenue?year=2026`

Revenue per bulan untuk satu tahun.

```bash
curl -X GET "http://localhost:8080/api/analytics/revenue?year=2026" \
  -H "Authorization: Bearer $TOKEN"
```

```json
{
  "period": "monthly",
  "data": [
    {"label": "Jan", "total": 12000000, "count": 5},
    {"label": "Feb", "total": 8500000, "count": 3}
  ]
}
```

SQL:
```sql
SELECT
  TO_CHAR(date, 'Mon') as label,
  EXTRACT(MONTH FROM date) as month,
  COALESCE(SUM(total_amount), 0) as total,
  COUNT(*) as count
FROM invoices
WHERE user_id = $1 AND EXTRACT(YEAR FROM date) = $2
GROUP BY month, label
ORDER BY month;
```

**Note:** `TO_CHAR(date, 'Mon')` menghasilkan "Jan", "Feb", dst. `EXTRACT(MONTH FROM date)` dipakai untuk ORDER BY agar urut 1-12 (bukan alphabetical).

#### `GET /api/analytics/top-clients?limit=5`

Top N clients berdasarkan total invoice value.

```bash
curl -X GET "http://localhost:8080/api/analytics/top-clients?limit=5" \
  -H "Authorization: Bearer $TOKEN"
```

```json
{
  "clients": [
    {"client_name": "Acme Corp", "total": 50000000, "count": 12},
    {"client_name": "PT Maju Jaya", "total": 35000000, "count": 8}
  ]
}
```

SQL:
```sql
SELECT
  COALESCE(c.name, i.client_name) as client_name,
  COALESCE(SUM(i.total_amount), 0) as total,
  COUNT(*) as count
FROM invoices i
LEFT JOIN clients c ON i.client_id = c.id
WHERE i.user_id = $1
GROUP BY client_name
ORDER BY total DESC
LIMIT $2;
```

**Pola `COALESCE(c.name, i.client_name)`**: Jika invoice punya `client_id` reference ke client yang masih ada di database → pakai nama client dari tabel `clients`. Jika client sudah dihapus (`ON DELETE SET NULL`) atau invoice dibuat tanpa `client_id` → fallback ke `i.client_name` (nilai manual yang diketik user saat buat invoice).

Ini adalah real-world pattern untuk handle soft-delete dan nullable foreign key di analytics query.

#### `GET /api/analytics/tax-summary?year=2026`

Tax collected per bulan.

```bash
curl -X GET "http://localhost:8080/api/analytics/tax-summary?year=2026" \
  -H "Authorization: Bearer $TOKEN"
```

```json
{
  "data": [
    {"label": "Jan", "tax": 1090909.09, "revenue": 12000000}
  ]
}
```

SQL:
```sql
SELECT
  TO_CHAR(date, 'Mon') as label,
  EXTRACT(MONTH FROM date) as month,
  COALESCE(SUM(total_amount - total_amount / (1 + tax_rate / 100)), 0) as tax,
  COALESCE(SUM(total_amount), 0) as revenue
FROM invoices
WHERE user_id = $1 AND EXTRACT(YEAR FROM date) = $2
GROUP BY month, label
ORDER BY month;
```

**Formula tax**: `total_amount - total_amount / (1 + tax_rate/100)` — menghitung tax mundur dari total inklusif. Contoh: total = 110, tax_rate = 10% → tax = 110 - 110/1.1 = 110 - 100 = 10.

#### `GET /api/analytics/report?format=pdf|excel&year=2026`

Download financial report.

```bash
# PDF
curl -X GET "http://localhost:8080/api/analytics/report?format=pdf&year=2026" \
  -H "Authorization: Bearer $TOKEN" \
  --output financial-report-2026.pdf

# Excel
curl -X GET "http://localhost:8080/api/analytics/report?format=excel&year=2026" \
  -H "Authorization: Bearer $TOKEN" \
  --output financial-report-2026.xlsx
```

Handler ini mengumpulkan overview + revenue + tax data dalam satu handler, lalu mendelegasikan ke:
- `generateAnalyticsPDF(overview, revenue, taxData, year)` — A4 PDF dengan header biru, 4 overview cards, tabel monthly breakdown
- `generateAnalyticsExcel(overview, revenue, taxData, year)` — 3 sheet: Overview, Monthly, Tax Summary

### Perubahan `main.go`

Route group analytics diregistrasi setelah products group, sebelum `r.Run()`:

```go
analytics := r.Group("/api/analytics")
analytics.Use(authenticate())
{
    analytics.GET("/overview", handleAnalyticsOverview)
    analytics.GET("/revenue", handleAnalyticsRevenue)
    analytics.GET("/top-clients", handleAnalyticsTopClients)
    analytics.GET("/tax-summary", handleAnalyticsTaxSummary)
    analytics.GET("/report", handleAnalyticsReport)
}
```

Handler functions di-referensi sebagai named functions (didefinisikan di `analytics.go`), berbeda dengan pattern existing yang inline. Ini adalah evolusi pattern — `main.go` sudah terlalu besar untuk terus menampung inline handler.

---

## Implementasi Frontend

### Library Baru: Recharts

Recharts dipilih karena:
- **React-native SVG rendering** — bukan Canvas wrapper, komposabel dengan React component tree
- **Deklaratif** — `<BarChart>`, `<Bar>`, `<PieChart>`, `<Pie>` langsung JSX
- **Responsive** — `ResponsiveContainer` otomatis scale ke parent width
- **Populer** — 24k+ GitHub stars, dokumentasi baik, banyak contoh

### File Baru

#### `frontend/src/types/analytics.ts`

TypeScript interfaces yang mirror backend Go structs. Semua field pakai `snake_case` (bukan `camelCase`) karena backend Go serialize JSON dengan snake_case.

```typescript
export interface AnalyticsOverview {
  total_revenue: number
  total_invoices: number
  total_clients: number
  avg_invoice_value: number
}
// ... (RevenueDataPoint, TopClientData, TaxDataPoint, dll)
```

**Lesson:** Di full-stack Go + TypeScript, JSON field naming harus konsisten. Go default ke snake_case (`json:"total_revenue"`), TypeScript biasanya camelCase. Pilih salah satu dan patuhi — di project ini pakai snake_case untuk mengikuti Go convention.

#### `frontend/src/components/DashboardCards.tsx`

Komponen 4 metric cards dengan skeleton loading state.

**Props:**
- `data: AnalyticsOverview | null` — data dari API
- `loading: boolean` — tampilkan skeleton saat loading

**State handling:**
1. **Loading** → 4 skeleton cards (animasi pulse)
2. **Null data** → render nothing
3. **Data tersedia** → 4 cards dengan warna berbeda (biru, hijau, ungu, amber)

**Format currency:**
```typescript
function formatIDR(value: number): string {
  if (value >= 1_000_000) return `Rp ${(value / 1_000_000).toFixed(1)}M`;
  if (value >= 1_000) return `Rp ${(value / 1_000).toFixed(0)}K`;
  return `Rp ${value.toFixed(0)}`;
}
```

#### `frontend/src/components/RevenueChart.tsx`

Bar chart revenue bulanan dengan Recharts.

**Props:**
- `data: RevenueDataPoint[]`
- `loading: boolean`
- `year: number`
- `onYearChange: (year: number) => void`

**Features:**
- Year selector dropdown (5 tahun terakhir)
- Bar chart dengan grid, tooltip format IDR
- Y-axis auto-format: 1000 → "1K", 1000000 → "1M"
- Empty state: "No invoices for {year}"

**Recharts pattern yang dipakai:**
```tsx
<ResponsiveContainer width="100%" height={260}>
  <BarChart data={data}>
    <CartesianGrid strokeDasharray="3 3" stroke="#f1f5f9" />
    <XAxis dataKey="label" />
    <YAxis tickFormatter={formatTick} />
    <Tooltip formatter={(value) => [formatIDR(Number(value)), "Revenue"]} />
    <Bar dataKey="total" fill="#2563eb" radius={[4, 4, 0, 0]} />
  </BarChart>
</ResponsiveContainer>
```

**Lesson:** `ResponsiveContainer` HARUS punya parent dengan explicit height (bisa `height={260}` atau CSS). Tanpa itu, chart akan collapse ke height 0.

#### `frontend/src/components/TopClientsChart.tsx`

Pie chart (donut) top 5 clients.

**Features:**
- Donut chart (`innerRadius={45}`, `outerRadius={90}`)
- 5 warna distinct (biru, ungu, hijau, amber, merah)
- Label dengan truncation (nama panjang → "...")
- Legend interaktif
- Empty state: "No client data yet"

**Truncation pattern:**
```typescript
function truncateName(name: string, maxLen: number = 18): string {
  return name.length > maxLen ? name.slice(0, maxLen - 2) + "…" : name;
}
```

#### `frontend/src/components/TaxSummaryCard.tsx`

Tabel monthly tax + download buttons.

**Props:**
- `data: TaxDataPoint[]`
- `loading: boolean`
- `year: number`

**Features:**
- Tabel 3 kolom: Month | Revenue | Tax
- Footer row: total revenue + total tax
- Download buttons: PDF (merah) dan Excel (hijau)
- Download via `downloadFile()` utility (sama seperti export invoice)
- Loading skeleton (3 baris animasi pulse)
- Empty state: "No tax data for {year}"

**Download flow:**
```typescript
const handleDownload = async (format: "pdf" | "excel") => {
  setDownloading(format);
  try {
    await downloadFile(
      `/analytics/report?format=${format}&year=${year}`,
      `financial-report-${year}.${format === "pdf" ? "pdf" : "xlsx"}`
    );
  } finally {
    setDownloading(null);
  }
};
```

Pattern download ini sama dengan yang dipakai di `ProtectedInvoiceDashboard` untuk download invoice PDF/CSV/Excel.

### File Dimodifikasi

#### `frontend/src/components/ProtectedInvoiceDashboard.tsx`

**Perubahan:**
1. Import 4 dashboard components + analytics types
2. Tambah 5 state variables untuk analytics
3. Tambah `fetchAnalytics()` dengan `Promise.all` (4 concurrent requests)
4. Tambah dashboard section di atas invoice form
5. Year state di-lift ke parent, dishare antara RevenueChart dan TaxSummaryCard

**Data fetching pattern — Promise.all:**
```typescript
const fetchAnalytics = useCallback(async (year: number) => {
  setAnalyticsLoading(true);
  try {
    const [ov, revResp, tcResp, taxResp] = await Promise.all([
      apiFetch<AnalyticsOverview>("/analytics/overview"),
      apiFetch<{ data: RevenueDataPoint[] }>(`/analytics/revenue?year=${year}`),
      apiFetch<{ clients: TopClientData[] }>("/analytics/top-clients?limit=5"),
      apiFetch<{ data: TaxDataPoint[] }>(`/analytics/tax-summary?year=${year}`),
    ]);
    setOverview(ov);
    setRevenue(revResp.data || []);
    setTopClients(tcResp.clients || []);
    setTaxSummary(taxResp.data || []);
  } catch (err) {
    console.error("Failed to fetch analytics:", err);
  } finally {
    setAnalyticsLoading(false);
  }
}, []);
```

**Lesson:** `Promise.all` dengan array destructuring adalah pattern ideal untuk parallel API calls. Semua request jalan bersamaan — total latency = latency ter-lambat (bukan jumlah semua latency). Tapi perhatikan: kalau salah satu reject, `Promise.all` langsung reject semua. Di sini kita wrap seluruh block dalam try/catch jadi kalau analytics gagal, dashboard tetap tampil empty state (tidak crash).

**`useCallback` + `useEffect` pattern:**
```typescript
useEffect(() => {
  fetchAnalytics(selectedYear);
}, [fetchAnalytics, selectedYear]);
```

Ini memastikan fetchAnalytics dipanggil saat mount dan setiap kali `selectedYear` berubah. `useCallback` mencegah re-create function setiap render (yang akan trigger infinite loop di useEffect).

---

## Workflow Testing

### 1. Testing Backend (curl)

**Login dulu:**
```bash
TOKEN=$(curl -s -X POST http://localhost:8080/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"test@example.com","password":"test1234"}' \
  | jq -r '.token')
```

**Overview:**
```bash
curl -s http://localhost:8080/api/analytics/overview \
  -H "Authorization: Bearer $TOKEN" | jq .
```

**Revenue per bulan:**
```bash
curl -s "http://localhost:8080/api/analytics/revenue?year=2026" \
  -H "Authorization: Bearer $TOKEN" | jq .
```

**Top clients:**
```bash
curl -s "http://localhost:8080/api/analytics/top-clients?limit=5" \
  -H "Authorization: Bearer $TOKEN" | jq .
```

**Tax summary:**
```bash
curl -s "http://localhost:8080/api/analytics/tax-summary?year=2026" \
  -H "Authorization: Bearer $TOKEN" | jq .
```

**Download report PDF:**
```bash
curl -s "http://localhost:8080/api/analytics/report?format=pdf&year=2026" \
  -H "Authorization: Bearer $TOKEN" \
  --output report.pdf && ls -lh report.pdf
```

**Download report Excel:**
```bash
curl -s "http://localhost:8080/api/analytics/report?format=excel&year=2026" \
  -H "Authorization: Bearer $TOKEN" \
  --output report.xlsx && ls -lh report.xlsx
```

**Test tanpa token (harus 401):**
```bash
curl -s http://localhost:8080/api/analytics/overview | jq .
# Response: {"error":"missing authorization header"}
```

### 2. Testing Frontend

1. Jalankan `./dev-local.sh` (butuh PostgreSQL di Docker/Podman)
2. Buka `http://localhost:5173`, login
3. Dashboard muncul di atas halaman:
   - 4 metric cards (total revenue, invoices, clients, avg)
   - Bar chart revenue + year selector
   - Pie chart top clients
   - Tax summary table
4. Ganti tahun di dropdown → chart + tax table update
5. Klik "PDF" atau "Excel" di tax card → download report
6. Scroll ke bawah → invoice form tetap berfungsi normal
7. Buat invoice baru → data analytics otomatis include invoice baru (next reload/fetch)

### 3. Testing Edge Cases

**User tanpa invoice:**
- Overview: `total_revenue: 0, total_invoices: 0, total_clients: 0, avg_invoice_value: 0`
- Revenue: `[]` (empty array)
- Top clients: `[]` (empty array)
- Tax: `[]` (empty array)
- Dashboard cards: tampil angka 0
- Charts: empty state message

**User dengan banyak invoice (>50 clients):**
- Top clients: `limit` default 5, max 50 (validasi di backend)
- Overview: `COUNT(DISTINCT client_id)` tetap akurat

**Tahun tanpa data:**
- Revenue: `[]`
- Tax: `[]`
- Dashboard tampil empty state, tidak crash

**Client dihapus setelah invoice dibuat:**
- Top clients query pakai `LEFT JOIN` + `COALESCE(c.name, i.client_name)`
- Client yang sudah dihapus tetap muncul dengan nama dari `i.client_name` (data historis)

---

## Struktur File

```
invoice-maker/
├── backend/
│   ├── main.go                         [MODIFIED — +analytics route group]
│   ├── analytics.go                    [NEW — types + 5 handlers + PDF/Excel report gen]
│   ├── pdf.go                          [UNCHANGED — formatMoney() dipakai analytics.go]
│   ├── export.go                       [UNCHANGED — cellName() dipakai analytics.go]
│   └── db.go                           [UNCHANGED]
├── frontend/
│   ├── src/
│   │   ├── types/
│   │   │   ├── invoice.ts              [UNCHANGED]
│   │   │   └── analytics.ts            [NEW — 7 TS interfaces]
│   │   ├── utils/
│   │   │   ├── api.ts                  [UNCHANGED — apiFetch()]
│   │   │   └── export.ts              [UNCHANGED — downloadFile()]
│   │   └── components/
│   │       ├── ProtectedInvoiceDashboard.tsx  [MODIFIED — +dashboard section]
│   │       ├── DashboardCards.tsx             [NEW — 4 metric cards]
│   │       ├── RevenueChart.tsx               [NEW — bar chart]
│   │       ├── TopClientsChart.tsx            [NEW — pie chart]
│   │       ├── TaxSummaryCard.tsx             [NEW — tax table + download]
│   │       ├── InvoiceForm.tsx                [UNCHANGED]
│   │       ├── InvoicePreview.tsx             [UNCHANGED]
│   │       └── ... (ClientSelector, ProductSelector, etc.)
├── docs/
│   ├── PHASE2_IMPLEMENTASI_AUTH.md
│   ├── PHASE3_IMPLEMENTASI_EXPORT.md
│   ├── PHASE4_IMPLEMENTASI_CLIENTS_PRODUCTS.md
│   └── PHASE5_IMPLEMENTASI_ANALYTICS.md       [FILE INI]
└── dev-local.sh
```

---

## Checklist Verifikasi

### Backend
- ✅ `go build ./...` — compile tanpa error
- ✅ `GET /api/analytics/overview` → 200, return 4 metric values
- ✅ `GET /api/analytics/revenue?year=2026` → 200, return array bulanan
- ✅ `GET /api/analytics/top-clients?limit=5` → 200, return top clients
- ✅ `GET /api/analytics/tax-summary?year=2026` → 200, return tax data
- ✅ `GET /api/analytics/report?format=pdf` → 200, file PDF valid
- ✅ `GET /api/analytics/report?format=excel` → 200, file Excel valid
- ✅ Semua endpoint JWT-protected (401 tanpa token)
- ✅ Isolasi data per user — hanya return data user sendiri
- ✅ Empty state: user tanpa invoice return `[]` bukan null

### Frontend
- ✅ `tsc` — TypeScript strict lulus
- ✅ `npm run build` — Vite build sukses (755 modules)
- ✅ DashboardCards: 4 metric cards dengan format IDR
- ✅ DashboardCards: skeleton loading state
- ✅ RevenueChart: bar chart + year selector
- ✅ RevenueChart: empty state "No invoices for {year}"
- ✅ TopClientsChart: pie chart donut top 5
- ✅ TopClientsChart: empty state "No client data yet"
- ✅ TaxSummaryCard: tabel tax + total row
- ✅ TaxSummaryCard: PDF/Excel download button dengan loading state
- ✅ Year selector: ganti tahun → revenue + tax update
- ✅ Dashboard section di atas invoice form, scroll ke bawah tetap bisa bikin invoice

---

## Masalah yang Ditemui & Diperbaiki

### 1. Recharts Tooltip Formatter Type Incompatibility

**Masalah**: TypeScript strict mode complain tentang type `formatter` callback di Recharts `Tooltip` component.

```
error TS2322: Type '(value: number) => string[]' is not assignable to type 'Formatter<ValueType, NameType>'.
  Type 'ValueType | undefined' is not assignable to type 'number'.
```

**Penyebab**: Recharts typing mendefinisikan `value` sebagai `ValueType | undefined` (generic union), bukan `number`. TypeScript strict mode tidak menerima cast implisit.

**Solusi**: Wrap value dengan `Number(value)` untuk explicit type conversion:

```tsx
// Before (error)
<Tooltip formatter={(value: number) => [formatTooltip(value), "Revenue"]} />

// After (ok)
<Tooltip formatter={(value) => [formatTooltip(Number(value)), "Revenue"]} />
```

**Lesson**: Library React dengan typing generic sering punya issue strict null checks. Pattern `Number(value)` adalah safe fallback karena:
- Kalau value number → tetap number
- Kalau value string angka → di-convert ke number
- Kalau value undefined → jadi `NaN` (yang kemudian diformat sebagai "Rp NaN" — masih lebih baik daripada crash)

### 2. Recharts Pie Label Implicit Any

**Masalah**: Callback `label` di Recharts `<Pie>` punya parameter `{ client_name }` yang dikenai `implicit any` error di strict mode.

```tsx
// Error: Binding element 'client_name' implicitly has an 'any' type
label={({ client_name }) => truncateName(client_name)}
```

**Solusi**: Explicit type annotation:

```tsx
label={({ client_name }: { client_name: string }) => truncateName(client_name)}
```

### 3. `dev-local.sh` Command Proxy untuk npm run

**Masalah**: `./dev-local.sh npm run build` gagal karena script parse "run" sebagai argumen invalid.

**Penyebab**: Script `dev-local.sh` hanya support single npm command tanpa subcommand:
```bash
npm|npx)
  CMD="$1"
  shift
  (cd frontend && exec "$CMD" "$@")
```

Ini seharusnya bekerja — `npm` jadi `$CMD`, `run build` jadi `"$@"`. Tapi ada bug di arg parsing loop setelahnya.

**Workaround**: Panggil dari project root:
```bash
(cd /home/sekuyy/project/invoice-maker && bash dev-local.sh npm run build)
```

Atau langsung:
```bash
cd frontend && npm run build
```

**Lesson**: Shell script dengan command proxying harus hati-hati dengan arg parsing order. Pattern yang lebih robust: parse subcommand dulu (`go`, `npm`, `psql`), lalu pass remaining args langsung tanpa re-parsing.

---

## Yang Dipelajari di Phase 5

### SQL Aggregation untuk Analytics

- `SUM()`, `COUNT()`, `AVG()` dengan `GROUP BY` — fondasi analytics query
- `COALESCE()` — handle NULL values di aggregation (user tanpa invoice = 0, bukan NULL)
- `TO_CHAR(date, 'Mon')` — format date ke label bulan tanpa application code
- `EXTRACT(MONTH FROM date)` — ekstrak komponen date untuk ordering
- `LEFT JOIN` + `COALESCE()` — handle nullable foreign key di analytics
- `COUNT(DISTINCT column)` — hitung unique values (termasuk NULL handling)

### Recharts di React + TypeScript

- `ResponsiveContainer` butuh explicit parent height
- `BarChart`, `PieChart` — composable component pattern
- `Tooltip` formatter typing issue + solusi `Number(value)`
- Year selector sebagai controlled input yang trigger re-fetch
- Loading/empty/error state pattern untuk charts

### Full-Stack Analytics Architecture

- Aggregation di database vs di frontend — database selalu lebih efisien
- Dedicated analytics endpoint vs reuse existing list endpoint
- `Promise.all` untuk parallel API calls
- State lifting untuk shared filter (year selector)
- Downloadable report (PDF/Excel) dari data agregat

### Go Code Organization

- Kapan memecah file (main.go 1048 baris → tambah analytics.go)
- Named function references vs inline handlers di route registration
- Reuse helper functions antar file dalam package yang sama (`round2`, `formatMoney`, `cellName`)

---

## Langkah Berikutnya (Phase 6)

Setelah Phase 5 selesai, yang bisa dikerjakan:

- **Phase 6: Invoice Status** — Tambah field `status` (Draft/Sent/Paid/Overdue) ke invoices. Setelah ini, dashboard bisa ditambah widget:
  - Paid vs Pending revenue
  - Outstanding invoices count
  - Client payment history report
  - Overdue invoice notifications

- **Phase 9: Testing** — Saat ini belum ada automated test. Bisa mulai dengan:
  - Unit test untuk analytics SQL queries (dengan test database)
  - Component test untuk dashboard cards/charts
  - E2E test untuk full analytics flow

---

## Referensi

### PostgreSQL
- [Date/Time Functions — TO_CHAR, EXTRACT](https://www.postgresql.org/docs/current/functions-datetime.html)
- [Aggregate Functions — SUM, COUNT, AVG](https://www.postgresql.org/docs/current/functions-aggregate.html)
- [COALESCE](https://www.postgresql.org/docs/current/functions-conditional.html#FUNCTIONS-COALESCE-NVL-IFNULL)

### Recharts
- [Recharts Guide](https://recharts.org/en-US/guide)
- [BarChart API](https://recharts.org/en-US/api/BarChart)
- [PieChart API](https://recharts.org/en-US/api/PieChart)
- [ResponsiveContainer](https://recharts.org/en-US/api/ResponsiveContainer)

### Go
- [database/sql — QueryRow, Query](https://pkg.go.dev/database/sql)
- [gin-gonic/gin — Query Parameters](https://pkg.go.dev/github.com/gin-gonic/gin#Context.DefaultQuery)
- [fpdf — go-pdf/fpdf](https://pkg.go.dev/github.com/go-pdf/fpdf)
- [excelize — xuri/excelize](https://pkg.go.dev/github.com/xuri/excelize/v2)

### React
- [useCallback](https://react.dev/reference/react/useCallback)
- [Promise.all — MDN](https://developer.mozilla.org/en-US/docs/Web/JavaScript/Reference/Global_Objects/Promise/all)
- [Lifting State Up](https://react.dev/learn/sharing-state-between-components)

---

**Phase 5 Selesai** ✅
Dashboard analytics + financial reports siap digunakan. Data bisnis kini terlihat langsung di halaman utama.
