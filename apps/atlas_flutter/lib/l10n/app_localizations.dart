import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_localizations/flutter_localizations.dart';
import 'package:intl/intl.dart' as intl;

import 'app_localizations_en.dart';
import 'app_localizations_zh.dart';

// ignore_for_file: type=lint

/// Callers can lookup localized strings with an instance of AppLocalizations
/// returned by `AppLocalizations.of(context)`.
///
/// Applications need to include `AppLocalizations.delegate()` in their app's
/// `localizationDelegates` list, and the locales they support in the app's
/// `supportedLocales` list. For example:
///
/// ```dart
/// import 'l10n/app_localizations.dart';
///
/// return MaterialApp(
///   localizationsDelegates: AppLocalizations.localizationsDelegates,
///   supportedLocales: AppLocalizations.supportedLocales,
///   home: MyApplicationHome(),
/// );
/// ```
///
/// ## Update pubspec.yaml
///
/// Please make sure to update your pubspec.yaml to include the following
/// packages:
///
/// ```yaml
/// dependencies:
///   # Internationalization support.
///   flutter_localizations:
///     sdk: flutter
///   intl: any # Use the pinned version from flutter_localizations
///
///   # Rest of dependencies
/// ```
///
/// ## iOS Applications
///
/// iOS applications define key application metadata, including supported
/// locales, in an Info.plist file that is built into the application bundle.
/// To configure the locales supported by your app, you’ll need to edit this
/// file.
///
/// First, open your project’s ios/Runner.xcworkspace Xcode workspace file.
/// Then, in the Project Navigator, open the Info.plist file under the Runner
/// project’s Runner folder.
///
/// Next, select the Information Property List item, select Add Item from the
/// Editor menu, then select Localizations from the pop-up menu.
///
/// Select and expand the newly-created Localizations item then, for each
/// locale your application supports, add a new item and select the locale
/// you wish to add from the pop-up menu in the Value field. This list should
/// be consistent with the languages listed in the AppLocalizations.supportedLocales
/// property.
abstract class AppLocalizations {
  AppLocalizations(String locale)
    : localeName = intl.Intl.canonicalizedLocale(locale.toString());

  final String localeName;

  static AppLocalizations of(BuildContext context) {
    return Localizations.of<AppLocalizations>(context, AppLocalizations)!;
  }

  static const LocalizationsDelegate<AppLocalizations> delegate =
      _AppLocalizationsDelegate();

  /// A list of this localizations delegate along with the default localizations
  /// delegates.
  ///
  /// Returns a list of localizations delegates containing this delegate along with
  /// GlobalMaterialLocalizations.delegate, GlobalCupertinoLocalizations.delegate,
  /// and GlobalWidgetsLocalizations.delegate.
  ///
  /// Additional delegates can be added by appending to this list in
  /// MaterialApp. This list does not have to be used at all if a custom list
  /// of delegates is preferred or required.
  static const List<LocalizationsDelegate<dynamic>> localizationsDelegates =
      <LocalizationsDelegate<dynamic>>[
        delegate,
        GlobalMaterialLocalizations.delegate,
        GlobalCupertinoLocalizations.delegate,
        GlobalWidgetsLocalizations.delegate,
      ];

  /// A list of this localizations delegate's supported locales.
  static const List<Locale> supportedLocales = <Locale>[
    Locale('en'),
    Locale('zh'),
  ];

  /// No description provided for @settings.
  ///
  /// In en, this message translates to:
  /// **'Settings'**
  String get settings;

  /// No description provided for @back.
  ///
  /// In en, this message translates to:
  /// **'Back'**
  String get back;

  /// No description provided for @appearance.
  ///
  /// In en, this message translates to:
  /// **'Appearance'**
  String get appearance;

  /// No description provided for @connections.
  ///
  /// In en, this message translates to:
  /// **'Connections'**
  String get connections;

  /// No description provided for @appearanceDescription.
  ///
  /// In en, this message translates to:
  /// **'How Atlas looks on this device.'**
  String get appearanceDescription;

  /// No description provided for @theme.
  ///
  /// In en, this message translates to:
  /// **'Theme'**
  String get theme;

  /// No description provided for @themeDescription.
  ///
  /// In en, this message translates to:
  /// **'Light, dark, or follow the system.'**
  String get themeDescription;

  /// No description provided for @light.
  ///
  /// In en, this message translates to:
  /// **'Light'**
  String get light;

