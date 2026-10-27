# Atlas 架构

> 本文描述当前的设计。`protocol` 的类型与线上格式还没有稳定，可能不兼容地变化；数据库格式从版本 1 起向后兼容，表结构变化通过迁移升级，不要求用户删库。

这份文档是 Atlas 的顶层设计，也是各包职责与数据流转的权威说明：一轮之中，设置从何而来，数据如何流转、存放在哪里，状态由谁管理。代码与本文不一致时，先判断哪一方有误，再修正有误的一方。

核心思路：**Atlas 的核心对外只有一组接口 `protocol.Service`，桌面 GUI、ACP、WebSocket 都只是它的客户端。**

## 前提

- 入口有多个：桌面 GUI（进程内）、ACP（编辑器以子进程启动 `atlas acp`，走 stdio）、WebSocket（位于 App 进程内，供手机等远程客户端使用）。目前只实现了 GUI，另外两个是后续的适配包，核心按它们的需要设计。
- 多个进程可以打开同一个数据库，各自独立加载会话。同一会话同一时刻只应由一个进程写入；若出现并发写入，后写入的一方收到 `ErrConflict`，该会话在其进程内失效，需要重新打开。不做自动对齐。
- 同一进程内，多个客户端可以同时订阅同一个会话，各自收到全部更新。
- 一个会话同一时刻只做一件事：一轮、压缩、更换模型、删除互斥。
- 一轮归会话所有，与发起它的连接无关：连接断开、取消订阅都不会中止一轮，只有 `CancelTurn` 和进程退出会。
- 消息是不可变的值，创建后不再修改。这是全局约定，不依赖深拷贝来保证。
- Atlas 没有权限审核，工具在本地执行，不依赖客户端提供任何能力（权限确认、文件、终端）。
- 数据库有版本号，表结构变化通过迁移升级，升级失败时整体回滚，库保持原样；库的版本比程序支持的新时报错，提示升级 Atlas。

## 用语

| 用语 | 含义 |
| --- | --- |
| 会话（session） | 用户与 Agent 之间的一段对话，各自保存自己的上下文、历史与状态 |
| 一轮（turn） | 从一条用户消息开始，到 Agent 回答完毕为止的完整过程，其间可能多次请求模型、调用工具。由一次 `SendPrompt` 调用受理，结束时给出停止原因（stop reason） |
| 用户消息 | 用户发给 Agent 的一条消息：文字、图片或技能调用。方法名里的 `Prompt` 专指用户消息，系统提示词一律写 `SystemPrompt` |
| 系统提示词（system prompt） | `systemprompt` 包按工作目录组装、放在每次模型请求最前面的指令；其中的指令来自全局的 `~/.atlas/AGENTS.md`，以及从根目录到工作目录各级的 `AGENTS.md` |
| 系统提示词快照 | 系统提示词变化时记入历史的一条记录，仅用于判断是否需要追加新快照，不进入上下文 |
| 通知（notice） | 只给人看的提示，如压缩失败。记入历史，回放时仍能看到，但不进入上下文，也不发给模型 |
| 订阅者 | 订阅了某个会话、接收其更新的连接 |
| 更新（update） | 核心推送给订阅者的内容，见“更新”一节 |
| 事件（event） | `agent` 的循环交给调用方的流式事件，仅在核心内部使用；核心把它们转换成更新 |
| 压缩（compaction） | 把较早的上下文替换为一段摘要，历史中的原消息保持不变 |
| 设置（settings） | `settings.json`、`auth.json` 中的内容，由 `config` 读取 |
| 配置项 | 会话的选项，Atlas 用它向客户端暴露模型与思考深度 |
| 设置页 | `gui` 里按 `⌘,` 或点侧栏的“设置”打开的界面，修改界面偏好与全局指令；分“外观”“系统”“全局指令”三类 |
| 偏好（preferences） | `gui.json` 中的内容，如主题与字体，只有 `gui` 读写；与 `settings.json` 的“设置”无关 |
| 后台运行 | 关闭窗口只隐藏窗口，应用与核心继续运行，会话与定时任务照常；`⌘Q`、Dock 菜单与托盘菜单的“退出”才结束进程 |
| 启动器（launcher） | `gui` 里按 `⌘K` 或点侧栏的“搜索”打开的搜索面板，同时搜索会话、操作与文件，选中即执行 |
| 定时任务（scheduled task） | 一条预设：到点在指定的工作目录新建一个会话，并发出一条用户消息，界面里称为任务的“指令”。存在 `schedule.json` 里，由 `schedule` 包读写。“任务”一词只指它，不用来称呼会话、一轮或工具调用 |
| 触发 | 定时任务的一次到点，由它的规则算出；触发后的运行就是新会话里的一轮 |
| 会话类型（session type） | 会话的种类，保存后不变，决定它在哪里列出：`interactive` 是用户自己发起、在其中来回对话的会话，`scheduled` 是定时任务触发的。会话另有属主（`ownerId`），任务会话的属主是定时任务的标识，普通会话没有 |
| 任务会话 | 类型是 `scheduled` 的会话，即定时任务的会话。不出现在侧栏与启动器里，只在定时任务视图里列出 |

