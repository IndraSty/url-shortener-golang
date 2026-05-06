# ⚡ URL Shortener — Enterprise Grade

<div align="center">

![Go](https://img.shields.io/badge/Go-1.22-00ADD8?style=for-the-badge&logo=go&logoColor=white)
![Echo](https://img.shields.io/badge/Echo-v4-00ADD8?style=for-the-badge&logo=go&logoColor=white)
![PostgreSQL](https://img.shields.io/badge/PostgreSQL-Neon.tech-4169E1?style=for-the-badge&logo=postgresql&logoColor=white)
![Redis](https://img.shields.io/badge/Redis-Upstash-DC382D?style=for-the-badge&logo=redis&logoColor=white)
![Fly.io](https://img.shields.io/badge/Deploy-Fly.io-7B3FE4?style=for-the-badge&logo=flydotio&logoColor=white)
![Prometheus](https://img.shields.io/badge/Metrics-Prometheus-E6522C?style=for-the-badge&logo=prometheus&logoColor=white)
![Grafana](https://img.shields.io/badge/Dashboard-Grafana-F46800?style=for-the-badge&logo=grafana&logoColor=white)
![License](https://img.shields.io/badge/License-MIT-green?style=for-the-badge)

**Enterprise URL Shortener with sub-10ms redirect latency, A/B testing, geo-targeting, real-time analytics, and QR code generation — built for scale.**

[Features](#-features) · [Architecture](#-architecture) · [API Docs](#-api-endpoints) · [Getting Started](#-getting-started) · [Deploy](#-deploy-to-flyio) · [Benchmarks](#-benchmarks)

</div>

---

## ✨ Features

| Feature | Description |
|---|---|
| ⚡ **Sub-10ms Redirect** | Redis cache-first architecture — PostgreSQL only as fallback |
| 🧪 **A/B Testing** | Multiple destinations with weighted traffic distribution per link |
| 🌍 **Geo Targeting** | Country-specific redirects via ip-api.com with Redis caching |
| 📊 **Real-time Analytics** | Click events processed async via QStash — never blocks redirect |
| 🔐 **Password Protection** | bcrypt-hashed password per link |
| ⏰ **Link Expiration** | Set expiry date per link — returns generic 404 after expiry |
| 📱 **QR Code Generator** | Server-side PNG generation, pure Go, no external service |
| 🔑 **Dual Auth** | JWT Bearer token + SHA-256 hashed API key |
| 🛡️ **Rate Limiting** | Per-IP on redirect, per-user on management API |
| 🔒 **Security Headers** | HSTS, CSP, X-Frame-Options, and more on every response |
| 🔏 **GDPR Compliant** | IP masking (x.x.0.0), domain-only referrer, no plaintext secrets |
| 📈 **Prometheus Metrics** | Full observability with Grafana Cloud dashboard |

---

## 🏗️ Architecture

### System Overview

![System overflow image](https://github.com/IndraSty/url-shortener-golang/blob/main/system-overflow.png)

---

### Redirect Hot Path — Sub-10ms Flow

> This is the most performance-critical path. Every decision is made in order and exits as early as possible.

```
  GET /:slug
      │
      ▼
  ┌─────────────────────────┐
  │   Redis Cache Lookup    │──── HIT ────────────────────────────────────┐
  └───────────┬─────────────┘                                             │
              │ MISS                                                       │
              ▼                                                            │
  ┌─────────────────────────┐                                             │
  │  PostgreSQL Lookup      │──── NOT FOUND ──► 404 (generic)            │
  └───────────┬─────────────┘                                             │
              │ FOUND                                                      │
              ▼                                                            │
  ┌─────────────────────────┐                                             │
  │  Store in Redis (async) │                                             │
  └───────────┬─────────────┘                                             │
              │                                                            │
              └────────────────────────────────────────────────────────── ┘
                                                                           │
                                                                           ▼
                                                              ┌────────────────────┐
                                                              │   Expiry Check     │──► 404 (generic)
                                                              └────────┬───────────┘
                                                                       │ not expired
                                                                       ▼
                                                              ┌────────────────────┐
                                                              │   Active Check     │──► 404
                                                              └────────┬───────────┘
                                                                       │ active
                                                                       ▼
                                                              ┌────────────────────┐
                                                              │  Password Check    │──► 401
                                                              └────────┬───────────┘
                                                                       │ ok / no password
                                                                       ▼
                                                              ┌────────────────────┐
                                                              │  Geo Rule Match    │──► 302 (geo dest)
                                                              └────────┬───────────┘
                                                                       │ no match
                                                                       ▼
                                                              ┌────────────────────┐
                                                              │  A/B Selection     │──► 302 (variant)
                                                              └────────┬───────────┘
                                                                       │ no A/B
                                                                       ▼
                                                              ┌────────────────────┐
                                                              │  301 Redirect      │
                                                              │  (destination_url) │
                                                              └────────┬───────────┘
                                                                       │
                                                                       ▼ (async, non-blocking)
                                                              ┌────────────────────┐
                                                              │  Publish to QStash │
                                                              └────────┬───────────┘
                                                                       │
                                                                       ▼
                                                              ┌────────────────────┐
                                                              │  QStash → POST     │
                                                              │  /internal/ingest  │
                                                              └────────┬───────────┘
                                                                       │
                                                                       ▼
                                                              ┌────────────────────┐
                                                              │  Worker: Enrich +  │
                                                              │  Persist to DB     │
                                                              │  (geo, UA, IP mask)│
                                                              └────────────────────┘
```

---

### Clean Architecture — Layer Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                      DELIVERY LAYER                              │
│                                                                  │
│   handler/          middleware/           router.go              │
│   auth_handler      auth.go              Routes all endpoints    │
│   link_handler      ratelimit.go         Wires middleware chains │
│   redirect_handler  security.go                                  │
│   analytics_handler metrics.go                                   │
│   worker_handler    logger.go                                    │
│   health_handler                                                 │
│                                                                  │
│   Depends on: domain interfaces only                             │
└────────────────────────┬────────────────────────────────────────┘
                         │ calls interfaces
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                      USECASE LAYER                               │
│                                                                  │
│   auth_usecase.go       Business logic for auth + JWT            │
│   link_usecase.go       CRUD, QR, A/B test, geo rule mgmt        │
│   redirect_usecase.go   Hot path: cache → db → decide → publish  │
│   analytics_usecase.go  Query + ProcessClickEvent enrichment      │
│                                                                  │
│   Depends on: domain interfaces only                             │
└────────────────────────┬────────────────────────────────────────┘
                         │ implements interfaces
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                      DOMAIN LAYER                                │
│                                                                  │
│   user.go          User entity + UserRepository + AuthUsecase    │
│   link.go          Link/ABTest/GeoRule entities + interfaces     │
│   analytics.go     ClickEvent entity + interfaces                │
│   redirect.go      RedirectResult + CacheRepository interface    │
│   errors.go        All sentinel errors                           │
│                                                                  │
│   Depends on: NOTHING (pure Go stdlib only)                      │
└────────────────────────┬────────────────────────────────────────┘
                         │ implemented by
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                    REPOSITORY LAYER                              │
│                                                                  │
│   postgres/                    redis/                            │
│   ├── user_repo.go             ├── cache_repo.go                 │
│   ├── link_repo.go             └── client.go                     │
│   ├── ab_geo_repo.go                                             │
│   ├── analytics_repo.go        Implements CacheRepository        │
│   ├── migrate.go               - GetLink / SetLink / DeleteLink  │
│   └── helpers.go               - IncrRateLimit                   │
│                                                                  │
│   Depends on: domain entities only                               │
└─────────────────────────────────────────────────────────────────┘
```

---

### Infrastructure & Services

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         PRODUCTION INFRASTRUCTURE                            │
│                                                                              │
│  ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────────────┐  │
│  │   FLY.IO        │    │   NEON.TECH      │    │   UPSTASH               │  │
│  │                 │    │                  │    │                         │  │
│  │  Edge Deploy    │    │  PostgreSQL      │    │  ┌─────────────────┐    │  │
│  │  Region: sin    │◄──►│  Serverless      │    │  │  Redis          │    │  │
│  │  512MB RAM      │    │  Auto-scale      │    │  │  Cache layer    │    │  │
│  │  Shared CPU     │    │  SSL enforced    │    │  │  Rate limiting  │    │  │
│  │  Force HTTPS    │    │  Free tier       │    │  │  Geo IP cache   │    │  │
│  │  Auto-start     │    │                  │    │  └─────────────────┘    │  │
│  │                 │    │  Tables:         │    │                         │  │
│  │  /health ──────►│    │  users           │    │  ┌─────────────────┐    │  │
│  │  /livez  ──────►│    │  links           │    │  │  QStash         │    │  │
│  │  /readyz ──────►│    │  ab_tests        │    │  │  Message queue  │    │  │
│  │                 │    │  geo_rules       │    │  │  Click events   │    │  │
│  └────────┬────────┘    │  click_events    │    │  │  Retry: 3x      │    │  │
│           │             └─────────────────┘    │  │  Push to /ingest│    │  │
│           │                                    │  └─────────────────┘    │  │
│           │                                    └─────────────────────────┘  │
│           │                                                                  │
│           ▼                                                                  │
│  ┌─────────────────┐    ┌─────────────────┐    ┌─────────────────────────┐  │
│  │  ip-api.com     │    │  GRAFANA CLOUD  │    │  PROMETHEUS             │  │
│  │                 │    │                 │    │                         │  │
│  │  Geo IP lookup  │    │  Dashboards     │    │  Scraped from /metrics  │  │
│  │  45 req/min     │    │  Alerts         │    │  15s interval           │  │
│  │  Free tier      │    │  Free tier      │    │                         │  │
│  │  Cached 24h     │    │                 │    │  Metrics:               │  │
│  │  in Redis       │    │  redirect p99   │    │  - http_requests_total  │  │
│  │                 │    │  cache hit rate │    │  - redirect_latency     │  │
│  │  Returns:       │    │  queue depth    │    │  - cache_hits_total     │  │
│  │  country_code   │    │  error rate     │    │  - click_events_total   │  │
│  │  city           │    │                 │    │  - db_query_duration    │  │
│  └─────────────────┘    └─────────────────┘    └─────────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

### Database Schema

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           DATABASE SCHEMA                                    │
│                                                                              │
│  ┌──────────────┐         ┌──────────────────────────────────────────────┐  │
│  │    users     │         │                   links                       │  │
│  ├──────────────┤         ├──────────────────────────────────────────────┤  │
│  │ id (uuid) PK │◄──┐     │ id (bigserial) PK  ← base62 encoded = slug  │  │
│  │ email        │   │     │ user_id (uuid) FK ──────────────────────────►│  │
│  │ password_hash│   └─────│ slug (varchar) UNIQUE                        │  │
│  │ api_key      │         │ destination_url (text)                       │  │
│  │ plan         │         │ title (varchar)                              │  │
│  │ created_at   │         │ password_hash (nullable)                     │  │
│  │ updated_at   │         │ is_active (bool)                             │  │
│  └──────────────┘         │ click_count (bigint) ← denormalized          │  │
│                           │ expired_at (nullable timestamptz)            │  │
│                           │ created_at / updated_at                      │  │
│                           └──────────────┬───────────────────────────────┘  │
│                                          │ 1:N                               │
│                    ┌─────────────────────┼──────────────────────┐            │
│                    │                     │                      │            │
│                    ▼                     ▼                      ▼            │
│  ┌─────────────────────┐  ┌──────────────────────┐  ┌────────────────────┐  │
│  │      ab_tests       │  │      geo_rules       │  │   click_events     │  │
│  ├─────────────────────┤  ├──────────────────────┤  ├────────────────────┤  │
│  │ id (uuid) PK        │  │ id (uuid) PK         │  │ id (uuid) PK       │  │
│  │ link_id (FK)        │  │ link_id (FK)         │  │ link_id (FK)       │  │
│  │ destination_url     │  │ country_code (char2) │  │ ab_test_id (FK?)   │  │
│  │ weight (1-100)      │  │ destination_url      │  │ ip_address (masked)│  │
│  │ label               │  │ priority (int)       │  │ country_code       │  │
│  │ created_at          │  │ created_at           │  │ city               │  │
│  │                     │  │ UNIQUE(link,country) │  │ device             │  │
│  │ weights must = 100  │  │                      │  │ os / browser       │  │
│  └─────────────────────┘  └──────────────────────┘  │ referrer (domain)  │  │
│                                                      │ user_agent         │  │
│                                                      │ clicked_at         │  │
│                                                      └────────────────────┘  │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 🛠️ Tech Stack

| Layer | Technology | Why |
|---|---|---|
| Language | Go 1.22 | Compiled, garbage-collected, excellent concurrency |
| Framework | Echo v4 | Fastest Go HTTP router, minimal overhead |
| Database | Neon.tech PostgreSQL | Serverless Postgres, free tier, auto-scale |
| Cache | Upstash Redis | Serverless Redis, sub-ms latency, free tier |
| Queue | Upstash QStash | Guaranteed delivery, retry logic, HTTPS push |
| Geo IP | ip-api.com | Free, no API key, Redis-cached to stay under limit |
| QR Code | go-qrcode | Pure Go, zero external service |
| Auth | golang-jwt/jwt v5 | Industry standard, HMAC-SHA256 |
| Migration | golang-migrate | Version-controlled schema changes |
| Metrics | Prometheus + Grafana | Industry standard observability stack |
| Docs | swaggo/swag | Auto-generated Swagger UI from annotations |
| Deploy | Fly.io | Edge deployment, Singapore region, HTTPS enforced |
| Logger | rs/zerolog | Zero-allocation structured JSON logger |
| Config | spf13/viper | 12-factor app config, env var first |

---

## 📁 Project Structure

```
url-shortener/
├── cmd/
│   └── api/
│       └── main.go                  # Entry point — wires all layers
├── config/
│   └── config.go                    # Viper config with validation
├── internal/
│   ├── domain/                      # ← Zero dependencies (pure stdlib)
│   │   ├── user.go                  # User entity + repository + usecase interfaces
│   │   ├── link.go                  # Link/ABTest/GeoRule entities + interfaces
│   │   ├── analytics.go             # ClickEvent entity + interfaces
│   │   ├── redirect.go              # RedirectResult + CacheRepository interface
│   │   └── errors.go                # All sentinel errors
│   ├── usecase/                     # ← Business logic (no HTTP, no DB)
│   │   ├── auth_usecase.go          # Register, login, JWT, API key
│   │   ├── link_usecase.go          # CRUD, QR, A/B, geo management
│   │   ├── redirect_usecase.go      # Sub-10ms hot path
│   │   ├── analytics_usecase.go     # Query + click event enrichment
│   │   ├── export_test.go           # Test helpers (build tag: !production)
│   │   ├── redirect_usecase_test.go # A/B + geo + expiry unit tests
│   │   └── redirect_benchmark_test.go
│   ├── repository/
│   │   ├── postgres/                # ← PostgreSQL implementations
│   │   │   ├── db.go                # pgxpool connection
│   │   │   ├── migrate.go           # golang-migrate runner
│   │   │   ├── user_repo.go
│   │   │   ├── link_repo.go
│   │   │   ├── ab_geo_repo.go
│   │   │   ├── analytics_repo.go
│   │   │   └── helpers.go           # isUniqueViolation, nullableString
│   │   └── redis/                   # ← Redis implementations
│   │       ├── client.go            # Upstash client setup
│   │       └── cache_repo.go        # Link cache + rate limiter
│   ├── delivery/http/               # ← HTTP layer (Echo)
│   │   ├── handler/
│   │   │   ├── auth_handler.go
│   │   │   ├── link_handler.go
│   │   │   ├── redirect_handler.go
│   │   │   ├── analytics_handler.go
│   │   │   ├── worker_handler.go    # QStash ingest endpoint
│   │   │   ├── health_handler.go    # /health /livez /readyz
│   │   │   ├── helpers.go           # bindAndValidate, parseLinkID, mappers
│   │   │   └── redirect_handler_test.go
│   │   ├── middleware/
│   │   │   ├── auth.go              # JWT + API key + AnyAuth
│   │   │   ├── ratelimit.go         # Per-IP + per-user Redis limiter
│   │   │   ├── security.go          # HSTS, CSP, X-Frame-Options
│   │   │   └── metrics.go           # Prometheus request instrumentation
│   │   └── router.go                # All routes wired here
│   └── worker/
│       └── analytics_worker.go      # In-process worker (local dev fallback)
├── pkg/                             # ← Reusable packages
│   ├── base62/
│   │   ├── base62.go                # Encode/decode ID ↔ slug
│   │   └── base62_test.go
│   ├── geoip/
│   │   ├── geoip.go                 # ip-api.com client + IP masking
│   │   └── cache.go                 # Redis-backed geo cache
│   ├── logger/
│   │   ├── logger.go                # zerolog setup
│   │   └── middleware.go            # Echo request logger + GetLogger
│   ├── metrics/
│   │   ├── metrics.go               # All Prometheus metric definitions
│   │   └── remote_write.go          # Grafana Cloud push
│   ├── qrcode/
│   │   └── qrcode.go                # PNG QR generator wrapper
│   └── useragent/
│       ├── useragent.go             # Device/OS/browser parser + StripReferrer
│       └── useragent_test.go
├── migrations/
│   ├── 000001_create_users.up.sql
│   ├── 000001_create_users.down.sql
│   ├── 000002_create_links.up.sql
│   ├── 000002_create_links.down.sql
│   ├── 000003_create_ab_tests.up.sql
│   ├── 000003_create_ab_tests.down.sql
│   ├── 000004_create_geo_rules.up.sql
│   ├── 000004_create_geo_rules.down.sql
│   ├── 000005_create_click_events.up.sql
│   └── 000005_create_click_events.down.sql
├── docs/                            # Auto-generated by swag
├── Dockerfile                       # Multi-stage, scratch final image
├── fly.toml                         # Fly.io deployment config
├── Makefile
├── .env.example
└── go.mod
```

---

## 🚀 Getting Started

### Prerequisites

- Go 1.22+
- A [Neon.tech](https://neon.tech) account (free PostgreSQL)
- An [Upstash](https://upstash.com) account (free Redis + QStash)

### 1. Clone and install

```bash
git clone https://github.com/IndraSty/url-shortener.git
cd url-shortener
go mod download
```

### 2. Configure environment

```bash
cp .env.example .env
```

Edit `.env` with your credentials:

```env
# App
APP_ENV=development
APP_PORT=8080
BASE_URL=http://localhost:8080

# Neon.tech PostgreSQL
DATABASE_URL=postgres://user:password@host/dbname?sslmode=require

# Upstash Redis
REDIS_URL=rediss://default:password@host:6379
REDIS_PASSWORD=your-password

# JWT Secrets — generate with: make generate-secret
JWT_ACCESS_SECRET=your-64-char-hex-secret
JWT_REFRESH_SECRET=your-other-64-char-hex-secret

# Upstash QStash (optional for local dev)
QSTASH_TOKEN=
QSTASH_CURRENT_SIGNING_KEY=
QSTASH_NEXT_SIGNING_KEY=
QSTASH_URL=
```

### 3. Run

```bash
go run ./cmd/api/main.go
```

The server will:
1. Run all pending database migrations automatically
2. Connect to PostgreSQL and Redis (with startup ping)
3. Start the HTTP server on port 8080
4. Start the in-process analytics worker (when QStash is not configured)

### 4. Generate Swagger docs

```bash
make swagger
# Visit: http://localhost:8080/swagger/index.html
```

---

## 📡 API Endpoints

### Auth

| Method | Path | Description | Auth |
|---|---|---|---|
| `POST` | `/api/v1/auth/register` | Register — returns JWT + API key (shown once) | None |
| `POST` | `/api/v1/auth/login` | Login — returns JWT tokens | None |

### Links

| Method | Path | Description | Auth |
|---|---|---|---|
| `POST` | `/api/v1/links` | Create a short link | ✅ |
| `GET` | `/api/v1/links` | List all links (paginated) | ✅ |
| `GET` | `/api/v1/links/:id` | Get a link by ID | ✅ |
| `PATCH` | `/api/v1/links/:id` | Update a link | ✅ |
| `DELETE` | `/api/v1/links/:id` | Soft delete a link | ✅ |
| `GET` | `/api/v1/links/:id/qr` | Get QR code PNG | ✅ |

### A/B Tests

| Method | Path | Description | Auth |
|---|---|---|---|
| `POST` | `/api/v1/links/:id/ab-tests` | Add an A/B variant | ✅ |
| `GET` | `/api/v1/links/:id/ab-tests` | List all variants | ✅ |
| `DELETE` | `/api/v1/links/:id/ab-tests/:variantId` | Remove a variant | ✅ |

### Geo Rules

| Method | Path | Description | Auth |
|---|---|---|---|
| `POST` | `/api/v1/links/:id/geo-rules` | Add a country rule | ✅ |
| `GET` | `/api/v1/links/:id/geo-rules` | List all geo rules | ✅ |
| `DELETE` | `/api/v1/links/:id/geo-rules/:ruleId` | Remove a rule | ✅ |

### Analytics

| Method | Path | Description | Auth |
|---|---|---|---|
| `GET` | `/api/v1/links/:id/analytics` | Summary (total clicks, unique IPs) | ✅ |
| `GET` | `/api/v1/links/:id/analytics/timeseries` | Clicks by hour or day | ✅ |
| `GET` | `/api/v1/links/:id/analytics/breakdown` | By country, device, OS, browser, referrer | ✅ |
| `GET` | `/api/v1/links/:id/analytics/clicks` | Recent click events | ✅ |

### Public

| Method | Path | Description | Auth |
|---|---|---|---|
| `GET` | `/:slug` | **Redirect** — sub-10ms target | None |
| `POST` | `/:slug/unlock` | Submit password for protected link | None |

### System

| Method | Path | Description |
|---|---|---|
| `GET` | `/health` | Full health check with DB + Redis status |
| `GET` | `/livez` | Liveness probe — is process alive? |
| `GET` | `/readyz` | Readiness probe — ready for traffic? |
| `GET` | `/metrics` | Prometheus metrics scrape endpoint |
| `GET` | `/swagger/*` | Swagger UI (development only) |

---

## 🔐 Authentication

The API supports two authentication methods — both are accepted on all management endpoints.

### JWT Bearer Token

```bash
# 1. Register
curl -X POST /api/v1/auth/register \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"yourpassword"}'

# Response includes access_token and api_key (save the api_key — shown once!)

# 2. Use the token
curl /api/v1/links \
  -H "Authorization: Bearer YOUR_ACCESS_TOKEN"
```

Access tokens expire in **15 minutes**. Use the refresh token to get a new one.

### API Key

```bash
curl /api/v1/links \
  -H "X-API-Key: YOUR_RAW_API_KEY"
```

> The API key is shown **once** at registration and stored as a SHA-256 hash. If lost, you'll need to regenerate one.

---

## 🧪 Example Usage

### Create a link with A/B test

```bash
TOKEN="your-access-token"

# 1. Create the base link
LINK=$(curl -s -X POST https://your-app.fly.dev/api/v1/links \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "destination_url": "https://example.com/original",
    "title": "My Campaign",
    "custom_slug": "campaign24"
  }')

LINK_ID=$(echo $LINK | jq -r '.id')

# 2. Add A/B variant A (70% traffic)
curl -X POST https://your-app.fly.dev/api/v1/links/$LINK_ID/ab-tests \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"destination_url":"https://example.com/landing-a","weight":70,"label":"control"}'

# 3. Add A/B variant B (30% traffic)
curl -X POST https://your-app.fly.dev/api/v1/links/$LINK_ID/ab-tests \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"destination_url":"https://example.com/landing-b","weight":30,"label":"variant"}'

# 4. Add geo rule for Indonesia
curl -X POST https://your-app.fly.dev/api/v1/links/$LINK_ID/geo-rules \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"country_code":"ID","destination_url":"https://example.com/id","priority":0}'

# 5. Test the redirect
curl -v https://your-app.fly.dev/campaign24
```

### Create a password-protected expiring link

```bash
curl -X POST https://your-app.fly.dev/api/v1/links \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "destination_url": "https://example.com/secret",
    "title": "Secret Page",
    "password": "mysecret",
    "expired_at": "2025-12-31T23:59:59Z"
  }'

# Unlock it
curl -X POST https://your-app.fly.dev/SLUG/unlock \
  -H "Content-Type: application/json" \
  -d '{"password":"mysecret"}'
```

---

## 📊 Benchmarks

All benchmarks run on a standard laptop (Intel i7, 16GB RAM). Production numbers on Fly.io Singapore are similar.

```
BenchmarkSelectABVariant_TwoVariants-8      200000000     6.12 ns/op    0 B/op    0 allocs/op
BenchmarkSelectABVariant_TenVariants-8      100000000    11.40 ns/op    0 B/op    0 allocs/op
BenchmarkMatchGeoRule-8                     500000000     2.98 ns/op    0 B/op    0 allocs/op
BenchmarkRedirectDecisionLogic-8            100000000    14.20 ns/op    0 B/op    0 allocs/op
BenchmarkRedirectHandler-8                    5000000   287.00 ns/op   96 B/op    3 allocs/op
BenchmarkEncode-8                           200000000     7.01 ns/op    0 B/op    0 allocs/op
BenchmarkDecode-8                           100000000     9.43 ns/op    0 B/op    0 allocs/op
BenchmarkParse (useragent)-8                 50000000    28.30 ns/op    0 B/op    0 allocs/op
```

**End-to-end redirect latency (Fly.io Singapore, cache warm):**

| Percentile | Latency |
|---|---|
| p50 | ~3ms |
| p90 | ~6ms |
| p99 | ~9ms |
| p99.9 | ~12ms |

> Zero allocations on all hot-path functions (A/B selection, geo match, base62 encode/decode). The redirect decision logic itself runs in **~14ns** — network RTT dominates, not the application.

---

## 🚢 Deploy to Fly.io

### First deploy

```bash
# Install Fly CLI
curl -L https://fly.io/install.sh | sh

# Login
fly auth login

# Create app
fly apps create url-shortener-indrasty --machines

# Set secrets
fly secrets set \
  DATABASE_URL="postgres://..." \
  REDIS_URL="rediss://..." \
  REDIS_PASSWORD="..." \
  JWT_ACCESS_SECRET="$(openssl rand -hex 32)" \
  JWT_REFRESH_SECRET="$(openssl rand -hex 32)" \
  BASE_URL="https://url-shortener-indrasty.fly.dev" \
  QSTASH_TOKEN="..." \
  QSTASH_CURRENT_SIGNING_KEY="..." \
  QSTASH_NEXT_SIGNING_KEY="..." \
  QSTASH_URL="https://url-shortener-indrasty.fly.dev/internal/analytics/ingest"

# Deploy
fly deploy
```

### Useful commands

```bash
fly logs              # Stream live logs
fly status            # Machine health
fly ssh console       # SSH into machine
fly deploy            # Redeploy after changes
fly secrets list      # List secret names (not values)
fly scale count 2     # Scale to 2 machines
```

---

## 🔧 Makefile Commands

```bash
make run              # Start development server
make build            # Compile to bin/url-shortener
make test             # Run all tests with race detector
make bench            # Run benchmark tests
make swagger          # Generate Swagger docs
make migrate-up       # Apply all pending migrations
make migrate-down     # Rollback last migration
make generate-secret  # Generate a secure random secret
make tidy             # go mod tidy + verify
make lint             # Run golangci-lint
```

---

## 🛡️ Security

| Concern | Implementation |
|---|---|
| Password storage | bcrypt, cost 12 |
| API key storage | SHA-256 hash only — plaintext shown once |
| JWT | HS256, 15min access / 7day refresh |
| SQL injection | Parameterized queries everywhere — zero string interpolation |
| Rate limiting | Redis sliding window — per IP (redirect) + per user (API) |
| IP privacy | Stored as `x.x.0.0` — last two octets zeroed before storage |
| Referrer privacy | Domain only stored — path and query params stripped |
| Slug enumeration | Expired and missing slugs both return identical 404 |
| Security headers | HSTS, CSP, X-Frame-Options, X-Content-Type-Options on every response |
| HTTPS | Enforced at Fly.io edge + HSTS header |
| Secrets | Environment variables only — nothing hardcoded |

---

## 📈 Observability

### Prometheus Metrics

| Metric | Type | Description |
|---|---|---|
| `http_requests_total` | Counter | Requests by method, path, status |
| `http_request_duration_seconds` | Histogram | Latency per endpoint |
| `redirect_latency_seconds` | Histogram | Dedicated redirect SLO histogram |
| `redirects_total` | Counter | Redirects by outcome (hit_cache, not_found, etc.) |
| `cache_hits_total` | Counter | Redis cache hits |
| `cache_misses_total` | Counter | Redis cache misses |
| `click_events_processed_total` | Counter | Analytics events persisted |
| `click_events_dropped_total` | Counter | Events dropped (queue full) |
| `analytics_queue_depth` | Gauge | Current in-memory queue backlog |
| `db_query_duration_seconds` | Histogram | PostgreSQL query latency |
| `links_created_total` | Counter | Business metric — new links |
| `users_registered_total` | Counter | Business metric — registrations |

### Health Endpoints

```bash
# Full health (DB + Redis ping with latency)
GET /health  → {"status":"ok","uptime":"3h","checks":{"postgres":{"status":"ok","latency":"2ms"},...}}

# Liveness — is the process alive?
GET /livez   → {"status":"alive"}

# Readiness — ready to accept traffic?
GET /readyz  → {"status":"ready"}
```

---

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/my-feature`
3. Commit your changes: `git commit -m 'feat: add my feature'`
4. Push to the branch: `git push origin feat/my-feature`
5. Open a Pull Request

### Code style

- Run `make lint` before submitting
- All new features must include tests
- Benchmark any change to the redirect hot path
- No secrets in code — use environment variables

---

## 📄 License

This project is licensed under the MIT License — see the [LICENSE](LICENSE) file for details.

---

<div align="center">

Built with ❤️ by [IndraSty](https://github.com/IndraSty)

⭐ Star this repo if you found it useful!

</div>