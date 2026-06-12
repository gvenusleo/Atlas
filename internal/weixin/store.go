package weixin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	stateFileName   = "state.json"
	accountsDirName = "accounts"
)

var safeFileNamePattern = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// Store 读写微信通道的本地账号和状态文件。
type Store struct {
	dir string
}

// SenderState 保存某个微信发送人的当前 Atlas 会话状态。
type SenderState struct {
	CWD         string `json:"cwd"`
	PreviousCWD string `json:"previous_cwd,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
}

type channelState struct {
	CurrentAccountID string                 `json:"current_account_id,omitempty"`
	GetUpdatesBuf    string                 `json:"get_updates_buf,omitempty"`
	TypingTicket     string                 `json:"typing_ticket,omitempty"`
	Senders          map[string]SenderState `json:"senders,omitempty"`
}

// DefaultStoreDir 返回微信通道默认本地状态目录。
func DefaultStoreDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".atlas", "weixin"), nil
}

// NewStore 创建微信通道本地存储。
func NewStore(dir string) (*Store, error) {
	if strings.TrimSpace(dir) == "" {
		var err error
		dir, err = DefaultStoreDir()
		if err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, accountsDirName), 0o700); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// SaveAccount 保存微信账号凭据。
func (s *Store) SaveAccount(account Account) error {
	if account.ID == "" {
		return fmt.Errorf("weixin account id is required")
	}
	if account.UpdatedAt.IsZero() {
		account.UpdatedAt = time.Now()
	}
	content, err := json.MarshalIndent(account, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(s.accountPath(account.ID), content, 0o600); err != nil {
		return err
	}
	state, err := s.loadState()
	if err != nil {
		return err
	}
	state.CurrentAccountID = account.ID
	return s.saveState(state)
}

// LoadAccount 读取指定微信账号；accountID 为空时读取当前账号。
func (s *Store) LoadAccount(accountID string) (Account, error) {
	if strings.TrimSpace(accountID) == "" {
		state, err := s.loadState()
		if err != nil {
			return Account{}, err
		}
		accountID = state.CurrentAccountID
	}
	if strings.TrimSpace(accountID) == "" {
		return Account{}, fmt.Errorf("weixin account is not logged in")
	}
	content, err := os.ReadFile(s.accountPath(accountID))
	if err != nil {
		return Account{}, err
	}
	var account Account
	if err := json.Unmarshal(content, &account); err != nil {
		return Account{}, err
	}
	return account, nil
}

// ListAccounts 返回本地保存的微信账号。
func (s *Store) ListAccounts() ([]Account, error) {
	entries, err := os.ReadDir(filepath.Join(s.dir, accountsDirName))
	if err != nil {
		return nil, err
	}
	var accounts []Account
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(s.dir, accountsDirName, entry.Name()))
		if err != nil {
			return nil, err
		}
		var account Account
		if err := json.Unmarshal(content, &account); err != nil {
			return nil, err
		}
		accounts = append(accounts, account)
	}
	sort.Slice(accounts, func(i, j int) bool {
		return accounts[i].UpdatedAt.After(accounts[j].UpdatedAt)
	})
	return accounts, nil
}

// DeleteAccount 删除指定微信账号。
func (s *Store) DeleteAccount(accountID string) error {
	if strings.TrimSpace(accountID) == "" {
		return fmt.Errorf("weixin account id is required")
	}
	err := os.Remove(s.accountPath(accountID))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	state, err := s.loadState()
	if err != nil {
		return err
	}
	if state.CurrentAccountID == accountID {
		state.CurrentAccountID = ""
	}
	return s.saveState(state)
}

// loadState 读取微信通道运行状态；不存在时返回空状态。
func (s *Store) loadState() (channelState, error) {
	content, err := os.ReadFile(s.statePath())
	if os.IsNotExist(err) {
		return channelState{Senders: map[string]SenderState{}}, nil
	}
	if err != nil {
		return channelState{}, err
	}
	var state channelState
	if err := json.Unmarshal(content, &state); err != nil {
		return channelState{}, err
	}
	if state.Senders == nil {
		state.Senders = map[string]SenderState{}
	}
	return state, nil
}

// saveState 保存微信通道运行状态。
func (s *Store) saveState(state channelState) error {
	if state.Senders == nil {
		state.Senders = map[string]SenderState{}
	}
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.statePath(), content, 0o600)
}

// accountPath 返回账号凭据文件路径。
func (s *Store) accountPath(accountID string) string {
	return filepath.Join(s.dir, accountsDirName, safeFileName(accountID)+".json")
}

// statePath 返回微信通道状态文件路径。
func (s *Store) statePath() string {
	return filepath.Join(s.dir, stateFileName)
}

// safeFileName 把外部账号 ID 转成可用作本地文件名的字符串。
func safeFileName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}
	return safeFileNamePattern.ReplaceAllString(value, "_")
}
