set shell := ["bash", "-eu", "-o", "pipefail", "-c"]
set windows-shell := ["powershell.exe", "-NoLogo", "-NoProfile", "-NonInteractive", "-Command"]

binary := if os_family() == "windows" { "atlas.exe" } else { "atlas" }
build_dir := "dist"
install_dir := if os_family() == "windows" { env("USERPROFILE", ".") + "/.local/bin" } else { env("HOME", ".") + "/.local/bin" }

default:
    @just --list

fmt:
    go fmt ./...

tidy:
    go mod tidy

fmt-check:
    @just --justfile {{ quote(justfile()) }} _fmt_check_{{ os_family() }}

gopls-check:
    @just --justfile {{ quote(justfile()) }} _gopls_check_{{ os_family() }}

_fmt_check_windows:
    $sources = @(git ls-files --cached --others --exclude-standard '*.go' | Where-Object { Test-Path -LiteralPath $_ -PathType Leaf }); $files = @(gofmt -l $sources); if ($files.Count -gt 0) { $files; exit 1 }

_fmt_check_unix:
    @sources=(); while IFS= read -r file; do if [ -f "$file" ]; then sources+=("$file"); fi; done < <(git ls-files --cached --others --exclude-standard '*.go'); files="$(gofmt -l "${sources[@]}")"; if [ -n "$files" ]; then printf '%s\n' "$files"; exit 1; fi

_gopls_check_windows:
    & go run golang.org/x/tools/gopls@v0.23.0 version *> $null; if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }; $sources = @(git ls-files --cached --others --exclude-standard '*.go' | Where-Object { Test-Path -LiteralPath $_ -PathType Leaf }); if ($sources.Count -eq 0) { exit 0 }; $output = @(& go run golang.org/x/tools/gopls@v0.23.0 check -severity=hint @sources 2>&1); if ($LASTEXITCODE -ne 0 -or $output.Count -gt 0) { $output; exit 1 }

_gopls_check_unix:
    @go run golang.org/x/tools/gopls@v0.23.0 version >/dev/null 2>&1; sources=(); while IFS= read -r file; do if [ -f "$file" ]; then sources+=("$file"); fi; done < <(git ls-files --cached --others --exclude-standard '*.go'); if [ "${#sources[@]}" -eq 0 ]; then exit 0; fi; if ! output="$(go run golang.org/x/tools/gopls@v0.23.0 check -severity=hint "${sources[@]}" 2>&1)"; then printf '%s\n' "$output"; exit 1; fi; if [ -n "$output" ]; then printf '%s\n' "$output"; exit 1; fi

test:
    go test ./...

# go-ci runs the complete Go verification pipeline without modifying files.
go-ci: fmt-check gopls-check
    go mod tidy -diff
    go build ./...
    go vet ./...
    go test -race ./...
    @just --justfile {{ quote(justfile()) }} _cross_build_{{ os_family() }}

[working-directory: 'app']
app-deps:
    fvm flutter pub get --enforce-lockfile

[working-directory: 'app']
app-fmt:
    fvm dart format lib test

[working-directory: 'app']
app-fmt-check:
    fvm dart format --output=none --set-exit-if-changed lib test

[working-directory: 'app']
app-analyze: app-deps
    fvm flutter analyze --no-pub

[working-directory: 'app']
app-test: app-deps
    fvm flutter test --no-pub

# app-ci runs the Flutter checks shared by local development and CI.
app-ci: app-deps app-fmt-check app-analyze app-test

# ci is the repository-wide verification entry point.
ci: go-ci app-ci

[working-directory: 'app']
app-run device='macos': app-deps
    fvm flutter run --no-pub -d {{ quote(device) }}

[working-directory: 'app']
app-build-linux: app-deps
    fvm flutter build linux --debug --no-pub

[working-directory: 'app']
app-build-macos: app-deps
    fvm flutter build macos --debug --no-pub

[working-directory: 'app']
app-build-windows: app-deps
    fvm flutter build windows --debug --no-pub

[working-directory: 'app']
app-build-android: app-deps
    fvm flutter build apk --debug --no-pub

[working-directory: 'app']
app-build-ios: app-deps
    fvm flutter build ios --debug --no-codesign --no-pub

_cross_build_windows:
    $env:CGO_ENABLED = "0"; $env:GOOS = "linux"; $env:GOARCH = "amd64"; go build ./...
    $env:CGO_ENABLED = "0"; $env:GOOS = "darwin"; $env:GOARCH = "amd64"; go build ./...
    $env:CGO_ENABLED = "0"; $env:GOOS = "windows"; $env:GOARCH = "amd64"; go build ./...

_cross_build_unix:
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build ./...
    CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build ./...
    CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...

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
