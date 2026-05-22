# Dev Container Setup

## Persyaratan

- VS Code
- Dev Container Extension (`ms-vscode-remote.remote-containers`)
- Docker Desktop atau Docker Engine
- Docker Compose

## Cara Menggunakan

### 1. Buka Project di Dev Container

- Buka folder project di VS Code
- Tekan `Ctrl+Shift+P` (atau `Cmd+Shift+P` di Mac)
- Cari dan pilih **"Dev Containers: Reopen in Container"**
- Tunggu sampai container terbuild dan siap (bisa 2-5 menit di pertama kali)

### 2. VS Code akan Secara Otomatis

- ✅ Install Go extensions
- ✅ Connect ke PostgreSQL container
- ✅ Set up environment variables
- ✅ Forward ports (8080 untuk API, 5432 untuk database)
- ✅ Install Air untuk hot reload

### 3. Menjalankan Backend

Ketika dev container sudah siap, backend akan langsung running dengan hot reload.

Untuk debugging atau menjalankan manual:

```bash
# Jalankan dengan hot reload (already running)
air

# Atau build & run manual
go run main.go

# Run migrations
migrate -path migrations -database "postgres://invoiceuser:invoicepassword@postgres:5432/invoicedb?sslmode=disable" up
```

### 4. Mengakses Services

| Service     | URL                                                             |
| ----------- | --------------------------------------------------------------- |
| Backend API | http://localhost:8080                                           |
| PostgreSQL  | postgres://invoiceuser:invoicepassword@localhost:5432/invoicedb |

### 5. Environment Variables

Semua env vars sudah dikonfigurasi di `.devcontainer/devcontainer.json`:

- `DB_HOST`: postgres
- `DB_PORT`: 5432
- `DB_USER`: invoiceuser
- `DB_PASSWORD`: invoicepassword
- `DB_NAME`: invoicedb
- `JWT_SECRET`: your-secret-key-change-in-production-min-32-chars
- `JWT_EXPIRATION`: 900

### 6. Troubleshooting

**Issue: "Cannot connect to PostgreSQL"**

```bash
# Check if postgres service is running
docker-compose ps

# Check postgres health
docker-compose logs postgres
```

**Issue: "Port already in use"**

```bash
# Kill existing container
docker-compose down

# Restart
docker-compose up -d
```

**Issue: Hot reload tidak berjalan**

- Cek file `.air.toml` di folder backend
- Pastikan file yang diedit termasuk dalam `include_ext`

### 7. Tips Pengembangan

1. **Debugging dengan Delve:**
   - VS Code sudah support debugging Go
   - Tekan `F5` untuk start debugger

2. **Database Migrations:**

   ```bash
   # Create new migration
   migrate create -ext sql -dir migrations -seq add_column_name

   # Run migrations
   migrate -path migrations -database postgres://... up
   ```

3. **Clean up:**

   ```bash
   # Keluar dari dev container
   # Tekan Ctrl+Shift+P → "Dev Containers: Reopen Folder Locally"

   # Hentikan containers
   docker-compose down

   # Remove all containers and volumes
   docker-compose down -v
   ```

## Architecture

```
Project Root
├── .devcontainer/
│   └── devcontainer.json      # Dev container config
├── backend/
│   ├── Dockerfile             # Multi-stage: dev & production
│   ├── .air.toml              # Air config untuk hot reload
│   ├── main.go
│   ├── migrations/
│   └── go.mod
├── frontend/
│   └── Dockerfile
└── docker-compose.yml         # Services: postgres, backend, frontend
```

## Notes

- Hot reload hanya untuk backend (Go dengan Air)
- Database tetap persistent walaupun container di-restart
- Semua file changes di-sync realtime antara host dan container
- VS Code extensions akan berjalan di dalam container
