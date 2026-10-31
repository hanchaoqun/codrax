package tracequery

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func hmc328MarkerSelectionTrace(rename bool) string {
	var b strings.Builder
	name := "Operation"
	if rename {
		name = "Opaque independent work"
	}
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "background-9 (9) [001] .... %.6f: tracing_mark_write: B|9|earlier\n", 1.1+float64(i)*.001)
		fmt.Fprintf(&b, "background-9 (9) [001] .... %.6f: tracing_mark_write: E|9\n", 1.1001+float64(i)*.001)
	}
	fmt.Fprintf(&b, "worker-7 (77) [000] .... 2.000000: tracing_mark_write: B|7000|%s\n", name)
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "worker-7 (77) [000] .... %.6f: tracing_mark_write: B|7000|tiny\n", 2.001+float64(i)*.001)
		fmt.Fprintf(&b, "worker-7 (77) [000] .... %.6f: tracing_mark_write: E|7000\n", 2.0011+float64(i)*.001)
	}
	fmt.Fprintln(&b, "worker-7 (77) [000] .... 2.100000: tracing_mark_write: B|7000|principal child")
	fmt.Fprintln(&b, "worker-7 (77) [000] .... 2.200000: tracing_mark_write: B|7000|inner")
	fmt.Fprintln(&b, "worker-7 (77) [000] .... 2.300000: tracing_mark_write: E|7000")
	fmt.Fprintln(&b, "worker-7 (77) [000] .... 2.900000: tracing_mark_write: E|7000")
	fmt.Fprintln(&b, "worker-7 (77) [000] .... 3.000000: tracing_mark_write: E|7000")
	fmt.Fprintf(&b, "worker-7 (77) [000] .... 3.100000: tracing_mark_write: B|7000|%s\n", name)
	fmt.Fprintln(&b, "worker-7 (77) [000] .... 3.200000: tracing_mark_write: B|7000|principal child")
	fmt.Fprintln(&b, "worker-7 (77) [000] .... 3.500000: tracing_mark_write: E|7000")
	fmt.Fprintln(&b, "worker-7 (77) [000] .... 3.600000: tracing_mark_write: E|7000")
	return b.String()
}

func TestHMC328MarkerTreeOwnerAccountsSurviveEarlierInventory(t *testing.T) {
	for _, rename := range []bool{false, true} {
		t.Run(fmt.Sprintf("renamed_%t", rename), func(t *testing.T) {
			idx := buildTraceIndex(t, "selection.ftrace", hmc328MarkerSelectionTrace(rename))
			q := Query{PID: 7, TargetScope: TargetScopeThread, TimeStart: 1, TimeEnd: 4}
			tree := markerTreePublic(t, idx, q)
			if tree.NodeCount != 85 || len(tree.Nodes) != 32 || tree.OmittedNodes != 53 {
				t.Fatalf("full inventory or unchanged 32-node budget lost: %d/%d/%d", tree.NodeCount, len(tree.Nodes), tree.OmittedNodes)
			}
			found := map[float64]TraceMarkerTreeNode{}
			for _, n := range tree.Nodes {
				if n.Thread.PID != 7 {
					t.Errorf("earlier background displaced an available explicit-owner account: %+v", n)
				}
				found[n.ActualStartTs] = n
			}
			for _, want := range []struct{ start, total, self float64 }{{2, 1000, 196}, {2.1, 800, 700}, {3.1, 500, 200}, {3.2, 300, 300}} {
				n, ok := found[want.start]
				if !ok || n.Inclusive == nil || n.Self == nil || !near(n.Inclusive.DurationMs, want.total, 1e-6) || !near(n.Self.DurationMs, want.self, 1e-6) {
					t.Errorf("complete owner account at %.6f missing or changed: got=%+v want total/self=%g/%g", want.start, n, want.total, want.self)
				}
			}
			if first, second := found[2], found[3.1]; first.ID == second.ID || first.DirectChildCount != 41 || second.DirectChildCount != 1 {
				t.Error("same-name instances merged, or full direct-child census changed")
			}
			if child, parent := found[2.1], found[2]; child.ParentID != parent.ID || child.ParentStatus != "observed_parent" {
				t.Error("display ordering changed physical nesting")
			}
		})
	}
}

