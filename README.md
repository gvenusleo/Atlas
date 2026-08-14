# Atlas

Atlas is a local general-purpose AI agent being rebuilt as a unified Dart and Flutter project.

[中文文档](README.zh-CN.md)

## Status

The repository currently contains:

- a Pub workspace that defines runtime, protocol, client, and adapter boundaries;
- an initial Flutter desktop and mobile application shell;
- an executable `atlas_runtime` agent engine and `atlas_storage` Drift adapter;
- an `atlas_provider` package with OpenAI-compatible Chat Completions and
  Responses adapters plus an Anthropic Messages adapter, and a composite
  provider for routing multiple providers to one runtime;
- an `atlas_prompt` package that builds the system prompt and loads AGENTS.md
  instruction files;
- an `atlas_cli` composition root that wires config, providers, tools, storage,
  and the system prompt into one runtime;
- a Nocterm chat interface in `atlas_tui` that runs as the default `atlas`
  terminal entry point, with slash commands (`/model`, `/new`, `/resume`,
  `/compact`, `/quit`) and skill injection;
- an ACP server adapter in `atlas_acp`, served by `atlas acp` over NDJSON
  stdio, covering session lifecycle, model and effort config, slash commands,
  turn streaming, agent plans, and Zed display enhancements (live shell
  terminals, file diffs, and follow-along locations);
- architecture and development contracts for the Dart implementation.

WebSocket transport and MCP integrations are not implemented yet. The CLI
builds as a single executable with `mise run build-cli`
(`build/bundle/bin/atlas`).

## Installation

Install the latest release with one command:

```sh
curl -fsSL https://github.com/gvenusleo/atlas/releases/download/latest/install.sh | bash
```

Windows (PowerShell):

```powershell
irm https://github.com/gvenusleo/atlas/releases/download/latest/install.ps1 | iex
```

Or build and install from source:

```sh
mise run build-cli    # build/bundle/bin/atlas
mise run install      # install into ~/.local/bin
```

## Development

Prerequisites are Git and [mise](https://mise.jdx.dev/).

```sh
mise install
mise run deps
mise run ci
```

Run the existing Flutter shell on macOS:

```sh
mise run app-run --device macos
```

See [Development](docs/development.md) for workspace commands and [Architecture](docs/architecture.md) for runtime boundaries.

## Security Model

Atlas is designed to run tools with the permissions of its local process. It
does not provide a sandbox, permission prompts, or an approval gate. The current
runtime and storage implementation follows this boundary; provider, tool, and
client integrations remain in development.

## License

[MIT](LICENSE)
