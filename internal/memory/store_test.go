package memory

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/liuyuxin/atlas/internal/model"
)

func TestStoreUpsertAndSearch(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	projectKey, projectPath := ProjectIdentity(t.TempDir())
	entry, err := store.UpsertEntry(ctx, Entry{
		Scope:       ScopeProject,
		ProjectKey:  projectKey,
		ProjectPath: projectPath,
		Type:        TypeWorkflow,
		Content:     "Run go test ./... before committing Atlas changes.",
		Confidence:  4,
	})
	if err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}
	if entry.Fingerprint == "" || entry.Status != statusActive {
		t.Fatalf("entry = %#v", entry)
	}

	results, err := store.Search(ctx, projectPath, "go test Atlas", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 || results[0].Content != entry.Content {
		t.Fatalf("results = %#v", results)
	}
	missing, err := store.Search(ctx, projectPath, "unrelated query", 5)
	if err != nil {
		t.Fatalf("missing Search() error = %v", err)
	}
	if len(missing) != 0 {
		t.Fatalf("missing results = %#v", missing)
	}
	empty, err := store.Search(ctx, projectPath, "", 5)
	if err != nil {
		t.Fatalf("empty Search() error = %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty query should return no results, got %#v", empty)
	}
}

func TestStoreJobs(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	if err := store.EnqueueSessionExtract(ctx, "session-1", "hash-1", "deepseek-v4-pro"); err != nil {
		t.Fatalf("EnqueueSessionExtract() error = %v", err)
	}
	job, ok, err := store.ClaimNextJob(ctx, "worker-1", 0)
	if err != nil {
		t.Fatalf("ClaimNextJob() error = %v", err)
	}
	if !ok || job.SessionID != "session-1" || job.Model != "deepseek-v4-pro" || job.Attempts != 1 || job.WorkerID != "worker-1" {
		t.Fatalf("job = %#v, ok = %v", job, ok)
	}
	if err := store.CompleteJob(ctx, job); err != nil {
		t.Fatalf("CompleteJob() error = %v", err)
	}
	counts, err := store.Counts(ctx)
	if err != nil {
		t.Fatalf("Counts() error = %v", err)
	}
	if counts.Pending != 0 || counts.Failed != 0 {
		t.Fatalf("counts = %#v", counts)
	}
}

func TestTranscriptHashIncludesContentParts(t *testing.T) {
	base := []model.Message{{
		Role:    model.RoleUser,
		Content: "describe",
		Parts: []model.ContentPart{
			{Type: model.ContentPartText, Text: "describe"},
			{Type: model.ContentPartImage, MimeType: "image/png", DataURL: "data:image/png;base64,one", Detail: model.ImageDetailAuto},
		},
	}}
	changed := []model.Message{{
		Role:    model.RoleUser,
		Content: "describe",
		Parts: []model.ContentPart{
			{Type: model.ContentPartText, Text: "describe"},
			{Type: model.ContentPartImage, MimeType: "image/png", DataURL: "data:image/png;base64,two", Detail: model.ImageDetailAuto},
		},
	}}

	if TranscriptHash(base) == TranscriptHash(changed) {
		t.Fatal("TranscriptHash ignored image content parts")
	}
}

func TestClaimNextJobRetriesExpiredRunningLease(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	if err := store.EnqueueSessionExtract(ctx, "session-1", "hash-1", "deepseek-v4-flash"); err != nil {
		t.Fatalf("EnqueueSessionExtract() error = %v", err)
	}
	first, ok, err := store.ClaimNextJob(ctx, "worker-1", -time.Second)
	if err != nil {
		t.Fatalf("first ClaimNextJob() error = %v", err)
	}
	if !ok {
		t.Fatal("first ClaimNextJob() ok = false")
	}
	second, ok, err := store.ClaimNextJob(ctx, "worker-2", time.Minute)
	if err != nil {
		t.Fatalf("second ClaimNextJob() error = %v", err)
	}
	if !ok || second.Key != first.Key || second.Attempts != 2 {
		t.Fatalf("second = %#v, ok = %v", second, ok)
	}
}

func TestClaimNextJobSkipsActiveLease(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	if err := store.EnqueueSessionExtract(ctx, "session-1", "hash-1", "deepseek-v4-flash"); err != nil {
		t.Fatalf("EnqueueSessionExtract() error = %v", err)
	}
	if _, ok, err := store.ClaimNextJob(ctx, "worker-1", time.Minute); err != nil || !ok {
		t.Fatalf("first ClaimNextJob() ok = %v, err = %v", ok, err)
	}
	if job, ok, err := store.ClaimNextJob(ctx, "worker-2", time.Minute); err != nil || ok {
		t.Fatalf("second ClaimNextJob() job = %#v, ok = %v, err = %v", job, ok, err)
	}
}

