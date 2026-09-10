# 手机端远程控制电脑端 Atlas —— 架构与开发计划（审阅稿）

> 状态：**Implemented（Phase A/B/C 已交付，2026-09-08）** · 初稿日期：2026-09-08
> 交付记录：Phase A（atlas_ws + atlas server）与 Phase B（Flutter 移动端远程
> 连接）按本文档实现并验证；文档 §15 分阶段计划与 §21 待决问题中的 Q1/Q4/Q9
> 采纳推荐默认（atlas server 为唯一 runtime owner、允许多连接）。工作目录
> 不随 profile 前置必填：连接与浏览历史都不需要它，首次发消息前由 UI 引导
> 设置并回写 profile（ACP wire 的 session/new 要求 cwd 必传，2026-09-09
> 调整）。远程文件浏览器/远程终端（Q7）与 P3（桌面统一走 server）仍
> 为后续工作项。

---

## 1. 背景与目标

Atlas 是跑在电脑上的本地通用 agent：唯一的 `atlas_runtime` 引擎、本地工具
（read/write/edit/shell/plan）、模型 provider 与 SQLite 持久化
（`~/.atlas/atlas.db`）都在电脑端。现有界面：Nocterm TUI、Flutter 桌面/
移动客户端。Flutter App 本地模式在进程内起 AcpServer 并以 AcpClient 身份
消费它；"远程连接"目前只支持以 stdio 拉起第三方 ACP agent。

**目标**：让手机（Android/iOS）远程操作电脑上的 Atlas——查看/新建/恢复/
删除会话、发 prompt、实时看模型输出与工具执行、取消、切模型/effort、
/compact、权限请求——而模型调用、Shell、文件修改、数据库全部留在电脑端。

### 1.1 完成标准

- 电脑端 `atlas server` 启动仅本机监听的 WebSocket ACP 服务端，复用
  `atlas_composition` 组装的唯一 runtime。
- 手机 Atlas App 不建本地 runtime，手动录入地址+token 后连接，用现有
  Workspace UI 完成上述全部操作。
- 断线重连后以 `session/load` 持久化 timeline 校准展示，**不自动重放
  断线前 prompt**。
- 服务端默认仅监听 127.0.0.1；跨网访问推荐 Tailscale；不开公网端口。

### 1.2 非目标（第一版不做）

| 不做 | 理由 |
|---|---|
| 远程控制屏幕/鼠标/键盘（远程桌面类） | 产品形态不同 |
| 手机直接读写电脑文件系统 / 直接执行电脑 Shell | 由 agent 工具在电脑端执行，结果经 ACP 回传 |
| 手机保存或使用模型 API key | 凭据只留电脑端 |
| 公网匿名访问 / 云中转 / 多用户账号 / 多 server 集群 | 无需求且扩大攻击面 |
| 第一版 QR 配对 | 手工录入已可验证；QR 二期 |
| 第一版远程交互式终端面板 | 独立大特性，见 §11 |
| 为远程发明第二套业务 REST API | 直接复用 ACP（§5） |

---

## 2. 现状盘点与可复用资产

```text
apps/atlas_flutter      桌面/移动 ACP 客户端（本地=进程内 AcpServer+AcpClient）
apps/atlas_cli          组合根：atlas→TUI；atlas acp→stdio ACP；规划 atlas server
packages/atlas_composition   composeRuntime(config)→唯一 AgentRuntime
packages/atlas_runtime   领域模型/事件/端口/唯一 agent loop/会话锁
packages/atlas_acp       AcpServer+AcpClient（acpd 引擎；stdio/channel/memory transport）
packages/atlas_ws        空壳目录（Planned：版本化 WebSocket wire contract）
packages/atlas_tools     内置工具；packages/atlas_storage Drift 持久化
```

关键结论：

1. **不需要第二套 agent loop**：ACP 已覆盖 session 全生命周期、prompt、
   事件流、权限请求、取消、compaction。
