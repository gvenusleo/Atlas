# Built-in Tools

[中文](zh-CN/tools.md)

## Built-in Tools

| Tool | Description |
|---|---|
| `read` | Read a bounded range from a UTF-8 text file with optional 1-indexed `offset` and `limit`; returns at most 2,000 complete lines or 50 KiB of content (an oversized single line is returned in full) and reports `next_offset` when more content remains |
| `write` | Create a file or replace its complete contents, creating parent directories as needed |
| `edit` | Apply one or more exact replacements to an existing UTF-8 file; every `old_text` must occur exactly once in the original content, edits must not overlap, and validation failure leaves the file unchanged |
| `shell` | Run a command through `/bin/sh -c` on Unix or `powershell -Command` on Windows, with optional one-shot stdin, `cwd`, and `timeout_seconds`; streams combined output and returns its final text and exit status |
| `plan` | Replace the complete task plan for multi-step work: one `step` description per entry with a `pending`, `in_progress`, or `completed` status, at most one step `in_progress` at a time; every call replaces the entire plan |

Relative paths for file tools are resolved from the session working directory.
`read` rejects directories and non-UTF-8 content. `edit` preserves a UTF-8 BOM
and the file's primary LF or CRLF line ending; it intentionally does not use
fuzzy whitespace or Unicode matching. `write` is a full-file operation and does
not append. `plan` accepts at most 50 steps of 500 characters each.

## Shell Execution

`shell` uses `dart:io` `Process.start()` with the executable and arguments above;
Dart does not select the shell. Each call starts one process with pipes, writes
optional `stdin` once, then closes input. There is no PTY or persistent interactive
session. Relative `cwd` values resolve against the session working directory;
omitting `cwd` uses that directory. Absolute paths outside it are allowed.

stdout and stderr are consumed concurrently with stdin and combined in the order
Atlas receives them. The captured text retains at most 50 KiB of UTF-8 bytes,
including its truncation marker, keeping the beginning and end without splitting
characters. `total_bytes` counts raw bytes received before filtering. Malformed
UTF-8 is replaced; ANSI/OSC and other terminal controls are removed, and CR/CRLF
become line breaks. Output is plain text, so terminal cursor effects are not
reproduced.

The first output is published immediately; subsequent replacement snapshots are
coalesced at roughly 100 ms intervals, followed by a final snapshot. Slow runtime
and ACP presentation consumers retain the latest pending snapshot. Only the
final result is persisted and sent back to the model. Nocterm, ACP clients, and
Flutter display output during execution; Flutter opens the shell card on its
first output and respects a subsequent manual collapse, including after scrolling
the card offscreen and back. Child programs may
buffer their own output, which Atlas cannot display before they flush it.

The default timeout is 30 seconds. `timeout_seconds` accepts positive integers
representable as a Dart `Duration`, including values above 300 seconds. It covers
input, process execution, and output drainage. Cancellation and timeouts preserve
captured output. A root process that exits while inherited pipes remain open gets
a bounded drain period; cleanup also has separate bounded waits.

Cleanup attempts to stop the process tree (`pgrep -P` plus TERM/KILL on Unix,
`taskkill /T /F` on Windows), with a direct-process fallback. Detached or already
reparented descendants cannot be reliably found by this approach. Missing cleanup
utilities, incomplete drainage, and unconfirmed cleanup are reported, rather than
claiming all descendants were terminated. This is not daemon supervision.
Cleanup utility output must fit its bound in full: oversized PID lists are
rejected rather than truncated into different process identifiers.

File and shell tools return structured metadata consumed by ACP adapters:
`read.next_offset` is the next 1-indexed line, `write`/`edit` use `path`,
`newText`, and `oldText` for bounded diffs. Shell results include `truncated`,
`total_bytes`, the actual `exit_code` when known, and `termination_reason` or
`cleanup_failed` when applicable. A normal nonzero exit is reported in the text
and metadata for the model to interpret; launch, I/O, timeout, cancellation, and
cleanup failures are tool errors with safe summaries.

## Security Boundary

Tools run with the permissions of the local Atlas process. Atlas does not
provide a sandbox, permission prompts, or an approval gate. `shell` executes
commands with those permissions; the model sees every exit code and decides
how to proceed.