  /// No description provided for @dark.
  ///
  /// In en, this message translates to:
  /// **'Dark'**
  String get dark;

  /// No description provided for @system.
  ///
  /// In en, this message translates to:
  /// **'System'**
  String get system;

  /// No description provided for @lightTooltip.
  ///
  /// In en, this message translates to:
  /// **'Always use the light appearance'**
  String get lightTooltip;

  /// No description provided for @darkTooltip.
  ///
  /// In en, this message translates to:
  /// **'Always use the dark appearance'**
  String get darkTooltip;

  /// No description provided for @systemThemeTooltip.
  ///
  /// In en, this message translates to:
  /// **'Follow the system appearance'**
  String get systemThemeTooltip;

  /// No description provided for @language.
  ///
  /// In en, this message translates to:
  /// **'Language'**
  String get language;

  /// No description provided for @languageDescription.
  ///
  /// In en, this message translates to:
  /// **'Choose the language used by Atlas on this device.'**
  String get languageDescription;

  /// No description provided for @systemLanguageTooltip.
  ///
  /// In en, this message translates to:
  /// **'Follow the system language'**
  String get systemLanguageTooltip;

  /// No description provided for @english.
  ///
  /// In en, this message translates to:
  /// **'English'**
  String get english;

  /// No description provided for @simplifiedChinese.
  ///
  /// In en, this message translates to:
  /// **'简体中文'**
  String get simplifiedChinese;

  /// No description provided for @sessions.
  ///
  /// In en, this message translates to:
  /// **'Sessions'**
  String get sessions;

  /// No description provided for @closeSessions.
  ///
  /// In en, this message translates to:
  /// **'Close sessions'**
  String get closeSessions;

  /// No description provided for @runtimeUnavailable.
  ///
  /// In en, this message translates to:
  /// **'Runtime unavailable'**
  String get runtimeUnavailable;

  /// No description provided for @today.
  ///
  /// In en, this message translates to:
  /// **'Today'**
  String get today;

  /// No description provided for @yesterday.
  ///
  /// In en, this message translates to:
  /// **'Yesterday'**
  String get yesterday;

  /// No description provided for @thisWeek.
  ///
  /// In en, this message translates to:
  /// **'This Week'**
  String get thisWeek;

  /// No description provided for @thisMonth.
  ///
  /// In en, this message translates to:
  /// **'This Month'**
  String get thisMonth;

  /// No description provided for @earlier.
  ///
  /// In en, this message translates to:
  /// **'Earlier'**
  String get earlier;

  /// No description provided for @now.
  ///
  /// In en, this message translates to:
  /// **'Now'**
  String get now;

  /// No description provided for @renameSession.
  ///
  /// In en, this message translates to:
  /// **'Rename session'**
  String get renameSession;

  /// No description provided for @deleteSession.
  ///
  /// In en, this message translates to:
  /// **'Delete session'**
  String get deleteSession;

  /// No description provided for @deleteSessionQuestion.
  ///
  /// In en, this message translates to:
  /// **'Delete \"{name}\"?'**
  String deleteSessionQuestion(String name);

  /// No description provided for @title.
  ///
  /// In en, this message translates to:
  /// **'Title'**
  String get title;

  /// No description provided for @cancel.
  ///
  /// In en, this message translates to:
  /// **'Cancel'**
  String get cancel;

  /// No description provided for @save.
  ///
  /// In en, this message translates to:
  /// **'Save'**
  String get save;

  /// No description provided for @delete.
  ///
  /// In en, this message translates to:
  /// **'Delete'**
  String get delete;

  /// No description provided for @rename.
  ///
  /// In en, this message translates to:
  /// **'Rename'**
  String get rename;

  /// No description provided for @untitledSession.
  ///
  /// In en, this message translates to:
  /// **'Untitled session'**
  String get untitledSession;

  /// No description provided for @newSession.
  ///
  /// In en, this message translates to:
  /// **'New session'**
  String get newSession;

  /// No description provided for @session.
  ///
  /// In en, this message translates to:
  /// **'Session'**
  String get session;

  /// No description provided for @newSessionHere.
  ///
  /// In en, this message translates to:
  /// **'New session here'**
  String get newSessionHere;

  /// No description provided for @newSessionInFolder.
  ///
  /// In en, this message translates to:
  /// **'New session in folder...'**
  String get newSessionInFolder;

