# Visoto Deployment Guide

This guide covers deploying Visoto to a production server using Docker with Caddy as a reverse proxy for automatic HTTPS.

## Prerequisites

- A server with SSH access (port 2222)
- Domain name pointing to the server (`visoto.hutzli.org`)
- Ports 80 and 443 open on the server firewall

## Quick Deploy

From your local machine, run:

```bash
./deploy.sh <server-ip>
```

This script will:
1. Check SSH connectivity
2. Install Docker if not present
3. Create `/opt/visoto` directory
4. Copy all project files
5. Build and start the containers
6. Verify the deployment

## Architecture

```
Internet
    │
    ▼
┌─────────┐   access.log (JSON)   ┌───────────┐
│  Caddy  │ ────────────────────> │ GoAccess  │
│         │ :80 (→ HTTPS)         │ (5 min    │
│         │ :443 (auto TLS)       │  refresh) │
└────┬────┘ <──────────────────── └───────────┘
     │         report HTML
     │         (served at /goaccess/, PUBLIC)
     ▼
┌─────────┐
│ Visoto  │ :8060 (internal only)
│  (Go)   │
└────┬────┘
     │
     ▼
┌─────────────────┐
│ LINDAS Triple   │
│ Store (external)│
│ ld.admin.ch     │
└─────────────────┘
```

## Files

| File | Purpose |
|------|---------|
| `Dockerfile` | Multi-stage build for the Go application |
| `docker-compose.yml` | Orchestrates Visoto and Caddy containers |
| `Caddyfile` | Caddy reverse proxy configuration |
| `visoto.config` | Application configuration (SPARQL endpoint, port, etc.) |
| `deploy.sh` | Deployment script |
| `scripts/test-goaccess-format.sh` | Guards the Caddy-log → GoAccess format contract |

## Server Management

SSH into the server and navigate to the project:

```bash
ssh -p 2222 hePeter@<server-ip>
cd /opt/visoto
```

### Common Commands

```bash
# View logs (all services)
docker compose logs -f

# View logs (specific service)
docker compose logs visoto -f
docker compose logs caddy -f

# Restart services
docker compose restart

# Stop services
docker compose down

# Start services
docker compose up -d

# Rebuild after code changes
docker compose up -d --build
```

## Configuration

### Application Config (`visoto.config`)

```toml
[application]
port = 8060
sparqlEndpoint = "https://ld.admin.ch/query/"
timeout = 30

[logging]
level = "DEBUG"  # DEBUG, INFO, WARN, ERROR
format = "text"  # text or json
output = "stdout"
```

### Caddy Config (`Caddyfile`)

```
visoto.hutzli.org {
    reverse_proxy visoto:8060
}
```

## Analytics (GoAccess)

Visitor and performance statistics are available at
**<https://visoto.hutzli.org/goaccess/>**.

> ⚠️ **The dashboard is currently PUBLIC — no authentication.** Anyone who knows
> the URL can read it. It exposes top URLs, referrers, user agents and visitor
> IPs (anonymized to /24). See **Access control** below for how to close it.