func TestCompleteJobIgnoresStaleClaim(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	if err := store.EnqueueSessionExtract(ctx, "session-1", "hash-1", "deepseek-v4-flash"); err != nil {
		t.Fatalf("first EnqueueSessionExtract() error = %v", err)
	}
	oldJob, ok, err := store.ClaimNextJob(ctx, "worker-1", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextJob() error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextJob() ok = false")
	}
	if err := store.EnqueueSessionExtract(ctx, "session-1", "hash-2", "deepseek-v4-pro"); err != nil {
		t.Fatalf("second EnqueueSessionExtract() error = %v", err)
	}
	if err := store.CompleteJob(ctx, oldJob); err != nil {
		t.Fatalf("CompleteJob() error = %v", err)
	}
	newJob, ok, err := store.ClaimNextJob(ctx, "worker-2", time.Minute)
	if err != nil {
		t.Fatalf("second ClaimNextJob() error = %v", err)
	}
	if !ok || newJob.Model != "deepseek-v4-pro" {
		t.Fatalf("newJob = %#v, ok = %v", newJob, ok)
	}
}

func TestFailJobMarksDeadAfterMaxAttempts(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	if err := store.EnqueueSessionExtract(ctx, "session-1", "hash-1", "deepseek-v4-flash"); err != nil {
		t.Fatalf("EnqueueSessionExtract() error = %v", err)
	}
	job, ok, err := store.ClaimNextJob(ctx, "worker-1", time.Minute)
	if err != nil || !ok {
		t.Fatalf("ClaimNextJob() error = %v, ok = %v", err, ok)
	}
	// Bump attempts to the limit so the next FailJob transitions to dead.
	if _, err := store.db.ExecContext(ctx, `update memory_jobs set attempts = ? where job_key = ?`, maxJobAttempts, job.Key); err != nil {
		t.Fatalf("update attempts error = %v", err)
	}
	job.Attempts = maxJobAttempts
	if err := store.FailJob(ctx, job, nil); err != nil {
		t.Fatalf("FailJob() error = %v", err)
	}
	// Dead jobs should not be claimable.
	if _, ok, err := store.ClaimNextJob(ctx, "worker-2", time.Minute); err != nil || ok {
		t.Fatalf("expected dead job to not be claimable, got ok = %v, err = %v", ok, err)
	}
	counts, err := store.Counts(ctx)
	if err != nil {
		t.Fatalf("Counts() error = %v", err)
	}
	if counts.Failed != 1 {
		t.Fatalf("Failed = %d, want dead job included", counts.Failed)
	}
}

func TestDecayConfidenceSkipsRecentEntries(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	projectKey, projectPath := ProjectIdentity(t.TempDir())
	entry, err := store.UpsertEntry(ctx, Entry{
		Scope: ScopeProject, ProjectKey: projectKey, ProjectPath: projectPath,
		Type: TypeFact, Content: "recent fact", Confidence: 5,
	})
	if err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}
	// updated_at is recent (just created), should not decay.
	if err := store.DecayConfidence(ctx, map[string]int{TypeFact: 90}); err != nil {
		t.Fatalf("DecayConfidence() error = %v", err)
	}
	unchanged, _ := store.GetEntryByFingerprint(ctx, entry.Fingerprint)
	if unchanged.Confidence != 5 {
		t.Fatalf("expected confidence unchanged at 5, got %d", unchanged.Confidence)
	}
}

