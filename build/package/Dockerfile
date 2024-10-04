FROM golang:1.22-bullseye AS build

WORKDIR /usr/src/

RUN useradd -u 1001 nonroot

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY ./cmd ./cmd
COPY ./config ./config
COPY ./internal ./internal
COPY ./web ./web


RUN go build -v \
  -ldflags="-linkmode external -extldflags -static" \
  -tags netgo \
  -o /usr/local/bin/tyche \
  ./cmd/tyche

###
FROM scratch
WORKDIR /app

COPY --from=build /etc/passwd /etc/passwd
COPY --from=build /usr/local/bin/tyche ./bin
COPY --from=build /usr/local/web/assets ./web/assets

USER nonroot

CMD ["./bin/tyche"]
