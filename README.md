# Atlas

Atlas is a Go coding agent prototype.

The core is intentionally small:

- session state persisted locally
- turn loop for model, tool, and context orchestration
- DeepSeek chat API provider
- built-in code tools with full local access
- terminal UI over a stream of agent events

Atlas does not implement a permission system. Tools run with the same filesystem
and process access as the user running the program.

