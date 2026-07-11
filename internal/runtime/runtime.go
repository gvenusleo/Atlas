// Package runtime assembles the configuration, provider, tools, prompts, and session store needed for a single Atlas run.
package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/compact"
	"github.com/liuyuxin/atlas/internal/config"
	"github.com/liuyuxin/atlas/internal/memory"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/prompt"
	"github.com/liuyuxin/atlas/internal/provider/chatcompletions"
	"github.com/liuyuxin/atlas/internal/provider/responses"
	"github.com/liuyuxin/atlas/internal/session"
	"github.com/liuyuxin/atlas/internal/skill"
	"github.com/liuyuxin/atlas/internal/tool"
	"github.com/liuyuxin/atlas/internal/transcript"
)

// Dependencies defines the external dependencies required by Runtime; tests can replace any of them.
type Dependencies struct {
	LoadConfig       func() (config.Config, error)
	ConfigPath       func() (string, error)
	NewProvider      func(config.ProviderConfig, config.ProviderModel) (model.Provider, error)
	Getwd            func() (string, error)
	LoadInstructions func(string) ([]prompt.InstructionFile, error)
	LoadSkills       func(string) (*skill.Catalog, error)
	NewSessionID     func(time.Time) (string, error)
	Now              func() time.Time
}

// Runtime is the Atlas execution entry point shared by the CLI and future interactive interfaces.
type Runtime struct {
	deps   Dependencies
	db     *sql.DB // shared database connection (WAL, busy_timeout, MaxOpenConns=1)
	dbPath string  // cached database path for drift detection
	dbMu   sync.Mutex
	dbWG   sync.WaitGroup // tracks active DB users for safe Close
}

// TurnOptions describes the execution parameters for a single user input.
type TurnOptions struct {
	SessionID string
	Prompt    string
	Parts     []model.ContentPart
	// Skills is the skill names explicitly selected by the caller, requiring full SKILL.md injection for this turn.
	Skills                   []string
	Model                    string
	ReasoningEffort          string
	AdditionalDirectories    []string
	AdditionalDirectoriesSet bool
	// ReasoningEffortSet indicates ReasoningEffort was explicitly selected by the caller.
	ReasoningEffortSet bool
	CWD                string
	Observer           agent.Observer
	ToolRunner         ToolRunner
}

// ToolRunner can override the execution of a specific tool call in the runtime.
type ToolRunner func(context.Context, model.ToolCall, tool.RunFunc) (tool.RunResult, error)

// TurnResult describes the result after a user input is completed.
type TurnResult struct {
	SessionID     string
	Content       string
	Usage         model.Usage
	ContextWindow int
}

// CompactOptions describes a manual context compaction request.
type CompactOptions struct {
	SessionID          string
	Model              string
	ReasoningEffort    string
	ReasoningEffortSet bool
	CWD                string
	Instruction        string
}

// CompactResult describes the result after context compaction.
type CompactResult struct {
	SessionID     string
	Compacted     bool
	CompactCount  int
	KeepCount     int
	TokensBefore  int
	TokensAfter   int
	ContextWindow int
	Summary       string
	Reason        string
}

// ModelOption describes a selectable model exposed by the runtime.
type ModelOption struct {
	Value            string
	Name             string
	Description      string
	ContextWindow    int
	MaxTokens        int
	InputFormats     []string
	ReasoningEfforts []ReasoningEffortOption
}

// ReasoningEffortOption describes a reasoning effort option supported by a model.
type ReasoningEffortOption struct {
	Value       string
	Name        string
	Description string
}

// ModelOptions describes the current configured model selection state.
type ModelOptions struct {
	Default string
	Models  []ModelOption
}

// SkillSummary describes a model-invocable skill available in the current working directory.
type SkillSummary struct {
	Name        string
	Description string
}

// DoctorStatus describes the severity of a doctor diagnostic result.
type DoctorStatus string

const (
	interruptedTurnSaveTimeout = 2 * time.Second

	// DoctorStatusOK indicates the check passed.
	DoctorStatusOK DoctorStatus = "ok"
	// DoctorStatusWarn indicates a capability is unavailable or configuration is missing, but does not block core operation.
	DoctorStatusWarn DoctorStatus = "warn"
	// DoctorStatusFail indicates Atlas's core runtime prerequisites are not met.
	DoctorStatusFail DoctorStatus = "fail"
)

// DoctorCheck describes a single diagnostic item in the atlas doctor output.
type DoctorCheck struct {
	Name   string
	Status DoctorStatus
	Detail string
}

// DoctorReport summarizes all diagnostic results from atlas doctor.
type DoctorReport struct {
	Checks []DoctorCheck
}

// Failed returns whether the report contains any failed items.
func (r DoctorReport) Failed() bool {
	for _, check := range r.Checks {
		if check.Status == DoctorStatusFail {
			return true
		}
	}
	return false
}

// DefaultDependencies returns the dependencies used by the real CLI runtime.
func DefaultDependencies() Dependencies {
	return Dependencies{
		LoadConfig: config.LoadDefault,
		ConfigPath: config.DefaultPath,
		NewProvider: func(cfg config.ProviderConfig, selected config.ProviderModel) (model.Provider, error) {
			return newAPIProvider(cfg, selected)
		},
		Getwd: os.Getwd,
		LoadInstructions: func(cwd string) ([]prompt.InstructionFile, error) {
			return prompt.LoadInstructions(cwd)
		},
		LoadSkills:   skill.Load,
		NewSessionID: session.NewID,
		Now:          time.Now,
	}
}

