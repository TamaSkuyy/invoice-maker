package main

import (
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
	ClientID    *string       `json:"client_id"`
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

// Client represents a saved client/customer.
type Client struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
	Address   string    `json:"address"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Product represents a saved product/service item.
type Product struct {
	ID           string    `json:"id"`
	UserID       string    `json:"user_id"`
	Name         string    `json:"name"`
	Description  string    `json:"description"`
	DefaultPrice float64   `json:"default_price"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
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

// runMigrationsWithConn runs database migrations against the given connection string.
func runMigrationsWithConn(connString string) error {
	m, err := migrate.New("file://./migrations", connString)
	if err != nil {
		return fmt.Errorf("unable to create migration instance: %w", err)
	}
	defer m.Close()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("unable to run migrations: %w", err)
	}
	
	return nil
}

// runMigrations runs the database migrations using connection settings from env vars.
func runMigrations() error {
	connString := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s?sslmode=disable",
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_HOST"),
		os.Getenv("DB_PORT"),
		os.Getenv("DB_NAME"),

	)
	return runMigrationsWithConn(connString)
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

	r := setupRouter()

	r.Run(":8080")
}
