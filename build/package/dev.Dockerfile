FROM golang:1.22-bullseye

# To bash the bash commands work
SHELL ["/bin/bash", "-o", "pipefail", "-c"]

WORKDIR /usr/src/

RUN curl -fsSL https://deb.nodesource.com/setup_20.x | bash -  && \
    apt-get -y --no-install-recommends install nodejs && \
    npm i -g tailwindcss && \
    \
    go install github.com/air-verse/air@latest && \
    go install github.com/a-h/templ/cmd/templ@latest

COPY go.mod go.sum ./

RUN go mod download

COPY Makefile .
COPY cmd cmd
COPY config config
COPY internal internal
COPY web web

RUN templ generate && go mod tidy


ENTRYPOINT [ "air" ]
CMD ["-c", "config/.air.toml"]
