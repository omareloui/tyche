run:
	@air -c internal/config/.air.toml

build: prebuild
	@go build -o ./tmp/main ./cmd/tyche

release: prebuild
	@go build -v -ldflags="-linkmode external -extldflags -static" -tags netgo -o ./bin/main ./cmd/tyche

prebuild: templ css sqlgen

start:
	@./tmp/main

sqlgen:
	@sqlc generate

.ONESHELL:
migrate-create:
	@read -p "Migration Name: " _res
	@if [[ -n $$_res ]]; then
	@  goose create "$$_res" sql;
	@fi

templ:
	@go tool templ generate --path web/views

templ-dev:
	@go tool templ generate --path web/views --watch

css:
	@npm run css

css-dev:
	@npm run css:dev

dev: sqlgen
	@make -j3 run templ-dev css-dev

.ONESHELL:
regen-db:
	@read -p "Are you sure you want to delete the current databases (y/N) " _res
	@if [[ $$_res == "y" || $$_res == "Y" ]]; then
	@  rm -rf data/*.sqlite internal/queries
	@  make --no-print-directory sqlgen
	@fi

docker-dev:
	@docker compose --profile dev up --watch

docker-prod:
	@docker compose --profile prod up
