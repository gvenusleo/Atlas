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
	// Memory extraction triggers incrementally, avoiding background model token consumption on every turn.
	memoryExtractMinNewMessages    = 6
	memoryExtractMinNewInputTokens = 4000
)

// memoryDecayDays specifies the confidence decay rate by memory type (days per 1-point decay).
var memoryDecayDays = map[string]int{
	memory.TypeInstruction: 180,
	memory.TypeFact:        90,
	memory.TypeWorkflow:    120,
}

const memoryPruneUnusedDays = 90

var memoryWorkerIDPrefix = "atlas-memory-"
var memoryExtractTriggerRatios = []float64{0.4, 0.6, 0.8}

// ProcessMemoryJobs processes a limited number of background memory jobs, primarily for testing and short-lived entry points.
func (r *Runtime) ProcessMemoryJobs(ctx context.Context, limit int) (int, error) {
	if limit <= 0 {
		limit = memoryWorkerBatchSize
	}
	return r.processMemoryJobs(ctx, limit, newMemoryWorkerID())
}

// RunMemoryWorker continuously processes background memory jobs until ctx is cancelled.
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

// processMemoryJobs batch-claims background jobs and creates providers by job model; a single failure only affects that job.
func (r *Runtime) processMemoryJobs(ctx context.Context, limit int, workerID string) (int, error) {
	cfg, err := r.deps.LoadConfig()
	if err != nil {
		return 0, err
	}
	memStore, err := r.openMemoryStore(ctx, cfg.Session)
	if err != nil {
		return 0, err
	}
	sessionStore, err := r.openSessionStore(ctx, cfg.Session)
	if err != nil {
		return 0, err
	}

	processed := 0
	decayed := false
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
		if !decayed {
			if err := memStore.DecayConfidence(ctx, memoryDecayDays); err != nil {
				return processed, err
			}
			decayed = true
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
		if err := r.processMemoryExtractJob(ctx, memStore, sessionStore, provider, selectedModel, job); err != nil {
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

// processMemoryExtractJob extracts long-term memory from incremental messages since the last processing boundary.
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
	for _, item := range output.Entries {
		entry, ok := normalizeMemoryExtractEntry(item, projectKey, projectPath, info.ID)
		if !ok {
			continue
		}
		if _, err := memStore.UpsertEntry(ctx, entry); err != nil {
			return err
		}
	}
	var archiveFingerprints []string
	for _, fingerprint := range output.ArchiveFingerprints {
		fingerprint = strings.TrimSpace(fingerprint)
		if _, ok := existingByFingerprint[fingerprint]; !ok {
			continue
		}
		archiveFingerprints = append(archiveFingerprints, fingerprint)
	}
	if err := memStore.ArchiveFingerprints(ctx, archiveFingerprints); err != nil {
		return err
	}
	return sessionStore.SaveMemoryExtraction(ctx, job.SessionID, len(messages), inputTokens, inputHash)
}

type memoryExtractTriggerOptions struct {
	Force         bool
	ContextWindow int
}

// maybeEnqueueMemoryExtract schedules memory extraction based on incremental thresholds; failures do not affect the main turn.
func (r *Runtime) maybeEnqueueMemoryExtract(ctx context.Context, cfg config.SessionConfig, info session.Session, sessionID string, messages []model.Message, model string, opts memoryExtractTriggerOptions) {
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
	store, err := r.openMemoryStore(ctx, cfg)
	if err != nil {
		return
	}
	_ = store.EnqueueSessionExtract(ctx, sessionID, inputHash, model)
}

// shouldEnqueueMemoryExtract determines whether the new transcript has reached the background memory extraction threshold.
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

// crossedMemoryExtractRatio determines whether context usage has crossed a fixed threshold level.
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

// containsExplicitMemoryDirective detects whether the user explicitly requested recording a long-term preference or constraint.
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

// loadExistingMemories reads recent memories for extraction comparison without updating usage statistics.
func loadExistingMemories(ctx context.Context, store *memory.Store, projectKey string, limit int) ([]memory.Entry, error) {
	return store.ListRecentEntries(ctx, projectKey, limit)
}

// memoryExtractPrompt serializes existing memories and new messages for the model to determine additions, deletions, and updates.
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

// formatMemoryTranscript formats messages for memory extraction, keeping the most
// recent messages that fit within maxRunes to prioritize latest user intent.
func formatMemoryTranscript(messages []model.Message, maxRunes int) string {
	formatted := make([]string, 0, len(messages))
	for _, msg := range messages {
		var b strings.Builder
		b.WriteString(string(msg.Role))
		b.WriteString(": ")
		if len(msg.ToolCalls) > 0 {
			b.WriteString(formatToolCalls(msg.ToolCalls))
		}
		content := strings.TrimSpace(formatMessageContentForMemory(msg))
		if msg.Role == model.RoleTool {
			content = trimRunes(content, 4000)
		}
		b.WriteString(content)
		b.WriteString("\n")
		formatted = append(formatted, b.String())
	}
	// Keep the most recent messages that fit within maxRunes, preserving chronological order.
	var kept []string
	total := 0
	for i := len(formatted) - 1; i >= 0; i-- {
		n := len([]rune(formatted[i]))
		if total+n > maxRunes {
			break
		}
		kept = append(kept, formatted[i])
		total += n
	}
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	if len(kept) == 0 && len(formatted) > 0 {
		return trimRunes(formatted[len(formatted)-1], maxRunes)
	}
	return strings.Join(kept, "")
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
