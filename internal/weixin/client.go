package weixin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/liuyuxin/atlas/internal/version"
)

const (
	loginPollTimeout = 35 * time.Second
	apiTimeout       = 15 * time.Second
	configTimeout    = 10 * time.Second
)

// Client calls the WeChat iLink Bot HTTP API.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	botAgent   string
}

// ClientOptions describes the parameters for creating a WeChat API client.
type ClientOptions struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// NewClient creates a WeChat API client.
func NewClient(opts ClientOptions) (*Client, error) {
	baseURL := strings.TrimSpace(opts.BaseURL)
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return nil, fmt.Errorf("weixin base url is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("weixin base url is invalid")
	}
	if opts.HTTPClient == nil {
		opts.HTTPClient = http.DefaultClient
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      strings.TrimSpace(opts.Token),
		httpClient: opts.HTTPClient,
		botAgent:   "Atlas/" + version.Current,
	}, nil
}

// FetchQRCode fetches the login QR code.
func (c *Client) FetchQRCode(ctx context.Context) (string, string, error) {
	var resp qrCodeResponse
	if err := c.post(ctx, c.baseURL, "ilink/bot/get_bot_qrcode?bot_type="+url.QueryEscape(defaultBotType), map[string]any{
		"local_token_list": []string{},
	}, &resp, apiTimeout); err != nil {
		return "", "", err
	}
	if err := apiError("fetch qrcode", resp.ReturnCode, 0, resp.Error); err != nil {
		return "", "", err
	}
	if resp.QRCode == "" || resp.QRCodeURL == "" {
		return "", "", fmt.Errorf("weixin qrcode response is incomplete")
	}
	return resp.QRCode, resp.QRCodeURL, nil
}

// PollQRStatus waits for a single QR code scan status.
func (c *Client) PollQRStatus(ctx context.Context, apiBaseURL, qrCode, verifyCode string) (qrStatusResponse, error) {
	endpoint := "ilink/bot/get_qrcode_status?qrcode=" + url.QueryEscape(qrCode)
	if verifyCode != "" {
		endpoint += "&verify_code=" + url.QueryEscape(verifyCode)
	}
	var resp qrStatusResponse
	err := c.get(ctx, apiBaseURL, endpoint, &resp, loginPollTimeout)
	return resp, err
}

// GetUpdates long-polls for new messages.
func (c *Client) GetUpdates(ctx context.Context, buf string, timeout time.Duration) (getUpdatesResponse, error) {
	if timeout <= 0 {
		timeout = loginPollTimeout
	}
	var resp getUpdatesResponse
	err := c.post(ctx, c.baseURL, "ilink/bot/getupdates", getUpdatesRequest{
		GetUpdatesBuf: buf,
		BaseInfo:      c.baseInfo(),
	}, &resp, timeout+5*time.Second)
	if err == nil {
		err = apiError("getupdates", resp.ReturnCode, resp.ErrorCode, resp.Error)
	}
	return resp, err
}

// GetConfig reads the account configuration, including the typing ticket.
func (c *Client) GetConfig(ctx context.Context, userID, contextToken string) (string, error) {
	var resp getConfigResponse
	if err := c.post(ctx, c.baseURL, "ilink/bot/getconfig", getConfigRequest{
		UserID:       userID,
		ContextToken: contextToken,
		BaseInfo:     c.baseInfo(),
	}, &resp, configTimeout); err != nil {
		return "", err
	}
	if err := apiError("getconfig", resp.ReturnCode, 0, resp.Error); err != nil {
		return "", err
	}
	return resp.TypingTicket, nil
}

// SendTyping sends or cancels the typing indicator.
func (c *Client) SendTyping(ctx context.Context, userID, ticket string, typing bool) error {
	status := typingStatusCancel
	if typing {
		status = typingStatusTyping
	}
	var resp apiStatus
	if err := c.post(ctx, c.baseURL, "ilink/bot/sendtyping", sendTypingRequest{
		UserID:       userID,
		TypingTicket: ticket,
		Status:       status,
		BaseInfo:     c.baseInfo(),
	}, &resp, configTimeout); err != nil {
		return err
	}
	return apiError("sendtyping", resp.ReturnCode, resp.ErrorCode, resp.Error)
}

