# Atlas

Atlas 是用 Go 编写的桌面 AI Agent。Agent 拥有完整的文件与命令权限，不提供权限确认。

## 开发

使用 [mise](https://mise.jdx.dev) 管理工具链与任务：

```sh
mise run dev       # 启动桌面应用
mise run ci        # 格式检查、vet、测试、编译、漏洞扫描
mise run icon      # 重新生成应用图标，改了 cmd/icon 之后用
```

本地 Go 版本由 `mise.toml` 固定，CI 和发布流程从 `go.mod` 读取 Go 版本。MyGo 的库和命令行工具共用 `go.mod` 中固定的稳定版。

## 安装

从 [Releases](https://github.com/gvenusleo/Atlas/releases) 下载对应平台的安装包，或用一行命令安装：

```sh
curl -fsSL https://github.com/gvenusleo/Atlas/releases/latest/download/install.sh | sh
```

```powershell
irm https://github.com/gvenusleo/Atlas/releases/latest/download/install.ps1 | iex
```

macOS 的应用没有经过苹果公证，用浏览器下载的 `.dmg` 首次打开会被拦下。到“系统设置 → 隐私与安全性”点“仍要打开”，或执行 `xattr -dr com.apple.quarantine /Applications/Atlas.app`。一行命令安装不受影响。Windows 的安装包没有签名，SmartScreen 会警告，点“更多信息 → 仍要运行”。

支持内置更新的安装包默认自动检查更新，也可以在设置页的“系统”里手动检查。Linux 的 `.deb` 由包管理器更新。

## 配置

配置与数据位于 `~/.atlas/`：

- `settings.json`：当前模型与 provider 清单，不含凭据；可在设置页的“模型接入”里编辑。
- `auth.json`：以 provider 名为键的 API key。API key 只从这个文件读取，
  不读环境变量；设置页的“模型接入”里填的 key 也写在这里。
- `AGENTS.md`：全局指令，可在设置页的“全局指令”里编辑；目录里的 `AGENTS.md` 作用于该目录及其子目录。
- `gui.json`：界面偏好（主题、配色、界面字体、代码字体、是否在菜单栏显示），由设置页写入，只有桌面端读取，缺失时用默认值。
- `schedule.json`：定时任务（名称、用户消息、工作目录、规则等），由定时任务视图写入，只有桌面端读写。
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
          "reasoning_options": [{ "type": "effort", "values": ["low", "high"] }]
        }
      }
    }
  }
}
```

`model` 是新会话用的默认模型。模型须在 `providers.*.models` 里声明。打开已有会话时，优先使用上次保存的模型，其次用默认模型，再其次用清单里第一个可用的模型。打开时选定的模型会保存到会话。在输入框下方更换模型或思考深度后，新的选择会保存到会话，从下一次模型请求开始生效。

模型的 `context_window`（上下文窗口，token）可选，建议填写。上下文接近窗口上限时，Atlas 会自动把较早的对话压缩成摘要，界面里的上下文占用圆环也依赖这个值。省略时，自动压缩只在模型报告上下文超限后触发。会话空闲时可单独发送 `/compact`，手动压缩会优先保留最近的完整轮次和附件原文，原始历史仍可回看。

模型的 `max_output`（单次回复的输出上限，token）可选：填写后每次请求都会带上它，没有填写时由服务端决定。

模型的思考深度由 `reasoning_options` 声明：`{"type": "effort", "values": ["low", "high"]}` 是可调档位，第一个是默认档；`{"type": "toggle"}` 表示只能开或关，默认开。目前只认这两种写法，只写 `budget_tokens` 时不出现思考深度；完全不写时请求里也不带相关参数，始终思考而不可配置的模型就该这样写。旧的 `reasoning` 字段已废弃，以 `reasoning_options` 为准。

在输入框开头或空白后输入 `/`，可以补全工作目录下发现的技能（来源包括工作目录与主目录下的 `.agents/skills` 和 `.atlas/skills`）。例如发送 `先用 /think 梳理一下`，Atlas 会把 `think` 的 SKILL.md 全文连同你的输入一起交给模型。一条消息可以调用多个技能，每个 `/技能名` 两侧须是空白或消息边界。

在输入框里输入 `@` 可以引用工作目录下的文件或目录：列表列出路径里包含你输入内容（忽略
大小写）的项，选中后以 `@路径` 写进消息，模型需要内容时自己去读。

### 图片

在设置的「模型接入」中为模型打开「接受图片」，也可以在 `settings.json` 中声明：

```json
"model-id": { "modalities": { "input": ["text", "image"] } }
```

没有声明的模型，输入框左侧的按钮是灰的。有三种方式添加图片，一条消息最多 6 张：粘贴
（截图，或在访达里复制的图片文件）、把图片文件拖到输入框上、点输入框左侧的按钮选文件。
支持 PNG、JPEG、WebP 和 GIF（只取第一帧）。输入框附件的原始文件上限为 20 MiB，`read` 工具的单文件上限为 10 MiB。每次请求都会把历史里的图片重传一遍，所以长边超过 2048 像素或超过 1 MiB 的图会先缩小再保存。

附了图片的会话换到不声明图片输入的模型后，历史里的图片在请求里换成一句说明，
会话仍可继续。

### 子进程的环境变量

从 Dock 或 Finder 启动的应用只有系统给的最小环境，找不到 brew、mise、nvm 装的命令。Atlas 启动时
在后台用你的登录 shell 取一份环境（`$SHELL -l -i -c`，要带 `-i`，因为这些工具多半在 `.zshrc` 里
激活），叠在当前环境上，给 shell 工具用；5 秒内取不到就用当前环境。所以 shell 配置
文件里不要等待输入，也不要太慢。终端面板自己以登录 shell 启动，会加载自己的环境，不受影响。

### 工具

模型可以调用 read、write、edit、shell 四个工具。外部能力通过技能（`.agents/skills`）说明，模型再通过 shell 调用相应的命令行工具。Atlas 没有 MCP 接口。连续的只读调用（read）可以并发执行，write、edit 和 shell 按模型发出的顺序逐个执行。

shell 命令默认最多运行 30 分钟，也可指定更短的超时。输出截断时，完整日志保留 7 天。

## 结构

`cmd/atlas` 打开数据库、创建核心并启动桌面界面。包职责和依赖关系见[设计文档的分层一节](docs/design.md#分层)。
