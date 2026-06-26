# ADR 0001: Initial Backend Architecture

- **Status:** Accepted
- **Date:** 2026-06-26

## Context

We are bootstrapping a new Go backend service. We need a structure that is
simple to start with, easy to test, and able to grow without a rewrite.

## Decision

- **Language/runtime:** Go 1.26.
- **Architecture:** Standard layered architecture —
  `handler` (HTTP) → `service` (business logic) → `repository` (data access),
  with `model` holding domain types. Dependencies flow inward and are wired
  explicitly in `cmd/api/main.go` (manual dependency injection).
- **HTTP framework:** Gin — mature, widely used, good middleware ecosystem.
- **Database:** PostgreSQL accessed via `pgx/v5` connection pool. Strong
  consistency, relational data, and ACID transactions fit our needs.
- **Migrations:** SQL files under `migrations/`, applied with `golang-migrate`.
- **Config:** Environment variables (with optional `.env`) loaded via Viper.
- **Logging:** Structured logging with the stdlib `log/slog` (JSON in prod).

## Project layout

```
cmd/api/            program entrypoint + dependency wiring
internal/
  config/           configuration loading
  database/         pgx connection pool
  handler/          HTTP handlers (Gin)
  service/          business logic
  repository/       data access (Postgres)
  model/            domain entities & DTOs
  server/           router + middleware
pkg/logger/         reusable structured logger
migrations/         SQL migrations
```

## Consequences

- **Positive:** Clear separation of concerns; interfaces at the repository and
  service boundaries make unit testing straightforward (see the stubbed
  service test). Low operational complexity for a single deployable unit.
- **Trade-offs:** A layered monolith, not microservices. If parts of the system
  later need independent scaling or deployment, modules can be extracted then.
  Manual DI is explicit but must be maintained by hand as the graph grows.
