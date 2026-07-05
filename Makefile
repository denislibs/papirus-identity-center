.PHONY: test test-unit run wire build

test:
	go test ./...

test-unit:
	go test -short ./...

run:
	go run ./cmd/server

wire:
	go run github.com/google/wire/cmd/wire ./internal/infrastructure/di

build:
	go build -o bin/server ./cmd/server
