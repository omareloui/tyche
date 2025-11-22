FROM golang:1.24-bullseye

# To bash the bash commands work
SHELL ["/bin/bash", "-o", "pipefail", "-c"]

WORKDIR /usr/src/

RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash -  && \
  apt-get -y --no-install-recommends install nodejs && \
  \
  go install github.com/air-verse/air@latest && \
  go install github.com/a-h/templ/cmd/templ@v0.3.943 && \
  go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest && \
  go install github.com/pressly/goose/v3/cmd/goose@latest

COPY package* .

RUN --mount=type=cache,target=/root/.npm npm ci

COPY go.mod go.sum ./

RUN go mod download

COPY cmd cmd
COPY internal internal
COPY web web
COPY sql sql
COPY sqlc.yaml sqlc.yaml
COPY Makefile Makefile

COPY data data

RUN make prebuild

ENTRYPOINT [ "make" ]
CMD ["run"]
