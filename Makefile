export GOOSE_DRIVER=sqlite3
export GOOSE_DBSTRING=./data/db.sqlite
export GOOSE_MIGRATION_DIR=./sql/schema

run:
	@air -c config/.air.toml

build: sqlgen migrate templ css
	@go build -o ./tmp/main ./cmd/odinls

start: build
	@./tmp/main

seed:
	@go run cmd/seed/main.go

sqlgen:
	@sqlc generate

migrate:
	@goose up

migrate-down:
	@goose down

migrate-create:
	@if [[ -n "$(MIGRATION_NAME)" ]]; then goose create "$(MIGRATION_NAME)" sql; else echo "MIGRATION_NAME env variable is missing."; fi

templ:
	@templ generate

templ-dev:
	@templ generate --watch

css:
	@pnpm dlx tailwindcss -c ./config/.tailwind.config.js -i ./web/assets/styles/main.css -o ./web/public/styles/main.css --minify

css-dev:
	@pnpm dlx tailwindcss -c ./config/.tailwind.config.js -i ./web/assets/styles/main.css -o ./web/public/styles/main.css --minify --watch

dev: migrate sqlgen
	@make -j3 run templ-dev css-dev
