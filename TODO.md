# Invoice Maker - Feature Roadmap & Learning Goals

Panduan pengembangan aplikasi Invoice Maker. Ini adalah project learning untuk mendalami full-stack development dengan Go + React + Docker.

---

## 🎯 Phase 1: Data Persistence (Critical)

### Database Integration

- [x] Replace in-memory storage with PostgreSQL database
  - Create database schema for invoices and items tables
  - Setup SQL migrations (use golang-migrate or sqlc)
  - Add connection pooling with pgx driver
- [x] Implement proper error handling for database operations
- [x] Add database transaction support (important for data consistency)

### Learning Goal

- Understand relational database design
- Learn SQL and database drivers
- Practice ACID principles

---

## 🎯 Phase 2: User Management & Authentication

### Authentication

- [x] Implement user registration/login system
  - Password hashing (bcrypt)
  - JWT token-based authentication
  - Refresh token mechanism
- [ ] Add role-based access control (RBAC)
  - Admin role: manage all invoices
  - User role: manage own invoices only
- [ ] User profile management page

### Database

- [x] Create users table with proper schema
- [x] Add user_id foreign key to invoices table
- [ ] Implement session management (optional with Redis)

### Frontend

- [x] Login/Register pages
- [x] Auth context/provider for React
- [x] Protected routes (redirect to login if not authenticated)
- [x] User profile dropdown menu

### Learning Goal

- Understand authentication & authorization
- Learn password security best practices
- Practice state management across routes

---

## 🎯 Phase 3: File Export & Downloads

### PDF Export

- [x] Generate PDF invoices from invoice data
  - Use library: `github.com/go-pdf/fpdf` (maintained fork of gofpdf)
  - Include watermark ("INVOICE") and footer ("Thank you for your business!")
  - Blue-themed professional layout matching frontend InvoicePreview
- [x] Download PDF from UI button
- [ ] Email PDF as attachment (optional)

### Excel/CSV Export

- [x] Export invoice list to Excel format
  - Use `github.com/xuri/excelize` (Go)
  - Formatted headers, money columns, auto-filter
- [x] Export single invoice details to CSV

### Frontend

- [x] Add "Download PDF" button in preview panel + each saved invoice row
- [x] Add "Export to Excel" button in saved invoices list
- [x] Add "CSV" download button in preview panel
- [x] Loading/disabled state on export buttons (exporting indicator)

### Learning Goal

- Learn file generation & streaming
- Understand different file formats
- Practice file download handling in browsers

---

## 🎯 Phase 4: Advanced Features

### Invoice Templates

- [-] Add multiple invoice templates (professional, minimal, detailed) — _skipped: low ROI_
- [-] Store template preferences in user profile — _skipped: low ROI_
- [-] Customize colors, fonts, logo upload — _skipped: low ROI_

### Client Management

- [x] Create separate clients database table
- [x] Client list with CRUD operations
- [x] Auto-fill client info when selecting from list
- [ ] Save client payment methods (optional)

### Invoice Line Items & Products

- [x] Create products table for frequently used items
- [x] Quick-select products when adding invoice items
- [x] Product price defaults & variations

### Tax & Currency

- [-] Support multiple currencies (USD, EUR, IDR, etc.) — _skipped: IDR + flat rate cukup_
- [-] Different tax rates per item — _skipped: IDR + flat rate cukup_
- [-] Sales tax vs service tax differentiation — _skipped: IDR + flat rate cukup_
- [-] Multi-country tax rules — _skipped: IDR + flat rate cukup_

### Learning Goal

- Understand database relationships (one-to-many, many-to-many)
- Practice complex form logic
- Learn internationalization/localization

---

## 🎯 Phase 5: Reporting & Analytics

### Dashboard

- [x] Display key metrics (total invoiced, paid, pending) — total invoiced ✅ (Phase 5); paid/pending/overdue ✅ (Phase 6 — `AnalyticsOverview.paid_amount`, `pending_amount`, `overdue_count` available in overview endpoint + DashboardCards)
- [x] Revenue chart (monthly/yearly)
- [x] Top clients chart
- [ ] Outstanding invoices chart — skipped (butuh Phase 6)

