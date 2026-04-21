# RntPuls

RentPulse is a production-oriented rent collection platform for small landlords and agencies. It includes a Go backend, React frontend, Postgres persistence, Docker orchestration, and GitHub Actions CI.

## What is implemented

- Landing page, authentication, role-bearing JWT context, dashboard, tenants, pricing, and settings screens.
- Go API with Postgres-backed organizations, users, properties, units, tenants, leases, payment intents, confirmations, communications, import jobs, and organization settings.
- Tenant CSV/XLSX imports, tenant CSV exports, and monthly Excel reports.
- Payment marking and landlord verification workflow, with optional M-Pesa transaction-status verification when Daraja credentials are configured.
- SMS reminder execution through Twilio when credentials are configured; skipped sends are recorded when the provider is not configured.
- Docker Compose for API, web, and Postgres.
- GitHub Actions CI for backend tests, frontend build, and Docker build.

## Run locally

```bash
cp .env.example .env
docker compose up --build
```

Open `http://localhost:5173`.

## Development

Backend API runs on `http://localhost:8080`.

Frontend expects `VITE_API_URL`, defaulting to `http://localhost:8080/api`.

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