## 三个概念

| 概念 | 是什么 | 唯一权威 |
| --- | --- | --- |
| 历史 | 会话中发生过的全部记录，只追加，供人查看 | 数据库 `messages` 表 |
| 上下文 | 下一次请求发给模型的消息 | 由历史按 `agent.Fold` 折叠得到 |
| 会话 | 已加载到内存的会话：配置、上下文、是否忙、订阅者 | 核心中的会话对象 |

上下文是历史的投影。两者有三处不同：压缩之后，上下文中较早的消息被替换为摘要，而历史中仍完整保留；系统提示词快照和通知记在历史中，不进入上下文。“上下文 = 依次把历史中的每条记录 `Fold` 进来”，这条规则只定义一次，在三处使用：循环在一轮之内推进自己的工作副本；会话在每条记录保存后推进自己的上下文；加载会话时从数据库重放历史。

压缩记录记下保留的消息条数：折叠到压缩记录时，上下文换成“摘要加上最后这几条消息”。同时，核心知道上下文里每条消息在历史中的序号，追加压缩记录时把会话的上下文起点（`context_from`）设为保留段第一条消息的序号。加载会话时只需从上下文起点开始读，不必读取更早的历史；这一段按同样的规则折叠，得到的上下文与完整折叠一致。

客户端显示的对话是历史的视图，只随核心推送的更新变化，本身不是数据。

## 分层

```
cmd/atlas ── 组装：打开库，建核心，交给入口
   │
   ├─► gui ───┐
   ├─► acp ───┼─► protocol（接口与数据类型，不依赖任何包）
   └─► ws ────┘        ▲
                       │ 实现              tools ─► protocol（rawOutput 的形状）
                     core ──► agent（不依赖任何包）
                       ├────► llm ───► agent
                       ├────► tools ─► agent
                       ├────► store（不依赖任何包）
                       ├────► config
                       └────► systemprompt ► skills
```

`acp`、`ws` 是后续要增加的适配包，其余均已存在。图里只画主要的依赖：`tools` 还用到 `imaging`（图片处理）和 `shellenv`（登录 shell 的环境变量），`gui` 的本地功能也用到这两个包，定时任务用到 `schedule`。

| 包 | 职责 |
| --- | --- |
| `protocol` | 对客户端开放的接口（`Service`、`Subscriber`）与全部请求、响应、更新类型。只有数据，没有逻辑 |
| `core` | 管理已加载的会话，实现 `protocol.Service`。是会话运行时数据的唯一权威。内部分三块：会话运行（忙、持久化编排）、更新转换（事件与消息到更新的纯函数）、模型解析（设置到 `agent.Model`，含回退规则） |
| `agent` | 消息、模型与工具的接口，一轮运行的循环、压缩、上下文折叠，以及消息的编解码。没有状态，不做 IO |
| `llm` | `agent.StreamFn` 的实现（OpenAI Responses），并提供“按设置构造流函数”的入口；核心不知道有哪些模型提供方，也不依赖 OpenAI SDK |
| `tools` | read、write、edit、shell |
| `store` | SQLite：会话表、消息表、blob 表。只存取不透明的消息正文与二进制，不理解消息内容，也不涉及折叠与运行 |
| `config` | 读取 `settings.json`、`auth.json` |
| `systemprompt`、`skills` | 按工作目录组装系统提示词，发现技能 |
| `gui` | 桌面客户端：会话功能只通过 `protocol` 使用，不 import `core`、`agent`、`store` |
| `schedule` | 定时任务的类型、`schedule.json` 的存取与触发判定。只依赖标准库，不依赖任何内部包，也不碰核心；由 `gui` 驱动 |
| `acp`、`ws` | 后续实现：把 `protocol.Service` 与 `protocol.Subscriber` 映射为各自的线上格式，分别经由 stdio 与 WebSocket 传输 |

`gui` 的会话功能只依赖 `protocol`，由编译器保证界面无法获取核心的内部状态：需要什么信息，只能通过接口查询，或等待核心推送。协议只管会话。GUI 里的文件树、编辑器、终端是本地功能，直接使用 `tools` 等包，不经过协议，远程客户端没有这些功能。

## protocol：对外接口

```go
// Service 是 Atlas 对一个客户端连接开放的接口。
type Service interface {
	NewSession(ctx context.Context, req NewSessionRequest) (NewSessionResponse, error)
	OpenSession(ctx context.Context, req OpenSessionRequest) (OpenSessionResponse, error)
	ListSessions(ctx context.Context, req ListSessionsRequest) (ListSessionsResponse, error)
	DeleteSession(ctx context.Context, req DeleteSessionRequest) error
	CloseSession(ctx context.Context, req CloseSessionRequest) error
	RenameSession(ctx context.Context, req RenameSessionRequest) error
	SendPrompt(ctx context.Context, req SendPromptRequest) (SendPromptResponse, error)
	CancelTurn(ctx context.Context, req CancelTurnRequest) error
	SetConfigOption(ctx context.Context, req SetConfigOptionRequest) (SetConfigOptionResponse, error)
	ListHistory(ctx context.Context, req ListHistoryRequest) (ListHistoryResponse, error)
	ListUserMessages(ctx context.Context, req ListUserMessagesRequest) (ListUserMessagesResponse, error)
}

// Subscriber 接收核心推送的更新，每个连接一个。核心在自己的 goroutine 里同步调用它，
// 实现必须很快返回，不得在里面调用 Service。
type Subscriber interface {
	Receive(n Notification)
}
```

