# Atlas

A general-purpose agent built in Go. The core is a testable headless agent loop that can read and write files, execute shell commands, search the web, and maintain long-term memory. CLI, ACP (for editors like Zed), and WeChat channels all call into the same capabilities via `internal/runtime` without duplicating loop logic.

[中文文档](README.zh-CN.md)

## Features

- **Headless agent core**: model → tool calls → tool results, written back to transcript in order, looping until completion or step limit.
- **Multi-provider adapters**: connect to OpenAI, DeepSeek, and other compatible backends via `chat_completions` and `responses` API formats.
- **Built-in tools**: file read/write, text search, precise editing, shell execution, web search and extraction — ready out of the box.
- **Context compaction**: automatically summarizes earlier conversation when the context window threshold is reached, keeping recent messages to continue.
- **Long-term memory**: incrementally extracts instruction / fact / workflow memories from sessions, organized by global / project scope, retrieved via FTS5 and injected into subsequent sessions.
- **Multiple entry points**: CLI one-shot execution, ACP persistent connection (with editor-embedded terminal and file diff), WeChat QR-code remote control.
- **Local-first**: all sessions and memories stored in local SQLite. Data never leaves your machine (except model API calls and optional Tavily search).
- **Extensible instructions**: inject project-level and global instructions via `AGENTS.md` and skill files. Skills are loaded on demand.

## Quick Start

### Prerequisites

- Go 1.26+
- A model backend compatible with OpenAI Chat Completions or Responses API (e.g. DeepSeek, OpenAI)

### Installation

Install the latest release with one command:

```sh
curl -fsSL https://github.com/gvenusleo/atlas/releases/download/latest/install.sh | bash
```

Windows (PowerShell):

```powershell
irm https://github.com/gvenusleo/atlas/releases/download/latest/install.ps1 | iex
```

Or build from source:

```sh
git clone https://github.com/gvenusleo/atlas.git
cd atlas
go build -o dist/atlas ./cmd/atlas
```

Or with [just](https://github.com/casey/just):

```sh
just build        # build to dist/atlas
just install      # build and install to ~/.local/bin
```

### Initial Configuration

Create a config file at `~/.atlas/config.json` (minimal example):

```json
{
  "default_model": "deepseek-v4-flash",
  "providers": [
    {
      "name": "deepseek",
      "format": "chat_completions",
      "base_url": "https://api.deepseek.com",
      "api_key": "sk-...",
      "models": [
        {
          "value": "deepseek-v4-flash",
          "name": "DeepSeek V4 Flash",
          "context_window": 1000000,
          "max_tokens": 384000,
          "input_formats": ["text"]
        }
      ]
    }
  ]
}
```

Verify your configuration:

```sh
go run ./cmd/atlas doctor
```

### Run Your First Task

```sh
go run ./cmd/atlas run "Read README and summarize"
```

## Usage

The primary way to use Atlas is through [ACP](https://agentclientprotocol.com/) in editors like Zed. See [Channels](docs/channels.md) for ACP features and Zed configuration.

```sh
atlas run "<prompt>"                    # run a one-shot task
atlas run --model <value> "<prompt>"    # specify model
atlas run --session <id> "<prompt>"     # resume or create a specific session
atlas acp                               # start ACP service
atlas doctor                            # offline diagnostics
atlas sessions                          # list sessions
atlas session show <id>                 # view session content
atlas session compact <id>              # compact session context
atlas session delete <id>               # delete a session
atlas weixin login                      # WeChat QR login
atlas weixin serve                      # start WeChat channel
atlas weixin accounts                   # list logged-in accounts
atlas weixin logout <account-id>        # logout a WeChat account
atlas version                           # show version
```

When user input starts with `!`, Atlas skips the model and directly executes the rest as a shell command, e.g. `!pwd`. Use single quotes or escape `!` in zsh/bash:

```sh
go run ./cmd/atlas run '!pwd'
```

## Permissions and Security

Atlas runs with the local permissions of the current process. Built-in tools can read and write files, search text, and execute shell commands. **Atlas does not provide a sandbox, permission prompts, or an approval gate.** Only run in trusted workspaces.

All session and memory data is stored in local SQLite and never leaves your machine — except for model API calls and optional Tavily search.

## Documentation

- [Architecture](docs/architecture.md) — layered design, core loop, long-term memory
- [Configuration](docs/configuration.md) — full config reference and field descriptions
- [Channels](docs/channels.md) — ACP and WeChat integration details
- [Tools and Skills](docs/tools.md) — built-in tools, AGENTS.md, skill system
- [Development](docs/development.md) — project structure, build, test, design principles

## License

[MIT](LICENSE)
