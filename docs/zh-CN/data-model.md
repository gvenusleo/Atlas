# 数据模型

[English](../data-model.md)

Atlas 将会话持久化为有序的 turn 与 timeline item，每个 timeline item 都属于与其操作相同的会话和 turn。

SQLite 将会话、turn 与 timeline 的时间戳存为 UTC Unix 毫秒（`INTEGER`）。这次精度变更需要重新创建数据库，不提供从旧版秒级时间戳迁移的方式。

## Turn 流程

1. `beginTurn` 原子地创建或更新会话、创建 turn 并保存用户消息。
2. 模型步骤按发生顺序追加助手内容与工具调用。
3. 模型继续执行前，每个持久化工具调用都必须获得结果。
4. 一个 turn 只能以 completed、cancelled 或 failed 终结一次。

## 压缩

压缩检查点保存摘要与最后一个被压缩的 timeline 序号，模型的活动上下文从该序号之后开始。检查点不得拆分 assistant/tool/result 组；超长单 turn 可以压缩最早的安全前缀，同时保留最新条目。

## 失败

持久化与用户可见的失败只包含脱敏摘要。Provider 凭据、请求 header、完整请求体与模型输出不得进入失败消息或结构化日志。