func TestListRecentEntriesOrdersByUsageThenRecency(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	projectKey, projectPath := ProjectIdentity(t.TempDir())

	// high-use: use_count=5, oldest updated_at
	// mid-use: use_count=2, middle updated_at
	// low-use: use_count=0, newest updated_at
	cases := []struct {
		Content  string
		UseCount int
		DaysAgo  int
	}{
		{"high use fact", 5, 30},
		{"mid use fact", 2, 20},
		{"low use fact", 0, 1},
	}
	var ids []int64
	for _, c := range cases {
		entry, err := store.UpsertEntry(ctx, Entry{
			Scope: ScopeProject, ProjectKey: projectKey, ProjectPath: projectPath,
			Type: TypeFact, Content: c.Content,
		})
		if err != nil {
			t.Fatalf("UpsertEntry() error = %v", err)
		}
		ids = append(ids, entry.ID)
	}
	for i, c := range cases {
		oldTime := time.Now().UTC().AddDate(0, 0, -c.DaysAgo).Format(time.RFC3339Nano)
		if _, err := store.db.ExecContext(ctx, `update memory_entries set use_count = ?, updated_at = ? where id = ?`, c.UseCount, oldTime, ids[i]); err != nil {
			t.Fatalf("backdate error = %v", err)
		}
	}

	results, err := store.ListRecentEntries(ctx, projectKey, 10)
	if err != nil {
		t.Fatalf("ListRecentEntries() error = %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
	// Order by use_count desc: high(5), mid(2), low(0), despite high being oldest.
	if results[0].Content != "high use fact" || results[1].Content != "mid use fact" || results[2].Content != "low use fact" {
		t.Fatalf("expected order by use_count: high, mid, low; got %s, %s, %s",
			results[0].Content, results[1].Content, results[2].Content)
	}
}

func TestListRecentEntriesIncludesGlobalScope(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	projectKey, projectPath := ProjectIdentity(t.TempDir())

	if _, err := store.UpsertEntry(ctx, Entry{
		Scope: ScopeGlobal, Type: TypeInstruction, Content: "global preference",
	}); err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}
	if _, err := store.UpsertEntry(ctx, Entry{
		Scope: ScopeProject, ProjectKey: projectKey, ProjectPath: projectPath,
		Type: TypeFact, Content: "project fact",
	}); err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}

	results, err := store.ListRecentEntries(ctx, projectKey, 10)
	if err != nil {
		t.Fatalf("ListRecentEntries() error = %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	contents := []string{results[0].Content, results[1].Content}
	if !contains(contents, "global preference") || !contains(contents, "project fact") {
		t.Fatalf("expected both global and project entries, got %v", contents)
	}
}

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

func TestSearchRanksByConfidenceAndUsage(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	projectKey, projectPath := ProjectIdentity(t.TempDir())

	// Two entries with the same distinctive tokens so BM25 scores are close.
	if _, err := store.UpsertEntry(ctx, Entry{
		Scope: ScopeProject, ProjectKey: projectKey, ProjectPath: projectPath,
		Type: TypeFact, Content: "atlas test command runs tests", Confidence: 1,
	}); err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}
	if _, err := store.UpsertEntry(ctx, Entry{
		Scope: ScopeProject, ProjectKey: projectKey, ProjectPath: projectPath,
		Type: TypeFact, Content: "atlas test coverage shows results", Confidence: 5,
	}); err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}

	results, err := store.Search(ctx, projectPath, "atlas test", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 results, got %d", len(results))
	}
	if results[0].Confidence < results[1].Confidence {
		t.Fatalf("expected higher confidence first, got %d then %d", results[0].Confidence, results[1].Confidence)
	}
}

func TestSearchChineseSubstring(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	projectKey, projectPath := ProjectIdentity(t.TempDir())

	if _, err := store.UpsertEntry(ctx, Entry{
		Scope: ScopeProject, ProjectKey: projectKey, ProjectPath: projectPath,
		Type: TypeFact, Content: "ACP删除会话会从数据库中删除", Confidence: 3,
	}); err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}
	if _, err := store.UpsertEntry(ctx, Entry{
		Scope: ScopeProject, ProjectKey: projectKey, ProjectPath: projectPath,
		Type: TypeFact, Content: "SQLite FTS5 对中文分词无效", Confidence: 3,
	}); err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}

	// Chinese substring search should match.
	results, err := store.Search(ctx, projectPath, "删除会话", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 || !strings.Contains(results[0].Content, "删除会话") {
		t.Fatalf("expected 1 Chinese match, got %v", results)
	}

	// Mixed Chinese/English query.
	results, err = store.Search(ctx, projectPath, "SQLite 中文", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 || !strings.Contains(results[0].Content, "FTS5") {
		t.Fatalf("expected 1 mixed match, got %v", results)
	}

	// Unrelated Chinese query returns nothing.
	results, err = store.Search(ctx, projectPath, "网络协议", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results for unrelated query, got %v", results)
	}
}

func TestSearchKeepsAtLeastOne(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	projectKey, projectPath := ProjectIdentity(t.TempDir())

	if _, err := store.UpsertEntry(ctx, Entry{
		Scope: ScopeProject, ProjectKey: projectKey, ProjectPath: projectPath,
		Type: TypeFact, Content: "unique alpha beta", Confidence: 3,
	}); err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}

	results, err := store.Search(ctx, projectPath, "alpha beta", 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) < 1 {
		t.Fatalf("expected at least 1 result, got %d", len(results))
	}
}