func TestHMC328MarkerTreeUnselectedQueryKeepsOriginalInventory(t *testing.T) {
	idx := buildTraceIndex(t, "unselected.ftrace", hmc328MarkerSelectionTrace(false))
	for _, q := range []Query{{TimeStart: 1, TimeEnd: 4}, {PID: 7, TargetScope: TargetScopeProcess, TimeStart: 1, TimeEnd: 4}, {Thread: "worker", TimeStart: 1, TimeEnd: 4}} {
		tree := markerTreePublic(t, idx, q)
		if tree.NodeCount != 85 || tree.OmittedNodes != 53 || len(tree.Nodes) != 32 {
			t.Fatal("unselected inventory changed")
		}
		for i, n := range tree.Nodes {
			if n.Thread.PID != 9 || i > 0 && n.StartLine <= tree.Nodes[i-1].StartLine {
				t.Fatal("unselected/process/name-only inventory no longer follows the original physical order")
			}
		}
	}
	// A typed display face remains unable to change causal ranking.
	q := normalizeQuery(idx, Query{PID: 7, TimeStart: 1, TimeEnd: 4})
	stats := ComputeWindowStats(idx, q)
	chain := BuildWakeupChain(idx, q)
	with := buildRootCauseRankFrom(idx, q, chain, stats)
	stats.BusinessTree = nil
	if !reflect.DeepEqual(with, buildRootCauseRankFrom(idx, q, chain, stats)) {
		t.Fatal("business display selection changed causal root identity")
	}
}

func TestHMC328MarkerTreeDisplayOrderKeepsUnknownAndIdentity(t *testing.T) {
	nodes := []TraceMarkerTreeNode{
		{ID: "unknown", Thread: ThreadRef{PID: 7}, StartLine: 1, Closure: "open"},
		{ID: "zero", Thread: ThreadRef{PID: 7}, StartLine: 2, Closure: "closed", Inclusive: &TraceMarkerTreeAccount{}},
		{ID: "known-b", SourcePath: "b", Thread: ThreadRef{PID: 7}, StartLine: 3, Name: "same", Closure: "closed", Inclusive: &TraceMarkerTreeAccount{DurationMs: 10}},
		{ID: "foreign", Thread: ThreadRef{PID: 8}, StartLine: 0, Closure: "closed", Inclusive: &TraceMarkerTreeAccount{DurationMs: 10000}},
		{ID: "known-a", SourcePath: "a", Thread: ThreadRef{PID: 7}, StartLine: 3, Name: "same", Closure: "closed", Inclusive: &TraceMarkerTreeAccount{DurationMs: 10}},
	}
	sort.SliceStable(nodes, func(i, j int) bool { return TraceMarkerTreeDisplayLess(nodes[i], nodes[j], 7) })
	for i, id := range []string{"known-a", "known-b", "zero", "unknown", "foreign"} {
		if nodes[i].ID != id {
			t.Fatalf("unknown became zero, foreign time outranked owner, or exact ties merged: %+v", nodes)
		}
	}
	if nodes[3].Inclusive != nil || nodes[3].Closure != "open" {
		t.Fatal("presentation fabricated an unknown elapsed account")
	}
}

func TestHMC328MarkerTreeSelectionKeepsClippedCompleteAccounts(t *testing.T) {
	idx := buildTraceIndex(t, "clip.ftrace", hmc328MarkerSelectionTrace(false))
	tree := markerTreePublic(t, idx, Query{PID: 7, TimeStart: 2.25, TimeEnd: 2.8})
	if tree.NodeCount != 3 || tree.OmittedNodes != 0 {
		t.Fatalf("overlap selection changed the query inventory: %+v", tree)
	}
	root := markerTreeNamed(t, tree, "Operation")
	child := markerTreeNamed(t, tree, "principal child")
	if root.ActualStartTs != 2 || *root.ActualEndTs != 3 || root.DirectChildCount != 41 || !near(root.Inclusive.DurationMs, 550, 1e-6) || !near(root.Self.DurationMs, 0, 1e-6) || !near(child.Self.DurationMs, 500, 1e-6) {
		t.Fatalf("display selection altered full nesting or the window/self intersection: root=%+v child=%+v", root, child)
	}
	if root.Inclusive.Segments[0].StartTs != 2.25 || root.Inclusive.Segments[0].EndTs != 2.8 || child.ParentID != root.ID {
		t.Fatal("physical instance and independently clipped measurement were conflated")
	}
}

