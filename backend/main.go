package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	migrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"golang.org/x/crypto/bcrypt"
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
	UserID      string        `json:"user_id"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
}

// User represents a user account.
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SignupRequest for user registration
type SignupRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8"`
}

// LoginRequest for user login
type LoginRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

// AuthResponse contains token and user info
type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// CustomClaims for JWT token
type CustomClaims struct {
	UserID string `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
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

// hashPassword hashes a password using bcrypt
func hashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(hash), err
}

// verifyPassword verifies a password against a hash
func verifyPassword(hash, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// generateJWT generates a JWT token for a user
func generateJWT(userID, email string) (string, error) {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "your-secret-key-change-in-production-use-min-32-chars"
	}

	expirationStr := os.Getenv("JWT_EXPIRATION")
	if expirationStr == "" {
		expirationStr = "900" // 15 minutes default
	}

	expirationSeconds, _ := strconv.Atoi(expirationStr)

	claims := CustomClaims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Duration(expirationSeconds) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(jwtSecret))
}

// validateJWT validates and parses a JWT token
func validateJWT(tokenString string) (*CustomClaims, error) {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "your-secret-key-change-in-production-use-min-32-chars"
	}

	claims := &CustomClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(jwtSecret), nil
	})

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	return claims, nil
}

// authenticate is a Gin middleware that validates JWT token
func authenticate() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		// Extract token from "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header format"})
			c.Abort()
			return
		}

		token := parts[1]
		claims, err := validateJWT(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		// Store user info in context
		c.Set("user_id", claims.UserID)
		c.Set("email", claims.Email)
		c.Next()
	}
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
				log.Printf("insert user error: %v", err)
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
			ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
			defer cancel()

			rows, err := db.Query(ctx, "SELECT id, client_name, CAST(date AS TEXT), tax_rate, total_amount, user_id, created_at, updated_at FROM invoices WHERE user_id = $1 ORDER BY created_at DESC", userID)
			if err != nil {
				log.Printf("query error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch invoices"})
				return
			}
			defer rows.Close()

			var invoices []Invoice
			for rows.Next() {
				var inv Invoice
				if err := rows.Scan(&inv.ID, &inv.ClientName, &inv.Date, &inv.TaxRate, &inv.TotalAmount, &inv.UserID, &inv.CreatedAt, &inv.UpdatedAt); err != nil {
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

			// Export all invoices to Excel (MUST be before /:id to avoid route conflict)
			api.GET("/export/excel", func(c *gin.Context) {
				userID, _ := c.Get("user_id")
				ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
				defer cancel()

				rows, err := db.Query(ctx, "SELECT id, client_name, CAST(date AS TEXT), tax_rate, total_amount, user_id, created_at, updated_at FROM invoices WHERE user_id = $1 ORDER BY created_at DESC", userID)
				if err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch invoices"})
					return
				}
				defer rows.Close()

				var invoices []Invoice
				for rows.Next() {
					var inv Invoice
					if err := rows.Scan(&inv.ID, &inv.ClientName, &inv.Date, &inv.TaxRate, &inv.TotalAmount, &inv.UserID, &inv.CreatedAt, &inv.UpdatedAt); err != nil {
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
					log.Printf("excel generation error: %v", err)
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
			err := db.QueryRow(ctx, "SELECT id, client_name, CAST(date AS TEXT), tax_rate, total_amount, user_id, created_at, updated_at FROM invoices WHERE id = $1 AND user_id = $2", id, userID).
				Scan(&inv.ID, &inv.ClientName, &inv.Date, &inv.TaxRate, &inv.TotalAmount, &inv.UserID, &inv.CreatedAt, &inv.UpdatedAt)
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
				log.Printf("transaction error: %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create invoice"})
				return
			}
			defer tx.Rollback(ctx)

			// Insert invoice
			_, err = tx.Exec(ctx,
				"INSERT INTO invoices (id, client_name, date, tax_rate, total_amount, user_id, created_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)",
				input.ID, input.ClientName, input.Date, input.TaxRate, input.TotalAmount, input.UserID, input.CreatedAt, input.UpdatedAt,
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
			err = db.QueryRow(ctx, "SELECT id, client_name, CAST(date AS TEXT), tax_rate, total_amount, user_id, created_at, updated_at FROM invoices WHERE id = $1", id).
				Scan(&updatedInv.ID, &updatedInv.ClientName, &updatedInv.Date, &updatedInv.TaxRate, &updatedInv.TotalAmount, &updatedInv.UserID, &updatedInv.CreatedAt, &updatedInv.UpdatedAt)
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
				log.Printf("delete error: %v", err)
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
				err := db.QueryRow(ctx, "SELECT id, client_name, CAST(date AS TEXT), tax_rate, total_amount, user_id, created_at, updated_at FROM invoices WHERE id = $1 AND user_id = $2", id, userID).
					Scan(&inv.ID, &inv.ClientName, &inv.Date, &inv.TaxRate, &inv.TotalAmount, &inv.UserID, &inv.CreatedAt, &inv.UpdatedAt)
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
					log.Printf("pdf generation error: %v", err)
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
				err := db.QueryRow(ctx, "SELECT id, client_name, CAST(date AS TEXT), tax_rate, total_amount, user_id, created_at, updated_at FROM invoices WHERE id = $1 AND user_id = $2", id, userID).
					Scan(&inv.ID, &inv.ClientName, &inv.Date, &inv.TaxRate, &inv.TotalAmount, &inv.UserID, &inv.CreatedAt, &inv.UpdatedAt)
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
					log.Printf("csv generation error: %v", err)
					c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate CSV"})
					return
				}

				c.Header("Content-Type", "text/csv")
				c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=invoice-%s.csv", id[:8]))
				c.Data(http.StatusOK, "text/csv", csvData)
			})
		}

		r.Run(":8080")
}
