# atlas_ws

The WebSocket transport for remote Atlas clients.

This package owns the versioned WebSocket wire contract, codecs, client and
server connection behavior, and explicit conversion to runtime types. It
accepts an injected request handler and must not compose or depend on storage,
provider, or tool implementations.
