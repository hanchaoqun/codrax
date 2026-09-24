package tracediag

import (
	"crypto/sha256"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func nonEventSchemaBeforeIOActivity(t *testing.T, typ reflect.Type, schema string) string {
	t.Helper()
	if typ != reflect.TypeOf(tracequery.WindowStats{}) {
		return schema
	}
	const added = "IOActivity|*tracequery.IOActivityStats|io_activity,omitempty"
	var prior []string
	count := 0
	for _, field := range strings.Split(schema, ";") {
		if field == added {
			count++
		} else {
			prior = append(prior, field)
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one reviewed IO activity addition, got %d: %s", count, schema)
	}
	return strings.Join(prior, ";")
}

func TestIOActivitySchemaEvolutionIsAdditive(t *testing.T) {
	typ := reflect.TypeOf(tracequery.WindowStats{})
	_, schema := detailSchemaFingerprint(typ)
	prior := nonEventSchemaBeforeIOActivity(t, typ, schema)
	sum := sha256.Sum256([]byte(prior))
	const want = "66e8903fcda92dae9dbba337b2a75a844097139d0949c32c35a6d0be7924b77f"
	if got := hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("IO activity addition changed prior WindowStats: got=%s want=%s", got, want)
	}
}

func TestIOActivityDetailFieldDisposition(t *testing.T) {
	cases := []struct {
		typ  reflect.Type
		want string
	}{
		{reflect.TypeOf(tracequery.IOActivityStats{}), "Window|*tracequery.IOActivityWindow|window,omitempty;WindowUnavailableReason|string|window_unavailable_reason,omitempty;Population|string|population;IssuerScope|string|issuer_scope;QueryPID|int|query_pid,omitempty;LineStart|int|line_start,omitempty;LineEnd|int|line_end,omitempty;BucketMs|float64|bucket_ms;GroupCount|int|group_count;Groups|[]tracequery.IOActivityGroup|groups,omitempty;OmittedGroups|int|omitted_groups;Coverage|tracequery.IOActivityCoverage|coverage"},
		{reflect.TypeOf(tracequery.IOActivityWindow{}), "StartTs|float64|start_ts;EndTs|float64|end_ts;EndInclusive|bool|end_inclusive,omitempty"},
		{reflect.TypeOf(tracequery.IOActivityCoverage{}), "SupportedEndpointCount|int|supported_endpoint_count;RejectedEndpointCount|int|rejected_endpoint_count;UnresolvedSourceCount|int|unresolved_source_count;Reasons|[]string|reasons,omitempty"},
		{reflect.TypeOf(tracequery.IOActivityGroup{}), "SourcePath|string|source_path;Layer|string|layer;EndpointFamily|string|endpoint_family;Phase|string|phase;Dev|string|dev;ByteCaliber|string|byte_caliber;Values|tracequery.IOActivityValues|values;Rates|*tracequery.IOActivityRates|rates,omitempty;Directions|[]tracequery.IOActivityDirection|directions;ReadWrite|*tracequery.IOActivityReadWriteRatio|read_write,omitempty;Buckets|[]tracequery.IOActivityBucket|buckets,omitempty;BucketCount|uint64|bucket_count;OmittedBuckets|uint64|omitted_buckets;BucketsUnavailableReason|string|buckets_unavailable_reason,omitempty"},
		{reflect.TypeOf(tracequery.IOActivityDirection{}), "Direction|string|direction;Values|tracequery.IOActivityValues|values;Rates|*tracequery.IOActivityRates|rates,omitempty"},
		{reflect.TypeOf(tracequery.IOActivityValues{}), "EventCount|int|event_count;KnownByteEventCount|int|known_byte_event_count;UnknownByteEventCount|int|unknown_byte_event_count;InvalidByteEventCount|int|invalid_byte_event_count;OverflowByteEventCount|int|overflow_byte_event_count;KnownBytes|*uint64|known_bytes,omitempty;BytesOverflow|bool|bytes_overflow;KnownSizeMeanBytes|*float64|known_size_mean_bytes,omitempty;SizeBuckets|[]tracequery.IOActivitySizeBucket|size_buckets"},
		{reflect.TypeOf(tracequery.IOActivitySizeBucket{}), "MinBytes|uint64|min_bytes;MaxBytes|*uint64|max_bytes,omitempty;Count|int|count"},
		{reflect.TypeOf(tracequery.IOActivityRates{}), "EventsPerSecond|float64|events_per_second;KnownBytesPerSecond|*float64|known_bytes_per_second,omitempty"},
		{reflect.TypeOf(tracequery.IOActivityReadWriteRatio{}), "EventDenominator|int|event_denominator;ReadEventShare|*float64|read_event_share,omitempty;WriteEventShare|*float64|write_event_share,omitempty;KnownByteDenominator|*uint64|known_byte_denominator,omitempty;ReadKnownByteShare|*float64|read_known_byte_share,omitempty;WriteKnownByteShare|*float64|write_known_byte_share,omitempty"},
		{reflect.TypeOf(tracequery.IOActivityBucket{}), "Window|tracequery.IOActivityWindow|window;Values|tracequery.IOActivityValues|values;Rates|*tracequery.IOActivityRates|rates,omitempty;Directions|[]tracequery.IOActivityDirection|directions"},
	}
	for _, tc := range cases {
		_, schema := detailSchemaFingerprint(tc.typ)
		if schema != tc.want {
			t.Errorf("%s needs explicit field-disposition review: got %s want %s", tc.typ, schema, tc.want)
		}
		for i := 0; i < tc.typ.NumField(); i++ {
			if policySkipsDetailField(&nonEventDetailPolicy, tc.typ, tc.typ.Field(i).Name) {
				t.Errorf("IO activity detail lost owner: %s.%s", tc.typ, tc.typ.Field(i).Name)
			}
		}
	}
}
