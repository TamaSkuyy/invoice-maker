# Phase 9: Backend Go Testing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.
>
> **Exception for this plan**: the user (learning Go) writes the test code themselves task-by-task; Claude reviews and guides rather than writing it. Do not dispatch a subagent to write code for a task without the user's explicit go-ahead for that specific task.

**Goal:** Add unit + integration tests to the Go backend (table-driven pure-function tests, `testcontainers-go`-backed HTTP integration tests) covering auth, invoices, clients, products, and analytics, so the repo is a credible Go portfolio piece.

**Architecture:** Extract routing from `main()` into a parameterless `setupRouter()` in a new `router.go`, so both `main()` and tests build the identical router. Tests reassign the existing package-level `db *pgxpool.Pool` var to a `testcontainers-go`-managed Postgres instance before calling `setupRouter()`. Pure-logic tests need neither the router nor a database.

**Tech Stack:** Go 1.25, `testing` (stdlib), `github.com/stretchr/testify` (assert/require), `github.com/testcontainers/testcontainers-go` + its `postgres` module, existing `gin`, `pgx/v5`, `golang-migrate`.

## Global Constraints

- Go module: `github.com/TamaSkuyy/invoice-maker/backend`, Go 1.25 (from `go.mod`).
- Test DB requires Docker/Podman running locally (same requirement as `dev-local.sh`).
- No handler is converted from closure to named function as part of this plan — only relocated into `setupRouter()` verbatim.
- Package-level `var db *pgxpool.Pool` (in `db.go`) remains the single source of truth for the DB connection in both prod and test code — do not introduce a second `db` variable or a parameter that shadows it.
- All new test files use `testify`'s `assert`/`require`, not raw `if err != nil { t.Fatal(...) }`.
- Every integration test must call a `truncateTables(t *testing.T)` helper at its start so tests don't leak state into each other.

---

### Task 1: Add testify, write pure unit tests for `round2` and `calculateTotal`

**Files:**
- Create: `backend/logic_test.go`
- Modify: `backend/go.mod`, `backend/go.sum` (via `go get`)

**Interfaces:**
- Consumes: `round2(v float64) float64` and `calculateTotal(items []InvoiceItem, taxRate float64) float64` from `backend/main.go:101-112`; `InvoiceItem{Description string, Qty float64, Price float64}` from `backend/main.go:24-28`.
- Produces: nothing consumed by later tasks (this file is appended to by Tasks 2 and 3).

- [ ] **Step 1: Add testify dependency**

Run: `cd backend && go get github.com/stretchr/testify`

Expected: `go.mod` gains a `require github.com/stretchr/testify vX.Y.Z` line (and `go.sum` updates).

- [ ] **Step 2: Write `backend/logic_test.go` with `TestRound2` and `TestCalculateTotal`**

```go
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRound2(t *testing.T) {
	tests := []struct {
		name string
		in   float64
		want float64
	}{
		{"already 2 decimals", 10.50, 10.50},
		{"needs rounding up", 10.555, 10.56},
		{"needs rounding down", 10.554, 10.55},
		{"negative value", -10.555, -10.56},
		{"integer value", 100, 100},
		{"zero", 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, round2(tt.in))
		})
	}
}

func TestCalculateTotal(t *testing.T) {
	tests := []struct {
		name    string
		items   []InvoiceItem
		taxRate float64
		want    float64
	}{
		{
			name: "single item no tax",
			items: []InvoiceItem{
				{Description: "Web Design", Qty: 1, Price: 500},
			},
			taxRate: 0,
			want:    500,
		},
		{
			name: "single item with tax",
			items: []InvoiceItem{
				{Description: "Web Design", Qty: 1, Price: 500},
			},
			taxRate: 10,
			want:    550,
		},
		{
			name: "multiple items with tax",
			items: []InvoiceItem{
				{Description: "Design", Qty: 2, Price: 250},
				{Description: "Hosting", Qty: 1, Price: 100},
			},
			taxRate: 10,
			want:    660, // (500+100) * 1.10
		},
		{
			name:    "no items",
			items:   []InvoiceItem{},
			taxRate: 10,
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, calculateTotal(tt.items, tt.taxRate))
		})
	}
}
```

- [ ] **Step 3: Run the tests and verify they pass**

Run: `cd backend && go test ./... -run 'TestRound2|TestCalculateTotal' -v`
Expected: `PASS` for both `TestRound2` and `TestCalculateTotal`, all subtests green. These test pre-existing implementation, so a pass on first run is correct (if any subtest fails, it means either the test's expected value or the underlying function has a real bug — investigate before moving on, don't just adjust the expectation to match).

