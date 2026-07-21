# Invoice Maker

Full-stack invoice management application built with Go + React + PostgreSQL. Create, manage, and export professional invoices with payment tracking, analytics, and PWA mobile support.

## Tech Stack

| Layer          | Technology                                          |
| -------------- | --------------------------------------------------- |
| Backend        | Go 1.25+ (Gin) — REST API                          |
| Frontend       | React 18 + TypeScript + Vite + Tailwind CSS         |
| Database       | PostgreSQL 16                                       |
| Auth           | JWT (Bearer token) + bcrypt                         |
| Export         | PDF (fpdf) + Excel (excelize) + CSV                 |
| Testing        | Go: `testing` + testify (67% coverage). TS: Vitest + React Testing Library |
| Linting        | ESLint (flat config) + Prettier + lint-staged       |
| Monitoring     | Prometheus + Grafana (7-panel dashboard)            |
| Error Tracking | Sentry (Go + React, session replay with PII masking)|
| CI/CD          | GitHub Actions (test → build → push to GHCR)        |
| Container      | Docker + Docker Compose (multi-stage builds)         |
| Reverse Proxy  | Caddy (auto-SSL via Let's Encrypt)                   |
| PWA            | Service worker (stale-while-revalidate), installable |
| Security       | Rate limiting, CSP/HSTS/XFO headers, input validation|

## Features

### Invoice Management
- Create, edit, delete invoices with multiple line items
- Client & product management (auto-fill from saved data)
- Invoice status lifecycle: Draft → Sent → Paid / Overdue / Cancelled
- Status history audit trail
- Partial & full payment recording with auto-Paid trigger

### Export & Reports
- PDF download (blue-themed professional layout)
- Excel export (all invoices, formatted)
- CSV export (single invoice)
- Financial reports: revenue chart, top clients, tax summary
- Time-based analytics: weekly, monthly, yearly

### Authentication
- User registration/login with JWT tokens
- Multi-user data isolation (each user sees only their own data)
- Protected routes on frontend

### PWA & Mobile
- Installable to home screen (standalone mode)
- Offline support via service worker
- Touch-friendly UI (44pt touch targets, safe area padding)
- Responsive tables with horizontal scroll

### DevOps & Monitoring
- Prometheus metrics: request rate, latency (p50/p95/p99), error %
- Grafana dashboard (auto-provisioned)
- Sentry error tracking with session replay (PII masked)
- Structured logging (`log/slog`, JSON in production)
- Docker health checks (deep: `db.Ping()`)
- CI/CD pipeline: auto test → build Docker image → push to GHCR

### Security
- Rate limiting (token bucket per-IP, 10 req/s)
- Security headers: HSTS, CSP, X-Frame-Options, X-Content-Type-Options
- Input validation (Gin binding tags)
- Parameterized queries (SQL injection prevention)
- Production: Caddy auto-SSL, internal-only database

## Project Structure

```
invoice-maker/
├── backend/
│   ├── main.go              # Entry point, structs, auth helpers
│   ├── router.go            # All routes + middleware chain
│   ├── db.go                # Database connection + pool
│   ├── logger.go            # slog setup (JSON/text, env config)
│   ├── metrics.go           # Prometheus Counter/Histogram/Gauge
│   ├── ratelimit.go         # Per-IP token bucket middleware
│   ├── sentry.go            # Sentry init + flush
│   ├── export.go            # PDF + Excel + CSV generation
│   ├── analytics.go         # Revenue, top-clients, tax-summary
│   ├── payments.go          # Payment recording + listing
│   ├── status.go            # Status transitions + history
│   ├── logic_test.go        # Unit tests (pure functions)
│   ├── *_test.go            # Integration tests (17 functions, 67%)
│   ├── migrations/          # SQL migrations (golang-migrate)
│   └── Dockerfile           # Multi-stage: dev → builder → production
├── frontend/
│   ├── src/
│   │   ├── main.tsx         # Entry: Sentry init + SW registration
│   │   ├── App.tsx          # Root component with routing
│   │   ├── components/      # 13 components (forms, charts, pages)
│   │   ├── hooks/           # useAuth hook
│   │   ├── utils/           # api.ts, export.ts
│   │   ├── types/           # TypeScript interfaces
│   │   ├── lib/             # sentry.ts
│   │   └── test/            # Test setup (Vitest globals)
│   ├── public/              # PWA icons + manifest + sw.js
│   ├── index.html           # PWA meta tags
│   ├── eslint.config.js     # ESLint flat config v10
│   ├── .prettierrc          # Prettier config
│   ├── vitest.config.ts     # Vitest + React Testing Library
│   ├── nginx.conf           # SPA fallback + API proxy
│   └── Dockerfile           # Multi-stage: bun build → nginx serve
├── monitoring/
│   ├── prometheus.yml       # Scrape config
│   └── grafana-dashboards/  # 7-panel API dashboard
├── .github/workflows/
│   └── ci.yml               # CI/CD: test → build → push GHCR
├── Caddyfile                # Reverse proxy + SSL + security headers
├── docker-compose.yml       # Dev: postgres + backend (air) + frontend
├── docker-compose.prod.yml  # Prod: Caddy + backend + frontend + postgres
├── docker-compose.monitoring.yml  # Prometheus + Grafana
├── deploy-production.sh     # Production deploy script
├── dev-local.sh             # Local dev: postgres container + host backend/frontend
├── .env.prod.example        # Production env template
└── docs/                    # Learning docs (Phase 1-10)
```

## Quick Start

### Prerequisites
- Go 1.25+
- Node.js 20+ (or Bun)
- Docker & Docker Compose

### Local Development (recommended)

```bash
# Start PostgreSQL + backend (air hot reload) + frontend (Vite HMR)
./dev-local.sh

# First time only: install dependencies
./dev-local.sh --init

# Frontend: http://localhost:5173
# Backend:  http://localhost:8080
# Database: localhost:5432
```

### With Docker Compose

```bash
docker compose up -d
# Frontend: http://localhost:3000
# Backend:  http://localhost:8080
```

## API Endpoints

| Method | Path | Auth | Description |
|--------|------|------|-------------|
| `GET` | `/api/health` | No | Health check (DB ping) |
| `GET` | `/api/metrics` | No | Prometheus metrics |
| `POST` | `/api/auth/register` | No | Register user |
| `POST` | `/api/auth/login` | No | Login user |
| `GET` | `/api/auth/me` | Yes | Current user |
| `GET` | `/api/invoices` | Yes | List invoices (filter: `?status=`) |
| `GET` | `/api/invoices/:id` | Yes | Get invoice by ID |
| `POST` | `/api/invoices` | Yes | Create invoice |
| `PUT` | `/api/invoices/:id` | Yes | Update invoice |
| `DELETE` | `/api/invoices/:id` | Yes | Delete invoice |
| `GET` | `/api/invoices/:id/pdf` | Yes | Download PDF |
| `GET` | `/api/invoices/:id/csv` | Yes | Download CSV |
| `GET` | `/api/invoices/export/excel` | Yes | Export all to Excel |
| `PUT` | `/api/invoices/:id/status` | Yes | Set invoice status |
| `GET` | `/api/invoices/:id/history` | Yes | Status history |
| `POST` | `/api/invoices/:id/payments` | Yes | Record payment |
| `GET` | `/api/invoices/:id/payments` | Yes | List payments |
| `GET` | `/api/clients` | Yes | List clients |
| `POST` | `/api/clients` | Yes | Create client |
| `PUT` | `/api/clients/:id` | Yes | Update client |
| `DELETE` | `/api/clients/:id` | Yes | Delete client |
| `GET` | `/api/products` | Yes | List products |
| `POST` | `/api/products` | Yes | Create product |
| `PUT` | `/api/products/:id` | Yes | Update product |
| `DELETE` | `/api/products/:id` | Yes | Delete product |
| `GET` | `/api/analytics/overview` | Yes | Dashboard overview |
| `GET` | `/api/analytics/revenue` | Yes | Revenue chart data |
| `GET` | `/api/analytics/top-clients` | Yes | Top clients data |
| `GET` | `/api/analytics/tax-summary` | Yes | Tax summary data |
| `GET` | `/api/analytics/report` | Yes | Export financial report |

### Invoice Model

```json
{
  "id": "abc123-...",
  "client_name": "PT Maju Jaya",
  "client_id": "def456-..." | null,
  "date": "2026-07-21",
  "due_date": "2026-08-21",
  "items": [
    { "description": "Website Development", "qty": 1, "price": 500000 }
  ],
  "tax_rate": 11,
  "total_amount": 555000,
  "status": "Draft",
  "user_id": "ghi789-...",
  "created_at": "2026-07-21T...",
  "updated_at": "2026-07-21T..."
}
```

## Scripts

### Frontend

```bash
npm run dev          # Vite dev server (HMR)
npm run build        # TypeScript check + Vite build
npm test             # Vitest run (3 files, 13 tests)
npm run test:watch   # Vitest watch mode
npm run lint         # ESLint check
npm run lint:fix     # ESLint auto-fix
npm run format       # Prettier write
npm run format:check # Prettier check
```

### Backend

```bash
go test ./...                    # Run all tests (67% coverage)
go test ./... -coverprofile=coverage.out  # With coverage
go build -o server .             # Build binary
./server                         # Run (reads env vars for DB config)
```

### DevOps

```bash
# Production deploy
./deploy-production.sh           # Full deploy
./deploy-production.sh --update  # Quick update (pull + rebuild)
./deploy-production.sh --logs    # Tail logs
./deploy-production.sh --down    # Stop stack

# Monitoring
docker compose -f docker-compose.yml -f docker-compose.monitoring.yml up -d
# Prometheus: http://localhost:9090
# Grafana:    http://localhost:3001  (admin/admin)

# CI/CD
# Push to main → GitHub Actions auto test + build + push to GHCR
```

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_HOST` | `postgres` | Database host |
| `DB_PORT` | `5432` | Database port |
| `DB_USER` | `invoiceuser` | Database user |
| `DB_PASSWORD` | — | Database password (required in prod) |
| `DB_NAME` | `invoicedb` | Database name |
| `JWT_SECRET` | — | JWT signing secret (min 32 chars in prod) |
| `JWT_EXPIRATION` | `86400` | Token expiration in seconds |
| `LOG_FORMAT` | `text` | `text` or `json` |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `SENTRY_DSN` | — | Sentry DSN (optional, disabled if empty) |
| `RATE_LIMIT_RPS` | `10` | Rate limit requests per second per IP |
| `RATE_LIMIT_BURST` | `20` | Rate limit burst capacity |
| `DOMAIN` | `localhost` | Production domain (for Caddy SSL) |
| `GIT_SHA` | — | Git commit SHA (for Sentry release tracking) |

## Testing

```
Backend:  17 test functions, 67.0% coverage
         Unit: pure functions (round2, calculateTotal, password, JWT)
         Integration: auth, invoices (CRUD + isolation), clients,
                      products, analytics, export smoke tests

Frontend: 3 test files, 13 tests
         ApiError class, LoginPage (6 tests), InvoicePreview (4 tests)
         Vitest + React Testing Library
```

## Deployment

See `docs/DEPLOYMENT_GUIDE.md` for step-by-step VPS deployment guide.

Quick overview:
1. Provision VPS (Ubuntu, Docker installed)
2. Clone repo + copy `.env.prod.example` → `.env.prod`
3. Set `DOMAIN`, `DB_PASSWORD`, `JWT_SECRET` in `.env.prod`
4. Run `./deploy-production.sh`
5. Caddy auto-obtains SSL certificate → HTTPS live

## Documentation

| Phase | Topic | Doc |
|-------|-------|-----|
| Phase 2 | Authentication | `docs/PHASE2_IMPLEMENTASI_AUTH.md` |
| Phase 3 | File Export | `docs/PHASE3_IMPLEMENTASI_EXPORT.md` |
| Phase 4 | Clients & Products | `docs/PHASE4_IMPLEMENTASI_CLIENTS_PRODUCTS.md` |
| Phase 5 | Analytics | `docs/PHASE5_IMPLEMENTASI_ANALYTICS.md` |
| Phase 6 | Status & Payments | `docs/PHASE6_IMPLEMENTASI_STATUS_PAYMENT.md` |
| Phase 7 | PWA & Mobile | `docs/PHASE7_IMPLEMENTASI_PWA.md` |
| Phase 8 | DevOps | `docs/PHASE8_IMPLEMENTASI_DEVOPS.md` |
| Phase 9 | Testing & Code Quality | `docs/PHASE9_IMPLEMENTASI_TESTING.md` |
| Phase 10 | Security | `docs/PHASE10_IMPLEMENTASI_SECURITY.md` |
| — | Deployment Guide | `docs/DEPLOYMENT_GUIDE.md` |

## License

MIT
