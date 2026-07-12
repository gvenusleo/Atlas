# Atlas

A general-purpose agent built in Go. The core is a testable headless agent loop that can read and write files, execute shell commands, and search the web. CLI, ACP (for editors like Zed), and WebSocket channels all call into the same capabilities via `internal/runtime` without duplicating loop logic.

[中文文档](README.zh-CN.md)

## Features

- **Headless agent core**: model → tool calls → tool results, written back to transcript in order, looping until completion or step limit.
- **Multi-provider adapters**: connect to OpenAI, DeepSeek, and other compatible backends via `chat_completions` and `responses` API formats.
- **Built-in tools**: file read/write, text search, precise editing, shell execution, web search and extraction — ready out of the box.
- **Context compaction**: automatically summarizes earlier conversation when the context window threshold is reached, keeping recent messages to continue.
- **Multiple entry points**: CLI one-shot execution, ACP persistent connection (with editor-embedded terminal and file diff), and a WebSocket service.
- **Local-first storage**: session records stay in local SQLite. Task content and results may be transmitted through configured model APIs, Tavily, or connected WebSocket clients.
- **Extensible instructions**: inject project-level and global instructions via `AGENTS.md` and skill files. Skills are loaded on demand.

## Quick Start

### Prerequisites

- Go 1.26+
- A model backend compatible with OpenAI Chat Completions or Responses API (e.g. DeepSeek, OpenAI)

### Installation

Install the latest release with one command:

```sh
curl -fsSL https://github.com/gvenusleo/atlas/releases/latest/download/install.sh | bash
```

Windows (PowerShell):

```powershell
irm https://github.com/gvenusleo/atlas/releases/latest/download/install.ps1 | iex
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
  "default_model": "deepseek/deepseek-v4-flash",
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

`default_model` is recommended in `provider/model` format (e.g. `"deepseek/deepseek-v4-flash"`). A bare model value (e.g. `"deepseek-v4-flash"`) is also accepted when unambiguous. The same applies to the `--model` flag.

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
atlas run --model <provider/model> "<prompt>"    # specify model (provider/model recommended)
atlas run --session <id> "<prompt>"     # resume or create a specific session
atlas acp                               # start ACP service
atlas serve                             # start WebSocket service (LAN)
atlas doctor                            # offline diagnostics
atlas sessions                          # list sessions
atlas session show <id>                 # view session content
atlas session compact <id>              # compact session context
atlas session delete <id>               # delete a session
atlas version                           # show version
```

When user input starts with `!`, Atlas skips the model and directly executes the rest as a shell command, e.g. `!pwd`. Use single quotes or escape `!` in zsh/bash:

```sh
go run ./cmd/atlas run '!pwd'
```

## Permissions and Security

Atlas runs with the local permissions of the current process. Built-in tools can read and write files, search text, and execute shell commands. **Atlas does not provide a sandbox, permission prompts, or an approval gate.** Only run in trusted workspaces.

Session records are stored in local SQLite. Atlas still sends request context to configured model providers and communicates through services or channels you enable, including Tavily and WebSocket clients.

## Documentation

- [Architecture](docs/architecture.md) — layered design, core loop, context compaction
- [Configuration](docs/configuration.md) — full config reference and field descriptions
- [Channels](docs/channels.md) — ACP and WebSocket integration details
- [Tools and Skills](docs/tools.md) — built-in tools, AGENTS.md, skill system
- [Development](docs/development.md) — project structure, build, test, design principles

## License

[MIT](LICENSE)
