# Atlas

Atlas is a local general-purpose AI agent built as a unified Dart and Flutter project.

[中文文档](README.zh-CN.md)

## Status

The repository is a Pub workspace that defines the runtime, protocol, client, and adapter boundaries. These pieces work today:

- `atlas_runtime`, the agent engine, with Drift persistence in `atlas_storage`;
- `atlas_provider`, with OpenAI-compatible Chat Completions and Responses adapters, an Anthropic Messages adapter, and a composite provider that routes several providers into one runtime;
- `atlas_composition`, which wires config, providers, tools, storage, and the system prompt into one runtime for `atlas_cli` and `atlas_flutter`;
- a Nocterm chat interface in `atlas_tui`, the default `atlas` terminal entry point, with slash commands (`/model`, `/new`, `/resume`, `/compact`, `/quit`) and skill injection;
- an ACP server adapter in `atlas_acp`, served by `atlas acp` over NDJSON stdio, and the `atlas_ws` WebSocket transport served by `atlas server`, so the Flutter mobile app can drive a computer's Atlas over a private network;
- a Flutter desktop and mobile client that always connects through ACP, with sessions, agent turns, a file browser, and an embedded terminal;
- MCP client tools for configured stdio and Streamable HTTP servers, documented in [MCP tools](docs/mcp.md);
- an `atlas cache` subcommand that reports prompt-cache reuse from the session database.

Architecture and engineering rules live in the [documentation guide](docs/README.md).

## Installation

Install the latest release:

```sh
curl -fsSL https://github.com/gvenusleo/atlas/releases/latest/download/install.sh | bash
```

Windows (PowerShell):

```powershell
irm https://github.com/gvenusleo/atlas/releases/latest/download/install.ps1 | iex
```

Or build and install from source:

```sh
mise run cli-build    # build/bundle/bin/atlas
mise run cli-install  # install into ~/.local/bin
```

Start the remote endpoint with `atlas server`, which prints the pairing token on every start, then connect from the app's remote connection screen.

## Development

Prerequisites are Git and [mise](https://mise.jdx.dev/).

```sh
mise install
mise run deps
mise run ci
```

Run the Flutter client on macOS with `mise run app-run --device macos`. See [Development](docs/development.md) for workspace commands, [Architecture](docs/architecture.md) for runtime boundaries, [Configuration](docs/configuration.md) for the config file schema, and [Built-in Tools](docs/tools.md) for tool behavior.

## Security Model

Atlas runs tools with the permissions of its local process. It provides no sandbox, permission prompts, or approval gate, and MCP tools share that boundary. OAuth login and ACP session-provided MCP configuration remain Planned.

## License

[MIT](LICENSE)