2. **`AcpServer` 传输已与通道解耦**：`serveChannel(StreamChannel<String>)`
   接受任意字符串通道；WebSocket 只需提供"一条消息一个 frame"的通道，
   即可零协议改动接入。atlas_acp 增量极小（公开 `serveTransport`，若需要）。
3. **`AcpClient` 与展示层解耦**：`RuntimeEnvironment.runtime` 是
   `PresentationAgentSession`；远程连接注入另一个 AcpClient 实例即可，
   workspace 展示层不感知 WebSocket。
4. **权限模型现成**：客户端 `PermissionPort` 处理
   `session/request_permission` → 手机弹权限对话框的机制已存在。
5. **`AcpConnection`（command+args）≠ 远程连接（url+token）**：需新增独立
   连接模型（§9.2）。

---

## 3. 总体架构

```text
┌──────────────────────────┐
│   手机 Atlas App（android/ios）│
│   RemoteConnectionProfile │
│   AcpClient                │
│   Workspace UI（现成）      │
└───────────┬──────────────┘
            │ WebSocket text frame = 一条 JSON-RPC 消息
            │ ws://<电脑tailnet名>:8765/acp  或  wss://…（tailscale serve）
┌───────────▼──────────────┐
│        Tailscale          │   私有加密 mesh / 可选 serve HTTPS
└───────────┬──────────────┘
            │ 127.0.0.1:8765
┌───────────▼──────────────┐
│      atlas server         │   (atlas_cli 新子命令)
│  WebSocket listener       │
│  token 认证/限流/审计       │
│  每连接一个 AcpServer      │
└───────────┬──────────────┘
            │ 同进程唯一 AgentRuntime（会话锁）
            ▼
   atlas_provider / atlas_tools / atlas_storage（atlas.db）
```

不变量：模型请求、工具执行、文件/Shell、SQLite 只在电脑端；手机永远是
ACP 客户端，不持有 provider 凭据、不复制 agent 循环。

---

## 4. 决策摘要

| # | 决策 | 一句话理由 |
|---|---|---|
| D1 | 协议 = ACP over WebSocket，每帧一条 JSON-RPC | ACP 已有全生命周期/事件/权限语义，官方 RFD 亦以 WS 为远程形态 |
| D2 | `atlas server` 作唯一 runtime owner | 进程内会话锁有效，杜绝多进程共享 DB 的并发 turn |
| D3 | 网络：localhost + Tailscale，不开公网端口 | agent 拥有 Shell/文件权限，公网直曝=开放远程执行 |
| D4 | 安全 = Tailscale（网络）+ Bearer token（应用）双层 | 网络成员≠应用授权；token 可独立轮换 |
| D5 | 断线不自动重放；服务端 in-flight turn 继续跑完 | 防副作用重复执行；结果可 session/load 恢复 |
| D6 | 移动端启动不建本地 runtime，直达远程连接页 | 手机无 config/凭据/工作目录，本地 runtime 只会产生错误数据 |
| D7 | 远程模式隐藏本地 FileBrowser/TerminalPanel | 手机端它们操作的是手机自己的文件系统/shell |

---

## 5. 协议设计：ACP over WebSocket

### 5.1 为什么不设计新 REST API

REST+SSE 需重造：session 生命周期、事件映射、permission 往返、取消、
响应-请求关联、compaction。最终两套协议、两个 client、双份维护。ACP 是
双向协议（agent 可发 `session/request_permission` 并等同一连接内回复），
WebSocket 全双工天然匹配。

### 5.2 依据

- ACP v1 transports：stdio 是标准；**custom transports 被允许**，但必须保留
  JSON-RPC 消息格式与 ACP 生命周期。
- ACP RFD "Streamable HTTP & WebSocket Transport"（in-progress）：WS 被提为
  远程形态；同一 JSON-RPC/生命周期；v1 断线不重放、resumability 属 v2。
  本方案与其一致，未来可平移。
