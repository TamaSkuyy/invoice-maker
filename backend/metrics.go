package main

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ── Metric Definitions ────────────────────────────────────────────────────

var (
	// Counter: selalu naik — jumlah total HTTP request.
	// Label: method (GET/POST/...), path (/api/invoices, ...), status (200/400/500).
	httpRequestsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "Total number of HTTP requests.",
		},
		[]string{"method", "path", "status"},
	)

	// Histogram: distribusi latency request dalam detik.
	// Bucket bawaan: .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10 detik.
	httpRequestDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "HTTP request latency in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	// Gauge: nilai yang naik-turun — koneksi database aktif saat ini.
	dbConnectionsActive = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "db_connections_active",
			Help: "Number of active database connections.",
		},
	)
)

func init() {
	// Daftarkan semua metric ke default Prometheus registry.
	prometheus.MustRegister(httpRequestsTotal)
	prometheus.MustRegister(httpRequestDuration)
	prometheus.MustRegister(dbConnectionsActive)
}

// ── Gin Middleware ─────────────────────────────────────────────────────────

// MetricsMiddleware mencatat setiap HTTP request ke Prometheus metrics.
// Pasang sebagai middleware global di Gin router.
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Lanjutkan ke handler berikutnya
		c.Next()

		// Hitung durasi setelah response terkirim
		duration := time.Since(start).Seconds()
		status := strconv.Itoa(c.Writer.Status())

		// Gunakan FullPath() untuk dapat path pattern (misal /api/invoices/:id),
		// bukan path aktual (misal /api/invoices/abc123). Ini penting untuk
		// agregasi — semua request ke /api/invoices/:id dikelompokkan jadi satu.
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path // fallback kalau gak match route
		}

		httpRequestsTotal.WithLabelValues(c.Request.Method, path, status).Inc()
		httpRequestDuration.WithLabelValues(c.Request.Method, path).Observe(duration)
	}
}

// ── Prometheus Scrape Endpoint ─────────────────────────────────────────────

// MetricsHandler mengembalikan Gin handler yang serve /api/metrics endpoint.
// Endpoint ini akan di-scrape oleh Prometheus setiap 15 detik.
func MetricsHandler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}