核心只有一个，每个连接通过 `core.Connect(subscriber)` 获得一个 `Service`。GUI 在进程内直接调用；`acp`、`ws` 收到请求后调用 `Service` 对应的方法，并把 `Subscriber` 收到的更新序列化后发送出去。

### 方法

会话的生命周期是：新建、打开、关闭。打开即加载（会话尚未加载时）并订阅；关闭即取消订阅。

| 方法 | 做什么 |
| --- | --- |
| `NewSession{cwd, type?, ownerId?}` | 新建会话，返回 `sessionId`、配置项与当前状态。新会话先只存在于内存，保存第一条用户消息时才写库；工作目录此后不再改变。`type` 缺省是 `interactive`；`scheduled` 必须带 `ownerId`。类型与属主随会话保存，此后不变 |
| `OpenSession{sessionId}` | 加载会话并订阅，返回配置项与当前状态，不回放历史；历史用 `ListHistory` 分页读取 |
| `ListSessions{cwd?, type?, ownerId?, cursor?}` | 列出已保存的会话：`sessionId`、`cwd`、`title`、`updatedAt`、`type`、`ownerId`。只列 `type` 指定的一种，缺省是 `interactive`；带 `ownerId` 时只列该属主的会话。侧栏与启动器用缺省值，所以看不到任务会话，分页也不会被它们占满 |
| `DeleteSession{sessionId}` | 删除空闲的会话；会话不存在时不报错 |
| `CloseSession{sessionId}` | 取消当前连接对该会话的订阅，不影响正在进行的一轮。会话没有订阅者且空闲时卸载；若正忙，待这一轮结束后卸载 |
| `RenameSession{sessionId, title}` | 修改标题 |
| `SendPrompt{sessionId, prompt}` | 发送一条用户消息（`prompt` 是内容块列表）。受理成功即返回，这一轮在会话内异步运行；停止原因与出错信息通过 `run_state` 送达。受理前的检查失败时返回错误 |
| `CancelTurn{sessionId}` | 取消正在运行的一轮，该轮以 `cancelled` 结束；没有正在运行的一轮时不报错 |
| `SetConfigOption{sessionId, configId, value}` | 更换模型或思考深度，保存后返回完整的配置项 |
| `ListHistory{sessionId, before, limit}` | 返回一页历史（以更新的形式），供界面向前翻页 |
| `ListUserMessages{sessionId, before}` | 返回全部用户消息的摘要，供会话大纲使用 |

方法名统一为“动词加宾语”，请求与响应类型是方法名加 `Request`、`Response`。对会话本身的操作，宾语是 `Session`；对会话内事物的操作，宾语是那个事物，会话由请求里的 `sessionId` 指明。所有方法都是“请求加响应”的形式，没有单向通知。

每个方法的结果都通过响应返回给调用方。会改变会话视图的变化（新消息、标题、配置项、运行状态等）另以更新推送给所有订阅者，包括调用方自己，所以 `RenameSession` 这类只返回成功或失败的方法，其结果也由更新体现。

`SendPrompt` 受理前会检查：会话存在且空闲；没有可用模型时重新选择一次，仍然没有则返回原因；工作目录仍然有效。检查通过后展开斜杠命令并开始运行。斜杠命令是以 `/` 开头的用户消息：`/compact` 压缩上下文；`/技能名 参数` 在原文后附上该技能的全文，再运行。

打开会话时，当前状态随响应一并给出（`state`，即 `protocol.SessionState`）：工作目录、标题、可用的命令（`compact` 与全部技能）、上下文占用、是否在运行，此后由更新推进。工作目录在会话中不变，技能随工作目录而定，因此命令不再单独推送更新。

不支持更换工作目录：工作目录在新建会话时确定。需要换目录时，新建一个会话。

会话归订阅它的连接所有：`SendPrompt`、`CancelTurn`、`SetConfigOption`、`CloseSession` 只对本连接订阅着的会话有效，也就是在本连接上新建或打开、且还没有关闭的会话。`DeleteSession`、`RenameSession`、`ListHistory`、`ListUserMessages` 不要求先打开会话。连接本身不记录会话，会话由核心持有，所以会话被卸载或失效后，连接上的后续请求同样得到 `ErrNotFound`。打开与卸载串行进行，因此不会出现连接订阅到正在卸载的会话上的情况。

### 配置项

模型与思考深度都是配置项，客户端据此绘制选择器，无需自备模型清单或档位规则：