### Reports

- [x] Generate financial reports (PDF/Excel)
- [x] Tax summary report
- [ ] Client payment history report — skipped (butuh Phase 6)
- [x] Time-based analytics (weekly, monthly, yearly)

### Learning Goal

- Learn data aggregation & analytics
- Practice chart/graph libraries
- Understand business metrics

---

## 🎯 Phase 6: Payment Integration & Invoice Status

### Invoice Status Tracking

- [x] Add status field: Draft, Sent, Paid, Overdue, Cancelled — hybrid model: manual (Draft→Sent, Draft/Sent→Cancelled) + auto (Payments≥Total→Paid, due_date<today→Overdue)
- [x] Status history log (when status changes, by whom) — `status_history` table with audit trail
- [x] Filter invoices by status — `GET /api/invoices?status=Draft|Sent|Paid|Overdue|Cancelled`

### Payment Tracking

- [x] Record partial & full payments — `payments` table, auto-Paid trigger when SUM≥total
- [x] Payment date tracking — `payments.date` column
- [x] Payment method recording — `payments.method` free-text field

### Payment Gateway Integration (Optional) — deferred

- [ ] Integrate Stripe or similar for online payments
- [ ] Send payment links to clients
- [ ] Auto-update invoice status when paid

### Email Notifications — deferred

- [ ] Send invoice to client via email (PDF attachment)
- [ ] Payment reminder emails
- [ ] Overdue invoice notifications

### Learning Goal

- ✅ SQL state machines (hybrid manual/auto status transitions)
- ✅ Audit trail pattern (`status_history` table + FK to users)
- ✅ Computed state (Overdue derived at query time with `CASE WHEN`, never stored)
- ✅ Aggregation triggers (auto-Paid when `SUM(payments.amount) >= total_amount`)
- ✅ Dynamic query building in Go (status filter with optional WHERE clauses)
- ✅ Multi-user isolation extended to new resources (status changes, payment recording)
- Learn payment processing & security — deferred (Stripe sub-phase)
- Understand email sending services — deferred (email sub-phase)

---

## 🎯 Phase 7: Mobile App & Progressive Web App

### PWA Features

- [ ] Add service worker for offline support
- [ ] Installable app (add to home screen)
- [ ] Offline invoice creation (sync when online)

### Mobile Optimization

- [ ] Improve mobile UI/UX
- [ ] Touch-friendly form inputs
- [ ] Mobile-specific layouts

### Alternative: Native Mobile App

- [ ] Build with React Native or Flutter
- [ ] Sync with backend API

### Learning Goal

- Understand PWA architecture
- Learn offline-first design
- Practice mobile development

---

## 🎯 Phase 8: Deployment & DevOps

### Container & Orchestration

- [x] Improve Docker setup (optimize images, reduce size) — `.dockerignore`, multi-stage build verified, build context optimized
- [x] Setup Docker networking properly (fix current Podman issues) — explicit bridge network, `expose` vs `ports` pattern documented
- [x] Add health checks to services — `GET /api/health` w/ `db.Ping()`, Docker healthcheck on backend + frontend + postgres
- [x] Setup proper logging (stdout/stderr) — `log/slog` with JSON/text format, `LOG_FORMAT` + `LOG_LEVEL` env config, 43× `log.Printf` → `slog.Error`

### Cloud Deployment

- [x] ~~Deploy to cloud (AWS, GCP, Azure, or DigitalOcean)~~ — production setup READY: Caddyfile, docker-compose.prod.yml, deploy-production.sh, .env.prod.example, DEPLOYMENT_GUIDE.md. Tinggal provisioning VPS.
- [x] Setup CI/CD pipeline (GitHub Actions, GitLab CI, etc.) — `.github/workflows/ci.yml`: test-backend (Go + PostgreSQL service), test-frontend (tsc + vite build), build-and-push (Docker → GHCR)
- [x] Automated testing in pipeline — Go test runs in CI with real PostgreSQL service container, frontend tsc + vite build check
- [x] Database migrations in deployment — auto-migrate at startup via `golang-migrate`, documented in deploy script + DEPLOYMENT_GUIDE.md

