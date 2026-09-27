# 文档指南

[English](../README.md)

Atlas 文档按用途组织：

- 根 `README.md`：产品状态与当前可用入口。
- `architecture.md`：系统边界与依赖方向。
- `configuration.md`：`~/.atlas/config.yaml` schema 与校验规则。
- `development.md`：workspace 结构、命令和工程规范。
- `tools.md`：内置工具目录、限额与安全边界。
- [MCP 工具](mcp.md)：stdio/HTTP 配置、行为与支持范围。
- [MCP 客户端开发计划](plans/mcp-support.md)：SDK 接入决策、开发阶段与后续范围。
- [手机远程控制计划](plans/remote-mobile-control.md)：ACP over WebSocket、认证、断线语义与交付阶段。
- 各 package 的 `README.md`：局部职责与依赖约束。

每个 Markdown 段落保持单行：不要对正文、列表项或引用段落做硬换行；一段连续文字就写在一行里。

英文文档定义结构与术语；存在 `zh-CN` 对应文档时，必须在同一个变更中同步更新。

统一使用以下状态描述：

- **Available**：当前仓库中已有实现并经过验证。
- **Planned**：边界或行为已经确定，但尚未实现。
- 已移除的行为必须从当前文档删除，Git 历史作为归档。

命令、配置或协议示例引用的实现存在之前，不得把示例写入用户文档。