- `model`：选项是全部可用的模型，值为 `provider/model-id`。每个选项的 `images` 表示该模型接受图片，界面据此判断是否允许附图。
- `thinking`：选项是当前模型支持的档位。模型不支持思考时没有这一项。

更换模型后，思考深度按以下规则调整：原档位若被新模型支持则保留，否则使用新模型的默认档。响应中是完整的新配置项，所有订阅者收到 `config_option_update`。

### 更新

核心推送给订阅者的全部更新（每种更新的种类名是它的 `Type()` 返回值）：

| 更新 | 何时 |
| --- | --- |
| `user_message_chunk` | 用户消息保存后发出。发送消息的客户端同样在收到它之后才显示该消息 |
| `agent_message_chunk`、`agent_thought_chunk` | 流式的文字与思考，同一条消息使用同一个 `messageId`。同一条回复中相邻的两段文字之间补一个空行，流式与回放保持一致 |
| `tool_call` | 模型开始发出一个工具调用：`toolCallId`、`name`、`title`、`kind`、`rawInput`，状态为 `pending` |
| `tool_call_update` | 参数继续流出（`rawInput`）、开始执行（`in_progress`）、结束（`completed` 或 `failed`，带 `content` 与 `rawOutput`） |
| `usage_update` | 每条助手消息保存后发出：`used` 是上下文占用，`contextWindow` 是窗口大小 |
| `compaction_update` | 压缩开始、完成（带摘要）、失败（带原因）、取消 |
| `notice` | 通知，如压缩失败、达到上限 |
| `session_info_update` | 会话第一次保存（有了标题）与改名 |
| `config_option_update` | 更换了模型或思考深度 |
| `run_state` | 一轮开始与结束：`running`、`since`；结束时附带 `stopReason`（`end_turn`、`max_tokens`、`max_turn_requests`、`cancelled`）或 `error`。中途接入的客户端据此得知会话是否在运行 |
| `request_retry` | 一次模型请求失败，等待重试：`messageId`、第几次、等待多久、原因。客户端应丢弃这条消息已流出的内容 |

工具的 `kind`：read 对应 `read`，write 与 edit 对应 `edit`，shell 对应 `execute`，其余对应 `other`。`rawOutput` 是工具附带的结构化数据（`ReadDetails`、`WriteDetails`、`EditDetails`），其形状由 `protocol` 定义，GUI 用它绘制 diff、行号与图片；其他客户端只依据 `title`、`content` 也能正常使用。

所有更新都由核心中同一组转换函数，根据 `agent` 的事件与消息生成：实时运行时由事件生成，`OpenSession` 之后用 `ListHistory` 读取的历史由数据库中的记录生成，定稿的消息在两种情形下都经过同一个转换。客户端因此只有一条渲染路径。

### 错误

| 错误 | 何时 | 客户端应对 |
| --- | --- | --- |
| `ErrNotFound` | 会话不存在，或当前连接没有打开它（包括已卸载、已失效） | 重新打开或新建会话 |
| `ErrBusy` | 会话正忙，不能受理 | 等待当前操作结束后重试 |
| `ErrConflict` | 写入时发现其他进程已写入该会话；会话在本进程内失效 | 重新打开会话 |
| `ErrNothingToCompact` | 上下文里没有可压缩的内容 | 无需处理 |
| `ErrClosed` | Atlas 正在关闭 | 放弃请求 |

其余失败（没有可用模型、工作目录失效、模型调用最终失败等）以带原因的错误返回，或通过 `run_state` 的 `error` 送达。

### 订阅与背压

核心同步调用 `Subscriber.Receive`，所以一个慢订阅者会拖住这一轮和其他订阅者。协议因此规定：`Receive` 必须很快返回；远程适配包自带有界队列，队列溢出时断开该订阅，由客户端重新打开会话并读取历史追上。核心本身不为订阅者缓冲。

## core：会话的唯一权威

核心持有本进程已加载的会话。每个会话的状态由这几项组成：工作目录、是否已写库、已知的最后序号、是否忙、模型与思考深度、系统提示词、工具与技能、上下文、订阅者。其中是否忙是唯一的控制状态：一轮、压缩、更换模型、删除都要先占用它，所以彼此互斥。一轮和压缩可以取消，并会推送 `run_state`；更换模型、删除这类很快的操作不可取消，也不推送。核心的锁只用于读写这些字段，不跨越 IO；唯一的例外是让打开与卸载串行的锁，加载会话时一直持有它。

### 加载

`NewSession` 与 `OpenSession` 同步完成加载后才返回：

1. 已有会话：读取会话信息，先检查工作目录，再读取从上下文起点开始的全部记录，依次 `Fold` 出上下文，并取出最后一份系统提示词快照与最后序号。本进程已经加载过该会话的，直接订阅，不重新加载。
2. 读取 `settings.json`，按“上次的模型 → 默认模型 → 清单中第一个可用的”选择模型。已有会话更换了模型的，保存新选择。没有任何可用模型或缺少凭据时不算失败，此时会话没有可用的模型，下次 `SendPrompt` 时再试一次。
3. 按工作目录组装系统提示词、技能与工具。已有会话的系统提示词与最后一份快照不同时，追加一份新快照。

