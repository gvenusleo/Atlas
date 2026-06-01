package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/app"
	"github.com/liuyuxin/atlas/internal/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "atlas:", err)
		os.Exit(1)
	}
}

// run parses flags and dispatches to TUI or one-shot CLI mode.
func run() error {
	var config app.Config
	var prompt string
	var noTUI bool
	var skillRoots multiFlag
	flag.StringVar(&config.DBPath, "db", "", "SQLite database path")
	flag.StringVar(&config.Workdir, "workdir", "", "workspace directory")
	flag.StringVar(&config.Model, "model", "", "DeepSeek model")
	flag.StringVar(&prompt, "prompt", "", "single prompt to run without TUI")
	flag.BoolVar(&noTUI, "no-tui", false, "run without the terminal UI")
	flag.Var(&skillRoots, "skill-root", "additional skills root to scan")
	flag.Parse()
	if prompt == "" && flag.NArg() > 0 {
		prompt = strings.Join(flag.Args(), " ")
	}
	config.SkillRoots = cleanSkillRoots(skillRoots)

	atlas, err := app.New(config)
	if err != nil {
		return err
	}
	defer atlas.Close()

	ctx := context.Background()
	if noTUI || prompt != "" {
		return runOnce(ctx, atlas.Agent, prompt)
	}
	return tui.Run(ctx, atlas.Agent)
}

// runOnce runs a single prompt and prints streamed output.
func runOnce(ctx context.Context, atlas *agent.Agent, prompt string) error {
	if strings.TrimSpace(prompt) == "" {
		return fmt.Errorf("prompt is required when using -no-tui")
	}
	session, err := atlas.CreateSession(ctx, "CLI session")
	if err != nil {
		return err
	}
	events, errs := atlas.RunTurn(ctx, session.ID, prompt)
	for events != nil || errs != nil {
		select {
		case event, ok := <-events:
			if !ok {
				events = nil
				continue
			}
			printEvent(event)
		case err, ok := <-errs:
			if !ok {
				errs = nil
				continue
			}
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// printEvent renders non-TUI event output.
func printEvent(event agent.Event) {
	switch event.Type {
	case agent.EventTextDelta:
		fmt.Print(event.Text)
	case agent.EventToolStarted:
		fmt.Printf("\n[%s]\n", event.ToolName)
	case agent.EventToolFinished:
		if event.Error {
			fmt.Printf("[tool failed]\n%s\n", event.Text)
		}
	case agent.EventTurnFinished:
		fmt.Println()
	}
}

// multiFlag collects repeatable string flags.
type multiFlag []string

func (m *multiFlag) String() string {
	return strings.Join(*m, ",")
}

func (m *multiFlag) Set(value string) error {
	*m = append(*m, value)
	return nil
}

// cleanSkillRoots normalizes explicitly configured skills roots.
func cleanSkillRoots(values []string) []string {
	var roots []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if abs, err := filepath.Abs(value); err == nil {
			value = abs
		}
		roots = append(roots, value)
	}
	return roots
}
