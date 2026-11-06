package tracediag

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func perfQualitySchemaBeforeBranchCensus(t *testing.T, schema string) string {
	t.Helper()
	return removeReviewedPerfField(t, schema, "BranchRecords|*tracequery.PerfBranchRecordCensus|branch_records,omitempty")
}

func TestHMC358PerfBranchCensusAdditiveSchema(t *testing.T) {
	_, leaf := detailSchemaFingerprint(reflect.TypeOf(tracequery.PerfBranchRecordCensus{}))
	if leaf != "SamplesWithCount|int|samples_with_count;SamplesWithEntries|int|samples_with_entries;SamplesWithoutCount|int|samples_without_count;RecordedEntries|int64|recorded_entries" {
		t.Fatalf("census leaf changed: %s", leaf)
	}
	for _, tc := range []struct {
		census *tracequery.PerfBranchRecordCensus
		want   string
	}{
		{nil, "{}"},
		{&tracequery.PerfBranchRecordCensus{SamplesWithCount: 1}, `{"branch_records":{"samples_with_count":1,"samples_with_entries":0,"samples_without_count":0,"recorded_entries":0}}`},
	} {
		raw, err := json.Marshal(tracequery.PerfQualitySummary{BranchRecords: tc.census})
		if err != nil || string(raw) != tc.want {
			t.Fatalf("optional census/recorded-zero shape: %v %s", err, raw)
		}
	}
}
