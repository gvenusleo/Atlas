package runtime

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/liuyuxin/atlas/internal/compact"
	"github.com/liuyuxin/atlas/internal/config"
	"github.com/liuyuxin/atlas/internal/memory"
	"github.com/liuyuxin/atlas/internal/model"
	"github.com/liuyuxin/atlas/internal/session"
)

const (
	memoryWorkerBatchSize = 4
	memoryWorkerInterval  = 5 * time.Second
	memoryJobLease        = 2 * time.Minute
	memoryPromptMaxRunes  = 40000
	memorySummaryMaxRunes = 16000
	// 记忆抽取按增量触发，避免每轮对话都消耗后台模型 token。
	memoryExtractMinNewMessages    = 6
	memoryExtractMinNewInputTokens = 4000
)

// memoryDecayDays 按记忆类型指定 confidence 衰减速率（每多少天衰减 1 点）。
var memoryDecayDays = map[string]int{
	memory.TypeInstruction: 180,
	memory.TypeFact:        90,
	memory.TypeWorkflow:    120,
}

const memoryPruneUnusedDays = 90

var memoryWorkerIDPrefix = "atlas-memory-"
var memoryExtractTriggerRatios = []float64{0.4, 0.6, 0.8}

// ProcessMemoryJobs 处理有限数量的后台记忆任务，主要供测试和短生命周期入口复用。
func (r *Runtime) ProcessMemoryJobs(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = memoryWorkerBatchSize
	}
	return r.processMemoryJobs(ctx, limit, newMemoryWorkerID())
}

