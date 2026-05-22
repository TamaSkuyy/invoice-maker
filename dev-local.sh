#!/usr/bin/env bash
# =============================================================================
# Invoice Maker – Local Development Script
#
# Menjalankan PostgreSQL di container, backend Go + frontend React di host.
#
# Usage:
#   ./dev-local.sh                 # Start full dev environment
#   ./dev-local.sh --init          # Install deps + run setup
#   ./dev-local.sh --no-infra      # Skip PostgreSQL container
#   ./dev-local.sh --stop          # Stop all processes & containers
#   ./dev-local.sh go [args...]    # Run go command in backend/
#   ./dev-local.sh npm [args...]   # Run npm command in frontend/
#   ./dev-local.sh psql [args...]  # Connect to PostgreSQL
# =============================================================================
set -euo pipefail

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
CYAN='\033[0;36m'
NC='\033[0m'

log()  { echo -e "${GREEN}[dev-local]${NC} $*"; }
warn() { echo -e "${YELLOW}[warn]${NC} $*"; }
err()  { echo -e "${RED}[error]${NC} $*"; }

usage() {
  cat <<'EOF'
Usage:
  ./dev-local.sh [--init] [--no-infra]
  ./dev-local.sh --stop
  ./dev-local.sh go [args...]
  ./dev-local.sh npm [args...]
  ./dev-local.sh psql [args...]

Options:
  --init       Install dependencies (go mod download, npm install)
  --no-infra   Skip starting PostgreSQL container
  --stop       Stop all containers and processes
  -h, --help   Show this help

Default behavior:
  1) Start PostgreSQL via docker/podman compose
  2) Run Go backend with air (hot reload) on :8080
  3) Run Vite dev server (frontend) on :5173
  4) Stop all processes automatically on Ctrl+C
EOF
}

INIT=false
NO_INFRA=false
STOP_ONLY=false
INFRA_STARTED=false

# ── Container runtime detection ───────────────────────────────────────────
if command -v podman >/dev/null 2>&1; then
  RUNTIME="podman"
  COMPOSE_CMD="podman compose"
elif command -v docker >/dev/null 2>&1; then
  RUNTIME="docker"
  COMPOSE_CMD="docker compose"
else
  err "Docker atau Podman tidak ditemukan."
  exit 1
fi

COMPOSE_FILE="docker-compose.yml"
PROJECT_NAME="invoice-maker-dev"

dc() {
  ${COMPOSE_CMD} -p "${PROJECT_NAME}" -f "${COMPOSE_FILE}" "$@"
}

# ── Command Proxying ──────────────────────────────────────────────────────
if [ "$#" -gt 0 ]; then
  case "$1" in
    go)
      shift
      (cd backend && exec go "$@")
      ;;
    npm|npx)
      CMD="$1"
      shift
      (cd frontend && exec "$CMD" "$@")
      ;;
    psql)
      shift
      if dc ps -q postgres >/dev/null 2>&1; then
        exec dc exec -it postgres psql -U invoiceuser -d invoicedb "$@"
      else
        err "PostgreSQL container tidak berjalan. Jalankan dev-local dulu."
        exit 1
      fi
      ;;
  esac
fi

# ── Parse options ─────────────────────────────────────────────────────────
for arg in "$@"; do
  case "$arg" in
    --init)     INIT=true ;;
    --no-infra) NO_INFRA=true ;;
    --stop)     STOP_ONLY=true ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      err "Unknown option: $arg"
      usage
      exit 1
      ;;
  esac
done

# ── Stop ──────────────────────────────────────────────────────────────────
if [ "$STOP_ONLY" = true ]; then
  log "Stopping all processes..."
  dc stop postgres 2>/dev/null || true
  dc down --remove-orphans 2>/dev/null || true
  pkill -f "air"               2>/dev/null || true
  pkill -f "go run"            2>/dev/null || true
  pkill -f "vite"              2>/dev/null || true
  pkill -f "invoice-backend"   2>/dev/null || true
  log "All stopped."
  exit 0
fi

