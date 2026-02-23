# Invoice Maker

A simple, modern full-stack application for generating and managing invoices.

## Tech Stack

| Layer      | Technology                          |
|------------|-------------------------------------|
| Backend    | Go (Gin) – REST API                 |
| Frontend   | React 18 + TypeScript + Vite        |
| Styling    | Tailwind CSS                        |
| Tooling    | Bun (package manager & runtime)     |
| Infra      | Docker & Docker Compose             |

## Project Structure

```
invoice-maker/
├── backend/            # Go REST API
│   ├── main.go
│   ├── go.mod / go.sum
│   └── Dockerfile
├── frontend/           # React + Vite + Tailwind
│   ├── src/
│   │   ├── App.tsx
│   │   ├── main.tsx
│   │   ├── components/
│   │   │   ├── InvoiceForm.tsx
│   │   │   └── InvoicePreview.tsx
│   │   └── types/invoice.ts
│   ├── index.html
│   ├── package.json
│   ├── vite.config.ts
│   ├── tailwind.config.js
│   ├── nginx.conf
│   └── Dockerfile
└── docker-compose.yml
```

## Running the Application

### With Docker Compose (recommended)

```bash
docker compose up --build
```

- Frontend: http://localhost:80
- Backend API: http://localhost:8080

### Local Development

**Backend:**
```bash
cd backend
go run .
```

**Frontend:**
```bash
cd frontend
bun install
bun run dev
```

## API Endpoints

| Method | Path                  | Description          |
|--------|-----------------------|----------------------|
| GET    | /api/invoices         | List all invoices    |
| GET    | /api/invoices/:id     | Get invoice by ID    |
| POST   | /api/invoices         | Create new invoice   |
| PUT    | /api/invoices/:id     | Update invoice       |
| DELETE | /api/invoices/:id     | Delete invoice       |

### Invoice Model

```json
{
  "id": "INV-20240101-0001",
  "client_name": "Acme Corp",
  "date": "2024-01-01",
  "items": [
    { "description": "Web Design", "qty": 1, "price": 500.00 }
  ],
  "tax_rate": 10,
  "total_amount": 550.00
}
```