- 参考：agentclientprotocol.com/protocol/v1/transports.md
  agentclientprotocol.com/rfds/streamable-http-websocket-transport.md

### 5.3 Wire 约定（Atlas custom transport v1，atlas_ws 自文档化）

- Endpoint：`GET /acp`（HTTP Upgrade）。
- 认证：HTTP `Authorization: Bearer <token>`（URL 不带 token）。
- 每个 **text frame** = 一条完整 JSON-RPC 2.0 文本（天然分帧，无需 NDJSON）。
- 双向：request / response / notification 均可；`session/update` 为通知。
- **binary frame / 非法 JSON / 超大帧 / 未认证 → 服务端关闭连接**。
- JSON-RPC `id` 由客户端生成；响应按 id 关联；通知无响应。
- 能力协商、session 方法、事件结构、`_atlas.dev` 扩展全部沿用 atlas_acp
  （与 stdio 一致）。
- 单连接多 session 并发读；同一 session 的 turn 保持串行（runtime 锁）。

```jsonc
// 手机 → 电脑（text frame）
{ "jsonrpc": "2.0", "id": 12, "method": "session/prompt",
  "params": { "sessionId": "session-…", "prompt": [
      { "type": "text", "text": "检查 Android 构建问题" } ] } }

// 电脑 → 手机（text frame）
{ "jsonrpc": "2.0", "method": "session/update",
  "params": { "sessionId": "session-…",
              "update": { "sessionUpdate": "agent_message_chunk",
                          "content": { "type": "text", "text": "我先看日志。" } } } }
```

### 5.4 生命周期

```text
手机                               atlas server
 │  WebSocket upgrade（Authorization） │
 │  initialize ─────────────────────> │
 │ <─ initialize（能力/版本）───────── │
 │  session/list|new|load|resume      │
 │  session/prompt ─────────────────> │
 │ <─ session/update 流式事件 ─────── │
 │ <─ session/request_permission ──── │ （如需要，双向）
 │   （回复 permission）────────────> │
 │  session/cancel ─────────────────> │
 │ <─ prompt 终态响应 ─────────────── │
```

唯一区别：AcpClient 的 transport 换成 WebSocket 字符串通道。

---

## 6. 关键决策详解

### 6.1 D2：电脑端单一 runtime owner（最重要）

`AgentRuntime` 的会话锁是**进程内**锁。若"手机直连另一个进程里的 runtime"
与"电脑 TUI/桌面本地 runtime"共享同一 atlas.db，两进程互不知晓对方持锁 →
同一 session 并发 turn、持久化交错。

**采用**：`atlas server` 进程成为唯一 runtime owner（唯一 SQLite 写者），
一切客户端（手机，未来桌面 Flutter）作 ACP client 连它。

> 若真实需求是控制"已打开的桌面 Flutter 窗口内的 runtime"，则不能另起
> atlas server 指向同一 DB；应在该 Flutter 进程内监听 WebSocket，复用其
> 已有同一 AgentRuntime（每远程连接包一个 AcpServer）。协议不变，只换
> host——见 Q1。

### 6.2 D5：断线不自动重放

- 服务端：连接断开**不**取消 running turn；通知写入变 no-op，turn 继续
  跑完并持久化（completed/aborted/failed）；只有显式 `session/cancel`
  才取消。
- 客户端：重连后 `session/load` 以服务端 timeline 覆盖本地；末次 prompt
  无终态响应时提示"结果可能已执行，请确认"，不自动重发。
- 依据：ACP 远程 transport RFD：v1 无消息重放与流恢复。

### 6.3 D1：协议收敛在 atlas_ws

`packages/atlas_ws`（现 Planned 空壳）负责：WS 接入、认证握手、帧边界/
大小、连接数、ping/pong、断连清理、审计——不组装 runtime，不拥有 ACP
语义。依赖方向 `atlas_ws → atlas_acp → atlas_runtime`，与文档一致。