- [ ] **Step 4: Commit**

```bash
git add backend/go.mod backend/go.sum backend/logic_test.go
git commit -m "test: add unit tests for round2 and calculateTotal"
```

---

### Task 2: Unit tests for password hashing

**Files:**
- Modify: `backend/logic_test.go` (append)

**Interfaces:**
- Consumes: `hashPassword(password string) (string, error)` and `verifyPassword(hash, password string) bool` from `backend/main.go:115-124`.
- Produces: nothing new consumed by later tasks.

- [ ] **Step 1: Append `TestHashPassword` and `TestVerifyPassword` to `backend/logic_test.go`**

```go
func TestHashPassword(t *testing.T) {
	hash, err := hashPassword("mysecretpassword")
	require.NoError(t, err)
	assert.NotEmpty(t, hash)
	assert.NotEqual(t, "mysecretpassword", hash)

	// bcrypt salts each hash, so hashing the same password twice
	// must produce two different hashes.
	hash2, err := hashPassword("mysecretpassword")
	require.NoError(t, err)
	assert.NotEqual(t, hash, hash2)
}

func TestVerifyPassword(t *testing.T) {
	hash, err := hashPassword("correct-password")
	require.NoError(t, err)

	assert.True(t, verifyPassword(hash, "correct-password"))
	assert.False(t, verifyPassword(hash, "wrong-password"))
	assert.False(t, verifyPassword(hash, ""))
}
```

Add `"github.com/stretchr/testify/require"` to the existing `import` block in `backend/logic_test.go`.

- [ ] **Step 2: Run the tests and verify they pass**

Run: `cd backend && go test ./... -run 'TestHashPassword|TestVerifyPassword' -v`
Expected: `PASS` for both tests.

- [ ] **Step 3: Commit**

```bash
git add backend/logic_test.go
git commit -m "test: add unit tests for password hashing"
```

---

### Task 3: Unit tests for JWT generation and validation

**Files:**
- Modify: `backend/logic_test.go` (append)

**Interfaces:**
- Consumes: `generateJWT(userID, email string) (string, error)` and `validateJWT(tokenString string) (*CustomClaims, error)` from `backend/main.go:127-174`; `CustomClaims{UserID string, Email string, jwt.RegisteredClaims}` from `backend/main.go:71-75`.
- Produces: nothing new consumed by later tasks.

- [ ] **Step 1: Append `TestGenerateAndValidateJWT` to `backend/logic_test.go`**

```go
func TestGenerateAndValidateJWT(t *testing.T) {
	t.Run("valid token round-trips claims", func(t *testing.T) {
		token, err := generateJWT("user-123", "test@example.com")
		require.NoError(t, err)
		assert.NotEmpty(t, token)

		claims, err := validateJWT(token)
		require.NoError(t, err)
		assert.Equal(t, "user-123", claims.UserID)
		assert.Equal(t, "test@example.com", claims.Email)
	})

	t.Run("malformed token is rejected", func(t *testing.T) {
		_, err := validateJWT("not-a-real-token")
		assert.Error(t, err)
	})

	t.Run("tampered token is rejected", func(t *testing.T) {
		token, err := generateJWT("user-123", "test@example.com")
		require.NoError(t, err)

		tampered := token[:len(token)-2] + "xx"
		_, err = validateJWT(tampered)
		assert.Error(t, err)
	})

	t.Run("expired token is rejected", func(t *testing.T) {
		t.Setenv("JWT_EXPIRATION", "-1") // already expired
		token, err := generateJWT("user-123", "test@example.com")
		require.NoError(t, err)

		_, err = validateJWT(token)
		assert.Error(t, err)
	})
}
```

`t.Setenv` automatically restores the previous env var value at the end of the subtest, so it won't leak into other tests.

- [ ] **Step 2: Run the tests and verify they pass**

Run: `cd backend && go test ./... -run 'TestGenerateAndValidateJWT' -v`
Expected: `PASS`, all 4 subtests green.

- [ ] **Step 3: Run the full `logic_test.go` file together**

Run: `cd backend && go test ./... -run 'TestRound2|TestCalculateTotal|TestHashPassword|TestVerifyPassword|TestGenerateAndValidateJWT' -v`
Expected: all `PASS`, no test-name collisions or shared-state issues.

- [ ] **Step 4: Commit**

```bash
git add backend/logic_test.go
git commit -m "test: add unit tests for JWT generation and validation"
```

---

### Task 4: Extract `setupRouter()` from `main()`

**Files:**
- Create: `backend/router.go`
- Modify: `backend/main.go:210-1059` (remove `runMigrations` body split, remove routing code from `main()`)

