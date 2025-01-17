FROM golang:1.23-bullseye AS build

WORKDIR /usr/src/

RUN go install github.com/a-h/templ/cmd/templ@v0.2.771 && \
  go install github.com/pressly/goose/v3/cmd/goose@latest && \
  go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

RUN useradd -u 1001 nonroot

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY vendor vendor
COPY Makefile Makefile
COPY web web
COPY cmd cmd
COPY sql sql
COPY internal internal
COPY sqlc.yaml sqlc.yaml
COPY data data

RUN rm -rf web/views/*_templ.go && \
  make templ sqlgen migrate && \
  go build \
  -ldflags="-linkmode external -extldflags -static" \
  -tags netgo \
  -o /usr/src/bin/tyche \
  ./cmd/tyche

###
FROM node:23-alpine AS css

RUN npm i -g tailwindcss

WORKDIR /app

COPY Makefile Makefile
COPY web/assets web/assets
COPY internal/config/.tailwind.config.js tailwind.config.js

RUN tailwindcss \
  -c tailwind.config.js \
  -i ./web/assets/styles/main.css \
  -o ./web/public/styles/main.css \
  --minify 

###
FROM scratch
WORKDIR /app

COPY --from=build /etc/passwd /etc/passwd

COPY --from=build /usr/src/data ./data
COPY --from=build /usr/src/bin/tyche ./bin/tyche

COPY --from=css /app/web/public ./web/public

USER nonroot

CMD ["/app/bin/tyche"]
