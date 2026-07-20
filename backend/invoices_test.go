package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInvoiceLifeCycle(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "invoiceowner@mail.com", "password")

	// Create
	createBody := Invoice{
		ClientName: "Supplier A",
		Date: "2026-01-15",
		TaxRate: 10,
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
	assert.Equal(t, "Supplier A", fetched.ClientName)
	require.Len(t, fetched.Items, 1)
	assert.Equal(t, "Web Design", fetched.Items[0].Description)

	// Update
	updateBody := Invoice{
		ClientName: "Supplier A Updated",
		Date: "2026-01-16",
		TaxRate: 10,
		Items: []InvoiceItem{
			{Description: "Web Design", Qty: 1, Price: 600},
		},
	}
	updateRec := doRequest(router, http.MethodPut, "/api/invoices/"+created.ID, updateBody, token)
	require.Equal(t, http.StatusOK, updateRec.Code, updateRec.Body.String())

	var updated Invoice
	require.NoError(t, decodeJSON(updateRec.Body.Bytes(), &updated))
	assert.Equal(t, "Supplier A Updated", updated.ClientName)
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


func  TestInvoiceIsolationBetweenUsers(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	tokenA := registerTestUser(t, router, "usera@mail.com", "password")
	tokenB := registerTestUser(t, router, "userb@mail.com", "password")

	createRec := doRequest(router, http.MethodPost, "/api/invoices", Invoice{
		ClientName: "User A's Client",
		Date: "2026-01-15",
		TaxRate: 0,
		Items: []InvoiceItem{{Description: "Item", Qty: 1, Price: 100}},
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
