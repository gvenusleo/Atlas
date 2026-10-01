# 文档指南

[English](../README.md)

Atlas 文档按用途组织：

- 根 `README.md`：产品状态与当前可用入口。
- `architecture.md`：系统边界与依赖方向。
- `configuration.md`：`~/.atlas/config.yaml` schema 与校验规则。
- `development.md`：workspace 结构、命令和工程规范。
- `tools.md`：内置工具目录、限额与安全边界。
- `mcp.md`：MCP 服务器配置、工具行为与支持范围。
- `protocol-acp.md`：ACP 接口、Atlas 扩展与 WebSocket transport。
- `data-model.md`：持久化结构与失败处理规则。
- 各 package 的 `README.md`：局部职责与依赖约束。

每个 Markdown 段落保持单行：不要对正文、列表项或引用段落做硬换行；一段连续文字就写在一行里。

英文文档定义结构与术语；存在 `zh-CN` 对应文档时，必须在同一个变更中同步更新。

统一使用以下状态描述：

- **Available**：当前仓库中已有实现并经过验证。
- **Planned**：边界或行为已经确定，但尚未实现。
- 已移除的行为必须从当前文档删除，Git 历史作为归档。

命令、配置或协议示例引用的实现存在之前，不得把示例写入用户文档。