**Interfaces:**
- Consumes: package-level `var db *pgxpool.Pool` (`backend/db.go:12`); `authenticate()` (`backend/main.go:177-207`); `handleAnalyticsOverview`, `handleAnalyticsRevenue`, `handleAnalyticsTopClients`, `handleAnalyticsTaxSummary`, `handleAnalyticsReport` (`backend/analytics.go`).
- Produces: `func setupRouter() *gin.Engine` — used by `main()` (Task 4) and by every integration test file (Tasks 5-9) via `setupRouter()` after assigning `db`. `func runMigrationsWithConn(connString string) error` — used by `main()`'s `runMigrations()` and by `TestMain` (Task 5).

- [ ] **Step 1: Split `runMigrations` into `runMigrationsWithConn` + `runMigrations`**

In `backend/main.go`, replace the existing `runMigrations` function (lines 210-235) with:

```go
// runMigrationsWithConn runs database migrations against the given connection string.
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

// runMigrations runs the database migrations using connection settings from env vars.
func runMigrations() error {
	connString := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_NAME"),
	)
	return runMigrationsWithConn(connString)
}
```

- [ ] **Step 2: Create `backend/router.go` and move all routing code into `setupRouter()`**

Create `backend/router.go`:

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

// setupRouter builds the full Gin router. It relies on the package-level
// `db` var (backend/db.go) already being set — by initDB() in main(), or
// directly assigned in tests before calling setupRouter().
func setupRouter() *gin.Engine {
	r := gin.Default()

	// CORS middleware for local frontend development
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// ... [paste verbatim: everything from `auth := r.Group("/api/auth")`
	// at old main.go:264 through the closing `}` of the analytics group
	// at old main.go:1056]

	return r
}
```

Move the entire block from `auth := r.Group("/api/auth")` (old `main.go:264`) through the closing brace of `analytics := r.Group("/api/analytics") { ... }` (old `main.go:1056`) into `setupRouter()`, replacing the `// ...` placeholder above. Do not alter any handler body — copy verbatim.

- [ ] **Step 3: Slim down `main()` in `backend/main.go`**

Replace the body of `main()` with:

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

- [ ] **Step 4: Fix imports**

Run `cd backend && go build ./...` and remove/add imports in `main.go` and `router.go` as the compiler reports (e.g. `main.go` no longer needs `gin`, `net/http`, `uuid` if nothing else in the file uses them; `router.go` needs them instead).

Expected: `go build ./...` succeeds with no errors.

- [ ] **Step 5: Manually verify the server still behaves the same**

Run: `cd backend && go vet ./...`
Expected: no warnings.

Run: `cd /home/sekuyy/project/invoice-maker && ./dev-local.sh` (or `cd backend && go run .` with a Postgres already up per `README.md`), then in another terminal:

```bash
curl -s -X POST http://localhost:8080/api/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"smoketest@example.com","password":"password123"}'
```

Expected: HTTP 201 with a JSON body containing `token` and `user`. Stop the server afterward.

- [ ] **Step 6: Commit**

```bash
git add backend/main.go backend/router.go
git commit -m "refactor: extract setupRouter() from main() for testability"
```

---

### Task 5: Integration test infrastructure (`TestMain` + testcontainers) and auth handler tests

**Files:**
- Create: `backend/main_test.go`
- Create: `backend/auth_test.go`
- Modify: `backend/go.mod`, `backend/go.sum` (via `go get`)

**Interfaces:**
- Consumes: `setupRouter()` and `runMigrationsWithConn(connString string) error` (Task 4); package-level `db` (`backend/db.go`); `SignupRequest{Email, Password}`, `LoginRequest{Email, Password}`, `AuthResponse{Token string, User User}` (`backend/main.go:53-68`).
- Produces: `truncateTables(t *testing.T)` helper — used by every later integration test file (Tasks 6-9). `registerTestUser(t *testing.T, router *gin.Engine, email, password string) string` helper returning a bearer token — used by Tasks 6-9 to authenticate requests.

- [ ] **Step 1: Add testcontainers-go dependencies**

Run:
```bash
cd backend
go get github.com/testcontainers/testcontainers-go
go get github.com/testcontainers/testcontainers-go/modules/postgres
```

Expected: `go.mod`/`go.sum` updated with both modules and their transitive deps.

- [ ] **Step 2: Write `backend/main_test.go` with `TestMain` and shared test helpers**

