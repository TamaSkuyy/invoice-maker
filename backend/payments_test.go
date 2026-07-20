package main

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func createTestInvoice(t *testing.T, router *gin.Engine, token string) Invoice {
	t.Helper()
	rec := doRequest(router, http.MethodPost, "/api/invoices", Invoice{
		ClientName: "Payment Test Client",
		Date:       "2026-07-20",
		TaxRate:    0,
		Items:      []InvoiceItem{{Description: "Item", Qty: 1, Price: 1000}},
	}, token)
	require.Equal(t, http.StatusCreated, rec.Code)
	var inv Invoice
	require.NoError(t, decodeJSON(rec.Body.Bytes(), &inv))
	// Mark as Sent so payments can be recorded.
	doRequest(router, http.MethodPut, "/api/invoices/"+inv.ID+"/status", StatusChangeRequest{Status: "Sent"}, token)
	return inv
}

func TestRecordPaymentPartial(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "partialpayment@mail.com", "password")
	inv := createTestInvoice(t, router, token) // total_amount = 1000

	rec := doRequest(router, http.MethodPost, "/api/invoices/"+inv.ID+"/payments",
		PaymentRequest{Amount: 600, Date: "2026-07-20", Method: "Transfer"}, token)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	// Status should NOT be Paid (600 < 1000).
	var paymentResp struct {
		Payment       Payment `json:"payment"`
		InvoiceStatus string  `json:"invoice_status"`
	}
	require.NoError(t, decodeJSON(rec.Body.Bytes(), &paymentResp))
	assert.Equal(t, "Sent", paymentResp.InvoiceStatus)
	assert.Equal(t, 600.0, paymentResp.Payment.Amount)
}

func TestRecordPaymentFull(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "fullpayment@mail.com", "password")
	inv := createTestInvoice(t, router, token) // total_amount = 1000

	rec := doRequest(router, http.MethodPost, "/api/invoices/"+inv.ID+"/payments",
		PaymentRequest{Amount: 1000, Date: "2026-07-20", Method: "Transfer"}, token)
	require.Equal(t, http.StatusCreated, rec.Code)

	var paymentResp struct {
		Payment       Payment `json:"payment"`
		InvoiceStatus string  `json:"invoice_status"`
	}
	require.NoError(t, decodeJSON(rec.Body.Bytes(), &paymentResp))
	assert.Equal(t, "Paid", paymentResp.InvoiceStatus)

	// Status history should have 2 entries: Draft→Sent (from createTestInvoice), Sent→Paid (auto).
	histRec := doRequest(router, http.MethodGet, "/api/invoices/"+inv.ID+"/history", nil, token)
	var history []StatusHistoryEntry
	decodeJSON(histRec.Body.Bytes(), &history)
	assert.Len(t, history, 2)
	assert.Equal(t, "Sent", history[0].NewStatus)
	assert.Equal(t, "Paid", history[1].NewStatus)
}

func TestRecordPaymentMultiplePartial(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "multipayment@mail.com", "password")
	inv := createTestInvoice(t, router, token) // total_amount = 1000

	// Payment 1: 600.
	rec1 := doRequest(router, http.MethodPost, "/api/invoices/"+inv.ID+"/payments",
		PaymentRequest{Amount: 600, Date: "2026-07-20", Method: "Transfer"}, token)
	require.Equal(t, http.StatusCreated, rec1.Code)
	var resp1 struct {
		Payment       Payment `json:"payment"`
		InvoiceStatus string  `json:"invoice_status"`
	}
	decodeJSON(rec1.Body.Bytes(), &resp1)
	assert.Equal(t, "Sent", resp1.InvoiceStatus) // Still not paid.

	// Payment 2: 400 → total 1000, should auto-Paid.
	rec2 := doRequest(router, http.MethodPost, "/api/invoices/"+inv.ID+"/payments",
		PaymentRequest{Amount: 400, Date: "2026-07-21", Method: "Cash"}, token)
	require.Equal(t, http.StatusCreated, rec2.Code)
	var resp2 struct {
		Payment       Payment `json:"payment"`
		InvoiceStatus string  `json:"invoice_status"`
	}
	decodeJSON(rec2.Body.Bytes(), &resp2)
	assert.Equal(t, "Paid", resp2.InvoiceStatus)

	// List payments should return 2 entries.
	listRec := doRequest(router, http.MethodGet, "/api/invoices/"+inv.ID+"/payments", nil, token)
	var payments []Payment
	decodeJSON(listRec.Body.Bytes(), &payments)
	assert.Len(t, payments, 2)
}


func TestRecordPaymentCancelledInvoice(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	token := registerTestUser(t, router, "cancelledpayment@mail.com", "password")

	rec := doRequest(router, http.MethodPost, "/api/invoices", Invoice{
		ClientName: "Will Be Cancelled",
		Date:       "2026-07-20",
		TaxRate:    0,
		Items:      []InvoiceItem{{Description: "Item", Qty: 1, Price: 1000}},
	}, token)
	var inv Invoice
	decodeJSON(rec.Body.Bytes(), &inv)

	// Cancel it.
	doRequest(router, http.MethodPut, "/api/invoices/"+inv.ID+"/status", StatusChangeRequest{Status: "Cancelled"}, token)

	// Try to pay → 400.
	payRec := doRequest(router, http.MethodPost, "/api/invoices/"+inv.ID+"/payments", PaymentRequest{Amount: 500, Date: "2026-07-20", Method: "Transfer"}, token)
	assert.Equal(t, http.StatusBadRequest, payRec.Code)
}

func TestListPaymentsIsolation(t *testing.T) {
	truncateTables(t)
	router := setupRouter()
	tokenA := registerTestUser(t, router, "paymentisoA@mail.com", "password")
	tokenB := registerTestUser(t, router, "paymentisoB@mail.com", "password")

	inv := createTestInvoice(t, router, tokenA)
	doRequest(router, http.MethodPost, "/api/invoices/"+inv.ID+"/payments",
		PaymentRequest{Amount: 500, Date: "2026-07-20", Method: "Transfer"}, tokenA)

	// User B tries to list payments for User A's invoice → 404.
	rec := doRequest(router, http.MethodGet, "/api/invoices/"+inv.ID+"/payments", nil, tokenB)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
