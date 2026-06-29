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

// Store reads and writes local account and state files for the WeChat channel.
type Store struct {
	dir string
}

// SenderState holds the current Atlas session state for a WeChat sender.
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

// DefaultStoreDir returns the default local state directory for the WeChat channel.
func DefaultStoreDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".atlas", "weixin"), nil
}

// NewStore creates local storage for the WeChat channel.
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
	if err := secureStorePath(filepath.Join(dir, accountsDirName), true); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// SaveAccount saves WeChat account credentials.
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
	if err := writeSecureFile(s.accountPath(account.ID), content); err != nil {
		return err
	}
	state, err := s.loadState()
	if err != nil {
		return err
	}
	state.CurrentAccountID = account.ID
	return s.saveState(state)
}

// LoadAccount reads the specified WeChat account; when accountID is empty, reads the current account.
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

// ListAccounts returns locally saved WeChat accounts.
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

// DeleteAccount deletes the specified WeChat account.
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

// loadState reads the WeChat channel runtime state; returns empty state if not found.
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

// saveState saves the WeChat channel runtime state.
func (s *Store) saveState(state channelState) error {
	if state.Senders == nil {
		state.Senders = map[string]SenderState{}
	}
	content, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return writeSecureFile(s.statePath(), content)
}

// accountPath returns the account credentials file path.
func (s *Store) accountPath(accountID string) string {
	return filepath.Join(s.dir, accountsDirName, safeFileName(accountID)+".json")
}

// statePath returns the WeChat channel state file path.
func (s *Store) statePath() string {
	return filepath.Join(s.dir, stateFileName)
}

// safeFileName converts an external account ID to a string usable as a local filename.
func safeFileName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}
	return safeFileNamePattern.ReplaceAllString(value, "_")
}