---

## 7. 断线与恢复语义（契约）

手机是移动网络，断线是常态。契约先行：

```text
连接断开：
  - 服务端：in-flight turn 继续跑完（D5），结果落库
  - 客户端：保留本地缓存 → 指数退避重连（1s/2s/4s/…/30s 封顶）
      → 重连成功 → 刷新 session/list → 当前 session 执行 session/load
      → 以持久化 timeline 校准展示
      → 提示"连接已恢复；上次消息状态以电脑端为准"，不自动重发
  - 用户主动取消（stop）→ session/cancel → runtime 取消，已收文本保留
  - 服务端不可达/重启 → UI 显示"电脑端未运行"，提示启动 atlas server
```

- App 退后台停止快速重连，回前台立即尝试一次；用户主动断开不自动重连。
- 丢失的只是断线期间的实时增量，不是会话数据。
- 服务端退出：关闭 listener → 关闭各连接 socket → 客户端按断开处理。

---

## 8. 认证与授权

### 威胁模型（关键前提）

AGENTS.md 明确：Atlas 工具以 Atlas 进程权限运行，**无沙箱、无权限提示、
无 approval gate**；协议适配器不得宣称 runtime 没有的安全边界。因此：
能连上 server 的客户端 = 能读写本机文件 + 执行 shell 的客户端。
认证的目标不是"限制工具"，而是"只有授权的人/设备能连"。

### 8.1 推荐：Tailscale（网络层）+ Bearer token（应用层）

| 层 | 作用 | 控制者 |
|---|---|---|
| Tailscale | 网络可达性 + 链路加密（WireGuard）+ ACL | 用户 tailnet ACL |
| Bearer token | 应用认证：知道 token 才能过 ACP initialize | atlas server |
| （可选）防火墙 | 纵深防御，确保 8765 不暴露 LAN/公网 | 用户 |

- 无账号系统、无 OAuth、无公网注册。
- token：`Random.secure()` 32+ 字节 → hex/base64url；首启生成；
  文件 `~/.atlas/remote_token`（0600）；token 在每次启动时打印到终端；
  **常数时间比较**；拒绝延迟恒等；不进 URL/日志/进程参数。
- 移动端保存：`flutter_secure_storage ^11.0.0`（Android Keystore / iOS
  Keychain；已核实 pub 最新版与当前 SDK/Flutter 兼容）；profile 其余字段
  （URL/名字）放普通偏好。

### 8.2 否决的备选

| 方案 | 否决理由 |
|---|---|
| 纯公网端口 + token | 无沙箱 agent 裸奔公网；token 泄露=远程 shell |
| 只信 Tailscale 身份（无 token） | 需额外库解析 tailnet 身份；token 已覆盖"同 tailnet 内仅授权设备"粒度；留 Q8 |

---

## 9. 移动端改造

### 9.1 启动分流

- 桌面：现状保留（本地 bootstrapRuntime）。
- Android/iOS：跳过本地 bootstrap → 远程连接页（空态引导 → 录入
  URL/token → 连接中 → 已连接进 Workspace / 失败可重试，profile 不丢）。
  工作目录不是 profile 必填项：首个 draft 发消息前，composer 上方提示条
  「Choose directory」→ 输入绝对路径 → 校验通过后回写 profile（secure
  storage）并新建以该目录为根的 draft；未设置前 send() 被护栏拦截并提示，
  不会以手机本地路径（沙箱）误建远端会话。

### 9.2 连接模型：新增 RemoteConnectionProfile

```dart
final class RemoteConnectionProfile {
  final String name;            // "我的电脑"
  final Uri wsUrl;              // wss://host/acp
  final String token;           // 仅内存/secure storage，不落普通 JSON
  final String? workingDirectory; // 电脑端绝对路径；null = 首消息前设置
  final DateTime lastConnectedAt;
}
```

