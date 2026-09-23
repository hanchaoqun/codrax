package agent

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	ctxbuilder "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// These are decoded from the actual public query payload. In particular, dev
// is not supplied by a test-authored observation or a private query helper.
type traceResourceSemanticsRow struct {
	Path, Address, Dev, Domain string
	Count, Line                int
	Bytes                      int64
	TotalLatencyMS             float64 `json:"total_latency_ms"`
	MaxLatencyMS               float64 `json:"max_latency_ms"`
}

func TestTraceResourceSemanticsPublicQueryToFinalizer(t *testing.T) {
	type fixture struct {
		name, event, family, predicate string
		fields                         []string
		want                           []traceResourceSemanticsRow
	}
	cases := []fixture{
		{
			name: "fault_addresses_are_not_paths", event: "page_fault_user", family: "page_fault_resources", predicate: "page_fault_resource",
			fields: []string{
				"operation=major address=0x1234 duration_us=150 size=1024",
				"operation=major address=0x5678 duration_us=250 size=2048",
				"operation=major address=0x1234 duration_us=150 size=1024",
			},
			want: []traceResourceSemanticsRow{
				{Address: "0x1234", Count: 2, Line: 1, Bytes: 2048, TotalLatencyMS: .3, MaxLatencyMS: .15},
				{Address: "0x5678", Count: 1, Line: 2, Bytes: 2048, TotalLatencyMS: .25, MaxLatencyMS: .25},
			},
		},
		{
			name: "fault_same_path_different_addresses", event: "page_fault_user", family: "page_fault_resources", predicate: "page_fault_resource",
			fields: []string{
				"operation=major path=/mapped.db address=0x1234 duration_us=150 size=1024",
				"operation=major path=/mapped.db address=0x5678 duration_us=250 size=2048",
			},
			want: []traceResourceSemanticsRow{
				{Path: "/mapped.db", Address: "0x1234", Count: 1, Line: 1, Bytes: 1024, TotalLatencyMS: .15, MaxLatencyMS: .15},
				{Path: "/mapped.db", Address: "0x5678", Count: 1, Line: 2, Bytes: 2048, TotalLatencyMS: .25, MaxLatencyMS: .25},
			},
		},
		{
			name: "all_resource_coordinates_remain_independent", event: "file_system", family: "filesystem_resources", predicate: "filesystem_resource",
			fields: []string{
				"syscall=read path=/same.db address=0x1234 dev=8,0 duration_ms=2 bytes=1024",
				"syscall=read path=/same.db address=0x1234 dev=8,1 duration_ms=3 bytes=2048",
				"syscall=read path=/same.db address=0x5678 dev=8,0 duration_ms=4 bytes=4096",
			},
			want: []traceResourceSemanticsRow{
				{Path: "/same.db", Address: "0x1234", Dev: "8,0", Count: 1, Line: 1, Bytes: 1024, TotalLatencyMS: 2, MaxLatencyMS: 2},
				{Path: "/same.db", Address: "0x1234", Dev: "8,1", Count: 1, Line: 2, Bytes: 2048, TotalLatencyMS: 3, MaxLatencyMS: 3},
				{Path: "/same.db", Address: "0x5678", Dev: "8,0", Count: 1, Line: 3, Bytes: 4096, TotalLatencyMS: 4, MaxLatencyMS: 4},
			},
		},
		{
			name: "missing_path_is_not_literal_unknown", event: "file_system", family: "filesystem_resources", predicate: "filesystem_resource",
			fields: []string{"syscall=read duration_ms=2 bytes=1024", "syscall=read path=unknown duration_ms=3 bytes=2048"},
			want: []traceResourceSemanticsRow{
				{Count: 1, Line: 1, Bytes: 1024, TotalLatencyMS: 2, MaxLatencyMS: 2},
				{Path: "unknown", Count: 1, Line: 2, Bytes: 2048, TotalLatencyMS: 3, MaxLatencyMS: 3},
			},
		},
		{
			name: "same_native_path_still_aggregates", event: "bio_latency", family: "bio_resources", predicate: "bio_resource",
			fields: []string{"op=R path=/data/native.db latency_ms=2 bytes=1024", "op=R path=/data/native.db latency_ms=3 bytes=2048"},
			want:   []traceResourceSemanticsRow{{Path: "/data/native.db", Count: 2, Line: 1, Bytes: 3072, TotalLatencyMS: 5, MaxLatencyMS: 3}},
		},
	}
	for _, event := range []struct{ name, event, family, op string }{
		{"bio", "bio_latency", "bio_resources", "op=R"},
		{"filesystem", "file_system", "filesystem_resources", "syscall=read"},
	} {
		cases = append(cases, fixture{
			name: event.name + "_devices_are_not_paths", event: event.event, family: event.family, predicate: event.name + "_resource",
			fields: []string{
				event.op + " dev=8,0 latency_ms=2 bytes=1024",
				event.op + " dev=8,1 latency_ms=3 bytes=2048",
				event.op + " dev=8,0 latency_ms=2 bytes=1024",
			},
			want: []traceResourceSemanticsRow{
				{Dev: "8,0", Count: 2, Line: 1, Bytes: 2048, TotalLatencyMS: 4, MaxLatencyMS: 2},
				{Dev: "8,1", Count: 1, Line: 2, Bytes: 2048, TotalLatencyMS: 3, MaxLatencyMS: 3},
			},
		})
	}
	for _, event := range []struct{ event, family, predicate, fields string }{
		{"ability_monitor", "ability_events", "ability_monitor", "event_name=AbilityStart metric=latency_ms value=12.5 category=foreground"},
		{"xpower_cpu", "xpower_events", "xpower", "component=CPU energy=8.2 usage=73 scene=foreground"},
		{"hi_sysevent", "hi_sysevent_events", "hi_sysevent", "eventname=THERMAL_REPORT type=STAT value=hot level=MINOR"},
	} {
		cases = append(cases, fixture{
			name: event.predicate + "_missing_domain_is_not_comm", event: event.event, family: event.family, predicate: event.predicate,
			// Explicit domain equal to comm is a real positive, not a value to
			// strip by spelling. Its absence must stay a distinct group.
			fields: []string{event.fields, "domain=emitter " + event.fields, event.fields},
			want:   []traceResourceSemanticsRow{{Count: 2, Line: 1}, {Domain: "emitter", Count: 1, Line: 2}},
		})
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var raw strings.Builder
			for i, fields := range tc.fields {
				fmt.Fprintf(&raw, "emitter-72 (72) [001] .... 8.%03d000: %s: %s\n", i*10, tc.event, fields)
			}
			result, stats, bus := traceResourceSemanticsQuery(t, raw.String())
			var rows []traceResourceSemanticsRow
			if err := json.Unmarshal(stats[tc.family], &rows); err != nil {
				t.Fatalf("native family %s absent or malformed: %v", tc.family, err)
			}
			if len(rows) != len(tc.want) {
				t.Errorf("different native objects collapsed: rows=%+v; want=%+v", rows, tc.want)
			}
			for _, want := range tc.want {
				found := false
				for _, got := range rows {
					if got.Line != want.Line {
						continue
					}
					found = true
					if got.Path != want.Path || got.Address != want.Address || got.Dev != want.Dev || got.Domain != want.Domain ||
						got.Count != want.Count || got.Bytes != want.Bytes || math.Abs(got.TotalLatencyMS-want.TotalLatencyMS) > 1e-9 || math.Abs(got.MaxLatencyMS-want.MaxLatencyMS) > 1e-9 {
						t.Errorf("native source fields/numbers changed: got=%+v want=%+v", got, want)
					}
				}
				if !found {
					t.Errorf("missing independently aggregated source line %d", want.Line)
				}
			}
			for _, lang := range []string{"zh", "en"} {
				t.Run(lang, func(t *testing.T) {
					traceResourceSemanticsFinalizer(t, bus, result, tc.predicate, tc.want, lang)
				})
			}
		})
	}
}

