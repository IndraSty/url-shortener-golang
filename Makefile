.PHONY: run build test lint migrate-up migrate-down swagger tidy

APP_NAME=url-shortener
MAIN_PATH=./cmd/api/main.go
MIGRATE_PATH=./migrations
DB_URL=$(shell grep DATABASE_URL .env | cut -d '=' -f2-)

## run: start the development server with live .env
run:
	go run $(MAIN_PATH)

## build: compile to binary
build:
	go build -o bin/$(APP_NAME) $(MAIN_PATH)

## test: run all tests with race detector
test:
	go test -race -v ./...

## bench: run benchmark tests
bench:
	go test -bench=. -benchmem ./...

## lint: run golangci-lint
lint:
	golangci-lint run ./...

## tidy: tidy and verify go modules
tidy:
	go mod tidy
	go mod verify

## swagger: generate swagger docs
swagger:
	swag init -g $(MAIN_PATH) -o ./docs

## migrate-up: run all pending migrations
migrate-up:
	migrate -path $(MIGRATE_PATH) -database "$(DB_URL)" up

## migrate-down: rollback last migration
migrate-down:
	migrate -path $(MIGRATE_PATH) -database "$(DB_URL)" down 1

## migrate-create: create new migration (usage: make migrate-create name=create_something)
migrate-create:
	migrate create -ext sql -dir $(MIGRATE_PATH) -seq $(name)

## generate-secret: generate a secure random secret for JWT
generate-secret:
	openssl rand -hex 32

## help: list all available make targets
help:
	@echo "Available targets:"
	@grep -E '^##' Makefile | sed 's/## /  /'