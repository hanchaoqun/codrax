package tracediag

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Remove only this reviewed additive field before the unchanged historical
// schema assertion. No legacy hash, field, or rendering visibility is relaxed.
func perfSchemaBeforeRecordedTime(t *testing.T, schema string) string {
	t.Helper()
	return removeReviewedPerfField(t, schema, "RecordedTime|*tracequery.PerfSampleTimeBounds|recorded_time,omitempty")
}

func TestHMC246SamplingTimeSchemaAndJSON(t *testing.T) {
	typ := reflect.TypeOf(tracequery.PerfSampleTimeBounds{})
	_, schema := detailSchemaFingerprint(typ)
	if schema != "KnownSampleCount|int|known_sample_count;UnknownSampleCount|int|unknown_sample_count;FirstTs|*float64|first_ts;LastTs|*float64|last_ts" {
		t.Fatalf("unreviewed recorded-time schema: %s", schema)
	}
	for _, parent := range []reflect.Type{reflect.TypeOf(tracequery.PerfCohort{}), reflect.TypeOf(tracequery.PerfHotspot{})} {
		if policySkipsDetailField(&nonEventDetailPolicy, parent, "RecordedTime") {
			t.Fatalf("recorded-time facts hidden in %s", parent.Name())
		}
	}
	for _, field := range []string{"KnownSampleCount", "UnknownSampleCount", "FirstTs", "LastTs"} {
		if policySkipsDetailField(&nonEventDetailPolicy, typ, field) {
			t.Fatalf("recorded-time leaf hidden: %s", field)
		}
	}
	zero, end := 0.0, 1.5
	for _, value := range []tracequery.PerfSampleTimeBounds{
		{KnownSampleCount: 1, FirstTs: &zero, LastTs: &zero},
		{UnknownSampleCount: 2},
		{KnownSampleCount: 1, UnknownSampleCount: 1, FirstTs: &end, LastTs: &end},
	} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		var decoded tracequery.PerfSampleTimeBounds
		if json.Unmarshal(raw, &decoded) != nil || !reflect.DeepEqual(value, decoded) {
			t.Fatalf("zero/unknown changed after JSON: %s", raw)
		}
	}
	for _, empty := range []any{tracequery.PerfCohort{}, tracequery.PerfHotspot{}} {
		raw, _ := json.Marshal(empty)
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		if _, ok := fields["recorded_time"]; ok {
			t.Fatal("legacy/no-measurement object acquired zero timestamps")
		}
	}
}
