set shell := ["bash", "-eu", "-o", "pipefail", "-c"]
set windows-shell := ["powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command"]

binary := if os_family() == "windows" { "atlas.exe" } else { "atlas" }
build_dir := "dist"
install_dir := if os_family() == "windows" { env_var_or_default("USERPROFILE", ".") + "/.local/bin" } else { env_var_or_default("HOME", ".") + "/.local/bin" }

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
    @just --justfile {{ quote(justfile()) }} _build_{{ os_family() }}

_build_windows:
    New-Item -ItemType Directory -Force -Path {{ quote(build_dir) }} | Out-Null
    go build -o {{ quote(build_dir + "/" + binary) }} ./cmd/atlas

_build_unix:
    mkdir -p {{ quote(build_dir) }}
    go build -o {{ quote(build_dir + "/" + binary) }} ./cmd/atlas

install: build
    @just --justfile {{ quote(justfile()) }} _install_{{ os_family() }}

_install_windows:
    New-Item -ItemType Directory -Force -Path {{ quote(install_dir) }} | Out-Null
    Copy-Item -Force {{ quote(build_dir + "/" + binary) }} {{ quote(install_dir + "/" + binary) }}

_install_unix:
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
    @just --justfile {{ quote(justfile()) }} _clean_{{ os_family() }}

_clean_windows:
    if (Test-Path {{ quote(build_dir) }}) { Remove-Item -Recurse -Force {{ quote(build_dir) }} }

_clean_unix:
    rm -rf {{ quote(build_dir) }}