func newAPIProvider(cfg config.ProviderConfig, selected config.ProviderModel) (model.Provider, error) {
	switch providerFormat(cfg) {
	case config.ProviderFormatChatCompletions:
		return chatcompletions.New(chatcompletions.Config{
			BaseURL:            cfg.BaseURL,
			APIKey:             cfg.APIKey,
			Model:              selected.Value,
			UserAgent:          cfg.UserAgent,
			PromptCacheEnabled: selected.PromptCache.Enabled,
		})
	case config.ProviderFormatResponses:
		return responses.New(responses.Config{
			BaseURL:            cfg.BaseURL,
			APIKey:             cfg.APIKey,
			Model:              selected.Value,
			UserAgent:          cfg.UserAgent,
			PromptCacheEnabled: selected.PromptCache.Enabled,
		})
	default:
		return nil, fmt.Errorf("unsupported provider format %q", cfg.Format)
	}
}

type sessionProvider struct {
	base      model.Provider
	sessionID string
}

func withSessionID(provider model.Provider, sessionID string) model.Provider {
	if sessionID == "" {
		return provider
	}
	return sessionProvider{base: provider, sessionID: sessionID}
}

func (p sessionProvider) Stream(ctx context.Context, req model.ChatRequest, emit func(model.StreamEvent) error) (model.ChatResponse, error) {
	if req.SessionID == "" {
		req.SessionID = p.sessionID
	}
	return p.base.Stream(ctx, req, emit)
}

func providerFormat(cfg config.ProviderConfig) string {
	if cfg.Format == "" {
		return config.ProviderFormatChatCompletions
	}
	return cfg.Format
}

// New creates a Runtime, filling in default implementations for unspecified dependencies.
func New(deps Dependencies) *Runtime {
	return &Runtime{deps: completeDependencies(deps)}
}

