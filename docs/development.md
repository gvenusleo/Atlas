# Development

[中文](zh-CN/development.md)

## Project Structure

```text
cmd/atlas              CLI entry point
internal/acp           ACP protocol adapter and client capability bridge
internal/agent         headless agent loop (core loop)
internal/compact       context compaction planning and summarization
internal/config        config loading and validation
internal/memory        long-term memory entries, summaries, FTS retrieval, and task queue
internal/model         generic chat protocol and Provider interface
internal/prompt        system prompt construction
internal/provider      provider adapters by API format
  ├── chatcompletions  Chat Completions API
  └── responses        OpenAI Responses API
internal/runtime       orchestration layer, connecting agent, tools, session, and memory
internal/session       SQLite session persistence
internal/skill         skill scanning and loading
internal/tool          tool registry and built-in tools
internal/transcript    in-memory message sequence
internal/version       version info
internal/weixin        WeChat channel
```

## Build and Test

```sh
go build ./cmd/atlas           # build
go test ./...                  # run all tests
go test ./internal/agent/...   # run a single package's tests
just check                     # fmt + tidy + test (requires just)
```

## Design Principles

- **Small and verifiable**: the agent loop stays pure and side-effect-free. All side effects are concentrated in runtime, making it easy to test with fake providers.
- **No premature abstraction**: don't abstract before two real call sites exist. Don't keep duplicate interfaces for "maybe later."
- **Local permission boundary**: no permission abstraction. Tools have the full permissions of the host process.
- **Single core**: CLI, ACP, and WeChat share the same `runtime.Runtime` and agent loop. Channel layers only do protocol adaptation.
