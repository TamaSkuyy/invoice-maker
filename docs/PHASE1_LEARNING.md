# Phase 1: Data Persistence - Learning Summary

## Apa yang Kita Pelajari?

Phase 1 adalah transformasi dari simpan data **di memory saja** menjadi simpan data **di database PostgreSQL** yang persistent (tahan lama).

---

## Problem dengan Approach Lama (In-Memory)

### ❌ Masalah In-Memory Storage

```go
type store struct {
    mu       sync.RWMutex
    invoices map[string]Invoice  // Data hanya di RAM
    counter  int
}
```

**Masalah:**

1. **Data hilang saat restart** - Semua invoice hilang jika server restart
2. **Tidak scalable** - Hanya bisa satu instance server
3. **Tidak production-ready** - Real application butuh persistent storage
4. **Sulit di-backup** - Tidak bisa backup data

**Contoh:**

```
Server restart terjadi
    ↓
Semua data di RAM dihapus
    ↓
Invoice yang ada sebelumnya: HILANG 😱
```

---

## Solusi: PostgreSQL Database

### ✅ Kenapa PostgreSQL?

| Aspek           | In-Memory              | PostgreSQL            |
| --------------- | ---------------------- | --------------------- |
| **Persistent**  | ❌ Hilang saat restart | ✅ Disimpan di disk   |
| **Scalable**    | ❌ Single instance     | ✅ Multiple instances |
| **Reliability** | ❌ Tidak aman          | ✅ ACID guarantee     |
| **Backup**      | ❌ Susah               | ✅ Mudah              |
| **Query**       | ❌ Iterate map         | ✅ SQL queries        |
| **Production**  | ❌ Demo only           | ✅ Enterprise-ready   |

### Konsep ACID (Penting!)

**ACID = Atomic, Consistent, Isolated, Durable**

Database menjamin 4 hal penting:

1. **Atomic (Atomik)**
   - Operasi all-or-nothing
   - Contoh: Create invoice + items semua berhasil atau semua gagal (tidak boleh setengah)

   ```go
   // Contoh transaction
   tx, err := db.Begin(ctx)

   // Insert invoice
   tx.Exec("INSERT INTO invoices...")

   // Insert items
   tx.Exec("INSERT INTO invoice_items...")

   // Commit semua atau Rollback semua
   tx.Commit(ctx)
   ```

2. **Consistent (Konsisten)**
   - Data selalu dalam state valid
   - Foreign key constraints menjaga integritas data

   ```sql
   -- Contoh: invoice_items harus punya invoice_id yang valid
   FOREIGN KEY (invoice_id) REFERENCES invoices(id) ON DELETE CASCADE
   ```

3. **Isolated (Terisolasi)**
   - Concurrent requests tidak saling interfere
   - Setiap transaction berjalan independent

4. **Durable (Tahan Lama)**
   - Data disimpan permanen di disk
   - Tidak hilang meski server crash

---

## Architecture Change: Visualisasi

### BEFORE (In-Memory)

```
┌─────────────────────────────────────────┐
│         Backend Server (Go)              │
│  ┌───────────────────────────────────┐  │
│  │  RAM                              │  │
│  │  ┌─────────────────────────────┐  │  │
│  │  │ invoices map[string]Invoice │  │  │
│  │  │                             │  │  │
│  │  │ INV-20260301-001: {...}     │  │  │
│  │  │ INV-20260301-002: {...}     │  │  │
│  │  │                             │  │  │
│  │  │ ❌ HILANG saat restart!     │  │  │
│  │  └─────────────────────────────┘  │  │
│  └───────────────────────────────────┘  │
└─────────────────────────────────────────┘
```

### AFTER (Database)

