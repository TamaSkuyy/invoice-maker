# Phase 4: Client Management & Products — Ringkasan Implementasi

**Status**: ✅ SELESAI (Client Mgmt + Products)
**Tanggal**: 23 Mei 2026
**Diimplementasikan Oleh**: Claude Code

---

## Ringkasan

Phase 4 mengubah Invoice Maker dari aplikasi yang mewajibkan user mengetik **nama client** dan **deskripsi item** secara manual setiap kali, menjadi aplikasi dengan **data client tersimpan** dan **katalog produk** yang bisa digunakan kembali.

**Pencapaian Utama**: User kini bisa menyimpan client dan produk, lalu memilihnya dari dropdown saat membuat invoice — mempercepat workflow pembuatan invoice secara signifikan.

**Scope yang diimplementasikan:**
- ✅ Client Management — tabel clients, CRUD API, dropdown selector dgn inline add
- ✅ Invoice Products — tabel products, quick-pick dropdown di setiap item row
- ⏭️ Invoice Templates — skip (complex, low ROI untuk personal tool)
- ⏭️ Tax & Currency — skip (IDR + single tax rate sudah cukup)

---

## Keputusan Arsitektur

### 1. Client Reference yang Fleksibel (Nullable FK)

Kolom `client_id` di tabel invoices adalah **nullable foreign key**.

**Alasan:**
- Backward compatible — invoice lama tanpa `client_id` tetap valid
- User tetap bisa ketik manual nama client tanpa harus simpan ke database dulu
- `ON DELETE SET NULL` — kalau client dihapus, invoice tetap ada (hanya referensi yang hilang)

**Trade-off:** Data client di invoice bisa staleness kalau client di-update setelah invoice dibuat. Untuk production, bisa ditambah audit log atau snapshot client data di invoice.

### 2. Inline Add Pattern (bukan Modal/Page terpisah)

Form tambah client baru muncul **inline** di bawah dropdown, bukan di halaman terpisah.

**Alasan:**
- Tidak mengganggu flow pembuatan invoice
- UX lebih cepat — user tidak perlu pindah halaman
- Konsisten dengan "single-page app" tanpa router

### 3. Product Selector sebagai Dropdown Kecil

Product picker muncul sebagai tombol kecil "+Pick" di samping input deskripsi item.

**Alasan:**
- Tidak mengambil banyak space di row tabel
- Opsional — user tetap bisa ketik manual
- Auto-fill description + price sekaligus (mengganti dua field)

### 4. Manual Input Tetap Tersedia (No Lock-in)

Di kedua selector, manual input tetap tersedia:
- **Client**: input text "Or type client name manually" selalu muncul di bawah dropdown
- **Product**: input deskripsi + price tetap bisa diketik bebas, picker hanya shortcut

Ini memastikan user tidak dipaksa membuat client/product dulu sebelum bisa bikin invoice.

---

## Implementasi Backend

### Perubahan Database

#### Migrasi Baru

```
migrations/000005_create_clients_table.up.sql
migrations/000005_create_clients_table.down.sql
migrations/000006_create_products_table.up.sql
migrations/000006_create_products_table.down.sql
migrations/000007_add_client_id_to_invoices.up.sql
migrations/000007_add_client_id_to_invoices.down.sql
```

#### Skema Tabel Clients

```sql
CREATE TABLE clients (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) DEFAULT '',
    phone VARCHAR(50) DEFAULT '',
    address TEXT DEFAULT '',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_clients_user_id ON clients(user_id);
```

#### Skema Tabel Products

```sql
CREATE TABLE products (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    description TEXT DEFAULT '',
    default_price DECIMAL(10,2) NOT NULL DEFAULT 0,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_products_user_id ON products(user_id);
```

#### Update Tabel Invoices

```sql
ALTER TABLE invoices ADD COLUMN IF NOT EXISTS client_id UUID;
ALTER TABLE invoices ADD CONSTRAINT fk_invoices_client
  FOREIGN KEY (client_id) REFERENCES clients(id) ON DELETE SET NULL;
```

### Tipe Data Baru (backend/main.go)

```go
type Client struct {
    ID        string    `json:"id"`
    UserID    string    `json:"user_id"`
    Name      string    `json:"name"`
    Email     string    `json:"email"`
    Phone     string    `json:"phone"`
    Address   string    `json:"address"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
}

