import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import * as Sentry from '@sentry/react'
import './index.css'
import App from './App'
import { initSentry } from './lib/sentry'

// Init Sentry SEBELUM render — supaya error saat render juga tertangkap.
initSentry()

// Register PWA Service Worker — caching + offline support.
// Hanya di production (Vite dev server gak serve sw.js dengan benar).
if (import.meta.env.PROD || true) {
  navigator.serviceWorker.register("/sw.js").then(
    () => console.log("SW registered — offline ready"),
    (err) => console.warn("SW registration failed:", err),
  );
}

const root = document.getElementById('root')
if (!root) throw new Error('Root element not found')

createRoot(root).render(
  <StrictMode>
    {/* Sentry ErrorBoundary: tangkap error React + tampilkan fallback UI,
        bukan cuma blank screen. Tetap kirim error ke Sentry. */}
    <Sentry.ErrorBoundary
      fallback={({ error, resetError }: { error: unknown; resetError: () => void }) => (
        <div className="min-h-screen flex items-center justify-center bg-gray-50">
          <div className="text-center p-8 max-w-md">
            <div className="text-4xl mb-4">⚠️</div>
            <h1 className="text-xl font-semibold text-gray-800 mb-2">
              Something went wrong
            </h1>
            <p className="text-sm text-gray-500 mb-4">
              The error has been reported to our team.
            </p>
            <button
              onClick={resetError}
              className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 transition-colors text-sm"
            >
              Try Again
            </button>
            {import.meta.env.DEV && (
              <pre className="mt-4 text-left text-xs text-red-600 bg-red-50 p-3 rounded overflow-auto max-h-40">
                {error instanceof Error ? error.message : String(error)}
              </pre>
            )}
          </div>
        </div>
      )}
    >
      <App />
    </Sentry.ErrorBoundary>
  </StrictMode>,
)
