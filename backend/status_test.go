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

	// Create invoice --> defaults to Draft.
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

	// Draft --> Sent
	statusRec := doRequest(router, http.MethodPut, "/api/invoices/"+inv.ID+"/status", StatusChangeRequest{Status: "Sent"}, token)
	require.Equal(t, http.StatusOK, statusRec.Code, statusRec.Body.String())

	// Verify status history.
	histRec := doRequest(router, http.MethodGet, "/api/invoices/"+inv.ID+"/history", nil, token)
	require.Equal(t, http.StatusOK, histRec.Code)
	var history []StatusHistoryEntry
	require.NoError(t, decodeJSON(histRec.Body.Bytes(), &history))
	require.Len(t, history, 1)
	assert.Equal(t, "Sent", history[0].NewStatus)
	require.NotNil(t, history[0].OldStatus)
	assert.Equal(t, "Draft", *history[0].OldStatus)
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
	statusRec := doRequest(router, http.MethodPut, "/api/invoices/"+inv.ID+"/status", StatusChangeRequest{Status: "Paid"}, token)
	assert.Equal(t, http.StatusBadRequest, statusRec.Code)

	// Draft → Overdue should fail (Overdue is computed, never set manually).
	statusRec = doRequest(router, http.MethodPut, "/api/invoices/"+inv.ID+"/status", StatusChangeRequest{Status: "Overdue"}, token)
	assert.Equal(t, http.StatusBadRequest, statusRec.Code)

	// Draft → Sent.
	doRequest(router, http.MethodPut, "/api/invoices/"+inv.ID+"/status", StatusChangeRequest{Status: "Sent"}, token)

	// Sent → Draft should fail (can't go backward to Draft).
	statusRec = doRequest(router, http.MethodPut, "/api/invoices/"+inv.ID+"/status", StatusChangeRequest{Status: "Draft"}, token)
	assert.Equal(t, http.StatusBadRequest, statusRec.Code)

	// Now cancel it.
	doRequest(router, http.MethodPut, "/api/invoices/"+inv.ID+"/status", StatusChangeRequest{Status: "Cancelled"}, token)

	// Cancelled → Sent should fail (can't revive).
	statusRec = doRequest(router, http.MethodPut, "/api/invoices/"+inv.ID+"/status", StatusChangeRequest{Status: "Sent"}, token)
	assert.Equal(t, http.StatusBadRequest, statusRec.Code)
}

func TestSetInvoiceStatusNotFound(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "notfoundstatustest@mail.com", "password")

	rec := doRequest(router, http.MethodPut, "/api/invoices/0000-00-00-0-000/status", StatusChangeRequest{Status: "Sent"}, token)
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
	require.Equal(t, http.StatusCreated, rec.Code)
	decodeJSON(rec.Body.Bytes(), &inv)

	// User B tries to change User A's invoice status → 404.
	statusRec := doRequest(router, http.MethodPut, "/api/invoices/"+inv.ID+"/status", StatusChangeRequest{Status: "Sent"}, tokenB)
	assert.Equal(t, http.StatusNotFound, statusRec.Code)
}
