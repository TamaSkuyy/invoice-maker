package main

import (
	"fmt"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
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

// store is a simple in-memory invoice store.
type store struct {
	mu       sync.RWMutex
	invoices map[string]Invoice
	counter  int
}

func newStore() *store {
	return &store{invoices: make(map[string]Invoice)}
}

// nextID generates a new invoice ID. Must be called with s.mu held.
func (s *store) nextID() string {
	s.counter++
	return "INV-" + time.Now().Format("20060102") + "-" + itoa(s.counter)
}

func itoa(n int) string {
	return fmt.Sprintf("%04d", n)
}

func main() {
	s := newStore()
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
			s.mu.RLock()
			defer s.mu.RUnlock()
			list := make([]Invoice, 0, len(s.invoices))
			for _, inv := range s.invoices {
				list = append(list, inv)
			}
			c.JSON(http.StatusOK, list)
		})

		// Get a single invoice
		api.GET("/:id", func(c *gin.Context) {
			id := c.Param("id")
			s.mu.RLock()
			inv, ok := s.invoices[id]
			s.mu.RUnlock()
			if !ok {
				c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
				return
			}
			c.JSON(http.StatusOK, inv)
		})

		// Create a new invoice
		api.POST("", func(c *gin.Context) {
			var input Invoice
			if err := c.ShouldBindJSON(&input); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			s.mu.Lock()
			input.ID = s.nextID()
			input.TotalAmount = calculateTotal(input.Items, input.TaxRate)
			s.invoices[input.ID] = input
			s.mu.Unlock()
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
			s.mu.Lock()
			defer s.mu.Unlock()
			if _, ok := s.invoices[id]; !ok {
				c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
				return
			}
			input.ID = id
			input.TotalAmount = calculateTotal(input.Items, input.TaxRate)
			s.invoices[id] = input
			c.JSON(http.StatusOK, input)
		})

		// Delete an invoice
		api.DELETE("/:id", func(c *gin.Context) {
			id := c.Param("id")
			s.mu.Lock()
			defer s.mu.Unlock()
			if _, ok := s.invoices[id]; !ok {
				c.JSON(http.StatusNotFound, gin.H{"error": "invoice not found"})
				return
			}
			delete(s.invoices, id)
			c.JSON(http.StatusOK, gin.H{"message": "invoice deleted"})
		})
	}

	r.Run(":8080")
}