工作目录不存在、历史损坏时，加载失败并返回错误。新建的会话待配置项读取成功后才登记并归属于连接，失败时不会留下未完成的会话。

### 一轮

`SendPrompt` 的过程：

1. **受理**（同步）：占用会话，不空闲则返回 `ErrBusy`；完成上文所述的检查，展开斜杠命令；取一份 `agent.Input` 快照，其中 `MaxRequests` 固定为 200，不可配置；返回。
2. **运行**（异步，归会话所有）：推送 `run_state{running}`，运行 `agent.Run`。事件到来时：
   - 消息定稿（`MessageEnd`）：先写库，再修改内存，最后推送。
     - 尚未写库：创建会话，标题取这条用户消息的开头，同时写入系统提示词快照。
     - 追加记录时带上期望的最后序号；序号不符返回 `ErrConflict`。任何写库错误都会停止这一轮，之后的事件全部丢弃。冲突之外的写库失败，会话仍然可用；发生冲突则会话失效，被卸载。
     - 消息 `Fold` 进上下文，推进序号。
     - 推送由它生成的更新。
   - 其他事件：直接推送由它生成的更新，不写库。
3. **结束**：释放占用，推送 `run_state{idle, stopReason | error}`；收到过“达到上限”事件的，停止原因是 `max_turn_requests`。会话没有订阅者时卸载。

运行中的 panic：工具的 panic 已收敛为错误结果；其余代码里的 panic 让这一轮以 `error` 结束，会话失效并被卸载，重新打开时从数据库恢复，不影响其他会话与进程。

因此内存中的上下文始终等于数据库中的历史折叠出的结果。

### 多客户端与多进程

- **同一进程**：一个会话只加载一次，所有订阅它的连接都收到每个更新。中途接入的客户端先获取历史（`ListHistory` 分页），再通过 `run_state` 得知会话是否在运行。对于正在流式输出的那条消息，接入之前已发出的部分，该客户端无法看到，待这条消息结束后，它才会出现在历史中。
- **不同进程**：各自加载各自的会话，彼此看不到实时更新。同时写入同一个会话是不支持的用法，一旦发生，由序号检查发现，后写入的一方得到 `ErrConflict`。

## agent：一轮运行

`agent` 是纯逻辑：接收一份快照作为输入，运行，并通过事件交付结果。它不持有会话，也不修改调用方的数据。

```go
// Model 是会话用的模型。引用、窗口和流函数是一个值，总是一起更换。
type Model struct {
	Ref           string   // provider/model-id，唯一标识一个模型
	ContextWindow int64    // 上下文窗口（token），未知为 0
	Stream        StreamFn // 已绑定客户端与模型 ID
}

// Input 是一轮看到的全部，由调用方给出快照，循环不修改它。
type Input struct {
	Model        Model
	Thinking     ThinkingLevel
	SystemPrompt string
	Tools        []Tool
	Messages     []Message // 上下文
	MaxRequests  int       // 一轮最多请求模型的次数，0 为不限
}

// Run 把 user 加进上下文，循环请求模型、执行工具，直到模型不再调用工具、ctx 取消或达到
// MaxRequests；返回这一轮新产生的消息。
func Run(ctx context.Context, in Input, user UserMessage, emit func(Event)) []Message

// Compact 把上下文中除保留段以外的部分压成摘要，返回新产生的消息。
func Compact(ctx context.Context, in Input, emit func(Event)) []Message

// Fold 把一条新记录并进上下文：压缩记录换成“摘要 + 之前保留的 Kept 条”，
// 系统提示词快照与通知不进上下文，其余追加。
func Fold(messages []Message, m Message) []Message
```

消息类型（用户消息、助手消息、工具结果、系统提示词快照、通知、压缩记录）、`Tool`、`StreamFn` 是面向模型的格式，也是数据库中保存的格式，编解码由 `agent` 负责。`AssistantMessage.Model` 由循环按 `Input.Model.Ref` 填入。模型调用失败时的重试次数与退避间隔由包内的策略决定。

`MaxRequests` 数的是这一轮里产生的助手消息个数；失败后重试与上下文溢出后重发的请求已作废，不计入，压缩时请求模型生成摘要也不计入。每次请求模型之前检查：已达上限就追加一条通知并返回，上限是策略，由核心设定。通知不进入上下文，上下文以工具结果结尾，之后的用户消息照常接上。最后一次回复若没有工具调用，这一轮自然结束，不触发上限。

事件只保留核心需要的几种：消息开始、增量、重试、消息定稿（任何一条消息定稿，每条历史记录恰好对应一个）、工具开始、压缩开始、达到上限。工具的结束就是工具结果的消息定稿。

## store