- 与 `AcpConnection`（command+args，桌面 stdio 第三方 agent）并存，
  互不侵入；设置对话框的 ACP connections 列表语义不变。
- 存储：普通 JSON 只放 name/wsUrl/lastConnectedAt；token 走 secure storage。

### 9.3 连接流程 bootstrapRemoteConnection

```text
AcpClient over WsChannel(wsUrl, {Authorization: Bearer token})
  → client.connect()（initialize）
  → 临时 session/new 探测 → 读 model catalog → 删探测 session
    （复用 bootstrapAcpClient 现有模式）
  → RuntimeEnvironment(runtime: client, models: catalog, …)
  → RuntimeEnvironmentController 切换（现有机制 + onClose 关 ws）
```

`AcpClient` 已实现 `PresentationAgentSession` + `PermissionPort`；
`WorkspaceController._subscribePermissions()` 自然接到服务端权限请求 →
现有弹窗。展示层零改动。

### 9.4 UI 增量

- 连接页（profile 管理：新建/编辑/删除/切换）。
- 连接状态条：已连接 / 重连中 / 离线（服务端不可达）。
- 远程目录提示条：profile 未带工作目录且焦点为 draft 时，composer 上方
  显示「Sessions run in a directory on your computer → Choose directory」；
  设置后该 profile 免再提示。
- 会话操作全部复用现有 workspace（list/new/resume/prompt/cancel/
  rename/delete/compact/usage/diff 展示）。
- 桌面不回归：仍走本地 runtime；ACP connections 列表原样。

---

## 10. 服务端：atlas server（atlas_cli 子命令）+ atlas_ws

### 10.1 CLI

```text
atlas server [--listen 127.0.0.1:8765] [--token-file PATH]   # 启动时打印 token
atlas server --rotate-token
```

- 默认 `127.0.0.1:8765`（安全默认，不默认 0.0.0.0）；`--listen 0.0.0.0:…`
  显式开放并打印警告（公网暴露建议经 Tailscale）。
- 复用 `composeRuntime(config)`；多连接共享同一 runtime（会话锁已串行）。
- 退出语义对齐 `atlas acp`：flush 后显式退出；优雅关闭全部连接。

### 10.2 atlas_ws 包（Planned 落地）

```text
packages/atlas_ws/
  lib/atlas_ws.dart
  lib/src/ws_transport.dart   WebSocket ↔ acpd Transport（StreamChannel 适配）
  lib/src/ws_server.dart      HttpServer + upgrade → 每连接一个 AcpServer
  lib/src/token_auth.dart     token 生成/常数时间校验/轮换/文件 0600
  test/…
```

依赖：`atlas_acp`、`dart:io`、`crypto`（常数时间比较；确认已在 lock）。
不引入三方 WebSocket 库（AGENTS：仅当实现需要时才加专用依赖；dart:io
先评估是否够用）。

### 10.3 atlas_acp 增量（很小）

现有 `AcpServer` 已按 transport 抽象（stdio/channel/memory）。WebSocket 是
新 transport 形态：公开 `serveTransport(Transport)`（现为私有）即可，
**不复制协议逻辑**。

### 10.4 连接生命周期与并发

- 每客户端 = 独立 WS + 独立 AcpServer 实例 + 一次 initialize。
- 服务端不做应用层 session 所有权互斥：同一 session 多客户端并发由
  runtime 会话锁兜底；"同一 session 仅单客户端操作"是 UI 层建议。
- `atlas acp` 与 `atlas server` 同一进程一次只跑一个；用户可开两个进程
  共享同一 DB——与今天"同时跑 atlas 与 atlas acp"同风险（跨进程无锁），
  **不在本计划解决**（Q6，文档标注已知边界）。

---

## 11. 文件与终端（远程模式的呈现边界）

`FileBrowser` / `TerminalPanel` 走本地 dart:io / pty2。手机远程模式下
它们操作的是手机自己的文件系统/shell——语义错误，因此：

