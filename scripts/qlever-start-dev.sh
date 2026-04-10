#!/bin/bash
#
# Start QLever triplestore for local development.
#
# Runs QLever as a persistent Docker container on port 7001.
# The container survives go run restarts — stop/start it independently.
#
# Usage:
#   ./scripts/qlever-start-dev.sh          # start (or resume) QLever
#   docker stop qlever                      # stop
#   docker start qlever                     # resume
#   docker logs -f qlever                   # view logs
#
# Data loading:
#   Bulk (re)index — stop server, index all files in qlever/data/, restart:
#     docker stop qlever
#     docker exec qlever qlever index
#     docker start qlever
#
#   Incremental (SPARQL UPDATE, no restart needed):
#     curl -X POST http://localhost:7001 \
#       -H "Content-Type: application/sparql-update" \
#       -d 'INSERT DATA { <http://example.org/s> <http://example.org/p> "o" . }'
#
# Note: heavy SPARQL UPDATE volume degrades query performance.
# Plan a periodic full re-index if you use many incremental updates.
#

set -e

CONTAINER_NAME="qlever"
PORT="7001"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DATA_DIR="${SCRIPT_DIR}/../qlever/data"

# Ensure data directory exists
mkdir -p "${DATA_DIR}"

# If container already exists, just start it
if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
    if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        echo "QLever is already running (port ${PORT})"
    else
        echo "Resuming existing QLever container..."
        docker start "${CONTAINER_NAME}"
        echo "QLever started on port ${PORT}"
    fi
    exit 0
fi

# Build a bootstrap index if none exists yet
if [ ! -f "${DATA_DIR}/visoto.index.pos" ]; then
    echo "No index found — building bootstrap index..."
    BOOTSTRAP_TTL="${DATA_DIR}/.bootstrap.ttl"
    echo '@prefix ex: <http://example.org/> . ex:init ex:status "bootstrapped" .' > "${BOOTSTRAP_TTL}"
    docker run --rm \
        -w /data \
        -v "$(realpath "${DATA_DIR}"):/data" \
        -u "$(id -u):$(id -g)" \
        --entrypoint /qlever/qlever-index \
        docker.io/adfreiburg/qlever \
        -i visoto \
        -f .bootstrap.ttl \
        -F ttl 2>&1 | grep -E "INFO|WARN|ERROR|completed|failed"
    echo "Bootstrap index built."
fi

echo "Starting QLever container..."
docker run -d \
    --name "${CONTAINER_NAME}" \
    -p "${PORT}:${PORT}" \
    -v "$(realpath "${DATA_DIR}"):/data" \
    -w /data \
    -u "$(id -u):$(id -g)" \
    --entrypoint /qlever/qlever-server \
    --restart unless-stopped \
    docker.io/adfreiburg/qlever \
    -i visoto \
    -p "${PORT}" \
    -m 1.5G \
    -c 1G \
    -e 256M \
    -s 60s \
    -a tOLk1n4gdSt00rYs

echo "QLever started on port ${PORT}"
echo "SPARQL endpoint: http://localhost:${PORT}"
echo ""
echo "Add to visoto.config:"
echo "  [[application.sparqlEndpoints]]"
echo "  name = \"QLever (local)\""
echo "  url = \"http://localhost:${PORT}\""
echo "  default = false"
echo "  monitor = true"
echo "  tag = \"qlever\""
