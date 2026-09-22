// ignore: unused_import
import 'package:intl/intl.dart' as intl;

import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for Chinese (`zh`).
class AppLocalizationsZh extends AppLocalizations {
  AppLocalizationsZh([String locale = 'zh']) : super(locale);

  @override
  String get settings => '设置';

  @override
  String get back => '返回';

  @override
  String get appearance => '外观';

  @override
  String get connections => '连接';

  @override
  String get appearanceDescription => '设置 Atlas 在此设备上的外观。';

  @override
  String get theme => '主题';

  @override
  String get themeDescription => '浅色、深色或跟随系统。';

  @override
  String get light => '浅色';

  @override
  String get dark => '深色';

  @override
  String get system => '跟随系统';

  @override
  String get lightTooltip => '始终使用浅色外观';

  @override
  String get darkTooltip => '始终使用深色外观';

  @override
  String get systemThemeTooltip => '跟随系统外观';

  @override
  String get language => '语言';

  @override
  String get languageDescription => '选择此设备上的 Atlas 界面语言。';

  @override
  String get systemLanguageTooltip => '跟随系统语言';

  @override
  String get english => 'English';

  @override
  String get simplifiedChinese => '简体中文';

  @override
  String get sessions => '会话';

  @override
  String get closeSessions => '关闭会话列表';

  @override
  String get runtimeUnavailable => '运行环境不可用';

  @override
  String get today => '今天';

  @override
  String get yesterday => '昨天';

  @override
  String get thisWeek => '本周';

  @override
  String get thisMonth => '本月';

  @override
  String get earlier => '更早';

  @override
  String get now => '刚刚';

  @override
  String get renameSession => '重命名会话';

  @override
  String get deleteSession => '删除会话';

  @override
  String deleteSessionQuestion(String name) {
    return '删除“$name”？';
  }

  @override
  String get title => '标题';

  @override
  String get cancel => '取消';

  @override
  String get save => '保存';

  @override
  String get delete => '删除';

  @override
  String get rename => '重命名';

  @override
  String get untitledSession => '未命名会话';

  @override
  String get newSession => '新建会话';

  @override
  String get session => '会话';

  @override
  String get newSessionHere => '在此处新建会话';

  @override
  String get newSessionInFolder => '在文件夹中新建会话…';

  @override
  String get search => '搜索';

  @override
  String get noSessionsYet => '暂无会话';

  @override
  String get openSessions => '打开会话列表';

  @override
  String get hideSessions => '隐藏会话列表';

  @override
  String get showSessions => '显示会话列表';

  @override
  String get hideDetails => '隐藏详情';

  @override
  String get showDetails => '显示详情';

  @override
  String get openWorkspaceTools => '打开工作区工具';

  @override
  String get workspaceTools => '工作区工具';

  @override
  String get remoteToolsHint => '远程会话：文件和终端在电脑上运行。可让智能体读取或修改文件，并查看变更。';

  @override
  String get closeWorkspaceTools => '关闭工作区工具';

  @override
  String get files => '文件';

  @override
  String get terminal => '终端';

  @override
  String cannotStartShell(String error) {
    return '无法启动 shell：$error';
  }

  @override
  String processExited(int code) {
    return '进程已退出，退出码为 $code';
  }

  @override
  String workingDirectoryChanged(String path) {
    return '工作目录已更改为 $path。';
  }

  @override
  String get reconnecting => '正在重新连接…';

  @override
  String get runtimeNotConfigured => 'Atlas 运行环境尚未配置。';

  @override
  String get startConversation => '开始对话';

  @override
  String get thinking => '思考中';

  @override
  String get compacting => '正在压缩上下文';

  @override
  String get working => '处理中';

  @override
  String get tool => '工具';

  @override
  String stepsCompleted(int completed, int total) {
    return '已完成 $completed/$total';
  }

  @override
  String get toggleActivityDetails => '切换活动详情';

  @override
  String get messageAtlas => '向 Atlas 发送消息';

  @override
  String get addCaptionOrSendImage => '添加说明，或直接发送图片';

  @override
  String get stop => '停止';

  @override
  String get send => '发送';

  @override
  String get attachImage => '附加图片';

  @override
  String get modelNoImages => '当前模型不支持图片';

  @override
  String modelImagesOmitted(String model) {
    return '$model 不支持图片；将忽略此对话中的图片。';
  }

  @override
  String modelImageInputUnsupported(String model) {
    return '$model 不支持图片输入。';
  }

  @override
  String get removeImage => '移除图片';

  @override
  String tooManyImages(int count) {
    return '最多可附加 $count 张图片。';
  }

  @override
  String get imagesTooLarge => '已跳过超过 10 MB 的图片。';

  @override
  String cannotLoadSessions(String error) {
    return '无法加载会话：$error';
  }

  @override
  String cannotResumeSession(String error) {
    return '无法恢复会话：$error';
  }

  @override
  String cannotRenameSession(String error) {
    return '无法重命名会话：$error';
  }

  @override
  String cannotDeleteSession(String error) {
    return '无法删除会话：$error';
  }

  @override
  String cannotSetMode(String error) {
    return '无法设置模式：$error';
  }

  @override
  String turnFailed(String error) {
    return '处理失败：$error';
  }

  @override
  String compactionFailed(String error) {
    return '上下文压缩失败：$error';
  }

  @override
  String get directorySaveFailed => '目录已生效，但无法保存供下次连接使用。';

  @override
  String get slashCommandsNoImages => '斜杠命令不支持图片。';

  @override
  String get chooseRemoteDirectoryFirst => '发送第一条消息前，请先选择电脑上的工作目录。';

  @override
  String get turnCancelled => '已取消处理';