// SendText sends a text reply.
func (c *Client) SendText(ctx context.Context, to, text, contextToken, runID string) error {
	if strings.TrimSpace(to) == "" {
		return fmt.Errorf("weixin message recipient is required")
	}
	if text == "" {
		text = " "
	}
	var resp apiStatus
	if err := c.post(ctx, c.baseURL, "ilink/bot/sendmessage", sendMessageRequest{
		Message: WeixinMessage{
			ToUserID:     to,
			ClientID:     newClientID(),
			MessageType:  messageTypeBot,
			MessageState: messageStateFinish,
			Items: []MessageItem{{
				Type:     messageItemTypeText,
				TextItem: &TextItem{Text: text},
			}},
			ContextToken: contextToken,
			RunID:        runID,
		},
		BaseInfo: c.baseInfo(),
	}, &resp, apiTimeout); err != nil {
		return err
	}
	return apiError("sendmessage", resp.ReturnCode, resp.ErrorCode, resp.Error)
}

// get sends a WeChat API GET request and decodes the response.
func (c *Client) get(ctx context.Context, baseURL, endpoint string, out any, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, joinEndpoint(baseURL, endpoint), nil)
	if err != nil {
		return err
	}
	for key, value := range c.headers(false) {
		req.Header.Set(key, value)
	}
	return c.do(req, out)
}

// post sends a WeChat API POST request and decodes the response.
func (c *Client) post(ctx context.Context, baseURL, endpoint string, body any, out any, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, joinEndpoint(baseURL, endpoint), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	for key, value := range c.headers(true) {
		req.Header.Set(key, value)
	}
	return c.do(req, out)
}

// do executes an HTTP request and converts non-2xx responses to errors.
func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("weixin api %s failed: status %d: %s", req.URL.Path, resp.StatusCode, strings.TrimSpace(string(content)))
	}
	if out == nil {
		return nil
	}
	if len(bytes.TrimSpace(content)) == 0 {
		return nil
	}
	if err := json.Unmarshal(content, out); err != nil {
		return fmt.Errorf("decode weixin api response: %w", err)
	}
	return nil
}

// headers returns the common request headers needed by the iLink Bot API.
func (c *Client) headers(jsonContent bool) map[string]string {
	headers := map[string]string{
		"AuthorizationType":       "ilink_bot_token",
		"X-WECHAT-UIN":            randomWechatUIN(),
		"iLink-App-Id":            "bot",
		"iLink-App-ClientVersion": clientVersion(),
		"User-Agent":              "atlas/" + version.Current,
	}
	if jsonContent {
		headers["Content-Type"] = "application/json"
	}
	if c.token != "" {
		headers["Authorization"] = "Bearer " + c.token
	}
	return headers
}

// baseInfo returns the client identification info in the WeChat API request body.
func (c *Client) baseInfo() baseInfo {
	return baseInfo{
		ChannelVersion: version.Current,
		BotAgent:       c.botAgent,
	}
}

// joinEndpoint joins a base URL and a relative endpoint path into a full URL.
func joinEndpoint(baseURL, endpoint string) string {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil {
		return strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(endpoint, "/")
	}
	if strings.Contains(endpoint, "?") {
		parts := strings.SplitN(endpoint, "?", 2)
		u.Path = path.Join(u.Path, strings.TrimLeft(parts[0], "/"))
		u.RawQuery = parts[1]
		return u.String()
	}
	u.Path = path.Join(u.Path, strings.TrimLeft(endpoint, "/"))
	return u.String()
}

// randomWechatUIN generates a random X-WECHAT-UIN required by the WeChat API.
func randomWechatUIN() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return base64.StdEncoding.EncodeToString([]byte("0"))
	}
	n := binary.BigEndian.Uint32(b[:])
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%d", n)))
}

// clientVersion converts the Atlas version number to an iLink client version integer.
func clientVersion() string {
	parts := strings.Split(version.Current, ".")
	values := []int{0, 0, 0}
	for i := 0; i < len(parts) && i < len(values); i++ {
		var value int
		fmt.Sscanf(parts[i], "%d", &value)
		values[i] = value & 0xff
	}
	return fmt.Sprintf("%d", values[0]<<16|values[1]<<8|values[2])
}

// newClientID generates a client message ID used when sending messages.
func newClientID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("atlas-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("atlas-%x", b[:])
}

// apiError converts a WeChat API JSON business error code to a Go error.
func apiError(operation string, ret int, errcode int, errmsg string) error {
	if ret == 0 && errcode == 0 {
		return nil
	}
	if strings.TrimSpace(errmsg) == "" {
		errmsg = "unknown error"
	}
	if errcode != 0 {
		return fmt.Errorf("weixin %s failed: errcode %d: %s", operation, errcode, errmsg)
	}
	return fmt.Errorf("weixin %s failed: ret %d: %s", operation, ret, errmsg)
}
