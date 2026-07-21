# 🚀 Deployment Guide — Invoice Maker

Panduan langkah demi langkah untuk deploy Invoice Maker ke VPS production.

---

## Prasyarat

- **VPS** dengan OS Ubuntu 22.04/24.04 (minimal 1GB RAM)
  - Rekomendasi: DigitalOcean $6/month, Vultr $6/month, atau Hetzner ~€4/month
  - Atau gratis: Oracle Cloud Free Tier (4-core ARM, 24GB RAM)
- **Domain** (opsional tapi recommended)
  - Bisa beli di Cloudflare ($10/year) atau pakai gratis: duckdns.org
- **Docker** terinstall di VPS

---

## Step 1: Setup VPS

```bash
# SSH ke VPS
ssh root@<IP_VPS>

# Install Docker (official script)
curl -fsSL https://get.docker.com | sh

# Tambah user kamu ke docker group (biar gak perlu sudo)
usermod -aG docker $USER
# Logout & login lagi

# Verifikasi
docker --version
docker compose version
```

---

## Step 2: Clone project + setup env

```bash
# Clone repository
git clone https://github.com/TamaSkuyy/invoice-maker.git
cd invoice-maker

# Copy dan isi environment production
cp .env.prod.example .env.prod
nano .env.prod
```

Isi `.env.prod`:

```bash
DOMAIN=invoice.example.com        # Ganti dengan domain/ IP kamu
DB_USER=invoiceuser
DB_PASSWORD=<password-kuat-20-char>
DB_NAME=invoicedb
JWT_SECRET=<random-string-min-32-char>
JWT_EXPIRATION=900
GIT_SHA=$(git rev-parse HEAD)
SENTRY_DSN=                       # Opsional, isi kalau pakai Sentry
```

---

## Step 3: Setup DNS (kalau pakai domain)

Arahkan DNS A record ke IP VPS:

```
Type:  A
Name:  @  (atau invoice)
Value: <IP_VPS>
TTL:   Auto
```

Cek propagasi DNS:

```bash
dig invoice.example.com +short
# Harus return IP VPS kamu
```

> **Tanpa domain:** Caddy bisa pakai IP publik + nip.io. Ganti `DOMAIN` jadi `<IP>.nip.io`. Contoh: `DOMAIN=167.99.123.45.nip.io`

---

## Step 4: Deploy

```bash
# Deploy pertama kali
./deploy-production.sh

# Script akan:
# 1. Validasi .env.prod
# 2. Build Docker images
# 3. Start semua service (Caddy, backend, frontend, postgres)
# 4. Tunggu backend healthy
# 5. Tampilkan URL akses

# Cek status
./deploy-production.sh --logs
```

---

## Step 5: Verifikasi

```bash
# Health check
curl https://invoice.example.com/api/health

# Harus return:
# {"status":"healthy","time":"..."}

# Akses frontend: buka https://invoice.example.com di browser
```

---

## Perintah Berguna

```bash
# Cek status semua container
docker compose -f docker-compose.prod.yml ps

# Lihat logs real-time
./deploy-production.sh --logs

# Update aplikasi (pull + rebuild + restart)
./deploy-production.sh --update

# Force rebuild dari nol
./deploy-production.sh --build

# Stop production
./deploy-production.sh --down

# Restart
docker compose -f docker-compose.prod.yml restart

# Cek SSL certificate Caddy
docker exec invoice-caddy-prod ls /data/caddy/certificates/

# Backup database
docker exec invoice-postgres-prod pg_dump -U invoiceuser invoicedb > backup.sql
```

---

## Arsitektur Production

```
                     Internet
                        │
                  ┌─────▼─────┐
                  │   Caddy   │  ← Auto SSL (Let's Encrypt)
                  │  :80 :443 │  ← Satu-satunya yg exposed
                  └─────┬─────┘
                        │
            ┌───────────┼───────────┐
            │           │           │
      ┌─────▼─────┐ ┌──▼───┐ ┌─────▼─────┐
      │ Frontend  │ │ API  │ │ Internal  │
      │ (nginx)   │ │ path │ │ Docker    │
      │   :80     │ │      │ │ Network   │
      └───────────┘ └──┬───┘ └───────────┘
                       │
                 ┌─────▼─────┐
                 │  Backend  │
                 │   :8080   │
                 └─────┬─────┘
                       │
                 ┌─────▼─────┐
                 │ PostgreSQL│
                 │   :5432   │
                 └───────────┘
```

---

## Keamanan Checklist

- [ ] Ganti semua password default di `.env.prod`
- [ ] JWT_SECRET minimal 32 karakter random
- [ ] Database TIDAK exposed ke internet (pakai `expose`, bukan `ports`)
- [ ] Firewall VPS: hanya port 80 & 443 yang terbuka
- [ ] SSH key-based auth (disable password login)
- [ ] Setup unattended-upgrades untuk security patches

```bash
# Setup firewall (UFW)
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 22/tcp
ufw enable
ufw status
```

---

## Troubleshooting

**Caddy gagal dapat SSL certificate:**
- Pastikan port 80 & 443 terbuka di firewall
- Pastikan DNS A record sudah mengarah ke IP VPS
- Cek log: `docker logs invoice-caddy-prod`

**Backend gagal konek database:**
```bash
docker compose -f docker-compose.prod.yml logs backend
# Cari "failed to initialize database"
```

**Frontend blank screen:**
- Cek nginx config: `docker exec invoice-frontend-prod cat /etc/nginx/conf.d/default.conf`
- Pastikan backend healthy: `curl https://DOMAIN/api/health`

**Performa lambat:**
```bash
# Cek resource usage
docker stats
# Cek koneksi database
docker exec invoice-postgres-prod psql -U invoiceuser -d invoicedb \
  -c "SELECT count(*) FROM pg_stat_activity;"
```
