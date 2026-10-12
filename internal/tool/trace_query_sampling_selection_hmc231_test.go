package tool

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func samplingSelectionPublicRecord(t *testing.T, selectors map[string]any) types.ObservationRecord {
	t.Helper()
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_lossless_sampling/paths.perftrace")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("sampled prefixes")}
	params := map[string]any{"source": "path", "path": path, "view": "perf_sampling_tree", "time_start": 1, "time_end": 2}
	for k, v := range selectors {
		params[k] = v
	}
	raw, _ := json.Marshal(params)
	result, err := (&TraceQuery{}).Execute(ctx, raw)
	if err != nil || !result.Success {
		t.Fatalf("public sampling query: %v %+v", err, result)
	}
	for _, record := range result.Observations {
		if record.Predicate == TracePerfSamplingTreePredicate {
			return record
		}
	}
	t.Fatalf("query did not publish sampling observation: selectors=%v summary=%s", selectors, result.Summary)
	return types.ObservationRecord{}
}

func TestSamplingSelectionPublicIdentityHMC231(t *testing.T) {
	for _, selectors := range []map[string]any{{"pid": 11}, {"thread": "document-worker"}, {"pid": 11, "sampling_depth": 1}, {"pid": 11, "limit": 1}} {
		r := samplingSelectionPublicRecord(t, selectors)
		before, _ := json.Marshal(r)
		subject, ok := TraceQueryAggregateSelectionSubject(r)
		if !ok || subject != "document-worker-11" {
			t.Errorf("native multi-cohort query lost selector identity %v: %q %t", selectors, subject, ok)
		}
		after, _ := json.Marshal(r)
		if !bytes.Equal(before, after) {
			t.Fatal("display selection mutated original receipt")
		}
	}
	for _, selectors := range []map[string]any{nil, {"pid": 12}, {"pid": 11, "pattern": "work"}} {
		r := samplingSelectionPublicRecord(t, selectors)
		if subject, ok := TraceQueryAggregateSelectionSubject(r); ok {
			t.Errorf("global/empty/unavailable query acquired target authority %v: %q", selectors, subject)
		}
	}
	// Known producer gap, not a permanent rejection contract: this spelling
	// currently normalizes only the engine target, so the outer binding does
	// not decode. Keep it unqualified while unsupported; once the producer
	// publishes a consistent binding it should gain ordinary display support.
	decorated := samplingSelectionPublicRecord(t, map[string]any{"thread": "document-worker-11 (tid=11)"})
	_, bound := DecodeTracePerfSamplingTree(decorated)
	subject, selected := TraceQueryAggregateSelectionSubject(decorated)
	if !bound {
		if selected {
			t.Fatal("currently inconsistent decorated target binding gained display authority")
		}
		t.Log("decorated selector native binding is currently unsupported; producer normalization remains to be repaired")
	} else if !selected || subject != "document-worker-11" {
		t.Fatal("consistent decorated target binding lost ordinary selected-thread display")
	}
}