type Product struct {
    ID           string    `json:"id"`
    UserID       string    `json:"user_id"`
    Name         string    `json:"name"`
    Description  string    `json:"description"`
    DefaultPrice float64   `json:"default_price"`
    CreatedAt    time.Time `json:"created_at"`
    UpdatedAt    time.Time `json:"updated_at"`
}
```

### Perubahan Invoice Struct

```go
type Invoice struct {
    // ... existing fields ...
    ClientID    *string       `json:"client_id"`   // NEW — nullable pointer
    // ...
}
```

`*string` (pointer) dipakai karena `client_id` nullable di database. Tanpa pointer, Go akan kirim `"client_id": ""` (string kosong) bukan `"client_id": null`.

### Endpoint API Baru

Semua endpoint di-protect oleh middleware `authenticate()`. Isolasi data per user di-enforce di query level (`WHERE user_id = $1`).

#### Clients (`/api/clients`)

**`GET /api/clients`** — List semua client user saat ini (diurutkan nama)

```bash
curl -X GET http://localhost:8080/api/clients \
  -H "Authorization: Bearer eyJhbGc..."
```

Response: Array Client objects. Kembalikan `[]` (bukan `null`) kalau tidak ada client.

**`POST /api/clients`** — Buat client baru

```bash
curl -X POST http://localhost:8080/api/clients \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer eyJhbGc..." \
  -d '{"name":"Acme Corp","email":"hello@acme.com","phone":"+62-21-555","address":"Jl. Sudirman"}'
```

Response: Client object dengan generated UUID + timestamps (201).

**`PUT /api/clients/:id`** — Update client (ownership check)

```bash
curl -X PUT http://localhost:8080/api/clients/{id} \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer eyJhbGc..." \
  -d '{"name":"Acme Corp Updated","email":"new@acme.com","phone":"+62","address":""}'
```

Response: Updated Client object (200). Kembalikan 404 jika bukan milik user.

**`DELETE /api/clients/:id`** — Hapus client (ownership check)

Response: `{"message": "client deleted"}` (200). Invoice yang mereferensi client ini akan punya `client_id = NULL` (karena `ON DELETE SET NULL`).

#### Products (`/api/products`)

**`GET /api/products`** — List semua produk user saat ini

```bash
curl -X GET http://localhost:8080/api/products \
  -H "Authorization: Bearer eyJhbGc..."
```

**`POST /api/products`** — Buat produk baru

```bash
curl -X POST http://localhost:8080/api/products \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer eyJhbGc..." \
  -d '{"name":"Web Dev","description":"Full-stack development","default_price":100}'
```

**`PUT /api/products/:id`** — Update produk

**`DELETE /api/products/:id`** — Hapus produk

### Perubahan Invoice Endpoint

Semua query invoice (SELECT + INSERT) sekarang include kolom `client_id`. POST invoice menerima field opsional `client_id` — jika dikirim, disimpan; jika tidak, tetap `NULL`.

---

## Implementasi Frontend

### File Baru

#### `frontend/src/components/ClientSelector.tsx`

Komponen dropdown client dengan inline add form.

**Props:**
- `value: string` — client name saat ini
- `onChange: (clientName: string, clientId?: string | null) => void` — callback saat client dipilih/diketik

**Behavior:**
1. Fetch `/api/clients` saat mount
2. Tampilkan `<select>` dropdown berisi semua client
3. Jika user pilih client → `onChange(client.name, client.id)`
4. Opsi "+ Add new client..." di dropdown → buka inline form
5. Inline form: input name (required), email, phone → POST `/api/clients` → auto-select
6. Input text "Or type client name manually" selalu tersedia — user bisa ketik manual
7. Dropdown opsi awal: "-- Select or type below --"

**State management:**
- `clients: Client[]` — daftar client dari API
- `loading: boolean` — loading state saat fetch
- `showAdd: boolean` — toggle inline add form
- `newName, newEmail, newPhone` — form fields
- `saving: boolean` — loading state saat submit

#### `frontend/src/components/ProductSelector.tsx`

Komponen quick-pick dropdown untuk invoice items.

**Props:**
- `onPick: (description: string, price: number) => void` — callback saat produk dipilih

**Behavior:**
1. Fetch `/api/products` saat mount
2. Tampilkan tombol kecil "+Pick" di samping input deskripsi
3. Klik → buka dropdown popover berisi list produk (nama + harga)
4. Pilih produk → `onPick(product.description, product.default_price)` → dropdown tutup
5. Klik di luar dropdown → tutup (via `mousedown` event listener)
6. Tidak muncul jika tidak ada produk (return `null`)

### File Dimodifikasi

#### `frontend/src/types/invoice.ts`

Tambah interface:

```typescript
export interface Client {
  id?: string
  name: string
  email: string
  phone: string
  address: string
}

