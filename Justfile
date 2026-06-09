set shell := ["bash", "-eu", "-o", "pipefail", "-c"]

binary := "atlas"
build_dir := "dist"
install_dir := env("HOME") + "/.local/bin"

default:
    @just --list

fmt:
    go fmt ./...

tidy:
    go mod tidy

test:
    go test ./...

check: fmt tidy test

build:
    mkdir -p {{ quote(build_dir) }}
    go build -o {{ quote(build_dir + "/" + binary) }} ./cmd/atlas

install: build
    mkdir -p {{ quote(install_dir) }}
    rm -f {{ quote(install_dir + "/" + binary) }}
    cp {{ quote(build_dir + "/" + binary) }} {{ quote(install_dir + "/" + binary) }}
    chmod +x {{ quote(install_dir + "/" + binary) }}

run prompt:
    go run ./cmd/atlas run {{ quote(prompt) }}

run-session session prompt:
    go run ./cmd/atlas run --session {{ quote(session) }} {{ quote(prompt) }}

acp:
    go run ./cmd/atlas acp

sessions:
    go run ./cmd/atlas sessions

session-show session:
    go run ./cmd/atlas session show {{ quote(session) }}

session-delete session:
    go run ./cmd/atlas session delete {{ quote(session) }}

clean:
    rm -rf {{ quote(build_dir) }}
