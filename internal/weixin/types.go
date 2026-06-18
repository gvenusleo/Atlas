// Package weixin 提供微信 iLink Bot 通道适配。
package weixin

import "time"

const (
	defaultBaseURL    = "https://ilinkai.weixin.qq.com"
	defaultCDNBaseURL = "https://novac2c.cdn.weixin.qq.com/c2c"
	defaultBotType    = "3"

	messageTypeUser = 1
	messageTypeBot  = 2

	messageStateFinish = 2

	messageItemTypeText  = 1
	messageItemTypeImage = 2

	typingStatusTyping = 1
	typingStatusCancel = 2
)

// Account 描述一次微信扫码登录得到的账号凭据。
type Account struct {
	ID        string    `json:"id"`
	Token     string    `json:"token"`
	BaseURL   string    `json:"base_url"`
	UserID    string    `json:"user_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

// LoginResult 描述扫码登录完成后的结果。
type LoginResult struct {
	Account          Account
	AlreadyConnected bool
}

type baseInfo struct {
	ChannelVersion string `json:"channel_version,omitempty"`
	BotAgent       string `json:"bot_agent,omitempty"`
}

type qrCodeResponse struct {
	QRCode     string `json:"qrcode"`
	QRCodeURL  string `json:"qrcode_img_content"`
	Error      string `json:"errmsg,omitempty"`
	ReturnCode int    `json:"ret,omitempty"`
}

type apiStatus struct {
	ReturnCode int    `json:"ret,omitempty"`
	ErrorCode  int    `json:"errcode,omitempty"`
	Error      string `json:"errmsg,omitempty"`
}

type qrStatusResponse struct {
	Status       string `json:"status"`
	BotToken     string `json:"bot_token,omitempty"`
	AccountID    string `json:"ilink_bot_id,omitempty"`
	BaseURL      string `json:"baseurl,omitempty"`
	UserID       string `json:"ilink_user_id,omitempty"`
	RedirectHost string `json:"redirect_host,omitempty"`
	Error        string `json:"errmsg,omitempty"`
}

type getUpdatesRequest struct {
	GetUpdatesBuf string   `json:"get_updates_buf"`
	BaseInfo      baseInfo `json:"base_info"`
}

type getUpdatesResponse struct {
	ReturnCode         int             `json:"ret,omitempty"`
	ErrorCode          int             `json:"errcode,omitempty"`
	Error              string          `json:"errmsg,omitempty"`
	Messages           []WeixinMessage `json:"msgs,omitempty"`
	GetUpdatesBuf      string          `json:"get_updates_buf,omitempty"`
	LongPollingTimeout int             `json:"longpolling_timeout_ms,omitempty"`
}

// WeixinMessage 描述微信 iLink 返回的一条消息。
type WeixinMessage struct {
	Seq          int64         `json:"seq,omitempty"`
	MessageID    int64         `json:"message_id,omitempty"`
	FromUserID   string        `json:"from_user_id,omitempty"`
	ToUserID     string        `json:"to_user_id,omitempty"`
	ClientID     string        `json:"client_id,omitempty"`
	SessionID    string        `json:"session_id,omitempty"`
	GroupID      string        `json:"group_id,omitempty"`
	MessageType  int           `json:"message_type,omitempty"`
	MessageState int           `json:"message_state,omitempty"`
	Items        []MessageItem `json:"item_list,omitempty"`
	ContextToken string        `json:"context_token,omitempty"`
	RunID        string        `json:"run_id,omitempty"`
}

// MessageItem 描述微信消息中的一个内容项。
type MessageItem struct {
	Type      int        `json:"type,omitempty"`
	TextItem  *TextItem  `json:"text_item,omitempty"`
	ImageItem *ImageItem `json:"image_item,omitempty"`
}

// TextItem 描述微信文本内容项。
type TextItem struct {
	Text string `json:"text,omitempty"`
}

// CDNMedia 描述微信 CDN 媒体引用。
type CDNMedia struct {
	EncryptQueryParam string `json:"encrypt_query_param,omitempty"`
	AESKey            string `json:"aes_key,omitempty"`
	EncryptType       int    `json:"encrypt_type,omitempty"`
	FullURL           string `json:"full_url,omitempty"`
}

// ImageItem 描述微信图片内容项。
type ImageItem struct {
	Media      *CDNMedia `json:"media,omitempty"`
	ThumbMedia *CDNMedia `json:"thumb_media,omitempty"`
	AESKey     string    `json:"aeskey,omitempty"`
	URL        string    `json:"url,omitempty"`
	MidSize    int64     `json:"mid_size,omitempty"`
	ThumbSize  int64     `json:"thumb_size,omitempty"`
	HDSize     int64     `json:"hd_size,omitempty"`
}

type sendMessageRequest struct {
	Message  WeixinMessage `json:"msg"`
	BaseInfo baseInfo      `json:"base_info"`
}

type getConfigRequest struct {
	UserID       string   `json:"ilink_user_id"`
	ContextToken string   `json:"context_token,omitempty"`
	BaseInfo     baseInfo `json:"base_info"`
}

type getConfigResponse struct {
	ReturnCode   int    `json:"ret,omitempty"`
	Error        string `json:"errmsg,omitempty"`
	TypingTicket string `json:"typing_ticket,omitempty"`
}

type sendTypingRequest struct {
	UserID       string   `json:"ilink_user_id"`
	TypingTicket string   `json:"typing_ticket,omitempty"`
	Status       int      `json:"status,omitempty"`
	BaseInfo     baseInfo `json:"base_info"`
}