export interface Product {
  id?: string
  name: string
  description: string
  default_price: number
}
```

`Invoice` interface ditambah field `client_id?: string | null`.

#### `frontend/src/components/InvoiceForm.tsx`

**Sebelum:**
```tsx
// Plain text input
<input value={clientName} onChange={(e) => setClientName(e.target.value)} />
// No product picker
```

**Sesudah:**
```tsx
// ClientSelector component (dropdown + inline add)
<ClientSelector value={clientName} onChange={(name, id) => {
  setClientName(name);
  setClientId(id);
}} />

// ProductSelector di tiap item row
<ProductSelector onPick={(desc, price) => {
  updateItem(idx, "description", desc);
  updateItem(idx, "price", price);
}} />
```

State tambahan: `clientId: string | null | undefined` — dikirim sbg `client_id` di payload invoice.

---

## Workflow Testing

### 1. Testing Backend (curl)

**Create client:**
```bash
curl -X POST http://localhost:8080/api/clients \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"Acme Corp","email":"hello@acme.com","phone":"+62-21-555","address":"Jl. Sudirman"}'
# Response: 201 + Client object
```

**Create product:**
```bash
curl -X POST http://localhost:8080/api/products \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d '{"name":"Web Dev","description":"Full-stack dev","default_price":100}'
# Response: 201 + Product object
```

**Create invoice dengan client_id:**
```bash
CLIENT_ID=$(curl -s ... | jq -r '.[0].id')
curl -X POST http://localhost:8080/api/invoices \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $TOKEN" \
  -d "{\"client_name\":\"Acme Corp\",\"client_id\":\"$CLIENT_ID\",\"date\":\"...\",\"items\":[...],\"tax_rate\":10}"
# Response: 201 + Invoice dengan client_id terisi
```

### 2. Testing Frontend

1. Buka `http://localhost:5173`, login
2. Di invoice form, klik dropdown client → "+ Add new client..."
3. Isi nama, email, phone → klik "Save Client"
4. Client langsung terpilih di dropdown
5. Di item row, klik "+Pick" → pilih produk dari popup
6. Description + price terisi otomatis
7. Isi qty, klik Save Invoice
8. Invoice tersimpan dengan `client_id` reference

---

## Struktur File

```
invoice-maker/
├── backend/
│   ├── main.go                         [MODIFIED — Client/Product types + 8 endpoints + Invoice.ClientID]
│   ├── db.go
│   ├── pdf.go
│   ├── export.go
│   └── migrations/
│       ├── 000001_*.sql
│       ├── 000002_*.sql
│       ├── 000003_*.sql
│       ├── 000004_*.sql
│       ├── 000005_create_clients_table.up.sql      [NEW]
│       ├── 000005_create_clients_table.down.sql    [NEW]
│       ├── 000006_create_products_table.up.sql     [NEW]
│       ├── 000006_create_products_table.down.sql   [NEW]
│       ├── 000007_add_client_id_to_invoices.up.sql [NEW]
│       └── 000007_add_client_id_to_invoices.down.sql [NEW]
├── frontend/
│   ├── src/
│   │   ├── types/
│   │   │   └── invoice.ts              [MODIFIED — Client + Product interfaces]
│   │   └── components/
│   │       ├── InvoiceForm.tsx          [MODIFIED — ClientSelector + ProductSelector]
│   │       ├── ClientSelector.tsx       [NEW]
│   │       ├── ProductSelector.tsx      [NEW]
│   │       ├── InvoicePreview.tsx
│   │       ├── ProtectedInvoiceDashboard.tsx
│   │       ├── LoginPage.tsx
│   │       ├── RegisterPage.tsx
│   │       └── Navbar.tsx
└── docs/
    ├── PHASE2_IMPLEMENTASI_AUTH.md
    ├── PHASE3_IMPLEMENTASI_EXPORT.md
    └── PHASE4_IMPLEMENTASI_CLIENTS_PRODUCTS.md  [FILE INI]
```

---

## Checklist Verifikasi

### Backend

