// ignore: unused_import
import 'package:intl/intl.dart' as intl;

import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for English (`en`).
class AppLocalizationsEn extends AppLocalizations {
  AppLocalizationsEn([String locale = 'en']) : super(locale);

  @override
  String get settings => 'Settings';

  @override
  String get back => 'Back';

  @override
  String get appearance => 'Appearance';

  @override
  String get connections => 'Connections';

  @override
  String get appearanceDescription => 'How Atlas looks on this device.';

  @override
  String get theme => 'Theme';

  @override
  String get themeDescription => 'Light, dark, or follow the system.';

  @override
  String get light => 'Light';

  @override
  String get dark => 'Dark';

  @override
  String get system => 'System';

  @override
  String get lightTooltip => 'Always use the light appearance';

  @override
  String get darkTooltip => 'Always use the dark appearance';

  @override
  String get systemThemeTooltip => 'Follow the system appearance';

  @override
  String get language => 'Language';

  @override
  String get languageDescription =>
      'Choose the language used by Atlas on this device.';

  @override
  String get systemLanguageTooltip => 'Follow the system language';

  @override
  String get english => 'English';

  @override
  String get simplifiedChinese => '简体中文';

  @override
  String get sessions => 'Sessions';

  @override
  String get closeSessions => 'Close sessions';

  @override
  String get runtimeUnavailable => 'Runtime unavailable';

  @override
  String get today => 'Today';

  @override
  String get yesterday => 'Yesterday';

  @override
  String get thisWeek => 'This Week';

  @override
  String get thisMonth => 'This Month';

  @override
  String get earlier => 'Earlier';

  @override
  String get now => 'Now';

  @override
  String get renameSession => 'Rename session';

  @override
  String get deleteSession => 'Delete session';

  @override
  String deleteSessionQuestion(String name) {
    return 'Delete \"$name\"?';
  }

  @override
  String get title => 'Title';

  @override
  String get cancel => 'Cancel';

  @override
  String get save => 'Save';

  @override
  String get delete => 'Delete';

  @override
  String get rename => 'Rename';

  @override
  String get untitledSession => 'Untitled session';

  @override
  String get newSession => 'New session';

  @override
  String get session => 'Session';

  @override
  String get newSessionHere => 'New session here';

  @override
  String get newSessionInFolder => 'New session in folder...';

  @override
  String get search => 'Search';

  @override
  String get noSessionsYet => 'No sessions yet';

  @override
  String get openSessions => 'Open sessions';

  @override
  String get hideSessions => 'Hide sessions';

  @override
  String get showSessions => 'Show sessions';

  @override
  String get hideDetails => 'Hide details';

  @override
  String get showDetails => 'Show details';

  @override
  String get openWorkspaceTools => 'Open workspace tools';

  @override
  String get workspaceTools => 'Workspace tools';

  @override
  String get remoteToolsHint =>
      'Remote session: files and terminal run on the computer. Ask the agent to read or modify files and watch the diffs.';

  @override
  String get closeWorkspaceTools => 'Close workspace tools';

  @override
  String get files => 'Files';

  @override
  String get terminal => 'Terminal';

  @override
  String cannotStartShell(String error) {
    return 'Cannot start shell: $error';
  }

  @override
  String processExited(int code) {
    return 'Process exited with code $code';
  }

  @override
  String workingDirectoryChanged(String path) {
    return 'Working directory changed to $path.';
  }

  @override
  String get reconnecting => 'Reconnecting…';

  @override
  String get runtimeNotConfigured => 'Atlas runtime is not configured.';

  @override
  String get startConversation => 'Start a conversation';

  @override
  String get thinking => 'Thinking';

  @override
  String get compacting => 'Compacting';

  @override
  String get working => 'Working';

  @override
  String get tool => 'Tool';

  @override
  String stepsCompleted(int completed, int total) {
    return '$completed/$total completed';
  }

