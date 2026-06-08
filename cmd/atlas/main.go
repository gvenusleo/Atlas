package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
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
	"github.com/liuyuxin/atlas/internal/tool"
	"github.com/liuyuxin/atlas/internal/transcript"
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
		newSessionID: session.NewID,
		now:          time.Now,
		stdout:       os.Stdout,
	})
}

type runDependencies struct {
	loadConfig       func() (config.Config, error)
	newProvider      func(config.ProviderConfig) (model.Provider, error)
	getwd            func() (string, error)
	loadInstructions func(string) ([]prompt.InstructionFile, error)
	newSessionID     func(time.Time) (string, error)
	now              func() time.Time
	stdout           io.Writer
}

func runWithDependencies(ctx context.Context, args []string, deps runDependencies) error {
	if len(args) > 0 {
		switch args[0] {
		case "sessions":
			return runSessionsCommand(ctx, args[1:], deps)
		case "session":
			return runSessionCommand(ctx, args[1:], deps)
		}
	}
	return runPrompt(ctx, args, deps)
}

func runPrompt(ctx context.Context, args []string, deps runDependencies) error {
	parsed, err := parseArgs(args)
	if err != nil {
		return err
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
	sessionID := parsed.sessionID
	resumeSession := sessionID != ""
	if sessionID == "" {
		sessionID, err = deps.newSessionID(deps.now())
		if err != nil {
			return err
		}
	}
	if err := session.ValidateID(sessionID); err != nil {
		return err
	}
	dbPath, err := sessionDBPath(cfg.Session)
	if err != nil {
		return err
	}
	sessionStore, err := session.Open(dbPath)
	if err != nil {
		return err
	}
	defer sessionStore.Close()
	if err := sessionStore.EnsureSchema(ctx); err != nil {
		return err
	}
	trans := transcript.New()
	if resumeSession {
		trans, err = sessionStore.LoadTranscript(ctx, sessionID)
		if err != nil {
			return err
		}
	}
	a, err := agent.New(agent.Config{
		Provider:   provider,
		Tools:      registry,
		Transcript: trans,
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

	if _, err := a.RunTurn(ctx, parsed.prompt); err != nil {
		return err
	}
	if err := sessionStore.SaveTranscript(ctx, sessionID, cwd, trans.Messages()); err != nil {
		return err
	}
	fmt.Fprintf(deps.stdout, "[session] %s\n", sessionID)
	return nil
}

func runSessionsCommand(ctx context.Context, args []string, deps runDependencies) error {
	flags := flag.NewFlagSet("atlas sessions", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	limit := flags.Int("limit", 20, "session limit")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("usage: atlas sessions [--limit <n>]")
	}

	store, err := openSessionStore(ctx, deps)
	if err != nil {
		return err
	}
	defer store.Close()

	sessions, err := store.ListSessions(ctx, *limit)
	if err != nil {
		return err
	}
	if len(sessions) == 0 {
		fmt.Fprintln(deps.stdout, "no sessions")
		return nil
	}
	fmt.Fprintln(deps.stdout, "ID\tUPDATED\tTITLE\tCWD")
	for _, session := range sessions {
		fmt.Fprintf(
			deps.stdout,
			"%s\t%s\t%s\t%s\n",
			session.ID,
			session.UpdatedAt.Format(time.RFC3339),
			session.Title,
			session.CWD,
		)
	}
	return nil
}

func runSessionCommand(ctx context.Context, args []string, deps runDependencies) error {
	if len(args) == 0 {
		return errors.New("usage: atlas session <show|delete> <id>")
	}
	switch args[0] {
	case "show":
		return runSessionShowCommand(ctx, args[1:], deps)
	case "delete":
		return runSessionDeleteCommand(ctx, args[1:], deps)
	default:
		return errors.New("usage: atlas session <show|delete> <id>")
	}
}

func runSessionShowCommand(ctx context.Context, args []string, deps runDependencies) error {
	if len(args) != 1 {
		return errors.New("usage: atlas session show <id>")
	}
	store, err := openSessionStore(ctx, deps)
	if err != nil {
		return err
	}
	defer store.Close()

	info, err := store.GetSession(ctx, args[0])
	if err != nil {
		return err
	}
	trans, err := store.LoadTranscript(ctx, args[0])
	if err != nil {
		return err
	}
	fmt.Fprintf(deps.stdout, "id: %s\n", info.ID)
	fmt.Fprintf(deps.stdout, "title: %s\n", info.Title)
	fmt.Fprintf(deps.stdout, "cwd: %s\n", info.CWD)
	fmt.Fprintf(deps.stdout, "created_at: %s\n", info.CreatedAt.Format(time.RFC3339))
	fmt.Fprintf(deps.stdout, "updated_at: %s\n", info.UpdatedAt.Format(time.RFC3339))
	fmt.Fprintln(deps.stdout, "messages:")
	for _, msg := range trans.Messages() {
		fmt.Fprintf(deps.stdout, "[%s] %s\n", msg.Role, msg.Content)
	}
	return nil
}

func runSessionDeleteCommand(ctx context.Context, args []string, deps runDependencies) error {
	if len(args) != 1 {
		return errors.New("usage: atlas session delete <id>")
	}
	store, err := openSessionStore(ctx, deps)
	if err != nil {
		return err
	}
	defer store.Close()

	if err := store.DeleteSession(ctx, args[0]); err != nil {
		return err
	}
	fmt.Fprintf(deps.stdout, "deleted session %s\n", args[0])
	return nil
}

func openSessionStore(ctx context.Context, deps runDependencies) (*session.Store, error) {
	cfg, err := deps.loadConfig()
	if err != nil {
		return nil, err
	}
	dbPath, err := sessionDBPath(cfg.Session)
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

type parsedArgs struct {
	sessionID string
	prompt    string
}

func parseArgs(args []string) (parsedArgs, error) {
	flags := flag.NewFlagSet("atlas", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sessionID := flags.String("session", "", "session id")
	if err := flags.Parse(args); err != nil {
		return parsedArgs{}, err
	}
	promptText := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if promptText == "" {
		return parsedArgs{}, errors.New("usage: atlas [--session <id>] <prompt>")
	}
	return parsedArgs{
		sessionID: *sessionID,
		prompt:    promptText,
	}, nil
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
