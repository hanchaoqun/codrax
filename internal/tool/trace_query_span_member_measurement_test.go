package tool

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc347SpanFamily() (tracequery.SemanticSpanFamily, types.ObservationSourceRef) {
	thread := tracequery.ThreadRef{PID: 17, Comm: "business"}
	family := tracequery.SemanticSpanFamily{SourcePath: "/capture.ftrace", Thread: thread,
		Members: []tracequery.TraceSpanSummary{
			{SourcePath: "/capture.ftrace", Thread: thread, Name: "work A", StartTs: 1, EndTs: 1.004, DurationMs: 4, StartLine: 2, EndLine: 4},
			{SourcePath: "/capture.ftrace", Thread: thread, Name: "work B", StartTs: 1.010, EndTs: 1.012, DurationMs: 2, StartLine: 8, EndLine: 12},
		}}
	ref := types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: family.SourcePath, QueryScopeID: "query", PayloadRef: "native.json",
		QueryWindowKnown: true, QueryWindowStartTs: 1, QueryWindowEndTs: 1.02}
	return family, ref
}

func TestHMC347SpanMembersValidateWholeSetWithoutGuessing(t *testing.T) {
	for name, mutate := range map[string]func(*tracequery.SemanticSpanFamily){
		"foreign family":          func(f *tracequery.SemanticSpanFamily) { f.SourcePath = "/other/capture.ftrace" },
		"foreign hidden tail":     func(f *tracequery.SemanticSpanFamily) { f.Members[1].SourcePath += ".other" },
		"other host":              func(f *tracequery.SemanticSpanFamily) { f.Members[1].Thread.PID++ },
		"other process same tid":  func(f *tracequery.SemanticSpanFamily) { f.Members[1].Thread.TGID++ },
		"missing name":            func(f *tracequery.SemanticSpanFamily) { f.Members[1].Name = "" },
		"duplicate physical pair": func(f *tracequery.SemanticSpanFamily) { f.Members[1] = f.Members[0] },
		"missing endpoint":        func(f *tracequery.SemanticSpanFamily) { f.Members[1].EndLine = 0 },
		"outside query":           func(f *tracequery.SemanticSpanFamily) { f.Members[1].EndTs = 2 },
		"wrong duration":          func(f *tracequery.SemanticSpanFamily) { f.Members[1].DurationMs = 100 },
		"nan":                     func(f *tracequery.SemanticSpanFamily) { f.Members[1].ActualStartTs = math.NaN() },
		"infinite actual":         func(f *tracequery.SemanticSpanFamily) { f.Members[1].ActualEndTs = math.Inf(1) },
		"incomplete actual":       func(f *tracequery.SemanticSpanFamily) { f.Members[1].ActualDurationMs = 2 },
		"reversed actual": func(f *tracequery.SemanticSpanFamily) {
			f.Members[1].ActualStartTs, f.Members[1].ActualEndTs = 1.014, 1.009
		},
		"not containing clipped": func(f *tracequery.SemanticSpanFamily) {
			f.Members[1].ActualStartTs, f.Members[1].ActualEndTs, f.Members[1].ActualDurationMs = 1.011, 1.013, 2
		},
	} {
		t.Run(name, func(t *testing.T) {
			family, ref := hmc347SpanFamily()
			mutate(&family)
			if _, ok := traceQuerySpanMemberMeasurementTable(family, ref, 1, 1.02); ok {
				t.Fatal("invalid member set acquired native display; do not partially publish a bad set")
			}
		})
	}
}

func TestHMC347SpanMembersKeepOverlapClippingOrderAndZeroOrigin(t *testing.T) {
	family, ref := hmc347SpanFamily()
	family.Members[1].StartTs, family.Members[1].EndTs, family.Members[1].DurationMs = 1.002, 1.006, 4
	family.Members[0].ActualStartTs, family.Members[0].ActualEndTs, family.Members[0].ActualDurationMs = .999, 1.004, 5
	before, _ := json.Marshal(family)
	table, ok := traceQuerySpanMemberMeasurementTable(family, ref, 1, 1.02)
	if !ok || len(table.Rows) != 2 || table.Rows[0][2] != "1.000000" || table.Rows[0][5] != "0.999000" || table.Rows[0][7] != "5" ||
		table.Rows[1][2] != "1.002000" || table.Rows[1][1] != "work B" || table.MemberSet != nil || table.DisplayPopulation != nil || table.DefaultPresentation {
		t.Fatalf("native overlap/clipped ruler/order was reinterpreted: %+v", table)
	}
	after, _ := json.Marshal(family)
	if !reflect.DeepEqual(before, after) || !strings.Contains(strings.Join(table.Notes, " "), "集合首尾包络中的空档") {
		t.Fatal("member publication modified native facts or lost interval-set boundary")
	}
	family.Members = family.Members[:1]
	family.Members[0].StartTs, family.Members[0].EndTs, family.Members[0].DurationMs = 0, .004, 4
	family.Members[0].ActualStartTs, family.Members[0].ActualEndTs, family.Members[0].ActualDurationMs = 0, 0, 0
	if table, ok = traceQuerySpanMemberMeasurementTable(family, ref, 0, .02); !ok || table.Rows[0][2] != "0.000000" || table.Rows[0][5] != "0.000000" {
		t.Fatal("valid zero origin was treated as an unknown endpoint")
	}
}

func TestHMC347SpanMemberPredicateHasOnlyMemberDisplayView(t *testing.T) {
	family, ref := hmc347SpanFamily()
	table, _ := traceQuerySpanMemberMeasurementTable(family, ref, 1, 1.02)
	r := types.ObservationRecord{ID: "native", Producer: "trace_query", Predicate: "trace_span_members_observation", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
		SourceRef: ref, GroundingPolicy: types.ClaimGroundingHard, ProvenanceLane: types.ObservationProvenanceArtifactSpan}
	for _, view := range []types.RuntimeMeasurementView{types.RuntimeMeasurementMembers, types.RuntimeMeasurementSummary, types.RuntimeMeasurementTimeline, types.RuntimeMeasurementDistribution} {
		table.ObservationID, table.View = r.ID, view
		raw, _ := json.Marshal(types.RuntimeMeasurementPublication{Version: 1, ObservationID: r.ID, Source: ref, Tables: []types.RuntimeMeasurementTable{table}})
		r.RichNotes = []string{types.TraceNoteKeyRuntimeMeasurement + "=" + string(raw)}
		_, valid := types.DecodeRuntimeMeasurementPublication(r)
		if valid != (view == types.RuntimeMeasurementMembers) {
			t.Fatalf("member table granted unrelated %s view", view)
		}
	}
}
