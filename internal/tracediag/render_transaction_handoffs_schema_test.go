package tracediag

import (
	"encoding/json"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"reflect"
	"strings"
	"testing"
)

func resultSchemaBeforeTransactionHandoffs(t *testing.T, schema string) string {
	t.Helper()
	schema = resultSchemaBeforeMeasurements(t, schema)
	const added = "TransactionHandoffs|*tracequery.TransactionHandoffsResult|transaction_handoffs,omitempty"
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
		t.Fatalf("expected exactly one optional transaction face, got %d", count)
	}
	return strings.Join(kept, ";")
}

func TestTransactionHandoffsSchemaOptionalAndVisible(t *testing.T) {
	typ := reflect.TypeOf(tracequery.Result{})
	_, schema := detailSchemaFingerprint(typ)
	resultSchemaBeforeTransactionHandoffs(t, schema)
	if policySkipsDetailField(&nonEventDetailPolicy, typ, "TransactionHandoffs") {
		t.Fatal("new protocol face skipped")
	}
	for _, set := range []bool{false, true} {
		var r tracequery.Result
		if set {
			r.TransactionHandoffs = &tracequery.TransactionHandoffsResult{Status: "unavailable"}
		}
		raw, err := json.Marshal(r)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		json.Unmarshal(raw, &fields)
		if _, ok := fields["transaction_handoffs"]; ok != set {
			t.Fatal("optional field changed")
		}
	}
}
