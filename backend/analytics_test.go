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
		Date: date,
		TaxRate: taxRate,
		Items: []InvoiceItem{{Description: "Item", Qty: 1, Price: price}},
	}, token)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
}

func TestAnalyticsOverview(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "analyticsuser@mail.com", "password")

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
	token := registerTestUser(t, router, "emptyanalytics@mail.com", "password")

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
	token := registerTestUser(t, router, "revenueuser@mail.com", "password")

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