# RntPuls

RentPulse is a production-oriented rent collection platform for small landlords and agencies. It includes a Go backend, React frontend, Postgres persistence, Docker orchestration, and GitHub Actions CI.

## What is implemented

- Landing page, authentication, role-bearing JWT context, dashboard, tenants, pricing, and settings screens.
- Go API with Postgres-backed organizations, users, properties, units, tenants, leases, payment intents, confirmations, communications, import jobs, and organization settings.
- Tenant CSV/XLSX imports, tenant CSV exports, and monthly Excel reports.
- Payment marking and landlord verification workflow, with optional M-Pesa transaction-status verification when Daraja credentials are configured.
- Tenant portal links for tenants to upload screenshots/PDF evidence and submit payment references without landlord account access.
- Tenant import template CSV plus an import wizard that previews files, maps spreadsheet columns, validates rows, and imports compatible CSV/XLSX files.
- Sample agency Excel workbooks live in `sample-data/imports/`: one master import workbook and one workbook per property, each with more than 50 units and mixed bank/M-Pesa paybill payment routes.
- Main app navigation: Dashboard, Tenants, Payments, and Settings. Pricing lives inside Settings.
- Plan-aware request throttling and unit limits: Free/Starter up to 2 units, Pro up to 10 units, Agency unlimited.
- SMS reminder execution through Twilio when credentials are configured; skipped sends are recorded when the provider is not configured.
- Docker Compose for API, web, and Postgres.
- GitHub Actions CI for backend tests, frontend build, and Docker build.

## Production features

- Versioned migrations with `schema_migrations` tracking table
- Refresh token rotation for secure session management
- Login attempt tracking with brute-force protection (5 attempts per 15 minutes)
- DB-backed sliding window rate limiter (scales across instances)
- Request ID middleware for tracing
- Structured JSON request logging with duration and status
- Prometheus-compatible `/metrics` endpoint
- Non-root Docker containers with health checks
- Resource limits in Docker Compose
- Automated backup script with rotation
- CORS with origin validation
- Panic recovery with request ID correlation

## Run locally

```bash
cp .env.example .env
docker compose up --build
```

Open `http://localhost:5173`.

## Development

Backend API runs on `http://localhost:8080`.

Frontend expects `VITE_API_URL`, defaulting to `http://localhost:8080/api`.

### Make targets

```bash
make test          # Run unit tests with race detection
make test-coverage # Run tests with coverage report
make lint          # Run go vet
make build         # Build binary
make run           # Run locally
make docker-up     # Start Docker Compose
make docker-down   # Stop Docker Compose
```

## Provider configuration

Set Twilio variables for live SMS delivery:

```bash
TWILIO_ACCOUNT_SID=
TWILIO_AUTH_TOKEN=
TWILIO_FROM_SMS=
```

Set M-Pesa Daraja variables for live transaction reference checks:

```bash
MPESA_BASE_URL=https://sandbox.safaricom.co.ke
MPESA_CONSUMER_KEY=
MPESA_CONSUMER_SECRET=
MPESA_SHORT_CODE=
```

## Tenant access

Landlords generate a tenant portal link from the tenant directory. The link opens `/tenant?token=...`, where the tenant can upload a JPG, PNG, or PDF proof of payment and submit the transaction reference. That submission creates a pending confirmation for landlord verification.

## Production deployment

Generate secrets before deploying:

```bash
openssl rand -base64 32  # JWT_SECRET
openssl rand -base64 32  # DB_PASSWORD
```

Set environment variables and deploy:

```bash
docker compose -f docker-compose.prod.yml up -d --build
```

### Backups

The backup script runs pg_dump with gzip compression and rotates backups older than 7 days:

```bash
BACKUP_DIR=/backups DB_PASSWORD=your_password ./scripts/backup.sh
```
