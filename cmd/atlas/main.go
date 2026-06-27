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
	"github.com/liuyuxin/atlas/internal/config"
	"github.com/liuyuxin/atlas/internal/runtime"
	"github.com/liuyuxin/atlas/internal/session"
	"github.com/liuyuxin/atlas/internal/version"
	"github.com/liuyuxin/atlas/internal/weixin"
	"github.com/liuyuxin/atlas/internal/ws"
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
	case "serve":
		return runServeCommand(ctx, args[1:], deps)
	case "doctor":
		return runDoctorCommand(ctx, args[1:], deps)
	case "weixin":
		return runWeixinCommand(ctx, args[1:], deps)
	case "sessions":
		return runSessionsCommand(ctx, args[1:], deps)
	case "session":
		return runSessionCommand(ctx, args[1:], deps)
	case "version":
		return runVersionCommand(args[1:], deps)
	default:
		return errors.New("usage: atlas [--session <id>] | atlas run [--session <id>] [--model <value>] <prompt> | atlas doctor | atlas weixin <login|serve|accounts|logout> | atlas acp | atlas serve | atlas version")
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

// runServeCommand 启动 WebSocket 服务。
func runServeCommand(ctx context.Context, args []string, deps runDependencies) error {
	if len(args) != 0 {
		return errors.New("usage: atlas serve")
	}
	cfg, err := config.LoadDefault()
	if err != nil {
		return err
	}
	srv, err := ws.NewServer(ws.ServerOptions{
		Runtime: deps.runtime,
		Host:    cfg.Services.WS.Host,
		Port:    cfg.Services.WS.Port,
		Output:  deps.stdout,
	})
	if err != nil {
		return err
	}
	return srv.Run(ctx)
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

func runWeixinCommand(ctx context.Context, args []string, deps runDependencies) error {
	if len(args) == 0 {
		return errors.New("usage: atlas weixin <login|serve|accounts|logout>")
	}
	switch args[0] {
	case "login":
		return runWeixinLoginCommand(ctx, args[1:], deps)
	case "serve":
		return runWeixinServeCommand(ctx, args[1:], deps)
	case "accounts":
		return runWeixinAccountsCommand(args[1:], deps)
	case "logout":
		return runWeixinLogoutCommand(args[1:], deps)
	default:
		return errors.New("usage: atlas weixin <login|serve|accounts|logout>")
	}
}

func runWeixinLoginCommand(ctx context.Context, args []string, deps runDependencies) error {
	if len(args) != 0 {
		return errors.New("usage: atlas weixin login")
	}
	cfg, err := config.LoadDefault()
	if err != nil {
		return err
	}
	store, client, err := newWeixinLoginClient(cfg.Services.Weixin)
	if err != nil {
		return err
	}
	result, err := weixin.Login(ctx, weixin.LoginOptions{
		Store:  store,
		Client: client,
		Output: deps.stdout,
	})
	if err != nil {
		return err
	}
	if result.AlreadyConnected {
		fmt.Fprintln(deps.stdout, "weixin account already connected")
		return nil
	}
	fmt.Fprintf(deps.stdout, "logged in weixin account %s\n", result.Account.ID)
	return nil
}

func runWeixinServeCommand(ctx context.Context, args []string, deps runDependencies) error {
	if len(args) != 0 {
		return errors.New("usage: atlas weixin serve")
	}
	store, client, account, weixinConfig, err := newWeixinServeRuntime()
	if err != nil {
		return err
	}
	server, err := weixin.NewServer(weixin.ServerOptions{
		Runtime:    deps.runtime,
		Store:      store,
		Client:     client,
		Account:    account,
		Output:     deps.stdout,
		CDNBaseURL: weixinConfig.CDNBaseURL,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(deps.stdout, "weixin serving account %s\n", account.ID)
	return server.Run(ctx)
}

func runWeixinAccountsCommand(args []string, deps runDependencies) error {
	if len(args) != 0 {
		return errors.New("usage: atlas weixin accounts")
	}
	store, err := weixin.NewStore("")
	if err != nil {
		return err
	}
	accounts, err := store.ListAccounts()
	if err != nil {
		return err
	}
	if len(accounts) == 0 {
		fmt.Fprintln(deps.stdout, "no weixin accounts")
		return nil
	}
	fmt.Fprintln(deps.stdout, "ID\tUSER\tUPDATED")
	for _, account := range accounts {
		fmt.Fprintf(deps.stdout, "%s\t%s\t%s\n", account.ID, account.UserID, account.UpdatedAt.Format(time.RFC3339))
	}
	return nil
}

func runWeixinLogoutCommand(args []string, deps runDependencies) error {
	if len(args) != 1 {
		return errors.New("usage: atlas weixin logout <account-id>")
	}
	store, err := weixin.NewStore("")
	if err != nil {
		return err
	}
	if err := store.DeleteAccount(args[0]); err != nil {
		return err
	}
	fmt.Fprintf(deps.stdout, "logged out weixin account %s\n", args[0])
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

func newWeixinLoginClient(cfg config.WeixinConfig) (*weixin.Store, *weixin.Client, error) {
	store, err := weixin.NewStore("")
	if err != nil {
		return nil, nil, err
	}
	client, err := weixin.NewClient(weixin.ClientOptions{
		BaseURL: cfg.BaseURL,
	})
	if err != nil {
		return nil, nil, err
	}
	return store, client, nil
}

func newWeixinServeRuntime() (*weixin.Store, *weixin.Client, weixin.Account, config.WeixinConfig, error) {
	cfg, err := config.LoadDefault()
	if err != nil {
		return nil, nil, weixin.Account{}, config.WeixinConfig{}, err
	}
	store, err := weixin.NewStore("")
	if err != nil {
		return nil, nil, weixin.Account{}, config.WeixinConfig{}, err
	}
	account, err := store.LoadAccount("")
	if err != nil {
		return nil, nil, weixin.Account{}, config.WeixinConfig{}, err
	}
	client, err := weixin.NewClient(weixin.ClientOptions{
		BaseURL: account.BaseURL,
		Token:   account.Token,
	})
	if err != nil {
		return nil, nil, weixin.Account{}, config.WeixinConfig{}, err
	}
	return store, client, account, cfg.Services.Weixin, nil
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
		return errors.New("usage: atlas session <show|delete|compact> <id>")
	}
	switch args[0] {
	case "show":
		return runSessionShowCommand(ctx, args[1:], deps)
	case "delete":
		return runSessionDeleteCommand(ctx, args[1:], deps)
	case "compact":
		return runSessionCompactCommand(ctx, args[1:], deps)
	default:
		return errors.New("usage: atlas session <show|delete|compact> <id>")
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

func runSessionCompactCommand(ctx context.Context, args []string, deps runDependencies) error {
	if len(args) != 1 {
		return errors.New("usage: atlas session compact <id>")
	}
	result, err := deps.runtime.CompactSession(ctx, runtime.CompactOptions{SessionID: args[0]})
	if err != nil {
		return err
	}
	if !result.Compacted {
		fmt.Fprintf(deps.stdout, "session %s not compacted: %s\n", args[0], result.Reason)
		return nil
	}
	fmt.Fprintf(deps.stdout, "compacted session %s: compacted %d messages, kept %d messages\n", args[0], result.CompactCount, result.KeepCount)
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
	streamedContent := false
	return func(event agent.Event) {
		switch event.Type {
		case agent.EventModelDelta:
			fmt.Fprint(out, event.Content)
			needsLineBreak = !strings.HasSuffix(event.Content, "\n")
			streamedContent = true
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
			if event.Content != "" {
				if streamedContent {
					if needsLineBreak {
						fmt.Fprintln(out)
						needsLineBreak = false
					}
					return
				}
				fmt.Fprint(out, event.Content)
				if !strings.HasSuffix(event.Content, "\n") {
					fmt.Fprintln(out)
				}
				needsLineBreak = false
			}
		}
	}
}