### Monitoring & Logging

- [x] Add structured logging (slog, winston) — `log/slog` implemented, text for dev, JSON for prod, all 5 files migrated
- [x] Setup monitoring dashboard (Prometheus + Grafana) — 4 metric types (Counter, Histogram, Gauge), 7-panel dashboard, Grafana provisioning, docker-compose.monitoring.yml
- [x] Error tracking (Sentry, Rollbar) — Sentry Go SDK (`sentrygin` middleware) + Sentry React SDK (`ErrorBoundary` + `replayIntegration` with PII masking)
- [x] Uptime monitoring — documented: UptimeRobot/Better Uptime/Grafana Cloud setup in PHASE8_DEVOPS_LEARNING.md

### Learning Goal

- [x] Understand containerization best practices — multi-stage, health checks, `.dockerignore`, `expose` vs `ports`
- [x] Learn CI/CD principles — GitHub Actions, service containers, action pinning, layer caching, GHCR
- [x] Practice DevOps fundamentals — structured logging, monitoring, error tracking, production deployment architecture

### Docs Produced
- `docs/PHASE8_DEVOPS_LEARNING.md` — hands-on learning guide (all 6 steps with concept explanations)
- `docs/PHASE8_IMPLEMENTASI_DEVOPS.md` — learning summary (Problem → Arsitektur → 6 Konsep, format standar)
- `docs/DEPLOYMENT_GUIDE.md` — step-by-step VPS deployment guide

---

## 🎯 Phase 9: Testing & Code Quality

### Backend Testing

- [x] Unit tests for pure functions (`round2`, `calculateTotal`, password hashing, JWT generate/validate) — see `backend/logic_test.go`
- [x] Integration tests with test database (`invoicedb_test`) — auth, invoices (CRUD + isolation), clients, products, analytics, export smoke tests — see `backend/*_test.go`, 17 test functions, **67.0% coverage**
- [ ] End-to-end tests for full workflows — deferred (frontend testing phase)
- [x] Test coverage tracked via `go test ./... -cover` — realistic target 50-60% noted; achieved 67.0%. Export/PDF byte-level output intentionally not exhaustive-tested — see `docs/superpowers/specs/2026-07-16-phase9-backend-testing-design.md`

### Frontend Testing

- [x] Component unit tests (Vitest) — 3 test files, 13 tests: ApiError class, LoginPage (render + interactions), InvoicePreview (render + data display)
- [x] Integration tests (React Testing Library) — component tests pakai RTL render + fireEvent + userEvent, mock props pattern
- [ ] E2E tests (Playwright) — deferred

### Code Quality

- [x] Setup linting (ESLint, Prettier) — `eslint.config.js` (flat config) + `.prettierrc`, scripts: `lint`, `lint:fix`, `format`, `format:check`
- [x] Code formatting standards — Prettier: 2-space tab, semicolons, trailing commas, 100 char width
- [x] Pre-commit hooks — `lint-staged`: auto ESLint --fix + Prettier --write on staged `.ts/.tsx` files
- [ ] Code review process — deferred (team practice)

### Documentation

- [ ] API documentation (Swagger/OpenAPI) — deferred
- [x] Architecture documentation (ADR) — Phase 2-10 implementation docs in `docs/`
- [x] Setup guide & deployment guide — `docs/DEPLOYMENT_GUIDE.md`
- [x] Code comments for complex logic — all middleware + security modules documented

### Learning Goal

- [x] Understand testing strategies & best practices — ✅ table-driven tests, httptest, integration w/ real Postgres, `TestMain`, test isolation (TRUNCATE), multi-user isolation, SQL aggregation verification, smoke tests for binary output, **Vitest + React Testing Library component tests**
- [x] Learn TDD approach — frontend: write test first (ApiError), then component tests with mock props
- [x] Practice code quality standards — ESLint flat config, Prettier, lint-staged, CI integration

---

## 🎯 Phase 10: Performance & Security

### Performance

- [ ] Optimize database queries (indexes, caching) — indexes already well-optimized (11 indexes audited), query caching not yet
- [ ] Frontend optimization (code splitting, lazy loading) — deferred
- [ ] Caching strategy (Redis, browser cache) — deferred
- [ ] Load testing & performance benchmarks — deferred

