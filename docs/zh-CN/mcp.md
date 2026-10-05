# MCP 工具

[English](../mcp.md)

Atlas 是 MCP 客户端：配置的服务器工具与 `read`、`write`、`edit`、`shell`、`plan` 处在同一个 agent loop 中。stdio 与 Streamable HTTP 通过 `mcp_dart` 2.4.2 支持。Atlas 自身不是 MCP 服务器。

## 配置

在 `~/.atlas/mcp.json` 中使用常见的 `mcpServers` 对象配置，修改后重启 Atlas。缺失文件表示没有外部服务器。Atlas 接受 Pi 风格 MCP 配置的 stdio/HTTP 子集，不支持的 OAuth 和 exposure 选项会被拒绝。

```json
{
  "mcpServers": {
    "local_tools": {
      "command": "/absolute/path/to/mcp-server",
      "args": [],
      "cwd": "~/workspace",
      "env": { "SERVICE_TOKEN": "${SERVICE_TOKEN}" },
      "startupTimeout": 15,
      "timeout": 60
    },
    "remote_tools": {
      "type": "http",
      "url": "https://tools.example.com/mcp",
      "headers": { "Authorization": "Bearer ${MCP_TOKEN}" },
      "enabled": true
    }
  }
}
```

Atlas 不安装服务器，也不把 `command` 当作 shell 命令，字面参数放入 `args`。不会发起 OAuth 浏览器登录，远程服务器需支持无认证或配置的静态请求头。

| 字段 | 规则 |
|---|---|
| 对象键 | Server ID：1 到 128 个 ASCII 字母、数字、下划线或连字符。 |
| `type` | `stdio` 或 `http`；省略时，有 `command` 则使用 stdio，否则为 HTTP。 |
| `enabled` | 布尔值，默认 `true`；禁用项检查结构，但不展开凭据、不连接。 |
| `startupTimeout` | Atlas 扩展：正整数秒，默认 15；连接和工具发现共享截止时间。 |
| `timeout` | 正整数秒，默认 60；与 Pi 不同，进度不会延长 Atlas 的总调用截止时间。 |
| `command`、`args` | stdio 可执行程序与可选字符串参数列表，不做 shell 展开。 |
| `cwd` | 仅 stdio；绝对路径，展开开头的 `~/`；省略时使用 Atlas 启动目录。 |
| `env` | stdio 字符串映射，覆盖 bootstrap 环境快照。 |
| `url` | HTTP(S) 地址，不含 userinfo 或 fragment；保留 query。 |
| `headers` | HTTP 字符串映射，不允许覆盖 MCP 协议、Host、Content-Type、Content-Length、Accept、Last-Event-ID、Connection、Transfer-Encoding 请求头。 |

`env`、`headers` 的值支持 `${VAR}`；启用项引用未定义变量时配置加载失败并报告字段路径。请求头名必须合法且大小写无关唯一，值不能包含换行，未知字段或与传输类型不兼容的字段会被拒绝。

## 资源归属与启动

CLI TUI、`atlas acp`、`atlas server` 与 Flutter 桌面本地模式各自组装 MCP 连接，每个主机 runtime 的会话共享每个服务器的一条连接。移动端与远程客户端使用所连接主机上配置的工具，不启动 MCP 进程、不转发本机凭据。CLI 继承启动环境，Flutter 桌面使用已有的 bootstrap 快照（含 macOS 登录 shell 导出的变量）。服务器 `cwd` 在连接期间固定。

在 runtime 可用前按配置顺序连接服务器。任一启用项失败都会报告服务器名与脱敏的错误分类并中止启动，同时关闭已打开的连接；禁用该项后重启即可继续使用。帮助、版本、缓存统计与 token 轮换不建立连接。进程退出先排空活动 turn，再关闭 MCP、provider 客户端与 storage；启动期间收到 SIGINT/SIGTERM 会取消发现并清理子进程。SDK 清理针对 Atlas 持有的进程，主动脱离的后代进程无法保证被终止。

## 工具行为

工具名由可读的 `mcp_` 前缀、处理过的服务器与工具名，以及稳定的 SHA-256 后缀组成，最长 64 个 provider 兼容字符，调用时仍使用原始名称。服务器按配置排序，每个服务器内部按原始工具名排序。

工具目录在启动时固定：目录变化通知对每条连接只记录一次 `mcp.catalog_changed_restart_required`，重启后刷新定义。必须通过 Tasks 执行的工具会被省略并记录安全诊断，服务器返回的 instructions 不会写入系统提示词。

目录元数据会进入每次 provider 请求，因此 description 超过 2 KiB UTF-8 会被截断并加标记，单个工具的 input schema 超过 64 KiB 或单个服务器的目录元数据超过 1 MiB 都会导致启动失败；后两者会报告服务器名，并阻止其定义进入 provider 请求。

结果保留文本块顺序、内嵌文本资源与资源链接（不自动抓取），结构化 JSON 同样进入模型可见输出。图片、音频与二进制资源替换为明确的省略标记；只有不支持的二进制内容时返回适配器错误，空成功结果仍然有效。输出与身份 metadata 合计不超过 50 KiB UTF-8，两者都会标记截断。进度是有界临时快照，不持久化也不传给模型；每次调用仍产生一条最终持久化结果，现有通用工具视图与 ACP 历史恢复无需改动即可展示。

取消与超时作用于单个请求。stdio 连接保持可用；Streamable HTTP 的 SDK 无法中断已发出的请求，因此 Atlas 会把该连接标记为不可用，而不是每次尝试再泄漏一个请求：失败调用报告超时或取消，之后对该服务器的调用返回 `connection_abandoned`，连接在后台关闭。重启 Atlas 可重新连接该服务器。超时、认证失败、无效结果与传输失败作为安全错误返回模型；被取消或断连的操作可能已产生远端副作用，Atlas 会说明结果不确定，不自动重试。

## 兼容性与当前限制

- SDK 使用 stable 协商配置，支持 MCP 2026-07-28 与旧初始化流程，测试覆盖 stdio、HTTP 上的 2025-11-25 与 2026-07-28，以及 HTTP JSON/SSE 响应；这不代表已实现所有 MCP 可选能力。
- 子进程自动重启与 HTTP 事件流重试被禁用，session 失效后 SDK 无法重放调用；连接丢失后请重启 Atlas。旧 stdio 对端需回应发现探测或允许超时。
- 最终结果限额不等于 SDK 的 HTTP 响应缓冲限额。stdio 保留 SDK 的 10 MiB 单条入站消息限制，当前没有 Atlas 传输层的 HTTP 总字节限额。
- 子进程 stderr 会被消费但不展示，SDK 原始日志被禁用：Atlas 只记录安全分类，不含配置凭据、原始协议消息或 SDK 异常文本。
- ACP 会话参数 `mcpServers` 被显式拒绝，请在 Atlas 主机配置服务器；会话级 roots 与配置需要独立的会话隔离实现。
- OAuth 登录与 token 刷新、动态目录、独立 resources/prompts、roots、sampling、elicitation、Tasks、Apps 及模型可见二进制结果仍为 Planned，客户端不声明这些能力。
