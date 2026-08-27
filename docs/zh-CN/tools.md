# 内置工具

[English](tools.md)

## 内置工具

| 工具 | 描述 |
|---|---|
| `read` | 读取 UTF-8 文本文件的有界范围，支持可选的 1 起始 `offset` 与 `limit`；最多返回 2,000 个完整行或 50 KiB 内容（超长单行会完整返回），剩余内容时返回 `next_offset` |
| `write` | 创建文件或替换其完整内容，按需创建父目录 |
| `edit` | 对现有 UTF-8 文件应用一个或多个精确替换；每个 `old_text` 在原始内容中必须恰好出现一次，编辑不得重叠，校验失败时文件保持不变 |
| `shell` | 使用平台默认 shell 执行命令，可选地传入标准输入、工作目录覆盖（`cwd`）与 `timeout_seconds`；返回合并输出与退出码 |
| `plan` | 替换多步任务的完整任务计划：每个条目一个 `step` 描述与 `pending`、`in_progress` 或 `completed` 状态，同时最多一个步骤为 `in_progress`；每次调用都会替换整个计划 |

文件工具的相对路径基于会话工作目录解析。`read` 拒绝目录与非法 UTF-8
内容。`edit` 保留 UTF-8 BOM 与文件的主 LF/CRLF 行尾；刻意不做模糊空白或
Unicode 匹配。`write` 是全量写入操作，不追加。`shell` 最多返回 50 KiB
输出（保留首尾两端），命令被中断时报告 `timed out` 或 `cancelled`；
默认超时 30 秒，最大 300 秒。省略 `cwd` 时，命令在会话工作目录中运行。
`plan` 最多接受 50 步，每步最多 500 字符。

文件与 shell 工具还会返回供 ACP 适配器使用的结构化 metadata：
`read.next_offset` 是下一行（从 1 开始），`write`/`edit` 对受限大小文件
使用 `path`、`newText`、`oldText` 描述差异，`shell` 使用 `exit_code`、
`truncated` 与 `total_bytes`。

## 安全边界

工具以本地 Atlas 进程的权限运行。Atlas 不提供沙箱、权限提示或
approval gate。`shell` 以这些权限执行命令；模型能看到每个退出码并自行
决定后续行动。
