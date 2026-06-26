# go-backend

A Go backend service using a standard layered architecture (Gin + PostgreSQL).

## Stack

| Concern        | Choice                          |
| -------------- | ------------------------------- |
| Language       | Go 1.26                         |
| HTTP framework | [Gin](https://gin-gonic.com)    |
| Database       | PostgreSQL via `pgx/v5`         |
| Migrations     | `golang-migrate` (SQL files)    |
| Config         | Viper (env + `.env`)            |
| Logging        | `log/slog` (structured)         |

## Architecture

Layered, with dependencies flowing inward and wired in `cmd/api/main.go`:

```
handler (HTTP)  ->  service (business logic)  ->  repository (data access)
                         |
                       model (domain types)
```

```
cmd/api/            entrypoint + dependency wiring
internal/
  config/           configuration loading
  database/         pgx connection pool
  handler/          HTTP handlers + health probes
  service/          business logic
  repository/       Postgres data access
  model/            domain entities & DTOs
  server/           router + middleware
pkg/logger/         structured logger
migrations/         SQL migrations
docs/adr/           architecture decision records
```

See [docs/adr/0001-initial-architecture.md](docs/adr/0001-initial-architecture.md).

## Getting started

```bash
# 1. Configure environment
cp .env.example .env

# 2. Start PostgreSQL locally and create the database
# expected defaults: postgres/postgres on localhost:5432
createdb -U postgres go_backend

# 3. Install dependencies and run migrations
make tidy          # download dependencies
make migrate-up    # apply migrations (needs golang-migrate installed)

# 4. Start the API locally
make run           # start the API on :8080

# --- or use Docker for Postgres + API ---
make docker-up
```

## Endpoints

| Method | Path              | Description          |
| ------ | ----------------- | -------------------- |
| GET    | `/healthz`        | Liveness probe       |
| GET    | `/readyz`         | Readiness (DB check) |
| POST   | `/api/v1/users`   | Create a user        |
| GET    | `/api/v1/users`   | List users (paged)   |
| GET    | `/api/v1/users/:id` | Get a user by ID   |

Example:

```bash
curl -s localhost:8080/healthz

curl -s -X POST localhost:8080/api/v1/users \
  -H 'content-type: application/json' \
  -d '{"name":"Ada Lovelace","email":"ada@example.com"}'
```

## Common commands

```bash
make help     # list all targets
make test     # go test ./... -race -cover
make build    # build binary to bin/go-backend
make vet      # go vet
```
