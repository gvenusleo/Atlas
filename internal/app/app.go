package app

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/prompt"
	"github.com/liuyuxin/atlas/internal/provider/deepseek"
	"github.com/liuyuxin/atlas/internal/storage"
	"github.com/liuyuxin/atlas/internal/tool"
)

// Config contains process-level Atlas settings.
type Config struct {
	DBPath      string
	Workdir     string
	Model       string
	DeepSeekKey string
}

// App owns the long-lived dependencies for one Atlas process.
type App struct {
	Agent *agent.Agent
	Store storage.Store
}

// New wires the default Atlas runtime.
func New(config Config) (*App, error) {
	dbPath := config.DBPath
	if dbPath == "" {
		dbPath = prompt.DefaultDBPath()
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db directory: %w", err)
	}
	store, err := storage.OpenSQLite(dbPath)
	if err != nil {
		return nil, err
	}

	modelName := config.Model
	if modelName == "" {
		modelName = deepseek.DefaultModel()
	}
	workdir := config.Workdir
	if workdir == "" {
		workdir, _ = os.Getwd()
	}
	provider := deepseek.New(deepseek.Config{APIKey: config.DeepSeekKey})
	runtime := tool.NewRuntime(
		tool.ListFiles{},
		tool.ReadFile{},
		tool.WriteFile{},
		tool.SearchText{},
		tool.RunShell{},
	)
	return &App{
		Agent: agent.New(store, provider, runtime, agent.Config{
			Workdir:  workdir,
			Model:    modelName,
			MaxSteps: 16,
		}),
		Store: store,
	}, nil
}

// Close releases process resources.
func (a *App) Close() error {
	return a.Store.Close()
}
