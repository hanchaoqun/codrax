package tracediag

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func perfQualitySchemaBeforeFrameStatuses(t *testing.T, schema string) string {
	t.Helper()
	schema = perfQualitySchemaBeforeRecordedCPU(t, schema)
	return removeReviewedPerfField(t, schema, "FrameStatuses|[]tracequery.PerfValueCount|frame_statuses,omitempty")
}

func removeReviewedPerfField(t *testing.T, schema, added string) string {
	t.Helper()
	var previous []string
	count := 0
	for _, field := range strings.Split(schema, ";") {
		if field == added {
			count++
		} else {
			previous = append(previous, field)
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one reviewed optional field %q, got %d", added, count)
	}
	return strings.Join(previous, ";")
}

func TestPerfFrameIntegrityAdditiveSchemaAndJSON(t *testing.T) {
	qualityType := reflect.TypeOf(tracequery.PerfQualitySummary{})
	hash := perfQualityHashBeforeRecordedCPU(t)
	if hash != "a74f58cf80605d4a8085aa77ae487fcdfb0b6c2d0db9ba29562776d3cf2c89b7" {
		t.Fatal("unreviewed frame quality schema change")
	}
	hotType := reflect.TypeOf(tracequery.PerfHotspot{})
	_, hotSchema := detailSchemaFingerprint(hotType)
	hotSchema = perfSchemaBeforeRecordedTime(t, hotSchema)
	previous := removeReviewedPerfField(t, hotSchema, "CallchainIdentity|*tracequery.PerfCallchainIdentity|callchain_identity,omitempty")
	const oldHotspot = "Symbol|string|symbol,omitempty;DSO|string|dso,omitempty;Callchain|string|callchain,omitempty;Event|string|event,omitempty;WeightUnit|string|weight_unit,omitempty;Source|string|source,omitempty;SymbolizationStatus|string|symbolization_status,omitempty;SampleCount|int|sample_count,omitempty;Period|int64|period,omitempty;Percent|float64|percent,omitempty;ThreadIdentityCount|int|thread_identity_count,omitempty;ThreadIdentityCountExact|*bool|thread_identity_count_exact,omitempty;ThreadIdentityUnknownSampleCount|int|thread_identity_unknown_sample_count,omitempty;ThreadIdentities|[]tracequery.PerfThreadIdentity|thread_identities,omitempty;Threads|[]tracequery.ThreadRef|threads,omitempty;CPUs|[]int|cpus,omitempty;LineStart|int|line_start,omitempty;LineEnd|int|line_end,omitempty;Example|string|example,omitempty"
	if previous != oldHotspot {
		t.Fatal("callchain identity changed an unrelated legacy hotspot field")
	}
	_, identity := detailSchemaFingerprint(reflect.TypeOf(tracequery.PerfCallchainIdentity{}))
	if identity != "Form|string|form;Reason|string|reason,omitempty;ID|string|id,omitempty;FrameCount|int|frame_count,omitempty" {
		t.Fatalf("unreviewed identity leaf schema: %s", identity)
	}
	for _, tc := range []struct {
		typ            reflect.Type
		field, key     string
		empty, present any
	}{
		{qualityType, "FrameStatuses", "frame_statuses", tracequery.PerfQualitySummary{}, tracequery.PerfQualitySummary{FrameStatuses: []tracequery.PerfValueCount{{Value: "exact_frames", Period: 30}}}},
		{hotType, "CallchainIdentity", "callchain_identity", tracequery.PerfHotspot{}, tracequery.PerfHotspot{CallchainIdentity: &tracequery.PerfCallchainIdentity{Form: "opaque_flat_label", Reason: "missing"}}},
	} {
		if policySkipsDetailField(&nonEventDetailPolicy, tc.typ, tc.field) {
			t.Fatalf("native frame provenance hidden from generic detail: %s", tc.field)
		}
		for i, value := range []any{tc.empty, tc.present} {
			raw, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(raw, &fields); err != nil {
				t.Fatal(err)
			}
			if _, present := fields[tc.key]; present != (i == 1) {
				t.Fatalf("optional JSON carrier changed: %s", raw)
			}
		}
	}
}