```
┌──────────────────────────────┐         ┌────────────────────────────┐
│  Backend Server (Go)         │         │  PostgreSQL Database       │
│  ┌──────────────────────────┐│         │  ┌──────────────────────┐ │
│  │ pgxpool.Pool             ││◄────────►  │ invoices table       │ │
│  │ (Connection Pool)        ││         │  │ ┌────────────────────┐ │
│  │ • min: 5 conn            ││         │  │ │ id (UUID)          │ │
│  │ • max: 25 conn           ││         │  │ │ client_name        │ │
│  │ • idle timeout: 5 min    ││         │  │ │ date (DATE)        │ │
│  └──────────────────────────┘│         │  │ │ total_amount       │ │
│                              │         │  │ │ created_at         │ │
│  SELECT, INSERT, UPDATE      │         │  │ │ updated_at         │ │
│  DELETE (SQL queries)        │         │  │ └────────────────────┘ │
└──────────────────────────────┘         │  ┌──────────────────────┐ │
                                         │  │ invoice_items table  │ │
                                         │  │ ┌────────────────────┐ │
                                         │  │ │ id (UUID)          │ │
                                         │  │ │ invoice_id (FK)    │ │
                                         │  │ │ description        │ │
                                         │  │ │ qty, price         │ │
                                         │  │ └────────────────────┘ │
                                         │  └──────────────────────┘ │
                                         │  📊 Data tersimpan permanen│
                                         └────────────────────────────┘
```

---

## Bagian-Bagian Penting yang Kita Implement

