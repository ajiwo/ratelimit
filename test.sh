#!/bin/bash
#

set -e

go test -count=1 -timeout=30s -v -race  -coverprofile=coverage.out \
  . ./strategies ./backends ./backends/memory 
go tool cover -func=coverage.out 

cd backends/postgres
go test -count=1 -timeout=30s -v -race  -coverprofile=coverage.out . 
go tool cover -func=coverage.out 

cd ../redis
go test -count=1 -timeout=30s -v -race  -coverprofile=coverage.out . 
go tool cover -func=coverage.out

cd ../..
# Combine all for report submission
tail -n +2 ./backends/postgres/coverage.out >> coverage.out
tail -n +2 ./backends/redis/coverage.out >> coverage.out
