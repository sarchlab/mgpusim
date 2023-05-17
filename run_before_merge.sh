#!/bin/bash

set -o xtrace

go get -u ./...
go get -t -u ./...
go mod tidy

go generate ./...

go build ./...

curl -sfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh| sh -s -- -b $(go env GOPATH)/bin 
golangci-lint run ./...

go install github.com/onsi/ginkgo/v2/ginkgo
ginkgo -r