- **远程模式不渲染**本地文件浏览器与本地交互终端。
- **保留呈现**：会话列表、对话、模型/effort、usage、工具调用与结果、
  文件编辑 diff（ACP tool_call_update 已带 diff content block）、权限
  对话框、compaction、session 元数据。
- Shell 工具输出以文本（rawOutput）呈现；**不做**手机↔电脑流式伪终端/
  远程文件浏览器（独立大特性，Q7，二期；若做则服务端限定
  workingDirectory + additionalDirectories，走 ACP fs/terminal 扩展）。

---

## 12. 测试计划

### 12.1 atlas_ws（Dart 单测 + 集成）

- 认证：无 token 拒绝、错 token 拒绝、轮换后旧 token 失效、token 文件 0600。
- 帧：合法 JSON-RPC 往返；非法 JSON / binary frame / 超大帧 → 关连接。
- 传输：request/response id 关联；服务端主动 request（权限）可达客户端；
  session/update 通知流式到达；多连接并发；单连接多 session 并行。
- 生命周期：客户端中途断线 → 服务端 in-flight turn 继续完成（假 provider
  慢响应）→ 重连后 session/load 见终态；服务端退出 → 客户端收到 close。
- 多连接共享 runtime：会话锁语义不回归。

### 12.2 atlas_acp 增量

- `serveTransport` 公开后：stdio/channel/memory/ws 同一套 handler 测试跑通
  （现有测试补 ws 变体，协议行为零新增）。

### 12.3 atlas_cli server

- 启动/关闭/帮助；`--rotate-token`；默认 127.0.0.1 断言。

### 12.4 Flutter

- remote_bootstrap：本地起假 ws server → profile 连接 → catalog →
  session 列表 → prompt → 权限弹窗 → 断线重连状态机。
- secure storage：`FlutterSecureStorage.setMockInitialValues` 验证 token
  不进普通存储/日志。
- 平台分流：Android/iOS 目标不启动本地 runtime。
- 桌面本地模式全量回归（现有 135 用例不得红）。

### 12.5 端到端手工验收（用户执行）

```text
电脑：tailscale up；atlas server --listen 127.0.0.1:8765
手机：同 tailnet；profile ws://<tailnet名>:8765/acp
流程：建会话 → prompt → 流式回复 → 让 agent 读/改一个文件（看 diff）→
      断 wifi 5s → 重连 → session/load 显示已完成 turn（无重复执行）→
      cancel 生效 → rotate token → 旧连接断开、新 token 可连
```

---

## 13. 依赖清单（新增全部）

| 用途 | 包 | 版本（已核最新） | 说明 |
|---|---|---|---|
| 移动端安全存储 | flutter_secure_storage | 11.0.0 | 实现期 `flutter pub add`（按仓库规则） |
| 常数时间比较 | crypto | 已在 lock（传递） | 直接使用 |
| WebSocket | dart:io（无三方） | — | 先评估够用；不足才加专用包 |
| 图标/UI | lucide/material_ui 等已有 | — | 无新增 |

Tailscale 为部署依赖（非代码依赖），进文档/验收步骤，不进 pubspec。

---

## 14. 安全要求清单

实现状态（Phase A/B 交付后复核，2026-09-08）：
- [x] 默认仅监听 localhost；对外 `--listen` 打印明确警告
- [x] Upgrade 前校验 Authorization；token ≥256 bit；文件 0600
- [x] token 不进 query/日志/进程参数；rotate 后旧 token 立即拒绝新连接
      （已建立连接保持，属有意语义）；常数时间比较
- [x] 帧上限（8 MiB）、连接数上限（4）、30s ping
- [ ] **未实现（留作后续）**：90s idle 断开、未认证限速——当前由连接数
      上限与 ping 兜底，恶意槽位占用有界（拒绝后立即释放）
