package core

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gvenusleo/atlas/internal/agent"
	"github.com/gvenusleo/atlas/internal/protocol"
	"github.com/gvenusleo/atlas/internal/store"
)

// listPage 是 ListSessions 一页的会话数。
const listPage = 100

// conn 是一个客户端连接：实现 protocol.Agent，它打开的会话的更新推给 client。
type conn struct {
	core   *Core
	client protocol.Client
}

func (c *conn) NewSession(
	ctx context.Context, req protocol.NewSessionRequest,
) (protocol.NewSessionResponse, error) {
	end, err := c.core.begin()
	if err != nil {
		return protocol.NewSessionResponse{}, err
	}
	defer end()
	s, err := c.core.create(req.Cwd)
	if err != nil {
		return protocol.NewSessionResponse{}, err
	}
	options, err := s.options()
	if err != nil {
		return protocol.NewSessionResponse{}, err
	}
	s.subscribe(c.client)
	return protocol.NewSessionResponse{
		SessionID: s.id, ConfigOptions: options, State: s.state(""),
	}, nil
}

func (c *conn) ResumeSession(
	ctx context.Context, req protocol.ResumeSessionRequest,
) (protocol.ResumeSessionResponse, error) {
	end, err := c.core.begin()
	if err != nil {
		return protocol.ResumeSessionResponse{}, err
	}
	defer end()
	return c.attach(ctx, req.SessionID)
}

func (c *conn) LoadSession(
	ctx context.Context, req protocol.LoadSessionRequest,
) (protocol.LoadSessionResponse, error) {
	end, err := c.core.begin()
	if err != nil {
		return protocol.LoadSessionResponse{}, err
	}
	defer end()
	_, records, err := c.core.db.Log(ctx, req.SessionID)
	if err != nil {
		return protocol.LoadSessionResponse{}, err
	}
	for _, u := range historyUpdates(records) {
		c.client.SessionUpdate(protocol.SessionNotification{SessionID: req.SessionID, Update: u})
	}
	return c.attach(ctx, req.SessionID)
}

// attach 打开会话并订阅它的更新，给出它的配置项与现在的状态。
func (c *conn) attach(ctx context.Context, id string) (protocol.ResumeSessionResponse, error) {
	s, err := c.core.open(ctx, id)
	if err != nil {
		return protocol.ResumeSessionResponse{}, err
	}
	options, err := s.options()
	if err != nil {
		return protocol.ResumeSessionResponse{}, err
	}
	info, err := c.core.db.Get(ctx, id)
	if err != nil {
		return protocol.ResumeSessionResponse{}, err
	}
	s.subscribe(c.client)
	return protocol.ResumeSessionResponse{ConfigOptions: options, State: s.state(info.Name)}, nil
}

func (c *conn) ListSessions(
	ctx context.Context, req protocol.ListSessionsRequest,
) (protocol.ListSessionsResponse, error) {
	end, err := c.core.begin()
	if err != nil {
		return protocol.ListSessionsResponse{}, err
	}
	defer end()
	var before *store.Cursor
	if req.Cursor != "" {
		at, id, _ := strings.Cut(req.Cursor, "/")
		ms, err := strconv.ParseInt(at, 10, 64)
		if err != nil {
			return protocol.ListSessionsResponse{}, errors.New("游标格式不对")
		}
		before = &store.Cursor{UpdatedAt: time.UnixMilli(ms), ID: id}
	}
	infos, err := c.core.db.List(ctx, req.Cwd, before, listPage)
	if err != nil {
		return protocol.ListSessionsResponse{}, err
	}
	var resp protocol.ListSessionsResponse
	for _, info := range infos {
		resp.Sessions = append(resp.Sessions, protocol.SessionInfo{
			SessionID: info.ID, Cwd: info.Cwd, Title: info.Name, UpdatedAt: info.UpdatedAt,
		})
	}
	if len(infos) == listPage {
		last := infos[len(infos)-1]
		resp.NextCursor = strconv.FormatInt(last.UpdatedAt.UnixMilli(), 10) + "/" + last.ID
	}
	return resp, nil
}