### 1. **Docker Compose Setup**

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_DB: invoicedb
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U invoiceuser"]
```

**Pembelajaran:**

- `postgres_data:/var/lib/postgresql/data` = volume untuk persistent storage
- `healthcheck` = memastikan DB ready sebelum backend koneksi
- `depends_on` dengan `condition: service_healthy` = proper ordering

### 2. **Database Schema (Migrations)**

```sql
-- invoices table
CREATE TABLE invoices (
    id UUID PRIMARY KEY,
    client_name VARCHAR(255) NOT NULL,
    date DATE NOT NULL,
    tax_rate DECIMAL(5, 2) NOT NULL,
    total_amount DECIMAL(10, 2) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- invoice_items table (relational!)
CREATE TABLE invoice_items (
    id UUID PRIMARY KEY,
    invoice_id UUID NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
    description VARCHAR(255) NOT NULL,
    qty DECIMAL(10, 2) NOT NULL,
    price DECIMAL(10, 2) NOT NULL
);
```

**Pembelajaran:**

- **UUID** = Unique identifier (lebih fleksibel dari counter)
- **Foreign Key** = Menjaga relasi antara 2 table (referential integrity)
- **ON DELETE CASCADE** = Hapus invoice → otomatis hapus items-nya
- **DECIMAL** = Tipe untuk uang (lebih akurat dari float)

### 3. **Connection Pool**

```go
// db.go
config.MaxConns = 25        // Max 25 koneksi simultan
config.MinConns = 5         // Minimum 5 koneksi (warm pool)
config.MaxConnIdleTime = 5 * time.Minute  // Close idle conn

db, _ := pgxpool.NewWithConfig(ctx, config)
```

**Pembelajaran:**

- Connection pool = reuse database connections
- Membuat koneksi baru itu **MAHAL** (time consuming)
- Pool menjaga beberapa koneksi ready → lebih cepat
- Idle timeout = tutup koneksi yang tidak dipakai

### 4. **Database Initialization dengan Migration**

```go
// runMigrations() di main.go
m, _ := migrate.New("file://./migrations", connString)
m.Up()  // Apply pending migrations
```

**Pembelajaran:**

- **Migration** = version control untuk database schema
- Run otomatis saat server startup → schema selalu up-to-date
- Bisa rollback dengan migration down files
- 000001*\*.up.sql dan 000001*\*.down.sql

### 5. **Transactions (Kritis!)**

```go
// CREATE operation
tx, _ := db.Begin(ctx)
defer tx.Rollback(ctx)  // Rollback if error

// Insert invoice
tx.Exec("INSERT INTO invoices...")

// Insert items
for _, item := range items {
    tx.Exec("INSERT INTO invoice_items...")
}

// Commit semua atau rollback semua
tx.Commit(ctx)
```

**Pembelajaran:**

- **Transaction** = group of operations jadi satu atomic unit
- Jika salah satu gagal → semua rollback
- Menjaga data consistency
- Contoh: Jangan insert invoice tanpa items (atau sebaliknya)

### 6. **SQL Queries vs In-Memory**

#### BEFORE (In-Memory)

```go
s.mu.RLock()
defer s.mu.RUnlock()
list := make([]Invoice, 0, len(s.invoices))
for _, inv := range s.invoices {
    list = append(list, inv)
}
```

#### AFTER (Database)

```go
rows, _ := db.Query(ctx,
    "SELECT id, client_name, ... FROM invoices ORDER BY created_at DESC")

var invoices []Invoice
for rows.Next() {
    rows.Scan(&inv.ID, &inv.ClientName, ...)
    invoices = append(invoices, inv)
}
```

**Pembelajaran:**

- SQL lebih powerful (sorting, filtering, aggregation)
- Database handle relationships & constraints
- Lebih scalable untuk large datasets

---

## Key Concepts Learned

### 1. **Relational Database**

- Data disimpan di multiple tables
- Tables saling berhubungan via Foreign Keys
- Normalisasi data (tidak redundant)

### 2. **Connection Pooling**

- Reuse database connections
- Reduce overhead
- Better performance

### 3. **Migrations**

- Schema version control
- Repeatable & reversible
- Team collaboration friendly

### 4. **Transactions (ACID)**

- Data integrity
- Atomic operations
- Consistency guarantee

### 5. **Query Performance**

- Proper indexing
- Avoid N+1 queries
- Use parameterized queries (SQL injection prevention)

---

## Testing & Verification

### ✅ Apa yang Kita Test?

#### 1. CREATE Operation

```bash
curl -X POST http://localhost:8080/api/invoices \
  -H "Content-Type: application/json" \
  -d '{"client_name": "Test", "items": [...], ...}'
# Result: 201 Created + Invoice data returned
```

#### 2. READ Operations

```bash
# Get all
curl http://localhost:8080/api/invoices

# Get single
curl http://localhost:8080/api/invoices/{id}
```

#### 3. UPDATE Operation

```bash
curl -X PUT http://localhost:8080/api/invoices/{id} \
  -H "Content-Type: application/json" \
  -d '{...new data...}'
```

#### 4. DELETE Operation

```bash
curl -X DELETE http://localhost:8080/api/invoices/{id}
```

#### 5. **Data Persistence (PALING PENTING!)**

```bash
# Create invoice
curl -X POST ... → returns id: "abc-123"

# Verify exists
curl http://localhost:8080/api/invoices/abc-123 → ✓ Found

# Restart backend
podman stop invoice-backend
podman start invoice-backend

# Verify data STILL EXISTS after restart
curl http://localhost:8080/api/invoices/abc-123 → ✓ Still Found!
```

**INI ADALAH PERBEDAAN NYATA ANTARA IN-MEMORY DAN DATABASE!**

---

## Real-World Implications

### Skenario 1: Production Environment

```
Sebelum (In-Memory):
  Invoice dibuat → disimpan di RAM
  Server crash → SEMUA hilang → Customer complain

Sesudah (Database):
  Invoice dibuat → disimpan di disk
  Server crash → restart → data tetap ada ✓
```

### Skenario 2: Scale Multiple Instances

```
Sebelum (In-Memory):
  instance: a -> {INV1, INV2}
  instance: b -> {INV3, INV4}
  Customer akses instance A, tapi data di instance B -> Tidak ketemu

Sesudah (Database):
  instance: a ──┐
  instance: b ──┼─→ Shared PostgreSQL ← semua see same data
  instance: c ──┘
```

### Skenario 3: Backup & Disaster Recovery

```
Sebelum:
  Database di-hack, data hilang → game over

Sesudah:
  Database di-backup daily → restore from backup ✓
```

---

## Lessons & Best Practices

### ✅ DO

1. **Gunakan database untuk production**

   ```
   Testing/Demo: In-memory bisa
   Production: HARUS database
   ```

2. **Always use transactions untuk operations yang complex**

   ```go
   // Create invoice + items = transaction
   // Jangan insert invoice dulu, item processing di app layer
   ```

3. **Implement proper error handling**

   ```go
   if err != nil {
       log.Printf("database error: %v", err)
       return error_response
   }
   ```

4. **Use connection pooling**

   ```
   Jangan buat koneksi baru every request!
   Pool = reuse existing connections
   ```

5. **Add indexes untuk frequently queried columns**
   ```sql
   CREATE INDEX idx_invoices_client_name ON invoices(client_name);
   ```

### ❌ DON'T

1. **Don't use in-memory for real data**

   ```go
   ❌ map[string]Invoice
   ✅ Database table
   ```

2. **Don't build queries with string concatenation**

   ```go
   ❌ "SELECT * FROM invoices WHERE id = '" + id + "'"
   ✅ "SELECT * FROM invoices WHERE id = $1" (with id as parameter)

   // The second prevents SQL injection!
   ```

3. **Don't ignore transaction failures**

   ```go
   ❌ _ = tx.Commit(ctx)  // Ignoring error!
   ✅ if err := tx.Commit(ctx); err != nil { ... }
   ```

4. **Don't query N times in a loop (N+1 problem)**
   ```go
   ❌ for each invoice { query items }           // N+1 queries
   ✅ query all items once, then assemble       // 1-2 queries
   ```

---

## Connection String Anatomy

```
postgres://username:password@host:port/database?sslmode=disable
│         │                  │    │     │
└─────────┴──────────────────┴────┴─────┘
   scheme  credentials        location   db name + options
```

Untuk kita:

```
postgres://invoiceuser:invoicepassword@postgres:5432/invoicedb?sslmode=disable
                                        ${DB_HOST}  ${DB_PORT}  ${DB_NAME}
```

**Catatan:** `postgres` sebagai namespace bukan localhost, karena Docker Compose network!

---

## Comparison: Old vs New Code

### OLD (In-Memory) - Create Operation

```go
s.mu.Lock()
input.ID = s.nextID()                    // Generate counter-based ID
input.TotalAmount = calculateTotal(...)
s.invoices[input.ID] = input             // Store in map
s.mu.Unlock()
c.JSON(http.StatusCreated, input)
```

### NEW (Database) - Create Operation

```go
input.ID = uuid.New().String()            // Generate UUID
input.TotalAmount = calculateTotal(...)
now := time.Now()
input.CreatedAt = now
input.UpdatedAt = now

ctx, cancel := context.WithTimeout(...)
defer cancel()

tx, _ := db.Begin(ctx)                    // Start transaction
defer tx.Rollback(ctx)

// Insert invoice
tx.Exec(
    "INSERT INTO invoices (id, client_name, ...) VALUES ($1, $2, ...)",
    input.ID, input.ClientName, ...
)

// Insert items
for _, item := range input.Items {
    tx.Exec(
        "INSERT INTO invoice_items (id, invoice_id, ...) VALUES ($1, $2, ...)",
        uuid.New().String(), input.ID, ...
    )
}

tx.Commit(ctx)                            // Atomic commit
c.JSON(http.StatusCreated, input)
```

**Perbedaan:**

- UUID vs counter-based ID
- Transaction (atomic) vs immediate storage
- Multiple queries vs single in-memory assignment
- Persistent vs volatile

---

## What's Next (Phase 2)?

Phase 1 memberikan foundation yang solid dengan:

- ✅ Persistent data (database)
- ✅ ACID guarantees
- ✅ Connection pooling
- ✅ Proper schema with relationships

Phase 2 akan add:

- 👤 User management (who owns the invoice?)
- 🔐 Authentication (JWT tokens)
- 🛡️ Authorization (user only sees their own invoices)
- 🔑 Role-based access control (admin vs user)

---

## Summary / TL;DR

| Aspek                | In-Memory     | PostgreSQL   |
| -------------------- | ------------- | ------------ |
| **Startup Speed**    | Fast ⚡       | Slow 🐢      |
| **Data Persistence** | ❌ Volatile   | ✅ Permanent |
| **Scalability**      | ❌ Limited    | ✅ Unlimited |
| **ACID Guarantee**   | ❌ No         | ✅ Yes       |
| **Use Case**         | Demo, Testing | Production   |
| **Complexity**       | Simple        | More complex |
| **Learning Value**   | Basic         | Enterprise   |

---

**Phase 1 berhasil! Kamu sudah understand:**

- ✅ Persistent vs volatile storage
- ✅ Database basics (PostgreSQL)
- ✅ Schema design & relationships
- ✅ Connection pooling
- ✅ Transactions & ACID
- ✅ Migrations
- ✅ SQL queries

**Selamat! 🎉 Kamu siap untuk Phase 2: Authentication!**
