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

// Reverse only the reviewed optional tree field. The complete old Result
// declaration order, field types and JSON tags remain in the older witnesses.
func resultSchemaBeforePerfSamplingTree(t *testing.T, schema string) string {
	t.Helper()
	const added = "PerfSamplingTree|*tracequery.PerfSamplingTreeResult|perf_sampling_tree,omitempty"
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
		t.Fatalf("expected exactly one optional sampling-tree field, got %d", count)
	}
	return strings.Join(previous, ";")
}

func TestPerfSamplingTreeResultSchemaAddsOnlyOptionalTree(t *testing.T) {
	typ := reflect.TypeOf(tracequery.Result{})
	current, schema := detailSchemaFingerprint(typ)
	const currentFullResult = "aefd0aaa1adde3b00685730bd355eb8f435e1b7d78d3ea416707a6bf542e0bd1"
	if current != currentFullResult {
		t.Fatalf("sampling-tree Result schema drift: got=%s want=%s\nschema=%s", current, currentFullResult, schema)
	}
	previous := resultSchemaBeforePerfSamplingTree(t, schema)
	sum := sha256.Sum256([]byte(previous))
	const beforeSamplingTree = "a475ef18e2544967e66953275a97c395b03ab05f692446f51c8fd7c10a6bd664"
	if got := hex.EncodeToString(sum[:]); got != beforeSamplingTree {
		t.Fatalf("sampling tree changed an unrelated Result field: got=%s want=%s\nprevious=%s", got, beforeSamplingTree, previous)
	}
	if policySkipsDetailField(&nonEventDetailPolicy, typ, "PerfSamplingTree") {
		t.Fatal("sampling tree must remain visible to the generic detail walker")
	}
	for _, populated := range []bool{false, true} {
		var result tracequery.Result
		if populated {
			result.PerfSamplingTree = &tracequery.PerfSamplingTreeResult{Status: "unavailable", Reason: "complete_scan_retention_budget_exceeded"}
		}
		raw, err := json.Marshal(result)
		if err != nil {
			t.Fatal(err)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		if _, present := fields["perf_sampling_tree"]; present != populated {
			t.Fatalf("sampling-tree optionality changed: populated=%t json=%s", populated, raw)
		}
	}
}
