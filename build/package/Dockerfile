FROM golang:1.23-bullseye AS build

WORKDIR /usr/src/

RUN go install github.com/a-h/templ/cmd/templ@v0.2.771 && \
  go install github.com/pressly/goose/v3/cmd/goose@latest && \
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
  ./cmd/tyche

###
FROM node:23-bullseye AS css

WORKDIR /app

COPY package* .

RUN --mount=type=cache,target=/root/.npm npm ci

COPY web/assets web/assets
COPY web/views/*.templ web/views
COPY internal/config/.tailwind.config.js internal/config/.tailwind.config.js
COPY Makefile Makefile

RUN make css

###
FROM scratch AS final

WORKDIR /app

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /etc/passwd /etc/passwd

COPY --from=build /usr/src/bin/tyche ./bin/tyche

COPY --from=css /app/web/public/styles ./web/public/styles
COPY ./web/public/js ./web/public/js

COPY ./data data

USER nonroot

CMD ["/app/bin/tyche"]
