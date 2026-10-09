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

// Reverse exactly the new optional navigation face before applying all
// historical Result schema witnesses; preserve their hashes and field order.
func resultSchemaBeforeRenderingCandidates(t *testing.T, schema string) string {
	t.Helper()
	schema = resultSchemaBeforeProcessMeasurements(t, schema)
	const added = "RenderingCandidates|*tracequery.RenderingCandidatesResult|rendering_candidates,omitempty"
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
		t.Fatalf("expected one optional rendering-candidates field, got %d", count)
	}
	return strings.Join(previous, ";")
}

func TestRenderingCandidatesResultSchemaAddsOnlyOptionalNavigation(t *testing.T) {
	_, schema := detailSchemaFingerprint(reflect.TypeOf(tracequery.Result{}))
	sum := sha256.Sum256([]byte(resultSchemaBeforeRenderingCandidates(t, schema)))
	if hex.EncodeToString(sum[:]) != "4de38a5440a99bb3663c1f12174d7f99e08f131704e284a112b8ae16a7df4d33" {
		t.Fatal("rendering navigation addition changed an unrelated result field")
	}
	for _, populated := range []bool{false, true} {
		var r tracequery.Result
		if populated {
			r.RenderingCandidates = &tracequery.RenderingCandidatesResult{Status: "available"}
		}
		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if _, present := fields["rendering_candidates"]; present != populated {
			t.Fatalf("rendering navigation optionality changed: %s", raw)
		}
	}
}
