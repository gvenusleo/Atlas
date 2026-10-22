// Package host 为 App、ACP 等入口组装 Atlas 后端并管理其生命周期。
//
// # 资源与会话
//
// 使用 Open 创建 Manager；空数据库路径使用 ~/.atlas/atlas.db。
// Manager 拥有数据库，不需要入口直接创建 config、llm、tools、prompt 或 Store。
// 同一进程中的各入口应共享 Manager；一个数据库在进程内只使用一个 Manager。
// 多个进程可共享数据库，但同一会话只应由一个进程运行：另一进程已追加消息后，
// 本进程的下一次保存返回 ErrConflict，会话需通过 Resume 重新初始化。
// Open 不加载模型凭据，列表和历史查询无需模型配置、凭据或有效的工作目录。
//
// Create 使用当前默认模型和首个可用思考档位。Resume 在同一个会话实例上
// 重新加载历史、配置、提示词和技能目录，提示词变化时追加完整快照。
// 普通 Run 复用初始化资源；SetModel 只更新模型，不刷新提示词。
// 会话的模型（完整引用 provider/model-id）、思考深度、提示词与技能目录只记在会话的
// LoopContext 里，State 是它的快照；会话表里的模型与档位只用来持久化，供 Resume 恢复。
// 助手消息的 Model 同样是完整引用，由循环按 LoopContext 记下。
// History 返回全部持久化记录，包括提示词快照；入口自行决定展示或协议映射。
// 缺失工具结果只在模型请求中临时补齐，不重执行工具或修改原始历史。
// 会话尚无名称时，首条用户消息保存后以其文本的前 30 个字作为名称，只有图片时叫“图片”。
//
// # 图片
//
// 入口先用 imaging.Prepare 整理用户给的图片（限制格式与大小，太大的缩小），再放进消息
// 交给 Run。模型没有声明图片输入（modalities.input）时，请求里的图片换成一句说明，
// 历史里的图片不动。
//
// # 运行与事件
//
// Run 同步执行；入口可在自己的后台 goroutine 或协议处理器中调用。
// 同一会话的运行、恢复和修改互斥，冲突返回 ErrBusy；不同会话可并行操作。
// Cancel 仅发送当前 Run 的取消信号；WaitIdle 等待调用时已登记的操作收尾，
// 不等待未来操作。State 可在运行期间读取，返回独立快照。
//
// Run 的事件同步、有序交付。message_end 先尝试保存，再通知入口，
// 最终保存结果以 Run 返回的错误为准。agent_end 不代表 Run 已完成清理；
// 需要等待 Run 返回或 WaitIdle。输入、事件与返回消息不共享内部历史的可变数据。
//
// 回调可调用 Cancel 和 State，但不得重入本会话的运行、恢复或修改方法，
// 也不得调用 WaitIdle 或 Manager.Close。回调必须及时返回；永久阻塞的回调
// 无法被 context 强制终止，会拖延运行和宿主关闭。
//
// # 关闭与错误
//
// Close 停止接收操作，取消已登记操作，等待运行、持久化与事件交付结束，
// 再关闭数据库。等待 context 超时后，后台仍继续关闭；可再次 Close 等待结果。
// Open 的 context 只控制打开过程，运行由每次操作的 context 与 Close 共同控制。
//
// 错误可用 errors.Is 判断：ErrClosed 表示宿主关闭中或已关闭；ErrDeleted
// 表示原会话实例已删除；ErrNotFound 表示没有对应记录；ErrNeedsReload
// 表示状态失效，需通过 Resume 重新初始化；ErrConflict 表示会话已被其他进程
// 追加消息，随保存失败与 ErrNeedsReload 一同返回。取消和超时保留标准 context 错误。
package host
