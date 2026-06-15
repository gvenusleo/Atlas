package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/compact"
	"github.com/liuyuxin/atlas/internal/config"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/prompt"
	"github.com/liuyuxin/atlas/internal/provider/openai"
	"github.com/liuyuxin/atlas/internal/session"
	"github.com/liuyuxin/atlas/internal/skill"
	"github.com/liuyuxin/atlas/internal/tool"
	"github.com/liuyuxin/atlas/internal/transcript"
)

// Dependencies 定义 Runtime 需要的外部依赖，测试可以替换其中任意一项。
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

// Runtime 是 CLI 和后续交互界面共享的 Atlas 执行入口。
type Runtime struct {
	deps Dependencies
}

// TurnOptions 描述一次用户输入的执行参数。
type TurnOptions struct {
	SessionID       string
	Prompt          string
	Model           string
	ReasoningEffort string
	// ReasoningEffortSet 表示 ReasoningEffort 是调用方显式选择的值，即使它为空。
	ReasoningEffortSet bool
	CWD                string
	Observer           agent.Observer
}

// TurnResult 描述一次用户输入完成后的结果。
type TurnResult struct {
	SessionID string
	Content   string
}

// CompactOptions 描述一次手动上下文压缩请求。
type CompactOptions struct {
	SessionID          string
	Model              string
	ReasoningEffort    string
	ReasoningEffortSet bool
	CWD                string
	Instruction        string
}

// CompactResult 描述上下文压缩完成后的结果。
type CompactResult struct {
	SessionID    string
	Compacted    bool
	CompactCount int
	KeepCount    int
	TokensBefore int
	TokensAfter  int
	Summary      string
	Reason       string
}

// ModelOption 描述 runtime 对外暴露的可选模型。
type ModelOption struct {
	Value         string
	Name          string
	Description   string
	ContextWindow int
	MaxTokens     int
}

// ModelOptions 描述当前配置的模型选择状态。
type ModelOptions struct {
	Default         string
	ReasoningEffort string
	Models          []ModelOption
}

// DoctorStatus 描述一项 doctor 诊断结果的严重程度。
type DoctorStatus string

const (
	// DoctorStatusOK 表示检查通过。
	DoctorStatusOK DoctorStatus = "ok"
	// DoctorStatusWarn 表示能力不可用或配置缺失，但不阻止核心运行。
	DoctorStatusWarn DoctorStatus = "warn"
	// DoctorStatusFail 表示 Atlas 的核心运行前提不满足。
	DoctorStatusFail DoctorStatus = "fail"
)

// DoctorCheck 描述 atlas doctor 输出中的一项诊断。
type DoctorCheck struct {
	Name   string
	Status DoctorStatus
	Detail string
}

// DoctorReport 汇总 atlas doctor 的全部诊断结果。
type DoctorReport struct {
	Checks []DoctorCheck
}

// Failed 返回报告中是否存在失败项。
func (r DoctorReport) Failed() bool {
	for _, check := range r.Checks {
		if check.Status == DoctorStatusFail {
			return true
		}
	}
	return false
}