```sql
CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,
    title      TEXT NOT NULL,
    cwd        TEXT NOT NULL,
    model      TEXT NOT NULL,  -- 上次选的模型，provider/model-id
    thinking   TEXT NOT NULL,
    last_seq   INTEGER NOT NULL,
    context_from INTEGER NOT NULL,  -- 上下文起点：读出上下文只需从这个序号开始的记录
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    session_type TEXT NOT NULL,  -- interactive、scheduled
    owner_id   TEXT NOT NULL  -- 会话的属主，如任务会话所属的定时任务；没有时为空串
) STRICT;
CREATE TABLE messages (
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    seq        INTEGER NOT NULL,
    kind       TEXT NOT NULL,  -- user、assistant、tool_result、system_prompt、notice、compaction
    body       TEXT NOT NULL,  -- 消息的 JSON；图片等二进制只存引用
    PRIMARY KEY (session_id, seq)
) STRICT;
CREATE INDEX messages_kind ON messages(session_id, kind, seq);
CREATE TABLE blobs (
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    hash       TEXT NOT NULL,  -- 内容的 SHA-256
    mime       TEXT NOT NULL,
    data       BLOB NOT NULL,
    PRIMARY KEY (session_id, hash)
) STRICT;
```

存储遵循这些规则：

- `messages` 只追加；`kind` 由调用方给出，`store` 不解析 `body`。`kind` 服务两类查询：会话大纲（`user`）、最近一份系统提示词快照（`system_prompt`）。
- 追加记录时带上期望的最后序号，库里的 `last_seq` 与它不符就返回 `ErrConflict`；记录与它引用的 blob 在同一个事务中写入；追加压缩记录时，同一个事务里更新 `context_from`。
- 图片等二进制不放进消息 JSON，而是存入 `blobs`，以内容哈希寻址，同一会话内相同内容只存一份，消息里只留引用。核心在入库前把二进制外置，读出时按需还原；`store` 只提供 blob 的存取。blob 属于会话，会话删除时随外键级联删除，无需单独清理。
- 多个进程可以同时打开同一个数据库（WAL）。停止原因、用量、每条消息的模型与档位等都保存在消息的 JSON 中，不单独建列。版本号记录在 `user_version`，当前是版本 2（版本 2 给 `sessions` 加了 `session_type` 与 `owner_id`，旧会话都是 `interactive`，没有属主）。打开数据库时：全新的库直接建当前结构；旧版本的库按顺序执行迁移，逐版本升级，整个升级在一个事务里，失败则回滚；比程序支持的更新的版本报错。每个迁移是 `migrations` 列表里的一个函数，第 n 项把版本 n 升到 n+1。改表结构时要同时升版本、追加迁移并更新全新建库用的当前结构，两条路径得到的结构必须一致。

## gui：一个客户端

GUI 实现 `protocol.Subscriber`，把更新交给主线程。界面里只有两类内容：

- **会话的视图**：对话的显示项、配置项、可用命令、占用、标题、是否在运行。它们只由更新与响应写入，GUI 不自行决定或推算。
- **界面自身的状态**：输入草稿与待发送的图片、滚动位置、历史翻到哪一页、大纲、面板、等待中的请求、界面自身的错误。

界面状态只在主线程读写。后台请求启动前先取出所需的值，结果经窗口的更新入口回到主线程；请求返回时若界面已经切换到别的会话，就丢弃结果，已建出的会话一并关闭。

具体做法：

- 新建会话：`NewSession`。更换工作目录：用新目录再调用一次 `NewSession`，并丢弃尚未发送过消息的旧会话。
- 打开已有会话：`OpenSession`，同时用 `ListHistory` 读取最近一页；向上滚动时继续读取更早的页。
- 发送：调用 `SendPrompt`（很快返回）；界面不预先绘制自己发出的消息，而是等待 `user_message_chunk`。能否发送取决于视图中的“是否在运行”，它只来自 `run_state`。
- 更换模型与思考深度：`SetConfigOption`，用响应中的配置项重绘选择器。
- 侧栏：`ListSessions`，在自身操作之后以及收到 `session_info_update` 时重新读取。
- 关闭会话标签：`CloseSession`。退出：核心的 `Close`（取消所有运行，等待收尾，关闭数据库）。
- 终端、更改、文件是 GUI 本地的面板，不经 `protocol`。面板按打开的先后排布：一个占满右侧栏，两个上下排，三个时前两个上下排、第三个在它们右侧。更改面板只读，调用本机 `git`（`status`、`diff`、`show`）对比 HEAD 与工作区。
- 搜索：侧栏的“搜索”入口打开启动器，`⌘K` 也能打开。启动器在一个居中的面板里同时搜索会话标题、操作与当前工作目录下的文件；会话取自 `ListSessions`（打开时翻页取全），不新增协议方法。不搜索消息内容。
- 设置页：界面偏好存在 `~/.atlas/gui.json`，只有 `gui` 读写，核心与 `config` 都不碰；文件缺失、损坏或值不认识时用默认值，不阻止启动。目前的偏好是主题（浅色、深色、跟随系统）、界面字体、代码字体与“在菜单栏显示 Atlas”（默认显示，`gui.json` 里记的是隐藏，`hide_tray`），选中即生效并保存，没有保存按钮。代码字体对新打开的终端生效，已打开的终端不变。“系统”一类放“开机时启动”与“在菜单栏显示 Atlas”；前者不是偏好，开关读写系统的登录项（`App.OpenAtLogin`、`SetOpenAtLogin`），每次打开设置页时重新读取，设置失败（如未打包的开发版）时如实显示原因。“全局指令”一类是一个文本框，直接读写 `~/.atlas/AGENTS.md`（不是偏好，文件不存在时为空，改动时才创建），改动即保存，没有保存按钮；`AGENTS.md` 是符号链接时写它指向的文件，打开设置页之后文件在磁盘上被改过时不覆盖并提示。修改对之后新建或恢复的会话生效，已加载的会话不变。
- 定时任务：侧栏的“定时任务”把主区换成定时任务视图，到点的触发由 `gui` 驱动，见“schedule”一节。
- 托盘与后台运行：“在菜单栏显示 Atlas”打开时（默认）有一个托盘图标，菜单是“打开 Atlas”“设置”“退出”，在设置里切换即生效。关闭窗口（红绿灯、`⌘W`）只隐藏窗口，运行与定时任务照常，有未保存修改的文件也不询问。点 Dock 图标或托盘的“打开 Atlas”显示并聚焦窗口，“设置”显示窗口并打开设置页。退出（`⌘Q`、Dock 菜单、托盘的“退出”）不变：有未保存修改的文件先确认（窗口隐藏时先显示），再由核心的 `Close` 取消运行；系统的关机与注销也走这条路，不受影响。由登录项启动时窗口先隐藏。关窗口只隐藏窗口与托盘无关，Dock 图标始终是回到窗口的入口。托盘建不起来时记一条日志，开关保持不变。