  /// No description provided for @search.
  ///
  /// In en, this message translates to:
  /// **'Search'**
  String get search;

  /// No description provided for @noSessionsYet.
  ///
  /// In en, this message translates to:
  /// **'No sessions yet'**
  String get noSessionsYet;

  /// No description provided for @openSessions.
  ///
  /// In en, this message translates to:
  /// **'Open sessions'**
  String get openSessions;

  /// No description provided for @hideSessions.
  ///
  /// In en, this message translates to:
  /// **'Hide sessions'**
  String get hideSessions;

  /// No description provided for @showSessions.
  ///
  /// In en, this message translates to:
  /// **'Show sessions'**
  String get showSessions;

  /// No description provided for @hideDetails.
  ///
  /// In en, this message translates to:
  /// **'Hide details'**
  String get hideDetails;

  /// No description provided for @showDetails.
  ///
  /// In en, this message translates to:
  /// **'Show details'**
  String get showDetails;

  /// No description provided for @openWorkspaceTools.
  ///
  /// In en, this message translates to:
  /// **'Open workspace tools'**
  String get openWorkspaceTools;

  /// No description provided for @workspaceTools.
  ///
  /// In en, this message translates to:
  /// **'Workspace tools'**
  String get workspaceTools;

  /// No description provided for @remoteToolsHint.
  ///
  /// In en, this message translates to:
  /// **'Remote session: files and terminal run on the computer. Ask the agent to read or modify files and watch the diffs.'**
  String get remoteToolsHint;

  /// No description provided for @closeWorkspaceTools.
  ///
  /// In en, this message translates to:
  /// **'Close workspace tools'**
  String get closeWorkspaceTools;

  /// No description provided for @files.
  ///
  /// In en, this message translates to:
  /// **'Files'**
  String get files;

  /// No description provided for @terminal.
  ///
  /// In en, this message translates to:
  /// **'Terminal'**
  String get terminal;

  /// No description provided for @cannotStartShell.
  ///
  /// In en, this message translates to:
  /// **'Cannot start shell: {error}'**
  String cannotStartShell(String error);

  /// No description provided for @processExited.
  ///
  /// In en, this message translates to:
  /// **'Process exited with code {code}'**
  String processExited(int code);

  /// No description provided for @workingDirectoryChanged.
  ///
  /// In en, this message translates to:
  /// **'Working directory changed to {path}.'**
  String workingDirectoryChanged(String path);

  /// No description provided for @reconnecting.
  ///
  /// In en, this message translates to:
  /// **'Reconnecting…'**
  String get reconnecting;

  /// No description provided for @runtimeNotConfigured.
  ///
  /// In en, this message translates to:
  /// **'Atlas runtime is not configured.'**
  String get runtimeNotConfigured;

  /// No description provided for @startConversation.
  ///
  /// In en, this message translates to:
  /// **'Start a conversation'**
  String get startConversation;

  /// No description provided for @thinking.
  ///
  /// In en, this message translates to:
  /// **'Thinking'**
  String get thinking;

  /// No description provided for @compacting.
  ///
  /// In en, this message translates to:
  /// **'Compacting'**
  String get compacting;

  /// No description provided for @working.
  ///
  /// In en, this message translates to:
  /// **'Working'**
  String get working;

  /// No description provided for @tool.
  ///
  /// In en, this message translates to:
  /// **'Tool'**
  String get tool;

  /// No description provided for @stepsCompleted.
  ///
  /// In en, this message translates to:
  /// **'{completed}/{total} completed'**
  String stepsCompleted(int completed, int total);

  /// No description provided for @toggleActivityDetails.
  ///
  /// In en, this message translates to:
  /// **'Toggle activity details'**
  String get toggleActivityDetails;

  /// No description provided for @messageAtlas.
  ///
  /// In en, this message translates to:
  /// **'Message Atlas'**
  String get messageAtlas;

  /// No description provided for @addCaptionOrSendImage.
  ///
  /// In en, this message translates to:
  /// **'Add a caption, or send the image'**
  String get addCaptionOrSendImage;

  /// No description provided for @stop.
  ///
  /// In en, this message translates to:
  /// **'Stop'**
  String get stop;

  /// No description provided for @send.
  ///
  /// In en, this message translates to:
  /// **'Send'**
  String get send;

