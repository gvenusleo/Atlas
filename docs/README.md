# Documentation Guide

[中文](zh-CN/README.md)

Atlas documentation is organized by purpose:

- `README.md`: product status and currently supported entry points.
- `architecture.md`: system boundaries and dependency direction.
- `development.md`: workspace layout, commands, and engineering rules.
- `decisions/`: accepted high-impact technical decisions and their consequences.
- package `README.md` files: local responsibility and dependency constraints.

English documents define structure and terminology. Update the corresponding `zh-CN` document in the same change when one exists.

Use status language consistently:

- **Available** means the behavior exists in the current repository and is verified.
- **Planned** means the boundary or behavior is approved but not implemented.
- Removed behavior must be removed from active documentation; Git history remains the archive.

Do not publish command, configuration, or protocol examples until the referenced implementation exists.
