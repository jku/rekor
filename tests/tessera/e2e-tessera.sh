#!/bin/bash
set -e

# A quick end-to-end test for rekor with Tessera:
# * Generates some entries (not even real entry types)
# * Checks that Rekor HTTP API seems to work

# Build the generator and rekor-cli
cd tests/tessera/gen-static-data
go build -o gen-data
cd ../../..
go build -o tests/tessera/rekor-cli cmd/rekor-cli/main.go

# Create data dir
DATADIR="tests/tessera/data"
rm -rf $DATADIR
mkdir -p $DATADIR

# temp HOME to avoid rekor-cli state pollution
TEST_HOME=$(mktemp -d)
export HOME=$TEST_HOME

# Generate data
./tests/tessera/gen-static-data/gen-data $DATADIR


echo "Starting rekor-server via Docker Compose..."

docker compose -f docker-compose.tessera.yml up -d --build --wait
trap "docker compose -f docker-compose.tessera.yml down; rm -rf $DATADIR; rm -f ./tests/tessera/rekor-cli; rm -rf $TEST_HOME" EXIT

echo "Server is ready! Running tests..."

# Fetch log info
CLI="./tests/tessera/rekor-cli --rekor_server http://127.0.0.1:3001"
$CLI loginfo

# We use curl to fetch entries because rekor-cli strictly validates signatures,
# which we don't generate in our static test data.
echo "Fetching entry 0 via curl..."
curl -s http://localhost:3001/api/v1/log/entries?logIndex=0 | jq .

echo "Fetching entry 1 via curl..."
curl -s http://localhost:3001/api/v1/log/entries?logIndex=1 | jq .

echo "E2E test passed!"
