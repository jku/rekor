#!/bin/bash
set -e

# Build the generator
cd tests/tessera/gen-static-data
go build -o gen-data
cd ../../..

# Create data dir
DATADIR="tests/tessera/data"
rm -rf $DATADIR
mkdir -p $DATADIR

# Generate data
./tests/tessera/gen-static-data/gen-data $DATADIR

PORT=3001

echo "Starting rekor-server via Docker Compose..."
docker compose -f docker-compose.tessera.yml up -d --build --wait

trap "docker compose -f docker-compose.tessera.yml down; rm -rf $DATADIR" EXIT

echo "Server is ready! Running tests..."

# Fetch log info
curl -s http://127.0.0.1:$PORT/api/v1/log | jq .

# Fetch entry 0
curl -s http://127.0.0.1:$PORT/api/v1/log/entries?logIndex=0 | jq .

# Fetch entry 1
curl -s http://127.0.0.1:$PORT/api/v1/log/entries?logIndex=1 | jq .

echo "E2E test passed!"
