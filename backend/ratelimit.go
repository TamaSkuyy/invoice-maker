package main

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/time/rate"
)

// ── Rate Limiter — Token Bucket per IP ──────────────────────────────────────
//
// Setiap IP dapat "token bucket" sendiri. Token diisi ulang secara periodik.
// Kalau token habis → request ditolak (429 Too Many Requests).
//
// Token bucket algorithm:
//   Bucket kapasitas = burst tokens (max request berturut-turut)
//   Refill rate      = requests per detik
//   Setiap request   → consume 1 token
//   Token habis      → 429
//
// Kenapa per-IP bukan global?
//   Global rate limit = semua user share quota → 1 user bisa habisin jatah
//   semua user lain. Per-IP = setiap user punya jatah sendiri.

// clientRateLimiter menyimpan rate limiter untuk setiap IP.
// Map[string]*rateLimiterEntry dengan mutex untuk concurrent access.
type clientRateLimiter struct {
	mu      sync.RWMutex
	entries map[string]*rateLimiterEntry
	rate    rate.Limit // token per detik
	burst   int        // max token (burst capacity)
}

type rateLimiterEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// getLimiter mengembalikan rate.Limiter untuk IP tertentu.
// Kalau IP belum ada → buat baru. Kalau sudah ada → return yg sudah ada.
func (r *clientRateLimiter) getLimiter(ip string) *rate.Limiter {
	r.mu.RLock()
	entry, exists := r.entries[ip]
	r.mu.RUnlock()

	if exists {
		entry.lastSeen = time.Now()
		return entry.limiter
	}

	// Buat rate limiter baru untuk IP ini
	r.mu.Lock()
	defer r.mu.Unlock()

	// Double-check — mungkin IP sudah dibuat oleh goroutine lain
	if entry, exists = r.entries[ip]; exists {
		entry.lastSeen = time.Now()
		return entry.limiter
	}

	limiter := rate.NewLimiter(r.rate, r.burst)
	r.entries[ip] = &rateLimiterEntry{
		limiter:  limiter,
		lastSeen: time.Now(),
	}
	return limiter
}

// cleanup menghapus entries yang sudah lama tidak dipakai.
// Dipanggil secara periodik via goroutine untuk cegah memory leak.
func (r *clientRateLimiter) cleanup(maxAge time.Duration) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		r.mu.Lock()
		cutoff := time.Now().Add(-maxAge)
		for ip, entry := range r.entries {
			if entry.lastSeen.Before(cutoff) {
				delete(r.entries, ip)
			}
		}
		r.mu.Unlock()
	}
}

// RateLimitMiddleware membuat Gin middleware untuk rate limiting.
//
// Config via environment variables:
//
//	RATE_LIMIT_RPS    — requests per detik per IP (default: 10)
//	RATE_LIMIT_BURST  — burst capacity per IP (default: 20)
func RateLimitMiddleware() gin.HandlerFunc {
	rps, _ := strconv.ParseFloat(getEnvOrDefault("RATE_LIMIT_RPS", "10"), 64)
	burst, _ := strconv.Atoi(getEnvOrDefault("RATE_LIMIT_BURST", "20"))

	limiter := &clientRateLimiter{
		entries: make(map[string]*rateLimiterEntry),
		rate:    rate.Limit(rps),
		burst:   burst,
	}

	// Goroutine untuk membersihkan entries lama (cegah memory leak)
	go limiter.cleanup(30 * time.Minute)

	// pathWhitelist: paths yg TIDAK di-rate-limit.
	// Docker healthcheck (tiap 10s) dan Prometheus scrape (tiap 15s)
	// harus selalu diizinkan supaya tidak false-positive.
	pathWhitelist := map[string]bool{
		"/api/health":  true,
		"/api/metrics": true,
	}

	return func(c *gin.Context) {
		// Skip whitelisted paths
		if pathWhitelist[c.Request.URL.Path] {
			c.Next()
			return
		}

		ip := clientIP(c)
		if !limiter.getLimiter(ip).Allow() {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate_limit_exceeded",
				"message":     "Too many requests. Please slow down.",
				"retry_after": "1s",
			})
			return
		}
		c.Next()
	}
}

// clientIP mengekstrak IP client dari request.
// Prioritas: X-Real-IP (dari Caddy) → X-Forwarded-For → RemoteAddr.
func clientIP(c *gin.Context) string {
	// X-Real-IP diset oleh Caddy reverse proxy — sumber terpercaya
	if ip := c.GetHeader("X-Real-IP"); ip != "" {
		return ip
	}

	// X-Forwarded-For bisa punya multiple IPs (client, proxy1, proxy2)
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		// Ambil IP pertama (client asli) dari daftar
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}

	// Fallback: remote address langsung
	ip := c.Request.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx] // strip port
	}
	return ip
}

func getEnvOrDefault(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}