  @override
  String get noSessionToCompact => '没有可压缩的会话。';

  @override
  String contextCompacted(int count) {
    return '上下文已压缩，保留了最近 $count 条消息。';
  }

  @override
  String get refreshFiles => '刷新文件';

  @override
  String get toggleMarkdownPreview => '切换 Markdown 预览';

  @override
  String get backToFiles => '返回文件列表';

  @override
  String get emptyFolder => '空文件夹';

  @override
  String get fileTooLarge => '文件超过 512 KB 的预览限制。';

  @override
  String get binaryFileCannotPreview => '无法预览二进制文件。';

  @override
  String get itemAlreadyExists => '已存在同名项目。';

  @override
  String get cannotMoveFolderIntoItself => '无法将文件夹移入自身。';

  @override
  String get enterValidName => '请输入有效名称。';

  @override
  String get couldNotFindFreeName => '无法找到可用名称。';

  @override
  String get pathOutsideWorkspace => '该路径位于工作区之外。';

  @override
  String get couldNotMoveToTrash => '无法将项目移到废纸篓。';

  @override
  String get couldNotRevealItem => '无法在文件管理器中显示该项目。';

  @override
  String get fileNoLongerExists => '该文件已不存在。';

  @override
  String get cannotLocateHome => '无法找到用于 Atlas 配置的用户主目录。';

  @override
  String cannotLoadConfiguration(String path, String error) {
    return '无法加载 $path：$error';
  }

  @override
  String cannotStartAtlas(String error) {
    return '无法启动 Atlas：$error';
  }

  @override
  String cannotStartAcpServer(String error) {
    return '无法启动 ACP 服务：$error';
  }

  @override
  String get name => '名称';

  @override
  String get newFile => '新建文件';

  @override
  String get newFolder => '新建文件夹';

  @override
  String get paste => '粘贴';

  @override
  String get copy => '复制';

  @override
  String get cut => '剪切';

  @override
  String get copyPath => '复制路径';

  @override
  String get copyRelativePath => '复制相对路径';

  @override
  String get moveToTrash => '移到废纸篓';

  @override
  String trashQuestion(String name) {
    return '将“$name”移到废纸篓？';
  }

  @override
  String get revealInFinder => '在访达中显示';

  @override
  String get revealInExplorer => '在资源管理器中显示';

  @override
  String get revealInFileManager => '在文件管理器中显示';

  @override
  String allowTool(String toolName) {
    return '允许使用 $toolName？';
  }

  @override
  String get reject => '拒绝';

  @override
  String get allowOnce => '允许一次';

  @override
  String get alwaysAllow => '始终允许';

  @override
  String get minimize => '最小化';

  @override
  String get restore => '还原';

  @override
  String get maximize => '最大化';

  @override
  String get close => '关闭';

  @override
  String get acpConnections => 'ACP 连接';

  @override
  String get acpConnectionsDescription => '在本机运行智能体，或切换到外部 ACP 连接。';

  @override
  String get noConnectionsYet => '暂无连接。添加连接以使用外部智能体。';

  @override
  String get addConnection => '添加连接';

  @override
  String get remoteConnections => '远程连接';

  @override
  String get backToLocalRuntime => '返回本地运行环境';

  @override
  String get activate => '启用';

  @override
  String get removeConnection => '移除连接';

  @override
  String get addAcpConnection => '添加 ACP 连接';

  @override
  String get presets => '预设';

  @override
  String get command => '命令';

  @override
  String get arguments => '参数（空格分隔）';

  @override
  String get add => '添加';

  @override
  String couldNotSaveConnection(String error) {
    return '无法保存连接：$error';
  }

  @override
  String get couldNotSaveConnectionTitle => '无法保存连接';

  @override
  String get connectionFailed => '连接失败';

  @override
  String get ok => '确定';

  @override
  String get connectToComputer => '连接到电脑上的 Atlas';

  @override
  String get remoteConnectInstructions =>
      '在电脑上运行 `atlas server`，然后在下方输入地址和令牌。模型和命令在电脑上运行；首次发送消息前可以选择新会话的工作目录。';

  @override
  String get noSavedConnections => '暂无连接。';

  @override
  String get connecting => '正在连接…';

  @override
  String get connected => '已连接';

  @override
  String get notConnected => '未连接';

  @override
  String get edit => '编辑';

  @override
  String get remove => '移除';

  @override
  String get disconnect => '断开连接';

  @override
  String get retry => '重试';

  @override
  String get connect => '连接';

  @override
  String get addRemoteConnection => '添加远程连接';

  @override
  String get editRemoteConnection => '编辑远程连接';

  @override
  String get myComputer => '我的电脑';

  @override
  String get webSocketUrl => 'WebSocket 地址';

  @override
  String get token => '令牌';

  @override
  String get tokenHint => '启动 `atlas server` 时打印的令牌';

  @override
  String get remoteWorkingDirectoryOptional => '电脑上的工作目录（可选）';

  @override
  String get nameRequired => '请输入名称。';

  @override
  String get urlRequired => '请输入 WebSocket 地址。';

  @override
  String get tokenRequired => '请输入令牌。';

  @override
  String get chooseDirectory => '选择目录';

  @override
  String get remoteDirectoryTitle => '电脑上的工作目录';

  @override
  String get remoteDirectoryDescription =>
      '新会话将在电脑上的此目录中运行（例如 /home/you/projects）。';

  @override
  String get directory => '目录';

  @override
  String get useDirectory => '使用此目录';

  @override
  String get absolutePathExample => '请输入绝对路径，例如 /home/you。';

  @override
  String get absolutePathRequired => '绝对路径必须以 / 开头。';

  @override
  String get remoteSessionsDirectory => '会话在电脑上的目录中运行';
}