func TestSamplingSelectionTypedBindingNegativesHMC231(t *testing.T) {
	base := samplingSelectionPublicRecord(t, map[string]any{"pid": 11})
	for _, tc := range []struct {
		name string
		edit func(*types.ObservationRecord, *tracequery.PerfSamplingTreeResult)
	}{
		{"source_binding", func(r *types.ObservationRecord, _ *tracequery.PerfSamplingTreeResult) { r.SourceRef.Path += ".other" }},
		{"window_binding", func(r *types.ObservationRecord, _ *tracequery.PerfSamplingTreeResult) {
			r.SourceRef.QueryWindowStartTs = 0
		}},
		{"target_binding", func(r *types.ObservationRecord, _ *tracequery.PerfSamplingTreeResult) {
			r.SourceRef.QueryTargetPID = 12
		}},
		{"role", func(r *types.ObservationRecord, _ *tracequery.PerfSamplingTreeResult) {
			r.Role = types.AnswerAggregateRolePrincipalAnswer
		}},
		{"negative", func(r *types.ObservationRecord, _ *tracequery.PerfSamplingTreeResult) { r.Negative = true }},
		{"foreign_cohort", func(_ *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			p.Cohorts[1].SourcePath += ".other"
		}},
		{"source_generation", func(_ *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			p.Cohorts[1].SourceGeneration += ".other"
		}},
		{"missing_identity", func(_ *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			p.Cohorts[1].ThreadIdentity = nil
		}},
		{"foreign_tid", func(_ *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			p.Cohorts[1].ThreadIdentity.TID = 12
		}},
		{"thread_generation", func(_ *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			p.Cohorts[1].ThreadIdentity.Generation++
		}},
		{"unknown_generation", func(_ *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			p.Cohorts[1].ThreadIdentity.Generation = 0
		}},
		{"foreign_tgid", func(_ *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			p.Cohorts[1].ThreadIdentity.TGID++
		}},
		{"different_name", func(_ *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			p.Cohorts[1].ThreadIdentity.DisplayComm = "peer"
		}},
		{"omitted_cohort", func(_ *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			p.Cohorts = p.Cohorts[:2]
			p.OmittedCohorts = 1
		}},
		{"global", func(r *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			r.SourceRef.QueryTargetPID = 0
			p.TargetPID = 0
		}},
		{"process_scope", func(r *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			r.SourceRef.QueryTargetScope = "process"
			p.TargetScope = "process"
		}},
		{"wrong_numeric_target", func(r *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			r.SourceRef.QueryTargetPID = 12
			p.TargetPID = 12
		}},
		{"invalid_numeric_target", func(r *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			r.SourceRef.QueryTargetPID, p.TargetPID = -1, -1
			r.SourceRef.QueryTargetThread, p.TargetThread = "document-worker", "document-worker"
		}},
		{"fuzzy_name", func(r *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			r.SourceRef.QueryTargetPID = 0
			p.TargetPID = 0
			r.SourceRef.QueryTargetThread = "document"
			p.TargetThread = "document"
		}},
		{"name_only_ambiguous", func(r *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			r.SourceRef.QueryTargetPID = 0
			p.TargetPID = 0
			r.SourceRef.QueryTargetThread = "document-worker"
			p.TargetThread = "document-worker"
			p.Cohorts[1].ThreadIdentity.TID = 12
		}},
		{"conflicting_decorated_target", func(r *types.ObservationRecord, p *tracequery.PerfSamplingTreeResult) {
			r.SourceRef.QueryTargetThread = "document-worker-12 (tid=11)"
			p.TargetThread = r.SourceRef.QueryTargetThread
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(base)
			var r types.ObservationRecord
			_ = json.Unmarshal(raw, &r)
			p, ok := DecodeTracePerfSamplingTree(r)
			if !ok {
				t.Fatal("bad test baseline")
			}
			tc.edit(&r, &p)
			body, _ := json.Marshal(p)
			for i, note := range r.RichNotes {
				if strings.HasPrefix(note, types.TraceNoteKeyPerfSamplingTree+"=") {
					r.RichNotes[i] = types.TraceNoteKeyPerfSamplingTree + "=" + string(body)
				}
			}
			if subject, ok := TraceQueryAggregateSelectionSubject(r); ok {
				t.Fatalf("mismatched aggregate acquired selector identity: %q", subject)
			}
		})
	}
	// A query's selector cannot retain an ordinary peer observation or confer
	// the aggregate's display qualification on a forged predicate-only row.
	peer := base
	peer.Subject, peer.Predicate, peer.ClaimKey, peer.RichNotes = "peer-12", "target_window_states", "target_window_states", nil
	if _, ok := TraceQueryAggregateSelectionSubject(peer); ok {
		t.Fatal("query target hint laundered an unrelated peer fact")
	}
}
