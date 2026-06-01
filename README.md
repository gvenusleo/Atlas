# Atlas

Atlas is a Go coding agent prototype.

The core is intentionally small:

- session state persisted locally
- turn loop for model, tool, and context orchestration
- DeepSeek chat API provider
- built-in code tools with full local access
- terminal UI over a stream of agent events

Atlas does not implement a permission system. Tools run with the same filesystem
and process access as the user running the program.

## Usage

Set a DeepSeek API key:

```sh
export DEEPSEEK_API_KEY=...
```

Run the TUI:

```sh
go run ./cmd/atlas
```

Run a single prompt without the TUI:

```sh
go run ./cmd/atlas -no-tui "list the files in this repository"
```

By default Atlas stores data in `~/.atlas/atlas.db`. Override it with
`ATLAS_DB` or `-db`.

The default DeepSeek model is `deepseek-v4-flash`; use `-model` to override it.

## Development

```sh
go fmt ./...
go test ./...
go vet ./...
go build ./cmd/atlas
```
