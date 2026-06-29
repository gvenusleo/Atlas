package weixin

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

// LoginOptions describes the parameters for the WeChat QR code login flow.
type LoginOptions struct {
	Store  *Store
	Client *Client
	Output io.Writer
	Now    func() time.Time
}

// Login performs a WeChat QR code login and saves the account credentials.
func Login(ctx context.Context, opts LoginOptions) (LoginResult, error) {
	if opts.Store == nil {
		return LoginResult{}, fmt.Errorf("weixin store is required")
	}
	if opts.Client == nil {
		client, err := NewClient(ClientOptions{})
		if err != nil {
			return LoginResult{}, err
		}
		opts.Client = client
	}
	if opts.Output == nil {
		opts.Output = io.Discard
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}

	qrCode, qrURL, err := opts.Client.FetchQRCode(ctx)
	if err != nil {
		return LoginResult{}, err
	}
	fmt.Fprintf(opts.Output, "Scan this Weixin QR code URL:\n%s\n", qrURL)

	deadline := opts.Now().Add(8 * time.Minute)
	apiBaseURL := opts.Client.baseURL
	for opts.Now().Before(deadline) {
		status, err := opts.Client.PollQRStatus(ctx, apiBaseURL, qrCode, "")
		if err != nil {
			return LoginResult{}, err
		}
		switch status.Status {
		case "wait", "":
			if err := ctx.Err(); err != nil {
				return LoginResult{}, err
			}
			time.Sleep(time.Second)
			continue
		case "scaned":
			fmt.Fprintln(opts.Output, "QR code scanned, waiting for confirmation...")
		case "scaned_but_redirect":
			if status.RedirectHost != "" {
				apiBaseURL = normalizeBaseURL(status.RedirectHost)
			}
		case "binded_redirect":
			return LoginResult{AlreadyConnected: true}, nil
		case "confirmed":
			account := Account{
				ID:        status.AccountID,
				Token:     status.BotToken,
				BaseURL:   status.BaseURL,
				UserID:    status.UserID,
				UpdatedAt: opts.Now(),
			}
			if account.ID == "" || account.Token == "" || account.UserID == "" {
				return LoginResult{}, fmt.Errorf("weixin confirmed login response is incomplete")
			}
			if account.BaseURL == "" {
				account.BaseURL = defaultBaseURL
			}
			if err := opts.Store.SaveAccount(account); err != nil {
				return LoginResult{}, err
			}
			return LoginResult{Account: account}, nil
		case "expired":
			return LoginResult{}, fmt.Errorf("weixin qrcode expired")
		case "need_verifycode", "verify_code_blocked":
			return LoginResult{}, fmt.Errorf("weixin login requires verification code, unsupported in Atlas CLI")
		default:
			return LoginResult{}, fmt.Errorf("weixin login status %q: %s", status.Status, status.Error)
		}
	}
	return LoginResult{}, fmt.Errorf("weixin login timed out")
}

// normalizeBaseURL normalizes a WeChat redirect host to an HTTPS base URL.
func normalizeBaseURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultBaseURL
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return strings.TrimRight(value, "/")
	}
	return "https://" + strings.TrimRight(value, "/")
}
