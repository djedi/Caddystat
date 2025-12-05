# Caddystat

Lightweight stats and a live dashboard for Caddy access logs.

- Live summaries via SSE for recent traffic
- GeoIP enrichment (optional) with privacy controls
- Docker- and Go-friendly deployment
- Minimal footprint with SQLite storage

## Quick start (local)

Prereqs: Go 1.22+. From the repo root:

```bash
go run ./cmd/caddystat
```

Visit `http://localhost:8404/` for the dashboard.

## Configuration

All configuration is done via environment variables:

### Core Settings

| Variable      | Default               | Description                     |
| ------------- | --------------------- | ------------------------------- |
| `LOG_PATH`    | `./caddy.log`         | Comma-separated Caddy log paths |
| `LISTEN_ADDR` | `:8404`               | HTTP bind address               |
| `DB_PATH`     | `./data/caddystat.db` | SQLite database path            |

### Data Retention

| Variable              | Default | Description                                                                  |
| --------------------- | ------- | ---------------------------------------------------------------------------- |
| `DATA_RETENTION_DAYS` | `7`     | Default retention for raw rows. Sites can override via `/api/sites` endpoint |
| `RAW_RETENTION_HOURS` | `48`    | Window used for realtime summaries                                           |

### GeoIP

| Variable          | Default   | Description                                        |
| ----------------- | --------- | -------------------------------------------------- |
| `MAXMIND_DB_PATH` | _(empty)_ | Path to `GeoLite2-City.mmdb` to enable geo lookups |

### Privacy

| Variable                       | Default     | Description                                 |
| ------------------------------ | ----------- | ------------------------------------------- |
| `PRIVACY_HASH_IPS`             | `false`     | Hash IPs before storing                     |
| `PRIVACY_HASH_SALT`            | `caddystat` | Salt used for IP hashing                    |
| `PRIVACY_ANONYMIZE_LAST_OCTET` | `false`     | Zero last IPv4 octet before hashing/storing |

### Authentication

| Variable        | Default   | Description                           |
| --------------- | --------- | ------------------------------------- |
| `AUTH_USERNAME` | _(empty)_ | Username for dashboard authentication |
| `AUTH_PASSWORD` | _(empty)_ | Password for dashboard authentication |

Both `AUTH_USERNAME` and `AUTH_PASSWORD` must be set to enable authentication.

**Site-specific Access:** When logging in via the API, you can restrict a session to specific sites by passing `allowed_sites` in the login request body. See the API section for details.

### Logging

| Variable    | Default | Description                                 |
| ----------- | ------- | ------------------------------------------- |
| `LOG_LEVEL` | `INFO`  | Log level: `DEBUG`, `INFO`, `WARN`, `ERROR` |

### Security

| Variable                 | Default   | Description                                      |
| ------------------------ | --------- | ------------------------------------------------ |
| `RATE_LIMIT_PER_MINUTE`  | `0`       | Max requests per minute per IP (0 = disabled)    |
| `MAX_REQUEST_BODY_BYTES` | `1048576` | Maximum request body size in bytes (1MB default) |

### Database

| Variable             | Default | Description                                         |
| -------------------- | ------- | --------------------------------------------------- |
| `DB_MAX_CONNECTIONS` | `1`     | Maximum database connections (increase for reads)   |
| `DB_QUERY_TIMEOUT`   | `30s`   | Query timeout duration (e.g., `30s`, `1m`, `2m30s`) |

### Bot Detection

| Variable              | Default   | Description                                                  |
| --------------------- | --------- | ------------------------------------------------------------ |
| `BOT_SIGNATURES_PATH` | _(empty)_ | Comma-separated list of bot signature JSON files (see below) |

Caddystat includes built-in bot detection with intent classification (SEO, social, monitoring, AI, archiver). To customize bot detection, create JSON files with the following format:

```json
{
  "version": "1.0",
  "bots": [
    { "signature": "mybot", "name": "MyBot", "intent": "seo" },
    { "signature": "customcrawler", "name": "CustomCrawler", "intent": "monitoring" }
  ]
}
```

Valid intent values: `seo`, `social`, `monitoring`, `ai`, `archiver`, `unknown`

See `bots.json` in the repository root for a complete example.

**Community Bot Lists:** You can load multiple bot signature files by providing a comma-separated list. Signatures from later files override earlier ones, and all signatures are merged with the built-in defaults.

```bash
# Load built-in defaults + your custom bots
BOT_SIGNATURES_PATH=/config/my-bots.json

# Load multiple community lists (later files override earlier ones)
BOT_SIGNATURES_PATH=/config/ai-bots.json,/config/seo-bots.json,/config/my-overrides.json
```