  @override
  String get toggleActivityDetails => 'Toggle activity details';

  @override
  String get messageAtlas => 'Message Atlas';

  @override
  String get addCaptionOrSendImage => 'Add a caption, or send the image';

  @override
  String get stop => 'Stop';

  @override
  String get send => 'Send';

  @override
  String get attachImage => 'Attach image';

  @override
  String get modelNoImages => 'Current model does not support images';

  @override
  String modelImagesOmitted(String model) {
    return '$model does not support images; images in this conversation will be omitted.';
  }

  @override
  String modelImageInputUnsupported(String model) {
    return '$model does not support image input.';
  }

  @override
  String get removeImage => 'Remove image';

  @override
  String tooManyImages(int count) {
    return 'You can attach up to $count images.';
  }

  @override
  String get imagesTooLarge => 'Images larger than 10 MB were skipped.';

  @override
  String cannotLoadSessions(String error) {
    return 'Cannot load sessions: $error';
  }

  @override
  String cannotResumeSession(String error) {
    return 'Cannot resume session: $error';
  }

  @override
  String cannotRenameSession(String error) {
    return 'Cannot rename session: $error';
  }

  @override
  String cannotDeleteSession(String error) {
    return 'Cannot delete session: $error';
  }

  @override
  String cannotSetMode(String error) {
    return 'Cannot set mode: $error';
  }

  @override
  String turnFailed(String error) {
    return 'Turn failed: $error';
  }

  @override
  String compactionFailed(String error) {
    return 'Compaction failed: $error';
  }

  @override
  String get directorySaveFailed =>
      'The directory is active but could not be saved for the next connection.';

  @override
  String get slashCommandsNoImages => 'Slash commands do not support images.';

  @override
  String get chooseRemoteDirectoryFirst =>
      'Choose the working directory on the computer before sending the first message.';

  @override
  String get turnCancelled => 'Turn cancelled';

  @override
  String get noSessionToCompact => 'No session to compact.';

  @override
  String contextCompacted(int count) {
    return 'Context compacted, kept $count recent messages.';
  }

  @override
  String get refreshFiles => 'Refresh files';

  @override
  String get toggleMarkdownPreview => 'Toggle markdown preview';

  @override
  String get backToFiles => 'Back to files';

  @override
  String get emptyFolder => 'Empty folder';

  @override
  String get fileTooLarge => 'File is larger than the 512 KB preview limit.';

  @override
  String get binaryFileCannotPreview => 'Binary files cannot be previewed.';

  @override
  String get itemAlreadyExists => 'An item with that name already exists.';

  @override
  String get cannotMoveFolderIntoItself => 'Cannot move a folder into itself.';

  @override
  String get enterValidName => 'Enter a valid name.';

  @override
  String get couldNotFindFreeName => 'Could not find a free name.';

  @override
  String get pathOutsideWorkspace => 'That path is outside the workspace.';

  @override
  String get couldNotMoveToTrash => 'Could not move the item to Trash.';

  @override
  String get couldNotRevealItem => 'Could not reveal the item.';

  @override
  String get fileNoLongerExists => 'This file no longer exists.';

  @override
  String get cannotLocateHome =>
      'Cannot locate the home directory for Atlas configuration.';

  @override
  String cannotLoadConfiguration(String path, String error) {
    return 'Cannot load $path: $error';
  }

  @override
  String cannotStartAtlas(String error) {
    return 'Cannot start Atlas: $error';
  }

  @override
  String cannotStartAcpServer(String error) {
    return 'Cannot start ACP server: $error';
  }

  @override
  String get name => 'Name';

  @override
  String get newFile => 'New File';

  @override
  String get newFolder => 'New Folder';

  @override
  String get paste => 'Paste';

  @override
  String get copy => 'Copy';

  @override
  String get cut => 'Cut';

  @override
  String get copyPath => 'Copy Path';