## schedule：定时任务

定时任务是一条预设：名称、用户消息、工作目录、模型与思考深度（可以留空，用新会话的默认选择）、规则、是否启用。每次触发新建一个会话，在其中发出这条用户消息；之后与手动发送没有区别：没有权限审核，一轮同样最多 200 次请求。

- **存储**：`~/.atlas/schedule.json`，只有 `schedule` 读写，核心与 `config` 都不碰。先写同目录的临时文件再改名；文件缺失时没有任务；损坏时把原文件改名为 `schedule.json.bad`，再当作没有任务，不阻止启动。
- **规则**：每隔 N 分钟、每天某时、每周某一天某时、仅一次。时间一律用本机时区，每隔 N 分钟以任务的创建时刻为起点。不支持 cron 表达式。保存编辑或重新启用时，创建时刻记为现在，规则从这一刻起算，之前的触发点（包括停用期间的）都不算错过。
- **触发判定**：纯逻辑，不读时钟。给定当前时刻，算出任务最近一次该触发的时刻 `Latest` 与下一次 `Next`。`Latest` 晚于已处理的触发（`last_run`）与任务的创建时刻，且距现在不超过 5 分钟时触发；超过的算错过，不补跑。5 分钟同时容忍休眠唤醒、应用刚启动与定时器的误差，因此应用没有运行期间的触发点都不会补跑。触发后把 `last_run` 记为这次触发点的时刻并保存，同一个触发点不会触发两次。一次性任务触发后自动停用。
- **重叠**：同一任务上一次触发的会话仍在运行时，这次触发跳过，同样记入 `last_run`，不排队。
- **执行**：只有 App 进程调度，由 `gui` 驱动，每隔不到一分钟检查一次。到点时 `gui` 用 `NewSession{cwd, type: scheduled, ownerId: 任务的标识}` 新建任务会话，按需用 `SetConfigOption` 换模型与思考深度，再 `SendPrompt`，不切换当前会话。会话登记进 `gui` 的会话表，所以能收到更新、得知运行结束。第一条消息保存后（收到 `session_info_update`），用 `RenameSession` 把标题改成触发的日期时间，任务名在历史里单独显示。建会话失败（工作目录失效、没有可用模型等）时这次触发不留会话，原因显示在任务上，不保存。
- **任务会话**：只在定时任务视图的“历史”里看到，用 `ListSessions{type: scheduled}` 分页读取（带 `ownerId` 时只读某个任务的）；侧栏与启动器用缺省的 `interactive`，看不到它们。删除任务时一并删除它的任务会话。
- **视图**：侧栏的“定时任务”把主区换成定时任务视图。内容放在居中的阅读栏里：顶部是页面标题，其下左边是“任务”“历史”两个标签，右边是“新建任务”。“任务”是任务的卡片：名称、用户消息的开头、状态（已启用或已停用、规则、工作目录），错过或失败时再加一行提示，不显示下次触发；点卡片进入任务面板，卡片行尾的菜单有立即运行、编辑、启用或停用、删除。“历史”是全部任务会话，新的在前，行的样式与侧栏的会话行一致：任务名与触发的日期时间，行尾是相对时间、运行中的指示器或状态点；默认列最近几个，其余在“显示更多”之后。点一行在主区打开对话。任务面板是任务自己的一页：上面是任务名、状态与下次触发，右上角是立即运行、编辑与菜单（启用或停用、删除）；其下左边是这个任务的历史记录，右边是指令、工作目录、触发方式、模型与思考深度。任务面板与从视图里打开的对话，标题栏左边都是面包屑，如“定时任务 / 任务名 / 日期时间”，点前面的几项回到对应的页面。新建与编辑弹出对话框，里面是表单；视图里不用启用开关，停用的任务名称变淡。有任务会话运行结束而没有看过或出错时，侧栏“定时任务”入口的行尾显示状态点。立即运行不受规则限制，也不改 `last_run`。暂时没有搜索、筛选与排序。控件用本应用安静的风格，与设置页一致。