// DefaultDependencies 返回真实命令行运行时使用的依赖。
func DefaultDependencies() Dependencies {
	return Dependencies{
		LoadConfig: config.LoadDefault,
		ConfigPath: config.DefaultPath,
		NewProvider: func(cfg config.ProviderConfig, selected config.ProviderModel) (model.Provider, error) {
			return openai.New(openai.Config{
				BaseURL: cfg.BaseURL,
				APIKey:  cfg.APIKey,
				Model:   selected.Value,
			})
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

// New 创建 Runtime，并为未指定的依赖填入默认实现。
func New(deps Dependencies) *Runtime {
	return &Runtime{deps: completeDependencies(deps)}
}

// RunTurn 恢复或创建 session，执行一次 agent turn，并保存 transcript。
func (r *Runtime) RunTurn(ctx context.Context, opts TurnOptions) (TurnResult, error) {
	if strings.TrimSpace(opts.Prompt) == "" {
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

	store, err := openSessionStore(ctx, cfg.Session)
	if err != nil {
		return TurnResult{}, err
	}
	defer store.Close()

	shellCommand, isDirectShell := directShellCommand(opts.Prompt)
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
		return runDirectShellTurn(ctx, store, sessionID, cwd, fullTrans.Messages(), opts.Prompt, shellCommand, opts.Observer)
	}

	selectedModel, err := cfg.Provider.ResolveModel(opts.Model)
	if err != nil {
		return TurnResult{}, err
	}
	provider, err := r.deps.NewProvider(cfg.Provider, selectedModel)
	if err != nil {
		return TurnResult{}, err
	}
	instructions, err := r.deps.LoadInstructions(cwd)
	if err != nil {
		return TurnResult{}, err
	}
	skills, err := r.deps.LoadSkills(cwd)
	if err != nil {
		return TurnResult{}, err
	}
	registry, err := buildToolRegistry(skills, cfg.Services)
	if err != nil {
		return TurnResult{}, err
	}
	if resumeSession {
		if shouldAutoCompact(sessionInfo, fullTrans.Messages(), opts.Prompt, selectedModel.ContextWindow, cfg.Agent.CompactionTriggerRatio) {
			result, err := r.compactLoadedSession(ctx, store, provider, cfg, selectedModel, sessionID, sessionInfo, fullTrans.Messages(), "", opts.ReasoningEffort, opts.ReasoningEffortSet, false)
			if err != nil {
				return TurnResult{}, err
			}
			if result.Compacted {
				sessionInfo.ContextSummary = result.Summary
				sessionInfo.CompactedMessageCount = result.CompactCount
				sessionInfo.CompactedInputTokens = result.TokensBefore
			}
		}
	}
	activeMessages := compact.BuildActiveMessages(sessionInfo.ContextSummary, sessionInfo.CompactedMessageCount, fullTrans.Messages())
	trans := transcript.New()
	for _, msg := range activeMessages {
		trans.Append(msg)
	}
	initialActiveLen := len(activeMessages)
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
		ReasoningEffort: selectedReasoningEffort(opts.ReasoningEffort, opts.ReasoningEffortSet, cfg.Agent.ReasoningEffort),
		Observer:        opts.Observer,
	})
	if err != nil {
		return TurnResult{}, err
	}

	content, err := a.RunTurn(ctx, opts.Prompt)
	if err != nil {
		return TurnResult{}, err
	}
	activeAfter := trans.Messages()
	if initialActiveLen > len(activeAfter) {
		initialActiveLen = len(activeAfter)
	}
	fullMessages := append(fullTrans.Messages(), activeAfter[initialActiveLen:]...)
	if err := store.SaveTranscript(ctx, sessionID, cwd, fullMessages); err != nil {
		return TurnResult{}, err
	}
	return TurnResult{
		SessionID: sessionID,
		Content:   content,
	}, nil
}

// directShellCommand 解析以 ! 开头的直接 shell 命令输入。
func directShellCommand(promptText string) (string, bool) {
	trimmed := strings.TrimSpace(promptText)
	if !strings.HasPrefix(trimmed, "!") {
		return "", false
	}
	command := strings.TrimSpace(strings.TrimPrefix(trimmed, "!"))
	return command, true
}

// runDirectShellTurn 跳过模型调用，直接执行 shell 命令并保存为一个完整 turn。
func runDirectShellTurn(ctx context.Context, store *session.Store, sessionID, cwd string, existing []model.Message, promptText, command string, observer agent.Observer) (TurnResult, error) {
	if command == "" {
		return TurnResult{}, fmt.Errorf("shell command is required after !")
	}
	call, err := directShellToolCall(fmt.Sprintf("direct_shell_%d", len(existing)+1), command, cwd)
	if err != nil {
		return TurnResult{}, err
	}

	emit(observer, agent.Event{Type: agent.EventTurnStarted})
	emit(observer, agent.Event{Type: agent.EventToolStarted, Step: 1, ToolCall: call})
	result, runErr := (tool.RunShell{}).Run(ctx, call.Arguments)
	if ctx.Err() != nil {
		return TurnResult{}, ctx.Err()
	}
	if runErr != nil && strings.TrimSpace(result) == "" {
		result = runErr.Error()
	}
	toolError := runErr != nil
	emit(observer, agent.Event{
		Type:       agent.EventToolFinished,
		Step:       1,
		ToolCall:   call,
		ToolResult: result,
		ToolError:  toolError,
		Err:        runErr,
	})
	emit(observer, agent.Event{Type: agent.EventTurnFinished, Step: 1, Content: result, Err: runErr})

	messages := append([]model.Message(nil), existing...)
	messages = append(messages,
		model.Message{Role: model.RoleUser, Content: strings.TrimSpace(promptText)},
		model.Message{
			Role: model.RoleAssistant,
			ToolCalls: []model.ToolCall{
				call,
			},
		},
		model.Message{Role: model.RoleTool, Content: result, ToolCallID: call.ID},
	)
	if err := store.SaveTranscript(ctx, sessionID, cwd, messages); err != nil {
		return TurnResult{}, err
	}
	return TurnResult{
		SessionID: sessionID,
		Content:   result,
	}, nil
}

// directShellToolCall 构造用于 observer 和历史回放的 run_shell 调用。
func directShellToolCall(id, command, cwd string) (model.ToolCall, error) {
	args := map[string]string{
		"command": command,
	}
	if cwd != "" {
		args["workdir"] = cwd
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

// emit 在 observer 存在时发送事件。
func emit(observer agent.Observer, event agent.Event) {
	if observer != nil {
		observer(event)
	}
}

// CompactSession 手动压缩指定 session 的 active context。
func (r *Runtime) CompactSession(ctx context.Context, opts CompactOptions) (CompactResult, error) {
	if err := session.ValidateID(opts.SessionID); err != nil {
		return CompactResult{}, err
	}
	cfg, err := r.deps.LoadConfig()
	if err != nil {
		return CompactResult{}, err
	}
	selectedModel, err := cfg.Provider.ResolveModel(opts.Model)
	if err != nil {
		return CompactResult{}, err
	}
	provider, err := r.deps.NewProvider(cfg.Provider, selectedModel)
	if err != nil {
		return CompactResult{}, err
	}
	store, err := openSessionStore(ctx, cfg.Session)
	if err != nil {
		return CompactResult{}, err
	}
	defer store.Close()

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

// compactLoadedSession 对已加载的完整 transcript 执行一次摘要压缩。
func (r *Runtime) compactLoadedSession(ctx context.Context, store *session.Store, provider model.Provider, cfg config.Config, selectedModel config.ProviderModel, sessionID string, info session.Session, messages []model.Message, instruction string, reasoningEffort string, reasoningEffortSet bool, manual bool) (CompactResult, error) {
	keepRecentTokens := autoKeepRecentTokens(selectedModel.ContextWindow, cfg.Agent.CompactionTriggerRatio)
	plan, ok := compact.SelectPlan(messages, info.CompactedMessageCount, keepRecentTokens)
	if manual {
		plan, ok = compact.SelectManualPlan(messages, info.CompactedMessageCount)
	}
	if !ok {
		return CompactResult{
			SessionID: sessionID,
			Reason:    "no safe compaction boundary",
		}, nil
	}
	start := info.CompactedMessageCount
	if start < 0 {
		start = 0
	}
	if start > len(messages) {
		start = len(messages)
	}
	resp, err := provider.Stream(ctx, model.ChatRequest{
		Messages:        compact.BuildSummaryMessages(info.ContextSummary, messages[start:plan.CompactCount], instruction),
		MaxTokens:       summaryMaxTokens(selectedModel.MaxTokens),
		Temperature:     cfg.Agent.Temperature,
		ReasoningEffort: selectedReasoningEffort(reasoningEffort, reasoningEffortSet, cfg.Agent.ReasoningEffort),
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
	return CompactResult{
		SessionID:    sessionID,
		Compacted:    true,
		CompactCount: plan.CompactCount,
		KeepCount:    plan.KeepCount,
		TokensBefore: plan.TokensBefore,
		TokensAfter:  plan.TokensAfter + compact.EstimateMessage(compact.SummaryMessage(summary)),
		Summary:      summary,
	}, nil
}

// shouldAutoCompact 判断追加当前用户输入后是否需要自动压缩。
func shouldAutoCompact(info session.Session, messages []model.Message, promptText string, contextWindow int, triggerRatio float64) bool {
	inputTokens := info.LastInputTokens
	if inputTokens <= 0 {
		active := compact.BuildActiveMessages(info.ContextSummary, info.CompactedMessageCount, messages)
		inputTokens = compact.EstimateMessages(active)
	}
	inputTokens += compact.EstimateMessage(model.Message{Role: model.RoleUser, Content: promptText})
	return compact.ShouldAutoCompact(inputTokens, contextWindow, triggerRatio)
}

// summaryMaxTokens 返回摘要请求可用的最大输出 token 数。
func summaryMaxTokens(maxTokens int) int {
	if maxTokens <= 0 || maxTokens > 4096 {
		return 4096
	}
	return maxTokens
}

// autoKeepRecentTokens 返回自动压缩时保留最近上下文的 token 目标。
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

// isSessionNotFound 判断错误是否表示 session 不存在。
func isSessionNotFound(err error) bool {
	return err != nil && strings.Contains(err.Error(), "not found")
}

// ModelOptions 返回当前配置文件中可供选择的模型。
func (r *Runtime) ModelOptions(context.Context) (ModelOptions, error) {
	cfg, err := r.deps.LoadConfig()
	if err != nil {
		return ModelOptions{}, err
	}
	models := cfg.Provider.ModelOptions()
	options := make([]ModelOption, 0, len(models))
	for _, model := range models {
		options = append(options, ModelOption{
			Value:         model.Value,
			Name:          model.Name,
			Description:   model.Description,
			ContextWindow: model.ContextWindow,
			MaxTokens:     model.MaxTokens,
		})
	}
	return ModelOptions{
		Default:         cfg.Provider.DefaultModel,
		ReasoningEffort: cfg.Agent.ReasoningEffort,
		Models:          options,
	}, nil
}

// Doctor 运行离线配置和本地运行环境诊断。
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
	report.add("provider", DoctorStatusOK, fmt.Sprintf("%s, default %s, %d models", cfg.Provider.BaseURL, cfg.Provider.DefaultModel, len(cfg.Provider.Models)))
	report.add("agent", DoctorStatusOK, fmt.Sprintf("max_steps %d, temperature %.2f, reasoning_effort %s, compaction_trigger_ratio %.2f", cfg.Agent.MaxSteps, cfg.Agent.Temperature, displayReasoningEffort(cfg.Agent.ReasoningEffort), cfg.Agent.CompactionTriggerRatio))
	report.addSession(ctx, cfg.Session)
	report.addTavily(cfg.Services.Tavily)
	report.addShell()
	return report
}

// ListSessions 返回最近更新的本地会话。
func (r *Runtime) ListSessions(ctx context.Context, limit int) ([]session.Session, error) {
	cfg, err := r.deps.LoadConfig()
	if err != nil {
		return nil, err
	}
	store, err := openSessionStore(ctx, cfg.Session)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	return store.ListSessions(ctx, limit)
}

// ListSessionsForCWD 返回指定工作目录下最近更新的本地会话。
func (r *Runtime) ListSessionsForCWD(ctx context.Context, cwd string, limit int) ([]session.Session, error) {
	if cwd == "" {
		return r.ListSessions(ctx, limit)
	}
	cfg, err := r.deps.LoadConfig()
	if err != nil {
		return nil, err
	}
	store, err := openSessionStore(ctx, cfg.Session)
	if err != nil {
		return nil, err
	}
	defer store.Close()
	return store.ListSessionsForCWD(ctx, cwd, limit)
}

// ShowSession 返回指定会话的元数据和 transcript。
func (r *Runtime) ShowSession(ctx context.Context, sessionID string) (session.Session, *transcript.Transcript, error) {
	cfg, err := r.deps.LoadConfig()
	if err != nil {
		return session.Session{}, nil, err
	}
	store, err := openSessionStore(ctx, cfg.Session)
	if err != nil {
		return session.Session{}, nil, err
	}
	defer store.Close()

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

// DeleteSession 删除指定本地会话。
func (r *Runtime) DeleteSession(ctx context.Context, sessionID string) error {
	cfg, err := r.deps.LoadConfig()
	if err != nil {
		return err
	}
	store, err := openSessionStore(ctx, cfg.Session)
	if err != nil {
		return err
	}
	defer store.Close()
	return store.DeleteSession(ctx, sessionID)
}

// DeleteSessionIfExists 删除指定本地会话，并忽略不存在的会话。
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

func buildToolRegistry(skills *skill.Catalog, services config.ServicesConfig) (*tool.Registry, error) {
	tools := []tool.Tool{
		tool.ListFiles{},
		tool.ReadFile{},
		tool.EditFile{},
		tool.SearchText{},
		tool.WriteFile{},
		tool.RunShell{},
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

func openSessionStore(ctx context.Context, cfg config.SessionConfig) (*session.Store, error) {
	dbPath, err := sessionDBPath(cfg)
	if err != nil {
		return nil, err
	}
	store, err := session.Open(dbPath)
	if err != nil {
		return nil, err
	}
	if err := store.EnsureSchema(ctx); err != nil {
		store.Close()
		return nil, err
	}
	return store, nil
}

func sessionDBPath(cfg config.SessionConfig) (string, error) {
	if cfg.DBPath == "" {
		return session.DefaultPath()
	}
	if strings.HasPrefix(cfg.DBPath, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, cfg.DBPath[2:]), nil
	}
	return cfg.DBPath, nil
}

func selectedReasoningEffort(override string, overrideSet bool, configured string) string {
	if overrideSet {
		return override
	}
	return configured
}

func displayReasoningEffort(value string) string {
	if value == "" {
		return "Default"
	}
	return value
}
