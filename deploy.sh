#!/bin/bash
#
# Visoto Deployment Script
# Deploys Visoto to a remote server via SSH
#
# Usage: ./deploy.sh <server> [user] [--with-qlever]
# Example: ./deploy.sh 192.168.1.100 hePeter
# Example with QLever: ./deploy.sh 192.168.1.100 hePeter --with-qlever
#

set -e

# Configuration
REMOTE_DIR="/opt/visoto"
DEFAULT_USER="hePeter"
SSH_PORT="2222"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Parse arguments
SERVER="${1}"
USER="${2:-$DEFAULT_USER}"
# $2 is positional, so `./deploy.sh <server> --with-qlever` would otherwise set
# USER to the flag and fail later as an opaque "cannot connect" error.
case "$USER" in --*) USER="$DEFAULT_USER" ;; esac
WITH_QLEVER=false

for arg in "$@"; do
    if [ "$arg" = "--with-qlever" ]; then
        WITH_QLEVER=true
    fi
done

if [ -z "$SERVER" ]; then
    echo -e "${RED}Error: Server address required${NC}"
    echo "Usage: ./deploy.sh <server> [user] [--with-qlever]"
    echo "Example: ./deploy.sh 192.168.1.100 hePeter"
    echo "Example with QLever: ./deploy.sh 192.168.1.100 hePeter --with-qlever"
    exit 1
fi

SSH_TARGET="${USER}@${SERVER}"
SSH_OPTS="-p ${SSH_PORT}"

echo -e "${GREEN}=== Visoto Deployment ===${NC}"
echo "Server: ${SERVER}"
echo "User: ${USER}"
echo "SSH Port: ${SSH_PORT}"
echo "Remote directory: ${REMOTE_DIR}"
echo "QLever: ${WITH_QLEVER}"
echo ""

# Step 1: Check SSH connectivity
echo -e "${YELLOW}[1/6] Checking SSH connectivity...${NC}"
if ! ssh ${SSH_OPTS} -o ConnectTimeout=5 "${SSH_TARGET}" "echo 'SSH OK'" > /dev/null 2>&1; then
    echo -e "${RED}Error: Cannot connect to ${SSH_TARGET} on port ${SSH_PORT}${NC}"
    echo "Make sure SSH is configured and you can connect to the server."
    exit 1
fi
echo "SSH connection OK"

# Step 2: Check/Install Docker on remote
echo -e "${YELLOW}[2/6] Checking Docker installation...${NC}"
if ! ssh ${SSH_OPTS} "${SSH_TARGET}" "command -v docker" > /dev/null 2>&1; then
    echo "Docker not found. Installing..."
    ssh ${SSH_OPTS} "${SSH_TARGET}" "curl -fsSL https://get.docker.com | sudo sh"
    ssh ${SSH_OPTS} "${SSH_TARGET}" "sudo usermod -aG docker ${USER}"
    echo -e "${YELLOW}NOTE: You may need to logout and login again for docker group to take effect${NC}"
fi
echo "Docker OK"

# Step 3: Create remote directory
echo -e "${YELLOW}[3/6] Creating remote directory...${NC}"
ssh ${SSH_OPTS} "${SSH_TARGET}" "sudo mkdir -p ${REMOTE_DIR} && sudo chown ${USER}:${USER} ${REMOTE_DIR}"
if [ "$WITH_QLEVER" = true ]; then
    ssh ${SSH_OPTS} "${SSH_TARGET}" "mkdir -p ${REMOTE_DIR}/qlever/data"
fi
echo "Directory created: ${REMOTE_DIR}"

# Step 4: Copy files to server
echo -e "${YELLOW}[4/6] Copying files to server...${NC}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SCP_OPTS="-P ${SSH_PORT}"

# Copy essential files
scp ${SCP_OPTS} "${SCRIPT_DIR}/Dockerfile" "${SSH_TARGET}:${REMOTE_DIR}/"
scp ${SCP_OPTS} "${SCRIPT_DIR}/docker-compose.yml" "${SSH_TARGET}:${REMOTE_DIR}/"
scp ${SCP_OPTS} "${SCRIPT_DIR}/Caddyfile" "${SSH_TARGET}:${REMOTE_DIR}/"
scp ${SCP_OPTS} "${SCRIPT_DIR}/visoto.config" "${SSH_TARGET}:${REMOTE_DIR}/"
# .env carries the secrets that visoto.config and docker-compose.yml reference
# as ${VAR}. It is gitignored, so it must be copied explicitly — without it
# compose cannot resolve QLEVER_ACCESS_TOKEN and the app exits at startup.
if [ -f "${SCRIPT_DIR}/.env" ]; then
    scp ${SCP_OPTS} "${SCRIPT_DIR}/.env" "${SSH_TARGET}:${REMOTE_DIR}/"
    ssh ${SSH_OPTS} "${SSH_TARGET}" "chmod 600 ${REMOTE_DIR}/.env"
elif [ "$WITH_QLEVER" = true ]; then
    echo -e "${RED}Error: --with-qlever needs a .env with QLEVER_ACCESS_TOKEN${NC}"
    echo "Copy .env.example to .env and set a token."
    exit 1
