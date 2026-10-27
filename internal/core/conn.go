package core

import (
	"cmp"
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

// conn 是一个客户端连接：实现 protocol.Service，它订阅的会话的更新推给 sub。
// 运行、取消、换配置与关闭只对这个连接订阅着的会话有效：会话要先在这个连接上新建或打开。
// 会话本身由核心持有，卸载或失效之后，连接上的后续请求同样得到 ErrNotFound。
type conn struct {
	core *Core
	sub  protocol.Subscriber
}

// session 返回这个连接订阅着的会话。
func (c *conn) session(id string) (*session, error) {
	s, err := c.core.lookup(id)
	if err != nil || !s.subscribed(c.sub) {
		return nil, protocol.ErrNotFound
	}
	return s, nil
}

func (c *conn) NewSession(
	_ context.Context, req protocol.NewSessionRequest,
) (protocol.NewSessionResponse, error) {
	end, err := c.core.begin()
	if err != nil {
		return protocol.NewSessionResponse{}, err
	}
	defer end()
	s, err := c.core.create(req.Cwd, req.Type, req.OwnerID)
	if err != nil {
		return protocol.NewSessionResponse{}, err
	}
	options, err := s.options()
	if err != nil {
		return protocol.NewSessionResponse{}, err
	}
	c.core.adopt(s, c.sub)
	return protocol.NewSessionResponse{
		SessionID: s.id, ConfigOptions: options, State: s.state(""),
	}, nil
}

func (c *conn) OpenSession(
	ctx context.Context, req protocol.OpenSessionRequest,
) (protocol.OpenSessionResponse, error) {
	end, err := c.core.begin()
	if err != nil {
		return protocol.OpenSessionResponse{}, err
	}
	defer end()
	info, err := c.core.db.Get(ctx, req.SessionID)
	if err != nil {
		return protocol.OpenSessionResponse{}, mapStoreError(err)
	}
	s, err := c.core.open(ctx, req.SessionID, c.sub)
	if err != nil {
		return protocol.OpenSessionResponse{}, err
	}
	options, err := s.options()
	if err != nil {
		c.core.release(s, c.sub)
		return protocol.OpenSessionResponse{}, err
	}
	return protocol.OpenSessionResponse{ConfigOptions: options, State: s.state(info.Title)}, nil
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
	filter := store.Filter{
		Cwd: req.Cwd, Type: string(cmp.Or(req.Type, protocol.SessionInteractive)),
		OwnerID: req.OwnerID,
	}
	infos, err := c.core.db.List(ctx, filter, before, listPage)
	if err != nil {
		return protocol.ListSessionsResponse{}, err
	}
	var resp protocol.ListSessionsResponse
	for _, info := range infos {
		resp.Sessions = append(resp.Sessions, protocol.SessionInfo{
			SessionID: info.ID, Cwd: info.Cwd, Title: info.Title, UpdatedAt: info.UpdatedAt,
			Type: protocol.SessionType(info.Type), OwnerID: info.OwnerID,
		})
	}
	if len(infos) == listPage {
		last := infos[len(infos)-1]
		resp.NextCursor = strconv.FormatInt(last.UpdatedAt.UnixMilli(), 10) + "/" + last.ID
	}
	return resp, nil
}

// DeleteSession 删除空闲的会话；会话不存在时什么也不做。
// 删除不要求这个连接打开过它。
func (c *conn) DeleteSession(ctx context.Context, req protocol.DeleteSessionRequest) error {
	end, err := c.core.begin()
	if err != nil {
		return err
	}
	defer end()
	s, _ := c.core.lookup(req.SessionID)
	if s != nil {
		done, err := s.begin(nil)
		if err != nil {
			return err
		}
		defer done()
	}
	err = c.core.db.Delete(ctx, req.SessionID)
	if err != nil && !errors.Is(err, store.ErrNotFound) { // 还没保存的新会话不在库里
		return err
	}
	if s != nil {
		c.core.forget(s)
	}
	return nil
}

// CloseSession 取消这个连接对会话的订阅，不影响正在进行的一轮。
func (c *conn) CloseSession(_ context.Context, req protocol.CloseSessionRequest) error {
	end, err := c.core.begin()
	if err != nil {
		return err
	}
	defer end()
	if s, err := c.session(req.SessionID); err == nil {
		c.core.release(s, c.sub)
	}
	return nil
}

// RenameSession 修改标题；订阅该会话的连接都会收到新标题。不要求当前连接打开过它。
func (c *conn) RenameSession(ctx context.Context, req protocol.RenameSessionRequest) error {
	end, err := c.core.begin()
	if err != nil {
		return err
	}
	defer end()
	if err := c.core.db.Rename(ctx, req.SessionID, req.Title); err != nil {
		return mapStoreError(err)
	}
	if s, err := c.core.lookup(req.SessionID); err == nil {
		s.broadcast(protocol.SessionInfoUpdate{Title: req.Title})
	}
	return nil
}

func (c *conn) SendPrompt(
	_ context.Context, req protocol.SendPromptRequest,
) (protocol.SendPromptResponse, error) {
	end, err := c.core.begin()
	if err != nil {
		return protocol.SendPromptResponse{}, err
	}
	defer end()
	s, err := c.session(req.SessionID)
	if err != nil {
		return protocol.SendPromptResponse{}, err
	}
	return protocol.SendPromptResponse{}, s.sendPrompt(req.Prompt)
}

func (c *conn) CancelTurn(_ context.Context, req protocol.CancelTurnRequest) error {
	end, err := c.core.begin()
	if err != nil {
		return err
	}
	defer end()
	s, err := c.session(req.SessionID)
	if err != nil {
		return err
	}
	s.cancelTurn()
	return nil
}

func (c *conn) SetConfigOption(
	ctx context.Context, req protocol.SetConfigOptionRequest,
) (protocol.SetConfigOptionResponse, error) {
	end, err := c.core.begin()
	if err != nil {
		return protocol.SetConfigOptionResponse{}, err
	}
	defer end()
	s, err := c.session(req.SessionID)
	if err != nil {
		return protocol.SetConfigOptionResponse{}, err
	}
	options, err := s.setConfig(ctx, req.ConfigID, req.Value)
	c.core.unloadIfIdle(s) // 期间若所有订阅者都关闭了会话，现在可以卸载
	return protocol.SetConfigOptionResponse{ConfigOptions: options}, err
}

func (c *conn) ListHistory(
	ctx context.Context, req protocol.ListHistoryRequest,
) (protocol.ListHistoryResponse, error) {
	end, err := c.core.begin()
	if err != nil {
		return protocol.ListHistoryResponse{}, err
	}
	defer end()
	records, err := c.core.db.Page(ctx, req.SessionID, req.Before, req.Limit)
	if err != nil || len(records) == 0 {
		return protocol.ListHistoryResponse{}, mapStoreError(err)
	}
	updates, err := c.core.historyUpdates(ctx, req.SessionID, records)
	if err != nil {
		return protocol.ListHistoryResponse{}, err
	}
	return protocol.ListHistoryResponse{Updates: updates, Oldest: records[0].Seq}, nil
}

func (c *conn) ListUserMessages(
	ctx context.Context, req protocol.ListUserMessagesRequest,
) (protocol.ListUserMessagesResponse, error) {
	end, err := c.core.begin()
	if err != nil {
		return protocol.ListUserMessagesResponse{}, err
	}
	defer end()
	records, err := c.core.db.ListKind(ctx, req.SessionID, kindUser, req.Before)
	if err != nil {
		return protocol.ListUserMessagesResponse{}, mapStoreError(err)
	}
	var resp protocol.ListUserMessagesResponse
	for _, r := range records {
		m, err := decode(req.SessionID, r)
		if err != nil {
			return protocol.ListUserMessagesResponse{}, err
		}
		if user, ok := m.(agent.UserMessage); ok {
			resp.Messages = append(resp.Messages, userMessageSummary(r.Seq, user))
		}
	}
	return resp, nil
}