# ── Init ──────────────────────────────────────────────────────────────────
if [ "$INIT" = true ]; then
  log "Running initial setup..."

  log "go mod download (backend)..."
  (cd backend && go mod download)

  if [ ! -d "frontend/node_modules" ]; then
    log "npm install (frontend)..."
    (cd frontend && npm install)
  else
    log "frontend/node_modules already exists, skip npm install."
  fi

  log "Init selesai. Backend akan auto-migrate database saat pertama kali berjalan."
fi

# ── Pre-flight checks ────────────────────────────────────────────────────
if [ ! -f "backend/go.sum" ]; then
  err "backend/go.sum tidak ditemukan. Jalankan: ./dev-local.sh --init"
  exit 1
fi

if [ ! -d "frontend/node_modules" ]; then
  err "frontend/node_modules tidak ditemukan. Jalankan: ./dev-local.sh --init"
  exit 1
fi

# ── Resolve Go hot-reload tool ───────────────────────────────────────────
resolve_go_runner() {
  if command -v air >/dev/null 2>&1; then
    echo "air"
    return 0
  fi

  if [ -f "$HOME/go/bin/air" ]; then
    echo "$HOME/go/bin/air"
    return 0
  fi

  warn "air tidak ditemukan. Menginstall air..."
  go install github.com/air-verse/air@latest

  if [ -f "$HOME/go/bin/air" ]; then
    echo "$HOME/go/bin/air"
    return 0
  fi

  warn "Gagal install air, fallback ke 'go run .' (tanpa hot reload)."
  echo "go run"
  return 0
}

GO_RUNNER="$(resolve_go_runner)"

# ── Start infrastructure ──────────────────────────────────────────────────
if [ "$NO_INFRA" = false ]; then
  log "Starting PostgreSQL with ${RUNTIME}..."
  dc up -d postgres
  INFRA_STARTED=true

  log "Waiting for PostgreSQL to be ready..."
  for i in $(seq 1 30); do
    if dc exec -T postgres pg_isready -U invoiceuser -d invoicedb >/dev/null 2>&1; then
      log "PostgreSQL ready!"
      break
    fi
    sleep 2
  done
else
  warn "Skipping infra startup (--no-infra). Pastikan PostgreSQL sudah berjalan."
fi

# ── Cleanup on exit ───────────────────────────────────────────────────────
cleanup() {
  warn "Stopping app processes..."
  if [ -n "${BACKEND_PID:-}" ] && kill -0 "$BACKEND_PID" 2>/dev/null; then
    kill "$BACKEND_PID" 2>/dev/null || true
    wait "$BACKEND_PID" 2>/dev/null || true
  fi
  if [ -n "${VITE_PID:-}" ] && kill -0 "$VITE_PID" 2>/dev/null; then
    kill "$VITE_PID" 2>/dev/null || true
    wait "$VITE_PID" 2>/dev/null || true
  fi
  wait 2>/dev/null || true

  if [ "$INFRA_STARTED" = true ]; then
    log "Stopping PostgreSQL container..."
    dc stop postgres 2>/dev/null || true
    dc down --remove-orphans 2>/dev/null || true
  fi

  log "Stopped."
}

trap cleanup INT TERM EXIT

# ── Start backend ─────────────────────────────────────────────────────────
log "Starting Go backend on http://localhost:8080 ..."

if [ "$GO_RUNNER" = "go run" ]; then
  (cd backend && go run .) &
else
  (cd backend && "$GO_RUNNER" -c .air.toml) &
fi
BACKEND_PID=$!

# ── Start frontend ────────────────────────────────────────────────────────
log "Starting Vite dev server on http://localhost:5173 ..."
(cd frontend && npm run dev) &
VITE_PID=$!

# ── Info ──────────────────────────────────────────────────────────────────
echo -e "${CYAN}----------------------------------------------${NC}"
echo -e "${CYAN}Backend     : http://localhost:8080${NC}"
echo -e "${CYAN}Frontend    : http://localhost:5173${NC}"
echo -e "${CYAN}PostgreSQL  : localhost:5432 (invoiceuser/invoicedb)${NC}"
echo -e "${CYAN}Press Ctrl+C to stop all processes${NC}"
echo -e "${CYAN}----------------------------------------------${NC}"

wait -n "$BACKEND_PID" "$VITE_PID" 2>/dev/null || true
