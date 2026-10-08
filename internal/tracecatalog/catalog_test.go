package tracecatalog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func putFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func discover(t *testing.T, root string) *Catalog {
	t.Helper()
	c, err := Discover(context.Background(), root, DiscoverOptions{})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func plan(t *testing.T, c *Catalog, artifact string, object string, window *Window) QueryRecord {
	t.Helper()
	q, err := c.RegisterPlannedQuery(context.Background(), Plan{ArtifactID: artifact, Object: Object{Kind: "thread", ID: object, Scope: "process:9"}, View: "resource_stack", Parameters: json.RawMessage(`{"thread":"` + object + `","limit":40}`), Window: window})
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func bind(t *testing.T, c *Catalog, a Artifact) {
	t.Helper()
	if err := c.BindSource(context.Background(), a.ID, "source-universe:"+a.RevisionID, func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func TestWindowBoundaryUnknownExclusiveInclusiveStayDistinctAndDetached(t *testing.T) {
	root := t.TempDir()
	putFile(t, filepath.Join(root, "capture"), "trace")
	c := discover(t, root)
	a := c.Snapshot().Artifacts[0]
	unknown := plan(t, c, a.ID, "101", &Window{StartNS: 0, EndNS: 50})
	exclusive, inclusive := false, true
	excluded := plan(t, c, a.ID, "101", &Window{StartNS: 0, EndNS: 50, EndInclusive: &exclusive})
	included := plan(t, c, a.ID, "101", &Window{StartNS: 0, EndNS: 50, EndInclusive: &inclusive})
	if unknown.ID == excluded.ID || unknown.ID == included.ID || excluded.ID == included.ID {
		t.Fatal("unknown, exclusive and inclusive endpoint contracts collapsed")
	}
	for _, row := range []QueryRecord{unknown, excluded, included} {
		data, err := json.Marshal(row)
		if err != nil {
			t.Fatal(err)
		}
		var decoded QueryRecord
		if err := json.Unmarshal(data, &decoded); err != nil || !reflect.DeepEqual(decoded.Window, row.Window) {
			t.Fatalf("boundary contract did not survive JSON: %s, %v", data, err)
		}
	}
	exclusive, inclusive = true, false
	*excluded.Window.EndInclusive = true
	*included.Window.EndInclusive = false
	for id, want := range map[string]bool{excluded.ID: false, included.ID: true} {
		row, _ := c.Query(id)
		if row.Window.EndInclusive == nil || *row.Window.EndInclusive != want {
			t.Fatal("caller mutated stored endpoint contract")
		}
		*row.Window.EndInclusive = !want
		next, _ := c.Clone().Query(id)
		if next.Window.EndInclusive == nil || *next.Window.EndInclusive != want {
			t.Fatal("query or clone shared endpoint pointer")
		}
	}
}

func finish(t *testing.T, c *Catalog, q QueryRecord, outcome Outcome) QueryRecord {
	t.Helper()
	completion := Completion{Outcome: outcome}
	if outcome == OutcomeSuccess || outcome == OutcomeEmpty {
		completion.SourceRevision = "source-universe:" + q.ArtifactRevision
		completion.PayloadRef = "/private/results/" + q.ID + ".json"
		completion.RawRef = "/private/results/" + q.ID + ".raw"
	} else {
		completion.Error = &Issue{Code: "fixture_failure", Message: "original failure"}
	}
	row, err := c.Complete(context.Background(), q.ID, completion)
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func TestDiscoverRecursesAllCandidatesWithoutStemOrExtensionAdmission(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"capture.trace", "capture.db", "nested/capture.trace", "nested/deep/no-extension", ".hidden/capture.trace", "unsupported.dat"} {
		putFile(t, filepath.Join(root, name), "source "+name)
	}
	outside := t.TempDir()
	putFile(t, filepath.Join(outside, "secret.trace"), "not authorized")
	if err := os.Symlink(outside, filepath.Join(root, "outside-dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.trace"), filepath.Join(root, "outside-file.trace")); err != nil {
		t.Fatal(err)
	}
	c := discover(t, root)
	s := c.Snapshot()
	if !s.Discovery.Complete || len(s.Artifacts) != 6 || s.Discovery.SymlinksSkipped != 2 {
		t.Fatalf("discovery=%+v artifacts=%+v", s.Discovery, s.Artifacts)
	}
	ids := map[string]bool{}
	for _, a := range s.Artifacts {
		if a.Status != "candidate" || a.RevisionID == "" || ids[a.ID] {
			t.Fatalf("invalid candidate %+v", a)
		}
		ids[a.ID] = true
	}
	if next := discover(t, root).Snapshot(); next.ID != s.ID || !reflect.DeepEqual(next.Artifacts, s.Artifacts) {
		t.Fatal("unchanged roster identity is unstable")
	}
	if !s.NavigationOnly {
		t.Fatal("snapshot acquired evidence authority")
	}
}

func TestDiscoveryBoundsFilterAndNonRecursiveAreExplicit(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"a", "b", "deep/c", "deep/d"} {
		putFile(t, filepath.Join(root, name), name)
	}
	c, err := Discover(context.Background(), root, DiscoverOptions{MaxArtifacts: 2})
	if err != nil {
		t.Fatal(err)
	}
	s := c.Snapshot()
	if s.Discovery.Complete || !s.Discovery.Truncated || len(s.Artifacts) != 2 || s.Discovery.RegularFilesSeen != 4 {
		t.Fatalf("artifact limit hidden: %+v", s)
	}
	c, err = Discover(context.Background(), root, DiscoverOptions{MaxEntries: 1})
	if err != nil {
		t.Fatal(err)
	}
	if d := c.Snapshot().Discovery; d.Complete || !d.Truncated || d.EntriesVisited != 1 {
		t.Fatalf("entry limit hidden: %+v", d)
	}
	c, err = Discover(context.Background(), root, DiscoverOptions{NonRecursive: true})
	if err != nil {
		t.Fatal(err)
	}
	if s := c.Snapshot(); !s.Discovery.Complete || s.Discovery.Recursive || s.Discovery.SubdirectoriesSkipped != 1 || len(s.Artifacts) != 2 {
		t.Fatalf("nonrecursive scope hidden: %+v", s)
	}
	c, err = Discover(context.Background(), root, DiscoverOptions{FilterLabel: "a only", Filter: func(rel string) bool { return rel == "a" }})
	if err != nil {
		t.Fatal(err)
	}
	if s := c.Snapshot(); s.Discovery.FilteredFiles != 3 || len(s.Artifacts) != 1 || s.Discovery.FilterLabel != "a only" {
		t.Fatalf("filter hidden: %+v", s)
	}
	if _, err := Discover(context.Background(), root, DiscoverOptions{Filter: func(string) bool { return true }}); err == nil {
		t.Fatal("unlabelled filter admitted")
	}
}

func TestDiscoveryCancellationReturnsPartialAndMutationNeverComplete(t *testing.T) {
	root := t.TempDir()
	putFile(t, filepath.Join(root, "a"), "a")
	putFile(t, filepath.Join(root, "b"), "b")
	ctx, cancel := context.WithCancel(context.Background())
	c, err := Discover(ctx, root, DiscoverOptions{FilterLabel: "cancel fixture", Filter: func(string) bool { cancel(); return true }})
	if !errors.Is(err, context.Canceled) || c == nil || c.Snapshot().Discovery.Complete || !c.Snapshot().Discovery.Canceled {
		t.Fatalf("cancel c=%v err=%v", c, err)
	}
	changed := false
	c, err = Discover(context.Background(), root, DiscoverOptions{FilterLabel: "mutation fixture", Filter: func(string) bool {
		if !changed {
			changed = true
			putFile(t, filepath.Join(root, "new"), "new")
		}
		return true
	}})
	if err != nil {
		t.Fatal(err)
	}
	if c.Snapshot().Discovery.Complete || c.Snapshot().Discovery.IssueCount == 0 {
		t.Fatal("changed directory claimed complete")
	}
}

func TestPerObjectPlansOutcomesReorderedAndFailedRetryHistory(t *testing.T) {
	root := t.TempDir()
	putFile(t, filepath.Join(root, "one/capture.trace"), "one")
	putFile(t, filepath.Join(root, "two/capture.trace"), "two")
	c := discover(t, root)
	artifacts := c.Snapshot().Artifacts
	for _, a := range artifacts {
		bind(t, c, a)
	}
	w := &Window{StartNS: 0, EndNS: 50_000_000}
	q0 := plan(t, c, artifacts[0].ID, "101", w)
	q1 := plan(t, c, artifacts[1].ID, "101", w)
	q2 := plan(t, c, artifacts[0].ID, "202", w)
	q3 := plan(t, c, artifacts[1].ID, "202", w)
	if q0.ID == q1.ID || q0.ID == q2.ID {
		t.Fatal("object/capture scopes collapsed")
	}
	finish(t, c, q3, OutcomeEmpty)
	finish(t, c, q1, OutcomeFailure)
	finish(t, c, q0, OutcomeSuccess)
	retry := finish(t, c, q1, OutcomeSuccess)
	if len(retry.Attempts) != 2 || retry.Attempts[0].Outcome != OutcomeFailure || retry.Attempts[0].Error.Message != "original failure" || retry.Attempts[1].Outcome != OutcomeSuccess {
		t.Fatalf("failure overwritten: %+v", retry)
	}
	if pending, _ := c.Query(q2.ID); pending.Outcome != OutcomeNotExecuted || len(pending.Attempts) != 0 {
		t.Fatalf("unexecuted object inferred complete: %+v", pending)
	}
	if empty, _ := c.Query(q3.ID); empty.Outcome != OutcomeEmpty || empty.ArtifactID != artifacts[1].ID {
		t.Fatalf("empty outcome/order association lost: %+v", empty)
	}
	inclusive := true
	qChangedWindow := plan(t, c, artifacts[0].ID, "101", &Window{StartNS: 0, EndNS: 50_000_000, EndInclusive: &inclusive})
	qUnbounded := plan(t, c, artifacts[0].ID, "101", nil)
	if qChangedWindow.ID == q0.ID || qUnbounded.ID == q0.ID {
		t.Fatal("exact window/unknown window collapsed")
	}
}

func TestPlanCanonicalJSONPrecisionAndAmbiguity(t *testing.T) {
	root := t.TempDir()
	putFile(t, filepath.Join(root, "trace"), "trace")
	c := discover(t, root)
	p := Plan{ArtifactID: c.Snapshot().Artifacts[0].ID, Object: Object{Kind: "thread", ID: "101"}, View: "resource_stack", Parameters: json.RawMessage(`{"large":9007199254740993,"filter":{"a":true}}`)}
	q, err := c.RegisterPlannedQuery(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	p.Parameters = json.RawMessage(` { "filter": {"a":true}, "large":9007199254740993 } `)
	q2, err := c.RegisterPlannedQuery(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	if q2.ID != q.ID || !strings.Contains(string(q2.Parameters), "9007199254740993") {
		t.Fatal("canonical parameters rounded/lost identity")
	}
	for _, bad := range []string{`{"a":1,"a":2}`, `{"nested":{"a":1,"a":2}}`, `[]`, `{} {}`, `null`} {
		p.Parameters = json.RawMessage(bad)
		if _, err := c.RegisterPlannedQuery(context.Background(), p); err == nil {
			t.Fatalf("ambiguous params accepted %s", bad)
		}
	}
	p.Parameters = nil
	p.Window = &Window{StartNS: 10, EndNS: 10}
	if _, err := c.RegisterPlannedQuery(context.Background(), p); err == nil {
		t.Fatal("empty right-open point admitted")
	}
}

func TestSourceReplacementAndUniverseChangeInvalidateNotReuse(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "trace")
	putFile(t, path, "same")
	c := discover(t, root)
	a := c.Snapshot().Artifacts[0]
	bind(t, c, a)
	q := plan(t, c, a.ID, "101", nil)
	finish(t, c, q, OutcomeSuccess)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	replacement := filepath.Join(root, "replacement")
	putFile(t, replacement, "same")
	if err := os.Chtimes(replacement, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Resolve(context.Background(), a.ID); !errors.Is(err, ErrStale) {
		t.Fatalf("replacement reused: %v", err)
	}
	old, _ := c.Query(q.ID)
	if old.Outcome != OutcomeStale || len(old.Attempts) < 2 || old.Attempts[0].Outcome != OutcomeSuccess {
		t.Fatalf("stale source lost history: %+v", old)
	}
	fresh := discover(t, root).Snapshot().Artifacts[0]
	if fresh.ID != a.ID || fresh.RevisionID == a.RevisionID {
		t.Fatal("artifact identity and revision not independent")
	}
	if err := c.BindSource(context.Background(), a.ID, "new", func(context.Context) error { return nil }); !errors.Is(err, ErrStale) {
		t.Fatal("stale source revived")
	}

	c = discover(t, root)
	a = c.Snapshot().Artifacts[0]
	invalid := false
	if err := c.BindSource(context.Background(), a.ID, "manifest-and-all-children", func(context.Context) error {
		if invalid {
			return errors.New("bundle member changed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	invalid = true
	if _, err := c.Resolve(context.Background(), a.ID); !errors.Is(err, ErrStale) {
		t.Fatalf("source universe not checked: %v", err)
	}
}

func TestSuccessfulCompletionRequiresLiveSourceAndOriginalPayload(t *testing.T) {
	root := t.TempDir()
	putFile(t, filepath.Join(root, "trace"), "trace")
	c := discover(t, root)
	a := c.Snapshot().Artifacts[0]
	q := plan(t, c, a.ID, "101", nil)
	completion := Completion{Outcome: OutcomeSuccess, SourceRevision: "forged", PayloadRef: "/result.json"}
	if _, err := c.Complete(context.Background(), q.ID, completion); err == nil {
		t.Fatal("unbound success admitted")
	}
	bind(t, c, a)
	if _, err := c.Complete(context.Background(), q.ID, completion); err == nil {
		t.Fatal("wrong source revision admitted")
	}
	completion.SourceRevision = "source-universe:" + a.RevisionID
	completion.PayloadRef = ""
	if _, err := c.Complete(context.Background(), q.ID, completion); err == nil {
		t.Fatal("payload-free success admitted")
	}
	completion.PayloadRef = "/result.json"
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	row, err := c.Complete(ctx, q.ID, completion)
	if !errors.Is(err, context.Canceled) || row.Outcome != OutcomeCanceled {
		t.Fatalf("canceled result became success: %+v %v", row, err)
	}
	completion.PayloadRef = ""
	completion.RawRef = "/raw-original-query-output.txt"
	row, err = c.Complete(context.Background(), q.ID, completion)
	if err != nil || row.Outcome != OutcomeSuccess || row.Attempts[len(row.Attempts)-1].PayloadRef != "" {
		t.Fatalf("raw-only reference was dropped or relabeled as payload: %+v %v", row, err)
	}
}

func TestLiveCloneIsolationMergeReorderAndForeignRunRejection(t *testing.T) {
	root := t.TempDir()
	putFile(t, filepath.Join(root, "trace"), "trace")
	c := discover(t, root)
	a := c.Snapshot().Artifacts[0]
	bind(t, c, a)
	q := plan(t, c, a.ID, "101", nil)
	one, two := c.Clone(), c.Clone()
	finish(t, one, q, OutcomeFailure)
	finish(t, two, q, OutcomeSuccess)
	if untouched, _ := c.Query(q.ID); untouched.Outcome != OutcomeNotExecuted {
		t.Fatal("fork mutated parent")
	}
	if err := c.Merge(two); err != nil {
		t.Fatal(err)
	}
	if err := c.Merge(one); err != nil {
		t.Fatal(err)
	}
	if err := c.Merge(two); err != nil {
		t.Fatal(err)
	}
	merged, _ := c.Query(q.ID)
	if len(merged.Attempts) != 2 || merged.Outcome != OutcomeSuccess || merged.Attempts[0].Outcome != OutcomeFailure {
		t.Fatalf("merge dropped/reordered attempts: %+v", merged)
	}
	if err := c.Merge(discover(t, root)); !errors.Is(err, ErrForeignRun) {
		t.Fatalf("same stable ID restored live authority: %v", err)
	}
	s := c.Snapshot()
	s.Queries[0].Parameters[0] = '!'
	s.Queries[0].Attempts[0].Error.Message = "mutated"
	s.Artifacts[0].Status = "changed"
	if got := c.Snapshot(); got.Artifacts[0].Status != "candidate" || got.Queries[0].Attempts[0].Error.Message != "original failure" {
		t.Fatal("snapshot aliases mutable state")
	}
}

func TestConcurrentPlansAndCompletionsRemainAssociated(t *testing.T) {
	root := t.TempDir()
	putFile(t, filepath.Join(root, "trace"), "trace")
	c := discover(t, root)
	a := c.Snapshot().Artifacts[0]
	bind(t, c, a)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			q := plan(t, c, a.ID, fmt.Sprint(i), nil)
			finish(t, c, q, OutcomeSuccess)
		}(i)
	}
	wg.Wait()
	s := c.Snapshot()
	if len(s.Queries) != 20 {
		t.Fatalf("lost queries: %d", len(s.Queries))
	}
	if err := validateSnapshot(s); err != nil {
		t.Fatal(err)
	}
}

func TestDeclaredExpectationIsPrivateAndNotRestoredByRetry(t *testing.T) {
	root := t.TempDir()
	putFile(t, filepath.Join(root, "trace"), "trace")
	c := discover(t, root)
	a := c.Snapshot().Artifacts[0]
	q := plan(t, c, a.ID, "101", nil)
	finish(t, c, q, OutcomeFailure)
	if _, err := c.RegisterPlannedQuery(context.Background(), q.Plan); err != nil {
		t.Fatal(err)
	}
	if c.IsDeclaredQuery(q.ID) {
		t.Fatal("ordinary repeat acquired declared authority")
	}
	declared, err := c.RegisterDeclaredQuery(context.Background(), Plan{ArtifactID: a.ID, Object: Object{Kind: "thread", ID: "202"}, View: "resource_stack"})
	if err != nil {
		t.Fatal(err)
	}
	clone := c.Clone()
	if !clone.IsDeclaredQuery(declared.ID) || clone.IsDeclaredQuery(q.ID) {
		t.Fatal("clone altered declared boundary")
	}
	extra, err := clone.RegisterDeclaredQuery(context.Background(), Plan{ArtifactID: a.ID, Object: Object{Kind: "thread", ID: "303"}, View: "resource_stack"})
	if err != nil {
		t.Fatal(err)
	}
	if c.IsDeclaredQuery(extra.ID) {
		t.Fatal("declared marker leaked before merge")
	}
	if err := c.Merge(clone); err != nil || !c.IsDeclaredQuery(extra.ID) {
		t.Fatalf("declared live merge: %v", err)
	}
	body, _ := json.Marshal(c.Snapshot())
	if strings.Contains(string(body), "declared") {
		t.Fatal("private marker entered JSON")
	}
}

func TestConflictingForkSourceUniversesArePersistableStaleNotFirstWins(t *testing.T) {
	root := t.TempDir()
	putFile(t, filepath.Join(root, "trace"), "trace")
	c := discover(t, root)
	a := c.Snapshot().Artifacts[0]
	q := plan(t, c, a.ID, "101", nil)
	left, right := c.Clone(), c.Clone()
	for _, branch := range []struct {
		c        *Catalog
		revision string
	}{{left, "universe-A"}, {right, "universe-B"}} {
		if err := branch.c.BindSource(context.Background(), a.ID, branch.revision, func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
		if _, err := branch.c.Complete(context.Background(), q.ID, Completion{Outcome: OutcomeSuccess, SourceRevision: branch.revision, PayloadRef: "/" + branch.revision + ".json"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Merge(left); err != nil {
		t.Fatal(err)
	}
	if err := c.Merge(right); err != nil {
		t.Fatal(err)
	}
	row, _ := c.Query(q.ID)
	if row.Outcome != OutcomeStale || len(row.Attempts) != 2 {
		t.Fatalf("conflict picked winner/lost history: %+v", row)
	}
	if _, err := c.Resolve(context.Background(), a.ID); !errors.Is(err, ErrStale) {
		t.Fatal("conflicting universe acquired access")
	}
	path := filepath.Join(t.TempDir(), "catalog.json")
	if err := c.Save(context.Background(), path); err != nil {
		t.Fatal(err)
	}
	s, err := Load(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Queries[0].Outcome != OutcomeStale || s.Artifacts[0].SourceRevision != "" {
		t.Fatal("persisted conflict selected a source revision")
	}
}