### Security

- [x] Input validation on both frontend & backend — backend: `binding` tags on Invoice, InvoiceItem, SignupRequest, LoginRequest, PaymentRequest, SetStatusRequest. Frontend: deferred.
- [x] SQL injection prevention (parameterized queries) — all queries use `$1`, `$2` placeholders, no `fmt.Sprintf` for SQL
- [x] XSS prevention (Content Security Policy) — CSP header in Caddyfile: `default-src 'self'; script-src 'self'`
- [x] CSRF protection — not required: API uses JWT Bearer token (Authorization header), browser doesn't auto-attach. Standard for SPA + API architecture.
- [x] Rate limiting on API endpoints — per-IP token bucket middleware (`golang.org/x/time/rate`), 10 rps dev / 20 rps prod, whitelist for /api/health + /api/metrics
- [x] HTTPS/TLS setup — Caddy auto-SSL from Let's Encrypt (Phase 8), HSTS header enforced
- [x] Security headers (HSTS, X-Content-Type-Options, etc.) — 6 headers in Caddyfile: HSTS, CSP, X-Frame-Options, X-Content-Type-Options, Referrer-Policy, Permissions-Policy

### Learning Goal

- [x] Understand OWASP Top 10 vulnerabilities — covered: Injection (SQL), Broken Authentication (JWT), XSS (CSP), Rate Limiting, Security Headers
- [x] Learn secure coding practices — parameterized queries, input validation, security headers, rate limiting
- [ ] Practice security testing — deferred (Phase 9 frontend testing / ZAP scan)

---

## 📊 Priority & Difficulty Matrix

| Phase                      | Priority    | Difficulty | Est. Effort |
| -------------------------- | ----------- | ---------- | ----------- |
| Phase 1: Data Persistence  | 🔴 Critical | ⭐⭐       | 2-3 days    |
| Phase 2: Authentication    | 🔴 Critical | ⭐⭐⭐     | 3-5 days    |
| Phase 3: File Export       | 🟡 High     | ⭐⭐       | 2-3 days    |
| Phase 4: Advanced Features | 🟡 High     | ⭐⭐⭐     | 1-2 weeks   |
| Phase 5: Analytics         | 🟢 Medium   | ⭐⭐       | 3-5 days    |
| Phase 6: Payments          | 🟢 Medium   | ⭐⭐⭐⭐   | 1-2 weeks   |
| Phase 7: Mobile            | 🟢 Medium   | ⭐⭐⭐     | 2-3 weeks   |
| Phase 8: DevOps            | 🟢 Medium   | ⭐⭐⭐     | 1-2 weeks   |
| Phase 9: Testing           | 🟡 High     | ⭐⭐⭐     | 1-2 weeks   |
| Phase 10: Security         | 🔴 Critical | ⭐⭐⭐⭐   | 2-3 weeks   |

---

## 🚀 Quick Start for Next Steps

**Recommended order for learning:**

1. ✅ Start with **Phase 1 (Database)** - Essential foundation
2. Then **Phase 2 (Authentication)** - Needed for multi-user support
3. Then **Phase 3 (File Export)** - Quick win, nice feature
4. Then **Phase 9 (Testing)** - Build testing habits early
5. Then **Phase 10 (Security)** - Protect your app
6. Then other phases based on interests

---

## 📚 Learning Resources

### Go

- GORM (database ORM)
- golang-jwt (authentication)
- Gin middleware patterns

### React

- Context API for state management (or Redux for larger apps)
- React Router for multi-page navigation
- React Testing Library for testing

### DevOps

- Docker best practices
- Kubernetes basics
- GitHub Actions CI/CD

### Security

- OWASP Top 10
- Securecode.miraheze.org
- Port 2002 security course

---

## 💡 Tips

- Start simple, iterate on features
- Write tests alongside code (TDD approach)
- Always version your code & use meaningful commits
- Document decisions (Architecture Decision Records)
- Get feedback from other developers
- Consider user experience, not just features
- Focus on learning, not just shipping

---

**Happy Coding!** 🎉