  /// No description provided for @attachImage.
  ///
  /// In en, this message translates to:
  /// **'Attach image'**
  String get attachImage;

  /// No description provided for @modelNoImages.
  ///
  /// In en, this message translates to:
  /// **'Current model does not support images'**
  String get modelNoImages;

  /// No description provided for @modelImagesOmitted.
  ///
  /// In en, this message translates to:
  /// **'{model} does not support images; images in this conversation will be omitted.'**
  String modelImagesOmitted(String model);

  /// No description provided for @modelImageInputUnsupported.
  ///
  /// In en, this message translates to:
  /// **'{model} does not support image input.'**
  String modelImageInputUnsupported(String model);

  /// No description provided for @removeImage.
  ///
  /// In en, this message translates to:
  /// **'Remove image'**
  String get removeImage;

  /// No description provided for @tooManyImages.
  ///
  /// In en, this message translates to:
  /// **'You can attach up to {count} images.'**
  String tooManyImages(int count);

  /// No description provided for @imagesTooLarge.
  ///
  /// In en, this message translates to:
  /// **'Images larger than 10 MB were skipped.'**
  String get imagesTooLarge;

  /// No description provided for @cannotLoadSessions.
  ///
  /// In en, this message translates to:
  /// **'Cannot load sessions: {error}'**
  String cannotLoadSessions(String error);

  /// No description provided for @cannotResumeSession.
  ///
  /// In en, this message translates to:
  /// **'Cannot resume session: {error}'**
  String cannotResumeSession(String error);

  /// No description provided for @cannotRenameSession.
  ///
  /// In en, this message translates to:
  /// **'Cannot rename session: {error}'**
  String cannotRenameSession(String error);

  /// No description provided for @cannotDeleteSession.
  ///
  /// In en, this message translates to:
  /// **'Cannot delete session: {error}'**
  String cannotDeleteSession(String error);

  /// No description provided for @cannotSetMode.
  ///
  /// In en, this message translates to:
  /// **'Cannot set mode: {error}'**
  String cannotSetMode(String error);

  /// No description provided for @turnFailed.
  ///
  /// In en, this message translates to:
  /// **'Turn failed: {error}'**
  String turnFailed(String error);

  /// No description provided for @compactionFailed.
  ///
  /// In en, this message translates to:
  /// **'Compaction failed: {error}'**
  String compactionFailed(String error);

  /// No description provided for @directorySaveFailed.
  ///
  /// In en, this message translates to:
  /// **'The directory is active but could not be saved for the next connection.'**
  String get directorySaveFailed;

  /// No description provided for @slashCommandsNoImages.
  ///
  /// In en, this message translates to:
  /// **'Slash commands do not support images.'**
  String get slashCommandsNoImages;

  /// No description provided for @chooseRemoteDirectoryFirst.
  ///
  /// In en, this message translates to:
  /// **'Choose the working directory on the computer before sending the first message.'**
  String get chooseRemoteDirectoryFirst;

  /// No description provided for @turnCancelled.
  ///
  /// In en, this message translates to:
  /// **'Turn cancelled'**
  String get turnCancelled;

  /// No description provided for @noSessionToCompact.
  ///
  /// In en, this message translates to:
  /// **'No session to compact.'**
  String get noSessionToCompact;

  /// No description provided for @contextCompacted.
  ///
  /// In en, this message translates to:
  /// **'Context compacted, kept {count} recent messages.'**
  String contextCompacted(int count);

  /// No description provided for @refreshFiles.
  ///
  /// In en, this message translates to:
  /// **'Refresh files'**
  String get refreshFiles;

  /// No description provided for @toggleMarkdownPreview.
  ///
  /// In en, this message translates to:
  /// **'Toggle markdown preview'**
  String get toggleMarkdownPreview;

  /// No description provided for @backToFiles.
  ///
  /// In en, this message translates to:
  /// **'Back to files'**
  String get backToFiles;

  /// No description provided for @emptyFolder.
  ///
  /// In en, this message translates to:
  /// **'Empty folder'**
  String get emptyFolder;

  /// No description provided for @fileTooLarge.
  ///
  /// In en, this message translates to:
  /// **'File is larger than the 512 KB preview limit.'**
  String get fileTooLarge;

  /// No description provided for @binaryFileCannotPreview.
  ///
  /// In en, this message translates to:
  /// **'Binary files cannot be previewed.'**
  String get binaryFileCannotPreview;

