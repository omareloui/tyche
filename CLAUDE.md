# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Tyche is a personal finance management application built with Go and HTMX. It's a web application that uses SQLite databases with a multi-tenant architecture where each user gets their own database file.

## Tech Stack

- **Backend**: Go (1.24.3) with standard library `net/http`
- **Frontend**: HTMX with Alpine.js, Tailwind CSS 4.0
- **Templates**: Templ (type-safe Go templates compiled to Go code)
- **Database**: SQLite (one database per user)
- **ORM/Query Builder**: sqlc (generates type-safe Go code from SQL)
- **Migrations**: Goose
- **Auth**: OAuth2 with JWT tokens
- **Dev Tools**: Air (live reload)

## Development Commands

### Initial Setup
```bash
# Copy environment config
cp .env.example .env

# Create required directories
mkdir data logs

# Install node dependencies (for Tailwind)
npm install
```

### Development
```bash
# Start dev server with hot reload (runs 3 parallel processes)
make dev

# Or run components individually:
make run          # Start Go app with Air
make templ-dev    # Watch and compile .templ files
make css-dev      # Watch and compile Tailwind CSS
```

### Building
```bash
# Development build
make build

# Production release (static binary)
make release

# Run production binary
make start
```

### Code Generation
```bash
# Generate Go code from SQL queries
make sqlgen

# Generate Go code from templ templates
make templ

# Compile Tailwind CSS
make css
```

### Database
```bash
# Create new migration
make migrate-create

# Reset database (WARNING: deletes all data)
make regen-db
```

### Docker
```bash
# Run in dev mode
make docker-dev

# Run in production mode
make docker-prod
```

## Architecture

### Multi-Tenant Database Architecture

The app uses a **per-user database model** where each authenticated user gets their own SQLite database file:

- Databases stored in `./data/` directory with naming: `{oauth_provider}_{oauth_id}.sqlite`
- `TenantManager` (`internal/repository/repository.go:33`) handles connection pooling with:
  - Lazy database creation on first access
  - Automatic migrations via Goose when database is accessed
  - Auto-seeding (currencies, transaction kinds, account kinds)
  - Idle connection cleanup (30 min timeout)
  - Thread-safe connection management

### Request Flow

1. **Router** (`internal/router/router.go`) - Go 1.22+ stdlib router with method+path patterns
2. **Middleware Chain** (`internal/middleware/`) - Applied in this order:
   - `CorrelationID` - Adds correlation ID for request tracing
   - `RequestLogger` - Logs requests with zap
   - `Auth` - Decodes JWT from cookie, adds claims to context
   - `Protected` - Enforces authentication (applied via `handle()`)
3. **Handler** (`internal/handler/`) - Business logic layer
4. **Repository** (`internal/repository/`) - Gets tenant DB, executes queries via sqlc
5. **Templ Views** (`web/views/`) - Type-safe HTML templates, return HTMX-friendly partials

### Directory Structure

- `cmd/tyche/main.go` - Application entry point
- `internal/`
  - `handler/` - HTTP handlers (one file per resource)
  - `repository/` - Database layer (one file per resource + tenant manager)
  - `queries/` - **Generated** sqlc code from `sql/query/*.sql`
  - `models/` - Domain models with validation
  - `middleware/` - HTTP middleware (auth, logging, correlation ID)
  - `router/` - Route definitions and middleware composition
  - `valid/` - Validation wrapper (go-playground/validator)
  - `sanitizer/` - Input sanitization (leebenson/conform)
  - `jwt/` - JWT token handling
  - `config/` - Config parsing and Air/Tailwind configs
  - `logger/` - Zap logger setup
  - `errs/` - Error handling and validation errors
- `sql/`
  - `schema/` - Goose migrations
  - `query/` - SQL queries for sqlc (one file per resource)
- `web/`
  - `views/` - Templ templates (generate `*_templ.go` files)
  - `public/` - Static assets (CSS, JS, images)
  - `assets/styles/` - Source Tailwind CSS

### Key Patterns

**Model Validation**: Models implement `SanitizeAndValidate(sani, valid)` that:
1. Sanitizes input (trim whitespace, etc.)
2. Validates using struct tags (`validate:"required,ulid"`)
3. Returns typed validation errors

**Handler Pattern**: Handlers receive `*Handler` with repo, validator, sanitizer:
```go
func (h *Handler) CreateAccount(w http.ResponseWriter, r *http.Request) error {
    // Get user claims from context
    claims := jwt.GetClaimsFromCtx(r.Context())

    // Get tenant DB
    _, queries, err := h.repo.GetTenant(claims.Provider, claims.OAuthID)

    // Parse, validate, execute query, render templ view
}
```

**Database Queries**: Use sqlc-generated code in `internal/queries/`:
- Write SQL in `sql/query/*.sql` with sqlc annotations
- Run `make sqlgen` to generate type-safe Go code
- Generated code is in `internal/queries/`

**Templ Templates**: Type-safe Go HTML templates:
- Write in `web/views/*.templ`
- Run `make templ` to generate `*_templ.go` files
- Import and call like Go functions

## Financial Domain Model

Core entities and their relationships:

- **User** - OAuth authenticated user, has default currency and account
- **Accounts** - User's financial accounts (bank, cash, credit card, etc.)
  - Has account kind (enum), currency, optional counterparty
  - Credit cards have: card_number, card_limit, expiration_date
- **Transactions** - Financial transactions (income, expense, transfer)
  - Has kind (income/expense/transfer), amount, scheduled date, optional fulfilled date
  - Can be linked to category, counterparty, goal, recurrent transaction
  - Transfers have to_account and to_amount
- **Recurrent Transactions** - Templates for recurring transactions
  - Has frequency (every N units: day/week/month/year)
  - Tracks last_transaction_scheduled_to to avoid duplicates
- **Categories** - Transaction categories with optional budgets
- **Goals** - Savings goals with target amount and currency
- **Counterparties** - People/companies involved in transactions
- **Currencies** - Supported currencies with exchange rate data

## Important Notes

- **Never commit `.env` files or `data/*.sqlite` databases**
- **All generated files** (`internal/queries/*.go`, `web/views/*_templ.go`) should be committed as they're part of the build
- **Authentication** is OAuth-only (no username/password)
- **HTMX convention**: Most handlers return HTML partials for HTMX, not full pages
- **Validation tags** use go-playground/validator syntax (`validate:"required,ulid"`)
- **ULID** is used for all IDs (github.com/oklog/ulid)
- **Foreign keys** are enforced with `PRAGMA foreign_keys = ON`
