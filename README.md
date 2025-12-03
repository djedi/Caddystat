# Caddystat

Minimal proof-of-concept stats and live dashboard for Caddy access logs.

## Quick start (local)

```bash
go run ./cmd/caddystat
```

Env vars:

- `LOG_PATH`: Comma-separated Caddy log paths (default `./caddy.log`).
- `LISTEN_ADDR`: HTTP bind (default `:8000`).
- `DB_PATH`: SQLite path (default `./data/caddystat.db`).
- `DATA_RETENTION_DAYS`: Purge raw rows older than N days (default `7`).
- `RAW_RETENTION_HOURS`: Window used for realtime summaries (default `48`).
- `MAXMIND_DB_PATH`: Optional path to `GeoLite2-City.mmdb` to enable geo.
- `PRIVACY_HASH_IPS`: `true|false` to hash IPs with `PRIVACY_HASH_SALT`.
- `PRIVACY_ANONYMIZE_LAST_OCTET`: `true|false` to zero last IPv4 octet before hashing/storing.

## Docker Compose (Development)

```bash
docker compose up --build
```

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
      - "80:80"
      - "443:443"
      - "443:443/udp"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile
      - ./caddy_data:/data
      - ./caddy_config:/config
      - caddy_logs:/var/log/caddy  # Add this shared volume

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
    - "8000:8000"
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

## Notes

- Uses pure-Go SQLite driver `modernc.org/sqlite`.
- Tails Caddy JSON logs (handles rotation) via `github.com/hpcloud/tail`.
- Privacy controls: hash IPs with a salt and/or anonymize last IPv4 octet before hashing/storing.
- Retention cleanup runs periodically to keep the DB small.