func traceResourceSemanticsQuery(t *testing.T, source string) (types.ToolResult, map[string]json.RawMessage, *types.BusContext) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "resource.systrace")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	bus := &types.BusContext{RepoRoot: dir, WorkDir: dir, Mutable: types.NewMutableState("finite resource facts")}
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "window_stats", "time_start": 8, "time_end": 8.05})
	result, err := (&tool.TraceQuery{}).Execute(bus, params)
	if err != nil || !result.Success {
		t.Fatalf("actual TraceQuery failed: %v; %s", err, result.Summary)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != source {
		t.Fatal("read query changed native source bytes")
	}
	for _, record := range result.Observations {
		if record.SourceRef.PayloadRef == "" {
			continue
		}
		data, err := os.ReadFile(record.SourceRef.PayloadRef)
		if err != nil {
			t.Fatal(err)
		}
		var payload struct {
			WindowStats map[string]json.RawMessage `json:"window_stats"`
		}
		if err := json.Unmarshal(data, &payload); err != nil || payload.WindowStats == nil {
			t.Fatalf("invalid native public result: %v", err)
		}
		return result, payload.WindowStats, bus
	}
	t.Fatal("actual TraceQuery published no payload reference")
	return types.ToolResult{}, nil, nil
}

func traceResourceSemanticsFinalizer(t *testing.T, bus *types.BusContext, result types.ToolResult, predicate string, want []traceResourceSemanticsRow, lang string) {
	t.Helper()
	start, end := 8.0, 8.05
	rm := types.RequestModel{
		Intent: types.IntentExplain, Language: lang,
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactOtherObservedValue}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "8..8.05 seconds"},
	}
	bus.AnalysisIR = &types.AnalysisIR{RequestModel: rm, AnswerContract: types.AnswerContract{Language: lang}}
	bus.Language = lang
	bus.Mutable.SetRequestModel(rm)
	bus.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{result}})
	ctx := ctxbuilder.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	ledger := answerDocObservationLedger(ctx)
	projection := types.CompileTraceCausalProjectionSet(ledger)
	before, _ := json.Marshal([]any{result, ledger, projection})
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, expected := range want {
		found := false
		for _, record := range ledger.Records {
			if record.Predicate != predicate || record.Span.LineStart != expected.Line {
				continue
			}
			found = true
			if record.Producer != "trace_query" || record.Origin != types.AnswerEvidenceOriginRuntimeArtifact || record.SourceRef.QueryScopeID == "" {
				t.Errorf("resource lost actual runtime provenance: %+v", record)
			}
			for _, field := range []struct{ key, value string }{{"path", expected.Path}, {"address", expected.Address}, {"dev", expected.Dev}, {"domain", expected.Domain}} {
				actual := ""
				for _, note := range record.RichNotes {
					if strings.HasPrefix(note, field.key+"=") {
						actual = strings.TrimPrefix(note, field.key+"=")
					}
				}
				if actual != field.value {
					t.Errorf("published %s line %d: %s=%q want %q", predicate, expected.Line, field.key, actual, field.value)
				}
				if field.value != "" && !strings.Contains(record.Summary, field.key+"="+field.value) {
					t.Errorf("model-facing summary lost typed %s=%s: %s", field.key, field.value, record.Summary)
				}
				if field.value == "" && strings.Contains(record.Summary, field.key+"=") {
					t.Errorf("model-facing summary invented %s: %s", field.key, record.Summary)
				}
			}
			if !strings.Contains(prompt, record.Summary) {
				t.Errorf("actual finalizer context lost native row: %s", record.Summary)
			}
		}
		if !found {
			t.Errorf("handoff lost %s source line %d", predicate, expected.Line)
		}
	}
	for _, tree := range projection.Projections {
		if tree.PrimaryRootCause != nil || len(tree.PrimaryRootCauses) != 0 || len(tree.OnChainCauses) != 0 {
			t.Error("finite resource metadata acquired chain/root-cause authority")
		}
	}
	afterLedger := answerDocObservationLedger(ctx)
	after, _ := json.Marshal([]any{result, afterLedger, types.CompileTraceCausalProjectionSet(afterLedger)})
	if string(before) != string(after) {
		t.Error("model context rendering mutated accepted facts or causal eligibility")
	}
}
