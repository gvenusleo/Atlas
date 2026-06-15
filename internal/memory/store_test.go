package memory

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreUpsertSearchAndPromptContext(t *testing.T) {
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

	results, err := store.Search(ctx, projectPath, "go test committing", 5)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 || results[0].Content != entry.Content {
		t.Fatalf("results = %#v", results)
	}
	contextText, err := store.PromptContext(ctx, projectPath, "commit")
	if err != nil {
		t.Fatalf("PromptContext() error = %v", err)
	}
	if !strings.Contains(contextText, "Run go test ./...") {
		t.Fatalf("context = %q", contextText)
	}
}

func TestStoreJobs(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	if err := store.EnqueueSessionExtract(ctx, "session-1", "/tmp/project", "hash-1", "deepseek-v4-pro"); err != nil {
		t.Fatalf("EnqueueSessionExtract() error = %v", err)
	}
	job, ok, err := store.ClaimNextJob(ctx, "worker-1", 0)
	if err != nil {
		t.Fatalf("ClaimNextJob() error = %v", err)
	}
	if !ok || job.Kind != JobKindSessionExtract || job.SessionID != "session-1" || job.Model != "deepseek-v4-pro" || job.Attempts != 1 || job.WorkerID != "worker-1" {
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

func TestClaimNextJobRetriesExpiredRunningLease(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	if err := store.EnqueueSessionExtract(ctx, "session-1", "/tmp/project", "hash-1", "deepseek-v4-flash"); err != nil {
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

	if err := store.EnqueueSessionExtract(ctx, "session-1", "/tmp/project", "hash-1", "deepseek-v4-flash"); err != nil {
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

	if err := store.EnqueueSessionExtract(ctx, "session-1", "/tmp/project", "hash-1", "deepseek-v4-flash"); err != nil {
		t.Fatalf("first EnqueueSessionExtract() error = %v", err)
	}
	oldJob, ok, err := store.ClaimNextJob(ctx, "worker-1", time.Minute)
	if err != nil {
		t.Fatalf("ClaimNextJob() error = %v", err)
	}
	if !ok {
		t.Fatal("ClaimNextJob() ok = false")
	}
	if err := store.EnqueueSessionExtract(ctx, "session-1", "/tmp/project", "hash-2", "deepseek-v4-pro"); err != nil {
		t.Fatalf("second EnqueueSessionExtract() error = %v", err)
	}
	if err := store.CompleteJob(ctx, oldJob); err != nil {
		t.Fatalf("CompleteJob() error = %v", err)
	}
	newJob, ok, err := store.ClaimNextJob(ctx, "worker-2", time.Minute)
	if err != nil {
		t.Fatalf("second ClaimNextJob() error = %v", err)
	}
	if !ok || newJob.InputHash != "hash-2" || newJob.Model != "deepseek-v4-pro" {
		t.Fatalf("newJob = %#v, ok = %v", newJob, ok)
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
