# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Tyche is a personal finance management application built with Go and HTMX. It's a web application that uses SQLite databases with a multi-tenant architecture where each user gets their own database file.

## Tech Stack

- **Backend**: Go (1.25) with standard library `net/http`
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

# Fetch simple-icons submodule (brand icons)
git submodule update --init --depth 1

# Install node dependencies (for Tailwind)
npm install
```

The sqlite3 driver is built with SQLCipher (go.mod replaces `mattn/go-sqlite3` with `jgiannuzzi/go-sqlite3`, sqlcipher branch), needing the `sqlcipher` build tag and OpenSSL's libcrypto (headers + `pkg-config`). The Makefile sets them (`GOFLAGS`, `CGO_CFLAGS`/`CGO_LDFLAGS` from pkg-config, rpath for NixOS), so build and test through it (`make test`, `make build`); a plain `go test` without the tag panics in `internal/sqlite`. A static `make release` needs a static libcrypto (`libcrypto.a`).

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

### Deployment
Pushing to `main` deploys to AWS (`.github/workflows/deploy.yml`, arm64 runner → ECR → ECS). One `t4g.micro` ECS host, no load balancer: the task (`aws/infra/task-definition.json`, host networking) runs the app + Caddy (TLS), the DBs live on a separate EBS volume mounted at `/srv/tyche` (→ `/app/data`) with daily DLM snapshots, secrets come from SSM (`/tyche/*`). `aws/infra/up.sh` creates/completes it; see `aws/README.md` (restore, costs)

## Architecture

### Multi-Tenant Database Architecture

The app uses a **per-user database model** where each authenticated user gets their own SQLite database file:

- Databases stored in `./data/` directory with naming: `{provider_prefix}_{oauth_id}.sqlite` (prefix from `OAuthProvider.Prefix()`, e.g. `GL` for Google)
- `TenantManager` (`internal/repository/repository.go:33`) handles connection pooling with:
  - Lazy database creation on first access
  - Automatic migrations via Goose when database is accessed
  - Auto-seeding via `internal/seeder/` (currencies, account kinds, default accounts, default categories)
  - Idle connection cleanup (30 min timeout)
  - Thread-safe connection management
- Optionally encrypted with the user's password (see **Database encryption**)
- One **shared DB** (`data/shared.sqlite`, own goose dir `sql/shared/schema`) holds data common to all users: `fx_rates` (latest rate per base → quote pair) and the shared `sms_templates` (+ a `meta` key/value table for their seed version). Opened in `main`; on a fresh one the first fx fetch blocks startup so conversions never start at 0

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
  - `resperr/` - HTTP response errors
  - `route_handler/` - Adapts handler methods (templ/JSON) to `http.Handler`
  - `seeder/` - Per-tenant DB seed data
  - `fxrate/` - Exchange rates held in memory for all users (`GetFxRate`, USD cross-rate fallback); `Job` fetches one base per seeded currency every `FX_RATE_FETCH_INTERVAL_HOURS` into the shared DB's `fx_rates`, then swaps the in-memory rates. Status + manual run at `/admin/fx-rates`
  - `vault/` - Keys of the encrypted tenant DBs (see **Database encryption**): `Store` (create/unlock/change password/recovery code, sessions, device slots)
  - `sqlite/` - Registers the SQLCipher sqlite3 driver (import it instead of the driver), checking it was built with the `sqlcipher` tag
  - `shareddb/` - Opens + migrates the shared (non-tenant) DB (`SHARED_DB_PATH`, default `data/shared.sqlite`)
  - `sharedqueries/` - **Generated** sqlc code from `sql/shared/query/*.sql`
  - `smsparser/` - Template engine reading bank SMS (see **SMS templates**): `Compile` (DSL → anchored regexp), `NewRegistry(mine, shared).Parse(sender, body)`, `Seeds()` (the built-in HSBC/CIB/AAIB/Arab Bank/QNB templates, `SeedVersion`)
  - `smstemplates/` - In-memory store of the shared templates (`Seed` on startup, `Load`, CRUD reloading them) + `Row`/`Params` converting the sqlc rows of both DBs
  - `smsingest/` - Builds transactions from parsed SMS (account matching via `accounts.sms_refs` / card / iban suffix, fx conversion, `TxTime`: the SMS date/time in the user's time zone)
  - `brandicons/` - Indexes the simple-icons submodule (`third_party/simple-icons`) plus local extras in `web/brand-icons/` (brands simple-icons lacks or forbids, takes precedence) to detect counterparty icons from name/aliases; both served at `/brand-icons/`
  - `avatar/` - Downloads the Google profile picture on sign in (also after unlocking a pending one) into the tenant `user` row (`picture_data`/`picture_type`/`picture_hash`), served at `/avatar?v=<hash>` (immutable cache, ETag) as Google's url is rate limited; claims' `picture` is `User.AvatarURL()` (the Google url until downloaded)
  - `idgen/` - ULID generation
  - `formatters/`, `numsutils/`, `slicesutils/`, `structutils/` - Helpers
- `mobile/android/` - Kotlin app forwarding bank SMS to `POST /api/sms` (see its README)
- `sql/`
  - `schema/` - Goose migrations
  - `query/` - SQL queries for sqlc (one file per resource)
  - `shared/schema/`, `shared/query/` - Migrations and queries of the shared DB (fx rates)
- `web/`
  - `views/` - Templ templates (generate `*_templ.go` files); subdirs `components/`, `form/`, `icons/`, `layouts/`
  - `public/` - Static assets (CSS, JS, images); vendored HTMX/Alpine/Chartist/Litepicker in `public/js/vendor/`
  - `assets/styles/` - Source Tailwind CSS

### Key Patterns

**Model Validation**: Models implement `SanitizeAndValidate(sani, valid)` that:
1. Sanitizes input (trim whitespace, etc.)
2. Validates using struct tags (`validate:"required,ulid"`)
3. Returns typed validation errors

**Handler Pattern**: Handlers are methods on `*Handler` (repo, validator, sanitizer) and return a templ component; `route_handler` renders it. Repository methods take `ctx` and resolve the tenant DB themselves from JWT claims in context (`dbFromCtx` in `internal/repository/utils.go`):
```go
func (h *Handler) Accounts(w http.ResponseWriter, r *http.Request) (templ.Component, error) {
    balances, err := h.repo.GetBalances(r.Context())
    if err != nil {
        return nil, err
    }
    // Parse, validate, call repo, return templ view
}
```
JSON endpoints use `HandlerJSONMethod` via `handlePubJSON()`.

**Errors**: return a `resperr.New(code, msg)` (anything else is a 500 whose message is only logged). `route_handler` and the middlewares respond through `middleware.WriteError` → `errpage.Write`: the plain message for HTMX requests (swapped inline, e.g. a form's 422), else the full `views.ErrorPage` (401 sign in, 403, 404, 429, 5xx with the correlation ID as reference; also for boosted navigations, which `layouts.Main` swaps in). JSON routes answer `{"error": msg}`

**List filters + pagination** (transactions page, the pattern for other lists): filters live in the query string (`models.TxFilter`, `ParseTxFilter` / `Values` / `Without`); the filters form (`hx-replace-url`) swaps only `#tx-list` (`HX-Target: tx-list`), out-of-form changes (clear, review) swap `#tx-body` (filters + list). One sqlc query with nullable filters (`sqlc.narg(x) IS NULL OR ...`, `LIMIT -1` = all) serves every call. `GetTransactionsPage` loads chunks of ~50 fulfilled txs rounded up to whole months (so month totals stay right), upcoming only in the first chunk; reimbursements are left out of the list and nested by a second query. Infinite scroll via a `revealed` sentinel (`GET /transactions/more?before=`); `#tx-until` holds how far back it's loaded, sent as `X-Tx-Until` on every request (transactions.js) so actions re-render the whole loaded list with the page's filters (`HX-Current-URL`). `/` focuses the search

**Dashboard** (`/`, `web/views/dashboard.templ`): the shell renders only cheap data (balances, `GetFlow`, review counts), the other widgets are `dashLazy` placeholders fetched on `load`/`revealed` from `/dashboard/*` (+ `/merchant-spending`), as the tenant DB has a single connection. `GetDailyFlow` (one query, per day + currency) feeds the hero, income vs expenses and spending pace charts; `GetCategorySpending` feeds budgets and the category chart; both bucket by `scheduled_to` like the transactions list. Charts are server-rendered HTML/SVG with CSS hover tooltips (no chart JS); cards use `components.GlassCard`

**Admin pages**: `/admin/*` routes use `handle(h.X, middleware.Admin)`: only users whose tenant ID (`models.TenantID`, same as the DB file name, e.g. `GL_123`) is in `ADMIN_TENANT_IDS` get through (cookie JWT only, `HX-Request` required on unsafe methods); everyone else gets a 404. Nav links via `adminNav`, shown when `claims.IsAdmin()`

**SMS templates** (`/sms/templates`, linked from `/sms`, every user): users create/edit/delete only their own templates (tenant DB); shared ones are read-only for them, "Override" opens the form prefilled with a copy saved as theirs. Admins can also write the shared ones (`mayWriteShared`: admin + `HX-Request` on unsafe methods, else 404). Unparsed SMS on `/sms` have a "Make template" link prefilling sender, bank (from the shared templates of the sender), body and sample. An SMS is read by the first matching template, the user's own (tenant `sms_templates`) then the shared ones, each by priority desc then age; only templates listing the sender (or no sender) are tried, every template if none lists it. Body = literal text (whitespace runs match any whitespace, whole SMS must match) + placeholders: `{{field}}` (`account_id`, `amount`, `currency`, `to_*`, `counterparty`, `date`, `time`, `ref`, `note`, `available_*`, see `smsparser/fields.go`), `{{counterparty:merchant}}` (auto-created counterparty is a merchant; plain `{{counterparty}}` auto-creates a non-merchant; existing ones are never updated), `{{date:DD/MM/YYYY}}` (else day-first formats are detected), `{{field:/re/}}`, `{{/re/}}` (skip), `{{*}}` (skip anything). Action `transaction` (with a tx kind and `fixed` constants, e.g. `to_account_id=Cash`) or `ignore`. Seeds are inserted by `seed_key` when `SeedVersion` grows, so admin edits survive; an unkeyed shared template with the same body as a new seed adopts its key instead of being duplicated. The page has a sandbox (dry-run with the user's accounts) and a live preview against the template's sample; `seed_cases_test.go` holds every message of the old hand-written parsers' tests. The SMS date (+time) becomes the tx date, in the user's time zone (`user.timezone`, set on `/sms`), falling back to `received_at` when missing or in the future

**Web sign in**: `GET /oauth/cb` renders `views.SigningIn` right away (no work, `code` dropped from the url), which posts state + code to `POST /oauth/finish` (code exchange, vault check, sign in, `HX-Redirect`). On the first sign in (`Repository.TenantExists`: no tenant DB file yet) finish keeps the google user (`pendingSignups`, `signup_pending` cookie) and swaps in `views.SettingUpAccount`, which posts `/oauth/setup` to create the DB (migrations + seed) and the user. The mobile app's callback stays synchronous (opening `tyche://` needs a server redirect). Sign in links show a busy state on click

**Mobile/SMS ingest**: the Android app signs in via `GET /oauth/mobile` (PKCE) → `tyche://auth?code=` → `POST /api/mobile/token`, getting a non-expiring device JWT (`jwt.NewDeviceToken`). `/api/*` device routes use `handleDeviceJSON()` + `middleware.DeviceAuth`, which checks the `devices` table (revocable from `/sms`; signing in again from the same app install, `install_id`, reuses the row and rotates its `token_id`, invalidating older tokens) and sets `AccessClaims` so repos resolve the tenant as usual. Every received SMS is stored in `sms_messages` (dedup by hash, `template_id` = the template that read it); parsed ones create fulfilled transactions with `needs_review`, ignore templates mark them ignored, others can be retried from `/sms` or ignored, one by one or all of the selected status pill ("Retry all" / "Ignore all": `POST /sms/retry` / `POST /sms/ignore`, `status` empty = every pending one).

**Database encryption** (`/security`, linked from the user menu; asked once on the first sign in, `user.encryption_prompted_at`, "Not now" or encrypt): a tenant DB can be encrypted with SQLCipher. Its random key (DEK, raw `x'…'` key, `_key` DSN param) is only stored wrapped (AES-GCM, bound to tenant/kind/slot id) in the shared DB's `vault_slots`, once per slot: `password` and `recovery` (Argon2id, the recovery code is shown once), `session` (one per unlocked browser, HKDF of the random secret in the `vault_session` cookie `<slot id>.<secret>`, 12h sliding / 30 days max) and `device` (one per phone, its secret in the device token's `vault_secret` claim, deleted when revoked); `tenant_vaults` lists the encrypted tenants (`vault.Store`, cached in memory). No key ever goes in a JWT. `middleware.Vault` (after `Auth`, every route) puts the key from the session cookie in the context (`middleware.VaultKeyCtxKey`, read by `dbFromCtx`, else `repository.ErrTenantLocked`), else sends to `/unlock` (password or recovery code, attempts rate limited) — a valid session is needed on every request, not just an open connection, as the connection is shared by all the user's browsers. As the user row is in the tenant DB, signing in to a locked one (OAuth callback, incl. the mobile one) keeps a pending sign in (`vault_pending` cookie) until unlocked; `RefreshSession` reads with the session's key. Encrypting/decrypting rewrites the file via `sqlcipher_export` under the tenant lock (`TenantManager.Encrypt`/`Decrypt`): the vault is created before encrypting and deleted after decrypting, a plain file of a tenant with a vault gets encrypted on its next open. Devices signed in before encrypting must sign in again. Unsafe `/security/*` requests need `HX-Request` (`middleware.HTMXOnly`)

**Account reset** (`/reset`, linked from the user menu): `models.ResetEntities` lists what can be wiped (transactions, recurrent, sms, sms_templates (own only), categories, counterparties, goals, accounts, wishlist, devices, settings), `Requires` adds dependents (accounts ⇒ transactions + recurrent). `Repository.Reset` deletes them in one transaction (`sql/query/reset.sql`), clearing references without an FK action first (`category_id`, `accounts.counterparty_id`); kept accounts get a 0 balance when transactions go. Selecting everything clears `seeded_at` and reseeds (`TenantManager.Reseed`). The handler requires typing `RESET` and drops the access cookie so the claims (default currency/account) refresh

**Database Queries**: Use sqlc-generated code in `internal/queries/`:
- Write SQL in `sql/query/*.sql` with sqlc annotations
- Run `make sqlgen` to generate type-safe Go code
- Generated code is in `internal/queries/`

**Templ Templates**: Type-safe Go HTML templates:
- Write in `web/views/*.templ`
- Run `make templ` to generate `*_templ.go` files
- Import and call like Go functions
- The `templ` CLI is a Go tool (`tool` directive in `go.mod`), run via `go tool templ`, so CLI and runtime library share one version. Don't use a globally installed `templ`: a version mismatch causes build errors like `undefined: templ.ResolveAttributeValue`. Upgrade with `go get -tool github.com/a-h/templ/cmd/templ@<ver>`

## Financial Domain Model

Core entities and their relationships:

- **User** - OAuth authenticated user, has default currency and account
- **Accounts** - User's financial accounts (bank, cash, credit card, etc.)
  - Has account kind (`account_kinds` table wrapping an enum), currency, optional counterparty, archived / exclude-from-balance flags
  - Cards have: card_number, card_limit, name_on_card, expiration_date; optional iban
- **Gam'yaas** (جمعية, rotating savings groups) - accounts of kind `AccKindGamyaa` with a 1:1 `gamyaas` row (full-share installment, members = months, shares e.g. 0.5/2, `turns` JSON of 1-based payout months, starts_on, pay-from account, recurrent tx). `CreateGamyaa` schedules everything upfront: a monthly recurrent Transfer pay-from → gam'yaa generating all N installments (`installment × shares`), plus one scheduled Transfer back per turn (`pot / len(turns)`). Balance = paid in (positive) or still owed after a payout (negative), 0 when done. Schedule is only set on create; later changes go through its transactions. Only transfers can use gam'yaa accounts (`getTxAccs`)
- **Transactions** - Financial transactions (expense, income, transfer, debt, refund, adjustment)
  - Has kind (`models.TxKind`), amount, scheduled date, optional fulfilled date
  - Refunds (merchant money back, e.g. refund SMS) add to the account like income but take expense categories (`TxKind.CategoryKind()`) and net against spending
  - Can be linked to category, counterparty, goal, recurrent transaction
  - Transfers have to_account and to_amount
  - Adjustments (`AdjustmentTxKind`) correct a balance: signed amount added to the account, no category/counterparty/goal, out of every analysis. Only created by **Fix balances** (`/accounts/fix-balances`, linked from `/accounts`: every active account's balance editable, applied as adjustments of the differences or written as is, `Repository.FixBalances`); not editable/duplicable (the form strips the sign), delete to undo
  - Reimbursements: an income can pay back (part of) an expense via `reimburses_id` (e.g. friends' shares of a bill you paid; linked from the income's "Reimburses…" menu, unlinked from the expense's breakdown). Balances are untouched; the expense shows its net amount (`amount − Σ reimbursements`) with the reimbursements nested under it. Only same-currency, non-goal expenses; Σ reimbursements ≤ expense amount
  - Transfer pairs: a transfer between own accounts often comes as two SMS (a debit read as an expense, a credit read as an income, minutes/hours apart). On ingest/retry `pairTransferWithTxn` (`internal/repository/transfer_pairs.go`) links the new one to an unpaired SMS tx of the opposite kind on another account (`models.MatchesTransferPair`: within 24h, in ≤ out, fee ≤ max(0.5%, 10); 5% across currencies; no goal/merchant/reimbursement), both rows' `transfer_pair_id` pointing at each other. The list shows one transfer row (`transferPair`: no SMS chip, only "Unlink transfer") with both sides nested under it ("Both sides", `transferLeg`: each with its own SMS chip and actions), paired by hand from an expense/income's "Pair transfer…" menu (`GET`/`PUT /transactions/{id}/transfer-pair`: any amount, opposite kind on another account within a week, `models.ValidateTransferPair`; the ones auto pairing would take flagged "Likely"), unlinked via `DELETE /transactions/{id}/transfer-pair`; past ones paired from `/sms` ("Match past transfers", `Repository.PairTransfers`)
  - **Analysis rule**: every analysis/chart (spending, income, categories, merchants) must use `Transaction.AnalysisAmount()` — net for expenses, 0 for reimbursements and transfer pair legs — never raw `amount`. In SQL: net expenses with `t.amount - (SELECT TOTAL(rt.amount) FROM transactions AS rt WHERE rt.reimburses_id = t.id)` and skip rows with `reimburses_id IS NOT NULL` or `transfer_pair_id IS NOT NULL` (see `GetMerchantSpending`)
- **Recurrent Transactions** - Templates for recurring transactions
  - Has frequency (every N units: day/week/month/year)
  - Tracks last_transaction_scheduled_to to avoid duplicates
- **Categories** - Transaction categories with optional budgets, a color, and `kinds` (JSON list of tx kinds the category applies to)
- **Goals** - Savings goals with target amount and currency
- **Counterparties** - People/companies involved in transactions; `is_merchant` flag (SMS-created ones are merchants; `default_category_id` and `icon` are merchant-only, cleared otherwise); optional `default_category_id` (applied to new transactions — form, recurrent, SMS — with no category, if the category allows the tx kind); optional `aliases` (SMS merchant names) and `icon` (simple-icons slug, auto-detected for merchants only, `''` = no icon, NULL = not yet detected) and `color` (icon's brand hex, else random placeholder color for the first-letter avatar); NULLs backfilled on list; merging one into another (`POST /counterparties/{id}/merge`) moves its transactions/recurrent txs/accounts and adds its name + aliases to the target's aliases
- **Decision helpers** (`/decide`) - "Can I afford it?" (`models.CheckAffordability`: a cash price or an installment plan against the balances, in the default currency, and the unfulfilled goals: share of what's left and delay at the last 3 months' pace, `GetGoalPaces`), cost in work hours (`models.WorkSettings` from `user.monthly_salary` / `salary_currency_id` / `work_hours_per_week`) and a wishlist (`wishlist_items`, cooling-off of 7 or 30 days, status waiting / bought / abandoned). Buying opens the transaction form prefilled with the first payment (the price, the plan's down payment + day-one fees, or its first monthly payment) and a hidden `wishlist_id`; creating it marks the item bought, links the transaction (`transaction_id`, `ON DELETE SET NULL`) and schedules the plan's other payments with their exact amounts
- **Installment plans** (`models.InstallmentPlan`, `internal/models/installments.go`) - strategies: known amount (a quote or interest-free, the extra counted as interest), flat rate (interest on the full amount for the whole term), reducing with equal payments (amortized), reducing with decreasing payments (equal principal). Rate per year or per month; extras: down payment and admin fee (amount or %, the fee paid on the day or added to the loan), monthly fee, insurance (% a year of the financed amount), balloon last payment (% of the price), grace months (interest added to the balance of reducing plans, to the term of flat ones). `Schedule(price)` gives the payments, totals and the effective APR (IRR, fees included). Stored on `wishlist_items.installment_plan` as JSON
- **Currencies** - Seeded currency list (string, symbol, kind, is_shown); exchange rates live in the shared DB (`fx_rates`, keyed by lowercase code, not per-tenant currency ID), fetched by `internal/fxrate` (fawazahmed0 currency-api) for the seeded codes only

## Important Notes

- **Never commit `.env` files or `data/*.sqlite` databases**
- **Generated files** (`internal/queries/*.go`, `internal/sharedqueries/*.go`, `web/views/**/*_templ.go`, `web/public/styles/main.css`) are gitignored; never commit them. Regenerate with `make sqlgen`, `make templ`, `make css` before building
- **Authentication** is OAuth-only (no username/password)
- **HTMX convention**: Most handlers return HTML partials for HTMX, not full pages
- **Validation tags** use go-playground/validator syntax (`validate:"required,ulid"`)
- **ULID** is used for all IDs (github.com/oklog/ulid)
- **Foreign keys** are enforced with `PRAGMA foreign_keys = ON`
