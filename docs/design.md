# Atlas 目标架构

这份文档描述重新设计后的 Agent 运行架构：一次运行里，配置从哪里来，数据怎样流转、存在哪里，
状态由谁管理。它不受现有命名约束，与现状的对照见末尾。

核心思路：**Atlas 的核心是一个 ACP 形态的提供端，桌面 GUI、ACP、WebSocket 都只是它的客户端。**
对外接口的语义与命名照 [ACP v1](https://agentclientprotocol.com)，ACP 不够用的地方用规范允许的
`_atlas` 扩展补上。

## 前提

- 入口有多个：桌面 GUI（进程内），ACP（编辑器以子进程启动 `atlas acp`，走 stdio），WebSocket
  （在 App 进程内，供手机等远程客户端）。现在只做 GUI，另外两个以后做，但核心按它们设计。
- 多个进程可以打开同一个数据库。同一会话同一时刻只应由一个进程写入，万一并发写入，靠序号冲突
  检测兜底。
- 同一进程里，多个客户端可以同时打开同一个会话，各自收到全部更新。
- 一个会话同一时刻只有一轮在运行。
- 消息是值，创建之后任何人都不修改它。这是全局约定，不靠深拷贝保证。
- Atlas 没有权限审核，工具在本地执行：ACP 里由客户端提供的能力（权限请求、`fs/*`、`terminal/*`）
  一概不用。

## 三个概念

| 概念 | 是什么 | 唯一权威 |
| --- | --- | --- |
| 历史 | 会话里发生过的全部消息，只追加，给人看 | 数据库 `messages` 表 |
| 上下文 | 下一次请求发给模型的消息 | 由历史按 `agent.Fold` 折叠得到 |
| 会话 | 一个已加载的会话：配置、上下文、是否在运行、谁在看 | 核心里的 `session` 对象 |

上下文是历史的投影。两者只在两处不同：压缩之后，上下文里较早的消息换成了摘要，历史里都还在；
提示词快照记在历史里，不进上下文。“上下文 = 依次把历史里的每条消息 `Fold` 进来”，这条规则只写
一份，在三处用：循环在一轮里推进自己的工作副本；会话在每条消息保存后推进自己的上下文；加载会话
时从库里重放历史。

客户端看到的对话是历史的视图，只由核心发来的更新推进，它不是数据。

## 分层

```
cmd/atlas ── 组装：打开库，建核心，交给入口
   │
   ├─► gui ───┐
   ├─► acp ───┼─► protocol（接口与数据类型，不依赖任何包）
   └─► ws ────┘        ▲
                       │ 实现
                     core ──► agent（不依赖任何包）
                       ├────► llm ───► agent
                       ├────► tools ─► agent
                       ├────► store ─► agent
                       ├────► config
                       └────► prompt ► skills
```

| 包 | 职责 |
| --- | --- |
| `protocol` | 对客户端开放的接口（`Agent`、`Client`）与全部请求、响应、更新类型，JSON 标签用 ACP 的线上名称。只有数据，没有逻辑 |
| `core` | ACP 的提供端：管理已加载的会话，实现 `protocol.Agent`。会话运行时数据的唯一权威 |
| `agent` | 消息、模型与工具的接口，一轮运行的循环、压缩、上下文折叠。没有状态，不做 IO |
| `llm` | `agent.StreamFn` 的实现（OpenAI Responses） |
| `tools` | read、write、edit、shell |
| `store` | SQLite：会话表与消息表。只存取，不懂折叠与运行 |
| `config` | 读 `settings.json`、`auth.json`，模型清单，选模型的回退规则 |
| `prompt`、`skills` | 按工作目录组装系统提示词，发现技能 |
| `gui` | 桌面客户端：只依赖 `protocol`，不 import `core`、`agent`、`store` |
| `acp`、`ws` | 以后做：把 `protocol.Agent` 与 `protocol.Client` 映射成 JSON-RPC，分别走 stdio 与 WebSocket |

`gui` 只依赖 `protocol`，由编译器保证界面拿不到核心的内部状态：它想知道什么，都只能通过接口问，
或者等核心推过来。

## protocol：对外接口

```go
// Agent 是 Atlas 对一个客户端连接开放的接口。语义与命名照 ACP v1。
type Agent interface {
	NewSession(ctx context.Context, req NewSessionRequest) (NewSessionResponse, error)
	ResumeSession(ctx context.Context, req ResumeSessionRequest) (ResumeSessionResponse, error)
	LoadSession(ctx context.Context, req LoadSessionRequest) (LoadSessionResponse, error)
	ListSessions(ctx context.Context, req ListSessionsRequest) (ListSessionsResponse, error)
	DeleteSession(ctx context.Context, req DeleteSessionRequest) error
	CloseSession(ctx context.Context, req CloseSessionRequest) error
	Prompt(ctx context.Context, req PromptRequest) (PromptResponse, error)
	Cancel(req CancelNotification)
	SetConfigOption(ctx context.Context, req SetConfigOptionRequest) (SetConfigOptionResponse, error)

	// Atlas 扩展，线上方法名为 _atlas/session/history 等。
	History(ctx context.Context, req HistoryRequest) (HistoryResponse, error)
	Prompts(ctx context.Context, req PromptsRequest) (PromptsResponse, error)
	Rename(ctx context.Context, req RenameRequest) error
}

// Client 是核心推送更新的对象，每个连接一个。在核心的 goroutine 里调用，必须很快返回，
// 不得在里面同步调用 Agent。
type Client interface {
	SessionUpdate(n SessionNotification)
}
```

核心只有一个，每个连接拿到一个 `Agent`：`conn := core.Connect(client)`。GUI 进程内直接调用；
`acp`、`ws` 收到 JSON-RPC 请求就调用 `conn` 对应的方法，把 `client` 收到的更新序列化发出去。

### 方法

| 方法 | ACP | 做什么 |
| --- | --- | --- |
| `NewSession{cwd}` | `session/new` | 建会话，返回 `sessionId` 与配置项。会话先只在内存里，第一条消息保存时才写库；工作目录从此不变 |
| `ResumeSession{sessionId}` | `session/resume` | 加载会话并订阅，不回放历史。GUI 用它，历史另外分页读 |
| `LoadSession{sessionId}` | `session/load` | 加载会话并订阅，先把全部历史以更新回放，再返回。给 ACP 客户端用 |
| `ListSessions{cwd?, cursor?}` | `session/list` | 列出已保存的会话：`sessionId`、`cwd`、`title`、`updatedAt` |
| `DeleteSession{sessionId}` | `session/delete` | 删除空闲会话 |
| `CloseSession{sessionId}` | `session/close` | 这个连接不再看这个会话。没有连接在看、也不在运行时，核心卸载它 |
| `Prompt{sessionId, prompt}` | `session/prompt` | 跑一轮，这一轮结束时才返回 `stopReason`：`end_turn`、`max_tokens`、`cancelled`。模型调用失败返回错误 |
| `Cancel{sessionId}` | `session/cancel` | 取消正在运行的一轮，被取消的 `Prompt` 以 `cancelled` 返回 |
| `SetConfigOption{sessionId, configId, value}` | `session/set_config_option` | 换模型或思考深度，保存后返回完整的配置项 |
| `History{sessionId, before, limit}` | `_atlas/session/history` | 一页历史，以更新的形式返回，供界面往前翻 |
| `Prompts{sessionId, before}` | `_atlas/session/prompts` | 全部用户消息的摘要，供会话目录 |
| `Rename{sessionId, title}` | `_atlas/session/rename` | 改名，打开着它的连接收到 `session_info_update` |

斜杠命令按 ACP 的方式处理，就是一条以 `/` 开头的 `Prompt`：`/compact` 压缩上下文；`/技能名 参数`
在原文后附上技能全文再运行。可用的命令（`compact` 与全部技能）通过 `available_commands_update` 推给客户端。

没有“换工作目录”：ACP 的工作目录在建会话时就定了。新会话想换目录，就建一个新会话。

非空闲会话上的 `Prompt`、`SetConfigOption`、`DeleteSession` 返回 `ErrBusy`。

### 配置项

模型与思考深度都是 ACP 的配置项，客户端照着画选择器，不需要自己的模型清单或档位规则：

- `model`（`category: "model"`）：选项是全部可用的模型，值是 `provider/model-id`。每个选项在
  `_meta.atlas` 里带 `images`，表示接受图片，界面据此决定能不能附图。
- `thought_level`（`category: "thought_level"`）：选项是当前模型支持的档位。模型不支持思考时没有这一项。

换模型后，思考深度按“原档位新模型支持就保留，否则用新模型的默认档”调整，响应里是完整的新配置项，
其他连接收到 `config_option_update`。

### 更新

核心推给客户端的全部更新（`session/update` 的 `sessionUpdate` 值）：

| 更新 | 何时 |
| --- | --- |
| `user_message_chunk` | 一条用户消息保存后。发消息的客户端也只在收到它之后才显示自己的消息 |
| `agent_message_chunk`、`agent_thought_chunk` | 流式的文字与思考，同一条消息用同一个 `messageId` |
| `tool_call` | 模型开始发出一个工具调用：`toolCallId`、`name`、`title`、`kind`、`rawInput`，状态 `pending` |
| `tool_call_update` | 参数继续流出（`rawInput`），开始执行（`in_progress`），结束（`completed` 或 `failed`，带 `content` 与 `rawOutput`） |
| `usage_update` | 每条助手消息保存后：`used` 是上下文占用，`size` 是窗口 |
| `compaction_update`、`compaction_summary_chunk` | 压缩开始、完成（带摘要）、失败、取消 |
| `notice` | 系统通知，如压缩失败、达到上限 |
| `session_info_update` | 会话第一次保存（有了名称）与改名 |
| `config_option_update` | 别的连接换了模型或思考深度 |
| `available_commands_update` | 加载会话时 |
| `_atlas_state` | 一轮开始与结束：`running`、`since`。仿照 ACP v2 草案的 `state_update`，中途接入的客户端靠它知道会话在不在运行 |
| `_atlas_retry` | 一次模型请求失败、等待重试：`messageId`、第几次、等多久、原因。客户端丢掉这条消息已流出的内容 |
| `_atlas_reset` | 别的进程写过这个会话，核心已从库里重新加载。客户端丢掉显示的对话，重新读历史 |

工具 `kind`：read 是 `read`，write 与 edit 是 `edit`，shell 是 `execute`。`rawOutput` 是工具附带的
结构化数据（现在的 Details），它的形状由 `protocol` 定义，GUI 用它画 diff、行号与图片；其他客户端只看
`title`、`content` 也能用。

所有更新由一个转换函数从 `agent` 的事件与消息生成：实时运行时由事件生成，`LoadSession` 与 `History`
回放时由库里的消息生成。客户端只有一条渲染路径。

## core：会话的唯一权威

```go
type Core struct {
	db       *store.DB
	mu       sync.Mutex
	sessions map[string]*session // 本进程已加载的会话
}

// session 是一个已加载的会话。字段都由 mu 保护，锁只在读写字段时持有，从不跨 IO。
type session struct {
	id string

	mu       sync.Mutex
	cwd      string
	saved    bool              // 已写进库；之前只在内存里
	seq      int64             // 本进程写入或读到的最后序号
	turn     *turn             // 正在运行的一轮；nil 表示空闲。这是唯一的控制状态
	model    agent.Model       // 没有可用的模型时 Stream 为空
	thinking agent.ThinkingLevel
	prompt   string            // 系统提示词
	tools    []agent.Tool
	skills   []skills.Skill
	messages []agent.Message   // 上下文 = Fold(历史)
	clients  []protocol.Client // 打开着这个会话的连接
}

type turn struct {
	cancel context.CancelFunc
	done   chan struct{}
	since  time.Time
}
```

现在 host 的 `Busy`、`Running`、`NeedsReload`、`Draft`、`Deleted`、`Closed`，与 GUI 的 `running`、
`opening`、`switching`，都由 `turn` 是否为空加上 `saved` 取代。

### 加载

`NewSession`、`ResumeSession`、`LoadSession` 同步完成加载再返回：

1. 已有会话：`db.Get` 与 `db.Log`，依次 `Fold` 出上下文，记下最后一份提示词快照与最后序号。
   本进程已经加载过就直接订阅，不重新加载。
2. 读 `settings.json`，按“上次的模型 → 默认模型 → 清单里第一个可用的”选模型。已有会话换了模型就
   `db.SetModel`。一个可用的都没有、或缺凭据时不算失败：`model.Stream` 为空，`Prompt` 时再试一次并报原因。
3. 按工作目录组装提示词、技能与工具。已有会话的提示词与最后一份快照不同时，追加一份新快照。

工作目录不存在、历史损坏时加载失败，返回错误。历史仍然可以用 `History` 读。

### 一轮

`Prompt` 的过程：

1. 加锁：`turn` 不为空返回 `ErrBusy`；否则建 `turn`，取一份 `agent.Input` 快照，解锁，广播 `_atlas_state`。
2. 已保存的会话读一次库里的最后序号。比 `seq` 新，说明别的进程写过：从库里重新加载上下文，广播 `_atlas_reset`，再继续。
3. 运行 `agent.Run`。事件到来时：
   - `MessageEnd(m)`：先写库，再改内存，最后广播。
     - 还没 `saved`：`db.Create`，名称取这条用户消息的开头，同时写入提示词快照。
     - `db.Append(id, seq, m)`：序号对不上返回冲突，任何错误都停止这一轮，之后的事件全部丢弃。
     - 加锁：`messages = Fold(messages, m)`，推进 `seq`，解锁。
     - 广播由它生成的更新。
   - 其他事件：直接广播由它生成的更新，不写库。
4. 加锁：清掉 `turn`，解锁，广播 `_atlas_state`，返回 `stopReason` 或错误。

所以内存里的上下文永远等于库里的历史折叠出来的结果。写库失败只停止这一轮，会话本身仍然可用：
下一轮开始时第 2 步会和库对齐。

### 多客户端与多进程

- **同一进程**：一个会话只加载一次，所有打开它的连接都在 `clients` 里，更新广播给每一个。中途接入的
  客户端先拿历史（`LoadSession` 回放或 `History` 分页），再靠 `_atlas_state` 知道是否在运行。正在流出的
  那条消息，接入之前已经发出的部分它看不到，等这条消息结束后它会在历史里出现。
- **不同进程**：各自加载各自的 `session`，互相看不到实时更新。写入时靠序号发现冲突；下一轮开始时与库对齐，
  并通知自己的客户端重新读历史。

## agent：一轮运行

`agent` 是纯逻辑：输入一份快照，运行，通过事件交出结果。它不持有会话，不修改调用方的数据。

```go
// Model 是会话用的模型。引用、窗口和流函数是一个值，总是一起换。
type Model struct {
	Ref    string   // provider/model-id，唯一指出一个模型
	Window int64    // 上下文窗口（token），未知为 0
	Stream StreamFn // 已绑定客户端与模型 ID
}

// Input 是一轮看到的全部，由调用方给出快照，循环不修改它。
type Input struct {
	Model    Model
	Thinking ThinkingLevel
	Prompt   string // 系统提示词
	Tools    []Tool
	Messages []Message // 上下文
}

// Run 把 user 加进上下文，循环请求模型、执行工具，直到模型不再调用工具或 ctx 取消；
// 返回这一轮新产生的消息。
func Run(ctx context.Context, in Input, user UserMessage, emit func(Event)) []Message

// Compact 把除最后一条外的上下文压成摘要，返回新产生的消息。
func Compact(ctx context.Context, in Input, emit func(Event)) []Message

// Fold 把一条新消息并进上下文：压缩记录换成“摘要 + 之前保留的 Kept 条”，
// 提示词快照不进上下文，其余追加。
func Fold(messages []Message, m Message) []Message

func Compactable(messages []Message) bool // 有没有可压缩的内容
func Tokens(messages []Message) int64     // 估算上下文大小
```

消息类型（`UserMessage`、`AssistantMessage`、`ToolResultMessage`、`SystemMessage`）、`Tool`、`StreamFn`
保持现在的形状，它们是面向模型的格式，也是库里存的格式。`AssistantMessage.Model` 由循环按 `Input.Model.Ref`
盖上。重试次数与退避是包内常量，`MaxTurns` 删除。

事件只留核心需要的：`MessageStart`（助手开始回复）、`Delta`、`Retry`、`MessageEnd`（任何一条消息定稿，
每条历史消息恰好对应一个）、`ToolStart`、`CompactStart`。工具的结束就是工具结果的 `MessageEnd`。

## store

```sql
CREATE TABLE sessions (
    id         TEXT PRIMARY KEY,
    name       TEXT NOT NULL,
    cwd        TEXT NOT NULL,
    model      TEXT NOT NULL,  -- 上次选的模型，provider/model-id
    thinking   TEXT NOT NULL,
    last_seq   INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;
CREATE TABLE messages (
    session_id TEXT NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    seq        INTEGER NOT NULL,
    role       TEXT NOT NULL,  -- 只为按角色查询（会话目录）
    body       TEXT NOT NULL,  -- 消息的 JSON
    PRIMARY KEY (session_id, seq)
) STRICT;
```

```go
func Open(path string) (*DB, error)

func (db *DB) Create(info Info, first ...agent.Message) (seq int64, err error)
// Append 追加一条。lastSeq 是调用方已知的最后序号，库里不是它时返回 ErrConflict。
func (db *DB) Append(id string, lastSeq int64, m agent.Message) (seq int64, err error)
func (db *DB) SetModel(id, model string, thinking agent.ThinkingLevel) error
func (db *DB) Rename(id, name string) error
func (db *DB) Delete(id string) error

func (db *DB) Get(id string) (Info, error) // 含 LastSeq
func (db *DB) List(cwd string, before *Cursor, limit int) ([]Info, error)
func (db *DB) Log(id string) ([]agent.Message, int64, error) // 全部历史与最后序号
func (db *DB) Page(id string, before int64, limit int) ([]Record, error)
func (db *DB) Prompts(id string, before int64) ([]Record, error)
```

多个进程可以同时打开（WAL）。冗余列（停止原因、是否出错、用量、每条消息的模型与档位）都删掉，它们都在
JSON 里。不做迁移：版本号不符就报错，由用户删库重建。

## gui：一个客户端

GUI 实现 `protocol.Client`，把更新交给主线程：

```go
func (a *App) SessionUpdate(n protocol.SessionNotification) {
	a.win.Update(func() { a.chats[n.SessionID].apply(n.Update) })
}
```

`chat` 里只有两类东西：

- **会话的视图**：对话的显示项、配置项、可用命令、占用、标题、是否在运行。它们只由更新与响应写入，
  GUI 从不自己决定或推算。
- **界面自己的状态**：输入草稿与待发图片、滚动、历史翻到哪一页、目录、面板、等待中的请求、界面自己的错误。

具体做法：

- 新会话：`NewSession`。换工作目录：用新目录再 `NewSession`，丢掉还没发过消息的那个。
- 打开已有会话：`ResumeSession`，同时用 `History` 读最近一页；往上滚时继续读更早的页。
- 发送：`Prompt` 放到后台 goroutine 里调用，界面不先画自己的消息，等 `user_message_chunk`。
  能不能发送看视图里的“是否在运行”，它只来自 `_atlas_state`。
- 换模型与思考深度：`SetConfigOption`，用响应里的配置项重画选择器。
- 侧栏：`ListSessions`，自己的操作之后与收到 `session_info_update` 时重新读。
- 关闭会话标签：`CloseSession`。退出：核心 `Close`（取消所有运行，等待收尾，关库）。

## 一次运行

```
GUI 主线程             core（Prompt 所在 goroutine）           agent.Run          store
go conn.Prompt ──────► 加锁：建 turn，取 Input 快照，解锁
                       广播 _atlas_state{running}
◄── apply ◄──────────┘
                       与库对齐序号（必要时重载并广播 _atlas_reset）
                       agent.Run(ctx, in, user, emit) ─────► 请求模型，执行工具
                       MessageEnd(user)：
                         第一次保存 ───────────────────────────────────────────► Create
                         Append(seq) ──────────────────────────────────────────► INSERT
                         加锁：messages = Fold(messages, m)，解锁
◄── apply ◄──────────  广播 user_message_chunk
◄── apply ◄──────────  Delta → agent_message_chunk，ToolStart → tool_call_update …
                       MessageEnd(assistant / tool result)：写库，Fold，广播
                       结束：加锁：清掉 turn，解锁
◄── apply ◄──────────  广播 _atlas_state{idle}
◄── stopReason ◄─────  Prompt 返回
```

配置、数据、状态各只走一条路：

- **配置**：`settings.json` 只在加载、换模型、`Prompt` 发现没有可用模型时读，结果只进 `session.model`，
  经配置项告诉客户端。
- **数据**：历史只在 `MessageEnd` 时写库；上下文只由 `Fold` 推进；客户端的对话只由更新推进。
- **状态**：只有 `session.turn`，经 `_atlas_state` 告诉客户端。

## 与现状的对照

| 现在 | 目标 |
| --- | --- |
| `host` 包、`Manager`、`Session` | `core` 实现 `protocol.Agent`；`Manager` 的注册表变成 `Core.sessions` |
| 注册表里的占位实例、`lockMutation`、操作登记 `begin` | 删除：加载是同步的请求，删除与改名只对空闲会话 |
| `agent.LoopContext` | 一轮的快照 `agent.Input`，加上 `core.session` 的字段 |
| `Model`、`Stream`、`ContextWindow` 三个字段 | 一个 `agent.Model` |
| 循环直接改 `rc.Messages` | 循环只改自己的副本，会话在写库后 `Fold` |
| 折叠规则在 `compact.go` 与 `session.Load` 各一份 | `agent.Fold` 一份 |
| host 的 6 个状态标志、GUI 的 `running`、`opening`、`switching` | `session.turn` 与 `saved`；GUI 只有收到的 `_atlas_state` |
| `NeedsReload` 与 GUI 重新 `Resume` | 删除：写库失败只停止这一轮，下一轮开始时与库对齐 |
| `ErrConflict` | 保留：序号冲突检测，多进程的兜底 |
| `agent.Event` 直接给 GUI，GUI 有实时与历史两套转换 | 核心把事件与消息转成 ACP 更新，GUI 只有一条渲染路径 |
| GUI 的模型清单、`selection`、`fit`、`usage`、`cwd` | 配置项、`usage_update`、建会话时的 `cwd` |
| `SetCwd` 与草稿换目录 | 删除：换目录就新建会话 |
| `Compact` 方法 | `/compact` 斜杠命令 |
| `autoName` | 第一次写库时命名 |
| `clone.go` 深拷贝 | 删除：消息创建后不修改；更新是新生成的值 |
| `agent_*`、`turn_*`、`tool_execution_end` 事件 | 删除 |
| `session` 包（SQLite） | `store` 包；冗余列与 `Usage` 查询删除 |
| `WaitIdle`、`Deleted`、`Closed` | `Core.Close`；其余删除 |
| `tools` 的 Details 类型 | 移到 `protocol`，作为 `rawOutput` 的形状 |

## 实施步骤

每一步单独提交并通过 `mise run ci`，完成后应用照常可用：

1. **agent**：`Model`、`Input`、`Fold`，精简事件；循环不再修改输入。host 适配。
2. **store**：新表结构与 API，`Log` 返回原始历史，折叠交给 `agent.Fold`。host 适配，删旧库重建。
3. **protocol 与 core**：定义接口与类型，`core` 实现它并带上测试；GUI 暂时仍用 host。
4. **GUI 改接**：GUI 实现 `protocol.Client`，只依赖 `protocol`；删除 host。同时更新 README 的结构表。

ACP 与 WebSocket 入口不在这次范围内，第 4 步之后各是一个独立的适配包。

## 测试

只测必要的行为：

- `Fold`：压缩记录、提示词快照、普通消息；重放历史与运行中推进得到同样的上下文。
- 一条渲染路径：同一段对话，GUI 由实时更新建出的显示项与由 `History` 回放建出的一致。
- 持久化：新会话第一条消息写库并命名；写库失败时这一轮停止，内存上下文与库一致，会话还能继续。
- 多进程：两个 `Core` 打开同一个库文件，交替在同一会话上运行，后来者先对齐再继续；并发写入时一方得到冲突。
- 多客户端：两个连接打开同一个会话，都收到全部更新；运行中第二个 `Prompt` 返回 `ErrBusy`。
- 并发：运行中反复读、`Cancel`，在 `-race` 下通过。
- 选模型：上次的模型被删、默认模型被删、一个可用的都没有。

## 待验证

- ACP 的 `notice`、`compaction_update` 还是 RFD，可能变动；适配时以当时的规范为准，核心里的转换函数是唯一要改的地方。
- ACP 的选项对象是否允许 `_meta`（用来带 `images`）；不允许就改用一个 `_atlas` 配置项。
- `_atlas_state` 等扩展更新的取名，按 ACP 对 `_` 开头枚举值的规则核对。