  /// No description provided for @itemAlreadyExists.
  ///
  /// In en, this message translates to:
  /// **'An item with that name already exists.'**
  String get itemAlreadyExists;

  /// No description provided for @cannotMoveFolderIntoItself.
  ///
  /// In en, this message translates to:
  /// **'Cannot move a folder into itself.'**
  String get cannotMoveFolderIntoItself;

  /// No description provided for @enterValidName.
  ///
  /// In en, this message translates to:
  /// **'Enter a valid name.'**
  String get enterValidName;

  /// No description provided for @couldNotFindFreeName.
  ///
  /// In en, this message translates to:
  /// **'Could not find a free name.'**
  String get couldNotFindFreeName;

  /// No description provided for @pathOutsideWorkspace.
  ///
  /// In en, this message translates to:
  /// **'That path is outside the workspace.'**
  String get pathOutsideWorkspace;

  /// No description provided for @couldNotMoveToTrash.
  ///
  /// In en, this message translates to:
  /// **'Could not move the item to Trash.'**
  String get couldNotMoveToTrash;

  /// No description provided for @couldNotRevealItem.
  ///
  /// In en, this message translates to:
  /// **'Could not reveal the item.'**
  String get couldNotRevealItem;

  /// No description provided for @fileNoLongerExists.
  ///
  /// In en, this message translates to:
  /// **'This file no longer exists.'**
  String get fileNoLongerExists;

  /// No description provided for @cannotLocateHome.
  ///
  /// In en, this message translates to:
  /// **'Cannot locate the home directory for Atlas configuration.'**
  String get cannotLocateHome;

  /// No description provided for @cannotLoadConfiguration.
  ///
  /// In en, this message translates to:
  /// **'Cannot load {path}: {error}'**
  String cannotLoadConfiguration(String path, String error);

  /// No description provided for @cannotStartAtlas.
  ///
  /// In en, this message translates to:
  /// **'Cannot start Atlas: {error}'**
  String cannotStartAtlas(String error);

  /// No description provided for @cannotStartAcpServer.
  ///
  /// In en, this message translates to:
  /// **'Cannot start ACP server: {error}'**
  String cannotStartAcpServer(String error);

  /// No description provided for @name.
  ///
  /// In en, this message translates to:
  /// **'Name'**
  String get name;

  /// No description provided for @newFile.
  ///
  /// In en, this message translates to:
  /// **'New File'**
  String get newFile;

  /// No description provided for @newFolder.
  ///
  /// In en, this message translates to:
  /// **'New Folder'**
  String get newFolder;

  /// No description provided for @paste.
  ///
  /// In en, this message translates to:
  /// **'Paste'**
  String get paste;

  /// No description provided for @copy.
  ///
  /// In en, this message translates to:
  /// **'Copy'**
  String get copy;

  /// No description provided for @cut.
  ///
  /// In en, this message translates to:
  /// **'Cut'**
  String get cut;

  /// No description provided for @copyPath.
  ///
  /// In en, this message translates to:
  /// **'Copy Path'**
  String get copyPath;

  /// No description provided for @copyRelativePath.
  ///
  /// In en, this message translates to:
  /// **'Copy Relative Path'**
  String get copyRelativePath;

  /// No description provided for @moveToTrash.
  ///
  /// In en, this message translates to:
  /// **'Move to Trash'**
  String get moveToTrash;

  /// No description provided for @trashQuestion.
  ///
  /// In en, this message translates to:
  /// **'Move “{name}” to the Trash?'**
  String trashQuestion(String name);

  /// No description provided for @revealInFinder.
  ///
  /// In en, this message translates to:
  /// **'Reveal in Finder'**
  String get revealInFinder;

  /// No description provided for @revealInExplorer.
  ///
  /// In en, this message translates to:
  /// **'Reveal in Explorer'**
  String get revealInExplorer;

  /// No description provided for @revealInFileManager.
  ///
  /// In en, this message translates to:
  /// **'Reveal in File Manager'**
  String get revealInFileManager;

  /// No description provided for @allowTool.
  ///
  /// In en, this message translates to:
  /// **'Allow {toolName}?'**
  String allowTool(String toolName);

  /// No description provided for @reject.
  ///
  /// In en, this message translates to:
  /// **'Reject'**
  String get reject;

  /// No description provided for @allowOnce.
  ///
  /// In en, this message translates to:
  /// **'Allow once'**
  String get allowOnce;

