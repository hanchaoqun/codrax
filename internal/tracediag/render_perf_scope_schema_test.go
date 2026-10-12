package tracediag

import (
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// The old aggregate leaf was not individually pinned. Review its complete
// pre-change shape here; only optional Scope may be added, never hide a field.
func TestPerfAggregateSchemaAddsOnlyOptionalScope(t *testing.T) {
	for _, tc := range []struct {
		typ reflect.Type
		old string
	}{
		{reflect.TypeOf(tracequery.PerfCohort{}), "Event|string|event;WeightUnit|string|weight_unit;WeightStatus|string|weight_status;SampleCount|int|sample_count,omitempty;TotalPeriod|int64|total_period,omitempty;ThreadIdentityCount|int|thread_identity_count,omitempty;ThreadIdentityCountExact|*bool|thread_identity_count_exact,omitempty;ThreadIdentityUnknownSampleCount|int|thread_identity_unknown_sample_count,omitempty;Quality|*tracequery.PerfQualitySummary|quality,omitempty;TopSymbols|[]tracequery.PerfHotspot|top_symbols,omitempty;TopDSO|[]tracequery.PerfHotspot|top_dso,omitempty;TopCallchains|[]tracequery.PerfHotspot|top_callchains,omitempty;TopThreads|[]tracequery.PerfThreadSummary|top_threads,omitempty;TopEvents|[]tracequery.PerfHotspot|top_events,omitempty"},
		{reflect.TypeOf(tracequery.PerfTimelineCohort{}), "Event|string|event;WeightUnit|string|weight_unit;WeightStatus|string|weight_status;SampleCount|int|sample_count,omitempty;Period|int64|period,omitempty;TopSymbol|string|top_symbol,omitempty;TopDSO|string|top_dso,omitempty"},
	} {
		_, schema := detailSchemaFingerprint(tc.typ)
		const added = "Scope|*tracequery.PerfCohortScope|scope,omitempty;"
		if !strings.HasPrefix(schema, added) || strings.TrimPrefix(schema, added) != tc.old {
			t.Fatalf("%s changed outside optional source scope: %s", tc.typ.Name(), schema)
		}
		if policySkipsDetailField(&nonEventDetailPolicy, tc.typ, "Scope") {
			t.Fatal("native cohort source scope hidden")
		}
	}
	_, scope := detailSchemaFingerprint(reflect.TypeOf(tracequery.PerfCohortScope{}))
	const want = "ID|string|id;SourcePath|string|source_path;SourceGeneration|string|source_generation;Producer|string|producer;SampleKind|string|sample_kind;Clock|string|clock;ThreadIdentity|*tracequery.PerfThreadIdentity|thread_identity,omitempty;Window|tracequery.ResourceStackWindow|window"
	if scope != want {
		t.Fatalf("new native scope needs explicit review: %s", scope)
	}
}
