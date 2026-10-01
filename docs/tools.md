# Built-in Tools

[中文](zh-CN/tools.md)

| Tool | Description |
|---|---|
| `read` | Read a bounded range from a UTF-8 text file with optional 1-indexed `offset` and `limit`; returns at most 2,000 complete lines or 50 KiB of content (an oversized single line is returned in full) and reports `next_offset` when more remains |
| `write` | Create a file or replace its complete contents, creating parent directories as needed |
| `edit` | Apply one or more exact replacements to an existing UTF-8 file; every `old_text` must occur exactly once, edits must not overlap, and a failed validation leaves the file unchanged |
| `shell` | Run a command through `/bin/sh -c` on Unix or `powershell -Command` on Windows, with optional one-shot stdin, `cwd`, and `timeout_seconds`; streams combined output and returns its final text and exit status |
| `plan` | Replace the complete task plan for multi-step work: one `step` description per entry with a `pending`, `in_progress`, or `completed` status, at most one of them `in_progress` |

Relative paths for file tools resolve from the session working directory. `read` rejects directories and non-UTF-8 content, `edit` preserves a UTF-8 BOM and the file's primary LF or CRLF line ending without fuzzy whitespace or Unicode matching, and `write` never appends. `plan` accepts at most 50 steps of 500 characters each.

## Shell Execution

`shell` starts one process per call through `dart:io` with pipes and the executable above, writes optional `stdin` once, then closes input. There is no PTY or persistent interactive session, and relative `cwd` values resolve against the session working directory, which is also the default.

stdout and stderr are consumed concurrently with stdin and combined in arrival order. The captured text keeps at most 50 KiB of UTF-8 bytes, including its truncation marker, preserving the beginning and end without splitting characters, while `total_bytes` counts raw bytes received before filtering. Malformed UTF-8 is replaced, ANSI/OSC and other terminal controls are removed, CR and CRLF become line breaks, and the result is plain text without cursor effects.

The first output is published immediately, later snapshots are coalesced for slow consumers, and only the final result is persisted and sent to the model, though Nocterm, ACP clients, and Flutter display output during execution. Child programs may buffer their own output until they flush it.

The default timeout is 30 seconds, and `timeout_seconds` accepts any positive integer representable as a Dart `Duration`. It covers input, process execution, and output drainage, and cancellation or timeout preserves captured output. A root process that exits while inherited pipes stay open gets a bounded drain period, and cleanup stops the process tree where the platform allows it (TERM/KILL on Unix, `taskkill /T /F` on Windows) with a direct-process fallback. Detached or already reparented descendants cannot be found reliably, so Atlas reports unconfirmed cleanup instead of claiming they were terminated; this is not daemon supervision.

File and shell tools return structured metadata consumed by ACP adapters: `read.next_offset` is the next 1-indexed line, `write` and `edit` use `path`, `newText`, and `oldText` for bounded diffs, and shell results include `truncated`, `total_bytes`, the actual `exit_code` when known, and `termination_reason` or `cleanup_failed` when applicable. A normal nonzero exit reaches the model in the text and metadata, while launch, I/O, timeout, cancellation, and cleanup failures are tool errors with safe summaries.

## Security Boundary

Tools run with the permissions of the local Atlas process. Atlas provides no sandbox, permission prompts, or approval gate; `shell` executes commands with those permissions, and the model sees every exit code and decides how to proceed.

## MCP tools

Configured MCP tools join the same registry and tool loop as the built-ins, with stable `mcp_`-prefixed names, paired results, cancellation, and history replay. Text and structured JSON are supported within a 50 KiB aggregate output and metadata budget, and binary content is marked as omitted. See [MCP tools](mcp.md).
