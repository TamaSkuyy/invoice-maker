#!/usr/bin/env bash
# =============================================================================
# Invoice Maker – Production Deployment Script
#
# Build dan deploy production stack dengan Docker Compose.
#
# Usage:
#   ./deploy-production.sh                 # Build & deploy
#   ./deploy-production.sh --build         # Force rebuild tanpa cache
#   ./deploy-production.sh --update        # Quick update (pull code, rebuild, restart)
#   ./deploy-production.sh --down          # Stop production stack
#   ./deploy-production.sh --logs          # Tail logs semua service
# =============================================================================
set -euo pipefail

GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[0;33m'
CYAN='\033[0;36m'
NC='\033[0m'

PROJECT_NAME="invoice-maker-prod"
COMPOSE_FILE="docker-compose.prod.yml"
ENV_FILE=".env.prod"

log()     { echo -e "${GREEN}[DEPLOY]${NC} $*"; }
warn()    { echo -e "${YELLOW}[WARN]${NC} $*"; }
err()     { echo -e "${RED}[ERROR]${NC} $*"; }
success() { echo -e "${GREEN}[OK]${NC} $*"; }

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
log "Container runtime: ${RUNTIME}"

dc() {
  ${COMPOSE_CMD} -p "${PROJECT_NAME}" -f "${COMPOSE_FILE}" --env-file "${ENV_FILE}" "$@"
}

# ── Parse arguments ───────────────────────────────────────────────────────
FORCE_BUILD=false
ACTION="deploy"

for arg in "$@"; do
  case "$arg" in
    --build)   FORCE_BUILD=true ;;
    --update)  ACTION="update" ;;
    --down)    ACTION="down" ;;
    --logs)    ACTION="logs" ;;
    --help|-h)
      echo ""
      echo -e "${CYAN}Usage: $0 [OPTIONS]${NC}"
      echo ""
      echo -e "  ${GREEN}(default)${NC}       Build & deploy production stack"
      echo -e "  ${GREEN}--build${NC}         Force rebuild semua image tanpa cache"
      echo -e "  ${GREEN}--update${NC}        Quick update: git pull + rebuild + restart"
      echo -e "  ${GREEN}--down${NC}          Stop production stack"
      echo -e "  ${GREEN}--logs${NC}          Tail logs semua service"
      echo ""
      exit 0
      ;;
    *)
      err "Unknown option: $arg"
      exit 1
      ;;
  esac
done

# ── Pre-flight: env file ──────────────────────────────────────────────────
check_env() {
  if [ ! -f "${ENV_FILE}" ]; then
    err "File ${ENV_FILE} tidak ditemukan!"
    echo ""
    echo "Buat file .env.prod dengan isi:"
    echo "  DB_USER=invoiceuser"
    echo "  DB_PASSWORD=<password-kuat>"
    echo "  DB_NAME=invoicedb"
    echo "  JWT_SECRET=<random-min-32-characters>"
    echo "  JWT_EXPIRATION=900"
    echo "  BACKEND_PORT=8080"
    echo "  FRONTEND_PORT=3000"
    exit 1
  fi

  source "${ENV_FILE}"

  if [ -z "${JWT_SECRET:-}" ] || [ "${#JWT_SECRET}" -lt 32 ]; then
    err "JWT_SECRET di .env.prod harus diisi minimal 32 karakter!"
    exit 1
  fi

  if [ -z "${DB_PASSWORD:-}" ] || [ "$DB_PASSWORD" = "invoicepassword" ]; then
    warn "DB_PASSWORD masih default! Ganti dengan password yang kuat di ${ENV_FILE}"
  fi

  log "Environment file: ${ENV_FILE} OK"
}

# ── Stop ──────────────────────────────────────────────────────────────────
if [ "$ACTION" = "down" ]; then
  log "Menghentikan production stack..."
  dc down
  log "Production stack dihentikan."
  exit 0
fi

# ── Logs ──────────────────────────────────────────────────────────────────
if [ "$ACTION" = "logs" ]; then
  exec dc logs -f
fi

# ── Quick update ──────────────────────────────────────────────────────────
if [ "$ACTION" = "update" ]; then
  check_env

  echo -e "${CYAN}"
  echo "========================================"
  echo "  Invoice Maker – Quick Update"
  echo "========================================"
  echo -e "${NC}"

  log "Pull latest code..."
  git pull

  log "Rebuild images dengan cache..."
  dc build

  log "Rolling restart services..."
  dc up -d --no-deps frontend 2>/dev/null || true
  dc up -d --no-deps backend

  log "Menunggu backend healthy..."
  for i in $(seq 1 24); do
    STATUS=$(dc ps backend --format '{{.Health}}' 2>/dev/null || echo "starting")
    if [ "$STATUS" = "healthy" ]; then
      success "Backend healthy!"
      break
    fi
    if [ "$i" = "24" ]; then
      err "Backend belum healthy. Cek logs: ./deploy-production.sh --logs"
      dc logs backend --tail=20 2>&1 || true
      exit 1
    fi
    echo -n "."
    sleep 5
  done
  echo

  # Migrations run automatically on backend startup via main.go
  log "Migrasi database berjalan otomatis saat backend start."

  success "Quick update selesai!"
  echo ""
  dc ps
  exit 0
fi

# ── Full deploy ───────────────────────────────────────────────────────────
check_env

echo -e "${CYAN}"
echo "========================================"
echo "  Invoice Maker – Production Deploy"
echo "========================================"
echo -e "${NC}"

# Build
if [ "$FORCE_BUILD" = true ]; then
  warn "Force rebuild semua image (--no-cache)..."
  dc build --no-cache
else
  log "Building images dengan layer cache..."
  dc build
fi

# Start
log "Menjalankan production stack..."
dc up -d

# Wait for backend health
log "Menunggu backend healthy..."
for i in $(seq 1 30); do
  STATUS=$(dc ps backend --format '{{.Health}}' 2>/dev/null || echo "starting")
  if [ "$STATUS" = "healthy" ]; then
    success "Backend healthy!"
    break
  fi
  if [ "$i" = "30" ]; then
    err "Backend belum healthy setelah 30 percobaan."
    echo ""
    warn "=== Container Status ==="
    dc ps
    echo ""
    warn "=== Backend Logs (last 30 lines) ==="
    dc logs backend --tail=30 2>&1 || true
    exit 1
  fi
  echo -n "."
  sleep 5
done
echo

# ── Summary ───────────────────────────────────────────────────────────────
BACKEND_PORT="${BACKEND_PORT:-8080}"
FRONTEND_PORT="${FRONTEND_PORT:-3000}"

echo
echo -e "${CYAN}========================================${NC}"
success "Deployment selesai!"
echo -e "${CYAN}========================================${NC}"
echo
echo -e "  Frontend  : ${GREEN}http://SERVER_IP:${FRONTEND_PORT}${NC}"
echo -e "  Backend   : ${GREEN}http://SERVER_IP:${BACKEND_PORT}${NC}"
echo -e "  Database  : ${GREEN}postgres:5432${NC}"
echo
echo "  Perintah berguna:"
echo "    Logs    : ./deploy-production.sh --logs"
echo "    Status  : ${COMPOSE_CMD} -p ${PROJECT_NAME} -f ${COMPOSE_FILE} ps"
echo "    Stop    : ./deploy-production.sh --down"
echo "    Update  : ./deploy-production.sh --update"
echo "    Rebuild : ./deploy-production.sh --build"
echo

dc ps
