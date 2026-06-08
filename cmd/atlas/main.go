package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/config"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/prompt"
	"github.com/liuyuxin/atlas/internal/provider/openai"
	"github.com/liuyuxin/atlas/internal/tool"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	return runWithDependencies(ctx, args, runDependencies{
		loadConfig: config.LoadDefault,
		newProvider: func(cfg config.ProviderConfig) (model.Provider, error) {
			return openai.New(openai.Config{
				BaseURL: cfg.BaseURL,
				APIKey:  cfg.APIKey,
				Model:   cfg.Model,
			})
		},
		getwd: os.Getwd,
		loadInstructions: func(cwd string) ([]prompt.InstructionFile, error) {
			return prompt.LoadInstructions(cwd)
		},
		now:    time.Now,
		stdout: os.Stdout,
	})
}

type runDependencies struct {
	loadConfig       func() (config.Config, error)
	newProvider      func(config.ProviderConfig) (model.Provider, error)
	getwd            func() (string, error)
	loadInstructions func(string) ([]prompt.InstructionFile, error)
	now              func() time.Time
	stdout           io.Writer
}

func runWithDependencies(ctx context.Context, args []string, deps runDependencies) error {
	promptText := strings.TrimSpace(strings.Join(args, " "))
	if promptText == "" {
		return errors.New("usage: atlas <prompt>")
	}

	cfg, err := deps.loadConfig()
	if err != nil {
		return err
	}
	provider, err := deps.newProvider(cfg.Provider)
	if err != nil {
		return err
	}
	registry, err := tool.NewRegistry(
		tool.ListFiles{},
		tool.ReadFile{},
		tool.SearchText{},
		tool.WriteFile{},
		tool.RunShell{},
	)
	if err != nil {
		return err
	}
	cwd, err := deps.getwd()
	if err != nil {
		return err
	}
	instructions, err := deps.loadInstructions(cwd)
	if err != nil {
		return err
	}
	a, err := agent.New(agent.Config{
		Provider: provider,
		Tools:    registry,
		System: prompt.BuildSystem(prompt.Options{
			WorkingDir:   cwd,
			Now:          deps.now(),
			Instructions: instructions,
		}),
		MaxSteps:    cfg.Agent.MaxSteps,
		Temperature: cfg.Agent.Temperature,
		Observer:    printEvent(deps.stdout),
	})
	if err != nil {
		return err
	}

	_, err = a.RunTurn(ctx, promptText)
	return err
}

func printEvent(out io.Writer) agent.Observer {
	needsLineBreak := false
	return func(event agent.Event) {
		switch event.Type {
		case agent.EventModelDelta:
			fmt.Fprint(out, event.Content)
			needsLineBreak = !strings.HasSuffix(event.Content, "\n")
		case agent.EventToolStarted:
			if needsLineBreak {
				fmt.Fprintln(out)
				needsLineBreak = false
			}
			fmt.Fprintf(out, "[tool] %s\n", event.ToolCall.Name)
		case agent.EventToolFinished:
			if event.ToolError {
				fmt.Fprintf(out, "[tool failed] %s\n", event.ToolCall.Name)
			}
		case agent.EventTurnFinished:
			if event.Content != "" && needsLineBreak {
				fmt.Fprintln(out)
				needsLineBreak = false
			}
		}
	}
}
