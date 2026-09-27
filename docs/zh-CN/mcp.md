# MCP 工具

[English](../mcp.md)

Atlas 作为 MCP 客户端，将配置的服务器工具与 `read`、`write`、`edit`、`shell`、`plan` 放入同一个 agent loop。通过 `mcp_dart` 2.4.2 支持 stdio 和 Streamable HTTP。尚未实现 Atlas 自身作为 MCP 服务器。

## 配置

在现有 `~/.atlas/config.yaml` 中增加可选列表 `mcp_servers`，默认空列表。修改后重启 Atlas。

```yaml
mcp_servers:
  - name: local_tools
    transport: stdio
    command: /absolute/path/to/mcp-server
    args: []
    cwd: ~/workspace
    env:
      SERVICE_TOKEN: ${SERVICE_TOKEN}
    startup_timeout_seconds: 15
    call_timeout_seconds: 60

  - name: remote_tools
    transport: streamable_http
    url: https://tools.example.com/mcp
    headers:
      Authorization: Bearer ${MCP_TOKEN}
    enabled: true
```

将示例命令与地址替换为已安装的 MCP 服务器。Atlas 不安装服务器，也不将 `command` 作为 shell 命令解析；字面参数放入 `args`。不会发起 OAuth 浏览器登录，远程服务器需要支持无认证或配置的静态请求头/token。

| 字段 | 规则 |
|---|---|
| `name` | 唯一；1–128 个 ASCII 字母、数字、下划线或连字符。 |
| `transport` | `stdio` 或 `streamable_http`。 |
| `enabled` | 布尔值，默认 `true`；禁用项检查结构，但不展开凭据、不连接。 |
| `startup_timeout_seconds` | 正整数，默认 15；连接和全部工具分页共享截止时间。 |
| `call_timeout_seconds` | 正整数，默认 60；进度不会延长总截止时间。 |
| `command`、`args` | stdio 可执行程序与可选字符串参数列表，不做 shell 展开。 |
| `cwd` | 仅 stdio；绝对路径，展开开头的 `~/`；省略时使用 Atlas 启动目录。 |
| `env` | stdio 字符串映射，覆盖 bootstrap 环境快照。 |
| `url` | HTTP(S) 地址，不含 userinfo 或 fragment；保留 query。 |
| `headers` | HTTP 字符串映射，不允许覆盖 MCP 协议、Host、Content-Type、Content-Length、Accept、Last-Event-ID、Connection、Transfer-Encoding 请求头。 |

`env`、`headers` 的值支持 `${VAR}`。启用项引用未定义变量时，配置加载失败并报告字段路径。请求头名必须合法且大小写无关唯一，值不能包含换行。未知字段或与传输类型不兼容的字段会被拒绝。时长必须在 Dart 可表示范围内。

CLI 继承启动环境。Flutter 桌面使用已有 bootstrap 快照，包括 macOS 登录 shell 导出的变量。服务器 `cwd` 在连接期间固定，不跟随 Atlas 会话目录变化。

## 资源归属与启动

CLI TUI、`atlas acp`、`atlas server`、Flutter 桌面本地模式分别组装自己的 MCP 连接。移动端与远程客户端使用主机端配置的工具，不启动 MCP 进程、不转发本机 MCP 凭据。每个主机 runtime 的所有会话共享每个服务器的一条连接。

在 runtime 可用前，按配置顺序连接服务器。任一启用项失败，报告服务器名和安全错误分类，启动失败，并关闭先前已打开的连接。可禁用失败项并重启以继续使用。帮助、版本、缓存统计及 token 轮换不连接 MCP。

进程退出先等待活动 turn 完成清理，再关闭 MCP、provider HTTP 客户端和 storage。CLI 启动期间收到 SIGINT/SIGTERM 会取消发现并清理子进程。普通远程客户端断线仍保留主机上的运行中 turn。SDK 子进程清理针对所持有的进程，不能保证终止其主动脱离或另外创建的后代进程。

## 工具行为

