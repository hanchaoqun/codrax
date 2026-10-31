package tool

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC328BusinessTreePublicOwnerSelectionRetainsNativeScope(t *testing.T) {
	var raw strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&raw, "other-8 (80) [000] .... %.6f: tracing_mark_write: B|800|same\n", 1.1+float64(i)*.001)
		fmt.Fprintf(&raw, "other-8 (80) [000] .... %.6f: tracing_mark_write: E|800\n", 1.1001+float64(i)*.001)
	}
	raw.WriteString("worker-7 (70) [001] .... 2.000000: tracing_mark_write: B|700|same\nworker-7 (70) [001] .... 2.100000: tracing_mark_write: B|700|same\nworker-7 (70) [001] .... 2.800000: tracing_mark_write: E|700\nworker-7 (70) [001] .... 3.000000: tracing_mark_write: E|700\n")
	path := filepath.Join(t.TempDir(), "selection.ftrace")
	if err := os.WriteFile(path, []byte(raw.String()), 0600); err != nil {
		t.Fatal(err)
	}
	// Name the physical input explicitly: the public preparation contract
	// resolves aliases such as macOS /var to /private/var before publication.
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("native owner selection")}
	result := businessRefTestQuery(t, ctx, map[string]any{"path": path, "view": "window_stats", "pid": 7, "target_scope": "thread", "time_start": 1, "time_end": 4})
	payload := businessSpanSchedulerPublicPayload(t, result)
	if payload.WindowStats == nil || payload.WindowStats.BusinessTree == nil || payload.WindowStats.BusinessTree.NodeCount != 42 || payload.WindowStats.BusinessTree.OmittedNodes != 10 {
		t.Fatal("complete census or unchanged display cap lost")
	}
	owners := map[string]TraceBusinessTreeFact{}
	for _, r := range result.Observations {
		if r.Predicate != types.TraceBusinessTreePredicate {
			continue
		}
		f, ok := DecodeTraceBusinessTreeFact(r)
		if !ok {
			t.Fatalf("genuine selected native fact rejected: %+v", r)
		}
		if r.SourceRef.QueryTargetPID != 7 || r.SourceRef.QueryTargetScope != "thread" || r.SourceRef.QueryWindowStartTs != 1 || r.SourceRef.QueryWindowEndTs != 4 || r.SourceRef.QueryScopeID == "" || r.SourceRef.PayloadRef == "" || r.Role != types.AnswerAggregateRoleSupportingCoverage {
			t.Fatal("selection changed query identity/window or acquired causal authority")
		}
		if f.Node.Thread.PID == 7 {
			owners[f.Node.ID] = f
		}
	}
	if len(owners) != 2 {
		t.Fatalf("same-name physical target accounts absent or merged: %+v", owners)
	}
	for _, f := range owners {
		n := f.Node
		if n.Thread.TGID != 70 || n.SourcePath != path || n.Inclusive == nil || n.Self == nil {
			t.Fatalf("emitter identity lost or marker payload PID promoted: node=%+v want_path=%q", n, path)
		}
		if n.ParentID != "" && owners[n.ParentID].Node.ID != n.ParentID {
			t.Fatal("physical parent does not bind to the selected original instance")
		}
	}
}
