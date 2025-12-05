# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Caddystat is a minimal stats and live dashboard for Caddy web server access logs. It tails Caddy's JSON access logs, stores metrics in SQLite, and provides a real-time web dashboard via SSE.

## Common Commands

```bash
# Run locally (Go)
go run ./cmd/caddystat

# Development with Docker (includes test Caddy + nginx site)
./dev              # Start in foreground
./dev up -d        # Start in background
./dev down         # Stop containers
./dev restart      # Restart containers
./dev rebuild      # Full rebuild (--no-cache)
./dev logs         # Follow logs
./dev shell        # Shell into caddystat container

# Frontend build (from web/ directory)
cd web && npm install && npm run build   # outputs to web/_site

# Frontend dev with hot reload (from web/ directory)
cd web && npm run dev

# Run tests
./test                    # Run all tests
./test --coverage         # Show coverage summary
./test --report           # Generate HTML coverage report and open in browser
./test --verbose --race   # Verbose output with race detection
./test --watch            # Re-run tests on file changes
./test ./internal/ingest  # Test specific package
```

## Environment Variables

- `LOG_PATH` - Comma-separated Caddy log paths (default: `./caddy.log`)
- `LISTEN_ADDR` - HTTP bind address (default: `:8404`)
- `DB_PATH` - SQLite database path (default: `./data/caddystat.db`)
- `DATA_RETENTION_DAYS` - Default purge window for raw rows (default: `7`). Sites can override this with per-site retention policies via the `/api/sites` endpoint.
- `RAW_RETENTION_HOURS` - Window for realtime summaries (default: `48`)
- `MAXMIND_DB_PATH` - Optional path to GeoLite2-City.mmdb for geo lookups
- `AUTH_USERNAME` - Optional username for dashboard authentication
- `AUTH_PASSWORD` - Optional password for dashboard authentication (both must be set to enable auth)
- `RATE_LIMIT_PER_MINUTE` - Max requests per minute per IP (default: `0` = disabled)
- `MAX_REQUEST_BODY_BYTES` - Maximum request body size in bytes (default: `1048576` = 1MB)
- `DB_MAX_CONNECTIONS` - Maximum database connections (default: `1`)
- `DB_QUERY_TIMEOUT` - Query timeout duration (default: `30s`)
- `BOT_SIGNATURES_PATH` - Comma-separated list of bot signature JSON files (community lists merged with defaults, see `bots.json` for format)
- `SSE_BUFFER_SIZE` - Channel buffer size for SSE clients (default: `32`)

### Alerting Configuration

- `ALERT_ENABLED` - Enable alerting system (default: `false`)
- `ALERT_EVALUATE_INTERVAL` - How often to check alert rules (default: `1m`)
- `ALERT_RULES_PATH` - Path to JSON file with alert rules (optional)

#### Error Rate Alert
- `ALERT_ERROR_RATE_THRESHOLD` - Trigger when 5xx rate exceeds this percentage (e.g., `5` for 5%)
- `ALERT_ERROR_RATE_DURATION` - Evaluation window (default: `5m`)
- `ALERT_ERROR_RATE_COOLDOWN` - Min time between alerts (default: `15m`)
- `ALERT_ERROR_RATE_SEVERITY` - Alert severity: `info`, `warning`, `critical` (default: `critical`)

#### Traffic Spike Alert
- `ALERT_TRAFFIC_SPIKE_THRESHOLD` - Trigger when traffic increases by this percentage (e.g., `50` for 50%)
- `ALERT_TRAFFIC_SPIKE_DURATION` - Evaluation window (default: `5m`)
- `ALERT_TRAFFIC_SPIKE_COOLDOWN` - Min time between alerts (default: `15m`)
- `ALERT_TRAFFIC_SPIKE_SEVERITY` - Alert severity (default: `warning`)

#### Traffic Drop Alert
- `ALERT_TRAFFIC_DROP_THRESHOLD` - Trigger when traffic drops by this percentage (e.g., `50` for 50%)
- `ALERT_TRAFFIC_DROP_DURATION` - Evaluation window (default: `5m`)
- `ALERT_TRAFFIC_DROP_COOLDOWN` - Min time between alerts (default: `15m`)
- `ALERT_TRAFFIC_DROP_SEVERITY` - Alert severity (default: `warning`)

#### 404 Threshold Alert
- `ALERT_404_THRESHOLD` - Trigger when 404 count exceeds this number
- `ALERT_404_DURATION` - Evaluation window (default: `5m`)
- `ALERT_404_COOLDOWN` - Min time between alerts (default: `15m`)
- `ALERT_404_SEVERITY` - Alert severity (default: `warning`)

