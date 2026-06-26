.PHONY: help run build test tidy fmt vet lint migrate-up migrate-down docker-up docker-down

APP        := go-backend
BIN        := bin/$(APP)
MAIN       := ./cmd/api
MIGRATIONS := ./migrations
DATABASE_URL ?= postgres://postgres:postgres@localhost:5432/go_backend?sslmode=disable

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2}'

run: ## Run the API locally
	go run $(MAIN)

build: ## Build the API binary
	go build -o $(BIN) $(MAIN)

test: ## Run tests
	go test ./... -race -cover

tidy: ## Tidy go modules
	go mod tidy

fmt: ## Format code
	go fmt ./...

vet: ## Run go vet
	go vet ./...

migrate-up: ## Apply database migrations (requires golang-migrate)
	migrate -path $(MIGRATIONS) -database "$(DATABASE_URL)" up

migrate-down: ## Roll back the last migration
	migrate -path $(MIGRATIONS) -database "$(DATABASE_URL)" down 1

docker-up: ## Start postgres + app via docker compose
	docker compose up -d --build

docker-down: ## Stop docker compose stack
	docker compose down