## 一轮的时序

```
GUI 主线程             core                                    agent.Run          store
SendPrompt ──────────► 受理：占用会话，检查，取 Input 快照
◄── 已受理 ◄──────────┘
                       推送 run_state{running}
◄── apply ◄──────────┘
                       agent.Run(ctx, in, user, emit) ─────► 请求模型，执行工具
                       MessageEnd(user)：
                         第一次保存 ───────────────────────────────────────────► 创建会话
                         追加记录（带期望序号）─────────────────────────────────► INSERT
                         Fold 进上下文
◄── apply ◄──────────  推送 user_message_chunk
◄── apply ◄──────────  Delta → agent_message_chunk，ToolStart → tool_call_update …
                       MessageEnd(assistant / tool result)：写库，Fold，推送
                       结束：释放占用
◄── apply ◄──────────  推送 run_state{idle, stopReason}
```

设置、数据、状态各只有一条流转路径：

- **设置**：`settings.json` 只在加载、更换模型，以及 `SendPrompt` 时发现没有可用模型的情况下读取，结果只进入会话的模型，并经配置项告知客户端。
- **数据**：历史只在消息定稿时写库；上下文只由 `Fold` 推进；客户端的对话只由更新推进。
- **状态**：只有会话是否忙，经 `run_state` 告知客户端。

## 需要自动测试保证的行为

只测并发与持久化格式这类容易悄悄出错的行为，不为小功能写单元测试：

- 持久化：消息的 JSON 无损往返；`store` 的追加、冲突、上下文起点、blob；图片与压缩在重新打开会话之后仍然正确。
- `Fold`：压缩、系统提示词快照、通知、普通消息得到的上下文。
- 一条渲染路径：同一段对话，由实时更新建出的显示项，与由 `ListHistory` 回放建出的一致。
- 并发：写入冲突让会话失效；一轮与连接解耦，关闭不中止一轮，`CancelTurn` 以 `cancelled` 结束；多订阅者都收到更新，未订阅的连接得到 `ErrNotFound`，运行中再次 `SendPrompt` 得到 `ErrBusy`；同一轮里连续的只读工具调用并发执行。
- 单轮请求数上限：达到时追加通知并停止，停止原因是 `max_turn_requests`，通知能经 `ListHistory` 回放；没有触发时不出通知。
- 定时任务：触发判定（`Latest`、`Next` 在跨日、跨周、跨月与夏令时的取值；容忍内触发，超出算错过；同一个触发点不重复；一次性任务触发后停用）与 `schedule.json` 的往返、损坏时改名。

## 已知缺口

- 新建与删除会话不会通知其他连接，其他连接的侧栏要到自己重新读取时才会更新；删除时仍订阅该会话的连接也不会被告知。
- 不同进程之间没有实时通道，只能在写入时靠序号发现冲突。
- 任何订阅者都能取消别人发起的一轮。
- 一轮只限制了模型请求的次数，没有时长、费用的上限；其他形式的失控只能靠取消。
- 历史中的图片已外置，但 `ListHistory` 返回的更新仍内联图片数据；历史很长时分页读取的成本有待观察。
- 定时任务只在 App 进程运行时触发（关窗口不退出，进程还在就仍在运行），错过的不补跑；`acp` 子进程不调度。同时运行两个 App 进程会各触发一次，并且各自把自己内存里的任务清单整个写回 `schedule.json`，后保存的一方会覆盖先保存的一方的修改，不报错；`gui.json` 同理。不要同时运行两个 App。
- 间隔很短的任务每次触发都留下一个任务会话，历史默认只列最近几个，其余留在库里，除非删除任务，没有别的清理办法。

## 后续

- **`acp`、`ws` 适配包**：把 `protocol` 的类型按各自的线上格式编解码。适配包去适配对方的协议，协议中没有的部分（能力协商等）由适配包补足，不反过来修改 `protocol`；例如 ACP 的 `session/load` 由适配包用 `OpenSession` 加 `ListHistory` 拼出，ACP 的 `session/close` 由适配包先 `CancelTurn` 再 `CloseSession`。是否采用现成的 SDK 待定。
- **定时任务的后续**：运行完成时发系统通知；每个任务的运行记录。
- **远程入口的约束**：Atlas 没有权限审核，接入远程入口等同于获得本机用户的 shell。因此远程入口必须认证，默认只监听本机回环地址，并在设计中明确传输加密的方式，之后才能对外开放。