This allows you to:

- Use community-maintained bot lists (e.g., from GitHub)
- Override default bot classifications
- Add organization-specific bot signatures
- Keep bot lists organized by category

### Alerting

Caddystat includes an alerting system that can notify you via email or webhook when certain conditions are met.

| Variable                  | Default   | Description                               |
| ------------------------- | --------- | ----------------------------------------- |
| `ALERT_ENABLED`           | `false`   | Enable the alerting system                |
| `ALERT_EVALUATE_INTERVAL` | `1m`      | How often to check alert rules            |
| `ALERT_RULES_PATH`        | _(empty)_ | Path to JSON file with custom alert rules |

#### Error Rate Alert

Triggers when 5xx error rate exceeds a threshold.

| Variable                     | Default    | Description                                  |
| ---------------------------- | ---------- | -------------------------------------------- |
| `ALERT_ERROR_RATE_THRESHOLD` | _(empty)_  | Error rate percentage to trigger (e.g., `5`) |
| `ALERT_ERROR_RATE_DURATION`  | `5m`       | Evaluation window                            |
| `ALERT_ERROR_RATE_COOLDOWN`  | `15m`      | Minimum time between alerts                  |
| `ALERT_ERROR_RATE_SEVERITY`  | `critical` | Severity: `info`, `warning`, `critical`      |

#### Traffic Spike Alert

Triggers when traffic increases suddenly.

| Variable                        | Default   | Description                                 |
| ------------------------------- | --------- | ------------------------------------------- |
| `ALERT_TRAFFIC_SPIKE_THRESHOLD` | _(empty)_ | Percentage increase to trigger (e.g., `50`) |
| `ALERT_TRAFFIC_SPIKE_DURATION`  | `5m`      | Evaluation window                           |
| `ALERT_TRAFFIC_SPIKE_COOLDOWN`  | `15m`     | Minimum time between alerts                 |
| `ALERT_TRAFFIC_SPIKE_SEVERITY`  | `warning` | Severity level                              |

#### Traffic Drop Alert

Triggers when traffic drops suddenly.

| Variable                       | Default   | Description                                 |
| ------------------------------ | --------- | ------------------------------------------- |
| `ALERT_TRAFFIC_DROP_THRESHOLD` | _(empty)_ | Percentage decrease to trigger (e.g., `50`) |
| `ALERT_TRAFFIC_DROP_DURATION`  | `5m`      | Evaluation window                           |
| `ALERT_TRAFFIC_DROP_COOLDOWN`  | `15m`     | Minimum time between alerts                 |
| `ALERT_TRAFFIC_DROP_SEVERITY`  | `warning` | Severity level                              |

#### 404 Threshold Alert

Triggers when 404 count exceeds a threshold.

| Variable              | Default   | Description                 |
| --------------------- | --------- | --------------------------- |
| `ALERT_404_THRESHOLD` | _(empty)_ | Count threshold to trigger  |
| `ALERT_404_DURATION`  | `5m`      | Evaluation window           |
| `ALERT_404_COOLDOWN`  | `15m`     | Minimum time between alerts |
| `ALERT_404_SEVERITY`  | `warning` | Severity level              |

#### Email Notifications

| Variable              | Default               | Description                         |
| --------------------- | --------------------- | ----------------------------------- |
| `ALERT_SMTP_HOST`     | _(empty)_             | SMTP server (enables email alerts)  |
| `ALERT_SMTP_PORT`     | `587`                 | SMTP port                           |
| `ALERT_SMTP_USERNAME` | _(empty)_             | SMTP username                       |
| `ALERT_SMTP_PASSWORD` | _(empty)_             | SMTP password                       |
| `ALERT_SMTP_FROM`     | `caddystat@localhost` | Sender email address                |
| `ALERT_EMAIL_TO`      | _(empty)_             | Comma-separated recipient addresses |

#### Webhook Notifications

| Variable                | Default   | Description                               |
| ----------------------- | --------- | ----------------------------------------- |
| `ALERT_WEBHOOK_URL`     | _(empty)_ | Webhook URL (enables webhook alerts)      |
| `ALERT_WEBHOOK_METHOD`  | `POST`    | HTTP method: `POST` or `GET`              |
| `ALERT_WEBHOOK_HEADERS` | _(empty)_ | Custom headers: `Key1:Value1,Key2:Value2` |

### Advanced

| Variable                    | Default | Description                       |
| --------------------------- | ------- | --------------------------------- |
| `AGGREGATION_INTERVAL`      | `1h`    | Duration between aggregation runs |
| `AGGREGATION_FLUSH_SECONDS` | `10`    | Seconds between flush writes      |

