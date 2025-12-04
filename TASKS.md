# Caddystat Improvement Tasks

A prioritized list of tasks to make Caddystat a production-ready, feature-rich alternative to AWStats.

---

## High Priority: Security & Stability

### Testing

- [x] Add unit tests for `internal/ingest` - test log parsing with various Caddy JSON formats
- [x] Add unit tests for `internal/useragent` - test browser/OS/bot detection
- [x] Add unit tests for `internal/storage` - test database operations and rollup logic
- [x] Add integration tests for API endpoints
- [x] Add test fixtures with sample Caddy logs (plain + gzip)
- [ ] Set up CI/CD pipeline (GitHub Actions) to run tests on push

### Logging & Observability

- [x] Replace `log.Printf` with structured logging (e.g., `slog` from Go 1.21+)
- [x] Add log levels (DEBUG, INFO, WARN, ERROR) with `LOG_LEVEL` env var
- [x] Log request parsing failures with context (line number, sample content)
- [x] Add startup banner with version, config summary, and loaded features

### Security Hardening

- [x] Add rate limiting middleware (per-IP, configurable via env)
- [x] Add request size limits to prevent DoS
- [x] Add CSRF protection for POST endpoints
- [x] Add Content Security Policy headers
- [x] Implement persistent session storage (SQLite-backed instead of in-memory)
- [x] Add session cleanup job for expired sessions
- [ ] Remove `.env.sample` from repo or ensure no real credentials

### Health & Operations

- [x] Add `GET /health` endpoint (returns DB connectivity status)
- [ ] Add `GET /api/stats/status` endpoint (DB size, row counts, last import time)
- [ ] Implement graceful shutdown (close SSE connections, flush pending writes)
- [ ] Add SIGTERM/SIGINT handler with cleanup

---

## Medium Priority: Performance & Operations

### Database Optimization

- [ ] Add configurable connection pool size (`DB_MAX_CONNECTIONS` env var)
- [ ] Add query timeout configuration (`DB_QUERY_TIMEOUT` env var)
- [ ] Use prepared statements for frequently-run queries
- [ ] Add VACUUM scheduling (or trigger after bulk imports)
- [ ] Add time-based filter to `RecentRequests()` query to avoid full table scan
- [ ] Cache GeoIP lookups in memory (LRU cache with TTL)

### Monitoring & Metrics

- [ ] Add Prometheus metrics endpoint (`GET /metrics`)
  - [ ] Request count by endpoint, status, method
  - [ ] Request latency histogram
  - [ ] SSE subscriber count
  - [ ] Database size and row counts
  - [ ] Ingestion rate (requests/second)
- [ ] Add optional metrics for geo lookups, cache hit rates

### Data Export

- [ ] Add CSV export endpoint (`GET /api/export/csv?range=24h`)
- [ ] Add JSON export endpoint (`GET /api/export/json?range=24h`)
- [ ] Add database backup endpoint or CLI command
- [ ] Document backup/restore procedures in README

### Error Handling

- [ ] Return proper HTTP error codes with JSON error bodies
- [ ] Add error tracking for failed imports (count per file, last error)
- [ ] Surface parsing errors in admin/status endpoint
- [ ] Add retry logic for transient failures (DB locks, file reads)

---

## Medium Priority: New Features

### Enhanced Analytics

