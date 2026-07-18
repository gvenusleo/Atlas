# Development

[中文](zh-CN/development.md)

## Project Structure

```text
cmd/atlas              CLI entry point
internal/acp           ACP protocol adapter and client capability bridge
internal/agent         headless agent loop (core loop)
internal/compact       context compaction planning and summarization
internal/config        config loading and validation
internal/model         generic chat protocol and Provider interface
internal/prompt        system prompt construction
internal/provider      provider adapters by API format
  ├── chatcompletions  Chat Completions API
  └── responses        OpenAI Responses API
internal/runtime       orchestration layer, connecting agent, tools, and session
internal/session       SQLite session persistence
internal/skill         skill scanning and loading
internal/tool          tool registry and built-in tools
internal/transcript    in-memory message sequence
internal/tui           interactive terminal UI
internal/version       version info
internal/ws            WebSocket channel
```

## Build and Test

```sh
go build ./cmd/atlas           # build
go test ./...                  # run all tests
go test ./internal/agent/...   # run a single package's tests
go test ./internal/tui         # run terminal UI tests
just ci                        # full non-modifying CI check (requires just)
```

## Run from Source

```sh
go run ./cmd/atlas                              # start the terminal UI
go run ./cmd/atlas run "Read README and summarize"  # run a one-shot task
go run ./cmd/atlas doctor                       # verify configuration
```

## Design Principles

- **Small and verifiable**: the agent loop stays headless and dependency-injected. Provider and tool effects enter through narrow interfaces, while runtime owns configuration, persistence, and compaction.
- **No premature abstraction**: don't abstract before two real call sites exist. Don't keep duplicate interfaces for "maybe later."
- **Local permission boundary**: no permission abstraction. Tools have the full permissions of the host process.
- **Single core**: TUI, CLI commands, ACP, and WebSocket share the same `runtime.Runtime` and agent loop. Entry layers only adapt their interface or protocol.