## Docker Compose (Development)

Use the `dev` script to manage the development environment:

```bash
./dev              # Start in foreground (default)
./dev up -d        # Start in background (detached)
./dev down         # Stop containers
./dev restart      # Restart containers
./dev rebuild      # Full rebuild with --no-cache
./dev logs         # Follow all container logs
./dev logs caddy   # Follow specific service logs
./dev ps           # Show container status
./dev shell        # Open shell in caddystat container
./dev clean        # Stop and remove all volumes (requires confirmation)
```

Run `./dev --help` for full usage information.

Includes a sample `Caddyfile` that logs to `/var/log/caddy/access.log`. Logs are shared with the `caddystat` container via a volume. Web UI is on `http://localhost:8404/`.

## Production Setup

To add Caddystat to an existing Caddy deployment:

### 1. Update your docker-compose.yml

```yaml
services:
  caddy:
    image: caddy:latest
    container_name: caddy
    restart: unless-stopped
    ports:
      - '80:80'
      - '443:443'
      - '443:443/udp'
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile
      - ./caddy_data:/data
      - ./caddy_config:/config
      - caddy_logs:/var/log/caddy # Add this shared volume

  caddystat:
    image: xhenxhe/caddystat:latest
    container_name: caddystat
    restart: unless-stopped
    environment:
      - LOG_PATH=/var/log/caddy/access.log
      - LISTEN_ADDR=:8000
      - DB_PATH=/data/caddystat.db
      - DATA_RETENTION_DAYS=90
      # Optional: - MAXMIND_DB_PATH=/maxmind/GeoLite2-City.mmdb
    volumes:
      - caddy_logs:/var/log/caddy:ro
      - caddystat_data:/data
      # Optional: - ./MaxMind:/maxmind:ro
    depends_on:
      - caddy

volumes:
  caddy_logs:
  caddystat_data:
```

### 2. Enable JSON logging in your Caddyfile

Add a global log block to log all sites:

```
{
    log {
        output file /var/log/caddy/access.log {
            roll_size 10mb
            roll_keep 5
        }
        format json
    }
}

example.com {
    reverse_proxy backend:8000
}
```

Or add logging to specific sites:

```
example.com {
    reverse_proxy backend:8000

    log {
        output file /var/log/caddy/access.log {
            roll_size 10mb
            roll_keep 5
        }
        format json
    }
}
```

### 3. Expose the dashboard

**Option A: Direct port (internal access only)**

```yaml
caddystat:
  ports:
    - '8000:8000'
```

**Option B: Proxy through Caddy (recommended for HTTPS)**

Add to your Caddyfile:

```
stats.yourdomain.com {
    reverse_proxy caddystat:8000
}
```

## GeoIP Support

To enable GeoLite, download the database once:

```bash
MAXMIND_LICENSE_KEY=XXXX ./scripts/fetch-geolite.sh
```

Then set `MAXMIND_DB_PATH=/maxmind/GeoLite2-City.mmdb` in the `caddystat` service.

## Frontend

11ty + Alpine.js + Tailwind (built via PostCSS). From `web/`:

```bash
npm install
npm run build   # outputs to web/_site
```

The Go server serves `web/_site` at `/`.

## API

### Stats Endpoints

- `GET /api/stats/summary?range=24h&host=` – totals, statuses, bandwidth, top paths/hosts, unique visitors, avg latency.
- `GET /api/stats/requests?range=24h` – hourly buckets.
- `GET /api/stats/geo?range=24h` – country/region/city counts (empty if GeoLite not configured).
- `GET /api/stats/bandwidth?range=24h&limit=10` – bandwidth statistics per host, path, and content type.
- `GET /api/stats/performance?range=24h&host=` – response time percentiles and slow pages.
- `GET /api/stats/sessions?range=24h&host=&limit=50` – visitor session reconstruction.
- `GET /api/stats/browsers` – browser usage stats.
- `GET /api/stats/os` – OS usage stats.
- `GET /api/stats/robots` – bot/spider stats.
- `GET /api/stats/referrers` – referrer stats.
- `GET /api/stats/hosts` – top hosts by request count.
- `GET /api/stats/monthly?months=12` – monthly history.
- `GET /api/stats/daily` – current month daily breakdown.
- `GET /api/stats/recent?limit=20` – recent individual requests.
- `GET /api/stats/status` – system status (DB size, row counts).
- `GET /api/sse?host=&range=24h` – server-sent events for live updates.

### Site Management

