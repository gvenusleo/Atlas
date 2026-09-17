# 内置工具

[English](tools.md)

## 内置工具

| 工具 | 描述 |
|---|---|
| `read` | 读取 UTF-8 文本文件的有界范围，支持可选的 1 起始 `offset` 与 `limit`；最多返回 2,000 个完整行或 50 KiB 内容（超长单行会完整返回），剩余内容时返回 `next_offset` |
| `write` | 创建文件或替换其完整内容，按需创建父目录 |
| `edit` | 对现有 UTF-8 文件应用一个或多个精确替换；每个 `old_text` 在原始内容中必须恰好出现一次，编辑不得重叠，校验失败时文件保持不变 |
| `shell` | 在 Unix 通过 `/bin/sh -c`、Windows 通过 `powershell -Command` 执行命令，支持一次性 stdin、`cwd` 与 `timeout_seconds`；实时显示合并输出，返回最终文本与退出状态 |
| `plan` | 替换多步任务的完整任务计划：每个条目一个 `step` 描述与 `pending`、`in_progress` 或 `completed` 状态，同时最多一个步骤为 `in_progress`；每次调用都会替换整个计划 |

文件工具的相对路径基于会话工作目录解析。`read` 拒绝目录与非法 UTF-8
内容。`edit` 保留 UTF-8 BOM 与文件的主 LF/CRLF 行尾；刻意不做模糊空白或
Unicode 匹配。`write` 是全量写入操作，不追加。`plan` 最多接受 50 步，
每步最多 500 字符。

## Shell 执行

`shell` 通过 `dart:io` 的 `Process.start()` 显式传入上述可执行文件与参数，
不由 Dart 选择 shell。每次调用启动一个使用管道的进程，可选的 `stdin` 写入
一次后关闭。不提供 PTY 或持久交互会话。相对 `cwd` 基于会话工作目录解析，
省略时使用会话目录；允许使用目录范围之外的绝对路径。

stdout、stderr 与 stdin 并发处理，输出按 Atlas 收到的先后顺序合并。
捕获文本最多保留 50 KiB UTF-8 字节（包含截断标记），保留首尾且不拆断字符。
`total_bytes` 统计过滤前收到的原始字节。非法 UTF-8 用替代字符显示；
ANSI/OSC 与其他终端控制序列会被移除，CR/CRLF 转为换行。输出为纯文本，
不会重现终端光标效果。

首次输出立即发布，后续替换快照约每 100 ms 合并一次，结束前再发送最后一次
快照。runtime 和 ACP 展示消费者处理较慢时，只保留最新的待显示快照。
仅最终结果会持久化并返回模型。Nocterm、ACP 客户端和 Flutter 均可在执行中
显示输出；Flutter 在 shell 首次输出时展开卡片，用户手动收起后不会再次自动
展开，包括将卡片滚出屏幕后再返回。子程序可能自行缓冲输出，只有其刷新缓冲区后 Atlas 才能显示。

默认超时为 30 秒。`timeout_seconds` 接受 Dart `Duration` 可表示的正整数，
支持超过 300 秒的长任务。超时覆盖输入、进程执行和输出排空。取消和超时会
保留已捕获输出。根进程已退出但继承的管道仍未关闭时，排空等待有独立期限；
进程清理也使用独立的有界等待。

清理会尝试终止进程树（Unix 使用 `pgrep -P` 与 TERM/KILL，Windows 使用
`taskkill /T /F`），并回退到直接终止根进程。此方式无法可靠找到已脱离或已被
重新托管的后代进程。清理工具缺失、输出未排空或无法确认清理时会明确报告，
不会声称所有后代都已终止；这不是守护进程管理能力。
清理工具的输出必须完整落在大小限制内；超限的 PID 列表会被拒绝，避免截断后变成其他进程标识。

文件与 shell 工具返回供 ACP 适配器使用的结构化 metadata：
`read.next_offset` 是下一行（从 1 开始），`write`/`edit` 对受限大小文件
使用 `path`、`newText`、`oldText` 描述差异。shell 返回 `truncated`、
`total_bytes`、已知时的真实 `exit_code`，以及适用时的 `termination_reason`
或 `cleanup_failed`。正常的非零退出会在文本和 metadata 中报告，交由模型
判断；启动、I/O、超时、取消与清理失败则作为工具错误返回安全摘要。

## 安全边界

工具以本地 Atlas 进程的权限运行。Atlas 不提供沙箱、权限提示或
approval gate。`shell` 以这些权限执行命令；模型能看到每个退出码并自行
决定后续行动。
