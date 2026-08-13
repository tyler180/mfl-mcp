.PHONY: build test tidy

build:
	go build -o bin/mfl-mcp ./cmd/mfl-mcp

test:
	go test ./...

tidy:
	go mod tidy