  /// No description provided for @alwaysAllow.
  ///
  /// In en, this message translates to:
  /// **'Always allow'**
  String get alwaysAllow;

  /// No description provided for @minimize.
  ///
  /// In en, this message translates to:
  /// **'Minimize'**
  String get minimize;

  /// No description provided for @restore.
  ///
  /// In en, this message translates to:
  /// **'Restore'**
  String get restore;

  /// No description provided for @maximize.
  ///
  /// In en, this message translates to:
  /// **'Maximize'**
  String get maximize;

  /// No description provided for @close.
  ///
  /// In en, this message translates to:
  /// **'Close'**
  String get close;

  /// No description provided for @acpConnections.
  ///
  /// In en, this message translates to:
  /// **'ACP connections'**
  String get acpConnections;

  /// No description provided for @acpConnectionsDescription.
  ///
  /// In en, this message translates to:
  /// **'Run the agent on this computer, or switch to an external ACP server.'**
  String get acpConnectionsDescription;

  /// No description provided for @noConnectionsYet.
  ///
  /// In en, this message translates to:
  /// **'No connections yet. Add one to use an external agent.'**
  String get noConnectionsYet;

  /// No description provided for @addConnection.
  ///
  /// In en, this message translates to:
  /// **'Add connection'**
  String get addConnection;

  /// No description provided for @remoteConnections.
  ///
  /// In en, this message translates to:
  /// **'Remote connections'**
  String get remoteConnections;

  /// No description provided for @backToLocalRuntime.
  ///
  /// In en, this message translates to:
  /// **'Back to local runtime'**
  String get backToLocalRuntime;

  /// No description provided for @activate.
  ///
  /// In en, this message translates to:
  /// **'Activate'**
  String get activate;

  /// No description provided for @removeConnection.
  ///
  /// In en, this message translates to:
  /// **'Remove connection'**
  String get removeConnection;

  /// No description provided for @addAcpConnection.
  ///
  /// In en, this message translates to:
  /// **'Add ACP Connection'**
  String get addAcpConnection;

  /// No description provided for @presets.
  ///
  /// In en, this message translates to:
  /// **'Presets'**
  String get presets;

  /// No description provided for @command.
  ///
  /// In en, this message translates to:
  /// **'Command'**
  String get command;

  /// No description provided for @arguments.
  ///
  /// In en, this message translates to:
  /// **'Arguments (space separated)'**
  String get arguments;

  /// No description provided for @add.
  ///
  /// In en, this message translates to:
  /// **'Add'**
  String get add;

  /// No description provided for @couldNotSaveConnection.
  ///
  /// In en, this message translates to:
  /// **'Could not save the connection: {error}'**
  String couldNotSaveConnection(String error);

  /// No description provided for @couldNotSaveConnectionTitle.
  ///
  /// In en, this message translates to:
  /// **'Could not save the connection'**
  String get couldNotSaveConnectionTitle;

  /// No description provided for @connectionFailed.
  ///
  /// In en, this message translates to:
  /// **'Connection failed'**
  String get connectionFailed;

  /// No description provided for @ok.
  ///
  /// In en, this message translates to:
  /// **'OK'**
  String get ok;

  /// No description provided for @connectToComputer.
  ///
  /// In en, this message translates to:
  /// **'Connect to the Atlas on your computer'**
  String get connectToComputer;

  /// No description provided for @remoteConnectInstructions.
  ///
  /// In en, this message translates to:
  /// **'Run `atlas server` on the computer and enter its address and token below. Models and commands run on the computer; the working directory for new sessions is chosen right before the first message.'**
  String get remoteConnectInstructions;

  /// No description provided for @noSavedConnections.
  ///
  /// In en, this message translates to:
  /// **'No connections yet.'**
  String get noSavedConnections;

  /// No description provided for @connecting.
  ///
  /// In en, this message translates to:
  /// **'Connecting…'**
  String get connecting;

  /// No description provided for @connected.
  ///
  /// In en, this message translates to:
  /// **'Connected'**
  String get connected;

  /// No description provided for @notConnected.
  ///
  /// In en, this message translates to:
  /// **'Not connected'**
  String get notConnected;

  /// No description provided for @edit.
  ///
  /// In en, this message translates to:
  /// **'Edit'**
  String get edit;

