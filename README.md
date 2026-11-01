# Tyche: Fortune Builder

Personal finance app built with Go and HTMX. Each user signs in with Google and
gets their own SQLite database, optionally encrypted (SQLCipher) with a password
only they know.

## Features

- **Accounts**: banks, cash, credit cards (limit, card number, IBAN) and
  gam'yaas (جمعية, rotating savings groups, with their whole schedule planned
  upfront)
- **Transactions**: expenses, income, transfers, debts, refunds and balance
  adjustments; recurrent transactions; reimbursements (friends paying back their
  share of a bill); transfer pairs (the two SMS of one transfer shown as one)
- **Categories** with budgets, **goals**, and **counterparties** with brand
  icons ([simple-icons](https://github.com/simple-icons/simple-icons)), aliases
  and default categories
- **Dashboard**: balances, cash flow, spending pace, budgets, category and
  merchant spending charts
- **Bank SMS ingest**: the [Android app](mobile/android/README.md) forwards bank
  SMS, read by templates (built-in ones for HSBC, CIB, AAIB, Arab Bank and QNB,
  plus your own, written in a small placeholder language with a sandbox) into
  transactions flagged for review
- **Multi-currency**: exchange rates fetched periodically and shared by all users
- **Decision helpers**: "can I afford it?" (cash or installment plans, against
  your balances and goals), cost in work hours, and a wishlist with a cooling-off
  period
- **Security**: database encryption with a password and a recovery code;
  account reset of selected data

## Tech Stack

Go (stdlib `net/http`), [templ](https://templ.guide), HTMX, Alpine.js,
Tailwind CSS 4, SQLite + SQLCipher, [sqlc](https://sqlc.dev),
[Goose](https://github.com/pressly/goose) migrations, Google OAuth2 + JWT.

## Prerequisites

- Go 1.25
- Node.js + npm (Tailwind)
- [`sqlc`](https://docs.sqlc.dev/en/latest/overview/install.html) and
  [`air`](https://github.com/air-verse/air) (live reload) on your `PATH`
- [`goose`](https://github.com/pressly/goose) only to create migrations
- OpenSSL's libcrypto (headers) and `pkg-config`, for SQLCipher. A static
  `make release` needs a static libcrypto (`libcrypto.a`)

`templ` is a Go tool of the module (`go tool templ`), don't install it globally:
a version mismatch with the runtime library breaks the build.

The sqlite driver is built with SQLCipher, which needs the `sqlcipher` build tag
and the libcrypto flags. The Makefile sets them, so build and test through it: a
plain `go build` / `go test` panics.

## Setup

Create the data and logs directories and your `.env`:

```bash
mkdir data logs
cp .env.example .env
```

Fetch the simple-icons submodule (brand icons) and the node dependencies:

```bash
git submodule update --init --depth 1
npm install
```

Create a Google OAuth client (web application) with the redirect URI
`<OAUTH_CB_ORIGIN>/oauth/cb` (e.g. `http://localhost:3333/oauth/cb`) and put its
ID and secret in `.env`.

## Configuration

| Variable                       | Default                      | Description                                                                                             |
| ------------------------------ | ---------------------------- | ------------------------------------------------------------------------------------------------------- |
| `PORT`                         | required                     | HTTP port                                                                                               |
| `APP_ENV`                      | required                     | `development` or `production`                                                                           |
| `LOG_LEVEL`                    | `0`                          | zap level (`-1` debug, `0` info, ...)                                                                   |
| `JWT_SECRET`                   | required                     | Secret signing the JWTs, use a long random string                                                       |
| `OAUTH_CB_ORIGIN`              | required                     | Public origin of the app, used for the OAuth redirect URI                                               |
| `GOOGLE_CLIENT_ID`             | required                     | Google OAuth client ID                                                                                  |
| `GOOGLE_CLIENT_SECRET`         | required                     | Google OAuth client secret                                                                              |
| `SHARED_DB_PATH`               | `./data/shared.sqlite`       | Database shared by all users (fx rates, shared SMS templates, encryption key slots)                     |
| `FX_RATE_FETCH_INTERVAL_HOURS` | `6`                          | How often exchange rates are fetched                                                                    |
| `ADMIN_TENANT_IDS`             | empty                        | Comma separated tenant IDs (the `data/*.sqlite` file names, e.g. `GL_1234567890`) allowed on `/admin/*` |
| `SIMPLE_ICONS_DIR`             | `./third_party/simple-icons` | simple-icons checkout                                                                                   |
| `EXTRA_BRAND_ICONS_DIR`        | `./web/brand-icons`          | Extra brand icons, taking precedence over simple-icons                                                  |

## Running

Development, with hot reload (Air, templ and Tailwind watchers):

```bash
make dev
```

Production build:

```bash
make build   # generates the code, builds ./tmp/main
make start
```

Static binary at `./bin/main`:

```bash
make release
```

Tests:

```bash
make test
```

### Docker

Both read `.env` and mount `./data` (and `./logs` in production):

```bash
make docker-dev    # rebuilds on changes (compose watch)
make docker-prod
```

## Development

Generated code is gitignored, regenerate it after changing its source (`make
build` and `make dev` do it for you):

```bash
make sqlgen   # sql/query/*.sql, sql/shared/query/*.sql -> internal/queries, internal/sharedqueries
make templ    # web/views/**/*.templ -> *_templ.go
make css      # web/assets/styles -> web/public/styles/main.css
```

Migrations live in `sql/schema` (user databases) and `sql/shared/schema` (shared
database) and run automatically when a database is opened. Create one with:

```bash
make migrate-create
```

User databases are stored as `data/{provider_prefix}_{oauth_id}.sqlite` (e.g.
`data/GL_1234567890.sqlite`), created, migrated and seeded on first sign in.
`make regen-db` **deletes every database** and regenerates the queries.

## Android App

`mobile/android/` holds a small Kotlin app that signs in to Tyche and forwards
bank SMS to `POST /api/sms`. Signed APKs are published as
[GitHub releases](https://github.com/omareloui/tyche/releases) (sideload, it's
not on the Play Store). See its [README](mobile/android/README.md) to build it.

## Architecture

See [CLAUDE.md](CLAUDE.md) for the architecture, request flow and domain model.
