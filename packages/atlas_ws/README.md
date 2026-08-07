# atlas_ws

The WebSocket transport for remote Atlas clients.

This package owns WebSocket client and server connection behavior around
`atlas_protocol`. It accepts an injected request handler and must not compose or
depend on runtime, storage, provider, or tool implementations.