  /// No description provided for @remove.
  ///
  /// In en, this message translates to:
  /// **'Remove'**
  String get remove;

  /// No description provided for @disconnect.
  ///
  /// In en, this message translates to:
  /// **'Disconnect'**
  String get disconnect;

  /// No description provided for @retry.
  ///
  /// In en, this message translates to:
  /// **'Retry'**
  String get retry;

  /// No description provided for @connect.
  ///
  /// In en, this message translates to:
  /// **'Connect'**
  String get connect;

  /// No description provided for @addRemoteConnection.
  ///
  /// In en, this message translates to:
  /// **'Add remote connection'**
  String get addRemoteConnection;

  /// No description provided for @editRemoteConnection.
  ///
  /// In en, this message translates to:
  /// **'Edit remote connection'**
  String get editRemoteConnection;

  /// No description provided for @myComputer.
  ///
  /// In en, this message translates to:
  /// **'My computer'**
  String get myComputer;

  /// No description provided for @webSocketUrl.
  ///
  /// In en, this message translates to:
  /// **'WebSocket URL'**
  String get webSocketUrl;

  /// No description provided for @token.
  ///
  /// In en, this message translates to:
  /// **'Token'**
  String get token;

  /// No description provided for @tokenHint.
  ///
  /// In en, this message translates to:
  /// **'printed by `atlas server` on startup'**
  String get tokenHint;

  /// No description provided for @remoteWorkingDirectoryOptional.
  ///
  /// In en, this message translates to:
  /// **'Working directory on the computer (optional)'**
  String get remoteWorkingDirectoryOptional;

  /// No description provided for @nameRequired.
  ///
  /// In en, this message translates to:
  /// **'Name is required.'**
  String get nameRequired;

  /// No description provided for @urlRequired.
  ///
  /// In en, this message translates to:
  /// **'WebSocket URL is required.'**
  String get urlRequired;

  /// No description provided for @tokenRequired.
  ///
  /// In en, this message translates to:
  /// **'Token is required.'**
  String get tokenRequired;

  /// No description provided for @chooseDirectory.
  ///
  /// In en, this message translates to:
  /// **'Choose directory'**
  String get chooseDirectory;

  /// No description provided for @remoteDirectoryTitle.
  ///
  /// In en, this message translates to:
  /// **'Working directory on the computer'**
  String get remoteDirectoryTitle;

  /// No description provided for @remoteDirectoryDescription.
  ///
  /// In en, this message translates to:
  /// **'New sessions run in this directory on the computer (for example /home/you/projects).'**
  String get remoteDirectoryDescription;

  /// No description provided for @directory.
  ///
  /// In en, this message translates to:
  /// **'Directory'**
  String get directory;

  /// No description provided for @useDirectory.
  ///
  /// In en, this message translates to:
  /// **'Use directory'**
  String get useDirectory;

  /// No description provided for @absolutePathExample.
  ///
  /// In en, this message translates to:
  /// **'Enter an absolute path such as /home/you.'**
  String get absolutePathExample;

  /// No description provided for @absolutePathRequired.
  ///
  /// In en, this message translates to:
  /// **'Absolute paths start with a /.'**
  String get absolutePathRequired;

  /// No description provided for @remoteSessionsDirectory.
  ///
  /// In en, this message translates to:
  /// **'Sessions run in a directory on your computer'**
  String get remoteSessionsDirectory;
}

class _AppLocalizationsDelegate
    extends LocalizationsDelegate<AppLocalizations> {
  const _AppLocalizationsDelegate();

  @override
  Future<AppLocalizations> load(Locale locale) {
    return SynchronousFuture<AppLocalizations>(lookupAppLocalizations(locale));
  }

  @override
  bool isSupported(Locale locale) =>
      <String>['en', 'zh'].contains(locale.languageCode);

  @override
  bool shouldReload(_AppLocalizationsDelegate old) => false;
}

AppLocalizations lookupAppLocalizations(Locale locale) {
  // Lookup logic when only language code is specified.
  switch (locale.languageCode) {
    case 'en':
      return AppLocalizationsEn();
    case 'zh':
      return AppLocalizationsZh();
  }

  throw FlutterError(
    'AppLocalizations.delegate failed to load unsupported locale "$locale". This is likely '
    'an issue with the localizations generation tool. Please file an issue '
    'on GitHub with a reproducible sample app and the gen-l10n configuration '
    'that was used.',
  );
}
