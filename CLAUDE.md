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
Pushing to `main` deploys to AWS (`.github/workflows/deploy.yml`, arm64 runner → ECR → ECS; buildx with the GitHub Actions layer cache, then it waits for `/health`'s `commit` (`internal/buildinfo`, `GIT_SHA` build arg) to be the pushed one rather than for ECS to call the service stable, which takes minutes; the deployment circuit breaker rolls failed tasks back). The task definition CI deploys is the registered one with the new image: changes to `aws/infra/task-definition.json` need `up.sh <image>`. One `t4g.micro` ECS host, no load balancer: the task (`aws/infra/task-definition.json`, host networking) runs the app + Caddy (TLS), the DBs live on a separate EBS volume mounted at `/srv/tyche` (→ `/app/data`) with daily DLM snapshots, secrets come from SSM (`/tyche/*`). `aws/infra/up.sh` creates/completes it; see `aws/README.md` (restore, costs)

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
- One **shared DB** (`data/shared.sqlite`, own goose dir `sql/shared/schema`) holds data common to all users: `fx_rates` (latest rate per base → quote pair) and the shared `sms_templates` (+ a `meta` key/value table for their seed version), the vaults of the encrypted tenants and the `deleted_tenants` (see **Admin pages**). Opened in `main`; on a fresh one the first fx fetch blocks startup so conversions never start at 0

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
  - `vaultctx/` - Context key of the unlocked tenant DB's key (`middleware.VaultKeyCtxKey` aliases it), `Unlocked(ctx)` for templates
  - `sqlite/` - Registers the SQLCipher sqlite3 driver (import it instead of the driver), checking it was built with the `sqlcipher` tag
  - `shareddb/` - Opens + migrates the shared (non-tenant) DB (`SHARED_DB_PATH`, default `data/shared.sqlite`)
  - `sharedqueries/` - **Generated** sqlc code from `sql/shared/query/*.sql`
  - `smsparser/` - Template engine reading bank SMS (see **SMS templates**): `Compile` (DSL → anchored regexp), `NewRegistry(mine, shared).Parse(sender, body)`, `Seeds()` (the built-in HSBC/CIB/AAIB/Arab Bank/QNB templates, `SeedVersion`)
  - `smstemplates/` - In-memory store of the shared templates (`Seed` on startup, `Load`, CRUD reloading them) + `Row`/`Params` converting the sqlc rows of both DBs
  - `smsingest/` - Builds transactions from parsed SMS (account matching via `accounts.sms_refs` / card / iban suffix, fx conversion, `TxTime`: the SMS date/time in the user's time zone)
  - `brandicons/` - Indexes the simple-icons submodule (`third_party/simple-icons`) plus local extras in `web/brand-icons/` (brands simple-icons lacks or forbids, takes precedence) to detect counterparty icons from name/aliases; both served at `/brand-icons/`
  - `avatar/` - Downloads the Google profile picture on sign in (also after unlocking a pending one) into the tenant `user` row (`picture_data`/`picture_type`/`picture_hash`), served at `/avatar?v=<hash>` (immutable cache, ETag) as Google's url is rate limited; claims' `picture` is `User.AvatarURL()` (the Google url until downloaded)
  - `badge/` - The installed app's badge (see **PWA badge**): `Counts`/`Source`, `Notifier` (Web Push subscriptions in the shared DB, pushes on a change)
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

**Migrations rebuilding tables**: dropping a column with a foreign key needs a table rebuild, and `DROP TABLE` with the foreign keys on fires the ON DELETE actions of the tables pointing at it; such a migration is `-- +goose NO TRANSACTION` with `PRAGMA foreign_keys = OFF` around its own `BEGIN`/`COMMIT` (see `20261103000000_goal_accounts.sql`), relying on the tenant DB's single connection

**Days picked in forms**: date-only values parse to midnight UTC; whether a picked day is today/ahead (fulfilled or scheduled, no future purchase) compares it with `Handler.userToday` (today in `user.timezone`, same form), never `time.Now()`, which takes today for ahead until midnight UTC east of UTC

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

**Details drawers in the url**: a fetched drawer with `DrawerProps.URLParam` (counterparty `cp`, goal `goal`, account `acc`, recurrent tx `rtx`; listed in drawer.js' `DRAWER_URL_PARAMS`) sets `?<param>=<id>` when it opens and drops it when a fetched drawer closes (an edit form opened from the details keeps it meanwhile); the page reads it and opens the details on load (`components.DrawerOnLoad`), like the transactions page's `?tx=`. Transaction amounts aren't `sensitive` (hidden with the balances) in lists, totals are

**Forms in drawers**: every add/edit form opens in a drawer (`components.Drawer`: from the right on wide screens, a bottom sheet on phones; `drawer.js`, `drawerBase`) around a `form.DrawerForm` (header + close, scrolling fields, Cancel/submit footer). Create forms are `form.SheetForm`, a drawer kept in the page bound to `openCreateForm` (`hx-select` swaps only the form; on success the handler calls `drawerCreated(w)`: `drawer-saved` trigger + delayed swap of the fresh form). Edit forms are fetched into `#drawer` (`components.EntryMenuEditItem` / `EntryMenuDrawerItem`, or `hx-target="#drawer"`): `EditX` is the drawer, `EditXForm` the form alone, answered with its errors; on success `drawerSaved(w, "#<card id>", swap)` retargets the answer to the entry's card and closes the drawer (`internal/handler/drawer.go`). On phones swiping the sheet up fills the screen (`data-expanded`), down shrinks it back or closes it. Escape / clicking outside ask before discarding changes (`askConfirm`, main.js). The browser's Back closes the open drawer too (`drawerHistory`, drawer.js: a history entry per open drawer, its capturing `popstate` listener stopping htmx's; closing otherwise goes back past them), or goes back to the details a form was opened from (a form fetched into `#drawer` while details with a `DRAWER_URL_PARAMS` param were there, or `txDetails`' edit from the details). An open form is kept in the url so reloading reopens it: `?form=<path it was fetched from>` (a GET into `#drawer`, or the tx edit panel; reopened after its details when the url has them), `?new` for the page's create form; the reloaded entry is taken over (`drawerHistory.resume`) rather than pushing new ones. The transaction details drawer (`txDetails`) extends `drawerBase`

**List filters + pagination** (transactions page, the pattern for other lists): filters live in the query string (`models.TxFilter`, `ParseTxFilter` / `Values` / `Without`); the filters form (`hx-replace-url`) swaps only `#tx-list` (`HX-Target: tx-list`), out-of-form changes (clear, review) swap `#tx-body` (filters + list). One sqlc query with nullable filters (`sqlc.narg(x) IS NULL OR ...`, `LIMIT -1` = all) serves every call. `GetTransactionsPage` loads chunks of ~50 fulfilled txs rounded up to whole months (so month totals stay right), upcoming only in the first chunk; reimbursements are left out of the list and nested by a second query. Infinite scroll via a `revealed` sentinel (`GET /transactions/more?before=`); `#tx-until` holds how far back it's loaded, sent as `X-Tx-Until` on every request (transactions.js) so actions re-render the whole loaded list with the page's filters (`HX-Current-URL`). `/` focuses the search. When filtered, the list starts with a summary (`GetTransactionsSummary`: count + earned/spent of the whole filter, its WHERE kept in sync with `GetTransactions`) and the set filters as removable chips; categories, accounts, counterparties and month headers in the list narrow it on click (`filterBy` in transactions.js, keeping the other filters)

**Dashboard** (`/`, `web/views/dashboard.templ`): the shell renders only cheap data (balances, `GetFlow`, review counts, the exchange rates strip under the hero: shown currencies, rates in memory), the other widgets are `dashLazy` placeholders fetched on `load`/`revealed` from `/dashboard/*` (+ `/merchant-spending`), as the tenant DB has a single connection. `GetDailyFlow` (one query, per day + currency) feeds the hero, income vs expenses and spending pace charts; `GetCategorySpending` feeds budgets and the category chart; both bucket by `scheduled_to` like the transactions list. Charts are server-rendered HTML/SVG with CSS hover tooltips (no chart JS); cards use `components.GlassCard`

**Budget summary** (top of `/categories`, `web/views/categories_budget.templ`): `GET /categories/budget?month=YYYY-MM&hide=id,…` renders a user's month's (see **Months**) `models.BudgetSummary` (every expense category with a budget or spending + uncategorized, in the default currency, from `GetDailyCategorySpending`): stacked total bar, burn-up line vs on pace, a bullet bar per category (the ones without a budget nested); the burn-up + bars share one collapsed section, the unbudgeted ones another (`budgetCollapse`, open ones in localStorage), with the dashboard's "ahead of the month" (spent % > month % passed; pace/ahead only count budgeted categories). Clicking a category hides it from every chart; the hidden ids (`none` = uncategorized) live in localStorage (`budgetHidden` / click handler in main.js) and are sent as `hide`. Category create/edit/delete send `HX-Trigger: budget-changed` to re-render it

**Months** (`models.MonthStart`, `internal/models/period.go`): budgets, the dashboard, analytics (category / merchant spending, a tx's month stats, a counterparty's or account's "this month") and the transactions list's month sections count the user's months, starting on `user.start_of_period` (1–31, a shorter month starting on its last day). A month is named after the one it ends in (start 25: November = Oct 25 – Nov 24); `MonthStart.Of(t)` is the `Period` (`Month` label, `[From, To)`) a day is in, `For(month)` the one named after a month (the `?month=YYYY-MM` keys). The start day rides in the access claims (`AccessClaims.MonthStart`, `month_start`, 1 when missing; `monthStartFromCtx` in repositories), so no query reads it. SQL takes the `From`/`To` range, never `STRFTIME('%Y-%m')`. Card statement cycles, gam'yaas, recurrent txs and goal-plan deposits stay calendar-based

**Settings** (`/settings`, linked from the user menu, `internal/handler/settings.go`, `web/views/settings.templ`): one card per setting, each its own `PUT /settings/<card>` (`HX-Request` required) answering the card (with errors, or "Saved"): profile (first/last name, never overwritten by signing in), region (main currency + time zone, `Repository.SetCurrencyAndTimezone`: unlike the onboarding's `SetRegion` no account changes currency; a new currency reloads the page), month (start day), work (salary + currency + hours a week, read by `/decide`, the wishlist and goal plans) and the default account. Cards changing the claims reissue the cookies (`refreshClaims`). `/decide` and `/sms` only show their value with a link here (`views.SettingsURL`)

**Admin pages**: `/admin/*` routes use `handle(h.X, middleware.Admin)`: only users whose tenant ID (`models.TenantID`, same as the DB file name, e.g. `GL_123`) is in `ADMIN_TENANT_IDS` get through (cookie JWT only, `HX-Request` required on unsafe methods); everyone else gets a 404. Nav links via `adminNav`, shown when `claims.IsAdmin()`. `/admin/tenants` lists every tenant DB file in `data/` (plain or encrypted; encrypted = has a `tenant_vaults` row, with its session/device slot counts; a vault without its file shows as missing) and deletes accounts (`POST /admin/tenants/{id}/delete`, typing the tenant ID, not the admin's own; `Repository.DeleteTenant`: the DB, its `.bak`/journal files and the vault), e.g. for a user who forgot both password and recovery code. A deleted tenant is kept in the shared DB's `deleted_tenants` (+ in memory, `Repository.IsDeleted`): `dbFromCtx` returns `ErrTenantDeleted` instead of creating the DB again, `middleware.Deleted` (after `Auth`) drops the old cookies and claims, device tokens get 401, until a new OAuth sign in (`finishOAuth` → `Repository.Undelete`) starts over as a first sign in. Users see their tenant ID ("Account ID", `components.TenantID`, copyable) in the user menu and on `/unlock`, to tell the admin which account is theirs

**SMS templates** (`/sms/templates`, linked from `/sms`, every user): users create/edit/delete only their own templates (tenant DB); shared ones are read-only for them, "Override" opens the form prefilled with a copy saved as theirs. Admins can also write the shared ones (`mayWriteShared`: admin + `HX-Request` on unsafe methods, else 404). Unparsed SMS on `/sms` have a "Make template" link prefilling sender, bank (from the shared templates of the sender), body and sample. An SMS is read by the first matching template, the user's own (tenant `sms_templates`) then the shared ones, each by priority desc then age; only templates listing the sender (or no sender) are tried, every template if none lists it. Body = literal text (whitespace runs match any whitespace, whole SMS must match) + placeholders: `{{field}}` (`account_id`, `amount`, `currency`, `to_*`, `counterparty`, `date`, `time`, `ref`, `note`, `available_*`, see `smsparser/fields.go`), `{{counterparty:merchant}}` (auto-created counterparty is a merchant; plain `{{counterparty}}` auto-creates a non-merchant; existing ones are never updated), `{{date:DD/MM/YYYY}}` (else day-first formats are detected), `{{field:/re/}}`, `{{/re/}}` (skip), `{{*}}` (skip anything). Action `transaction` (with a tx kind and `fixed` constants, e.g. `to_account_id=Cash`) or `ignore`. Seeds are inserted by `seed_key` when `SeedVersion` grows, so admin edits survive; an unkeyed shared template with the same body as a new seed adopts its key instead of being duplicated. The page has a sandbox (dry-run with the user's accounts) and a live preview against the template's sample; `seed_cases_test.go` holds every message of the old hand-written parsers' tests. The SMS date (+time) becomes the tx date, in the user's time zone (`user.timezone`, set on `/settings`), falling back to `received_at` when missing or in the future

**Web sign in**: `GET /oauth/cb` renders `views.SigningIn` right away (no work, `code` dropped from the url), which posts state + code to `POST /oauth/finish` (code exchange, vault check, sign in, `HX-Redirect`). On the first sign in (`Repository.TenantExists`: no tenant DB file yet) finish keeps the google user (`pendingSignups`, `signup_pending` cookie) and swaps in `views.SettingUpAccount`, which posts `/oauth/setup` to create the DB (migrations + seed) and the user. The mobile app's callback stays synchronous (opening `tyche://` needs a server redirect). Sign in links show a busy state on click

**PWA**: installable via `web/public/manifest.webmanifest` + `web/public/sw.js`, both served from the root (`rootFile` in the router; a service worker's scope is its path), icons in `web/public/favicons/` (PNGs rendered from `favicon.svg` / `icon-maskable.svg` with inkscape), head tags in `layouts.head`. The service worker never caches pages or partials (user data): navigations are network only, falling back to `/offline` (`views.OfflinePage` in `layouts.Bare`, routed without auth/vault middlewares so it's never a redirect); static files are network first, the cache only used offline. Bump its `CACHE` when changing `PRECACHE` (`TestPWA` checks every precached url exists). "Install app" in the user menu shows while the browser offers `beforeinstallprompt`; `?new` on a `layouts.WithCreateForm` page opens its create form (the manifest's "Add transaction" shortcut)

**PWA badge** (`internal/badge`, `web/public/js/badge.js`, `internal/handler/push.go`): the installed app counts the SMS needing attention, the txs to review and the overdue ones (unfulfilled, `scheduled_to` before today UTC, `Repository.GetBadgeCounts`). Off until turned on: each device picks its sources on `/settings` ("App badge" card, only in the installed app, `display-mode: standalone`, and with VAPID keys set; also on the onboarding's badge step, `views.BadgeSwitches`), kept in localStorage and sent with its Web Push subscription; turning one on asks for the notifications, and without them (denied, dismissed, unsupported, or revoked later) every source is turned off again (`PUT`/`DELETE /push/subscription`, shared DB `push_subscriptions` with `last_sent`). `middleware.Badge` (inside `vault` / `DeviceAuth`, so encrypted DBs open with the request's key) recounts after every write of a tenant with a subscription, plus its first request of the day (so overdue ones only show once the DB is opened that day: the server never opens a DB by itself), and pushes the subscriptions whose counts or items changed (`last_sent` holds a signature of them). A push carries the counts + the first `badge.MaxItems` items (`badge.Item`: an SMS to check, a tx to review / overdue, deep linked; cut to fit the 4 KB payload); `sw.js` shows one notification per item (tag `sms:<id>` / `tx:<id>`, vibrating when new, untouched while still there, closed when gone) + a `badge-more` one titled with the counts (`Counts.Title()`, e.g. "3 SMS to check · 2 to review") for the rest, with the monochrome `favicons/badge-96.png` (from `badge.svg`) in the status bar, and `setAppBadge` where there is one. A notification closed by the user (dismissed or clicked) stays hidden while unhandled (the service worker's state, last payload + dismissed tags, in the `tyche-badge-state` cache) and is reported to `POST /push/dismissed` (no auth/vault middlewares: the endpoint authorizes it), setting the subscription's `repost_at` to its `repost_hours` (picked per device on the badge card, default 1, 0 = never) later; `Notifier.RunReposts` then pushes `{"repost":true}` and the service worker shows the dismissed ones again silently, without opening any tenant DB. Android Chrome has no Badging API: launchers count the notifications, hence one per item. While open, `GET /badge` sets the badge. Keys: `VAPID_PUBLIC_KEY`/`VAPID_PRIVATE_KEY` (`go run ./cmd/vapid`), `VAPID_SUBJECT`; unset = no pushes

**Onboarding** (`/welcome`, `internal/handler/onboarding.go`, `web/views/onboarding.templ`): signing in (`signInBrowser`, also a pending sign in once unlocked) sends to it until `user.onboarded_at` is set by `POST /welcome/finish` (finished or "Skip setup"). Steps (`models.OnboardingSteps`, `?step=`, each swapped into `#welcome` with `HX-Push-Url`): region (main currency + time zone; while they're still the new user's defaults the step reloads with the browser's `tz`/`langs` to preselect them, `models.SuggestCurrency`; `Repository.SetRegion` also moves the unused accounts of the old default currency, e.g. the seeded Cash, and the JWT cookies are reissued), accounts (bank/credit/cash with today's balance, written as is via `FixBalances` without adjustments, credit cards as owed), security (the encryption question; `POST /security/encryption` with `welcome` answers with the step showing the recovery code), badge (the installed app's badge switches: only in the installed app, `badge.js` sending `X-Installed-App: 1` with its htmx requests, and with VAPID keys, else skipped: `models.OnboardingStepsOf`, the progress and Back skip it too; a full page load, without the header, asks for its step again from the installed app, `WelcomeData.Recheck`), done (next steps links)

**Mobile/SMS ingest**: the Android app signs in via `GET /oauth/mobile` (PKCE) → `tyche://auth?code=` → `POST /api/mobile/token`, getting a non-expiring device JWT (`jwt.NewDeviceToken`). `/api/*` device routes use `handleDeviceJSON()` + `middleware.DeviceAuth`, which checks the `devices` table (revocable from `/sms`; signing in again from the same app install, `install_id`, reuses the row and rotates its `token_id`, invalidating older tokens) and sets `AccessClaims` so repos resolve the tenant as usual. Every received SMS is stored in `sms_messages` (dedup by hash, `template_id` = the template that read it); parsed ones create fulfilled transactions with `needs_review`, ignore templates mark them ignored, others can be retried from `/sms` or ignored, one by one or all of the selected status pill ("Retry all" / "Ignore all": `POST /sms/retry` / `POST /sms/ignore`, `status` empty = every pending one).

**Database encryption** (`/security`, linked from the user menu; also a step of the onboarding, "Not now" or encrypt): a tenant DB can be encrypted with SQLCipher. Its random key (DEK, raw `x'…'` key, `_key` DSN param) is only stored wrapped (AES-GCM, bound to tenant/kind/slot id) in the shared DB's `vault_slots`, once per slot: `password` and `recovery` (Argon2id, the recovery code is shown once), `session` (one per unlocked browser, HKDF of the random secret in the `vault_session` cookie `<slot id>.<secret>`, 12h sliding / 30 days max) `device` (one per phone, its secret in the device token's `vault_secret` claim, deleted when revoked) and `passkey` (fingerprint / face / screen lock unlock: a WebAuthn platform credential, user verification required, wrapped with HKDF of its PRF extension output for the per-credential `prf_salt`; credential JSON in `vault_passkeys`, verified by go-webauthn, RP = `OAUTH_CB_ORIGIN`); `tenant_vaults` lists the encrypted tenants (`vault.Store`, cached in memory). No key ever goes in a JWT. `middleware.Vault` (after `Auth`, every route) puts the key from the session cookie in the context (`middleware.VaultKeyCtxKey`, read by `dbFromCtx`, else `repository.ErrTenantLocked`), else sends to `/unlock` (password or recovery code, attempts rate limited); while unlocked, a lock button (user menu; `vaultctx.Unlocked(ctx)`, the key's ctx key living in `internal/vaultctx` for templates) ends the session (`POST /security/lock`) and unlocking comes back to the page — a valid session is needed on every request, not just an open connection, as the connection is shared by all the user's browsers. As the user row is in the tenant DB, signing in to a locked one (OAuth callback, incl. the mobile one) keeps a pending sign in (`vault_pending` cookie) until unlocked; `RefreshSession` reads with the session's key. Encrypting/decrypting rewrites the file via `sqlcipher_export` under the tenant lock (`TenantManager.Encrypt`/`Decrypt`): the vault is created before encrypting and deleted after decrypting, a plain file of a tenant with a vault gets encrypted on its next open. Devices signed in before encrypting must sign in again. Passkeys (`internal/handler/passkeys.go`, `web/public/js/passkeys.js`): added on `/security` (password required, `POST /security/passkeys/options` → `navigator.credentials.create` → `POST /security/passkeys`) or right after encrypting when the encrypt form's "also unlock with fingerprint" box is checked (also on the onboarding; the answer's `data-passkey-enroll` triggers it), shown only where the browser supports them; `/unlock` offers them (`POST /unlock/passkey/options` / `POST /unlock/passkey`, same `finishUnlock` as the password, so pending web and mobile sign ins work: the app's Custom Tab unlocks with them too) and auto-prompts on a device that used one (localStorage hint). Password and recovery code always work, e.g. on devices without the passkey. Unsafe `/security/*` requests need `HX-Request` (`middleware.HTMXOnly`)

**Account reset** (`/reset`, linked from the user menu): `models.ResetEntities` lists what can be wiped (transactions, recurrent, sms, sms_templates (own only), categories, counterparties, goals, accounts, wishlist, devices, settings), `Requires` adds dependents (accounts ⇒ transactions + recurrent + goals, goals ⇒ transactions). `Repository.Reset` deletes them in one transaction (`sql/query/reset.sql`), clearing references without an FK action first (`category_id`, `accounts.counterparty_id`); kept accounts get a 0 balance when transactions go. Selecting everything clears `seeded_at` and reseeds (`TenantManager.Reseed`). The handler requires typing `RESET` and drops the access cookie so the claims (default currency/account) refresh

**Account backup** (`/backup`, linked from the user menu): export downloads the tenant DB as a plain sqlite file (`Repository.ExportDB`: `sqlcipher_export` on the tenant connection, so decrypted when encrypted). Import (`POST /backup/import`, multipart with `IMPORT` typed before the file, ≤ 200 MB, `HX-Request` required) never uses the uploaded file as is (`internal/repository/backup.go`): it's checked (sqlite header, `integrity_check`, no triggers, goose version ≤ ours, `trusted_schema = OFF`), migrated, then its tables' rows are copied into a fresh DB with our schema; the user row keeps the current identity (`userIdentityCols` + oauth id/provider), `devices` are emptied (+ their vault slots), foreign keys must check out. `TenantManager.replace` then writes it (encrypted with the tenant's key) over the tenant DB under the lock, keeping the old one as `<tenant>.sqlite.bak`, which `POST /backup/restore` swaps back (so restoring twice redoes the import). Changing the key (encrypt/decrypt) deletes the `.bak`

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
  - Soft deleted (`deleted_at`; names unique among live ones, partial index `accounts_live_name`): `Repository.DeleteAccount` optionally moves the balance to another account first (delete drawer, `POST /accounts/{id}/delete`), drops its recurrent and scheduled txs and the default; deleted and goals' accounts are hidden from `GetAccounts`/balances/pickers, 404 on account routes (`Handler.LiveAccount`), shown "(deleted)" in the history, their txs locked
  - Cards have: card_number, card_limit, name_on_card, expiration_date; optional iban
  - **Account details** (drawer, `GET /accounts/{id}/details`, kept in the url as `?acc=`, opened by clicking the card (not its controls) or its menu, `web/views/account_details.templ`): balance, card statement / gam'yaa progress, a balance chart of the last `models.AccountHistoryMonths` months (`Repository.GetAccountHistory`: worked back from the current balance through `GetAccountDailyNet`, the raw money moved per `fulfilled_at` day, transfers and debts included, so not an analysis), in/out stats, upcoming and recent transactions (signed as they moved this account, `accountTxMove`; transfer pairs loaded as a pair is listed by its outgoing leg) and its details with copy buttons (card number, IBAN). Its Edit/Delete open their drawers in its place (Back returns to it)
  - Credit card statement cycle (`statement_day`, then `due_days` after it or on `due_day` of the next month, per card; `models.CardCycle`, `internal/models/card_statement.go`): `Repository.AttachCardStatements` sets `Account.Statement` (last closed statement: what it owed then, from the balance minus `GetCardActivitySince`, minus what was paid in since = what to pay by its due date to avoid interest; "today" in `user.timezone`), plus the projected next statement: owed now (`SoFar`, the last statement's unpaid `Due` included) + the unfulfilled txs scheduled by the next close (`Scheduled`, e.g. recurrent ones); shown on the card with the interest-free days of a purchase made today; `GetCardStatements` feeds the dashboard's alert pills: one per card left to pay (red when due in ≤ 3 days or overdue) and a neutral one per card with its projected statement
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
  - Created from the transaction form's Repeat toggle or the `/recurrent-tx` page's create form (`POST /recurrent-tx`, `CreateRecurrentTx`: a first transaction on "Starts on", fulfilled when not ahead, the next ones counted from it)
  - **Details** (drawer, `GET /recurrent-tx/{id}/details`, kept in the url as `?rtx=`, opened by clicking the card or its menu, `web/views/recurrent_tx_details.templ`): amount + recurrence, Edit / Schedule more / Delete (`from=details`: schedule more answers the details again with the card out of band, delete closes the drawer), money stats, upcoming and past transactions (`Repository.GetRecurrentTxTransactions`)
- **Categories** - Transaction categories with optional budgets, a color, and `kinds` (JSON list of tx kinds the category applies to)
- **Goals** - Savings goals with target amount and currency, each with its own hidden account (`goals.account_id`, kind `AccKindGoal`, `exclude_from_balance`, named `Goal: <name>`; created with the goal, backfilled by the seeder for goals without one). Money only moves through the goal's actions (`/goals/{id}/money?dir=deposit|withdraw`): a deposit is a transfer account → goal account (saved, not spent), a withdraw the way back; `Goal.Progress()` = its account's balance + `Purchased`. "Mark as bought" (`PUT /goals/{id}/fulfill`) records the purchase, an expense out of the goal account (the spending counts then); "Undo bought" deletes it. A tx's goal is the one whose account it's in or out of (no `goal_id` column), or whose loan it pays (`loan_goal_id`); every goal tx is locked (`Transaction.IsLocked`: no edit/duplicate, the purchase is only undone from the goal). Deleting a goal with money left asks where to move it (`POST /goals/{id}/delete`), then soft deletes its account so the history stays. `accounts.set_aside` (accounts page + Fix balances hint) is what an account moved to goals in progress, still in the bank but out of its balance. Goal currency can't change once money moved
  - **Goal plans** (`goals.plan` JSON, `models.GoalPlan`, `internal/models/goal_plan.go`, drawer `GET /goals/{id}/plan`, `internal/handler/goal_plans.go`, `web/views/goal_plans.templ`): a monthly amount (in the goal's currency, "Use all" fills what's free: salary − expense budgets − the other goals' plans' `Outlay`, `NewGoalPlanMoney`) and a mode: save the price, or save the down payment + day-one fees (`Goal.SaveTarget`) then a loan (`InstallmentPlan`, months 0 = the shortest term whose payments fit the monthly amount, `FitLoan`). Nothing is scheduled ahead: `Project` works it out on every render from the goal's progress (scheduled deposits counted as made): from everything put in since `StartsOn`, what's missing through this month is asked today (missed months caught up), then one deposit a month from the next (topping up one partly paid ahead): more than the monthly amount (one deposit or many) only leaves fewer deposits, it doesn't push them further out; with on track / behind / ahead against `StartSaved + Monthly × months since StartsOn` (reset when the plan is saved again). The card's "Save next" opens the deposit drawer prefilled (`?goal_amount=&account_id=`, dated today) from `PickGoalSource` (active accounts, excluded-from-balance ones included, no cards/debts/gam'yaas/goals), disabled when they don't hold it. An installments goal is ready once the down payment is saved; buying it can schedule the loan's payments as expenses from a picked account (`planPaymentTxs`, shared with the wishlist), linked to the goal by `transactions.loan_goal_id` (`Transaction.IsLoanPayment`: the goal's transactions, joined like the ones of its account, but editable and out of `GoalAmount`). Bought with a loan, a goal is "paying off" (`goalPaying`, in the in progress section) until every payment is made: `Goal.Loan` sums them, `PayoffPercentage` = (purchase + paid payments) / (purchase + all payments). "Undo bought" deletes the unpaid ones and unlinks the paid ones
  - **Goal details** (drawer like the transaction details, `GET /goals/{id}/details`, kept in the url as `?goal=`, opened by clicking the card (not its controls, like a counterparty's) or its menu, `web/views/goal_details.templ`): progress, actions (opening their drawers in its place with `from=details`: saving answers the details again, the card swapped out of band, `goalSaved`/`goalAnswer`, and Cancel goes back to them), recent pace, the plan (timeline started → today → saved/buy → loan paid off, next deposit, saving schedule `GoalProjection.Deposits`, the loan's payments from `BuyOn`) or the loan it was bought with, and the history (each linking to its details on `/transactions?tx=`)
- **Counterparties** - People/companies involved in transactions; `is_merchant` flag (SMS-created ones are merchants; `default_category_id` and `icon` are merchant-only, cleared otherwise); optional `default_category_id` (applied to new transactions — form, recurrent, SMS — with no category, if the category allows the tx kind); optional `aliases` (SMS merchant names) and `icon` (simple-icons slug, auto-detected for merchants only, `''` = no icon, NULL = not yet detected) and `color` (icon's brand hex, else random placeholder color for the first-letter avatar); NULLs backfilled on list; merging one into another (`POST /counterparties/{id}/merge`) moves its transactions/recurrent txs/accounts and adds its name + aliases to the target's aliases; archiving (card menu `PUT /counterparties/{id}/archive` or the edit form's toggle, `is_archived`) moves it to the page's collapsed "Archived" section (active ones are grouped in "Merchants" and "People & others", `views.CounterpartiesListID`; editing `is_merchant` or `is_archived` moves the card) and out of every picker (`counterpartyComboOptions` keeps the selected one), SMS still match it and quick-creating its name unarchives it; `note` (any counterparty, searchable on the page, notes joined on merge); `labels` (any counterparty, JSON list, e.g. Restaurant / Online store: edited with `form.TagsInputSuggest` offering the labels in use + `models.CounterpartyLabelSuggestions`, written with the casing already in use (`MatchLabelCasing`), unioned on merge, shown as chips filtering the page on click, and a label filter row matching any picked one; also edited in place in the details: clicking a label removes it, a one-line scrolling row of suggestions + a new label input adds one, each saved at once with the whole list, `PUT /counterparties/{id}/labels` with `labels` (+ `add`)); merchant-only (cleared by `clearNonMerchantFields` like `icon`): `rating` (1–5, NULL unrated, `models.MaxRating`; set in the form or from the card's stars, `PUT /counterparties/{id}/rating`, clicking the current star clears it) and `counterparty_locations` (branches: label, address, maps link, `position`; `ON DELETE CASCADE`, replaced as a whole on save, moved after the target's on merge; `CounterpartyLocation.MapsLink()` falls back to a Google Maps search of the address). Clicking the card (not its controls, like a tx row) / "Details" opens `GET /counterparties/{id}/details` (kept in the url as `?cp=`, see below) (`web/views/counterparty_details.templ`: stars rating with `from=details` (answered with the card out of band, `HX-Reswap: none`), note, spent this month / in all via `GetTransactionsSummary`, locations with open in maps + copy address and an inline add form (`POST /counterparties/{id}/locations`, appended), last fulfilled txs); the page sorts by name or rating and filters by stars client side (`counterpartiesFilterData`, remembered in localStorage `counterpartiesView`)
- **Decision helpers** (`/decide`) - "Can I afford it?" (`models.CheckAffordability`: a cash price or an installment plan against the balances, in the default currency, and the unfulfilled goals: share of what's left and delay at the last 3 months' pace, `GetGoalPaces`), cost in work hours (`models.WorkSettings` from `user.monthly_salary` / `salary_currency_id` / `work_hours_per_week`, set on `/settings`) and a wishlist (`wishlist_items`, cooling-off of 7 or 30 days, status waiting / bought / abandoned). Buying opens the transaction form prefilled with the first payment (the price, the plan's down payment + day-one fees, or its first monthly payment) and a hidden `wishlist_id`; creating it marks the item bought, links the transaction (`transaction_id`, `ON DELETE SET NULL`) and schedules the plan's other payments with their exact amounts
- **Installment plans** (`models.InstallmentPlan`, `internal/models/installments.go`) - strategies: known amount (a quote or interest-free, the extra counted as interest), flat rate (interest on the full amount for the whole term), reducing with equal payments (amortized), reducing with decreasing payments (equal principal). Rate per year or per month; extras: down payment and admin fee (amount or %, the fee paid on the day or added to the loan), monthly fee, insurance (% a year of the financed amount), balloon last payment (% of the price), grace months (interest added to the balance of reducing plans, to the term of flat ones). `Schedule(price)` gives the payments, totals and the effective APR (IRR, fees included). Stored on `wishlist_items.installment_plan` as JSON
- **Currencies** - Seeded currency list (string, symbol, kind, is_shown); exchange rates live in the shared DB (`fx_rates`, keyed by lowercase code, not per-tenant currency ID), fetched by `internal/fxrate` (fawazahmed0 currency-api) for the seeded codes only
  - Precious metals (XAU/XAG/XPT/XPD): rates and stored amounts are in troy ounces; `currencies.in_grams` (oz/g toggle on `/currencies`, `PUT /currencies/{id}/unit`) only changes how they're shown: format amounts with `Currency.Format` (never `formatters.FormatAmount` with a symbol), which converts and adds the unit. The transaction form has an oz/g toggle by a metal account's amounts (`amount_unit` / `to_amount_unit`, converted back to troy ounces on parse, `Account.AmountUnit`). Amounts are rounded to 6 digits in SQL (a gram's hundredths in troy ounces); fiat ones are rounded to cents in Go where computed (`Currency.Round`)

## Important Notes

- **Never commit `.env` files or `data/*.sqlite` databases**
- **Generated files** (`internal/queries/*.go`, `internal/sharedqueries/*.go`, `web/views/**/*_templ.go`, `web/public/styles/main.css`) are gitignored; never commit them. Regenerate with `make sqlgen`, `make templ`, `make css` before building
- **Authentication** is OAuth-only (no username/password)
- **HTMX convention**: Most handlers return HTML partials for HTMX, not full pages
- **Validation tags** use go-playground/validator syntax (`validate:"required,ulid"`)
- **ULID** is used for all IDs (github.com/oklog/ulid)
- **Foreign keys** are enforced with `PRAGMA foreign_keys = ON`
