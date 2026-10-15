package tool

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC246ManifestPublicResumeAndReference(t *testing.T) {
	bus, root := catalogPublicFixture(t)
	old := catalogPublicDiscover(t, bus)
	if out := catalogPublicQuery(t, bus, filepath.Join(root, "capture.data"), 101); !out.Success {
		t.Fatal(out.Summary)
	}
	checkpoint, err := saveTraceCatalog(bus, old)
	if err != nil {
		t.Fatal(err)
	}
	next := bus.ShallowClone()
	next.Mutable = types.NewMutableState("resume captures")
	next.UserPinnedFiles = append(next.UserPinnedFiles, checkpoint)
	raw, _ := json.Marshal(map[string]any{"action": "resume", "root": root, "snapshot_path": checkpoint})
	out, err := (&TraceCatalog{}).Execute(next, raw)
	if err != nil || !out.Success {
		t.Fatalf("public resume: %v %s", err, out.Summary)
	}
	current := next.Mutable.TraceCatalogs()[0]
	if current.ID() == old.ID() {
		t.Fatal("historical run identity reused")
	}
	if len(current.Snapshot().Queries) != 6 {
		t.Fatal("lost original plans")
	}
	for _, q := range current.Snapshot().Queries {
		if q.Outcome != tracecatalog.OutcomeNotExecuted || len(q.Attempts) != 0 || current.IsDeclaredQuery(q.ID) {
			t.Fatal("history restored execution or declared-window authority")
		}
		args, _ := json.Marshal(map[string]any{"catalog_query_ref": map[string]string{"catalog_id": current.ID(), "query_id": q.ID}})
		result, err := (&TraceQuery{}).Execute(next, args)
		if err != nil {
			t.Fatal(err)
		}
		a := current.Snapshot().Artifacts
		broken := false
		for _, candidate := range a {
			if candidate.ID == q.ArtifactID {
				broken = filepath.Base(candidate.Path) == "broken.systrace"
			}
		}
		if result.Success == broken {
			t.Fatalf("reference result mismatch: %s", result.Summary)
		}
	}
	for _, q := range current.Snapshot().Queries {
		if q.Outcome == tracecatalog.OutcomeNotExecuted || len(q.Attempts) != 1 {
			t.Fatalf("execution not recorded: %+v", q)
		}
	}
}

func resumePublic246(t *testing.T, old *types.BusContext, c *tracecatalog.Catalog) (*types.BusContext, *tracecatalog.Catalog, string) {
	t.Helper()
	checkpoint, err := saveTraceCatalog(old, c)
	if err != nil {
		t.Fatal(err)
	}
	next := old.ShallowClone()
	next.Mutable = types.NewMutableState("resume")
	next.Ctx = context.Background()
	next.UserPinnedFiles = append(next.UserPinnedFiles, checkpoint)
	raw, _ := json.Marshal(map[string]any{"action": "resume", "root": c.Snapshot().Root, "snapshot_path": checkpoint})
	out, err := (&TraceCatalog{}).Execute(next, raw)
	if err != nil || !out.Success {
		t.Fatalf("resume: %v %s", err, out.Summary)
	}
	return next, next.Mutable.TraceCatalogs()[0], checkpoint
}

func refPublic246(c *tracecatalog.Catalog, q tracecatalog.QueryRecord) json.RawMessage {
	b, _ := json.Marshal(map[string]any{"catalog_query_ref": map[string]string{"catalog_id": c.ID(), "query_id": q.ID}})
	return b
}

func planFor246(t *testing.T, c *tracecatalog.Catalog, suffix, id string) tracecatalog.QueryRecord {
	t.Helper()
	for _, a := range c.Snapshot().Artifacts {
		if strings.HasSuffix(a.Path, suffix) {
			for _, q := range c.Snapshot().Queries {
				if q.ArtifactID == a.ID && q.Object.ID == id {
					return q
				}
			}
		}
	}
	t.Fatal("missing plan")
	return tracecatalog.QueryRecord{}
}

