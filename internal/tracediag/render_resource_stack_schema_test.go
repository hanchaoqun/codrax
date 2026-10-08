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

// Resource stacks are an optional factual-detail face. Reverse only this
// exact extension so older priority/profile/CPU evolution witnesses retain
// their complete original field, type, declaration-order and JSON-tag pins.
func resultSchemaBeforeResourceStack(t *testing.T, schema string) string {
	t.Helper()
	const added = "ResourceStack|*tracequery.ResourceStackResult|resource_stack,omitempty"
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
		t.Fatalf("expected exactly one optional resource-stack field, got %d", count)
	}
	return strings.Join(previous, ";")
}

func TestResourceStackResultSchemaAddsOnlyOptionalStack(t *testing.T) {
	_, schema := detailSchemaFingerprint(reflect.TypeOf(tracequery.Result{}))
	sum := sha256.Sum256([]byte(resultSchemaBeforeResourceStack(t, schema)))
	if hex.EncodeToString(sum[:]) != "4e71f90506825b960cdc6b9316048169663b995a59d234cd73e20917ae9cbc93" {
		t.Fatal("resource-stack addition changed an unrelated result field")
	}
	for _, populated := range []bool{false, true} {
		var result tracequery.Result
		if populated {
			result.ResourceStack = &tracequery.ResourceStackResult{Status: "unavailable", Reason: "no_resource_stack_observations"}
		}
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		_, present := fields["resource_stack"]
		if present != populated {
			t.Fatalf("resource-stack optionality changed: populated=%t json=%s", populated, raw)
		}
	}
}
