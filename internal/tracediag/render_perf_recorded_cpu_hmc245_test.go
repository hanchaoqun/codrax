package tracediag

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Exact reviewed additive fields only; all historical schema bytes/hashes stay.
func perfQualitySchemaBeforeRecordedCPU(t *testing.T, schema string) string {
	t.Helper()
	schema = removeReviewedPerfField(t, schema, "RecordedCPUs|[]tracequery.PerfValueCount|recorded_cpus,omitempty")
	return removeReviewedPerfField(t, schema, "RecordedCPUOmittedCount|int|recorded_cpu_omitted_count,omitempty")
}

func perfQualityHashBeforeRecordedCPU(t *testing.T) string {
	t.Helper()
	_, schema := detailSchemaFingerprint(reflect.TypeOf(tracequery.PerfQualitySummary{}))
	sum := sha256.Sum256([]byte(perfQualitySchemaBeforeRecordedCPU(t, schema)))
	return hex.EncodeToString(sum[:])
}

func TestHMC245RecordedCPUOptionalSchemaAndJSON(t *testing.T) {
	if got := perfQualityHashBeforeRecordedCPU(t); got != "a74f58cf80605d4a8085aa77ae487fcdfb0b6c2d0db9ba29562776d3cf2c89b7" {
		t.Fatalf("unrelated quality schema changed: %s", got)
	}
	typ := reflect.TypeOf(tracequery.PerfQualitySummary{})
	for _, field := range []string{"RecordedCPUs", "RecordedCPUOmittedCount"} {
		if policySkipsDetailField(&nonEventDetailPolicy, typ, field) {
			t.Fatalf("native recorded CPU data hidden: %s", field)
		}
	}
	for _, tc := range []struct {
		value         tracequery.PerfQualitySummary
		rows, omitted bool
	}{
		{tracequery.PerfQualitySummary{}, false, false},
		{tracequery.PerfQualitySummary{RecordedCPUs: []tracequery.PerfValueCount{{Value: "cpu:0", SampleCount: 1, Period: 3}}}, true, false},
		{tracequery.PerfQualitySummary{RecordedCPUs: []tracequery.PerfValueCount{{Value: "other_known_cpus", SampleCount: 4, Period: 10}, {Value: "unknown", SampleCount: 1, Period: 2}}, RecordedCPUOmittedCount: 4}, true, true},
	} {
		raw, err := json.Marshal(tc.value)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			t.Fatal("invalid JSON")
		}
		_, rows := fields["recorded_cpus"]
		_, omitted := fields["recorded_cpu_omitted_count"]
		if rows != tc.rows || omitted != tc.omitted {
			t.Fatalf("optional carrier changed: %s", raw)
		}
		var roundtrip tracequery.PerfQualitySummary
		if json.Unmarshal(raw, &roundtrip) != nil || !reflect.DeepEqual(roundtrip, tc.value) {
			t.Fatal("JSON changed CPU0/unknown/omission measures")
		}
	}
}
