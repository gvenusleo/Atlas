# Built-in Tools

[中文](zh-CN/tools.md)

## Built-in Tools

| Tool | Description |
|---|---|
| `read` | Read a bounded range from a UTF-8 text file with optional 1-indexed `offset` and `limit`; returns at most 2,000 complete lines or 50 KiB of content (an oversized single line is returned in full) and reports `next_offset` when more content remains |
| `write` | Create a file or replace its complete contents, creating parent directories as needed |
| `edit` | Apply one or more exact replacements to an existing UTF-8 file; every `old_text` must occur exactly once in the original content, edits must not overlap, and validation failure leaves the file unchanged |
| `shell` | Run a command with the platform default shell, optionally passing standard input, a working-directory override (`cwd`), and `timeout_seconds`; returns combined output with the exit code |
| `plan` | Replace the complete task plan for multi-step work: one `step` description per entry with a `pending`, `in_progress`, or `completed` status, at most one step `in_progress` at a time; every call replaces the entire plan |

Relative paths for file tools are resolved from the session working directory.
`read` rejects directories and non-UTF-8 content. `edit` preserves a UTF-8 BOM
and the file's primary LF or CRLF line ending; it intentionally does not use
fuzzy whitespace or Unicode matching. `write` is a full-file operation and does
not append. `shell` returns at most 50 KiB of output (keeping both edges) and
reports `timed out` or `cancelled` when a command is interrupted; the default
timeout is 30 seconds and the maximum is 300. When `cwd` is omitted, the
command runs in the session working directory. `plan` accepts at most 50
steps of 500 characters each.

File and shell tools also return structured metadata consumed by ACP adapters:
`read.next_offset` is the next 1-indexed line, `write`/`edit` use `path`,
`newText`, and `oldText` for bounded diffs, and `shell` uses `exit_code`,
`truncated`, and `total_bytes`.

## Security Boundary

Tools run with the permissions of the local Atlas process. Atlas does not
provide a sandbox, permission prompts, or an approval gate. `shell` executes
commands with those permissions; the model sees every exit code and decides
how to proceed.
