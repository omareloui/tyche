FROM golang:1.22-bullseye AS build

# To bash the bash commands work
SHELL ["/bin/bash", "-o", "pipefail", "-c"]

WORKDIR /usr/src/

RUN curl -fsSL https://deb.nodesource.com/setup_20.x | bash -  && \
  apt-get -y --no-install-recommends install nodejs && \
  npm i -g tailwindcss && \
  \
  go install github.com/a-h/templ/cmd/templ@latest && \
  go install github.com/pressly/goose/v3/cmd/goose@latest && \
  go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

RUN useradd -u 1001 nonroot

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY ./sqlc.yaml ./sqlc.yaml
COPY ./Makefile ./Makefile
COPY ./web ./web
COPY ./cmd ./cmd
COPY ./sql ./sql
COPY ./internal ./internal

RUN mkdir ./data && \
  make prebuild

RUN go build -v \
  -ldflags="-linkmode external -extldflags -static" \
  -tags netgo \
  -o /usr/src/bin/tyche \
  ./cmd/tyche

###
FROM scratch
WORKDIR /app

COPY --from=build /etc/passwd /etc/passwd

COPY --from=build /usr/src/data ./data
COPY --from=build /usr/src/web/public ./web/public

COPY --from=build /usr/src/bin/tyche ./bin/tyche

USER nonroot

CMD ["/app/bin/tyche"]
