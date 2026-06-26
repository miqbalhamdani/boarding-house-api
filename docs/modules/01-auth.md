# Module 01: Auth

## Goal
Provide secure authentication for owners and tenants.

## Scope
Included:
- owner registration
- owner login
- tenant login
- owner auth guard
- tenant auth guard
- owner data isolation helper

Excluded:
- complex staff roles
- password reset
- social login

## Main Tables
- owners
- owner_users
- tenants

## API Endpoints
- `POST /auth/owner/register`
- `POST /auth/owner/login`
- `POST /auth/tenant/login`
- `GET /tenant/me`

## Business Rules
- Owner token must include owner user ID and owner ID.
- Tenant token must include tenant ID and owner ID.
- Owner endpoints require owner token.
- Tenant portal endpoints require tenant token.
- Never trust owner ID from frontend request body.

## Acceptance Criteria
- Owner can register and login.
- Tenant can login if credentials exist.
- Owner endpoints reject tenant token.
- Tenant endpoints reject owner token.
- Authenticated owner ID is available to services.