- [x] 非 /acp 路径 404；拒绝 binary/超大帧并立即释放连接槽
- [x] 服务端退出关闭全部连接；连接/断开事件日志（stdout/stderr 回调，
      不含 token/provider key）；持久化审计文件留作后续
- [x] 手机可见仅 model descriptor/usage/tool update/error summary；API key
      与上游请求/响应头不离开电脑端

---

## 15. 分阶段交付计划（每阶段独立可合并、可回滚）

### Phase A：atlas_ws + atlas server（纯电脑侧）

- 内容：新包 atlas_ws（transport/auth/server）、atlas_cli server 子命令、
  atlas_acp 公开 serveTransport（若需要）。
- 交付后：电脑可 `atlas server`；用 dart 测试客户端/脚本验证全协议
  （initialize → session/new → prompt → cancel → load）。手机尚无 UI，
  但协议与可靠性已钉死。
- 验证：`dart test packages/atlas_ws`、cli-build、本地 ws 冒烟。
- 可独立发布：任何 ACP 客户端可接入（未来桌面远程连接也依赖它）。

### Phase B：Flutter 移动端远程连接（依赖 A 的 wire 约定）

- 内容：平台分流、RemoteConnectionProfile、remote_bootstrap、连接页/状态
  条、远程模式隐藏本地 File/Terminal。
- 交付后：手机连电脑端 Atlas 完成全部会话操作 + 断线重连。
- 验证：`mise run ci` 全绿；真机/模拟器 + 电脑手工验收；
  `mise run app-build-android` / `app-build-ios`。
- 说明：B 的 UI 壳（连接页/状态条）可与 A 并行开发，但集成依赖 A 的
  wire 定稿，否则返工。

### Phase C：桌面/移动一致性打磨与文档（可选收尾）

- 桌面设置新增"远程连接"入口（复用 B）。
- 文档：README、docs/architecture.md、docs/protocol-acp.md、
  docs/configuration.md + zh-CN 同步；atlas_ws / atlas server 从 Planned
  转 Available；本审阅稿归档。
- 安全审计复核。

---

## 16. 回滚策略

- A/B/C 各自独立提交；A 回滚=revert atlas_ws+server 子命令（不触碰现有
  acp/stdio/本地路径）；B 回滚=revert 移动端分流（桌面不受影响；B 前手机
  版本就不可用，回滚只是回到原状，无数据风险）。
- 数据：零 schema 变更、零迁移；token 文件删除即失联，可重建。
- 外部状态：无云服务、无对外 API、无订阅。

---

## 17. 被拒绝的方案

| 方案 | 拒绝理由 |
|---|---|
| A. 手机直连 provider API | key 上手机、第二套 agent loop、工具无法安全执行、session 分裂 |
| B. REST + SSE 业务 API | 双向协议需重造；两套 client；ACP 升级双份维护 |
| C. 公网端口转发 | 无沙箱 agent 裸奔公网 |
| D. 零信任网络即认证（只用 Tailscale） | 网络成员 ≠ Atlas 授权；无法单独撤销 |

---

## 18. 风险与缓解

| 风险 | 说明 | 缓解 |
|---|---|---|
| 无沙箱语义被误解为安全边界 | ACP/token 只控"谁能连"不控"连上能做什么" | 文档明确；默认仅 127.0.0.1；不宣称权限闸 |
| 断线语义分叉 | 本地 turn 一旦开始跑到底；远程若断即取消会丢结果 | 契约：服务端 in-flight 继续跑，重连靠 load 校准（§7） |
| WS 无正式 ACP 标准 | 自研 wire 未来要对齐 ACP v2 | wire 最小化（JSON-RPC over text frame）；atlas_ws 自文档化版本 |
| 移动端后台被杀连接 | 重连风暴/状态混乱 | 前台恢复触发重连 + load 校准；退避封顶 30s |
| 桌面本地模式回归 | 入口分流影响桌面 | 按平台分支，桌面路径零改动；现有用例全量回归 |
| token 明文泄露 | 截图/日志 | 只打印一次；0600；不进 URL；secure storage |
| **最脆弱假设** | 电脑端权威 runtime 由 atlas server 承担 | 若真实需求是控制已打开桌面窗口的 runtime：host 改 Flutter 进程内 listener，协议/认证/移动端设计不变（Q1） |