- [ ] Add page load time tracking (from Caddy's `duration` field)
- [ ] Add bandwidth tracking per host/path
- [ ] Add visitor session reconstruction (group requests by IP + UA + time)
- [ ] Add entry/exit page tracking
- [ ] Add bounce rate calculation
- [ ] Add configurable visit timeout (currently hardcoded at 30 minutes)

### Improved Bot Detection

- [ ] Move bot signatures to external config file (easier updates)
- [ ] Add bot intent classification (SEO crawler, spam, monitoring, AI)
- [ ] Add case-insensitive bot matching
- [ ] Add community-contributed bot list support
- [ ] Track bot-specific metrics separately (requests, bandwidth)

### Multi-Site Management

- [ ] Add site management API (`GET/POST /api/sites`)
- [ ] Add per-site retention policies
- [ ] Add cross-site aggregate view
- [ ] Add site-specific authentication/permissions

### Alerting System

- [ ] Add alerting framework (email, webhook)
- [ ] Alert on high error rate (5xx spike)
- [ ] Alert on traffic anomalies (sudden spike/drop)
- [ ] Alert on specific status codes (404 threshold)
- [ ] Add alert configuration via env vars or config file

### Report Generation

- [ ] Add scheduled report generation (daily/weekly email)
- [ ] Add PDF report export
- [ ] Add customizable report templates
- [ ] Add report history/archive

---

## Frontend Improvements

### UI/UX Enhancements

- [ ] Add loading indicators for API calls
- [ ] Add error messages when API calls fail
- [ ] Add pagination for large tables (visitors, paths, referrers)
- [ ] Make "top N" limits configurable in UI (top 5 → top 10/25/50)
- [ ] Add date range picker for archive view
- [ ] Add "compare to previous period" feature
- [ ] Add keyboard shortcuts (R = refresh, L = live view, A = archive)

### Data Visualization

- [ ] Add Chart.js or similar library for better charts
- [ ] Add line charts for time series (hourly/daily trends)
- [ ] Add pie/donut charts for browser/OS distribution
- [ ] Add heatmap for traffic by hour-of-day and day-of-week
- [ ] Add world map visualization for geo data
- [ ] Add sparklines for quick trend indicators

### Responsive & Accessibility

- [ ] Audit and fix mobile layout issues
- [ ] Add ARIA labels for screen readers
- [ ] Ensure proper color contrast in both themes
- [ ] Add skip-to-content link
- [ ] Test with keyboard-only navigation

### Code Organization

- [ ] Split `index.html` into components (Alpine.js components or partials)
- [ ] Add TypeScript for frontend JavaScript
- [ ] Bundle and minify JS/CSS for production
- [ ] Add service worker for offline access to cached stats

---

## Low Priority: Nice-to-Have

### CLI Tools

- [ ] Add `caddystat import <file>` command for manual imports
- [ ] Add `caddystat export` command for data dumps
- [ ] Add `caddystat query` command for ad-hoc SQL queries
- [ ] Add `caddystat config` command to validate/print configuration

### Configuration

- [ ] Add YAML/TOML config file support (in addition to env vars)
- [ ] Add config validation on startup (warn about invalid combinations)
- [ ] Add web-based configuration UI (admin panel)
- [ ] Add config reload without restart (SIGHUP handler)

### API Improvements

- [ ] Add API versioning (`/api/v1/*`)
- [ ] Add OpenAPI/Swagger documentation
- [ ] Add API rate limiting per token
- [ ] Add API key authentication (for integrations)
- [ ] Add CORS configuration for external dashboards

### Advanced Features

- [ ] Add real-time visitor tracking (who's on site now)
- [ ] Add funnel analysis (conversion tracking)
- [ ] Add custom event tracking (JavaScript snippet)
- [ ] Add URL campaign tracking (UTM parameters)
- [ ] Add A/B testing support
- [ ] Add user accounts with roles (admin, viewer, per-site)

### Documentation

- [ ] Add architecture diagram to README
- [ ] Add API documentation with examples
- [ ] Add deployment guide (Docker, systemd, Kubernetes)
- [ ] Add troubleshooting guide
- [ ] Add contributing guide with code style requirements
- [ ] Add changelog (CHANGELOG.md)

### Plugin System

- [ ] Design plugin architecture (Go plugins or external processes)
- [ ] Add custom metric extractors
- [ ] Add custom output formats
- [ ] Add custom bot classifiers
- [ ] Add webhook integrations

---

## Quick Wins (Can Do Today)

These are small improvements that provide immediate value:

- [x] Add version number to startup log and `/health` endpoint
- [x] Add `--version` flag to CLI
- [x] Add favicon to web UI
- [x] Add "last updated" timestamp to dashboard
- [x] Fix inconsistent port documentation (8404 vs 8000)
- [x] Add `robots.txt` to prevent search engine indexing of dashboard
- [x] Add `X-Robots-Tag: noindex` header to all responses
- [x] Document all environment variables in README
- [x] Add `.gitignore` entries for common editor files
- [x] Add Docker health check in Dockerfile

---

## Bug Fixes

Known issues to address:

- [x] Sessions lost on container restart (implement persistent sessions)
- [ ] Import progress only saved every 10,000 rows (reduce to 1,000)
- [ ] `ListenAddr` default mismatch between config (`:8404`) and docs (`:8000`)
- [ ] Timezone handling: ensure frontend displays times correctly
- [ ] SSE broadcasts can be dropped silently under high load
- [ ] No error shown in UI when API requests fail
- [ ] MacOS version detection is basic/incomplete
- [ ] Kindle/Playbook device detection may be incomplete

---

## Technical Debt

Items that should be addressed for long-term maintainability:

- [ ] Refactor large `storage.go` (1277 lines) into smaller files
- [ ] Extract query building logic into separate functions
- [ ] Add interfaces for storage layer (enables testing with mocks)
- [ ] Add interfaces for SSE hub (enables testing)
- [ ] Document all exported functions and types
- [ ] Add golangci-lint configuration and fix issues
- [ ] Update dependencies to latest versions
- [ ] Add dependabot configuration for security updates

---

## Progress Tracking

| Category                 | Total   | Completed | Percentage |
| ------------------------ | ------- | --------- | ---------- |
| Security & Stability     | 17      | 16        | 94%        |
| Performance & Operations | 16      | 0         | 0%         |
| New Features             | 20      | 0         | 0%         |
| Frontend                 | 18      | 0         | 0%         |
| Nice-to-Have             | 21      | 0         | 0%         |
| Quick Wins               | 10      | 10        | 100%       |
| Bug Fixes                | 8       | 1         | 13%        |
| Technical Debt           | 8       | 0         | 0%         |
| **Total**                | **118** | **27**    | **23%**    |

---

## Suggested Order of Attack

1. **Start with Quick Wins** - Build momentum with easy victories
2. **Add Health Endpoint** - Essential for production deployments
3. **Add Structured Logging** - Makes debugging everything else easier
4. **Write Tests for Ingest** - Most critical path, highest risk area
5. **Fix Session Persistence** - Security issue that affects usability
6. **Add Rate Limiting** - Basic security requirement
7. **Add Prometheus Metrics** - Enables monitoring as you build more
8. **Improve Frontend Error Handling** - Better user experience
9. **Add Chart.js** - Visual improvements with high impact
10. **Add Data Export** - Frequently requested feature

Each task is designed to be completable in a single focused session. Mark items `[x]` as you complete them and update the progress table periodically.