// RunMemoryWorker 持续处理后台记忆任务，直到 ctx 取消。
func (r *Runtime) RunMemoryWorker(ctx context.Context) error {
	workerID := newMemoryWorkerID()
	ticker := time.NewTicker(memoryWorkerInterval)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return nil
		}
		processed, _ := r.processMemoryJobs(ctx, memoryWorkerBatchSize, workerID)
		if processed == memoryWorkerBatchSize {
			continue
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// processMemoryJobs 批量领取后台任务，并按任务模型创建 provider；单条失败只影响该任务。
func (r *Runtime) processMemoryJobs(ctx context.Context, limit int, workerID string) (int, error) {
	cfg, err := r.deps.LoadConfig()
	if err != nil {
		return 0, err
	}
	if !cfg.Memory.IsEnabled() {
		return 0, nil
	}
	memStore, err := openMemoryStore(ctx, cfg.Session)
	if err != nil {
		return 0, err
	}
	defer memStore.Close()
	sessionStore, err := openSessionStore(ctx, cfg.Session)
	if err != nil {
		return 0, err
	}
	defer sessionStore.Close()

	processed := 0
	for processed < limit {
		job, ok, err := memStore.ClaimNextJob(ctx, workerID, memoryJobLease)
		if err != nil {
			return processed, err
		}
		if !ok {
			if processed == 0 {
				_, _ = memStore.PruneStaleEntries(ctx, memoryPruneUnusedDays)
			}
			return processed, nil
		}
		activeProvider, selectedModel, err := cfg.ResolveModel(job.Model)
		if err != nil {
			_ = memStore.FailJob(ctx, job, err)
			processed++
			continue
		}
		provider, err := r.deps.NewProvider(activeProvider, selectedModel)
		if err != nil {
			_ = memStore.FailJob(ctx, job, err)
			processed++
			continue
		}
		if err := r.processMemoryJob(ctx, memStore, sessionStore, provider, selectedModel, job); err != nil {
			_ = memStore.FailJob(ctx, job, err)
			processed++
			continue
		}
		if err := memStore.CompleteJob(ctx, job); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

func (r *Runtime) processMemoryJob(ctx context.Context, memStore *memory.Store, sessionStore *session.Store, provider model.Provider, selectedModel config.ProviderModel, job memory.Job) error {
	switch job.Kind {
	case memory.JobKindSessionExtract:
		return r.processMemoryExtractJob(ctx, memStore, sessionStore, provider, selectedModel, job)
	case memory.JobKindScopeSummarize:
		return r.processMemorySummarizeJob(ctx, memStore, provider, selectedModel, job)
	default:
		return fmt.Errorf("unknown memory job kind %q", job.Kind)
	}
}

type memoryExtractOutput struct {
	Entries             []memoryExtractEntry `json:"entries"`
	ArchiveFingerprints []string             `json:"archive_fingerprints"`
}

type memoryExtractEntry struct {
	Scope      string `json:"scope"`
	Type       string `json:"type"`
	Content    string `json:"content"`
	SourceNote string `json:"source_note"`
	Confidence int    `json:"confidence"`
}

type memorySummaryOutput struct {
	Summary string `json:"summary"`
}

// processMemoryExtractJob 从上次处理边界后的增量消息抽取长期记忆。
func (r *Runtime) processMemoryExtractJob(ctx context.Context, memStore *memory.Store, sessionStore *session.Store, provider model.Provider, selectedModel config.ProviderModel, job memory.Job) error {
	info, err := sessionStore.GetSession(ctx, job.SessionID)
	if err != nil {
		if isSessionNotFound(err) {
			return nil
		}
		return err
	}
	trans, err := sessionStore.LoadTranscript(ctx, job.SessionID)
	if err != nil {
		return err
	}
	messages := trans.Messages()
	if len(messages) == 0 {
		return nil
	}
	inputTokens := compact.EstimateMessages(messages)
	inputHash := memory.TranscriptHash(messages)
	if info.MemoryExtractedHash == inputHash {
		return nil
	}
	start := info.MemoryExtractedMessageCount
	if start < 0 {
		start = 0
	}
	if start > len(messages) {
		start = len(messages)
	}
	newMessages := messages[start:]
	if len(newMessages) == 0 {
		return sessionStore.SaveMemoryExtraction(ctx, job.SessionID, len(messages), inputTokens, inputHash)
	}
	projectKey, projectPath := memory.ProjectIdentity(info.CWD)
	existing, err := loadExistingMemories(ctx, memStore, projectKey, 30)
	if err != nil {
		return err
	}
	if err := memStore.DecayConfidence(ctx, memoryDecayDays); err != nil {
		return err
	}
	reasoningEffort, err := selectedReasoningEffort("", false, selectedModel)
	if err != nil {
		return err
	}
	resp, err := provider.Stream(ctx, model.ChatRequest{
		System:          memoryExtractSystemPrompt(),
		Messages:        []model.Message{model.TextMessage(model.RoleUser, memoryExtractPrompt(info, newMessages, existing, start))},
		MaxTokens:       summaryMaxTokens(selectedModel.MaxTokens),
		Temperature:     0,
		ReasoningEffort: reasoningEffort,
		ResponseFormat:  model.ResponseFormatJSONObject,
	}, nil)
	if err != nil {
		return err
	}
	var output memoryExtractOutput
	if err := decodeJSONObject(resp.Content, &output); err != nil {
		return err
	}
	existingByFingerprint := make(map[string]memory.Entry, len(existing))
	for _, entry := range existing {
		existingByFingerprint[entry.Fingerprint] = entry
	}
	touched := make(map[string]memory.Summary)
	for _, item := range output.Entries {
		entry, ok := normalizeMemoryExtractEntry(item, projectKey, projectPath, info.ID)
		if !ok {
			continue
		}
		saved, err := memStore.UpsertEntry(ctx, entry)
		if err != nil {
			return err
		}
		touched[memoryScopeKey(saved.Scope, saved.ProjectKey)] = memory.Summary{
			Scope:       saved.Scope,
			ProjectKey:  saved.ProjectKey,
			ProjectPath: saved.ProjectPath,
		}
	}
	var archiveFingerprints []string
	for _, fingerprint := range output.ArchiveFingerprints {
		fingerprint = strings.TrimSpace(fingerprint)
		entry, ok := existingByFingerprint[fingerprint]
		if !ok {
			continue
		}
		archiveFingerprints = append(archiveFingerprints, fingerprint)
		touched[memoryScopeKey(entry.Scope, entry.ProjectKey)] = memory.Summary{
			Scope:       entry.Scope,
			ProjectKey:  entry.ProjectKey,
			ProjectPath: entry.ProjectPath,
		}
	}
	if err := memStore.ArchiveFingerprints(ctx, archiveFingerprints); err != nil {
		return err
	}
	for _, summary := range touched {
		if err := memStore.EnqueueSummarize(ctx, summary.Scope, summary.ProjectKey, summary.ProjectPath, selectedModel.Value); err != nil {
			return err
		}
	}
	return sessionStore.SaveMemoryExtraction(ctx, job.SessionID, len(messages), inputTokens, inputHash)
}

// processMemorySummarizeJob 将某个作用域的 active 记忆压成短摘要，供后续提示词注入。
func (r *Runtime) processMemorySummarizeJob(ctx context.Context, memStore *memory.Store, provider model.Provider, selectedModel config.ProviderModel, job memory.Job) error {
	entries, err := memStore.ListEntries(ctx, job.Scope, job.ProjectKey)
	if err != nil {
		return err
	}
	inputHash := memory.EntriesInputHash(entries)
	if len(entries) == 0 {
		return memStore.SaveSummary(ctx, memory.Summary{
			Scope:       job.Scope,
			ProjectKey:  job.ProjectKey,
			ProjectPath: job.ProjectPath,
			EntryCount:  0,
			InputHash:   inputHash,
		})
	}
	reasoningEffort, err := selectedReasoningEffort("", false, selectedModel)
	if err != nil {
		return err
	}
	resp, err := provider.Stream(ctx, model.ChatRequest{
		System:          memorySummarySystemPrompt(),
		Messages:        []model.Message{model.TextMessage(model.RoleUser, memorySummaryPrompt(job, entries))},
		MaxTokens:       summaryMaxTokens(selectedModel.MaxTokens),
		Temperature:     0,
		ReasoningEffort: reasoningEffort,
		ResponseFormat:  model.ResponseFormatJSONObject,
	}, nil)
	if err != nil {
		return err
	}
	var output memorySummaryOutput
	if err := decodeJSONObject(resp.Content, &output); err != nil {
		return err
	}
	summary := strings.TrimSpace(output.Summary)
	if summary == "" {
		return fmt.Errorf("memory summary is empty")
	}
	return memStore.SaveSummary(ctx, memory.Summary{
		Scope:       job.Scope,
		ProjectKey:  job.ProjectKey,
		ProjectPath: job.ProjectPath,
		Content:     summary,
		EntryCount:  len(entries),
		InputHash:   inputHash,
	})
}

// loadMemoryContext 以 best-effort 方式读取长期记忆，失败时不影响主 turn。
// 同一 session 连续轮次中，上轮已注入且本轮仍在结果中的记忆会被跳过以防重复注入。
func (r *Runtime) loadMemoryContext(ctx context.Context, cfg config.SessionConfig, sessionID, cwd, query string) string {
	store, err := openMemoryStore(ctx, cfg)
	if err != nil {
		return ""
	}
	defer store.Close()

	r.lastInjectedMemMu.Lock()
	exclude := r.lastInjectedMemories[sessionID]
	r.lastInjectedMemMu.Unlock()

	contextText, fingerprints, err := store.PromptContext(ctx, cwd, query, exclude)
	if err != nil {
		return ""
	}

	r.lastInjectedMemMu.Lock()
	r.lastInjectedMemories[sessionID] = fingerprints
	r.lastInjectedMemMu.Unlock()

	return contextText
}

type memoryExtractTriggerOptions struct {
	Force         bool
	ContextWindow int
}

// maybeEnqueueMemoryExtract 按增量阈值安排记忆抽取，失败时不影响主 turn。
func (r *Runtime) maybeEnqueueMemoryExtract(ctx context.Context, cfg config.SessionConfig, info session.Session, sessionID, cwd string, messages []model.Message, model string, opts memoryExtractTriggerOptions) {
	if len(messages) == 0 {
		return
	}
	inputTokens := compact.EstimateMessages(messages)
	inputHash := memory.TranscriptHash(messages)
	if info.MemoryExtractedHash == inputHash {
		return
	}
	if !opts.Force && !shouldEnqueueMemoryExtract(info, messages, inputTokens, opts.ContextWindow) {
		return
	}
	store, err := openMemoryStore(ctx, cfg)
	if err != nil {
		return
	}
	defer store.Close()
	_ = store.EnqueueSessionExtract(ctx, sessionID, cwd, inputHash, model)
}

// shouldEnqueueMemoryExtract 判断新增 transcript 是否达到后台记忆抽取阈值。
func shouldEnqueueMemoryExtract(info session.Session, messages []model.Message, inputTokens, contextWindow int) bool {
	start := info.MemoryExtractedMessageCount
	if start < 0 {
		start = 0
	}
	if start > len(messages) {
		start = len(messages)
	}
	if len(messages)-start >= memoryExtractMinNewMessages {
		return true
	}
	if inputTokens-info.MemoryExtractedInputTokens >= memoryExtractMinNewInputTokens {
		return true
	}
	if crossedMemoryExtractRatio(info.MemoryExtractedInputTokens, inputTokens, contextWindow) {
		return true
	}
	return containsExplicitMemoryDirective(messages[start:])
}

// crossedMemoryExtractRatio 判断上下文用量是否跨过固定水位档。
func crossedMemoryExtractRatio(previousTokens, currentTokens, contextWindow int) bool {
	if previousTokens < 0 {
		previousTokens = 0
	}
	if currentTokens <= previousTokens || contextWindow <= 0 {
		return false
	}
	for _, ratio := range memoryExtractTriggerRatios {
		if ratio <= 0 {
			continue
		}
		threshold := int(float64(contextWindow) * ratio)
		if threshold > 0 && previousTokens < threshold && currentTokens >= threshold {
			return true
		}
	}
	return false
}

// containsExplicitMemoryDirective 检测用户是否明确要求记录长期偏好或约束。
func containsExplicitMemoryDirective(messages []model.Message) bool {
	for _, msg := range messages {
		if msg.Role != model.RoleUser {
			continue
		}
		text := strings.ToLower(model.TextFromParts(model.MessageParts(msg)))
		for _, marker := range []string{"remember", "always", "never", "记住", "以后", "每次", "必须", "不要"} {
			if strings.Contains(text, marker) {
				return true
			}
		}
	}
	return false
}

func configuredMemoryModel(cfg config.Config, sessionModel string) string {
	if strings.TrimSpace(cfg.Memory.Model) != "" {
		return cfg.Memory.Model
	}
	return strings.TrimSpace(sessionModel)
}

func memoryExtractSystemPrompt() string {
	return `You update Atlas long-term memory from completed agent sessions.
Return only a JSON object. Do not include markdown.
Keep durable memories that help future sessions: user preferences, project facts, and repeatable workflows.
Do not store transient chat, private chain-of-thought, generic programming advice, raw logs, secrets, API keys, or content that is only useful inside the current turn.
Use scope "global" only for durable user preferences that apply across projects. Use scope "project" for repository facts and workflows.
Use type "instruction", "fact", or "workflow".
If an existing memory is contradicted or obsolete, list its fingerprint in archive_fingerprints, but only when the fingerprint appears in the provided existing memories.
Schema: {"entries":[{"scope":"global|project","type":"instruction|fact|workflow","content":"...","source_note":"...","confidence":1-5}],"archive_fingerprints":["..."]}`
}

func memorySummarySystemPrompt() string {
	return `Summarize Atlas long-term memories for prompt injection.
Return only a JSON object. Do not include markdown.
Keep the summary compact, concrete, and useful for a coding agent. Preserve user instructions, stable project facts, and repeatable workflows.`
}

// loadExistingMemories 读取抽取任务需要对照的现有记忆，不更新使用统计。
func loadExistingMemories(ctx context.Context, store *memory.Store, projectKey string, limit int) ([]memory.Entry, error) {
	global, err := store.ListEntries(ctx, memory.ScopeGlobal, "")
	if err != nil {
		return nil, err
	}
	project, err := store.ListEntries(ctx, memory.ScopeProject, projectKey)
	if err != nil {
		return nil, err
	}
	entries := append(global, project...)
	if limit > 0 && len(entries) > limit {
		entries = entries[:limit]
	}
	return entries, nil
}

// memoryExtractPrompt 序列化现有记忆和新增消息，供模型判断增删改。
func memoryExtractPrompt(info session.Session, messages []model.Message, existing []memory.Entry, previousMessageCount int) string {
	var builder strings.Builder
	builder.WriteString("Session ID: ")
	builder.WriteString(info.ID)
	builder.WriteString("\nProject path: ")
	builder.WriteString(filepath.ToSlash(info.CWD))
	builder.WriteString("\nPreviously extracted message count: ")
	builder.WriteString(fmt.Sprintf("%d", previousMessageCount))
	if strings.TrimSpace(info.ContextSummary) != "" {
		builder.WriteString("\n\nCurrent compacted context summary:\n")
		builder.WriteString(trimRunes(info.ContextSummary, 4000))
		builder.WriteString("\n")
	}
	builder.WriteString("\n\nExisting memories with fingerprints:\n")
	if len(existing) == 0 {
		builder.WriteString("(none)\n")
	} else {
		for _, entry := range existing {
			builder.WriteString("- ")
			builder.WriteString(entry.Fingerprint)
			builder.WriteString(" [")
			builder.WriteString(entry.Scope)
			builder.WriteString("/")
			builder.WriteString(entry.Type)
			builder.WriteString("] ")
			builder.WriteString(entry.Content)
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\nNew transcript messages since the previous memory extraction:\n")
	builder.WriteString(formatMemoryTranscript(messages, memoryPromptMaxRunes))
	return builder.String()
}

// memorySummaryPrompt 序列化某个作用域的 active 记忆，供模型生成短摘要。
func memorySummaryPrompt(job memory.Job, entries []memory.Entry) string {
	var builder strings.Builder
	builder.WriteString("Scope: ")
	builder.WriteString(job.Scope)
	if job.ProjectPath != "" {
		builder.WriteString("\nProject path: ")
		builder.WriteString(filepath.ToSlash(job.ProjectPath))
	}
	builder.WriteString("\n\nActive memories:\n")
	for _, entry := range entries {
		builder.WriteString("- [")
		builder.WriteString(entry.Type)
		builder.WriteString("] ")
		builder.WriteString(entry.Content)
		if entry.SourceNote != "" {
			builder.WriteString(" (source: ")
			builder.WriteString(entry.SourceNote)
			builder.WriteString(")")
		}
		builder.WriteString("\n")
	}
	return trimRunes(builder.String(), memorySummaryMaxRunes)
}

func formatMemoryTranscript(messages []model.Message, maxRunes int) string {
	var builder strings.Builder
	for _, msg := range messages {
		builder.WriteString(string(msg.Role))
		builder.WriteString(": ")
		if len(msg.ToolCalls) > 0 {
			builder.WriteString(formatToolCalls(msg.ToolCalls))
		}
		content := strings.TrimSpace(formatMessageContentForMemory(msg))
		if msg.Role == model.RoleTool {
			content = trimRunes(content, 4000)
		}
		builder.WriteString(content)
		builder.WriteString("\n")
		if builder.Len() > maxRunes*4 {
			break
		}
	}
	return trimRunes(builder.String(), maxRunes)
}

func formatMessageContentForMemory(msg model.Message) string {
	parts := model.MessageParts(msg)
	if len(parts) == 0 {
		return msg.Content
	}
	var lines []string
	for _, part := range parts {
		switch part.Type {
		case model.ContentPartImage:
			mimeType := strings.TrimSpace(part.MimeType)
			if mimeType == "" {
				mimeType = "image"
			}
			detail := part.Detail
			if detail == "" {
				detail = model.ImageDetailAuto
			}
			lines = append(lines, fmt.Sprintf("[Image: %s, detail=%s]", mimeType, detail))
		default:
			if strings.TrimSpace(part.Text) != "" {
				lines = append(lines, part.Text)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func formatToolCalls(calls []model.ToolCall) string {
	var builder strings.Builder
	builder.WriteString("tool_calls=")
	for i, call := range calls {
		if i > 0 {
			builder.WriteString(", ")
		}
		builder.WriteString(call.Name)
		if call.Arguments != "" {
			builder.WriteString("(")
			builder.WriteString(trimRunes(call.Arguments, 800))
			builder.WriteString(")")
		}
	}
	if builder.Len() > 0 {
		builder.WriteString(" ")
	}
	return builder.String()
}

func normalizeMemoryExtractEntry(item memoryExtractEntry, projectKey, projectPath, sessionID string) (memory.Entry, bool) {
	item.Scope = strings.TrimSpace(item.Scope)
	item.Type = strings.TrimSpace(item.Type)
	item.Content = strings.TrimSpace(item.Content)
	if item.Content == "" {
		return memory.Entry{}, false
	}
	if item.Scope != memory.ScopeGlobal && item.Scope != memory.ScopeProject {
		return memory.Entry{}, false
	}
	if item.Type != memory.TypeInstruction && item.Type != memory.TypeFact && item.Type != memory.TypeWorkflow {
		return memory.Entry{}, false
	}
	entry := memory.Entry{
		Scope:           item.Scope,
		Type:            item.Type,
		Content:         item.Content,
		SourceNote:      strings.TrimSpace(item.SourceNote),
		Confidence:      item.Confidence,
		SourceSessionID: sessionID,
	}
	if entry.Scope == memory.ScopeProject {
		entry.ProjectKey = projectKey
		entry.ProjectPath = projectPath
	}
	return entry, true
}

func decodeJSONObject(content string, target any) error {
	content = strings.TrimSpace(content)
	start := strings.IndexByte(content, '{')
	end := strings.LastIndexByte(content, '}')
	if start < 0 || end < start {
		return fmt.Errorf("json object not found")
	}
	return json.Unmarshal([]byte(content[start:end+1]), target)
}

func memoryScopeKey(scope, projectKey string) string {
	return scope + "\x00" + projectKey
}

func trimRunes(content string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(content)
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "\n..."
}

func newMemoryWorkerID() string {
	var suffix [4]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return memoryWorkerIDPrefix + "unknown"
	}
	return fmt.Sprintf("%s%x", memoryWorkerIDPrefix, suffix[:])
}
