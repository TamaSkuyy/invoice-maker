import * as Sentry from "@sentry/react";

/**
 * Initialize Sentry error tracking for the frontend.
 *
 * DSN dibaca dari VITE_SENTRY_DSN (environment variable Vite).
 * Kalau tidak diset, Sentry disabled — aplikasi tetap jalan normal.
 *
 * Fitur yang diaktifkan:
 *   - browserTracing: track performance + navigation timing
 *   - replay: rekam user session saat error terjadi (session replay)
 */
export function initSentry(): void {
  const dsn = import.meta.env.VITE_SENTRY_DSN;
  if (!dsn) {
    console.warn("VITE_SENTRY_DSN not set — frontend error tracking disabled");
    return;
  }

  Sentry.init({
    dsn,
    environment: import.meta.env.MODE, // "development" / "production"
    integrations: [
      Sentry.browserTracingIntegration(),
      Sentry.replayIntegration({
        // Mask SEMUA text content — mencegah data sensitif (nama klien,
        // email, nominal invoice, password) terekam di session replay.
        maskAllText: true,
        maskAllInputs: true,
        blockAllMedia: true,
      }),
    ],

    // Production: kirim 10% traces (hemat quota). Dev: 100%.
    tracesSampleRate: import.meta.env.PROD ? 0.1 : 1.0,

    // Replay: 100% session yang ada error-nya direkam
    replaysSessionSampleRate: import.meta.env.PROD ? 0.1 : 1.0,
    replaysOnErrorSampleRate: 1.0,
  });
}