// openDB returns the shared *sql.DB, initializing it on first call.
// dbPath is resolved by the caller from the same config used for the rest of the operation,
// ensuring a single turn never sees two different DB paths.
// Sets WAL mode, busy_timeout, and MaxOpenConns(1) for safe concurrent access.
func (r *Runtime) openDB(ctx context.Context, dbPath string) (*sql.DB, error) {
	r.dbMu.Lock()
	defer r.dbMu.Unlock()
	if r.db != nil {
		if r.dbPath != dbPath {
			return nil, fmt.Errorf("database path changed from %q to %q; restart Atlas to switch databases", r.dbPath, dbPath)
		}
		return r.db, nil
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, `PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, err
	}
	if _, err := db.ExecContext(ctx, `PRAGMA busy_timeout=5000`); err != nil {
		db.Close()
		return nil, err
	}
	r.db = db
	r.dbPath = dbPath
	return db, nil
}

// Close closes the shared database connection. It waits for all active
// long-running operations (turns, compactions, memory jobs) to finish
// before closing. Quick read operations (list, show, delete) are
// synchronous and complete before the caller reaches Close.
// Safe to call multiple times.
func (r *Runtime) Close() error {
	r.dbWG.Wait()
	r.dbMu.Lock()
	defer r.dbMu.Unlock()
	if r.db == nil {
		return nil
	}
	err := r.db.Close()
	r.db = nil
	r.dbPath = ""
	return err
}

// RunTurn restores or creates a session, executes an agent turn, and saves the transcript.
func (r *Runtime) RunTurn(ctx context.Context, opts TurnOptions) (TurnResult, error) {
	r.dbWG.Add(1)
	defer r.dbWG.Done()

	parts := turnContentParts(opts)
	promptText := strings.TrimSpace(model.TextFromParts(parts))
	if len(parts) == 0 || promptText == "" && !hasImageParts(parts) {
		return TurnResult{}, fmt.Errorf("prompt is required")
	}

	cfg, err := r.deps.LoadConfig()
	if err != nil {
		return TurnResult{}, err
	}
	cwd := opts.CWD
	if cwd == "" {
		cwd, err = r.deps.Getwd()
		if err != nil {
			return TurnResult{}, err
		}
	}

	sessionID := opts.SessionID
	resumeSession := sessionID != ""
	if sessionID == "" {
		sessionID, err = r.deps.NewSessionID(r.deps.Now())
		if err != nil {
			return TurnResult{}, err
		}
	}
	if err := session.ValidateID(sessionID); err != nil {
		return TurnResult{}, err
	}

	store, err := r.openSessionStore(ctx, cfg.Session)
	if err != nil {
		return TurnResult{}, err
	}

	shellCommand, isDirectShell := directShellCommand(promptText)
	if isDirectShell && hasImageParts(parts) {
		return TurnResult{}, fmt.Errorf("direct shell input does not support images")
	}
	if isDirectShell && shellCommand == "" {
		return TurnResult{}, fmt.Errorf("shell command is required after !")
	}
	fullTrans := transcript.New()
	var sessionInfo session.Session
	if resumeSession {
		fullTrans, err = store.LoadTranscript(ctx, sessionID)
		if err != nil {
			return TurnResult{}, err
		}
		sessionInfo, err = store.GetSession(ctx, sessionID)
		if err != nil && !isSessionNotFound(err) {
			return TurnResult{}, err
		}
		if err != nil {
			sessionInfo = session.Session{}
		}
	}
	if isDirectShell {
		result, savedMessages, err := runDirectShellTurn(ctx, store, sessionID, cwd, fullTrans.Messages(), promptText, shellCommand, opts.Observer, opts.ToolRunner, session.SaveTranscriptOptions{
			AdditionalDirectories:    opts.AdditionalDirectories,
			AdditionalDirectoriesSet: opts.AdditionalDirectoriesSet,
		})
		if err == nil {
			r.maybeEnqueueMemoryExtract(ctx, cfg.Session, sessionInfo, sessionID, savedMessages, configuredMemoryModel(cfg, ""), memoryExtractTriggerOptions{})
		}
		return result, err
	}

	activeProvider, selectedModel, err := cfg.ResolveModel(opts.Model)
	if err != nil {
		return TurnResult{}, err
	}
	if hasImageParts(parts) && !selectedModel.SupportsInputFormat(config.ModelInputFormatImage) {
		return TurnResult{}, fmt.Errorf("model %q does not support image input", selectedModel.Value)
	}
	reasoningEffort, err := selectedReasoningEffort(opts.ReasoningEffort, opts.ReasoningEffortSet, selectedModel)
	if err != nil {
		return TurnResult{}, err
	}
	provider, err := r.deps.NewProvider(activeProvider, selectedModel)
	if err != nil {
		return TurnResult{}, err
	}
	provider = withSessionID(provider, sessionID)
	instructions, err := r.deps.LoadInstructions(cwd)
	if err != nil {
		return TurnResult{}, err
	}
	skills, err := r.deps.LoadSkills(cwd)
	if err != nil {
		return TurnResult{}, err
	}
	// Open memory store and create search function for the memory_search tool.
	var memorySearch tool.MemorySearchFunc
	if memStore, err := r.openMemoryStore(ctx, cfg.Session); err == nil {
		memorySearch = func(searchCtx context.Context, query string, limit int) ([]tool.MemoryEntry, error) {
			entries, err := memStore.Search(searchCtx, cwd, query, limit)
			if err != nil {
				return nil, err
			}
			result := make([]tool.MemoryEntry, 0, len(entries))
			for _, e := range entries {
				result = append(result, tool.MemoryEntry{
					Scope:      e.Scope,
					Type:       e.Type,
					Content:    e.Content,
					SourceNote: e.SourceNote,
				})
			}
			return result, nil
		}
	}
	registry, err := buildToolRegistry(cwd, skills, cfg.Services, memorySearch)
	if err != nil {
		return TurnResult{}, err
	}
	if opts.ToolRunner != nil {
		baseRunner := registry.RunDefault
		registry = registry.WithRunner(func(ctx context.Context, call model.ToolCall) (tool.RunResult, error) {
			return opts.ToolRunner(ctx, call, baseRunner)
		})
	}
	memoryForceExtract := false
	selectedSkillMessages := skillMessages(opts.Skills, skills)
	if resumeSession {
		if shouldAutoCompact(sessionInfo, fullTrans.Messages(), selectedSkillMessages, parts, selectedModel.ContextWindow, cfg.Agent.CompactionTriggerRatio) {
			result, err := r.compactLoadedSession(ctx, store, provider, cfg, selectedModel, sessionID, sessionInfo, fullTrans.Messages(), "", opts.ReasoningEffort, opts.ReasoningEffortSet, false)
			if err != nil {
				return TurnResult{}, err
			}
			if result.Compacted {
				sessionInfo.ContextSummary = result.Summary
				sessionInfo.CompactedMessageCount = result.CompactCount
				sessionInfo.CompactedInputTokens = result.TokensBefore
				memoryForceExtract = true
			}
		}
	}
	activeMessages := compact.BuildActiveMessages(sessionInfo.ContextSummary, sessionInfo.CompactedMessageCount, fullTrans.Messages())
	// If the current model does not support image input, filter out image segments from active messages.
	// Historical image messages are not deleted; image parts are only removed when sending to the model.
	if !selectedModel.SupportsInputFormat(config.ModelInputFormatImage) {
		activeMessages = stripImageParts(activeMessages)
	}
	trans := transcript.New()
	for _, msg := range activeMessages {
		trans.Append(msg)
	}
	compacted := false
	persistFrom := 0
	// Full history and post-reset message count recorded during compaction recovery, used to rebuild the logical transcript.
	var preCompactionMessages []model.Message
	postCompactionStart := 0
	var persistErr error
	a, err := agent.New(agent.Config{
		Provider:   provider,
		Tools:      registry,
		Transcript: trans,
		System: prompt.BuildSystem(prompt.Options{
			WorkingDir:   cwd,
			Now:          r.deps.Now(),
			Shell:        tool.DefaultShell().DisplayName,
			Instructions: instructions,
			Skills:       promptSkillSummaries(skills),
		}),
		MaxSteps:        cfg.Agent.MaxSteps,
		MaxTokens:       selectedModel.MaxTokens,
		Temperature:     cfg.Agent.Temperature,
		ReasoningEffort: reasoningEffort,
		Compactor: func(compactCtx context.Context) error {
			// Merge full history: fullTrans's compacted prefix + new messages in agent transcript
			currentMessages := append(fullTrans.Messages(), trans.Messages()[persistFrom:]...)
			result, compactErr := r.compactLoadedSession(compactCtx, store, provider, cfg, selectedModel, sessionID, sessionInfo, currentMessages, "", opts.ReasoningEffort, opts.ReasoningEffortSet, false)
			if compactErr != nil {
				return compactErr
			}
			if !result.Compacted {
				return fmt.Errorf("compaction skipped: %s", result.Reason)
			}
			sessionInfo.ContextSummary = result.Summary
			sessionInfo.CompactedMessageCount = result.CompactCount
			sessionInfo.CompactedInputTokens = result.TokensBefore
			// Rebuild agent transcript with compacted active messages
			activeMsgs := compact.BuildActiveMessages(result.Summary, result.CompactCount, currentMessages)
			// Active messages rebuilt after compaction may re-include historical images; filter again.
			if !selectedModel.SupportsInputFormat(config.ModelInputFormatImage) {
				activeMsgs = stripImageParts(activeMsgs)
			}
			trans.Reset(activeMsgs)
			// Re-inject explicitly selected skill context for this turn.
			// After Reset, trans only contains compacted active messages; skill messages are appended before persistFrom,
			// Not in currentMessages; must be re-appended for model visibility during retry.
			for _, msg := range selectedSkillMessages {
				trans.Append(msg)
			}
			// Record the full history and the first real post-compaction message index.
			// Temporary skill context remains model-visible but outside the persisted transcript.
			preCompactionMessages = currentMessages
			postCompactionStart = len(trans.Messages())
			compacted = true
			return nil
		},
		OnAppend: func(msg model.Message) {
			if persistErr == nil {
				persistErr = appendNow(store, sessionID, cwd, msg, opts)
			}
		},
		Observer: opts.Observer,
	})
	if err != nil {
		return TurnResult{}, err
	}

	for _, msg := range selectedSkillMessages {
		trans.Append(msg)
	}
	persistFrom = len(trans.Messages())
	content, err := a.RunTurnParts(ctx, parts)
	activeAfter := trans.Messages()
	var fullMessages []model.Message
	if compacted {
		// After compaction recovery, save the full transcript: full history before compaction + new messages produced by the agent after compaction.
		// SaveCompaction has recorded CompactedMessageCount; the next BuildActiveMessages will correctly cut the prefix.
		fullMessages = append(preCompactionMessages, activeAfter[postCompactionStart:]...)
	} else {
		if persistFrom > len(activeAfter) {
			persistFrom = len(activeAfter)
		}
		fullMessages = append(fullTrans.Messages(), activeAfter[persistFrom:]...)
	}
	if persistErr != nil {
		saveCtx, cancel := context.WithTimeout(context.Background(), interruptedTurnSaveTimeout)
		defer cancel()
		if saveErr := store.SaveTranscriptWithOptions(saveCtx, sessionID, cwd, fullMessages, session.SaveTranscriptOptions{
			AdditionalDirectories:    opts.AdditionalDirectories,
			AdditionalDirectoriesSet: opts.AdditionalDirectoriesSet,
		}); saveErr != nil {
			if err != nil {
				return TurnResult{}, fmt.Errorf("%w; incremental persistence failed: %v; snapshot fallback failed: %v", err, persistErr, saveErr)
			}
			return TurnResult{}, fmt.Errorf("incremental persistence failed: %v; snapshot fallback failed: %w", persistErr, saveErr)
		}
	}
	if err != nil {
		return TurnResult{}, err
	}
	r.maybeEnqueueMemoryExtract(ctx, cfg.Session, sessionInfo, sessionID, fullMessages, configuredMemoryModel(cfg, selectedModel.Value), memoryExtractTriggerOptions{
		Force:         memoryForceExtract,
		ContextWindow: selectedModel.ContextWindow,
	})
	return TurnResult{
		SessionID:     sessionID,
		Content:       content,
		Usage:         latestAssistantUsage(fullMessages),
		ContextWindow: selectedModel.ContextWindow,
	}, nil
}

// appendNow persists one newly appended transcript message without rewriting history.
func appendNow(store *session.Store, sessionID, cwd string, msg model.Message, opts TurnOptions) error {
	saveCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return store.AppendMessagesWithOptions(saveCtx, sessionID, cwd, []model.Message{msg}, session.SaveTranscriptOptions{
		AdditionalDirectories:    opts.AdditionalDirectories,
		AdditionalDirectoriesSet: opts.AdditionalDirectoriesSet,
	})
}

// directShellCommand parses direct shell command input starting with !.
func directShellCommand(promptText string) (string, bool) {
	trimmed := strings.TrimSpace(promptText)
	if !strings.HasPrefix(trimmed, "!") {
		return "", false
	}
	command := strings.TrimSpace(strings.TrimPrefix(trimmed, "!"))
	return command, true
}

// runDirectShellTurn skips the model call, directly executes a shell command, and saves it as a complete turn.
func runDirectShellTurn(ctx context.Context, store *session.Store, sessionID, cwd string, existing []model.Message, promptText, command string, observer agent.Observer, runner ToolRunner, saveOpts session.SaveTranscriptOptions) (TurnResult, []model.Message, error) {
	if command == "" {
		return TurnResult{}, nil, fmt.Errorf("shell command is required after !")
	}
	call, err := directShellToolCall(fmt.Sprintf("direct_shell_%d", len(existing)+1), command, cwd)
	if err != nil {
		return TurnResult{}, nil, err
	}

	emit(observer, agent.Event{Type: agent.EventTurnStarted})
	emit(observer, agent.Event{Type: agent.EventToolStarted, Step: 1, ToolCall: call})
	runDefault := func(ctx context.Context, call model.ToolCall) (tool.RunResult, error) {
		content, err := (tool.RunShell{}).Run(ctx, call.Arguments)
		return tool.RunResult{Content: content}, err
	}
	var result tool.RunResult
	var runErr error
	if runner != nil {
		result, runErr = runner(ctx, call, runDefault)
	} else {
		result, runErr = runDefault(ctx, call)
	}
	if ctx.Err() != nil {
		return TurnResult{}, nil, ctx.Err()
	}
	if runErr != nil && strings.TrimSpace(result.Content) == "" {
		result.Content = runErr.Error()
	}
	toolError := runErr != nil
	emit(observer, agent.Event{
		Type:         agent.EventToolFinished,
		Step:         1,
		ToolCall:     call,
		ToolResult:   result.Content,
		ToolMetadata: result.Metadata,
		ToolError:    toolError,
		Err:          runErr,
	})
	emit(observer, agent.Event{Type: agent.EventTurnFinished, Step: 1, Content: result.Content, Err: runErr})

	newMessages := []model.Message{
		model.TextMessage(model.RoleUser, strings.TrimSpace(promptText)),
		model.Message{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				call,
			},
		},
		model.Message{Role: model.RoleTool, Content: result.Content, ToolCallID: call.ID, ToolMetadata: result.Metadata},
	}
	messages := append(append([]model.Message(nil), existing...), newMessages...)
	if err := store.AppendMessagesWithOptions(ctx, sessionID, cwd, newMessages, saveOpts); err != nil {
		return TurnResult{}, nil, err
	}
	return TurnResult{
		SessionID: sessionID,
		Content:   result.Content,
	}, messages, nil
}

// turnContentParts returns the structured content for the current turn's input.
func turnContentParts(opts TurnOptions) []model.ContentPart {
	if len(opts.Parts) > 0 {
		parts := make([]model.ContentPart, 0, len(opts.Parts))
		for _, part := range opts.Parts {
			if part.Type == "" {
				part.Type = model.ContentPartText
			}
			if part.Type == model.ContentPartImage && part.Detail == "" {
				part.Detail = model.ImageDetailAuto
			}
			parts = append(parts, part)
		}
		return parts
	}
	if strings.TrimSpace(opts.Prompt) == "" {
		return nil
	}
	return []model.ContentPart{{Type: model.ContentPartText, Text: opts.Prompt}}
}

// hasImageParts determines whether the input contains image segments.
func hasImageParts(parts []model.ContentPart) bool {
	for _, part := range parts {
		if part.Type == model.ContentPartImage {
			return true
		}
	}
	return false
}

// stripImageParts removes all image segments from the message list, keeping only text segments.
// Used to filter images from historical messages when the current model does not support image input.
func stripImageParts(messages []model.Message) []model.Message {
	filtered := make([]model.Message, len(messages))
	for i, msg := range messages {
		if len(msg.Parts) == 0 {
			filtered[i] = msg
			continue
		}
		textOnly := make([]model.ContentPart, 0, len(msg.Parts))
		for _, part := range msg.Parts {
			if part.Type != model.ContentPartImage {
				textOnly = append(textOnly, part)
			}
		}
		filtered[i] = msg
		filtered[i].Parts = textOnly
	}
	return filtered
}

// directShellToolCall constructs a run_shell call for observer and history replay.
func directShellToolCall(id, command, cwd string) (model.ToolCall, error) {
	args := map[string]string{
		"command": command,
	}
	if cwd != "" {
		args["cwd"] = cwd
	}
	content, err := json.Marshal(args)
	if err != nil {
		return model.ToolCall{}, err
	}
	return model.ToolCall{
		ID:        id,
		Name:      "run_shell",
		Arguments: string(content),
	}, nil
}

// emit sends an event when an observer is present.
func emit(observer agent.Observer, event agent.Event) {
	if observer != nil {
		observer(event)
	}
}

// CompactSession manually compacts the active context of the specified session.
func (r *Runtime) CompactSession(ctx context.Context, opts CompactOptions) (CompactResult, error) {
	r.dbWG.Add(1)
	defer r.dbWG.Done()

	if err := session.ValidateID(opts.SessionID); err != nil {
		return CompactResult{}, err
	}
	cfg, err := r.deps.LoadConfig()
	if err != nil {
		return CompactResult{}, err
	}
	activeProvider, selectedModel, err := cfg.ResolveModel(opts.Model)
	if err != nil {
		return CompactResult{}, err
	}
	if _, err := selectedReasoningEffort(opts.ReasoningEffort, opts.ReasoningEffortSet, selectedModel); err != nil {
		return CompactResult{}, err
	}
	provider, err := r.deps.NewProvider(activeProvider, selectedModel)
	if err != nil {
		return CompactResult{}, err
	}
	provider = withSessionID(provider, opts.SessionID)
	store, err := r.openSessionStore(ctx, cfg.Session)
	if err != nil {
		return CompactResult{}, err
	}

	info, err := store.GetSession(ctx, opts.SessionID)
	if err != nil {
		return CompactResult{}, err
	}
	if opts.CWD != "" && info.CWD != opts.CWD {
		return CompactResult{}, fmt.Errorf("session %q cwd mismatch: %s", opts.SessionID, info.CWD)
	}
	trans, err := store.LoadTranscript(ctx, opts.SessionID)
	if err != nil {
		return CompactResult{}, err
	}
	return r.compactLoadedSession(ctx, store, provider, cfg, selectedModel, opts.SessionID, info, trans.Messages(), opts.Instruction, opts.ReasoningEffort, opts.ReasoningEffortSet, true)
}

// compactLoadedSession performs a summary compaction on a fully loaded transcript.
func (r *Runtime) compactLoadedSession(ctx context.Context, store *session.Store, provider model.Provider, cfg config.Config, selectedModel config.ProviderModel, sessionID string, info session.Session, messages []model.Message, instruction string, reasoningEffort string, reasoningEffortSet bool, manual bool) (CompactResult, error) {
	keepRecentTokens := autoKeepRecentTokens(selectedModel.ContextWindow, cfg.Agent.CompactionTriggerRatio)
	plan, ok := compact.SelectPlan(messages, info.CompactedMessageCount, keepRecentTokens)
	if manual {
		plan, ok = compact.SelectManualPlan(messages, info.CompactedMessageCount)
	}
	if !ok {
		return CompactResult{
			SessionID:     sessionID,
			ContextWindow: selectedModel.ContextWindow,
			Reason:        "no safe compaction boundary",
		}, nil
	}
	start := info.CompactedMessageCount
	if start < 0 {
		start = 0
	}
	if start > len(messages) {
		start = len(messages)
	}
	selectedReasoning, err := selectedReasoningEffort(reasoningEffort, reasoningEffortSet, selectedModel)
	if err != nil {
		return CompactResult{}, err
	}
	resp, err := provider.Stream(ctx, model.ChatRequest{
		Messages:        compact.BuildSummaryMessages(info.ContextSummary, messages[start:plan.CompactCount], instruction),
		MaxTokens:       summaryMaxTokens(selectedModel.MaxTokens),
		Temperature:     cfg.Agent.Temperature,
		ReasoningEffort: selectedReasoning,
	}, nil)
	if err != nil {
		return CompactResult{}, err
	}
	summary := strings.TrimSpace(resp.Content)
	if summary == "" {
		return CompactResult{}, fmt.Errorf("compaction summary is empty")
	}
	if err := store.SaveCompaction(ctx, sessionID, summary, plan.CompactCount, plan.TokensBefore); err != nil {
		return CompactResult{}, err
	}
	info.ContextSummary = summary
	info.CompactedMessageCount = plan.CompactCount
	info.CompactedInputTokens = plan.TokensBefore
	r.maybeEnqueueMemoryExtract(ctx, cfg.Session, info, sessionID, messages, configuredMemoryModel(cfg, selectedModel.Value), memoryExtractTriggerOptions{
		Force:         true,
		ContextWindow: selectedModel.ContextWindow,
	})
	return CompactResult{
		SessionID:     sessionID,
		Compacted:     true,
		CompactCount:  plan.CompactCount,
		KeepCount:     plan.KeepCount,
		TokensBefore:  plan.TokensBefore,
		TokensAfter:   plan.TokensAfter + compact.EstimateMessage(compact.SummaryMessage(summary)),
		ContextWindow: selectedModel.ContextWindow,
		Summary:       summary,
	}, nil
}

// shouldAutoCompact determines whether auto-compaction is needed after appending the current user input.
func shouldAutoCompact(info session.Session, messages, contextMessages []model.Message, parts []model.ContentPart, contextWindow int, triggerRatio float64) bool {
	inputTokens := info.LastInputTokens
	if inputTokens <= 0 {
		active := compact.BuildActiveMessages(info.ContextSummary, info.CompactedMessageCount, messages)
		inputTokens = compact.EstimateMessages(active)
	}
	inputTokens += compact.EstimateMessages(contextMessages)
	inputTokens += compact.EstimateMessage(model.Message{Role: model.RoleUser, Content: model.TextFromParts(parts), Parts: parts})
	return compact.ShouldAutoCompact(inputTokens, contextWindow, triggerRatio)
}

// summaryMaxTokens returns the maximum output tokens available for a summary request.
func summaryMaxTokens(maxTokens int) int {
	if maxTokens <= 0 || maxTokens > 4096 {
		return 4096
	}
	return maxTokens
}

// autoKeepRecentTokens returns the token target for retaining recent context during auto-compaction.
func autoKeepRecentTokens(contextWindow int, triggerRatio float64) int {
	if contextWindow <= 0 {
		return compact.DefaultKeepRecentTokens
	}
	budget := int(float64(contextWindow) * triggerRatio)
	if budget <= 0 {
		return compact.DefaultKeepRecentTokens
	}
	return min(budget, compact.DefaultKeepRecentTokens)
}

// latestAssistantUsage returns the token usage recorded by the most recent model reply.
func latestAssistantUsage(messages []model.Message) model.Usage {
	for i := len(messages) - 1; i >= 0; i-- {
		msg := messages[i]
		if msg.Role == model.RoleAssistant && (msg.Usage.InputTokens != 0 || msg.Usage.OutputTokens != 0 || msg.Usage.TotalTokens != 0) {
			return msg.Usage
		}
	}
	return model.Usage{}
}

// isSessionNotFound determines whether an error indicates the session was not found.
func isSessionNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not found")
}

// ModelOptions returns the selectable models from the current configuration file.
func (r *Runtime) ModelOptions(context.Context) (ModelOptions, error) {
	cfg, err := r.deps.LoadConfig()
	if err != nil {
		return ModelOptions{}, err
	}
	allModels := cfg.AllModels()
	options := make([]ModelOption, 0, len(allModels))
	for _, m := range allModels {
		reasoningEfforts := make([]ReasoningEffortOption, 0, len(m.Model.ReasoningEfforts))
		for _, effort := range m.Model.ReasoningEfforts {
			reasoningEfforts = append(reasoningEfforts, ReasoningEffortOption{
				Value:       effort.Value,
				Name:        effort.Name,
				Description: effort.Description,
			})
		}
		options = append(options, ModelOption{
			Value:            m.CompoundValue(),
			Name:             m.Model.Name,
			Description:      m.Model.Description,
			ContextWindow:    m.Model.ContextWindow,
			MaxTokens:        m.Model.MaxTokens,
			InputFormats:     append([]string(nil), m.Model.InputFormats...),
			ReasoningEfforts: reasoningEfforts,
		})
	}
	defaultProvider, defaultModel, err := cfg.ResolveModel(cfg.DefaultModel)
	if err != nil {
		return ModelOptions{}, err
	}
	return ModelOptions{
		Default: defaultProvider.Name + "/" + defaultModel.Value,
		Models:  options,
	}, nil
}

// SkillSummaries returns model-invocable skill summaries available in the current working directory.
func (r *Runtime) SkillSummaries(_ context.Context, cwd string) ([]SkillSummary, error) {
	if cwd == "" {
		var err error
		cwd, err = r.deps.Getwd()
		if err != nil {
			return nil, err
		}
	}
	catalog, err := r.deps.LoadSkills(cwd)
	if err != nil {
		return nil, err
	}
	summaries := catalog.Summaries()
	result := make([]SkillSummary, 0, len(summaries))
	for _, summary := range summaries {
		result = append(result, SkillSummary{
			Name:        summary.Name,
			Description: summary.Description,
		})
	}
	return result, nil
}

// Doctor runs offline configuration and local environment diagnostics.
func (r *Runtime) Doctor(ctx context.Context) DoctorReport {
	var report DoctorReport
	configPath, pathErr := r.deps.ConfigPath()
	if pathErr != nil {
		report.add("config", DoctorStatusFail, pathErr.Error())
		return report
	}

	cfg, err := r.deps.LoadConfig()
	if err != nil {
		report.add("config", DoctorStatusFail, fmt.Sprintf("%s: %v", configPath, err))
		return report
	}
	report.add("config", DoctorStatusOK, configPath)
	for _, provider := range cfg.Providers {
		report.add("provider", DoctorStatusOK, fmt.Sprintf("%s, %s, %s, %d models", provider.Name, providerFormat(provider), provider.BaseURL, len(provider.Models)))
	}
	report.add("agent", DoctorStatusOK, fmt.Sprintf("max_steps %d, temperature %.2f, compaction_trigger_ratio %.2f", cfg.Agent.MaxSteps, cfg.Agent.Temperature, cfg.Agent.CompactionTriggerRatio))
	report.addSession(ctx, cfg.Session)
	report.addMemory(ctx, cfg)
	report.addTavily(cfg.Services.Tavily)
	report.addShell()
	return report
}

// ListSessions returns recently updated local sessions.
func (r *Runtime) ListSessions(ctx context.Context, limit int) ([]session.Session, error) {
	page, err := r.ListSessionsPage(ctx, "", limit)
	return page.Sessions, err
}

// ListSessionsPage returns a paginated list of recently updated local sessions.
func (r *Runtime) ListSessionsPage(ctx context.Context, cursor string, limit int) (session.ListPage, error) {
	cfg, err := r.deps.LoadConfig()
	if err != nil {
		return session.ListPage{}, err
	}
	store, err := r.openSessionStore(ctx, cfg.Session)
	if err != nil {
		return session.ListPage{}, err
	}
	return store.ListSessionsPage(ctx, cursor, limit)
}

// ListSessionsForCWD returns recently updated local sessions under the specified working directory.
func (r *Runtime) ListSessionsForCWD(ctx context.Context, cwd string, limit int) ([]session.Session, error) {
	page, err := r.ListSessionsForCWDPage(ctx, cwd, "", limit)
	return page.Sessions, err
}

// ListSessionsForCWDPage returns a paginated list of recently updated local sessions under the specified working directory.
func (r *Runtime) ListSessionsForCWDPage(ctx context.Context, cwd, cursor string, limit int) (session.ListPage, error) {
	if cwd == "" {
		return r.ListSessionsPage(ctx, cursor, limit)
	}
	cfg, err := r.deps.LoadConfig()
	if err != nil {
		return session.ListPage{}, err
	}
	store, err := r.openSessionStore(ctx, cfg.Session)
	if err != nil {
		return session.ListPage{}, err
	}
	return store.ListSessionsForCWDPage(ctx, cwd, cursor, limit)
}

// SaveSessionRoots saves the ACP additional working directory roots for an existing session.
func (r *Runtime) SaveSessionRoots(ctx context.Context, sessionID string, additionalDirectories []string) error {
	cfg, err := r.deps.LoadConfig()
	if err != nil {
		return err
	}
	store, err := r.openSessionStore(ctx, cfg.Session)
	if err != nil {
		return err
	}
	return store.SaveSessionRoots(ctx, sessionID, additionalDirectories)
}

// ShowSession returns the metadata and transcript for the specified session.
func (r *Runtime) ShowSession(ctx context.Context, sessionID string) (session.Session, *transcript.Transcript, error) {
	cfg, err := r.deps.LoadConfig()
	if err != nil {
		return session.Session{}, nil, err
	}
	store, err := r.openSessionStore(ctx, cfg.Session)
	if err != nil {
		return session.Session{}, nil, err
	}

	info, err := store.GetSession(ctx, sessionID)
	if err != nil {
		return session.Session{}, nil, err
	}
	trans, err := store.LoadTranscript(ctx, sessionID)
	if err != nil {
		return session.Session{}, nil, err
	}
	return info, trans, nil
}

// DeleteSession deletes the specified local session.
func (r *Runtime) DeleteSession(ctx context.Context, sessionID string) error {
	cfg, err := r.deps.LoadConfig()
	if err != nil {
		return err
	}
	store, err := r.openSessionStore(ctx, cfg.Session)
	if err != nil {
		return err
	}
	if err := store.DeleteSession(ctx, sessionID); err != nil {
		return err
	}
	return nil
}

// DeleteSessionIfExists deletes the specified local session, ignoring non-existent sessions.
func (r *Runtime) DeleteSessionIfExists(ctx context.Context, sessionID string) error {
	err := r.DeleteSession(ctx, sessionID)
	if err != nil && strings.Contains(err.Error(), "not found") {
		return nil
	}
	return err
}

func (r *DoctorReport) add(name string, status DoctorStatus, detail string) {
	r.Checks = append(r.Checks, DoctorCheck{
		Name:   name,
		Status: status,
		Detail: detail,
	})
}

func (r *DoctorReport) addSession(ctx context.Context, cfg config.SessionConfig) {
	dbPath, err := sessionDBPath(cfg)
	if err != nil {
		r.add("session", DoctorStatusFail, err.Error())
		return
	}
	store, err := session.Open(dbPath)
	if err != nil {
		r.add("session", DoctorStatusFail, fmt.Sprintf("%s: %v", dbPath, err))
		return
	}
	defer store.Close()
	if err := store.EnsureSchema(ctx); err != nil {
		r.add("session", DoctorStatusFail, fmt.Sprintf("%s: %v", dbPath, err))
		return
	}
	r.add("session", DoctorStatusOK, dbPath)
}

func (r *DoctorReport) addMemory(ctx context.Context, cfg config.Config) {
	dbPath, err := sessionDBPath(cfg.Session)
	if err != nil {
		r.add("memory", DoctorStatusFail, err.Error())
		return
	}
	store, err := memory.Open(dbPath)
	if err != nil {
		r.add("memory", DoctorStatusFail, fmt.Sprintf("%s: %v", dbPath, err))
		return
	}
	defer store.Close()
	if err := store.EnsureSchema(ctx); err != nil {
		r.add("memory", DoctorStatusFail, fmt.Sprintf("%s: %v", dbPath, err))
		return
	}
	counts, err := store.Counts(ctx)
	if err != nil {
		r.add("memory", DoctorStatusFail, fmt.Sprintf("%s: %v", dbPath, err))
		return
	}
	status := DoctorStatusOK
	if counts.Failed > 0 {
		status = DoctorStatusWarn
	}
	r.add("memory", status, fmt.Sprintf("%d entries, %d pending, %d failed, model %s", counts.Entries, counts.Pending, counts.Failed, displayMemoryModel(cfg.Memory.Model)))
}

func (r *DoctorReport) addTavily(cfg config.TavilyConfig) {
	if cfg.APIKey == "" {
		r.add("tavily", DoctorStatusWarn, "disabled")
		return
	}
	r.add("tavily", DoctorStatusOK, cfg.BaseURL)
}

func (r *DoctorReport) addShell() {
	spec, err := tool.CheckDefaultShell()
	if err != nil {
		r.add("shell", DoctorStatusFail, err.Error())
		return
	}
	r.add("shell", DoctorStatusOK, spec.DisplayName)
}

func completeDependencies(deps Dependencies) Dependencies {
	defaults := DefaultDependencies()
	if deps.LoadConfig == nil {
		deps.LoadConfig = defaults.LoadConfig
	}
	if deps.ConfigPath == nil {
		deps.ConfigPath = defaults.ConfigPath
	}
	if deps.NewProvider == nil {
		deps.NewProvider = defaults.NewProvider
	}
	if deps.Getwd == nil {
		deps.Getwd = defaults.Getwd
	}
	if deps.LoadInstructions == nil {
		deps.LoadInstructions = defaults.LoadInstructions
	}
	if deps.LoadSkills == nil {
		deps.LoadSkills = defaults.LoadSkills
	}
	if deps.NewSessionID == nil {
		deps.NewSessionID = defaults.NewSessionID
	}
	if deps.Now == nil {
		deps.Now = defaults.Now
	}
	return deps
}

func buildToolRegistry(cwd string, skills *skill.Catalog, services config.ServicesConfig, memorySearch tool.MemorySearchFunc) (*tool.Registry, error) {
	tools := []tool.Tool{
		tool.ApplyPatch{CWD: cwd},
		tool.RunShell{CWD: cwd},
		tool.TodoWrite{},
		tool.LoadSkill{Skills: skills},
	}
	if services.Tavily.APIKey != "" {
		client, err := tool.NewTavilyClient(services.Tavily.BaseURL, services.Tavily.APIKey, nil)
		if err != nil {
			return nil, err
		}
		tools = append(tools,
			tool.TavilySearch{Client: client},
			tool.TavilyFetch{Client: client},
		)
	}
	if memorySearch != nil {
		tools = append(tools, tool.MemorySearch{Search: memorySearch})
	}
	return tool.NewRegistry(tools...)
}

func promptSkillSummaries(catalog *skill.Catalog) []prompt.SkillSummary {
	summaries := catalog.Summaries()
	result := make([]prompt.SkillSummary, 0, len(summaries))
	for _, summary := range summaries {
		result = append(result, prompt.SkillSummary{
			Name:        summary.Name,
			Description: summary.Description,
		})
	}
	return result
}

// skillMessages renders explicitly selected skills as model-visible, non-persistent context messages for the current turn.
func skillMessages(names []string, catalog *skill.Catalog) []model.Message {
	if len(names) == 0 || catalog == nil {
		return nil
	}
	messages := make([]model.Message, 0, len(names))
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		found, ok := catalog.Lookup(name)
		if !ok {
			continue
		}
		messages = append(messages, model.TextMessage(model.RoleUser, skillContext(found)))
	}
	return messages
}

// skillContext wraps the full SKILL.md content in XML instruction tags.
func skillContext(found skill.Skill) string {
	return fmt.Sprintf(
		"<skill>\n<name>%s</name>\n<path>%s</path>\n%s\n</skill>",
		found.Name,
		found.Path,
		strings.TrimSpace(found.Content),
	)
}

// openSessionStore returns a session Store backed by the shared *sql.DB.
// cfg.Session provides the DB path, ensuring the caller uses the same config for the entire operation.
// The caller must not call Store.Close; the Runtime owns the connection lifecycle.
func (r *Runtime) openSessionStore(ctx context.Context, cfg config.SessionConfig) (*session.Store, error) {
	dbPath, err := sessionDBPath(cfg)
	if err != nil {
		return nil, err
	}
	db, err := r.openDB(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	store := session.OpenDB(db)
	if err := store.EnsureSchema(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

// openMemoryStore returns a memory Store backed by the shared *sql.DB.
// cfg provides the DB path, ensuring the caller uses the same config for the entire operation.
// The caller must not call Store.Close; the Runtime owns the connection lifecycle.
func (r *Runtime) openMemoryStore(ctx context.Context, cfg config.SessionConfig) (*memory.Store, error) {
	dbPath, err := sessionDBPath(cfg)
	if err != nil {
		return nil, err
	}
	db, err := r.openDB(ctx, dbPath)
	if err != nil {
		return nil, err
	}
	store := memory.OpenDB(db)
	if err := store.EnsureSchema(ctx); err != nil {
		return nil, err
	}
	return store, nil
}

func sessionDBPath(cfg config.SessionConfig) (string, error) {
	if cfg.DBPath == "" {
		return session.DefaultPath()
	}
	if rest, ok := homeRelativePath(cfg.DBPath); ok {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, rest), nil
	}
	return cfg.DBPath, nil
}

// homeRelativePath recognizes ~/ and ~\ path notation in user configuration.
func homeRelativePath(path string) (string, bool) {
	if len(path) < 2 || path[0] != '~' {
		return "", false
	}
	if path[1] != '/' && path[1] != '\\' {
		return "", false
	}
	return path[2:], true
}

func selectedReasoningEffort(override string, overrideSet bool, selectedModel config.ProviderModel) (string, error) {
	if overrideSet {
		if len(selectedModel.ReasoningEfforts) == 0 {
			if override == "" {
				return "", nil
			}
			return "", fmt.Errorf("reasoning effort %q is not supported by model %q", override, selectedModel.Value)
		}
		if selectedModel.SupportsReasoningEffort(override) {
			return override, nil
		}
		return "", fmt.Errorf("reasoning effort %q is not supported by model %q", override, selectedModel.Value)
	}
	if len(selectedModel.ReasoningEfforts) == 0 {
		return "", nil
	}
	return selectedModel.ReasoningEfforts[0].Value, nil
}

func displayMemoryModel(value string) string {
	if strings.TrimSpace(value) == "" {
		return "session model"
	}
	return value
}
