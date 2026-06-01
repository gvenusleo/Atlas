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

The TUI keeps a compact transcript, fixed composer/status rows, summarized tool
output previews, and scrollback with `Up`/`Down`, `PgUp`/`PgDown`, or the mouse
wheel. Press `Enter` to send, `Esc` or `Ctrl+C` to quit.

Run a single prompt without the TUI:

```sh
go run ./cmd/atlas -no-tui "list the files in this repository"
```

By default Atlas stores data in `~/.atlas/atlas.db`. Override it with
`ATLAS_DB` or `-db`.

The default DeepSeek model is `deepseek-v4-flash`; use `-model` to override it.

## Skills

Atlas can load local skills from `SKILL.md` files. By default it scans:

- `<workdir>/.agents/skills`
- `~/.agents/skills`
- `~/.atlas/skills`

Add more roots with repeated `-skill-root` flags. A skill is triggered for a
turn when the prompt names it, for example `$think`, or links directly to a
skill file with `[$think](skill:///absolute/path/SKILL.md)`.

## Development

```sh
go fmt ./...
go test ./...
go vet ./...
go build ./cmd/atlas
```
