#!/bin/bash
#

set -e

go test -count=1 -timeout=30s -race -coverprofile=coverage.out ./...

cd backends/postgres
go test -count=1 -timeout=30s -race -coverprofile=coverage.out . 

cd ../redis
go test -count=1 -timeout=30s -race -coverprofile=coverage.out . 

cd ../..

# Run integration tests
echo "Running integration tests..."
cd tests
sync
sleep 2
go test -count=1 -timeout=120s -race  .

cd ..
# Combine all for report submission
tail -n +2 ./backends/postgres/coverage.out >> coverage.out
tail -n +2 ./backends/redis/coverage.out >> coverage.out

# cd to tests module, because it has all the required dependencies required to display report for all modules
cd tests
go tool cover -func=../coverage.out