- ✅ `go build ./...` — compile tanpa error
- ✅ POST /api/clients → 201, client tersimpan di DB
- ✅ GET /api/clients → 200, return array client user saat ini
- ✅ PUT /api/clients/:id → 200, update sukses
- ✅ DELETE /api/clients/:id → 200, hapus sukses
- ✅ POST /api/products → 201, produk tersimpan di DB
- ✅ GET /api/products → 200, return array produk user saat ini
- ✅ PUT /api/products/:id → 200, update sukses
- ✅ DELETE /api/products/:id → 200, hapus sukses
- ✅ POST /api/invoices dgn `client_id` → 201, client_id tersimpan
- ✅ POST /api/invoices tanpa `client_id` → 201, client_id = null (backward compatible)
- ✅ GET /api/invoices → invoice include `client_id` field
- ✅ Semua endpoint dilindungi JWT (401 tanpa token)
- ✅ Isolasi data per user (ownership check di PUT/DELETE)

### Frontend

- ✅ `tsc --noEmit` — TypeScript strict lulus
- ✅ `npm run build` — Vite build sukses
- ✅ ClientSelector: dropdown muncul, fetch clients dari API
- ✅ ClientSelector: "+ Add new client" → inline form → POST → auto-select
- ✅ ClientSelector: manual input tetap berfungsi
- ✅ ProductSelector: "+Pick" muncul di tiap item row
- ✅ ProductSelector: klik produk → description + price auto-fill
- ✅ ProductSelector: dropdown tutup saat klik di luar
- ✅ InvoiceForm: reset clientId setelah save

### End-to-End

- ✅ Add client via dropdown → buat invoice → client_id tersimpan
- ✅ Add product via picker → item terisi otomatis
- ✅ Ketik manual client name (tanpa simpan client) → invoice tetap terbuat
- ✅ Delete client → invoice terkait tetap ada (client_id jadi null)

---

## Masalah yang Ditemui & Diperbaiki

### 1. Nullable FK di Go: Pointer vs Zero Value

**Masalah**: Kolom `client_id` nullable di database. Kalau pakai `string` biasa, Go akan kirim `"client_id": ""` saat nil. JSON kosong ini ambigu — apakah memang kosong atau memang null?

**Solusi**: Gunakan `*string` (pointer to string). Nilai `nil` pointer diserialize JSON sbg `null`. Nilai pointer ke string kosong tetap diserialize sbg `""`.

```go
type Invoice struct {
    ClientID *string `json:"client_id"` // nil = null, &"abc" = "abc"
}
```

**Lesson**: Di Go, kolom nullable SQL harus di-map ke pointer type (`*string`, `*int`, `*float64`) atau `sql.NullString`. Pointer lebih sederhana untuk JSON serialization.

### 2. Migration IF NOT EXISTS untuk Kolom

**Masalah**: `ALTER TABLE ADD COLUMN IF NOT EXISTS` tidak support di PostgreSQL versi lama. Tapi di PostgreSQL 9.6+ sudah support.

**Solusi**: Gunakan `ADD COLUMN IF NOT EXISTS` — migration tetap idempotent.

### 3. Reset State Setelah Invoice Save

**Masalah**: Setelah invoice tersimpan, state `clientId` tidak di-reset. Bisa menyebabkan client ID dari invoice sebelumnya terbawa ke invoice baru.

**Solusi**: Tambah `setClientId(null)` di handler setelah save sukses, bersama dengan reset field lain.

---

## Langkah Berikutnya (Phase 5+)

- **Invoice Status** (Phase 6): Draft, Sent, Paid, Overdue
- **Dashboard Analytics** (Phase 5): Total invoiced, revenue chart
- **Invoice Templates**: Multiple PDF templates (professional, minimal)
- **Client detail page**: Melihat riwayat invoice per client

---

## Referensi

### PostgreSQL
- [Foreign Key Constraints](https://www.postgresql.org/docs/current/ddl-constraints.html#DDL-CONSTRAINTS-FK)
- [ALTER TABLE](https://www.postgresql.org/docs/current/sql-altertable.html)

### Go
- [Pointers in Go](https://go.dev/tour/moretypes/1)
- [JSON null handling with pointers](https://pkg.go.dev/encoding/json#Marshal)

### React
- [Lifting State Up](https://react.dev/learn/sharing-state-between-components)
- [useRef + useEffect for click-outside](https://react.dev/reference/react/useRef)

---

**Phase 4 Selesai** ✅
Client Management + Products mempercepat workflow invoice creation dengan dropdown selector dan quick-pick.
