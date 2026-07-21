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

// setupRouter builds the full Gin router. It relies on the package-level
// `db` var (backend/db.go) already being set — by initDB() in main(), or
// directly assigned in tests before calling setupRouter().
func setupRouter() *gin.Engine {
	r := gin.Default()

	// Prometheus metrics middleware — catat setiap request (latency + status).
	// Harus dipasang SEBELUM route handler, SESUDAH gin.Default().
	r.Use(MetricsMiddleware())

	// CORS  middleware for local frontend development
	r.Use(func(c *gin.Context)  {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// Auth routes (no authentication required)
	auth := r.Group("/api/auth")
	{
		// Register new user
		auth.POST("/register", func(c *gin.Context) {
			var req SignupRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer cancel()

			// Check if email already exists
			var exists bool
			err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)", req.Email).Scan(&exists)
			if err != nil || exists {
				c.JSON(http.StatusBadRequest, gin.H{"error": "email already registered"})
				return
			}

			// Hash password
			hash, err := hashPassword(req.Password)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process password"})
				return
			}

			// Create user
			userID := uuid.New().String()
			now := time.Now()
			_, err = db.Exec(ctx,
				"INSERT INTO users (id, email, password_hash, created_at, updated_at) VALUES ($1, $2, $3, $4, $5)",
				userID, req.Email, hash, now, now,
			)
			if err != nil {
				slog.Error("insert user error", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create user"})
				return
			}

			// Generate JWT
			token, err := generateJWT(userID, req.Email)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
				return
			}

			resp := AuthResponse{
				Token: token,
				User: User{
					ID:        userID,
					Email:     req.Email,
					CreatedAt: now,
					UpdatedAt: now,
				},
			}
			c.JSON(http.StatusCreated, resp)
		})

		// Login user
		auth.POST("/login", func(c *gin.Context) {
			var req LoginRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer cancel()

			// Find user by email
			var user User
			var passwordHash string
			err := db.QueryRow(ctx, "SELECT id, email, password_hash, created_at, updated_at FROM users WHERE email = $1", req.Email).
				Scan(&user.ID, &user.Email, &passwordHash, &user.CreatedAt, &user.UpdatedAt)
			if err != nil {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
				return
			}

			// Verify password
			if !verifyPassword(passwordHash, req.Password) {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid email or password"})
				return
			}

			// Generate JWT
			token, err := generateJWT(user.ID, user.Email)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
				return
			}

			resp := AuthResponse{
				Token: token,
				User:  user,
			}
			c.JSON(http.StatusOK, resp)
		})

		// Get current user (protected)
		auth.GET("/me", authenticate(), func(c *gin.Context) {
			userID, _ := c.Get("user_id")

			ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer cancel()

			var user User
			err := db.QueryRow(ctx, "SELECT id, email, created_at, updated_at FROM users WHERE id = $1", userID).
				Scan(&user.ID, &user.Email, &user.CreatedAt, &user.UpdatedAt)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
				return
			}

			c.JSON(http.StatusOK, user)
		})
	}

	// Invoice routes (protected)
	api := r.Group("/api/invoices")
	api.Use(authenticate())
	{
		// List all invoices for current user
		api.GET("", func(c *gin.Context) {
			userID, _ := c.Get("user_id")
			statusFilter := c.DefaultQuery("status", "")
			ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer cancel()

			query := `SELECT id, client_name, client_id, CAST(date AS TEXT), COALESCE(CAST(due_date AS TEXT), ''), tax_rate, total_amount, status, user_id, created_at, updated_at FROM invoices WHERE user_id = $1`
			args := []interface{}{userID}

			if statusFilter == "Overdue" {
				query += ` AND status NOT IN ('Paid','Cancelled') AND due_date < CURRENT_DATE`
			} else if statusFilter != "" {
				query += ` AND status = $2`
				args = append(args, statusFilter)
			}
			query += ` ORDER BY created_at DESC`

			rows, err := db.Query(ctx, query, args...)
			if err != nil {
				slog.Error("query error", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch invoices"})
				return
			}
			defer rows.Close()

			var invoices []Invoice
			for rows.Next() {
				var inv Invoice
				if err := rows.Scan(&inv.ID, &inv.ClientName, &inv.ClientID, &inv.Date, &inv.DueDate, &inv.TaxRate, &inv.TotalAmount, &inv.Status, &inv.UserID, &inv.CreatedAt, &inv.UpdatedAt); err != nil {
					slog.Error("scan error", "error", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse invoices"})
					return
				}

				// Fetch invoice items
				itemRows, err := db.Query(ctx, "SELECT description, qty, price FROM invoice_items WHERE invoice_id = $1 ORDER BY created_at", inv.ID)
				if err != nil {
					slog.Error("query items error", "error", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch invoice items"})
					return
				}

				var items []InvoiceItem
				for itemRows.Next() {
					var item InvoiceItem
					if err := itemRows.Scan(&item.Description, &item.Qty, &item.Price); err != nil {
						slog.Error("scan item error", "error", err)
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

		// Export all invoices to Excel (MUST be before /:id to avoid route conflict)
		api.GET("/export/excel", func(c *gin.Context) {
			userID, _ := c.Get("user_id")
			ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
			defer cancel()

			rows, err := db.Query(ctx, "SELECT id, client_name, client_id, CAST(date AS TEXT), COALESCE(CAST(due_date AS TEXT), ''), tax_rate, total_amount, status, user_id, created_at, updated_at FROM invoices WHERE user_id = $1 ORDER BY created_at DESC", userID)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch invoices"})
				return
			}
			defer rows.Close()

			var invoices []Invoice
			for rows.Next() {
				var inv Invoice
				if err := rows.Scan(&inv.ID, &inv.ClientName, &inv.ClientID, &inv.Date, &inv.DueDate, &inv.TaxRate, &inv.TotalAmount, &inv.Status, &inv.UserID, &inv.CreatedAt, &inv.UpdatedAt); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse invoices"})
					return
				}
				var itemCount int
				db.QueryRow(ctx, "SELECT COUNT(*) FROM invoice_items WHERE invoice_id = $1", inv.ID).Scan(&itemCount)
				inv.Items = make([]InvoiceItem, itemCount)
				invoices = append(invoices, inv)
			}

			if invoices == nil {
				invoices = []Invoice{}
			}

			data, err := generateInvoicesExcel(invoices)
			if err != nil {
				slog.Error("excel generation error", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate Excel"})
				return
			}

			c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
			c.Header("Content-Disposition", "attachment; filename=invoices.xlsx")
			c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
		})

		// Get a single invoice (with ownership check)
		api.GET("/:id", func(c *gin.Context) {
			id := c.Param("id")
			userID, _ := c.Get("user_id")
			ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer cancel()

			var inv Invoice
			err := db.QueryRow(ctx, "SELECT id, client_name, client_id, CAST(date AS TEXT), COALESCE(CAST(due_date AS TEXT), ''), tax_rate, total_amount, status, user_id, created_at, updated_at FROM invoices WHERE id = $1 AND user_id = $2", id, userID).
				Scan(&inv.ID, &inv.ClientName, &inv.ClientID, &inv.Date, &inv.DueDate, &inv.TaxRate, &inv.TotalAmount, &inv.Status, &inv.UserID, &inv.CreatedAt, &inv.UpdatedAt)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
				return
			}

			// Fetch invoice items
			itemRows, err := db.Query(ctx, "SELECT description, qty, price FROM invoice_items WHERE invoice_id = $1 ORDER BY created_at", id)
			if err != nil {
				slog.Error("query items error", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch invoice items"})
				return
			}
			defer itemRows.Close()

			var items []InvoiceItem
			for itemRows.Next() {
				var item InvoiceItem
				if err := itemRows.Scan(&item.Description, &item.Qty, &item.Price); err != nil {
					slog.Error("scan item error", "error", err)
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

			userID, _ := c.Get("user_id")

			// Generate new ID
			input.ID = uuid.New().String()
			input.UserID = userID.(string)
			input.TotalAmount = calculateTotal(input.Items, input.TaxRate)
			now := time.Now()
			input.CreatedAt = now
			input.UpdatedAt = now

			ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
			defer cancel()

			// Start transaction
			tx, err := db.Begin(ctx)
			if err != nil {
				slog.Error("transaction error", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create invoice"})
				return
			}
			defer tx.Rollback(ctx)

			var dueDate interface{}
			if input.DueDate != "" {
				dueDate = input.DueDate
			}

			// Insert invoice
			_, err = tx.Exec(ctx,
				"INSERT INTO invoices (id, client_name, client_id, date, due_date, tax_rate, total_amount, status, user_id, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)",
				input.ID, input.ClientName, input.ClientID, input.Date, dueDate, input.TaxRate, input.TotalAmount, "Draft", input.UserID, input.CreatedAt, input.UpdatedAt,
			)
			if err != nil {
				slog.Error("insert invoice error", "error", err)
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
					slog.Error("insert item error", "error", err)
					c.JSON(http.StatusBadRequest, gin.H{"error": "failed to create invoice items"})
					return
				}
			}

			// Commit transaction
			if err := tx.Commit(ctx); err != nil {
				slog.Error("commit error", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create invoice"})
				return
			}

			input.Status = "Draft"

			c.JSON(http.StatusCreated, input)
		})

		// Update an existing invoice (with ownership check)
		api.PUT("/:id", func(c *gin.Context) {
			id := c.Param("id")
			userID, _ := c.Get("user_id")
			var input Invoice
			if err := c.ShouldBindJSON(&input); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}

			input.ID = id
			input.UserID = userID.(string)
			input.TotalAmount = calculateTotal(input.Items, input.TaxRate)
			input.UpdatedAt = time.Now()

			ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
			defer cancel()

			// Check if invoice exists and owned by user
			var exists bool
			err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM invoices WHERE id = $1 AND user_id = $2)", id, userID).Scan(&exists)
			if err != nil || !exists {
				c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
				return
			}

			// Start transaction
			tx, err := db.Begin(ctx)
			if err != nil {
				slog.Error("transaction error", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update invoice"})
				return
			}
			defer tx.Rollback(ctx)

			var dueDate interface{}
			if input.DueDate != "" {
				dueDate = input.DueDate
			}

			// Update invoice
			_, err = tx.Exec(ctx,
				"UPDATE invoices SET client_name = $1, date = $2, due_date = $3, tax_rate = $4, total_amount = $5, updated_at = $6 WHERE id = $7",
				input.ClientName, input.Date, dueDate, input.TaxRate, input.TotalAmount, input.UpdatedAt, id,
			)
			if err != nil {
				slog.Error("update invoice error", "error", err)
				c.JSON(http.StatusBadRequest, gin.H{"error": "failed to update invoice"})
				return
			}

			// Delete old items
			_, err = tx.Exec(ctx, "DELETE FROM invoice_items WHERE invoice_id = $1", id)
			if err != nil {
				slog.Error("delete items error", "error", err)
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
					slog.Error("insert item error", "error", err)
					c.JSON(http.StatusBadRequest, gin.H{"error": "failed to update invoice items"})
					return
				}
			}

			// Commit transaction
			if err := tx.Commit(ctx); err != nil {
				slog.Error("commit error", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update invoice"})
				return
			}

			// Fetch and return updated invoice
			var updatedInv Invoice
			err = db.QueryRow(ctx, "SELECT id, client_name, client_id, CAST(date AS TEXT), COALESCE(CAST(due_date AS TEXT), ''), tax_rate, total_amount, status, user_id, created_at, updated_at FROM invoices WHERE id = $1", id).
				Scan(&updatedInv.ID, &updatedInv.ClientName, &updatedInv.ClientID, &updatedInv.Date, &updatedInv.DueDate, &updatedInv.TaxRate, &updatedInv.TotalAmount, &updatedInv.Status, &updatedInv.UserID, &updatedInv.CreatedAt, &updatedInv.UpdatedAt)
			if err != nil {
				slog.Error("fetch updated invoice error", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch updated invoice"})
				return
			}

			// Fetch items
			itemRows, err := db.Query(ctx, "SELECT description, qty, price FROM invoice_items WHERE invoice_id = $1", id)
			if err != nil {
				slog.Error("query items error", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch invoice items"})
				return
			}
			defer itemRows.Close()

			var items []InvoiceItem
			for itemRows.Next() {
				var item InvoiceItem
				if err := itemRows.Scan(&item.Description, &item.Qty, &item.Price); err != nil {
					slog.Error("scan item error", "error", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse invoice items"})
					return
				}
				items = append(items, item)
			}

			updatedInv.Items = items
			c.JSON(http.StatusOK, updatedInv)
		})

		// Delete an invoice (with ownership check)
		api.DELETE("/:id", func(c *gin.Context) {
			id := c.Param("id")
			userID, _ := c.Get("user_id")
			ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer cancel()

			// Check if invoice exists and owned by user
			var exists bool
			err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM invoices WHERE id = $1 AND user_id = $2)", id, userID).Scan(&exists)
			if err != nil || !exists {
				c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
				return
			}

			// Delete invoice (cascade delete will handle items)
			_, err = db.Exec(ctx, "DELETE FROM invoices WHERE id = $1", id)
			if err != nil {
				slog.Error("delete error", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete invoice"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"message": "invoice deleted"})
		})

		// Download invoice as PDF
		api.GET("/:id/pdf", func(c *gin.Context) {
			id := c.Param("id")
			userID, _ := c.Get("user_id")
			ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
			defer cancel()

			var inv Invoice
			err := db.QueryRow(ctx, "SELECT id, client_name, client_id, CAST(date AS TEXT), COALESCE(CAST(due_date AS TEXT), ''), tax_rate, total_amount, status, user_id, created_at, updated_at FROM invoices WHERE id = $1 AND user_id = $2", id, userID).
				Scan(&inv.ID, &inv.ClientName, &inv.ClientID, &inv.Date, &inv.DueDate, &inv.TaxRate, &inv.TotalAmount, &inv.Status, &inv.UserID, &inv.CreatedAt, &inv.UpdatedAt)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
				return
			}

			itemRows, err := db.Query(ctx, "SELECT description, qty, price FROM invoice_items WHERE invoice_id = $1 ORDER BY created_at", id)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch items"})
				return
			}
			defer itemRows.Close()

			for itemRows.Next() {
				var item InvoiceItem
				if err := itemRows.Scan(&item.Description, &item.Qty, &item.Price); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse items"})
					return
				}
				inv.Items = append(inv.Items, item)
			}

			pdfData, err := generateInvoicePDF(inv)
			if err != nil {
				slog.Error("pdf generation error", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate PDF"})
				return
			}

			c.Header("Content-Type", "application/pdf")
			c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=invoice-%s.pdf", id[:8]))
			c.Data(http.StatusOK, "application/pdf", pdfData)
		})

		// Download invoice as CSV
		api.GET("/:id/csv", func(c *gin.Context) {
			id := c.Param("id")
			userID, _ := c.Get("user_id")
			ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
			defer cancel()

			var inv Invoice
			err := db.QueryRow(ctx, "SELECT id, client_name, client_id, CAST(date AS TEXT), COALESCE(CAST(due_date AS TEXT), ''), tax_rate, total_amount, status, user_id, created_at, updated_at FROM invoices WHERE id = $1 AND user_id = $2", id, userID).
				Scan(&inv.ID, &inv.ClientName, &inv.ClientID, &inv.Date, &inv.DueDate, &inv.TaxRate, &inv.TotalAmount, &inv.Status, &inv.UserID, &inv.CreatedAt, &inv.UpdatedAt)
			if err != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
				return
			}

			itemRows, err := db.Query(ctx, "SELECT description, qty, price FROM invoice_items WHERE invoice_id = $1 ORDER BY created_at", id)
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch items"})
				return
			}
			defer itemRows.Close()

			for itemRows.Next() {
				var item InvoiceItem
				if err := itemRows.Scan(&item.Description, &item.Qty, &item.Price); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse items"})
					return
				}
				inv.Items = append(inv.Items, item)
			}

			csvData, err := generateInvoiceCSV(inv)
			if err != nil {
				slog.Error("csv generation error", "error", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate CSV"})
				return
			}

			c.Header("Content-Type", "text/csv")
			c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=invoice-%s.csv", id[:8]))
			c.Data(http.StatusOK, "text/csv", csvData)
		})

		// Set invoice status (manual transition)
		api.PUT("/:id/status", handleSetInvoiceStatus)

		// Get invoice status history
		api.GET("/:id/history", handleStatusHistory)

		// Record a payment
		api.POST("/:id/payments", handleRecordPayment)

		// List payments for an invoice
		api.GET("/:id/payments", handleListPayments)
	}

		// Client management routes (protected)
		clients := r.Group("/api/clients")
		clients.Use(authenticate())
		{
			clients.GET("", func(c *gin.Context) {
				userID, _ := c.Get("user_id")
				ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
				defer cancel()

				rows, err := db.Query(ctx, "SELECT id, user_id, name, email, phone, address, created_at, updated_at FROM clients WHERE user_id = $1 ORDER BY name", userID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch clients"})
					return
				}
				defer rows.Close()

				var clients []Client
				for rows.Next() {
					var cl Client
					if err := rows.Scan(&cl.ID, &cl.UserID, &cl.Name, &cl.Email, &cl.Phone, &cl.Address, &cl.CreatedAt, &cl.UpdatedAt); err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse clients"})
						return
					}
					clients = append(clients, cl)
				}
				if clients == nil {
					clients = []Client{}
				}
				c.JSON(http.StatusOK, clients)
			})

			clients.POST("", func(c *gin.Context) {
				var input Client
				if err := c.ShouldBindJSON(&input); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				userID, _ := c.Get("user_id")

				input.ID = uuid.New().String()
				input.UserID = userID.(string)
				now := time.Now()
				input.CreatedAt = now
				input.UpdatedAt = now

				ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
				defer cancel()

				_, err := db.Exec(ctx,
					"INSERT INTO clients (id, user_id, name, email, phone, address, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
					input.ID, input.UserID, input.Name, input.Email, input.Phone, input.Address, input.CreatedAt, input.UpdatedAt,
				)
				if err != nil {
					slog.Error("insert client error", "error", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create client"})
					return
				}
				c.JSON(http.StatusCreated, input)
			})

			clients.PUT("/:id", func(c *gin.Context) {
				id := c.Param("id")
				userID, _ := c.Get("user_id")
				var input Client
				if err := c.ShouldBindJSON(&input); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}

				ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
				defer cancel()

				var exists bool
				err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM clients WHERE id = $1 AND user_id = $2)", id, userID).Scan(&exists)
				if err != nil || !exists {
					c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
					return
				}

				now := time.Now()
				_, err = db.Exec(ctx,
					"UPDATE clients SET name = $1, email = $2, phone = $3, address = $4, updated_at = $5 WHERE id = $6 AND user_id = $7",
					input.Name, input.Email, input.Phone, input.Address, now, id, userID,
				)
				if err != nil {
					slog.Error("update client error", "error", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update client"})
					return
				}

				var updated Client
				db.QueryRow(ctx, "SELECT id, user_id, name, email, phone, address, created_at, updated_at FROM clients WHERE id = $1", id).
					Scan(&updated.ID, &updated.UserID, &updated.Name, &updated.Email, &updated.Phone, &updated.Address, &updated.CreatedAt, &updated.UpdatedAt)
				c.JSON(http.StatusOK, updated)
			})

			clients.DELETE("/:id", func(c *gin.Context) {
				id := c.Param("id")
				userID, _ := c.Get("user_id")
				ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
				defer cancel()

				var exists bool
				err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM clients WHERE id = $1 AND user_id = $2)", id, userID).Scan(&exists)
				if err != nil || !exists {
					c.JSON(http.StatusNotFound, gin.H{"error": "client not found"})
					return
				}

				_, err = db.Exec(ctx, "DELETE FROM clients WHERE id = $1", id)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete client"})
					return
				}

				c.JSON(http.StatusOK, gin.H{"message": "client deleted"})
			})
		}

		// Product management routes (protected)
		products := r.Group("/api/products")
		products.Use(authenticate())
		{
			products.GET("", func(c *gin.Context) {
				userID, _ := c.Get("user_id")
				ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
				defer cancel()

				rows, err := db.Query(ctx, "SELECT id, user_id, name, description, default_price, created_at, updated_at FROM products WHERE user_id = $1 ORDER BY name", userID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch products"})
					return
				}
				defer rows.Close()

				var products []Product
				for rows.Next() {
					var p Product
					if err := rows.Scan(&p.ID, &p.UserID, &p.Name, &p.Description, &p.DefaultPrice, &p.CreatedAt, &p.UpdatedAt); err != nil {
						c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse products"})
						return
					}
					products = append(products, p)
				}
				if products == nil {
					products = []Product{}
				}
				c.JSON(http.StatusOK, products)
			})

			products.POST("", func(c *gin.Context) {
				var input Product
				if err := c.ShouldBindJSON(&input); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				userID, _ := c.Get("user_id")

				input.ID = uuid.New().String()
				input.UserID = userID.(string)
				now := time.Now()
				input.CreatedAt = now
				input.UpdatedAt = now

				ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
				defer cancel()

				_, err := db.Exec(ctx,
					"INSERT INTO products (id, user_id, name, description, default_price, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7)",
					input.ID, input.UserID, input.Name, input.Description, input.DefaultPrice, input.CreatedAt, input.UpdatedAt,
				)
				if err != nil {
					slog.Error("insert product error", "error", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create product"})
					return
				}
				c.JSON(http.StatusCreated, input)
			})

			products.PUT("/:id", func(c *gin.Context) {
				id := c.Param("id")
				userID, _ := c.Get("user_id")
				var input Product
				if err := c.ShouldBindJSON(&input); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}

				ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
				defer cancel()

				var exists bool
				err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM products WHERE id = $1 AND user_id = $2)", id, userID).Scan(&exists)
				if err != nil || !exists {
					c.JSON(http.StatusNotFound, gin.H{"error": "product not found"})
					return
				}

				now := time.Now()
				_, err = db.Exec(ctx,
					"UPDATE products SET name = $1, description = $2, default_price = $3, updated_at = $4 WHERE id = $5 AND user_id = $6",
					input.Name, input.Description, input.DefaultPrice, now, id, userID,
				)
				if err != nil {
					slog.Error("update product error", "error", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update product"})
					return
				}

				var updated Product
				db.QueryRow(ctx, "SELECT id, user_id, name, description, default_price, created_at, updated_at FROM products WHERE id = $1", id).
					Scan(&updated.ID, &updated.UserID, &updated.Name, &updated.Description, &updated.DefaultPrice, &updated.CreatedAt, &updated.UpdatedAt)
				c.JSON(http.StatusOK, updated)
			})

			products.DELETE("/:id", func(c *gin.Context) {
				id := c.Param("id")
				userID, _ := c.Get("user_id")
				ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
				defer cancel()

				var exists bool
				err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM products WHERE id = $1 AND user_id = $2)", id, userID).Scan(&exists)
				if err != nil || !exists {
					c.JSON(http.StatusNotFound, gin.H{"error": "product not found"})
					return
				}

				_, err = db.Exec(ctx, "DELETE FROM products WHERE id = $1", id)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete product"})
					return
				}

				c.JSON(http.StatusOK, gin.H{"message": "product deleted"})
			})
		}

	// Prometheus metrics endpoint — no auth required.
	// Di-scrape oleh Prometheus setiap 15 detik untuk mengumpulkan data
	// performa aplikasi (request rate, latency, error rate).
	r.GET("/api/metrics", MetricsHandler())

	// Health check endpoint — no auth required.
	// Used by Docker healthcheck, Prometheus, and uptime monitors.
	// Verifies the database connection is alive, not just that the HTTP
	// server is listening.
	r.GET("/api/health", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		// Ping the database — a real health check, not just "return 200".
		if err := db.Ping(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"db":     "unreachable",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status": "healthy",
			"time":   time.Now().UTC().Format(time.RFC3339),
		})
	})

		// Analytics routes (protected)
		analytics := r.Group("/api/analytics")
		analytics.Use(authenticate())
		{
			analytics.GET("/overview", handleAnalyticsOverview)
			analytics.GET("/revenue", handleAnalyticsRevenue)
			analytics.GET("/top-clients", handleAnalyticsTopClients)
			analytics.GET("/tax-summary", handleAnalyticsTaxSummary)
			analytics.GET("/report", handleAnalyticsReport)
		}

	return r
}