func TestDecayConfidenceReducesOldEntries(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	projectKey, projectPath := ProjectIdentity(t.TempDir())

	entry, err := store.UpsertEntry(ctx, Entry{
		Scope: ScopeProject, ProjectKey: projectKey, ProjectPath: projectPath,
		Type: TypeFact, Content: "old fact", Confidence: 5,
	})
	if err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}

	// Backdate updated_at by 100 days.
	oldTime := time.Now().UTC().AddDate(0, 0, -100).Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(ctx, `update memory_entries set updated_at = ? where id = ?`, oldTime, entry.ID); err != nil {
		t.Fatalf("backdate error = %v", err)
	}

	// fact decays 1 point per 90 days; 100 days -> 1 point decay.
	if err := store.DecayConfidence(ctx, map[string]int{TypeFact: 90}); err != nil {
		t.Fatalf("DecayConfidence() error = %v", err)
	}

	decayed, err := store.GetEntryByFingerprint(ctx, entry.Fingerprint)
	if err != nil {
		t.Fatalf("GetEntryByFingerprint() error = %v", err)
	}
	if decayed.Confidence != 4 {
		t.Fatalf("expected confidence 4, got %d", decayed.Confidence)
	}
}

func TestDecayConfidenceFloorsAtOne(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	projectKey, projectPath := ProjectIdentity(t.TempDir())

	entry, err := store.UpsertEntry(ctx, Entry{
		Scope: ScopeProject, ProjectKey: projectKey, ProjectPath: projectPath,
		Type: TypeFact, Content: "very old fact", Confidence: 2,
	})
	if err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}

	oldTime := time.Now().UTC().AddDate(0, 0, -365).Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(ctx, `update memory_entries set updated_at = ? where id = ?`, oldTime, entry.ID); err != nil {
		t.Fatalf("backdate error = %v", err)
	}

	if err := store.DecayConfidence(ctx, map[string]int{TypeFact: 30}); err != nil {
		t.Fatalf("DecayConfidence() error = %v", err)
	}

	decayed, err := store.GetEntryByFingerprint(ctx, entry.Fingerprint)
	if err != nil {
		t.Fatalf("GetEntryByFingerprint() error = %v", err)
	}
	if decayed.Confidence != 1 {
		t.Fatalf("expected confidence floored at 1, got %d", decayed.Confidence)
	}
}

func TestDecayConfidenceByType(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	projectKey, projectPath := ProjectIdentity(t.TempDir())

	fact, err := store.UpsertEntry(ctx, Entry{
		Scope: ScopeProject, ProjectKey: projectKey, ProjectPath: projectPath,
		Type: TypeFact, Content: "fact entry", Confidence: 5,
	})
	if err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}
	instruction, err := store.UpsertEntry(ctx, Entry{
		Scope: ScopeProject, ProjectKey: projectKey, ProjectPath: projectPath,
		Type: TypeInstruction, Content: "instruction entry", Confidence: 5,
	})
	if err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}

	oldTime := time.Now().UTC().AddDate(0, 0, -100).Format(time.RFC3339Nano)
	for _, id := range []int64{fact.ID, instruction.ID} {
		if _, err := store.db.ExecContext(ctx, `update memory_entries set updated_at = ? where id = ?`, oldTime, id); err != nil {
			t.Fatalf("backdate error = %v", err)
		}
	}

	// fact: 90 days/point -> 1 decay; instruction: 180 days/point -> 0 decay.
	if err := store.DecayConfidence(ctx, map[string]int{TypeFact: 90, TypeInstruction: 180}); err != nil {
		t.Fatalf("DecayConfidence() error = %v", err)
	}

	decayedFact, _ := store.GetEntryByFingerprint(ctx, fact.Fingerprint)
	decayedInst, _ := store.GetEntryByFingerprint(ctx, instruction.Fingerprint)
	if decayedFact.Confidence != 4 {
		t.Fatalf("fact expected confidence 4, got %d", decayedFact.Confidence)
	}
	if decayedInst.Confidence != 5 {
		t.Fatalf("instruction expected confidence 5, got %d", decayedInst.Confidence)
	}
}