func TestHMC246ManifestCurrentAuthorityGenerationAndSchema(t *testing.T) {
	for _, kind := range []string{"outside_root", "snapshot_unpinned", "missing_root", "stale_generation", "replaced_generation", "missing_member", "symlink_member", "invalid_plan", "parallel_metadata", "extra_ref", "foreign_ref", "authority_revoked"} {
		t.Run(kind, func(t *testing.T) {
			bus, root := catalogPublicFixture(t)
			c := catalogPublicDiscover(t, bus)
			checkpoint, err := saveTraceCatalog(bus, c)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "invalid_plan" || kind == "parallel_metadata" {
				q := c.Snapshot().Queries[0]
				if kind == "invalid_plan" {
					q.Parameters = json.RawMessage(`{"view":"resource_stack","sampling_depth":900}`)
				} else {
					q.Object.Kind = "process"
				}
				if _, err := c.RegisterPlannedQuery(context.Background(), q.Plan); err != nil {
					t.Fatal(err)
				}
				checkpoint, err = saveTraceCatalog(bus, c)
				if err != nil {
					t.Fatal(err)
				}
			}
			next := bus.ShallowClone()
			next.Mutable = types.NewMutableState("new")
			next.UserPinnedFiles = []string{checkpoint}
			args := map[string]any{"action": "resume", "root": root, "snapshot_path": checkpoint}
			if kind == "outside_root" {
				next.RepoRoot = t.TempDir()
			}
			if kind == "snapshot_unpinned" {
				next.UserPinnedFiles = nil
			}
			if kind == "missing_root" {
				delete(args, "root")
			}
			path := filepath.Join(root, "capture.data")
			if kind == "missing_member" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "symlink_member" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(filepath.Join(root, "nested/capture.data"), path); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "replaced_generation" {
				b, err := os.ReadFile(path)
				if err != nil {
					t.Fatal(err)
				}
				b = append(b, '\n')
				if err := os.WriteFile(path, b, 0600); err != nil {
					t.Fatal(err)
				}
			}
			raw, _ := json.Marshal(args)
			out, err := (&TraceCatalog{}).Execute(next, raw)
			if kind == "outside_root" || kind == "snapshot_unpinned" || kind == "missing_root" || kind == "invalid_plan" || kind == "parallel_metadata" {
				if out.Success || len(next.Mutable.TraceCatalogs()) != 0 {
					t.Fatalf("invalid resume accepted: %v %s", err, out.Summary)
				}
				return
			}
			if err != nil || !out.Success {
				t.Fatalf("resume: %v %s", err, out.Summary)
			}
			current := next.Mutable.TraceCatalogs()[0]
			q := planFor246(t, current, "/capture.data", "101")
			// Choose the root member, never a basename heuristic for execution.
			for _, a := range current.Snapshot().Artifacts {
				if a.Path == path {
					for _, p := range current.Snapshot().Queries {
						if p.ArtifactID == a.ID && p.Object.ID == "101" {
							q = p
						}
					}
				}
			}
			if len(current.Snapshot().Artifacts) != 3 || len(current.Snapshot().History) != 1 {
				t.Fatal("roster/history lost")
			}
			if kind == "stale_generation" {
				info, _ := os.Stat(path)
				b, _ := os.ReadFile(path)
				b[0] ^= 1
				if err := os.WriteFile(path, b, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
					t.Fatal(err)
				}
			}
			call := refPublic246(current, q)
			if kind == "authority_revoked" {
				next.RepoRoot = t.TempDir()
			}
			if kind == "extra_ref" {
				var x map[string]any
				_ = json.Unmarshal(call, &x)
				x["pid"] = 999
				call, _ = json.Marshal(x)
				if toolparam.Validate(call, (&TraceQuery{}).Parameters()) == nil {
					t.Fatal("schema accepted ref plus override")
				}
			}
			if kind == "foreign_ref" {
				call = refPublic246(c, c.Snapshot().Queries[0])
			}
			result, err := (&TraceQuery{}).Execute(next, call)
			if kind == "replaced_generation" {
				if err != nil || !result.Success {
					t.Fatalf("fresh generation not really queried: %v %s", err, result.Summary)
				}
				return
			}
			if result.Success {
				t.Fatalf("invalid reference executed: %s", result.Summary)
			}
			stored, _ := current.Query(q.ID)
			if kind == "extra_ref" || kind == "foreign_ref" {
				if len(stored.Attempts) != 0 {
					t.Fatal("unidentified call attributed to current plan")
				}
				return
			}
			if len(stored.Attempts) == 0 {
				t.Fatalf("failure not recorded: %v %s", err, result.Summary)
			}
			if kind == "stale_generation" && stored.Outcome != tracecatalog.OutcomeStale {
				t.Fatal("stale downgraded to generic failure")
			}
			ref, err := saveTraceCatalog(next, current)
			if err != nil {
				t.Fatal(err)
			}
			disk, err := tracecatalog.Load(context.Background(), ref)
			if err != nil {
				t.Fatal(err)
			}
			for _, dq := range disk.Queries {
				if dq.ID == q.ID && dq.Outcome != stored.Outcome {
					t.Fatal("disk outcome differs")
				}
			}
		})
	}
}

