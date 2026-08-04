# 内置工具与 Skill

[English](../tools.md)

## 内置工具

| 工具 | 说明 |
|---|---|
| `read` | 按路径读取 UTF-8 文本，支持可选的 1 起始 `offset` 和 `limit`；最多返回 2,000 个完整行或 50 KiB 文件内容，仍有内容时会给出下一次读取的 offset |
| `run_shell` | 使用 Windows PowerShell 或其他平台的 `/bin/sh` 发现路径、搜索文本、列出目录、执行命令和验证；必须提供简短的用户可见执行目的和命令，支持可选标准输入与可接受退出码，本地输出截断时保留完整临时日志 |
| `edit` | 对已有 UTF-8 文件执行一个或多个精确替换；每个 `old_text` 必须在原始内容中恰好出现一次，各项编辑不能重叠，校验失败时文件保持不变 |
| `write` | 新建文件或完整替换文件内容，按需创建父目录；覆盖已有文件时保留其权限 |
| `load_skill` | 按名称加载本地 skill 指令 |
| `web_search` | 使用 Tavily 搜索公网网页，需配置 `services.tavily.api_key` |
| `web_fetch` | 使用 Tavily 提取公网网页内容，需配置 `services.tavily.api_key` |
| `update_plan` | 管理多步骤工作的结构化任务计划，每次调用全量替换 |

`read`、`edit` 和 `write` 的相对路径均以 session 工作目录为基准。`read` 拒绝目录和非 UTF-8 内容。`edit` 保留 UTF-8 BOM 及文件主要使用的 LF 或 CRLF 换行，不进行空白或 Unicode 模糊匹配。`write` 是完整文件操作，不支持追加。目录发现、`rg`/`find` 搜索、二进制或字节级检查、生成器、格式化器及其他命令仍使用 `run_shell`。

## 计划追踪

`update_plan` 工具让模型用 `pending` / `in_progress` / `completed` 三种状态追踪多步骤工作。每次调用全量替换上一次的计划。每份计划最多包含 50 个步骤，每个步骤最多 500 个字符，且最多只能有一个 `in_progress` 步骤。系统提示词指示模型仅在跨多次工具调用的任务中使用，并避免频繁无意义的更新。

计划更新保存在 transcript 工具调用及其结构化 metadata 中。上下文压缩时，如果最后一次 `update_plan` 计划仍有未完成步骤，该完整计划会被注入摘要提示词，使已完成进度和待处理工作都能在压缩后保留。

各通道的展示方式：

- **ACP**：计划更新作为 `plan_update` session update 发送，每个步骤映射为 `PlanEntry`。Zed 等编辑器会渲染为结构化计划面板。
- **TUI**：每次更新都显示为结构化计划快照，并区分已完成、进行中和待处理步骤。

## 指令与 Skill

Atlas 加载两个附加指令文件（当前用户请求优先于指令文件，当前目录指令优先于全局指令，不递归查找父目录或子目录）：

- `~/.atlas/AGENTS.md`
- 当前工作目录下的 `AGENTS.md`

Atlas 也会扫描用户级和当前目录级 skill，只把 `name` 和 `description` 摘要放进系统提示词；需要完整指令时，模型通过 `load_skill` 读取对应 `SKILL.md`。通过 ACP 连接时，可调用 skill 会按当前 session 工作目录暴露为 `/<skill>` 命令；用户输入会原样传给模型，并在本轮直接注入对应的完整 `SKILL.md`。
