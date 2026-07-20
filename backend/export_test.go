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
	token := registerTestUser(t, router, "pdfuser@mail.com", "password")

	createRec := doRequest(router, http.MethodPost, "/api/invoices", Invoice{
		ClientName: "PDF Test Client",
		Date: "2026-01-15",
		TaxRate: 10,
		Items: []InvoiceItem{{Description: "Item", Qty: 1, Price: 100}},
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
	token := registerTestUser(t, router, "csvuser@example.com", "password")

	createRec := doRequest(router, http.MethodPost, "/api/invoices", Invoice{
		ClientName: "CSV Test Client",
		Date: "2026-01-15",
		TaxRate: 10,
		Items: []InvoiceItem{{Description: "Item", Qty: 1, Price: 100}},
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
	token := registerTestUser(t, router, "reportuser@mail.com", "password")

	pdfRec := doRequest(router, http.MethodGet, "/api/analytics/report?format=pdf&year=2026", nil, token)
	assert.Equal(t, http.StatusOK, pdfRec.Code)
	assert.NotEmpty(t, pdfRec.Body.Bytes())

	excelRec := doRequest(router, http.MethodGet, "/api/analytics/report?format=excel&year=2026", nil, token)
	assert.Equal(t, http.StatusOK, excelRec.Code)
	assert.NotEmpty(t, excelRec.Body.Bytes())
}