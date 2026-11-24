DB ?= tkb.db

.PHONY: build test seed run report snapshot lint

build:
	go build -o bin/tkb-api ./cmd/api
	go build -o bin/tkb-seed ./cmd/seed
	go build -o bin/tkb ./cmd/tkb

test:
	go test -race ./...

seed:
	go run ./cmd/seed -db $(DB) -loans 5000

run:
	DB_PATH=$(DB) go run ./cmd/api

report:
	go run ./cmd/tkb -db $(DB) -as-of $(or $(AS_OF),2025-07-31)

lint:
	go vet ./...