---

## 19. 架构决策记录（ADR 摘要）

| 决策 | 结论 | 否决备选 |
|---|---|---|
| D1 协议 | ACP over WebSocket（atlas_ws） | REST+SSE |
| D2 服务端承载 | atlas server 子命令 + composeRuntime | 桌面 App 内嵌 server（Q1 后定，二期不否定） |
| D3 认证 | Tailscale + Bearer token 双层 | 公网裸奔；纯 tailnet 身份（Q8 暂缓） |
| D4 断线 | 服务端 in-flight 不取消；客户端 load 校准、不重发 | socket 断即取消 |
| D5 移动端 | 不建本地 runtime，纯远程 client | 手机本地 runtime |
| D6 文件/终端 | 远程模式不渲染本地 FileBrowser/Terminal；diff 文本呈现 | 远程文件系统/远程 pty（二期） |

---

## 20. 术语

| 术语 | 含义 |
|---|---|
| ACP | Agent Client Protocol（agent↔client 的 JSON-RPC 协议） |
| AcpServer / AcpClient | atlas_acp 现成实现（agent 侧 / client 侧） |
| RuntimeEnvironment | Flutter 注入展示层的运行时门面（现成） |
| RemoteConnectionProfile | 新增：name + wsUrl + token 的连接模型 |
| Tailscale Serve | 把本机端口暴露给 tailnet 的官方代理（HTTPS） |

---

## 21. 待决问题（请审阅时拍板）

| # | 问题 | 选项 | 我的建议 |
|---|---|---|---|
| Q1 | 被控"电脑端 Atlas" = (a) atlas server 新进程（本计划按此），还是 (b) 桌面 Flutter 窗口内已启动的 runtime？ | a/b | (a)；选 (b) 则 Phase A 改为 Flutter 进程内 WS listener，协议不变 |
| Q2 | 电脑端如何常驻？ | 手动 / systemd / 桌面 App 内嵌 | 手动 + 文档；内嵌启动属另一特性 |
| Q3 | 是否需要"已连接设备列表/踢下线"管理 | 要/不要 | 要（atlas server sessions + 断开），安全可见性 |
| Q4 | 允许多手机同时连同一 server？ | 允许多连接 | 允许多连接（runtime 锁已串行），同一 session 建议单客户端操作 |
| Q5 | 移动端 token 输入 UX | 手工粘贴 | 手工粘贴第一版；QR 二期 |
| Q6 | 多进程共享 atlas.db 的并发（server 与桌面本地同跑） | 不管/管 | 不管，文档标注已知边界（与现状同风险） |
| Q7 | 远程文件浏览器 / 远程终端 | 不做/二期 | 二期独立特性，走 ACP fs/terminal 扩展 |
| Q8 | Tailscale 身份直接免 token | 做/不做 | 不做（第一版 token 已够） |
| Q9 | 远程会话工作目录 | 默认电脑端家目录 / 手机先选项目 | 默认家目录；项目选择属二期（server 端 cwd 白名单） |

**Q1 决定 Phase A 的 host 归属；Q2/Q3 影响 server 命令面；其余可默认。**

---

## 22. 附录：命令速查（实现后）

```sh
# 电脑端
tailscale up
atlas server --listen 127.0.0.1:8765

# 手机端 Atlas App → 新建远程连接
# ws://<电脑tailnet名>:8765/acp（Tailscale MagicDNS）或经 serve 的 wss://…

# 验证（开发期 dart 脚本/测试）
# initialize → session/new → session/prompt → session/update 流 → session/cancel
```