工具名由可读的 `mcp_` 前缀、处理过的服务器/工具名和稳定 SHA-256 后缀组成，最长 64 个模型 provider 兼容字符；调用仍使用原始工具名。服务器按配置排序，每个服务器内部按原始工具名排序。输入 schema 和说明通过现有 runtime 描述传给 provider。

工具目录在启动时固定。开启日志时，目录变化通知对每条连接只记录一次 `mcp.catalog_changed_restart_required`。新协议使用已确认的工具列表订阅。重启以刷新定义。必须通过 Tasks 执行的工具会被省略并记录安全诊断。服务器返回的 instructions 不会加进系统提示词。

目录元数据体积有上限，因为它会进入每次 provider 请求：服务器提供的 description 超过 2 KiB UTF-8 会被截断并加明确标记；单个工具 input schema 超过 64 KiB 会导致启动失败；单个服务器的目录元数据合计超过 1 MiB 也会导致启动失败。后两者会报告服务器名，并阻止其定义进入 provider 请求。

结果保留文本块顺序、内嵌文本资源及资源链接，不自动抓取链接。结构化 JSON 也会进入模型可见输出，包括新协议的标量和数组。图片、音频、二进制资源替换为明确的省略标记；只有不支持的二进制内容时返回适配器错误。空成功结果仍有效。

输出与身份 metadata 合计不超过 **50 KiB UTF-8**，文本和 metadata 都标记截断，不拆分 UTF-8 字符。进度采用有界临时快照，不持久化、不传给模型。每次调用仍产生一条最终持久化结果。现有通用工具视图与 ACP 历史恢复可展示结果，无需新 UI 或数据库 schema。

取消与超时作用于单个请求。stdio 连接保持可用；Streamable HTTP 已发出的请求无法被 SDK 中断，因此 Atlas 会把该连接标记为不可用，而不是每次尝试再泄漏一个请求：失败调用报告超时或取消，之后对该服务器的调用返回 `connection_abandoned`，连接会在后台关闭以释放其余流。被放弃的请求会在服务器响应或进程退出时结束；重启 Atlas 可重新连接该服务器。超时、认证失败、无效结果和传输失败作为安全错误返回模型。取消或断连的操作可能已经产生远端副作用；Atlas 会说明结果不确定，不自动重试。

## 兼容性与当前限制

- SDK 使用 stable 协商配置，支持 MCP 2026-07-28 和旧初始化流程。测试覆盖 stdio、HTTP 的 2025-11-25 与 2026-07-28，以及 HTTP JSON/SSE 响应；这不代表已实现所有 MCP 可选能力。
- Atlas 禁用子进程自动重启和 HTTP 事件流重试，阻止 session 失效后 SDK 自动初始化并重放调用。连接/session 丢失后重启 Atlas。旧 stdio 对端需回应发现探测，或允许超时回退；探测时直接退出的服务器会导致启动失败。
- Streamable HTTP 超时会在该连接上留下恰好一个在途请求；Atlas 不再向该连接发起调用，因此残留开销以配置的服务器数量为上限，而不是以尝试次数为上限。
- 最终结果限额不等于整个 SDK HTTP 响应缓冲限额。stdio 保留 SDK 的 10 MiB 单条入站消息限制；HTTP JSON/SSE 解码可能先分配完整响应，再由 Atlas 截断结果。当前没有 Atlas 传输层 HTTP 总字节限额。
- 子进程 stderr 会被消费，但不直接展示。SDK 原始日志被禁用，Atlas 仅通过现有 logger 记录安全分类，不包含配置凭据、原始协议消息或 SDK 异常文本。
- ACP 会话参数 `mcpServers` 仍被显式拒绝，请在 Atlas 主机配置服务器。会话级 roots 和配置需要独立的会话隔离实现。
- OAuth 登录/token 刷新、动态目录、独立 resources/prompts、roots、sampling、elicitation、Tasks、Apps 及模型可见二进制结果仍为 Planned。客户端不声明这些可选能力。

实现决策与后续范围见[开发计划](plans/mcp-support.md)，完整配置见 [配置指南](configuration.md)。
