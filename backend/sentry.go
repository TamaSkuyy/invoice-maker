package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/getsentry/sentry-go"
)

// initSentry initializes Sentry error tracking.
//
// Sentry DSN diambil dari env SENTRY_DSN. Kalau env tidak diset,
// Sentry disabled — aplikasi tetap jalan normal tanpa error tracking.
//
// DSN (Data Source Name) adalah URL unik yang mengarahkan error ke
// project kamu di sentry.io. Format:
//
//	https://<key>@o<org>.ingest.sentry.io/<project>
func initSentry() {
	dsn := os.Getenv("SENTRY_DSN")
	if dsn == "" {
		slog.Warn("SENTRY_DSN not set — error tracking disabled")
		return
	}

	env := os.Getenv("ENV")
	if env == "" {
		env = "development"
	}

	err := sentry.Init(sentry.ClientOptions{
		Dsn:              dsn,
		Environment:      env,
		Debug:            env != "production", // verbose logging di dev
		TracesSampleRate: traceSampleRate(env),
		// Kirim release version supaya Sentry tahu versi mana yang error
		Release: os.Getenv("GIT_SHA"),
	})
	if err != nil {
		slog.Error("sentry init failed", "error", err)
		return
	}

	slog.Info("sentry initialized", "env", env)
}

// traceSampleRate returns the percentage of transactions sent to Sentry.
// Development: 100% (debugging). Production: 10% (hemat quota).
func traceSampleRate(env string) float64 {
	if env == "production" {
		return 0.1
	}
	return 1.0
}

// flushSentry mengirim semua event yang masih di buffer sebelum aplikasi exit.
// Dipanggil di main() dengan defer.
// Flush aman dipanggil meskipun Sentry tidak di-init — langsung return true.
func flushSentry() {
	sentry.Flush(2 * time.Second)
}
