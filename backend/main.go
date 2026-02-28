package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	migrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

// InvoiceItem represents a single line item in an invoice.
type InvoiceItem struct {
	Description string  `json:"description"`
	Qty         float64 `json:"qty"`
	Price       float64 `json:"price"`
}

// Invoice represents the full invoice document.
type Invoice struct {
	ID          string        `json:"id"`
	ClientName  string        `json:"client_name"`
	Date        string        `json:"date"`
	Items       []InvoiceItem `json:"items"`
	TaxRate     float64       `json:"tax_rate"`
	TotalAmount float64       `json:"total_amount"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// round2 rounds a float to 2 decimal places.
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}

// calculateTotal computes the total amount for an invoice.
func calculateTotal(items []InvoiceItem, taxRate float64) float64 {
	var subtotal float64
	for _, item := range items {
		subtotal += item.Qty * item.Price
	}
	return round2(subtotal + subtotal*taxRate/100)
}

// runMigrations runs the database migrations
func runMigrations() error {
	connString := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_NAME"),
	)

	m, err := migrate.New(
		"file://./migrations",
		connString,
	)
	if err != nil {
		return fmt.Errorf("unable to create migration instance: %w", err)
	}
	defer m.Close()

	// Apply migrations (up)
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("unable to run migrations: %w", err)
	}

	return nil
}

func main() {
	// Initialize database
	if err := initDB(); err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer closeDB()

	// Run migrations
	if err := runMigrations(); err != nil {
		log.Fatalf("failed to run migrations: %v", err)
	}

	r := gin.Default()

	// CORS middleware for local frontend development
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	api := r.Group("/api/invoices")
	{
		// List all invoices
		api.GET("", func(c *gin.Context) {
			ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer cancel()

			rows, err := db.Query(ctx, "SELECT id, client_name, CAST(date AS TEXT), tax_rate, total_amount, created_at, updated_at FROM invoices ORDER BY created_at DESC")
			if err != nil {
				log.Printf("query error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch invoices"})
				return
			}
			defer rows.Close()

			var invoices []Invoice
			for rows.Next() {
				var inv Invoice
				if err := rows.Scan(&inv.ID, &inv.ClientName, &inv.Date, &inv.TaxRate, &inv.TotalAmount, &inv.CreatedAt, &inv.UpdatedAt); err != nil {
					log.Printf("scan error: %v", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse invoices"})
					return
				}

				// Fetch invoice items
				itemRows, err := db.Query(ctx, "SELECT description, qty, price FROM invoice_items WHERE invoice_id = $1 ORDER BY created_at", inv.ID)
				if err != nil {
					log.Printf("query items error: %v", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch invoice items"})
					return
				}

				var items []InvoiceItem
				for itemRows.Next() {
					var item InvoiceItem
					if err := itemRows.Scan(&item.Description, &item.Qty, &item.Price); err != nil {
						log.Printf("scan item error: %v", err)
						itemRows.Close()
						c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse invoice items"})
						return
					}
					items = append(items, item)
				}
				itemRows.Close()

				inv.Items = items
				invoices = append(invoices, inv)
			}

			if invoices == nil {
				invoices = []Invoice{}
			}

			c.JSON(http.StatusOK, invoices)
		})

		// Get a single invoice
		api.GET("/:id", func(c *gin.Context) {
			id := c.Param("id")
			ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer cancel()

			var inv Invoice
			err := db.QueryRow(ctx, "SELECT id, client_name, CAST(date AS TEXT), tax_rate, total_amount, created_at, updated_at FROM invoices WHERE id = $1", id).
				Scan(&inv.ID, &inv.ClientName, &inv.Date, &inv.TaxRate, &inv.TotalAmount, &inv.CreatedAt, &inv.UpdatedAt)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
				return
			}

			// Fetch invoice items
			itemRows, err := db.Query(ctx, "SELECT description, qty, price FROM invoice_items WHERE invoice_id = $1 ORDER BY created_at", id)
			if err != nil {
				log.Printf("query items error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch invoice items"})
				return
			}
			defer itemRows.Close()

			var items []InvoiceItem
			for itemRows.Next() {
				var item InvoiceItem
				if err := itemRows.Scan(&item.Description, &item.Qty, &item.Price); err != nil {
					log.Printf("scan item error: %v", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse invoice items"})
					return
				}
				items = append(items, item)
			}

			inv.Items = items
			c.JSON(http.StatusOK, inv)
		})

		// Create a new invoice
		api.POST("", func(c *gin.Context) {
			var input Invoice
			if err := c.ShouldBindJSON(&input); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			// Generate new ID
			input.ID = uuid.New().String()
			input.TotalAmount = calculateTotal(input.Items, input.TaxRate)
			now := time.Now()
			input.CreatedAt = now
			input.UpdatedAt = now

			ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
			defer cancel()

			// Start transaction
			tx, err := db.Begin(ctx)
			if err != nil {
				log.Printf("transaction error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create invoice"})
				return
			}
			defer tx.Rollback(ctx)

			// Insert invoice
			_, err = tx.Exec(ctx,
				"INSERT INTO invoices (id, client_name, date, tax_rate, total_amount, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7)",
				input.ID, input.ClientName, input.Date, input.TaxRate, input.TotalAmount, input.CreatedAt, input.UpdatedAt,
			)
			if err != nil {
				log.Printf("insert invoice error: %v", err)
				c.JSON(http.StatusBadRequest, gin.H{"error": "failed to create invoice"})
				return
			}

			// Insert invoice items
			for _, item := range input.Items {
				_, err = tx.Exec(ctx,
					"INSERT INTO invoice_items (id, invoice_id, description, qty, price) VALUES ($1, $2, $3, $4, $5)",
					uuid.New().String(), input.ID, item.Description, item.Qty, item.Price,
				)
				if err != nil {
					log.Printf("insert item error: %v", err)
					c.JSON(http.StatusBadRequest, gin.H{"error": "failed to create invoice items"})
					return
				}
			}

			// Commit transaction
			if err := tx.Commit(ctx); err != nil {
				log.Printf("commit error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create invoice"})
				return
			}

			c.JSON(http.StatusCreated, input)
		})

		// Update an existing invoice
		api.PUT("/:id", func(c *gin.Context) {
			id := c.Param("id")
			var input Invoice
			if err := c.ShouldBindJSON(&input); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			input.ID = id
			input.TotalAmount = calculateTotal(input.Items, input.TaxRate)
			input.UpdatedAt = time.Now()

			ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
			defer cancel()

			// Check if invoice exists
			var exists bool
			err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM invoices WHERE id = $1)", id).Scan(&exists)
			if err != nil || !exists {
				c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
				return
			}

			// Start transaction
			tx, err := db.Begin(ctx)
			if err != nil {
				log.Printf("transaction error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update invoice"})
				return
			}
			defer tx.Rollback(ctx)

			// Update invoice
			_, err = tx.Exec(ctx,
				"UPDATE invoices SET client_name = $1, date = $2, tax_rate = $3, total_amount = $4, updated_at = $5 WHERE id = $6",
				input.ClientName, input.Date, input.TaxRate, input.TotalAmount, input.UpdatedAt, id,
			)
			if err != nil {
				log.Printf("update invoice error: %v", err)
				c.JSON(http.StatusBadRequest, gin.H{"error": "failed to update invoice"})
				return
			}

			// Delete old items
			_, err = tx.Exec(ctx, "DELETE FROM invoice_items WHERE invoice_id = $1", id)
			if err != nil {
				log.Printf("delete items error: %v", err)
				c.JSON(http.StatusBadRequest, gin.H{"error": "failed to update invoice items"})
				return
			}

			// Insert new items
			for _, item := range input.Items {
				_, err = tx.Exec(ctx,
					"INSERT INTO invoice_items (id, invoice_id, description, qty, price) VALUES ($1, $2, $3, $4, $5)",
					uuid.New().String(), id, item.Description, item.Qty, item.Price,
				)
				if err != nil {
					log.Printf("insert item error: %v", err)
					c.JSON(http.StatusBadRequest, gin.H{"error": "failed to update invoice items"})
					return
				}
			}

			// Commit transaction
			if err := tx.Commit(ctx); err != nil {
				log.Printf("commit error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update invoice"})
				return
			}

			// Fetch and return updated invoice
			var updatedInv Invoice
			err = db.QueryRow(ctx, "SELECT id, client_name, CAST(date AS TEXT), tax_rate, total_amount, created_at, updated_at FROM invoices WHERE id = $1", id).
				Scan(&updatedInv.ID, &updatedInv.ClientName, &updatedInv.Date, &updatedInv.TaxRate, &updatedInv.TotalAmount, &updatedInv.CreatedAt, &updatedInv.UpdatedAt)
			if err != nil {
				log.Printf("fetch updated invoice error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch updated invoice"})
				return
			}

			// Fetch items
			itemRows, err := db.Query(ctx, "SELECT description, qty, price FROM invoice_items WHERE invoice_id = $1", id)
			if err != nil {
				log.Printf("query items error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch invoice items"})
				return
			}
			defer itemRows.Close()

			var items []InvoiceItem
			for itemRows.Next() {
				var item InvoiceItem
				if err := itemRows.Scan(&item.Description, &item.Qty, &item.Price); err != nil {
					log.Printf("scan item error: %v", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse invoice items"})
					return
				}
				items = append(items, item)
			}

			updatedInv.Items = items
			c.JSON(http.StatusOK, updatedInv)
		})

		// Delete an invoice
		api.DELETE("/:id", func(c *gin.Context) {
			id := c.Param("id")
			ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer cancel()

			// Check if invoice exists
			var exists bool
			err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM invoices WHERE id = $1)", id).Scan(&exists)
			if err != nil || !exists {
				c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
				return
			}

			// Delete invoice (cascade delete will handle items)
			_, err = db.Exec(ctx, "DELETE FROM invoices WHERE id = $1", id)
			if err != nil {
				log.Printf("delete error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete invoice"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "invoice deleted"})
		})
	}

	r.Run(":8080")
}
