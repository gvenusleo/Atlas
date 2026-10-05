# Documentation Guide

[中文](zh-CN/README.md)

Atlas documentation is organized by purpose:

- `README.md`: product status and currently supported entry points.
- `architecture.md`: system boundaries and dependency direction.
- `configuration.md`: Pi-shaped JSON settings, models, authentication, catalog refresh, and validation rules.
- `development.md`: workspace layout, commands, and engineering rules.
- `tools.md`: the built-in tool catalog, limits, and security boundary.
- `mcp.md`: MCP server setup, tool behavior, and supported scope.
- `protocol-acp.md`: the ACP surface, Atlas extensions, and the WebSocket transport.
- `data-model.md`: persistence layout and failure rules.
- package `README.md` files: local responsibility and dependency constraints.

Keep each Markdown paragraph on a single line: do not hard-wrap prose, list items, or blockquote paragraphs.

English documents define structure and terminology. Update the corresponding `zh-CN` document in the same change when one exists.

Use status language consistently:

- **Available** means the behavior exists in the current repository and is verified.
- **Planned** means the boundary or behavior is approved but not implemented.
- Removed behavior must be removed from active documentation; Git history remains the archive.

Do not publish command, configuration, or protocol examples until the referenced implementation exists.
