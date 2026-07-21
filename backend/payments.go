package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func handleRecordPayment(c *gin.Context) {
	invoiceID := c.Param("id")
	userID, _ := c.Get("user_id")

	var req PaymentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	// Fetch invoice — ownership check + current status.
	var inv Invoice
	err := db.QueryRow(ctx, 
		`SELECT id, total_amount, status, user_id FROM invoices WHERE id = $1 AND user_id = $2`, invoiceID, userID,).Scan(&inv.ID, &inv.TotalAmount, &inv.Status, &inv.UserID,
	)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
		return
	}

	if inv.Status == "Cancelled"  {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot pay a cancelled invoice"})
		return
	}

	// Insert payment.
	paymentID := uuid.New().String()
	now := time.Now()
	_, err = db.Exec(ctx, 
		`INSERT INTO payments (id, invoice_id, amount, date, method, recorded_by, created_at) VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		paymentID, invoiceID, req.Amount, req.Date, req.Method, userID, now,
	)

	if err != nil {
		slog.Error("insert payment error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record payment"})
		return
	}

	// Check if fully paid.
	var totalPaid float64
	db.QueryRow(ctx,
		`SELECT COALESCE(SUM(amount), 0) FROM payments WHERE invoice_id = $1`,
		invoiceID,
	).Scan(&totalPaid)

	newStatus := inv.Status
	if totalPaid >= inv.TotalAmount && inv.Status != "Paid" {
		// Auto-transition to Paid.
		db.Exec(ctx,
			`UPDATE invoices SET status = 'Paid', updated_at = $1 WHERE id = $2`,
			now, invoiceID,
		)
		newStatus = "Paid"
		// Record auto-transition in status history.
		writeStatusHistory(ctx, invoiceID, inv.Status, "Paid", userID.(string))
	}

	c.JSON(http.StatusCreated, gin.H{
		"payment": Payment{
			ID:			paymentID,
			InvoiceID: 	invoiceID,
			Amount: 	req.Amount,
			Date: 		req.Date,
			Method: 	req.Method,
			RecordedBy:	userID.(string),
			CreatedAt: 	now,
		},
		"invoice_status": newStatus,
	})
}

func handleListPayments(c *gin.Context) {
	invoiceID := c.Param("id")
	userID, _ := c.Get("user_id")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Ownership check.
	var ownerID string
	err := db.QueryRow(ctx, `SELECT user_id FROM invoices WHERE id = $1`, invoiceID).Scan(&ownerID)
	if err != nil || ownerID != userID.(string) {
		c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
		return
	}

	rows, err := db.Query(ctx,
		`SELECT id, invoice_id, amount, CAST(date AS TEXT), method, recorded_by, created_at FROM payments WHERE invoice_id = $1 ORDER BY date DESC, created_at DESC`,
		invoiceID,
	)

	if err != nil {
		slog.Error("query payments error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch payments"})
		return
	}
	defer rows.Close()

	var payments []Payment
	for rows.Next() {
		var p Payment
		if err := rows.Scan(&p.ID, &p.InvoiceID, &p.Amount, &p.Date, &p.Method, &p.RecordedBy, &p.CreatedAt); err != nil {
			slog.Error("scan payment error", "error", err)
			continue
		}
		payments = append(payments, p)
	}
	if payments == nil {
		payments = []Payment{}
	}

	c.JSON(http.StatusOK, payments)
}