func TestHMC246ManifestRetryCancelPersistenceAndHistory(t *testing.T) {
	bus, root := catalogPublicFixture(t)
	old := catalogPublicDiscover(t, bus)
	_ = catalogPublicQuery(t, bus, filepath.Join(root, "broken.systrace"), 101)
	next, c, _ := resumePublic246(t, bus, old)
	q := planFor246(t, c, "broken.systrace", "101")
	for i := 0; i < 2; i++ {
		out, err := (&TraceQuery{}).Execute(next, refPublic246(c, q))
		if err != nil || out.Success {
			t.Fatalf("failed query: %v %s", err, out.Summary)
		}
	}
	failed, _ := c.Query(q.ID)
	if failed.Outcome != tracecatalog.OutcomeFailure || len(failed.Attempts) != 2 {
		t.Fatalf("retry lost: %+v", failed)
	}
	for _, a := range c.Snapshot().Artifacts {
		if a.Path == filepath.Join(c.Snapshot().Root, "capture.data") {
			for _, p := range c.Snapshot().Queries {
				if p.ArtifactID == a.ID && p.Object.ID == "101" {
					q = p
				}
			}
		}
	}
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	next.Ctx = cancelCtx
	out, _ := (&TraceQuery{}).Execute(next, refPublic246(c, q))
	if out.Success {
		t.Fatal("canceled query succeeded")
	}
	path := filepath.Join(next.WorkDir, "trace-catalog", c.ID()+".json")
	disk, err := tracecatalog.Load(context.Background(), path)
	if err != nil {
		t.Fatalf("cancel not durably recorded: %v %s", err, out.Summary)
	}
	for _, p := range disk.Queries {
		if p.ID == q.ID && (p.Outcome != tracecatalog.OutcomeCanceled || len(p.Attempts) != 1) {
			t.Fatalf("cancel persistence: %+v", p)
		}
	}
	next.Ctx = context.Background()
	out, err = (&TraceQuery{}).Execute(next, refPublic246(c, q))
	if err != nil || !out.Success {
		t.Fatalf("cancel retry: %v %s", err, out.Summary)
	}
	retried, _ := c.Query(q.ID)
	if len(retried.Attempts) != 2 || retried.Outcome != tracecatalog.OutcomeSuccess || len(retried.Attempts[1].ExecutedParameters) == 0 {
		t.Fatal("fresh retry not retained")
	}
	_, again, _ := resumePublic246(t, next, c)
	if len(again.Snapshot().History) != 2 {
		t.Fatal("second history missing")
	}
	if len(again.Snapshot().History[1].Queries) != 6 {
		t.Fatal("history silently trimmed")
	}
	for _, p := range again.Snapshot().Queries {
		if p.Outcome != tracecatalog.OutcomeNotExecuted || again.IsDeclaredQuery(p.ID) {
			t.Fatal("prior current status restored authority")
		}
	}
	// A repaired failed member is genuinely prepared/read in a fresh generation,
	// not returned from either historical failure or the previous run's memo.
	body, err := os.ReadFile(filepath.Join(root, "capture.data"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "broken.systrace"), body, 0600); err != nil {
		t.Fatal(err)
	}
	repairedBus, repaired, _ := resumePublic246(t, next, c)
	repairedQuery := planFor246(t, repaired, "broken.systrace", "101")
	result, err := (&TraceQuery{}).Execute(repairedBus, refPublic246(repaired, repairedQuery))
	if err != nil || !result.Success || len(result.Observations) == 0 {
		t.Fatalf("repaired failure did not run: %v %s", err, result.Summary)
	}
	actual, _ := repaired.Query(repairedQuery.ID)
	if actual.Outcome != tracecatalog.OutcomeSuccess || len(actual.Attempts) != 1 || actual.Attempts[0].PayloadRef == "" {
		t.Fatalf("repaired evidence missing: %+v", actual)
	}
}