func TestUpsertAfterDecayRestoresConfidence(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	projectKey, projectPath := ProjectIdentity(t.TempDir())

	entry, err := store.UpsertEntry(ctx, Entry{
		Scope: ScopeProject, ProjectKey: projectKey, ProjectPath: projectPath,
		Type: TypeFact, Content: "repeated fact", Confidence: 5,
	})
	if err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}

	oldTime := time.Now().UTC().AddDate(0, 0, -200).Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(ctx, `update memory_entries set updated_at = ? where id = ?`, oldTime, entry.ID); err != nil {
		t.Fatalf("backdate error = %v", err)
	}

	if err := store.DecayConfidence(ctx, map[string]int{TypeFact: 90}); err != nil {
		t.Fatalf("DecayConfidence() error = %v", err)
	}

	// Re-upsert with high confidence; should take max(decayed, new).
	restored, err := store.UpsertEntry(ctx, Entry{
		Scope: ScopeProject, ProjectKey: projectKey, ProjectPath: projectPath,
		Type: TypeFact, Content: "repeated fact", Confidence: 5,
	})
	if err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}
	if restored.Confidence != 5 {
		t.Fatalf("expected confidence restored to 5, got %d", restored.Confidence)
	}
}

func TestPruneStaleEntriesArchivesOldUnused(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	projectKey, projectPath := ProjectIdentity(t.TempDir())

	entry, err := store.UpsertEntry(ctx, Entry{
		Scope: ScopeProject, ProjectKey: projectKey, ProjectPath: projectPath,
		Type: TypeFact, Content: "stale unused low confidence", Confidence: 1,
	})
	if err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}

	oldTime := time.Now().UTC().AddDate(0, 0, -100).Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(ctx, `update memory_entries set updated_at = ? where id = ?`, oldTime, entry.ID); err != nil {
		t.Fatalf("backdate error = %v", err)
	}

	n, err := store.PruneStaleEntries(ctx, 90)
	if err != nil {
		t.Fatalf("PruneStaleEntries() error = %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 pruned, got %d", n)
	}
	pruned, _ := store.GetEntryByFingerprint(ctx, entry.Fingerprint)
	if pruned.Status != statusArchived {
		t.Fatalf("expected status archived, got %s", pruned.Status)
	}
}

func TestPruneStaleEntriesKeepsUsedEntries(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	projectKey, projectPath := ProjectIdentity(t.TempDir())

	entry, err := store.UpsertEntry(ctx, Entry{
		Scope: ScopeProject, ProjectKey: projectKey, ProjectPath: projectPath,
		Type: TypeFact, Content: "used entry", Confidence: 1,
	})
	if err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}
	// Mark as used via Search.
	_, _ = store.Search(ctx, projectPath, "used", 5)

	oldTime := time.Now().UTC().AddDate(0, 0, -100).Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(ctx, `update memory_entries set updated_at = ? where id = ?`, oldTime, entry.ID); err != nil {
		t.Fatalf("backdate error = %v", err)
	}

	n, err := store.PruneStaleEntries(ctx, 90)
	if err != nil {
		t.Fatalf("PruneStaleEntries() error = %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 pruned, got %d", n)
	}
}

func TestPruneStaleEntriesKeepsHighConfidence(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	projectKey, projectPath := ProjectIdentity(t.TempDir())

	entry, err := store.UpsertEntry(ctx, Entry{
		Scope: ScopeProject, ProjectKey: projectKey, ProjectPath: projectPath,
		Type: TypeFact, Content: "high confidence old unused", Confidence: 3,
	})
	if err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}

	oldTime := time.Now().UTC().AddDate(0, 0, -100).Format(time.RFC3339Nano)
	if _, err := store.db.ExecContext(ctx, `update memory_entries set updated_at = ? where id = ?`, oldTime, entry.ID); err != nil {
		t.Fatalf("backdate error = %v", err)
	}

	n, err := store.PruneStaleEntries(ctx, 90)
	if err != nil {
		t.Fatalf("PruneStaleEntries() error = %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 pruned, got %d", n)
	}
}

func TestPruneStaleEntriesKeepsRecentEntries(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	projectKey, projectPath := ProjectIdentity(t.TempDir())

	entry, err := store.UpsertEntry(ctx, Entry{
		Scope: ScopeProject, ProjectKey: projectKey, ProjectPath: projectPath,
		Type: TypeFact, Content: "recent low confidence unused", Confidence: 1,
	})
	if err != nil {
		t.Fatalf("UpsertEntry() error = %v", err)
	}
	_ = entry
	// updated_at is recent (just created), so should not be pruned.
	n, err := store.PruneStaleEntries(ctx, 90)
	if err != nil {
		t.Fatalf("PruneStaleEntries() error = %v", err)
	}
	if n != 0 {
		t.Fatalf("expected 0 pruned, got %d", n)
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := Open(filepath.Join(t.TempDir(), "atlas.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	if err := store.EnsureSchema(context.Background()); err != nil {
		t.Fatal(err)
	}
	return store
}
