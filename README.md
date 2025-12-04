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

| Variable | Default | Description |
|----------|---------|-------------|
| `LOG_PATH` | `./caddy.log` | Comma-separated Caddy log paths |
| `LISTEN_ADDR` | `:8404` | HTTP bind address |
| `DB_PATH` | `./data/caddystat.db` | SQLite database path |

### Data Retention

| Variable | Default | Description |
|----------|---------|-------------|
| `DATA_RETENTION_DAYS` | `7` | Purge raw rows older than N days |
| `RAW_RETENTION_HOURS` | `48` | Window used for realtime summaries |

### GeoIP

| Variable | Default | Description |
|----------|---------|-------------|
| `MAXMIND_DB_PATH` | _(empty)_ | Path to `GeoLite2-City.mmdb` to enable geo lookups |

### Privacy

| Variable | Default | Description |
|----------|---------|-------------|
| `PRIVACY_HASH_IPS` | `false` | Hash IPs before storing |
| `PRIVACY_HASH_SALT` | `caddystat` | Salt used for IP hashing |
| `PRIVACY_ANONYMIZE_LAST_OCTET` | `false` | Zero last IPv4 octet before hashing/storing |

### Authentication

| Variable | Default | Description |
|----------|---------|-------------|
| `AUTH_USERNAME` | _(empty)_ | Username for dashboard authentication |
| `AUTH_PASSWORD` | _(empty)_ | Password for dashboard authentication |

Both `AUTH_USERNAME` and `AUTH_PASSWORD` must be set to enable authentication.

### Logging

| Variable | Default | Description |
|----------|---------|-------------|
| `LOG_LEVEL` | `INFO` | Log level: `DEBUG`, `INFO`, `WARN`, `ERROR` |

### Advanced

| Variable | Default | Description |
|----------|---------|-------------|
| `AGGREGATION_INTERVAL` | `1h` | Duration between aggregation runs |
| `AGGREGATION_FLUSH_SECONDS` | `10` | Seconds between flush writes |

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

Includes a sample `Caddyfile` that logs to `/var/log/caddy/access.log`. Logs are shared with the `caddystat` container via a volume. Web UI is on `http://localhost:8000/`.

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

## API (MVP)

- `GET /api/stats/summary?range=24h` – totals, statuses, bandwidth, top paths/hosts, unique visitors, avg latency.
- `GET /api/stats/requests?range=24h` – hourly buckets.
- `GET /api/stats/geo?range=24h` – country/region/city counts (empty if GeoLite not configured).
- `GET /api/sse` – server-sent events with live summary snapshots.

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
