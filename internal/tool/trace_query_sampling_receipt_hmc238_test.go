package tool

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSamplingSelectionReceiptPublicHMC238SummaryPayloadAndAdvisory(t *testing.T) {
	dir := t.TempDir()
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("sampling")}
	wrong := samplingRepairQueryHMC235(t, ctx, map[string]any{"pid": 10, "thread": "document-worker"})
	if !wrong.Success || wrong.Repair != nil {
		t.Fatalf("advisory selection was turned into a hard failure: %+v", wrong)
	}
	w, originalPath, originalRaw := samplingRepairPayloadHMC235(t, wrong)
	if w.ThreadSelection == nil || !w.ThreadSelection.NameMismatch || w.ThreadSelection.Selected.PID != 10 ||
		len(w.ThreadSelection.NameCandidates) != 1 || w.ThreadSelection.NameCandidates[0].PID != 11 ||
		w.PerfSamplingTree.Status != "available" || w.PerfSamplingTree.TargetPID != 10 || w.PerfSamplingTree.SampleCount != 0 {
		t.Fatalf("numeric target was hidden or replaced by name candidate: %+v / %+v", w.ThreadSelection, w.PerfSamplingTree)
	}
	for _, token := range []string{"thread_selection status=exact_tid_name_mismatch", "requested_pid=10", "requested_name=document-worker", "routing=exact_tid_preserved", "name_candidates=[document-worker-11]"} {
		if !strings.Contains(wrong.Summary, token) {
			t.Fatalf("actual tool preview lost same native receipt %q: %s", token, wrong.Summary)
		}
	}
	var mismatch *types.ObservationRecord
	for i := range wrong.Observations {
		r := &wrong.Observations[i]
		if r.Predicate == "thread_selector_exact_name_mismatch" {
			if mismatch != nil {
				t.Fatal("duplicate selector mismatch observation")
			}
			mismatch = r
		}
	}
	if mismatch == nil || mismatch.SourceRef.Path != w.SourcePath || mismatch.SourceRef.PayloadRef != originalPath ||
		mismatch.SourceRef.QueryTargetPID != 10 || mismatch.SourceRef.QueryWindowStartTs != 1 || mismatch.SourceRef.QueryWindowEndTs != 2 ||
		mismatch.Role != types.AnswerAggregateRoleSupportingCoverage ||
		!strings.Contains(strings.Join(mismatch.RichNotes, " "), "name_candidate_role_authority=none") {
		t.Fatalf("same-source advisory gained authority or lost selection provenance: %+v", mismatch)
	}

	// A new, explicit caller choice is necessary. The first query and its
	// persisted receipt remain unchanged; discovering a name does not retry it.
	right := samplingRepairQueryHMC235(t, ctx, map[string]any{"pid": 11, "thread": "document-worker"})
	r, _, _ := samplingRepairPayloadHMC235(t, right)
	if !right.Success || right.Repair != nil || r.ThreadSelection == nil || r.ThreadSelection.NameMismatch || r.ThreadSelection.Selected.PID != 11 ||
		r.PerfSamplingTree.SampleCount != 5 || r.SourcePath != w.SourcePath || r.PerfSamplingTree.Window != w.PerfSamplingTree.Window {
		t.Fatalf("explicit retry did not preserve source/window: %+v", r)
	}
	weights := map[string]int64{}
	for _, c := range r.PerfSamplingTree.Cohorts {
		weights[c.Producer+"/"+c.Event] = c.TotalWeight
	}
	if !reflect.DeepEqual(weights, map[string]int64{"raw_perfdata_fallback/cycles": 35, "simpleperf_report_sample/cycles": 7, "raw_perfdata_fallback/instructions": 100}) {
		t.Fatalf("selection receipt changed independent complete denominators: %v", weights)
	}
	after, err := os.ReadFile(originalPath)
	if err != nil || !bytes.Equal(after, originalRaw) {
		t.Fatal("explicit retry overwrote first result evidence")
	}
	for _, view := range []string{"perf_stats", "window_stats", "trace_perf_bundle", "perf_timeline"} {
		legacy := samplingRepairQueryHMC235(t, ctx, map[string]any{"view": view, "pid": 10, "thread": "document-worker"})
		lp, _, _ := samplingRepairPayloadHMC235(t, legacy)
		if !legacy.Success || legacy.Repair != nil || !reflect.DeepEqual(lp.ThreadSelection, w.ThreadSelection) {
			t.Fatalf("stream differs from unchanged legacy %s receipt: %+v vs %+v", view, w.ThreadSelection, lp.ThreadSelection)
		}
	}
}

func TestSamplingSelectionReceiptPublicHMC238EmptyAndUnknownRemainDistinct(t *testing.T) {
	dir := t.TempDir()
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("sampling")}
	empty := samplingRepairQueryHMC235(t, ctx, map[string]any{"pid": 11, "thread": "document-worker", "time_start": 4, "time_end": 5})
	p, _, _ := samplingRepairPayloadHMC235(t, empty)
	if !empty.Success || p.ThreadSelection == nil || p.ThreadSelection.NameMismatch || p.ThreadSelection.Selected.PID != 11 ||
		p.PerfSamplingTree.Status != "available" || p.PerfSamplingTree.SampleCount != 0 ||
		!strings.Contains(empty.Summary, "thread_selection status=exact_tid") {
		t.Fatalf("real empty window was hidden as selector mismatch: %+v", p)
	}
	for _, row := range empty.Observations {
		if row.Predicate == "thread_selector_exact_name_mismatch" {
			t.Fatal("real empty window published mismatch")
		}
	}
	path := filepath.Join(dir, "unknown-identity.perftrace")
	if err := os.WriteFile(path, []byte("worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 thread_identity_known=false sample_weight=3 event=cycles symbol=work source=raw\n"), 0600); err != nil {
		t.Fatal(err)
	}
	unknown := samplingRepairQueryHMC235(t, ctx, map[string]any{"path": path, "pid": 11, "thread": "worker"})
	u, _, _ := samplingRepairPayloadHMC235(t, unknown)
	if !unknown.Success || u.ThreadSelection != nil || u.PerfSamplingTree.SampleCount != 0 ||
		!strings.Contains(strings.Join(u.Caveats, " "), "not establish absence") || strings.Contains(unknown.Summary, "thread_selection status=") {
		t.Fatalf("unknown typed identity was rehabilitated through header: %+v", u)
	}
	for _, row := range unknown.Observations {
		if row.Predicate == "thread_selector_exact_name_mismatch" {
			t.Fatal("unknown typed identity published a mismatch certificate")
		}
	}
}
