package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Allowed status values
var validStatuses = map[string]bool{
	"Draft":		true,
	"Sent":			true,
	"Paid":			true,
	"Overdue":		true,
	"Cancelled":	true,
}

// Transition rules — which from→to pairs are allowed for manual changes.
// Paid can never be set manually. Overdue is computed, never set.
var allowedTransitions = map[string]map[string]bool{
	"Draft": {"Sent": true, "Cancelled": true},
	"Sent": {"Cancelled": true},
}

func isValidStatusTransition(oldStatus,  newStatus string)bool {
	if !validStatuses[newStatus] {
		return false
	}
	// Paid is auto-only; Overdue is computed, never set manually.
	if newStatus == "Paid" || newStatus == "Overdue" {
		return false
	}
	allowed, ok := allowedTransitions[oldStatus]
	return ok && allowed[newStatus]
}

func writeStatusHistory(ctx context.Context, invoiceID, oldStatus, newStatus, changedBy string) error {
	_, err := db.Exec(ctx, `INSERT INTO status_history  (id, invoice_id, old_status, new_status, changed_by, changed_at) VALUES ($1, $2, $3, $4, $5, $6)`, uuid.New().String(), invoiceID, oldStatus, newStatus, changedBy, time.Now())
	return err
}

func handleSetInvoiceStatus(c *gin.Context) {
	invoiceID :=  c.Param("id")
	userID, _ := c.Get("user_id")

	var req StatusChangeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})

		return
	}

	ctx, cancel  := context.WithTimeout(c.Request.Context(),  5*time.Second)
	defer cancel()

	// Fetch current status - ownership check built-in.
	var currentStatus string
	err := db.QueryRow(ctx, `SELECT status FROM invoices WHERE id = $1 AND user_id = $2`, invoiceID, userID).Scan(&currentStatus)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
		return
	}

	if !isValidStatusTransition(currentStatus, req.Status) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("invalid status transition: %s --> %s", currentStatus, req.Status),
		})
		return
	}

	// Update status
	now := time.Now()
	_, err = db.Exec(ctx, `UPDATE invoices SET status = $1, updated_at = $2 WHERE id = $3`, req.Status, now, invoiceID,)
	if err != nil {
		slog.Error("update status error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update status"})
		return
	}

	// Record in history
	if err := writeStatusHistory(ctx, invoiceID, currentStatus, req.Status, userID.(string)); err != nil {
		slog.Error("write status history error", "error", err)
		// Non-fatal - status already updated, log the missing audit entry
	}

	c.JSON(http.StatusOK, gin.H{"status": req.Status, "previous_status": currentStatus})
}

func handleStatusHistory(c *gin.Context) {
	invoiceID := c.Param("id")
	userID, _ := c.Get("user_id")

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	// Ownership check - invoice must belong to user.
	var ownerID string
	err := db.QueryRow(ctx, `SELECT user_id FROM invoices WHERE id = $1`, invoiceID).Scan(&ownerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
		return
	}
	if ownerID != userID.(string) {
		c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
		return
	}

	rows, err := db.Query(ctx, `SELECT id, invoice_id, old_status, new_status, changed_by, changed_at FROM status_history WHERE invoice_id = $1 ORDER BY changed_at ASC`, invoiceID,)
	if err != nil {
		slog.Error("query status history error", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch status history"})
		return
	}
	defer rows.Close()

	var history []StatusHistoryEntry
	for rows.Next() {
		var h StatusHistoryEntry
		if err := rows.Scan(&h.ID, &h.InvoiceID, &h.OldStatus, &h.NewStatus, &h.ChangedBy, &h.ChangedAt); err != nil {
			slog.Error("scan history error", "error", err)
			continue
		}
		history = append(history, h)
	}
	if history == nil {
		history = []StatusHistoryEntry{}
	}

	c.JSON(http.StatusOK, history)
}
