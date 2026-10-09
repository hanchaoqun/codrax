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

// Reverse exactly the optional process-measurement face. Historical Result
// witnesses still pin every prior field, type, declaration order and JSON tag.
func resultSchemaBeforeProcessMeasurements(t *testing.T, schema string) string {
	t.Helper()
	const added = "ProcessMeasurements|*tracequery.ProcessMeasurementsResult|process_measurements,omitempty"
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
		t.Fatalf("expected exactly one optional process-measurements field, got %d", count)
	}
	return strings.Join(previous, ";")
}

func TestProcessMeasurementsResultSchemaAddsOnlyOptionalObservations(t *testing.T) {
	typ := reflect.TypeOf(tracequery.Result{})
	_, schema := detailSchemaFingerprint(typ)
	previous := resultSchemaBeforeProcessMeasurements(t, schema)
	sum := sha256.Sum256([]byte(previous))
	// Complete pre-addition Result schema, including all earlier extensions.
	const beforeProcessMeasurements = "c001f2e6f6ef765d742916acc0a9cb54bb0313b20bcdb4ab9ed814ae8d210456"
	if got := hex.EncodeToString(sum[:]); got != beforeProcessMeasurements {
		t.Fatalf("process measurements changed an unrelated result field: got=%s want=%s\nprevious_schema=%s", got, beforeProcessMeasurements, previous)
	}
	if policySkipsDetailField(&nonEventDetailPolicy, typ, "ProcessMeasurements") {
		t.Fatal("process measurements cannot silently enter the detail skip policy")
	}
	for _, populated := range []bool{false, true} {
		var result tracequery.Result
		if populated {
			result.ProcessMeasurements = &tracequery.ProcessMeasurementsResult{Status: "unavailable"}
		}
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if _, present := fields["process_measurements"]; present != populated {
			t.Fatalf("process-measurements optionality changed: populated=%t json=%s", populated, raw)
		}
	}
}
