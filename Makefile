watch:
	@air -c config/.air.toml

build: templ css
	@echo "Building the binary..."
	@go build -o ./tmp/main ./cmd/odinls

templ:
	@echo "Generating the templates..."
	@templ generate

start: build
	@echo "Starting the service..."
	@./tmp/main

css-dev:
	@pnpm dlx tailwindcss -c ./config/.tailwind.config.js -i ./web/assets/styles/main.css -o ./web/public/styles/main.css --minify --watch

css:
	@pnpm dlx tailwindcss -c ./config/.tailwind.config.js -i ./web/assets/styles/main.css -o ./web/public/styles/main.css --minify