```go
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()

	container, err := postgres.Run(ctx, "postgres:16-alpine",
		postgres.WithDatabase("invoice_test"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(60*time.Second),
		),
	)
	if err != nil {
		log.Fatalf("failed to start postgres container: %v", err)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("failed to get connection string: %v", err)
	}

	db, err = pgxpool.New(ctx, connStr)
	if err != nil {
		log.Fatalf("failed to connect to test db: %v", err)
	}

	if err := runMigrationsWithConn(connStr); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	code := m.Run()

	db.Close()
	_ = container.Terminate(ctx)
	os.Exit(code)
}

// truncateTables clears all tables between tests so each test starts
// from a known-empty state. Order matters: children before parents,
// or use CASCADE.
func truncateTables(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	_, err := db.Exec(ctx, `TRUNCATE TABLE invoice_items, invoices, clients, products, users CASCADE`)
	require.NoError(t, err)
}

// doRequest performs an HTTP request against the given router and
// returns the recorded response.
func doRequest(router *gin.Engine, method, path string, body any, token string) *httptest.ResponseRecorder {
	var reqBody *bytes.Buffer
	if body != nil {
		b, _ := json.Marshal(body)
		reqBody = bytes.NewBuffer(b)
	} else {
		reqBody = bytes.NewBuffer(nil)
	}

	req := httptest.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// registerTestUser registers a new user via the real HTTP handler and
// returns the auth token, so integration tests exercise the same code
// path a real client would.
func registerTestUser(t *testing.T, router *gin.Engine, email, password string) string {
	t.Helper()
	rec := doRequest(router, http.MethodPost, "/api/auth/register", SignupRequest{
		Email:    email,
		Password: password,
	}, "")
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var resp AuthResponse
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp.Token
}
```

- [ ] **Step 3: Write `backend/auth_test.go`**

```go
package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegister(t *testing.T) {
	truncateTables(t)
	router := setupRouter()

	t.Run("happy path returns 201 with token", func(t *testing.T) {
		rec := doRequest(router, http.MethodPost, "/api/auth/register", SignupRequest{
			Email:    "newuser@example.com",
			Password: "password123",
		}, "")

		require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

		var resp AuthResponse
		require.NoError(t, decodeJSON(rec.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp.Token)
		assert.Equal(t, "newuser@example.com", resp.User.Email)
	})

	t.Run("duplicate email returns 400", func(t *testing.T) {
		registerTestUser(t, router, "dupe@example.com", "password123")

		rec := doRequest(router, http.MethodPost, "/api/auth/register", SignupRequest{
			Email:    "dupe@example.com",
			Password: "anotherpassword",
		}, "")

		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

func TestLogin(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	registerTestUser(t, router, "logintest@example.com", "correctpassword")

	t.Run("happy path returns 200 with token", func(t *testing.T) {
		rec := doRequest(router, http.MethodPost, "/api/auth/login", LoginRequest{
			Email:    "logintest@example.com",
			Password: "correctpassword",
		}, "")

		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		var resp AuthResponse
		require.NoError(t, decodeJSON(rec.Body.Bytes(), &resp))
		assert.NotEmpty(t, resp.Token)
	})

	t.Run("wrong password returns 401", func(t *testing.T) {
		rec := doRequest(router, http.MethodPost, "/api/auth/login", LoginRequest{
			Email:    "logintest@example.com",
			Password: "wrongpassword",
		}, "")

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}

func TestProtectedRouteRequiresAuth(t *testing.T) {
	truncateTables(t)
	router := setupRouter()

	t.Run("no authorization header returns 401", func(t *testing.T) {
		rec := doRequest(router, http.MethodGet, "/api/invoices", nil, "")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})

	t.Run("invalid token returns 401", func(t *testing.T) {
		rec := doRequest(router, http.MethodGet, "/api/invoices", nil, "not-a-valid-token")
		assert.Equal(t, http.StatusUnauthorized, rec.Code)
	})
}
```

Add a small `decodeJSON` helper to `backend/main_test.go` (used above and by every later test file):

```go
func decodeJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}
```