Caddy writes a JSON access log to the `caddy_logs` volume; a `goaccess`
container re-parses it every 5 minutes into a static HTML report, which Caddy
serves read-only from the `goaccess_report` volume. The dashboard is therefore
**up to 5 minutes behind** — reload the page to pick up a new report. There is
no live/WebSocket mode by design (it would require loosening the site's CSP).

The report covers both halves of the picture: unique visitors, top URLs,
referrers, status codes and user agents, plus a **Time Served** panel with
per-URL latency percentiles for finding slow pages.

Client IPs are anonymized (last IPv4 octet zeroed) and Docker-internal traffic
(healthchecks, the app's own cache purge) is excluded from visitor counts.

### Access control

The dashboard is **open to the internet**. To close it, edit the
`route /goaccess/*` block in the `Caddyfile`.

**Recommended — private ranges only**, viewed over an SSH tunnel. Mirrors the
`/souin-api/*` lockdown and puts no credential in this public repo:

```
route /goaccess/* {
    @external not remote_ip private_ranges
    respond @external 403

    uri strip_prefix /goaccess
    ...
}
```

Then reach it with:

```bash
ssh -p 2222 -L 8080:localhost:80 <user>@<server>
curl -H 'Host: visoto.hutzli.org' http://localhost:8080/goaccess/
```

(The `Host` header is required — Caddy routes by hostname, and `localhost`
does not match the site block.)

#### Why not basic auth

Tried and reverted on 2026-09-17. Two findings worth keeping:

**`basic_auth` does not expand `{env.*}` placeholders.** Caddy stores the
literal string `{env.GOACCESS_HASH}` as the password. Confirm with:

```bash
docker compose exec caddy caddy adapt --config /etc/caddy/Caddyfile --adapter caddyfile \
  | grep -o 'accounts[^]]*]'
```

**An empty or invalid password is a config-load error, not a login failure.**
Caddy exits with `account 0: username and password are required` and
crash-loops, so the **whole site** goes down — not just `/goaccess/`. This is
the opposite of the fail-closed behaviour you might assume.

Together these mean basic auth here requires a literal bcrypt hash committed to
a public repo, where it becomes a permanent offline-cracking target next to the
hostname it guards. Hence the source-restriction approach above.

Always validate before deploying a `Caddyfile` change, using the project's own
image (stock `caddy:alpine` lacks the cache and ratelimit plugins and fails on
unrelated directives):

```bash
docker run --rm --entrypoint sh -v "$PWD/Caddyfile:/etc/caddy/Caddyfile:ro" \
  visoto-caddy -c "caddy validate --config /etc/caddy/Caddyfile --adapter caddyfile"
```

**Keep it a `route`, not a `handle`.** Caddy re-sorts directives by type and
`file_server` sorts **before** `respond` — including inside a `handle` block,
which re-sorts its nested directives too. Written as a `handle`, the 403 loses
to `file_server` and the lockdown **fails open** while looking correct. `route`
is the only construct that preserves written order. The sibling form used for
`/souin-api/*` is safe only because nothing else handles that path; it does not
transfer to a path backed by a `file_server`.

**Verify both directions** — a passing internal check proves nothing about the
external one. Note `-I` sends a HEAD request, which the Go app does not route
and answers 404; use `-o /dev/null -w '%{http_code}'` to test app paths.

```bash
curl -s -o /dev/null -w '%{http_code}\n' https://visoto.hutzli.org/goaccess/  # 403
# then the tunneled request                                                    # 200
curl -s -o /dev/null -w '%{http_code}\n' https://visoto.hutzli.org/           # 200
```

That last check is not optional: a bad `route` block can stop Caddy loading its
config entirely, which takes the whole site down, not just this path.

Finish with `docker compose up -d --force-recreate caddy` and commit the change
— the `Caddyfile` deploys from the repo, so otherwise the next `./deploy.sh`
overwrites it.

### Troubleshooting an empty dashboard

The most common failure is silent: GoAccess parses **zero** lines, exits 0, and
writes an empty report. It is almost always a log-format mismatch.

```bash
docker compose logs goaccess          # "No valid hits" == format mismatch
docker compose exec caddy head -1 /var/log/caddy/access.log
```

Caddy's JSON encoder writes `ts` as a Unix epoch float, which is why the
`goaccess` service passes `--date-format=%s --time-format=%s`. If a Caddy
upgrade ever changes that, run the guard from a checkout:

```bash
./scripts/test-goaccess-format.sh                       # built-in fixture
./scripts/test-goaccess-format.sh /path/to/access.log   # a real log
```

If panels render but are blank, check the browser console for a Content-Security-Policy
violation — the site-wide CSP also applies to the report.

### Data retention

| Volume | Holds | Lost on `down -v` |
|--------|-------|-------------------|
| `caddy_logs` | Raw JSON access log, rolled at 50 MiB × 5 files | Yes |
| `goaccess_db` | Cumulative visitor totals (survives log rotation) | Yes |
| `goaccess_report` | The generated HTML (regenerates in ≤5 min) | Harmless |

> **Warning:** `docker compose down -v` deletes **all** named volumes, including
> `visoto_data` (the endpoint-monitoring history) and `goaccess_db` (all
> accumulated visitor statistics). Use plain `down` to stop the stack.

## Updating the Application

1. Make changes locally
2. Run the deploy script:
   ```bash
   ./deploy.sh <server-ip>
   ```

The script will copy files and rebuild the containers.

## Troubleshooting

### Container won't start

Check logs:
```bash
docker compose logs visoto
```

### HTTPS certificate issues

Check Caddy logs:
```bash
docker compose logs caddy
```

Ensure:
- DNS is properly configured for `visoto.hutzli.org`
- Ports 80 and 443 are open
- No other service is using these ports

### Can't connect to LINDAS triple store

The container needs outgoing HTTPS access to `ld.admin.ch`. Check:
```bash
docker compose exec visoto wget -q -O- https://ld.admin.ch/query/
```

### Health check failing

Test the ping endpoint:
```bash
curl http://localhost:8060/ping
```

Should return `pong`.
