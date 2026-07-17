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

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	ctx := context.Background()

	adminConnStr := "postgres://invoiceuser:invoicepassword@localhost:5432/postgres?sslmode=disable"
	adminPool, err :=  pgxpool.New(ctx, adminConnStr)
	if err != nil {
		log.Fatalf("failed to connect to postgres admin db: %v", err)
	}

	// Terminate any lingering connections to the test db from a previous
	// run so DROP DATABASE doesn't fail with "database is being accessed
	// by other users".
	_, _ = adminPool.Exec(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = 'invoicedb_test' AND pid <> pg_backend_pid()`)
	if _, err := adminPool.Exec(ctx, `DROP DATABASE IF EXISTS invoicedb_test`); err != nil {
		log.Fatalf("failed to drop test db: %v", err)
	}
	if _, err := adminPool.Exec(ctx, `CREATE DATABASE invoicedb_test`); err != nil {
		log.Fatalf("failed to create test db: %v", err)
	}
	adminPool.Close()

	testConnStr := "postgres://invoiceuser:invoicepassword@localhost:5432/invoicedb_test?sslmode=disable"

	// Assign the package-level `db` var (declared in db.go) — every handler,
	// closure or named function, reads from this same var.
	db, err = pgxpool.New(ctx, testConnStr)
	if err != nil {
		log.Fatalf("failed to connect to test db: %v", err)
	}

	if err := runMigrationsWithConn(testConnStr);  err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}
	
	code := m.Run()

	db.Close()
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

func decodeJSON(data []byte, v any) error {
	return json.Unmarshal(data, v)
}