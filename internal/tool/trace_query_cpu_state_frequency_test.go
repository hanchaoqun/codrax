package tool

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestCPUStateFrequencyPublicToolAndHandoff(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_cpu_state_frequency/events.systrace")
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("CPU概览"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{RuntimeTargets: []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 41, Source: "user_explicit", Confidence: 0.95}}}}}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "cpu_state_frequency", "time_start": 1, "time_end": 1.04})
	r, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !r.Success {
		t.Fatalf("public query: %v %+v", err, r)
	}
	var record types.ObservationRecord
	for _, candidate := range r.Observations {
		if candidate.Predicate == TraceCPUStateFrequencyPredicate {
			record = candidate
		}
	}
	p, ok := DecodeTraceCPUStateFrequency(record)
	if !ok || p.CPUCount != 3 || math.Abs(p.CPUTimeMs-120) > 1e-6 || math.Abs(p.KnownJointMs-60) > 1e-6 || math.Abs(p.UnknownJointMs-60) > 1e-6 {
		t.Fatalf("lost full-window census (or inherited emitter): %+v %+v", record, p)
	}
	for _, want := range []string{"CPU0", "CPU1", "CPU2", "全部核时间=120", "联合未知=60", "idle状态0", "1000000", "parsed_events=11", "scanned_lines=12", "unparsed_lines=1"} {
		if !strings.Contains(r.Summary, want) {
			t.Errorf("preview lost %q: %s", want, r.Summary)
		}
	}
	if strings.Contains(r.Summary, "parse_diagnostic=zero_events") {
		t.Fatalf("stream's unretained index was mistaken for zero parsed events: %s", r.Summary)
	}
	for _, mutate := range []func(*types.ObservationRecord){
		func(r *types.ObservationRecord) { r.SourceRef.QueryScopeID = "" },
		func(r *types.ObservationRecord) { r.SourceRef.QueryWindowStartTs = 0 },
		func(r *types.ObservationRecord) { r.SourceRef.Path += ".other" },
		func(r *types.ObservationRecord) { r.SourceRef.QueryTargetPID = 41 },
		func(r *types.ObservationRecord) { r.Role = types.AnswerAggregateRolePrincipalAnswer },
		func(r *types.ObservationRecord) { r.Producer = "model" },
		func(r *types.ObservationRecord) {
			r.RichNotes = append(append([]string(nil), r.RichNotes...), r.RichNotes[0])
		},
		func(r *types.ObservationRecord) {
			r.RichNotes = []string{strings.Replace(r.RichNotes[0], `"status":`, `"status":"unavailable","status":`, 1)}
		},
	} {
		copy := record
		mutate(&copy)
		if _, ok := DecodeTraceCPUStateFrequency(copy); ok {
			t.Fatalf("accepted wrong authority: %+v", copy)
		}
	}
	p.CPUs[0].Groups[0].WindowPct = 500
	data, _ := json.Marshal(p)
	corrupt := record
	corrupt.RichNotes = []string{types.TraceNoteKeyCPUStateFrequency + "=" + string(data)}
	if _, ok := DecodeTraceCPUStateFrequency(corrupt); ok {
		t.Fatal("accepted corrupt percentage")
	}
	if !strings.Contains(string((&TraceQuery{}).Parameters()), "Omit pid/thread") {
		t.Fatal("missing CPU ownership teaching")
	}
}

func TestCPUStateFrequencyRejectsEmitterBeforeReadingSource(t *testing.T) {
	for _, params := range []string{`{"view":"cpu_state_frequency","pid":41,"time_start":1,"time_end":1.04}`, `{"view":"cpu_state_frequency","thread":"ui","time_start":1,"time_end":1.04}`} {
		r, err := (&TraceQuery{}).Execute(nil, json.RawMessage(params))
		if err != nil || r.Success || r.Repair == nil || r.Repair.Code != "trace_query_cpu_owned_view" || r.RawRef != "" || len(r.Observations) > 0 {
			t.Fatalf("selector became evidence: %v %+v", err, r)
		}
	}
}

func TestCPUStateFrequencyBoundedHandoffKeepsDenominator(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_cpu_state_frequency/events.systrace")
	r, err := tracequery.StreamCPUStateFrequency(context.Background(), path, tracequery.Query{TimeStart: 1, TimeEnd: 1.04})
	if err != nil {
		t.Fatal(err)
	}
	p := *r.CPUStateFrequency
	row := p.CPUs[0]
	p.CPUs = nil
	for i := 0; i < 20; i++ {
		copy := row
		copy.CPU = i
		p.CPUs = append(p.CPUs, copy)
	}
	p.CPUCount = 20
	p.CPUTimeMs = 20 * p.WindowWallMs
	p.KnownJointMs = p.CPUTimeMs
	p.UnknownJointMs = 0
	ref := types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: path, PayloadRef: "payload", QueryScopeID: "scope", QueryWindowKnown: true, QueryWindowStartTs: 1, QueryWindowEndTs: 1.04}
	records := traceQueryCPUStateFrequencyObservations(&p, ref, "scope", "now")
	got, ok := DecodeTraceCPUStateFrequency(records[0])
	if !ok || len(got.CPUs) != 16 || got.OmittedCPUs != 4 || got.CPUCount != 20 || math.Abs(got.CPUTimeMs-800) > 1e-6 {
		t.Fatalf("truncation rebased totals: %+v", got)
	}
	if text := TraceCPUStateFrequencyText(got, 8); !strings.Contains(text, "展示CPU=8，省略=12") || !strings.Contains(text, "全部核时间=800") {
		t.Fatalf("lost preview omissions: %s", text)
	}
}

func TestCPUStateFrequencySQLiteIntervalUnknownIsNotZero(t *testing.T) {
	path, _ := filepath.Abs("../../eval/fixtures/hmosperf_hisys_row_identity/capture.data")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, _, _ := hmc17NamedPathContext(t)
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "cpu_state_frequency", "time_start": 2, "time_end": 2.08})
	r, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !r.Success {
		t.Fatalf("prepare/query: %v %+v", err, r)
	}
	found := false
	for _, record := range r.Observations {
		if p, ok := DecodeTraceCPUStateFrequency(record); ok {
			found = p.Status == "unavailable" && p.Reason == "sql_measure_interval_semantics_not_preserved"
		}
	}
	if !found || !strings.Contains(r.Summary, "均未知，不能按零处理") {
		t.Fatalf("unsupported intervals became zero: %+v", r)
	}
	after, err := os.ReadFile(path)
	if err != nil || sha256.Sum256(before) != sha256.Sum256(after) {
		t.Fatal("read query mutated native source")
	}
}