func (c *conn) DeleteSession(ctx context.Context, req protocol.DeleteSessionRequest) error {
	end, err := c.core.begin()
	if err != nil {
		return err
	}
	defer end()
	s, err := c.core.lookup(req.SessionID)
	if err != nil {
		return c.core.db.Delete(ctx, req.SessionID)
	}
	done, err := s.begin(nil)
	if err != nil {
		return err
	}
	defer done()
	s.mu.Lock()
	saved := s.saved
	s.mu.Unlock()
	if saved {
		if err := c.core.db.Delete(ctx, req.SessionID); err != nil {
			return err
		}
	}
	c.core.forget(s)
	return nil
}

func (c *conn) CloseSession(_ context.Context, req protocol.CloseSessionRequest) error {
	s, err := c.core.lookup(req.SessionID)
	if err != nil {
		return nil
	}
	s.unsubscribe(c.client)
	return nil
}

func (c *conn) Prompt(
	ctx context.Context, req protocol.PromptRequest,
) (protocol.PromptResponse, error) {
	end, err := c.core.begin()
	if err != nil {
		return protocol.PromptResponse{}, err
	}
	defer end()
	s, err := c.core.lookup(req.SessionID)
	if err != nil {
		return protocol.PromptResponse{}, err
	}
	reason, err := s.send(ctx, req.Prompt)
	return protocol.PromptResponse{StopReason: reason}, err
}

func (c *conn) Cancel(req protocol.CancelNotification) {
	if s, err := c.core.lookup(req.SessionID); err == nil {
		s.cancelTurn()
	}
}

func (c *conn) SetConfigOption(
	ctx context.Context, req protocol.SetConfigOptionRequest,
) (protocol.SetConfigOptionResponse, error) {
	end, err := c.core.begin()
	if err != nil {
		return protocol.SetConfigOptionResponse{}, err
	}
	defer end()
	s, err := c.core.lookup(req.SessionID)
	if err != nil {
		return protocol.SetConfigOptionResponse{}, err
	}
	options, err := s.setConfig(ctx, req.ConfigID, req.Value)
	return protocol.SetConfigOptionResponse{ConfigOptions: options}, err
}

func (c *conn) History(
	ctx context.Context, req protocol.HistoryRequest,
) (protocol.HistoryResponse, error) {
	end, err := c.core.begin()
	if err != nil {
		return protocol.HistoryResponse{}, err
	}
	defer end()
	records, err := c.core.db.Page(ctx, req.SessionID, req.Before, req.Limit)
	if err != nil || len(records) == 0 {
		return protocol.HistoryResponse{}, err
	}
	return protocol.HistoryResponse{Updates: historyUpdates(records), Oldest: records[0].Seq}, nil
}

func (c *conn) Prompts(
	ctx context.Context, req protocol.PromptsRequest,
) (protocol.PromptsResponse, error) {
	end, err := c.core.begin()
	if err != nil {
		return protocol.PromptsResponse{}, err
	}
	defer end()
	records, err := c.core.db.Prompts(ctx, req.SessionID, req.Before)
	if err != nil {
		return protocol.PromptsResponse{}, err
	}
	var resp protocol.PromptsResponse
	for _, r := range records {
		if m, ok := r.Message.(agent.UserMessage); ok {
			resp.Prompts = append(resp.Prompts, promptEntry(r.Seq, m))
		}
	}
	return resp, nil
}

func (c *conn) Rename(ctx context.Context, req protocol.RenameRequest) error {
	end, err := c.core.begin()
	if err != nil {
		return err
	}
	defer end()
	if err := c.core.db.Rename(ctx, req.SessionID, req.Title); err != nil {
		return err
	}
	if s, err := c.core.lookup(req.SessionID); err == nil {
		s.broadcast(protocol.SessionInfoUpdate{Title: req.Title})
	}
	return nil
}
