package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	atlasacp "github.com/liuyuxin/atlas/internal/acp"
	"github.com/liuyuxin/atlas/internal/agent"
	"github.com/liuyuxin/atlas/internal/runtime"
	"github.com/liuyuxin/atlas/internal/session"
	"github.com/liuyuxin/atlas/internal/version"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	return runWithDependencies(ctx, args, runDependencies{
		runtime: runtime.New(runtime.DefaultDependencies()),
		stdin:   os.Stdin,
		stdout:  os.Stdout,
		runACP:  atlasacp.Run,
	})
}

type runDependencies struct {
	runtime *runtime.Runtime
	stdin   io.Reader
	stdout  io.Writer
	runACP  func(context.Context, atlasacp.Options) error
}

func runWithDependencies(ctx context.Context, args []string, deps runDependencies) error {
	deps = completeRunDependencies(deps)
	if isVersionCommand(args) {
		return runVersionCommand(nil, deps)
	}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return runInteractivePlaceholder(ctx, args, deps)
	}
	switch args[0] {
	case "run":
		return runPrompt(ctx, args[1:], deps)
	case "acp":
		return runACPCommand(ctx, args[1:], deps)
	case "doctor":
		return runDoctorCommand(ctx, args[1:], deps)
	case "sessions":
		return runSessionsCommand(ctx, args[1:], deps)
	case "session":
		return runSessionCommand(ctx, args[1:], deps)
	case "version":
		return runVersionCommand(args[1:], deps)
	default:
		return errors.New("usage: atlas [--session <id>] | atlas run [--session <id>] [--model <value>] <prompt> | atlas doctor | atlas acp | atlas version")
	}
}

func isVersionCommand(args []string) bool {
	return len(args) == 1 && (args[0] == "--version" || args[0] == "-v")
}

func runVersionCommand(args []string, deps runDependencies) error {
	if len(args) != 0 {
		return errors.New("usage: atlas version")
	}
	fmt.Fprintf(deps.stdout, "atlas %s\n", version.Current)
	return nil
}

func runPrompt(ctx context.Context, args []string, deps runDependencies) error {
	parsed, err := parseArgs(args)
	if err != nil {
		return err
	}

	result, err := deps.runtime.RunTurn(ctx, runtime.TurnOptions{
		SessionID: parsed.sessionID,
		Prompt:    parsed.prompt,
		Model:     parsed.model,
		Observer:  printEvent(deps.stdout),
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(deps.stdout, "[session] %s\n", result.SessionID)
	return nil
}

func runACPCommand(ctx context.Context, args []string, deps runDependencies) error {
	if len(args) != 0 {
		return errors.New("usage: atlas acp")
	}
	return deps.runACP(ctx, atlasacp.Options{
		Runtime: deps.runtime,
		Input:   deps.stdin,
		Output:  deps.stdout,
	})
}

func runDoctorCommand(ctx context.Context, args []string, deps runDependencies) error {
	if len(args) != 0 {
		return errors.New("usage: atlas doctor")
	}
	report := deps.runtime.Doctor(ctx)
	for _, check := range report.Checks {
		fmt.Fprintf(deps.stdout, "%s %s: %s\n", doctorStatusLabel(check.Status), check.Name, check.Detail)
	}
	if report.Failed() {
		fmt.Fprintln(deps.stdout, "doctor: failed")
		return errors.New("doctor failed")
	}
	fmt.Fprintln(deps.stdout, "doctor: ok")
	return nil
}

func runInteractivePlaceholder(_ context.Context, args []string, deps runDependencies) error {
	parsed, err := parseInteractiveArgs(args)
	if err != nil {
		return err
	}
	if parsed.sessionID != "" {
		if err := session.ValidateID(parsed.sessionID); err != nil {
			return err
		}
	}
	if parsed.sessionID == "" {
		fmt.Fprintln(deps.stdout, "Atlas interactive mode is not implemented yet. Use `atlas run <prompt>` for now.")
		return nil
	}
	fmt.Fprintf(deps.stdout, "Atlas interactive mode is not implemented yet. Requested session: %s\n", parsed.sessionID)
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

	sessions, err := deps.runtime.ListSessions(ctx, *limit)
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
	info, trans, err := deps.runtime.ShowSession(ctx, args[0])
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
	if err := deps.runtime.DeleteSession(ctx, args[0]); err != nil {
		return err
	}
	fmt.Fprintf(deps.stdout, "deleted session %s\n", args[0])
	return nil
}

type parsedArgs struct {
	sessionID string
	model     string
	prompt    string
}

func parseArgs(args []string) (parsedArgs, error) {
	flags := flag.NewFlagSet("atlas", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sessionID := flags.String("session", "", "session id")
	model := flags.String("model", "", "model value")
	if err := flags.Parse(args); err != nil {
		return parsedArgs{}, err
	}
	promptText := strings.TrimSpace(strings.Join(flags.Args(), " "))
	if promptText == "" {
		return parsedArgs{}, errors.New("usage: atlas run [--session <id>] [--model <value>] <prompt>")
	}
	return parsedArgs{
		sessionID: *sessionID,
		model:     *model,
		prompt:    promptText,
	}, nil
}

func parseInteractiveArgs(args []string) (parsedArgs, error) {
	flags := flag.NewFlagSet("atlas", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	sessionID := flags.String("session", "", "session id")
	if err := flags.Parse(args); err != nil {
		return parsedArgs{}, err
	}
	if flags.NArg() != 0 {
		return parsedArgs{}, errors.New("usage: atlas [--session <id>]")
	}
	return parsedArgs{sessionID: *sessionID}, nil
}

func completeRunDependencies(deps runDependencies) runDependencies {
	if deps.runtime == nil {
		deps.runtime = runtime.New(runtime.DefaultDependencies())
	}
	if deps.stdin == nil {
		deps.stdin = strings.NewReader("")
	}
	if deps.stdout == nil {
		deps.stdout = io.Discard
	}
	if deps.runACP == nil {
		deps.runACP = atlasacp.Run
	}
	return deps
}

func doctorStatusLabel(status runtime.DoctorStatus) string {
	switch status {
	case runtime.DoctorStatusOK:
		return "OK"
	case runtime.DoctorStatusWarn:
		return "WARN"
	case runtime.DoctorStatusFail:
		return "FAIL"
	default:
		return strings.ToUpper(string(status))
	}
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