fi
scp ${SCP_OPTS} "${SCRIPT_DIR}/go.mod" "${SSH_TARGET}:${REMOTE_DIR}/"
scp ${SCP_OPTS} "${SCRIPT_DIR}/go.sum" "${SSH_TARGET}:${REMOTE_DIR}/"
if [ "$WITH_QLEVER" = true ]; then
    # Write init.ttl on the remote rather than copying it: qlever/data/ is
    # gitignored, so the file is absent on a fresh clone and the scp would abort
    # the deploy. Its content is irrelevant — qlever-server refuses to start
    # without an index, so qlever-init builds one from this single filler triple.
    # Real data arrives later via /api/upload or a bulk reindex, and the
    # `[ ! -f /data/visoto.index.pos ]` guard makes this a no-op on redeploy.
    ssh ${SSH_OPTS} "${SSH_TARGET}" "cat > ${REMOTE_DIR}/qlever/data/init.ttl" <<'EOF'
@prefix ex: <http://example.org/> . ex:init ex:status "bootstrapped" .
EOF
fi

# Remove source directories first to avoid stale files from previous deploys
ssh ${SSH_OPTS} "${SSH_TARGET}" "rm -rf ${REMOTE_DIR}/cmd ${REMOTE_DIR}/internal ${REMOTE_DIR}/templates ${REMOTE_DIR}/static ${REMOTE_DIR}/locales"

# Copy source directories
scp ${SCP_OPTS} -r "${SCRIPT_DIR}/cmd" "${SSH_TARGET}:${REMOTE_DIR}/"
scp ${SCP_OPTS} -r "${SCRIPT_DIR}/internal" "${SSH_TARGET}:${REMOTE_DIR}/"
scp ${SCP_OPTS} -r "${SCRIPT_DIR}/templates" "${SSH_TARGET}:${REMOTE_DIR}/"
scp ${SCP_OPTS} -r "${SCRIPT_DIR}/static" "${SSH_TARGET}:${REMOTE_DIR}/"
scp ${SCP_OPTS} -r "${SCRIPT_DIR}/locales" "${SSH_TARGET}:${REMOTE_DIR}/"

echo "Files copied"

# Step 5: Build and start container
echo -e "${YELLOW}[5/6] Building and starting container...${NC}"
if [ "$WITH_QLEVER" = true ]; then
    ssh ${SSH_OPTS} "${SSH_TARGET}" "cd ${REMOTE_DIR} && docker compose --profile qlever up -d --build"
else
    ssh ${SSH_OPTS} "${SSH_TARGET}" "cd ${REMOTE_DIR} && docker compose up -d --build"
fi

# Recreate Caddy: `up -d` above does not touch it (the Caddyfile is a bind mount
# and the xcaddy image build is layer-cached), so without this the old Caddyfile
# stays loaded and the in-memory Souin cache keeps serving pre-deploy responses.
#
# `up -d --force-recreate`, not `restart`: a plain restart reuses the existing
# container, which can leave stale state behind. Recreating reloads the config
# and clears the cache in one stroke.
ssh ${SSH_OPTS} "${SSH_TARGET}" "cd ${REMOTE_DIR} && docker compose up -d --force-recreate caddy"
echo "Caddy recreated (fresh Caddyfile + empty cache)"

# Step 6: Verify deployment
echo -e "${YELLOW}[6/6] Verifying deployment...${NC}"
# Poll the container's own healthcheck rather than curling the host: visoto uses
# `expose: 8060`, not `ports:`, so port 8060 is reachable only on the container
# network and a probe from the host always gets connection-refused. Polling also
# rides out the ~3s the app spends loading config, locales, icons and templates.
HEALTH_TIMEOUT=60

if ssh ${SSH_OPTS} "${SSH_TARGET}" "for i in \$(seq 1 ${HEALTH_TIMEOUT}); do [ \"\$(docker inspect -f '{{.State.Health.Status}}' visoto 2>/dev/null)\" = healthy ] && exit 0; sleep 1; done; exit 1"; then
    echo -e "${GREEN}✓ Deployment successful!${NC}"
    echo ""
    echo "Visoto is running at:"
    echo "  https://visoto.hutzli.org"
    echo ""
    echo "Analytics dashboard (PUBLIC - no auth; first report ~5 min after deploy):"
    echo "  https://visoto.hutzli.org/goaccess/"
    echo ""
    echo "Useful commands on the server:"
    echo "  cd ${REMOTE_DIR}"
    echo "  docker compose logs -f                          # View all logs"
    echo "  docker compose logs caddy -f                    # View Caddy logs"
    echo "  docker compose logs goaccess -f                 # View analytics generator logs"
    echo "  docker compose restart goaccess                 # Force a report rebuild"
    echo "  docker compose restart                          # Restart services"
    echo "  docker compose down                             # Stop services"
    if [ "$WITH_QLEVER" = true ]; then
        echo "  docker compose --profile qlever logs qlever -f # View QLever logs"
        echo "  docker compose --profile qlever down            # Stop all incl. QLever"
        echo "  docker volume rm visoto_qlever_data             # Delete ONLY QLever index (preserves monitoring DB)"
        echo "  WARNING: 'down -v' deletes ALL volumes incl. the monitoring database!"
    fi
else
    echo -e "${RED}Warning: Health check failed after ${HEALTH_TIMEOUT}s${NC}"
    echo "The container is not answering /ping. Check logs with:"
    echo "  ssh ${SSH_OPTS} ${SSH_TARGET} 'cd ${REMOTE_DIR} && docker compose logs'"
fi