// The full external material is opt-in, never an excerpt embedded in a model
// question. The expected independent marker accounts are assertion data only.
func TestHMC328MarkerTreeRealDonghuAccounts(t *testing.T) {
	path := os.Getenv("CODRAX_HMC328_DONGHU_TRACE")
	if path == "" {
		t.Skip("set CODRAX_HMC328_DONGHU_TRACE to the complete registered 105 Yuanbao ftrace")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	h := sha256.New()
	size, err := io.Copy(h, f)
	closeErr := f.Close()
	if err != nil || closeErr != nil || size != 63659046 || fmt.Sprintf("%x", h.Sum(nil)) != "be59c0648ac7e1d8a2d117f5dc36efa0c92b5f3686671719ada435c16713a20c" {
		t.Fatalf("registered full input mismatch: bytes=%d SHA=%x read=%v close=%v", size, h.Sum(nil), err, closeErr)
	}
	// Read the registered full original through the production bounded index
	// path; a complete-file index would hit the unchanged 250k event ceiling.
	idx, err := BuildIndexWithOptions(context.Background(), path, BuildOptions{
		TimeStart: 3733.491839, TimeEnd: 3733.795890,
		TimeStartSet: true, TimeEndSet: true,
		TimePaddingBefore: .5, TimePaddingAfter: .5, AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	accounts := []struct {
		line, children                                            int
		start, end, total, self, running, runnable, sleep, dTotal float64
	}{
		{154186, 30, 3733.491839, 3733.551534, 59.695, 6.770, 35.932, 1.158, 19.120, 3.485},
		{155107, 44, 3733.504661, 3733.551261, 46.600, 21.605, 26.886, .818, 15.442, 3.454},
		{172560, 17, 3733.684403, 3733.795890, 111.487, 3.938, 83.926, 1.971, 24.234, 1.356},
		{174030, 50, 3733.692357, 3733.795672, 103.315, 83.143, 80.727, 1.743, 19.489, 1.356},
	}
	for _, window := range []struct {
		start, end  float64
		first, last int
	}{{3733.491839, 3733.795890, 0, 4}, {3733.491839, 3733.551534, 0, 2}, {3733.684403, 3733.795890, 2, 4}} {
		t.Run(fmt.Sprintf("window_%.6f_%.6f", window.start, window.end), func(t *testing.T) {
			tree := markerTreePublic(t, idx, Query{PID: 46571, TargetScope: TargetScopeThread, TimeStart: window.start, TimeEnd: window.end})
			if len(tree.Nodes) != 32 || tree.NodeCount != len(tree.Nodes)+tree.OmittedNodes {
				t.Fatalf("bounded inventory accounting changed: %+v", tree)
			}
			byLine := map[int]TraceMarkerTreeNode{}
			for _, n := range tree.Nodes {
				byLine[n.StartLine] = n
			}
			for _, a := range accounts[window.first:window.last] {
				n, ok := byLine[a.line]
				if !ok {
					t.Errorf("complete target account at original line %d is absent from %d/%d published nodes", a.line, len(tree.Nodes), tree.NodeCount)
					continue
				}
				if n.SourcePath != path || n.Thread.PID != 46571 || n.Thread.TGID != 46571 || n.ActualStartTs != a.start || n.ActualEndTs == nil || *n.ActualEndTs != a.end || n.DirectChildCount != a.children || n.Closure != "closed" || n.Inclusive == nil || n.Self == nil {
					t.Fatalf("physical instance identity/complete account changed: %+v", n)
				}
				if !near(n.Inclusive.DurationMs, a.total, 1e-6) || !near(n.Self.DurationMs, a.self, 1e-6) {
					t.Errorf("line %d total/self=%g/%g, expected %g/%g", a.line, n.Inclusive.DurationMs, n.Self.DurationMs, a.total, a.self)
				}
				st := n.Inclusive.States
				if st == nil || st.Values == nil || st.Coverage != "complete" || st.UnknownMs != 0 {
					t.Fatalf("state coverage unavailable: %+v", st)
				}
				v := st.Values
				if !near(v.RunningMs, a.running, 1e-6) || !near(v.RunnableMs, a.runnable, 1e-6) || !near(v.SleepMs, a.sleep, 1e-6) || !near(v.DStateMs+v.IOWaitMs, a.dTotal, 1e-6) {
					t.Errorf("line %d state denominator changed: %+v", a.line, v)
				}
				t.Logf("original_line=%d ID=%s total_ms=%.6f self_ms=%.6f owner=%d/%d inventory=%d displayed=%d omitted=%d", a.line, n.ID, n.Inclusive.DurationMs, n.Self.DurationMs, n.Thread.PID, n.Thread.TGID, tree.NodeCount, len(tree.Nodes), tree.OmittedNodes)
			}
		})
	}
}
