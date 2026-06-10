package runtime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/liuyuxin/atlas/internal/agent"
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
	SessionID string
	Prompt    string
	Model     string
	CWD       string
	Observer  agent.Observer
}

// TurnResult 描述一次用户输入完成后的结果。
type TurnResult struct {
	SessionID string
	Content   string
}

// ModelOption 描述 runtime 对外暴露的可选模型。
type ModelOption struct {
	Value         string
	Name          string
	Description   string
	ContextLength int
}

// ModelOptions 描述当前配置的模型选择状态。
type ModelOptions struct {
	Default string
	Models  []ModelOption
}

// DefaultDependencies 返回真实命令行运行时使用的依赖。
func DefaultDependencies() Dependencies {
	return Dependencies{
		LoadConfig: config.LoadDefault,
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
	selectedModel, err := cfg.Provider.ResolveModel(opts.Model)
	if err != nil {
		return TurnResult{}, err
	}
	provider, err := r.deps.NewProvider(cfg.Provider, selectedModel)
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
	instructions, err := r.deps.LoadInstructions(cwd)
	if err != nil {
		return TurnResult{}, err
	}
	skills, err := r.deps.LoadSkills(cwd)
	if err != nil {
		return TurnResult{}, err
	}
	registry, err := buildToolRegistry(skills)
	if err != nil {
		return TurnResult{}, err
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

	trans := transcript.New()
	if resumeSession {
		trans, err = store.LoadTranscript(ctx, sessionID)
		if err != nil {
			return TurnResult{}, err
		}
	}
	a, err := agent.New(agent.Config{
		Provider:   provider,
		Tools:      registry,
		Transcript: trans,
		System: prompt.BuildSystem(prompt.Options{
			WorkingDir:   cwd,
			Now:          r.deps.Now(),
			Instructions: instructions,
			Skills:       promptSkillSummaries(skills),
		}),
		MaxSteps:    cfg.Agent.MaxSteps,
		Temperature: cfg.Agent.Temperature,
		Observer:    opts.Observer,
	})
	if err != nil {
		return TurnResult{}, err
	}

	content, err := a.RunTurn(ctx, opts.Prompt)
	if err != nil {
		return TurnResult{}, err
	}
	if err := store.SaveTranscript(ctx, sessionID, cwd, trans.Messages()); err != nil {
		return TurnResult{}, err
	}
	return TurnResult{
		SessionID: sessionID,
		Content:   content,
	}, nil
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
			ContextLength: model.ContextLength,
		})
	}
	return ModelOptions{
		Default: cfg.Provider.DefaultModel,
		Models:  options,
	}, nil
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

func completeDependencies(deps Dependencies) Dependencies {
	defaults := DefaultDependencies()
	if deps.LoadConfig == nil {
		deps.LoadConfig = defaults.LoadConfig
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

func buildToolRegistry(skills *skill.Catalog) (*tool.Registry, error) {
	return tool.NewRegistry(
		tool.ListFiles{},
		tool.ReadFile{},
		tool.SearchText{},
		tool.WriteFile{},
		tool.RunShell{},
		tool.LoadSkill{Skills: skills},
	)
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
