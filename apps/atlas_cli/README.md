# Atlas CLI

The command-line and Nocterm entry point for Atlas.

The planned `atlas` executable starts the TUI by default. The `atlas server`
subcommand will compose the runtime and expose it through `atlas_ws`. Other
non-interactive commands will share the same runtime instead of duplicating the
agent loop.

No executable is implemented yet.
