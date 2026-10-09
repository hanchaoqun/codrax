package tracediag

import (
	"encoding/json"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"reflect"
	"strings"
	"testing"
)

// Invert exactly this one optional field before all historical schema hashes.
// No other field, type, declaration order or JSON tag is excluded.
func resultSchemaBeforePreferredFrameRate(t *testing.T, schema string) string {
	t.Helper()
	const added = "PreferredFrameRate|*tracequery.PreferredFrameRateResult|preferred_frame_rate,omitempty"
	var kept []string
	count := 0
	for _, field := range strings.Split(schema, ";") {
		if field == added {
			count++
			continue
		}
		kept = append(kept, field)
	}
	if count != 1 {
		t.Fatalf("expected one optional PreferredFrameRate field, got %d", count)
	}
	return strings.Join(kept, ";")
}

func TestPreferredFrameRateResultSchemaOptionalAndVisible(t *testing.T) {
	typ := reflect.TypeOf(tracequery.Result{})
	_, schema := detailSchemaFingerprint(typ)
	resultSchemaBeforePreferredFrameRate(t, schema)
	if policySkipsDetailField(&nonEventDetailPolicy, typ, "PreferredFrameRate") {
		t.Fatal("new rate result silently skipped")
	}
	for _, set := range []bool{false, true} {
		var r tracequery.Result
		if set {
			r.PreferredFrameRate = &tracequery.PreferredFrameRateResult{Status: "unavailable"}
		}
		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		json.Unmarshal(raw, &fields)
		if _, ok := fields["preferred_frame_rate"]; ok != set {
			t.Fatal("optional field changed", string(raw))
		}
	}
}
