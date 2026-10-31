FROM golang:1.25-bookworm AS build

WORKDIR /usr/src/

RUN go install github.com/pressly/goose/v3/cmd/goose@latest && \
  go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

RUN useradd -u 1001 nonroot

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY cmd cmd
COPY sql sql
COPY internal internal
COPY sqlc.yaml sqlc.yaml
COPY web web

COPY Makefile Makefile

RUN make templ sqlgen && \
  go build \
  -ldflags="-linkmode external -extldflags -static" \
  -tags netgo \
  -o /usr/src/bin/tyche \
  ./cmd/tyche && \
  mkdir data logs

###
FROM node:23-bullseye AS css

WORKDIR /app

COPY package* .

RUN --mount=type=cache,target=/root/.npm npm ci

COPY web/ web/
COPY internal/config/.tailwind.config.js internal/config/.tailwind.config.js

RUN npm run css

###
FROM scratch AS final

WORKDIR /app

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /etc/passwd /etc/passwd

COPY --from=build /usr/src/bin/tyche bin/tyche
COPY --from=build /usr/src/sql sql

COPY --from=build --chown=nonroot:nonroot /usr/src/data data
COPY --from=build --chown=nonroot:nonroot /usr/src/logs logs

COPY --from=css /app/web/public/styles web/public/styles

COPY --from=build /usr/src/web/public/favicons web/public/favicons
COPY --from=build /usr/src/web/public/js web/public/js
COPY --from=build /usr/src/web/public/fonts web/public/fonts
COPY --from=build /usr/src/web/public/icons web/public/icons

COPY --from=build /usr/src/web/brand-icons web/brand-icons
COPY third_party/simple-icons/icons third_party/simple-icons/icons
COPY third_party/simple-icons/data/simple-icons.json third_party/simple-icons/data/simple-icons.json

USER nonroot

CMD ["/app/bin/tyche"]