- `GET /api/sites` – list all sites (configured + discovered from logs).
- `POST /api/sites` – create a site configuration.
- `GET /api/sites/{id}` – get a specific site.
- `PUT /api/sites/{id}` – update a site configuration.
- `DELETE /api/sites/{id}` – delete a site configuration.

Site configuration body:

```json
{
  "host": "example.com",
  "display_name": "Example Site",
  "retention_days": 30,
  "enabled": true
}
```

### Authentication

- `GET /api/auth/check` – check authentication status (returns permissions if authenticated).
- `POST /api/auth/login` – login with username/password.
- `POST /api/auth/logout` – logout and clear session.

Login request body:

```json
{
  "username": "admin",
  "password": "secret",
  "allowed_sites": ["site1.com", "site2.com"]
}
```

The `allowed_sites` field is optional. If omitted, the session has access to all sites. If provided, access is restricted to only those hosts.

### System

- `GET /metrics` – Prometheus metrics endpoint.
- `GET /health` – health check endpoint (returns DB status and version).

## Data Export & Backup

Caddystat provides several endpoints for exporting data and backing up the database.

### Export Endpoints

All export endpoints require authentication if `AUTH_USERNAME` and `AUTH_PASSWORD` are configured.

| Endpoint                 | Description                   | Query Parameters               |
| ------------------------ | ----------------------------- | ------------------------------ |
| `GET /api/export/csv`    | Export requests as CSV        | `range` (default: 24h), `host` |
| `GET /api/export/json`   | Export requests as JSON array | `range` (default: 24h), `host` |
| `GET /api/export/backup` | Download SQLite database file | None                           |

**Examples:**

```bash
# Export last 24 hours as CSV
curl -o export.csv http://localhost:8404/api/export/csv

# Export last 7 days for a specific host
curl -o export.csv "http://localhost:8404/api/export/csv?range=168h&host=example.com"

# Export as JSON
curl -o export.json http://localhost:8404/api/export/json?range=48h

# Download full database backup
curl -o backup.db http://localhost:8404/api/export/backup
```

### Backup Procedures

**Manual Backup:**

1. Download the database via the API:

   ```bash
   curl -o caddystat-backup-$(date +%Y%m%d).db http://localhost:8404/api/export/backup
   ```

2. Or copy the database file directly (stop the service first to ensure consistency):
   ```bash
   docker compose stop caddystat
   cp /path/to/caddystat.db /backup/caddystat-$(date +%Y%m%d).db
   docker compose start caddystat
   ```

**Automated Backup (cron):**

Add to your crontab for daily backups:

```bash
0 2 * * * curl -s -o /backups/caddystat-$(date +\%Y\%m\%d).db http://localhost:8404/api/export/backup
```

### Restore Procedures

1. Stop the Caddystat service:

   ```bash
   docker compose stop caddystat
   ```

2. Replace the database file:

   ```bash
   cp /backup/caddystat-backup.db /path/to/caddystat.db
   ```

3. Start the service:
   ```bash
   docker compose start caddystat
   ```

**Note:** The backup endpoint streams the live database file. For guaranteed consistency during high-traffic periods, prefer stopping the service or using the file copy method.

## Development

- Run API locally: `go run ./cmd/caddystat`.
- Build frontend: `cd web && npm install && npm run build`.
- Document new env vars or flags when adding features.

### Testing

Use the `./test` script to run tests:

| Flag               | Description                                                 |
| ------------------ | ----------------------------------------------------------- |
| `--report`         | Generate HTML coverage report and open in browser           |
| `--coverage`, `-c` | Show coverage summary in terminal                           |
| `--verbose`, `-v`  | Verbose test output                                         |
| `--race`, `-r`     | Enable race detector                                        |
| `--watch`, `-w`    | Re-run tests on file changes (requires `fswatch` or `entr`) |
| `--bench`, `-b`    | Run benchmarks with memory stats                            |
| `--short`, `-s`    | Skip slow tests                                             |
| `--clean`          | Remove coverage files                                       |

Options can be combined: `./test --verbose --race --coverage`

Target specific packages: `./test ./internal/ingest/...`

## Contributing and support

Contributions are welcome! Please read `CONTRIBUTING.md` for workflows and `CODE_OF_CONDUCT.md` for expected behavior. Open issues for bugs/ideas; include logs, repro steps, and config snippets when possible.

## License

Distributed under the MIT License. See `LICENSE` for details.

## Notes

- Uses pure-Go SQLite driver `modernc.org/sqlite`.
- Tails Caddy JSON logs (handles rotation) via `github.com/hpcloud/tail`.
- Privacy controls: hash IPs with a salt and/or anonymize last IPv4 octet before hashing/storing.
- Retention cleanup runs periodically to keep the DB small.
