package tracediag

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Invert only this reviewed optional addition before the older schema witnesses.
// Keep every other field, type, order and JSON tag in their historical hashes.
func resultSchemaBeforeMeasurements(t *testing.T, schema string) string {
	t.Helper()
	const added = "Measurements|*tracequery.MeasurementsResult|measurements,omitempty"
	var previous []string
	count := 0
	for _, field := range strings.Split(schema, ";") {
		if field == added {
			count++
			continue
		}
		previous = append(previous, field)
	}
	if count != 1 {
		t.Fatalf("expected exactly one optional Measurements field, got %d", count)
	}
	return strings.Join(previous, ";")
}

func TestMeasurementsResultSchemaAddsOnlyOptionalObservations(t *testing.T) {
	typ := reflect.TypeOf(tracequery.Result{})
	current, schema := detailSchemaFingerprint(typ)
	const currentFullResult = "a475ef18e2544967e66953275a97c395b03ab05f692446f51c8fd7c10a6bd664"
	if current != currentFullResult {
		t.Fatalf("complete Measurements-era Result schema drift: got=%s want=%s\nschema=%s", current, currentFullResult, schema)
	}
	previous := resultSchemaBeforeMeasurements(t, schema)
	sum := sha256.Sum256([]byte(previous))
	const beforeMeasurements = "ba037fae6b5d3c1d80b578b3980de9b9974fe502a576b8c4cbbac81870ff120f"
	if got := hex.EncodeToString(sum[:]); got != beforeMeasurements {
		t.Fatalf("Measurements changed an unrelated Result field: got=%s want=%s\nprevious_schema=%s", got, beforeMeasurements, previous)
	}
	if policySkipsDetailField(&nonEventDetailPolicy, typ, "Measurements") {
		t.Fatal("Measurements must remain visible to the generic detail walker")
	}
	for _, populated := range []bool{false, true} {
		var result tracequery.Result
		if populated {
			result.Measurements = &tracequery.MeasurementsResult{Status: "unavailable"}
		}
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if _, present := fields["measurements"]; present != populated {
			t.Fatalf("Measurements optionality changed: populated=%t json=%s", populated, raw)
		}
	}
}

func TestMeasurementsDetailSchemaPins(t *testing.T) {
	for _, tc := range []struct {
		typ  reflect.Type
		want string
	}{
		{
			reflect.TypeOf(tracequery.MeasurementsResult{}),
			"Status|string|status;SourcePath|string|source_path;Window|tracequery.ProcessMeasurementsWindow|window;Rows|[]tracequery.MeasurementRow|rows,omitempty;TotalRows|int|total_rows;OmittedRows|int|omitted_rows;UnpositionedRows|int|unpositioned_rows;Caveats|[]string|caveats,omitempty",
		},
		{
			reflect.TypeOf(tracequery.MeasurementRow{}),
			"SourcePath|string|source_path;Line|int|line;SourceLine|int|source_line;Record|tracewire.MeasureInterval|record;ClippedStartNS|*int64|clipped_start_ns,omitempty,string;ClippedEndNS|*int64|clipped_end_ns,omitempty,string;Selection|string|selection;Unit|string|unit",
		},
	} {
		_, schema := detailSchemaFingerprint(tc.typ)
		if schema != tc.want {
			t.Errorf("%s detail schema drift:\n got=%s\nwant=%s", tc.typ, schema, tc.want)
		}
	}
}