#### Email Channel
- `ALERT_SMTP_HOST` - SMTP server hostname (enables email notifications)
- `ALERT_SMTP_PORT` - SMTP port (default: `587`)
- `ALERT_SMTP_USERNAME` - SMTP username for authentication
- `ALERT_SMTP_PASSWORD` - SMTP password for authentication
- `ALERT_SMTP_FROM` - Sender email address (default: `caddystat@localhost`)
- `ALERT_EMAIL_TO` - Comma-separated list of recipient email addresses

#### Webhook Channel
- `ALERT_WEBHOOK_URL` - Webhook URL (enables webhook notifications)
- `ALERT_WEBHOOK_METHOD` - HTTP method: `POST` or `GET` (default: `POST`)
- `ALERT_WEBHOOK_HEADERS` - Custom headers in format `Key1:Value1,Key2:Value2`

## Architecture

```
cmd/caddystat/main.go     Entry point, wires up components
internal/
├── alerts/               Alerting framework (email, webhook notifications)
├── config/               Environment-based configuration
├── ingest/               Log file tailing and parsing (supports gzip)
├── storage/              SQLite storage with hourly/daily rollups
├── sse/                  Server-sent events hub for live updates
└── useragent/            User-agent parsing for browser/OS/bot detection

web/                      Frontend (Alpine.js + Tailwind, built with PostCSS)
└── _site/                Built static files served by Go at /
```

**Data Flow:**
1. `ingest` tails Caddy JSON logs, imports historical logs (including rotated .gz files)
2. Each log entry is parsed, enriched with geo (if MaxMind configured), and stored in SQLite
3. `storage` maintains raw `requests` table plus `rollups_hourly` and `rollups_daily` for aggregates
4. `server` exposes REST API at `/api/stats/*` and SSE at `/api/sse`
5. New requests broadcast to SSE subscribers for real-time dashboard updates

**Key Implementation Details:**
- Uses pure-Go SQLite driver `modernc.org/sqlite` (no CGO)
- Log tailing via `github.com/hpcloud/tail` handles rotation
- Privacy controls: can hash IPs with salt and/or anonymize last IPv4 octet
- Import progress tracked in DB to resume after restarts

## API Endpoints

- `GET /api/stats/summary?range=24h&host=` - Dashboard summary stats
- `GET /api/stats/requests?range=24h` - Hourly time series
- `GET /api/stats/geo?range=24h` - Country/region/city counts
- `GET /api/stats/hosts` - Top hosts by request count
- `GET /api/stats/browsers` - Browser usage stats
- `GET /api/stats/os` - OS usage stats
- `GET /api/stats/performance?range=24h&host=` - Response time percentiles and slow pages
- `GET /api/stats/bandwidth?range=24h&host=&limit=10` - Bandwidth statistics per host/path/content type
- `GET /api/stats/sessions?range=24h&host=&limit=50&timeout=1800` - Visitor session reconstruction (grouped by IP+UA, with entry/exit pages, bounce rate)
- `GET /api/stats/robots` - Bot/spider stats
- `GET /api/stats/referrers` - Referrer stats
- `GET /api/stats/status` - System status (DB size, row counts, last import time)
- `GET /api/stats/monthly?months=12` - Monthly history
- `GET /api/stats/daily` - Current month daily breakdown
- `GET /api/stats/recent?limit=20` - Recent individual requests
- `GET /api/sse?host=&range=24h` - SSE stream for live updates
- `GET /api/auth/check` - Check authentication status (returns permissions if authenticated)
- `POST /api/auth/login` - Login with username/password (optional: `allowed_sites` array for site-specific access)
- `POST /api/auth/logout` - Logout and clear session
- `GET /api/export/csv?range=24h&host=` - Export requests as CSV
- `GET /api/export/json?range=24h&host=` - Export requests as JSON
- `GET /api/export/backup` - Download SQLite database backup
- `GET /api/sites` - List all sites (configured + discovered from logs)
- `POST /api/sites` - Create a site configuration (body: `{host, display_name, retention_days, enabled}`)
- `GET /api/sites/{id}` - Get a specific site by ID
- `PUT /api/sites/{id}` - Update a site configuration
- `DELETE /api/sites/{id}` - Delete a site configuration
- `GET /health` - Health check (DB status, version)
- `GET /metrics` - Prometheus metrics endpoint
