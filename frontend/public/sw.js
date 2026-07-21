// =============================================================================
// Service Worker — Invoice Maker PWA
// =============================================================================
// Caching strategy: Stale-While-Revalidate
//   1. Return cached version IMMEDIATELY (fast)
//   2. Fetch fresh version in background
//   3. Update cache with fresh version for NEXT visit
//
// Cocok untuk: SPA assets (HTML, JS, CSS) yg berubah saat deploy baru.
// Assets app shell (logo, font, icon) pakai Cache-First (jarang berubah).
// API calls TIDAK di-cache (data dinamis).
// =============================================================================

const CACHE_VERSION = "v1";
const APP_SHELL = "invoice-maker-app-shell-" + CACHE_VERSION;
const DYNAMIC = "invoice-maker-dynamic-" + CACHE_VERSION;

// Assets yg di-cache saat service worker install.
// Ini adalah "app shell" — resource minimal yg dibutuhkan untuk render UI.
const PRECACHE_URLS = [
  "/",
  "/index.html",
  "/icon-192.png",
  "/icon-512.png",
  "/manifest.webmanifest",
];

// ── Install: pre-cache app shell ─────────────────────────────────────────

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches
      .open(APP_SHELL)
      .then((cache) => cache.addAll(PRECACHE_URLS))
      .then(() => self.skipWaiting()), // activate immediately
  );
});

// ── Activate: hapus cache versi lama ─────────────────────────────────────

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) =>
        Promise.all(
          keys
            .filter((key) => key !== APP_SHELL && key !== DYNAMIC)
            .map((key) => caches.delete(key)),
        ),
      )
      .then(() => self.clients.claim()), // take control of all clients
  );
});

// ── Fetch: stale-while-revalidate strategy ───────────────────────────────

self.addEventListener("fetch", (event) => {
  const { request } = event;
  const url = new URL(request.url);

  // Skip: API calls (data dinamis, gak di-cache)
  if (url.pathname.startsWith("/api/")) {
    return; // network-only, no caching
  }

  // Skip: non-GET requests
  if (request.method !== "GET") {
    return;
  }

  // Strategy: Stale-While-Revalidate untuk HTML + assets
  event.respondWith(
    caches.open(DYNAMIC).then((cache) =>
      cache.match(request).then((cached) => {
        // Fetch fresh version di background
        const fetched = fetch(request)
          .then((response) => {
            // Cache the fresh version
            if (response.ok) {
              cache.put(request, response.clone());
            }
            return response;
          })
          .catch(() => {
            // Network error — return cached version kalau ada
            return cached || new Response("Offline", { status: 503 });
          });

        // Return cached IMMEDIATELY, atau tunggu network kalau gak ada cache
        return cached || fetched;
      }),
    ),
  );
});
