export GOOSE_DRIVER=sqlite3
export GOOSE_DBSTRING=./data/db.sqlite
export GOOSE_MIGRATION_DIR=./sql/schema

run:
	@air -c internal/config/.air.toml

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

.ONESHELL:
migrate-create:
	@read -p "Migration Name: " _res
	@if [[ -n $$_res ]]; then
	@  goose create "$$_res" sql;
	@fi

templ:
	@templ generate --path web/views

templ-dev:
	@templ generate --path web/views --watch

css:
	@pnpm dlx tailwindcss -c internal/config/.tailwind.config.js -i ./web/assets/styles/main.css -o ./web/public/styles/main.css --minify

css-dev:
	@pnpm dlx tailwindcss -c internal/config/.tailwind.config.js -i ./web/assets/styles/main.css -o ./web/public/styles/main.css --minify --watch

dev: migrate sqlgen
	@make -j4 run templ-dev css-dev

.ONESHELL:
regen-db:
	@read -p "Are you sure you want to delete the current database (y/N) " _res
	@if [[ $$_res == "y" || $$_res == "Y" ]]; then
	@  rm -rf data/db.sqlite internal/db
	@  make --no-print-directory migrate &&
	@  make --no-print-directory sqlgen &&
	@  make --no-print-directory seed
	@fi

