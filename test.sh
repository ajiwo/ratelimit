#!/bin/sh
#

go test -count=1 -timeout=300s -v -race  -coverprofile=coverage.out \
  . ./backends/... ./strategies/...  && \
  go tool cover -func=coverage.out \

