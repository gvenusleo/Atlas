# 内置工具

[English](../tools.md)

| 工具 | 描述 |
|---|---|
| `read` | 读取 UTF-8 文本文件的有界范围，支持可选的 1 起始 `offset` 与 `limit`；最多返回 2,000 个完整行或 50 KiB 内容（超长单行完整返回），剩余内容时返回 `next_offset` |
| `write` | 创建文件或替换完整内容，按需创建父目录 |
| `edit` | 对现有 UTF-8 文件应用一个或多个精确替换；每个 `old_text` 在原始内容中必须恰好出现一次，编辑不得重叠，校验失败时文件保持不变 |
| `shell` | 在 Unix 通过 `/bin/sh -c`、Windows 通过 `powershell -Command` 执行命令，支持一次性 stdin、`cwd` 与 `timeout_seconds`；实时显示合并输出，返回最终文本与退出状态 |
| `plan` | 替换多步任务的完整计划：每个条目一个 `step` 描述与 `pending`、`in_progress` 或 `completed` 状态，同时最多一个为 `in_progress` |

文件工具的相对路径基于会话工作目录解析。`read` 拒绝目录与非法 UTF-8 内容；`edit` 保留 UTF-8 BOM 与文件的主 LF/CRLF 行尾，不做模糊空白或 Unicode 匹配；`write` 是全量写入，不追加。`plan` 最多接受 50 步，每步最多 500 字符。

## Shell 执行

`shell` 每次调用通过 `dart:io` 启动一个使用管道的进程，显式传入上述可执行文件与参数，可选 `stdin` 写入一次后关闭输入。不提供 PTY 或持久交互会话；相对 `cwd` 基于会话工作目录解析，省略时即使用该目录。

stdout、stderr 与 stdin 并发处理，输出按到达顺序合并。捕获文本最多保留 50 KiB UTF-8 字节（含截断标记），保留首尾且不拆断字符；`total_bytes` 统计过滤前收到的原始字节。非法 UTF-8 用替代字符显示，ANSI/OSC 与其他终端控制序列被移除，CR/CRLF 转为换行，输出为纯文本，不重现终端光标效果。

首次输出立即发布，后续快照对慢消费者合并，只有最终结果会持久化并返回模型，但 Nocterm、ACP 客户端与 Flutter 都能在执行中显示输出。子程序可能自行缓冲输出，直到其刷新缓冲区。

默认超时 30 秒，`timeout_seconds` 接受 Dart `Duration` 可表示的正整数，覆盖输入、进程执行与输出排空；取消或超时会保留已捕获输出。根进程退出但继承管道仍未关闭时，排空等待有独立期限。清理在平台允许时终止进程树（Unix 用 TERM/KILL，Windows 用 `taskkill /T /F`），并回退到直接终止根进程；已脱离或重新托管的后代无法可靠找到，因此 Atlas 报告“无法确认清理”，不会声称已终止它们，这不是守护进程管理能力。

文件与 shell 工具返回供 ACP 适配器使用的结构化 metadata：`read.next_offset` 为下一行（1 起始），`write` 与 `edit` 使用 `path`、`newText`、`oldText` 描述受限差异，shell 结果包含 `truncated`、`total_bytes`、已知时的真实 `exit_code`，以及适用时的 `termination_reason` 或 `cleanup_failed`。正常非零退出在文本与 metadata 中交给模型判断，启动、I/O、超时、取消与清理失败则作为带安全摘要的工具错误返回。

## 安全边界

工具以本地 Atlas 进程的权限运行。Atlas 不提供沙箱、权限提示或 approval gate；`shell` 以这些权限执行命令，模型能看到每个退出码并自行决定后续行动。

## MCP 工具

配置的 MCP 工具与内置工具共用 registry 与工具循环，名称以 `mcp_` 开头，保留配对结果、取消与历史恢复。支持文本与结构化 JSON，输出及 metadata 合计限额 50 KiB，二进制内容标记为省略。详见 [MCP 工具](mcp.md)。
