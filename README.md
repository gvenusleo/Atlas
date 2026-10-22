# Atlas

Atlas 是用 Go 编写的桌面 AI Agent，当前处于重写阶段。Agent 拥有完整的文件与命令权限，
不提供权限确认。

## 开发

使用 [mise](https://mise.jdx.dev) 管理工具链与任务：

```sh
mise run dev       # 启动桌面应用
mise run ci        # 格式检查、vet、测试、编译、漏洞扫描
mise run icon      # 重新生成应用图标，改了 cmd/icon 之后用
```

MyGo 不等发版，`go.mod` 固定在它 main 分支的某个提交（伪版本），本地和 CI 用的是同一份。
要用 MyGo 的最新提交：

```sh
go get github.com/egoist/mygo@main && go mod tidy
```

## 配置

配置与数据位于 `~/.atlas/`：

- `settings.json`：当前模型与 provider 清单，不含凭据。
- `auth.json`：以 provider 名为键的 API key，权限必须为 `0600`。API key 只从这个文件读取，
  不读环境变量。
- `AGENTS.md`：全局指令。
- `atlas.db`：会话数据库。

最小的 `settings.json`：

```json
{
  "model": "provider/model-id",
  "providers": {
    "provider": {
      "api": "openai-responses",
      "base_url": "https://api.example.com/v1",
      "models": {
        "model-id": {
          "context_window": 200000,
          "reasoning": true,
          "reasoning_options": [{ "type": "effort", "values": ["low", "high"] }]
        }
      }
    }
  }
}
```

`model` 是新会话用的默认模型。模型必须在 `providers.*.models` 里声明，没有声明的视为不存在。
打开已有会话时，上次用的模型还在、且受支持就接着用；否则退到默认模型，默认模型也不可用就用清单里
第一个可用的，退到的模型会存进会话。在输入框下方换模型或思考深度，当场就应用并保存到会话。

模型的 `context_window`（上下文窗口，token）可选，但建议填写：上下文接近窗口上限时，
Atlas 会自动把较早的对话压缩成摘要，界面里的上下文占用圆环也依赖它。没有填写时只在
请求被拒绝为上下文超限之后才压缩。在输入框里输入 `/` 会列出可用的命令，其中 `/compact`
可以随时手动压缩：除最后一条消息外的对话都会换成摘要，与有没有填写 `context_window` 无关。

列表里还有工作目录下发现的技能（`.agents/skills`、`.atlas/skills`，以及主目录下的同名目录）：
输入 `/技能名 参数` 显式调用它，Atlas 会把技能的 SKILL.md 全文连同你的输入一起交给模型，
不必等模型自己去读。

在输入框里输入 `@` 可以引用工作目录下的文件或目录：列表列出路径里包含你输入内容（忽略
大小写）的项，选中后以 `@路径` 写进消息，模型需要内容时自己去读。

### 图片

在模型的配置里声明它接受图片，输入框才能附图片：

```json
"model-id": { "modalities": { "input": ["text", "image"] } }
```

没有声明的模型，输入框左侧的按钮是灰的。有三种方式添加图片，一条消息最多 6 张：粘贴
（截图，或在访达里复制的图片文件）、把图片文件拖到输入框上、点输入框左侧的按钮选文件。
支持 PNG、JPEG、WebP 和 GIF（只取第一帧），原始文件不能超过 20 MB。每次请求都会把
历史里的图片重传一遍，所以长边超过 2048 像素或超过 1 MiB 的图会先缩小再保存；模型用 `read`
读图片文件时也按同样的规则处理。

附了图片的会话换到不声明图片输入的模型后，历史里的图片在请求里换成一句说明，
会话仍可继续。

### 子进程的环境变量

从 Dock 或 Finder 启动的应用只有系统给的最小环境，找不到 brew、mise、nvm 装的命令。Atlas 启动时
在后台用你的登录 shell 取一份环境（`$SHELL -l -i -c`，要带 `-i`，因为这些工具多半在 `.zshrc` 里
激活），叠在当前环境上，给 shell 工具用；5 秒内取不到就用当前环境。所以 shell 配置
文件里不要等待输入，也不要太慢。终端面板自己以登录 shell 启动，会加载自己的环境，不受影响。

### 工具

模型只有四个工具：read、write、edit、shell。Atlas 不接入 MCP：要用的外部能力写成技能
（`.agents/skills`），教模型通过 shell 调用对应的命令行工具。同一轮里模型发出的多个调用，
只读的（read）并发执行，会改动文件或运行命令的（write、edit、shell）按发出的顺序一个个执行。

## 结构

| 包 | 职责 |
| --- | --- |
| `internal/agent` | 消息模型与 agent 循环，不依赖其他层 |
| `internal/llm` | 把 `agent.StreamFn` 落到具体的模型 API |
| `internal/tools` | read、write、edit、shell 工具 |
| `internal/imaging` | 整理要交给模型的图片：限制格式与大小，缩小，按 EXIF 方向转正 |
| `internal/shellenv` | 向登录 shell 取环境，给 shell 工具用 |
| `internal/prompt`、`internal/skills` | 系统提示词、指令与技能发现 |
| `internal/config` | `settings.json` 与凭据读取 |
| `internal/store` | SQLite 会话存储 |
| `internal/host` | 组装以上各层，供 App、ACP 等入口使用 |