(This is just a readability wrapper — feel free to call `json.Unmarshal` directly instead if you'd rather skip it; keep it consistent across files either way.)

- [ ] **Step 4: Run the integration tests and verify they pass**

Run: `cd backend && go test ./... -run 'TestRegister|TestLogin|TestProtectedRouteRequiresAuth' -v`
Expected: Docker pulls `postgres:16-alpine` on first run (may take a minute), then all subtests `PASS`. If it fails with a Docker connection error, confirm Docker/Podman is running (`docker ps` or `podman ps`).

- [ ] **Step 5: Commit**

```bash
git add backend/go.mod backend/go.sum backend/main_test.go backend/auth_test.go
git commit -m "test: add integration test infrastructure and auth handler tests"
```

---

### Task 6: Invoice handler integration tests

**Files:**
- Create: `backend/invoices_test.go`

**Interfaces:**
- Consumes: `truncateTables`, `doRequest`, `registerTestUser`, `decodeJSON` (Task 5); `Invoice{ID, ClientName, ClientID, Date, Items, TaxRate, TotalAmount, UserID, CreatedAt, UpdatedAt}` (`backend/main.go:31-42`); `InvoiceItem{Description, Qty, Price}` (`backend/main.go:24-28`).
- Produces: nothing new consumed by later tasks.

- [ ] **Step 1: Write `backend/invoices_test.go` covering the full CRUD lifecycle**

```go
package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoiceLifecycle(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "invoiceowner@example.com", "password123")

	// Create
	createBody := Invoice{
		ClientName: "Acme Corp",
		Date:       "2026-01-15",
		TaxRate:    10,
		Items: []InvoiceItem{
			{Description: "Web Design", Qty: 1, Price: 500},
		},
	}
	createRec := doRequest(router, http.MethodPost, "/api/invoices", createBody, token)
	require.Equal(t, http.StatusCreated, createRec.Code, createRec.Body.String())

	var created Invoice
	require.NoError(t, decodeJSON(createRec.Body.Bytes(), &created))
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, 550.0, created.TotalAmount)

	// Get
	getRec := doRequest(router, http.MethodGet, "/api/invoices/"+created.ID, nil, token)
	require.Equal(t, http.StatusOK, getRec.Code, getRec.Body.String())

	var fetched Invoice
	require.NoError(t, decodeJSON(getRec.Body.Bytes(), &fetched))
	assert.Equal(t, "Acme Corp", fetched.ClientName)
	require.Len(t, fetched.Items, 1)
	assert.Equal(t, "Web Design", fetched.Items[0].Description)

	// Update
	updateBody := Invoice{
		ClientName: "Acme Corp Updated",
		Date:       "2026-01-16",
		TaxRate:    10,
		Items: []InvoiceItem{
			{Description: "Web Design", Qty: 1, Price: 600},
		},
	}
	updateRec := doRequest(router, http.MethodPut, "/api/invoices/"+created.ID, updateBody, token)
	require.Equal(t, http.StatusOK, updateRec.Code, updateRec.Body.String())

	var updated Invoice
	require.NoError(t, decodeJSON(updateRec.Body.Bytes(), &updated))
	assert.Equal(t, "Acme Corp Updated", updated.ClientName)
	assert.Equal(t, 660.0, updated.TotalAmount)

	// Delete
	deleteRec := doRequest(router, http.MethodDelete, "/api/invoices/"+created.ID, nil, token)
	assert.Equal(t, http.StatusOK, deleteRec.Code)

	// Get after delete -> 404
	getAfterDeleteRec := doRequest(router, http.MethodGet, "/api/invoices/"+created.ID, nil, token)
	assert.Equal(t, http.StatusNotFound, getAfterDeleteRec.Code)
}

func TestCreateInvoiceInvalidPayload(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "invalidpayload@example.com", "password123")

	// Missing required JSON body entirely (nil body is still valid JSON
	// `null` via doRequest, which ShouldBindJSON rejects as EOF/invalid).
	rec := doRequest(router, http.MethodPost, "/api/invoices", nil, token)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestInvoiceIsolationBetweenUsers(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	tokenA := registerTestUser(t, router, "usera@example.com", "password123")
	tokenB := registerTestUser(t, router, "userb@example.com", "password123")

	createRec := doRequest(router, http.MethodPost, "/api/invoices", Invoice{
		ClientName: "User A's Client",
		Date:       "2026-01-15",
		TaxRate:    0,
		Items:      []InvoiceItem{{Description: "Item", Qty: 1, Price: 100}},
	}, tokenA)
	require.Equal(t, http.StatusCreated, createRec.Code)

	var created Invoice
	require.NoError(t, decodeJSON(createRec.Body.Bytes(), &created))

	// User B tries to fetch User A's invoice directly by ID -> 404, not leaked.
	getRec := doRequest(router, http.MethodGet, "/api/invoices/"+created.ID, nil, tokenB)
	assert.Equal(t, http.StatusNotFound, getRec.Code)

	// User B's invoice list must not contain User A's invoice.
	listRec := doRequest(router, http.MethodGet, "/api/invoices", nil, tokenB)
	require.Equal(t, http.StatusOK, listRec.Code)

	var invoices []Invoice
	require.NoError(t, decodeJSON(listRec.Body.Bytes(), &invoices))
	assert.Empty(t, invoices)
}
```

- [ ] **Step 2: Run the tests and verify they pass**

Run: `cd backend && go test ./... -run 'TestInvoiceLifecycle|TestCreateInvoiceInvalidPayload|TestInvoiceIsolationBetweenUsers' -v`
Expected: all `PASS`.

- [ ] **Step 3: Commit**

```bash
git add backend/invoices_test.go
git commit -m "test: add invoice handler integration tests"
```

---

### Task 7: Client and product handler integration tests

**Files:**
- Create: `backend/clients_test.go`
- Create: `backend/products_test.go`

**Interfaces:**
- Consumes: `truncateTables`, `doRequest`, `registerTestUser`, `decodeJSON` (Task 5); `Client{ID, UserID, Name, Email, Phone, Address, CreatedAt, UpdatedAt}` (`backend/main.go:78-87`); `Product{ID, UserID, Name, Description, DefaultPrice, CreatedAt, UpdatedAt}` (`backend/main.go:90-98`).
- Produces: nothing new consumed by later tasks.

- [ ] **Step 1: Write `backend/clients_test.go`**

```go
package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientCreateAndList(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "clientowner@example.com", "password123")

	createRec := doRequest(router, http.MethodPost, "/api/clients", Client{
		Name:    "Acme Corp",
		Email:   "billing@acme.com",
		Phone:   "555-1234",
		Address: "123 Main St",
	}, token)
	require.Equal(t, http.StatusCreated, createRec.Code, createRec.Body.String())

	var created Client
	require.NoError(t, decodeJSON(createRec.Body.Bytes(), &created))
	assert.NotEmpty(t, created.ID)

	listRec := doRequest(router, http.MethodGet, "/api/clients", nil, token)
	require.Equal(t, http.StatusOK, listRec.Code)

	var clients []Client
	require.NoError(t, decodeJSON(listRec.Body.Bytes(), &clients))
	require.Len(t, clients, 1)
	assert.Equal(t, "Acme Corp", clients[0].Name)
}

func TestClientDeleteNotFound(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "clientowner2@example.com", "password123")

	rec := doRequest(router, http.MethodDelete, "/api/clients/00000000-0000-0000-0000-000000000000", nil, token)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
```

- [ ] **Step 2: Write `backend/products_test.go`**

```go
package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProductCreateAndList(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "productowner@example.com", "password123")

	createRec := doRequest(router, http.MethodPost, "/api/products", Product{
		Name:         "Web Design",
		Description:  "Custom website design",
		DefaultPrice: 500,
	}, token)
	require.Equal(t, http.StatusCreated, createRec.Code, createRec.Body.String())

	var created Product
	require.NoError(t, decodeJSON(createRec.Body.Bytes(), &created))
	assert.NotEmpty(t, created.ID)
	assert.Equal(t, 500.0, created.DefaultPrice)

	listRec := doRequest(router, http.MethodGet, "/api/products", nil, token)
	require.Equal(t, http.StatusOK, listRec.Code)

	var products []Product
	require.NoError(t, decodeJSON(listRec.Body.Bytes(), &products))
	require.Len(t, products, 1)
	assert.Equal(t, "Web Design", products[0].Name)
}

func TestProductDeleteNotFound(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "productowner2@example.com", "password123")

	rec := doRequest(router, http.MethodDelete, "/api/products/00000000-0000-0000-0000-000000000000", nil, token)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
```

- [ ] **Step 3: Run the tests and verify they pass**

Run: `cd backend && go test ./... -run 'TestClient|TestProduct' -v`
Expected: all `PASS`.

- [ ] **Step 4: Commit**

```bash
git add backend/clients_test.go backend/products_test.go
git commit -m "test: add client and product handler integration tests"
```

---

### Task 8: Analytics handler integration tests with seeded data

**Files:**
- Create: `backend/analytics_test.go`

**Interfaces:**
- Consumes: `truncateTables`, `doRequest`, `registerTestUser`, `decodeJSON` (Task 5); `AnalyticsOverview{TotalRevenue float64, TotalInvoices int, TotalClients int, AvgInvoiceValue float64}`, `RevenueDataPoint{Label string, Total float64, Count int}`, `RevenueResponse{Period string, Data []RevenueDataPoint}` (`backend/analytics.go:19-35`); `Invoice`, `InvoiceItem` (`backend/main.go`).
- Produces: nothing new consumed by later tasks.

- [ ] **Step 1: Write `backend/analytics_test.go` with known seed data**

```go
package main

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedInvoice(t *testing.T, router *gin.Engine, token, date string, price float64, taxRate float64) {
	t.Helper()
	rec := doRequest(router, http.MethodPost, "/api/invoices", Invoice{
		ClientName: "Test Client",
		Date:       date,
		TaxRate:    taxRate,
		Items:      []InvoiceItem{{Description: "Item", Qty: 1, Price: price}},
	}, token)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
}

func TestAnalyticsOverview(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "analyticsuser@example.com", "password123")

	seedInvoice(t, router, token, "2026-01-10", 1000, 0) // total 1000
	seedInvoice(t, router, token, "2026-02-10", 2000, 0) // total 2000

	rec := doRequest(router, http.MethodGet, "/api/analytics/overview", nil, token)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var overview AnalyticsOverview
	require.NoError(t, decodeJSON(rec.Body.Bytes(), &overview))
	assert.Equal(t, 3000.0, overview.TotalRevenue)
	assert.Equal(t, 2, overview.TotalInvoices)
	assert.Equal(t, 1500.0, overview.AvgInvoiceValue)
}

func TestAnalyticsOverviewEmptyState(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "emptyanalytics@example.com", "password123")

	rec := doRequest(router, http.MethodGet, "/api/analytics/overview", nil, token)
	require.Equal(t, http.StatusOK, rec.Code)

	var overview AnalyticsOverview
	require.NoError(t, decodeJSON(rec.Body.Bytes(), &overview))
	assert.Equal(t, 0.0, overview.TotalRevenue)
	assert.Equal(t, 0, overview.TotalInvoices)
}

func TestAnalyticsRevenueByMonth(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "revenueuser@example.com", "password123")

	seedInvoice(t, router, token, "2026-01-10", 1000, 0)
	seedInvoice(t, router, token, "2026-01-20", 500, 0)
	seedInvoice(t, router, token, "2026-02-05", 2000, 0)

	rec := doRequest(router, http.MethodGet, "/api/analytics/revenue?year=2026", nil, token)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp RevenueResponse
	require.NoError(t, decodeJSON(rec.Body.Bytes(), &resp))
	assert.Equal(t, "monthly", resp.Period)

	byLabel := map[string]RevenueDataPoint{}
	for _, dp := range resp.Data {
		byLabel[dp.Label] = dp
	}

	require.Contains(t, byLabel, "Jan")
	assert.Equal(t, 1500.0, byLabel["Jan"].Total)
	assert.Equal(t, 2, byLabel["Jan"].Count)

	require.Contains(t, byLabel, "Feb")
	assert.Equal(t, 2000.0, byLabel["Feb"].Total)
	assert.Equal(t, 1, byLabel["Feb"].Count)
}
```

- [ ] **Step 2: Run the tests and verify they pass**

Run: `cd backend && go test ./... -run 'TestAnalytics' -v`
Expected: all `PASS`. If a computed value doesn't match, double check the seeded `Date` values land in the year/month you expect (dates are `YYYY-MM-DD` strings) before assuming the handler is wrong.

- [ ] **Step 3: Commit**

```bash
git add backend/analytics_test.go
git commit -m "test: add analytics handler integration tests with seeded data"
```

---

### Task 9: Export (PDF/CSV/Excel) smoke tests

**Files:**
- Create: `backend/export_test.go`

**Interfaces:**
- Consumes: `truncateTables`, `doRequest`, `registerTestUser`, `decodeJSON` (Task 5); `Invoice`, `InvoiceItem` (`backend/main.go`).
- Produces: nothing new consumed by later tasks.

- [ ] **Step 1: Write `backend/export_test.go`**

```go
package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadInvoicePDF(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "pdfuser@example.com", "password123")

	createRec := doRequest(router, http.MethodPost, "/api/invoices", Invoice{
		ClientName: "PDF Test Client",
		Date:       "2026-01-15",
		TaxRate:    10,
		Items:      []InvoiceItem{{Description: "Item", Qty: 1, Price: 100}},
	}, token)
	require.Equal(t, http.StatusCreated, createRec.Code)

	var created Invoice
	require.NoError(t, decodeJSON(createRec.Body.Bytes(), &created))

	rec := doRequest(router, http.MethodGet, "/api/invoices/"+created.ID+"/pdf", nil, token)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/pdf", rec.Header().Get("Content-Type"))
	assert.NotEmpty(t, rec.Body.Bytes())
}

func TestDownloadInvoiceCSV(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "csvuser@example.com", "password123")

	createRec := doRequest(router, http.MethodPost, "/api/invoices", Invoice{
		ClientName: "CSV Test Client",
		Date:       "2026-01-15",
		TaxRate:    10,
		Items:      []InvoiceItem{{Description: "Item", Qty: 1, Price: 100}},
	}, token)
	require.Equal(t, http.StatusCreated, createRec.Code)

	var created Invoice
	require.NoError(t, decodeJSON(createRec.Body.Bytes(), &created))

	rec := doRequest(router, http.MethodGet, "/api/invoices/"+created.ID+"/csv", nil, token)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/csv", rec.Header().Get("Content-Type"))
	assert.NotEmpty(t, rec.Body.Bytes())
}

func TestExportInvoicesExcel(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "exceluser@example.com", "password123")

	createRec := doRequest(router, http.MethodPost, "/api/invoices", Invoice{
		ClientName: "Excel Test Client",
		Date:       "2026-01-15",
		TaxRate:    10,
		Items:      []InvoiceItem{{Description: "Item", Qty: 1, Price: 100}},
	}, token)
	require.Equal(t, http.StatusCreated, createRec.Code)

	rec := doRequest(router, http.MethodGet, "/api/invoices/export/excel", nil, token)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", rec.Header().Get("Content-Type"))
	assert.NotEmpty(t, rec.Body.Bytes())
}

func TestDownloadAnalyticsReport(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "reportuser@example.com", "password123")

	pdfRec := doRequest(router, http.MethodGet, "/api/analytics/report?format=pdf&year=2026", nil, token)
	assert.Equal(t, http.StatusOK, pdfRec.Code)
	assert.NotEmpty(t, pdfRec.Body.Bytes())

	excelRec := doRequest(router, http.MethodGet, "/api/analytics/report?format=excel&year=2026", nil, token)
	assert.Equal(t, http.StatusOK, excelRec.Code)
	assert.NotEmpty(t, excelRec.Body.Bytes())
}
```

- [ ] **Step 2: Run the tests and verify they pass**

Run: `cd backend && go test ./... -run 'TestDownload|TestExportInvoicesExcel' -v`
Expected: all `PASS`.

- [ ] **Step 3: Commit**

```bash
git add backend/export_test.go
git commit -m "test: add export (PDF/CSV/Excel) smoke tests"
```

---

### Task 10: Full suite verification, coverage report, and TODO.md housekeeping

**Files:**
- Modify: `TODO.md`

**Interfaces:**
- Consumes: all test files from Tasks 1-9.
- Produces: nothing (final task).

- [ ] **Step 1: Run the entire backend test suite**

Run: `cd backend && go test ./... -v`
Expected: every test from Tasks 1-9 passes. If anything fails intermittently on repeated runs, suspect a `truncateTables` ordering issue or a test not calling it — fix before proceeding.

- [ ] **Step 2: Generate and inspect coverage**

Run: `cd backend && go test ./... -cover`
Expected: a coverage percentage is printed (roughly 50-60% per the spec's realistic target — export/PDF byte-level generation code and some error branches are intentionally untested). Note the actual number for the TODO.md update in Step 4.

Optional deeper look: `go test ./... -coverprofile=coverage.out && go tool cover -html=coverage.out -o coverage.html` to see exactly which lines are untested (don't commit `coverage.out`/`coverage.html` — they're generated artifacts; add `backend/coverage.out` and `backend/coverage.html` to `.gitignore` if you generate them).

- [ ] **Step 3: Run `go vet` and `go build` one more time**

Run: `cd backend && go vet ./... && go build ./...`
Expected: no warnings, successful build.

- [ ] **Step 4: Update `TODO.md`**

In `TODO.md`, under `## 🎯 Phase 5: Reporting & Analytics`, change every `- [ ]` to `- [x]` for the items actually shipped (Dashboard metrics, Revenue chart, Top clients chart; leave "Outstanding invoices chart" and anything explicitly noted as Phase-6-dependent as `- [ ]` per `docs/PHASE5_IMPLEMENTASI_ANALYTICS.md`'s own scope notes) and "Tax summary report" under Reports as `- [x]`.

Under `## 🎯 Phase 9: Testing & Code Quality`, update the `### Backend Testing` block:

```markdown
### Backend Testing

- [x] Unit tests for API handlers
- [x] Integration tests with test database
- [ ] End-to-end tests for full workflows
- [x] Test coverage tracked (`go test ./... -cover`) — realistic ~50-60% target, not blind >80% (export/PDF byte-level output intentionally excluded; see `docs/superpowers/specs/2026-07-16-phase9-backend-testing-design.md`)
```

Leave `### Frontend Testing` and `### Code Quality` sections untouched — out of scope for this plan.

- [ ] **Step 5: Commit**

```bash
git add TODO.md
git commit -m "docs: update TODO.md to reflect Phase 5 completion and Phase 9 backend testing progress"
```

---

## Post-Plan Note

E2E tests, frontend testing, and coverage beyond the ~50-60% realistic target are intentionally out of scope (see spec's "What We Skip" section) — they're candidates for a future phase, not omissions to silently fix here.
