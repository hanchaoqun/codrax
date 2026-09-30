package tool

import (
	"encoding/json"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
	"path/filepath"
	"strings"
	"testing"
)

func TestProcessProfilePublicToolAndTypedHandoff(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_process_profile/events.systrace")
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("show process threads")}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "process_profile", "pid": 10, "time_start": 1, "time_end": 1.02, "limit": 2})
	r, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !r.Success {
		t.Fatalf("public query: %v %+v", err, r)
	}
	var record types.ObservationRecord
	for _, candidate := range r.Observations {
		if candidate.Predicate == TraceProcessProfilePredicate {
			record = candidate
		}
	}
	p, ok := DecodeTraceProcessProfile(record)
	if !ok || p.ThreadCount != 3 || p.OmittedThreads != 1 || p.UnavailableThreads != 1 || len(p.Threads) != 2 {
		t.Fatalf("handoff lost producer census: %+v %+v", record, p)
	}
	if !strings.Contains(r.Summary, "原生省略=1") {
		t.Fatal("preview lost capacity")
	}
	for _, mutate := range []func(*types.ObservationRecord){
		func(r *types.ObservationRecord) { r.SourceRef.QueryScopeID = "" },
		func(r *types.ObservationRecord) { r.SourceRef.QueryWindowStartTs = 0 },
		func(r *types.ObservationRecord) { r.Role = types.AnswerAggregateRolePrincipalAnswer },
		func(r *types.ObservationRecord) { r.SourceRef.Path += ".other" },
		func(r *types.ObservationRecord) { r.Object = "causal" },
	} {
		copy := record
		mutate(&copy)
		if _, ok := DecodeTraceProcessProfile(copy); ok {
			t.Fatalf("accepted mismatched proof: %+v", copy)
		}
	}
	copy := p
	copy.Threads = append([]tracequery.ProcessProfileThread(nil), p.Threads...)
	copy.Threads[0].RunningWindowPct = new(float64)
	data, _ := json.Marshal(copy)
	corrupt := record
	corrupt.RichNotes = []string{types.TraceNoteKeyProcessProfile + "=" + string(data)}
	if _, ok := DecodeTraceProcessProfile(corrupt); ok {
		t.Fatal("accepted incompatible percentage")
	}
	if !strings.Contains(string((&TraceQuery{}).Parameters()), "process_profile.threads") {
		t.Fatal("missing field teaching")
	}
}
