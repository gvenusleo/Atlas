# Atlas

Atlas is a local general-purpose AI agent being rebuilt as a unified Dart and Flutter project.

[中文文档](README.zh-CN.md)

## Status

The repository currently contains:

- a Pub workspace that defines runtime, protocol, client, and adapter boundaries;
- an initial Flutter desktop and mobile application shell;
- an executable `atlas_runtime` agent engine and `atlas_storage` Drift adapter;
- an `atlas_provider` adapter for OpenAI-compatible Chat Completions and
  Responses streaming APIs;
- architecture and development contracts for the Dart implementation.

CLI, WebSocket transport, Nocterm TUI, ACP, and MCP integrations are not
implemented yet. There is currently no supported Atlas command-line release
from this branch.

## Development

Prerequisites are Git, [mise](https://mise.jdx.dev/), and [just](https://github.com/casey/just).

```sh
mise install
just deps
just ci
```

Run the existing Flutter shell on macOS:

```sh
just app-run macos
```

See [Development](docs/development.md) for workspace commands and [Architecture](docs/architecture.md) for runtime boundaries.

## Security Model

Atlas is designed to run tools with the permissions of its local process. It
does not provide a sandbox, permission prompts, or an approval gate. The current
runtime and storage implementation follows this boundary; provider, tool, and
client integrations remain in development.

## License

[MIT](LICENSE)