func TestHMC246ManifestIndependentProcess(t *testing.T) {
	if os.Getenv("CODRAX_MANIFEST246_CHILD") == "1" {
		root, checkpoint, work := os.Getenv("CODRAX_MANIFEST246_ROOT"), os.Getenv("CODRAX_MANIFEST246_SNAPSHOT"), os.Getenv("CODRAX_MANIFEST246_WORK")
		bus := &types.BusContext{RepoRoot: root, WorkDir: work, UserPinnedFiles: []string{checkpoint}, Mutable: types.NewMutableState("resume"), TraceInputPreparer: traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: filepath.Join(work, "prepared")})}
		raw, _ := json.Marshal(map[string]any{"action": "resume", "root": root, "snapshot_path": checkpoint})
		out, err := (&TraceCatalog{}).Execute(bus, raw)
		if err != nil || !out.Success {
			t.Fatalf("child resume: %v %s", err, out.Summary)
		}
		c := bus.Mutable.TraceCatalogs()[0]
		nativeResults := 0
		for _, q := range c.Snapshot().Queries {
			if q.Outcome != tracecatalog.OutcomeNotExecuted {
				t.Fatal("child inherited result")
			}
			result, _ := (&TraceQuery{}).Execute(bus, refPublic246(c, q))
			if result.Success && len(result.Observations) > 0 && result.RawRef != "" {
				nativeResults++
			}
		}
		for _, q := range c.Snapshot().Queries {
			if len(q.Attempts) != 1 || q.Outcome == tracecatalog.OutcomeNotExecuted {
				t.Fatalf("child missing genuine attempt %+v", q)
			}
		}
		if nativeResults != 4 {
			t.Fatalf("child had %d real native result payloads, want 4", nativeResults)
		}
		return
	}
	bus, root := catalogPublicFixture(t)
	c := catalogPublicDiscover(t, bus)
	_ = catalogPublicQuery(t, bus, filepath.Join(root, "capture.data"), 101)
	snapshot, err := saveTraceCatalog(bus, c)
	if err != nil {
		t.Fatal(err)
	}
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bin, "-test.run=^TestHMC246ManifestIndependentProcess$", "-test.count=1")
	cmd.Env = append(os.Environ(), "CODRAX_MANIFEST246_CHILD=1", "CODRAX_MANIFEST246_ROOT="+root, "CODRAX_MANIFEST246_SNAPSHOT="+snapshot, "CODRAX_MANIFEST246_WORK="+t.TempDir())
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("child: %v\n%s", err, output)
	}
}
