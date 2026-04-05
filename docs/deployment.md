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
┌─────────┐
│  Caddy  │ :80 (redirects to HTTPS)
│         │ :443 (HTTPS with auto Let's Encrypt)
└────┬────┘
     │
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