  @override
  String get copyRelativePath => 'Copy Relative Path';

  @override
  String get moveToTrash => 'Move to Trash';

  @override
  String trashQuestion(String name) {
    return 'Move “$name” to the Trash?';
  }

  @override
  String get revealInFinder => 'Reveal in Finder';

  @override
  String get revealInExplorer => 'Reveal in Explorer';

  @override
  String get revealInFileManager => 'Reveal in File Manager';

  @override
  String allowTool(String toolName) {
    return 'Allow $toolName?';
  }

  @override
  String get reject => 'Reject';

  @override
  String get allowOnce => 'Allow once';

  @override
  String get alwaysAllow => 'Always allow';

  @override
  String get minimize => 'Minimize';

  @override
  String get restore => 'Restore';

  @override
  String get maximize => 'Maximize';

  @override
  String get close => 'Close';

  @override
  String get acpConnections => 'ACP connections';

  @override
  String get acpConnectionsDescription =>
      'Run the agent on this computer, or switch to an external ACP server.';

  @override
  String get noConnectionsYet =>
      'No connections yet. Add one to use an external agent.';

  @override
  String get addConnection => 'Add connection';

  @override
  String get remoteConnections => 'Remote connections';

  @override
  String get backToLocalRuntime => 'Back to local runtime';

  @override
  String get activate => 'Activate';

  @override
  String get removeConnection => 'Remove connection';

  @override
  String get addAcpConnection => 'Add ACP Connection';

  @override
  String get presets => 'Presets';

  @override
  String get command => 'Command';

  @override
  String get arguments => 'Arguments (space separated)';

  @override
  String get add => 'Add';

  @override
  String couldNotSaveConnection(String error) {
    return 'Could not save the connection: $error';
  }

  @override
  String get couldNotSaveConnectionTitle => 'Could not save the connection';

  @override
  String get connectionFailed => 'Connection failed';

  @override
  String get ok => 'OK';

  @override
  String get connectToComputer => 'Connect to the Atlas on your computer';

  @override
  String get remoteConnectInstructions =>
      'Run `atlas server` on the computer and enter its address and token below. Models and commands run on the computer; the working directory for new sessions is chosen right before the first message.';

  @override
  String get noSavedConnections => 'No connections yet.';

  @override
  String get connecting => 'Connecting…';

  @override
  String get connected => 'Connected';

  @override
  String get notConnected => 'Not connected';

  @override
  String get edit => 'Edit';

  @override
  String get remove => 'Remove';

  @override
  String get disconnect => 'Disconnect';

  @override
  String get retry => 'Retry';

  @override
  String get connect => 'Connect';

  @override
  String get addRemoteConnection => 'Add remote connection';

  @override
  String get editRemoteConnection => 'Edit remote connection';

  @override
  String get myComputer => 'My computer';

  @override
  String get webSocketUrl => 'WebSocket URL';

  @override
  String get token => 'Token';

  @override
  String get tokenHint => 'printed by `atlas server` on startup';

  @override
  String get remoteWorkingDirectoryOptional =>
      'Working directory on the computer (optional)';

  @override
  String get nameRequired => 'Name is required.';

  @override
  String get urlRequired => 'WebSocket URL is required.';

  @override
  String get tokenRequired => 'Token is required.';

  @override
  String get chooseDirectory => 'Choose directory';

  @override
  String get remoteDirectoryTitle => 'Working directory on the computer';

  @override
  String get remoteDirectoryDescription =>
      'New sessions run in this directory on the computer (for example /home/you/projects).';

  @override
  String get directory => 'Directory';

  @override
  String get useDirectory => 'Use directory';

  @override
  String get absolutePathExample => 'Enter an absolute path such as /home/you.';

  @override
  String get absolutePathRequired => 'Absolute paths start with a /.';

  @override
  String get remoteSessionsDirectory =>
      'Sessions run in a directory on your computer';
}
