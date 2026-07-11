# 内置工具与 Skill

[English](../tools.md)

## 内置工具

| 工具 | 说明 |
|---|---|
| `read_file` | 读取文本文件 |
| `edit_file` | 精确替换一个唯一文本块 |
| `apply_patch` | 应用 Codex 风格文本 patch，可新增、更新、删除或移动一个或多个文件 |
| `write_file` | 写入文件内容 |
| `run_shell` | Windows 使用 PowerShell，其他平台使用 `/bin/sh`；支持可接受退出码，本地执行的有界输出截断时保留完整临时日志 |
| `load_skill` | 按名称加载本地 skill 指令 |
| `web_search` | 使用 Tavily 搜索公网网页，需配置 `services.tavily.api_key` |
| `web_fetch` | 使用 Tavily 提取公网网页内容，需配置 `services.tavily.api_key` |
| `todo_write` | 管理多步骤任务的结构化任务列表，每次调用全量替换 |
| `memory_search` | 按关键词搜索长期记忆条目；记忆存储可用时自动注册 |

## 任务追踪

`todo_write` 工具让模型用 `pending` / `in_progress` / `completed` 三种状态追踪多步骤任务。每次调用全量替换上一次的列表。系统提示词指示模型仅在跨多次工具调用的任务中使用，并避免频繁无意义的更新。

Todo 状态不持久化到数据库。上下文压缩时，会从 transcript 中提取最后一次 `todo_write` 调用，将未完成的条目注入摘要提示词，使任务状态在压缩后得以保留。

各通道的展示方式：

- **ACP**：todo 更新作为 `plan_update` session update 发送，每个条目映射为 `PlanEntry`。Zed 等编辑器会渲染为结构化计划面板。
- **微信**：`in_progress` 状态的条目作为进度消息发送给用户。
- **CLI**：todo 以工具调用形式出现在 transcript 中，标题为 `Todo: N items`。

## 指令与 Skill

Atlas 加载两个附加指令文件（当前用户请求优先于指令文件，当前目录指令优先于全局指令，不递归查找父目录或子目录）：

- `~/.atlas/AGENTS.md`
- 当前工作目录下的 `AGENTS.md`

Atlas 也会扫描用户级和当前目录级 skill，只把 `name` 和 `description` 摘要放进系统提示词；需要完整指令时，模型通过 `load_skill` 读取对应 `SKILL.md`。通过 ACP 连接时，可调用 skill 会按当前 session 工作目录暴露为 `/<skill>` 命令；用户输入会原样传给模型，并在本轮直接注入对应的完整 `SKILL.md`。
