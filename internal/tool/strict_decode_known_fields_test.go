package tool

// strict_decode_known_fields_test.go — LT-HYG decoder-remap hint pins
// (§29.75 立案, 2026-07-14): a model that INVENTS a parameter and is
// rejected by strict decode must see the tool's real parameter list in the
// rejection text (reflected from the schema, never hand-copied), so the
// retry re-aims instead of re-guessing. Witness: `ignore_case` fabricated on
// trace_query (a grep field).

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// ① schema reflection: top-level properties in DECLARATION order; nested
// object properties never leak into the top-level list.
func TestStrictDecodeSchemaTopLevelFieldsDeclarationOrder(t *testing.T) {
	schema := json.RawMessage(`{
	  "type": "object",
	  "properties": {
	    "view": {"type":"string"},
	    "path": {"type":"string"},
	    "nested": {"type":"object","properties":{"inner_only":{"type":"string"}}},
	    "limit": {"type":"integer"}
	  }
	}`)
	got := strictDecodeSchemaTopLevelFields(schema)
	if strings.Join(got, ",") != "view,path,nested,limit" {
		t.Fatalf("declaration-order reflection broken: %v", got)
	}
	if strictDecodeSchemaTopLevelFields(nil) != nil {
		t.Fatal("nil schema must reflect no fields")
	}
	if strictDecodeSchemaTopLevelFields(json.RawMessage(`not json`)) != nil {
		t.Fatal("unparseable schema must reflect no fields (additive hint, never a gate)")
	}
}

// ② the fabricated-field rejection carries the reflected list; a matched
// misplaced-field hint suppresses it (the relocate guidance already names
// the correct paths); nil-schema callers stay byte-identical.
func TestStrictDecodeUnknownFieldRejectionTeachesParameterList(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"view":{"type":"string"},"pattern":{"type":"string"}}}`)
	decodeErr := func(payload string) error {
		var dst struct {
			View    string `json:"view"`
			Pattern string `json:"pattern"`
		}
		dec := json.NewDecoder(strings.NewReader(payload))
		dec.DisallowUnknownFields()
		err := dec.Decode(&dst)
		if err == nil {
			t.Fatalf("expected strict-decode error for %q", payload)
		}
		return err
	}

	err := decodeErr(`{"view":"window_stats","ignore_case":true}`)
	res, _ := failStrictDecodeWithErrorSchema("trace_query", time.Now(), err, nil, nil, schema)
	if !strings.Contains(res.Summary, `unknown field "ignore_case"`) {
		t.Fatalf("rejection must keep the unknown-field naming: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "this tool accepts only these parameters: view, pattern") {
		t.Fatalf("fabricated-field rejection must teach the reflected parameter list: %s", res.Summary)
	}

	// Hint-matched relocation keeps its own guidance and does NOT append the
	// list (the field exists in the schema — it is misplaced, not invented).
	hints := []MisplacedFieldHint{{Field: "ignore_case", ContainerNames: []string{"top"}, CorrectPaths: []string{"filters.ignore_case"}}}
	hinted, _ := failStrictDecodeWithErrorSchema("trace_query", time.Now(), decodeErr(`{"view":"x","ignore_case":true}`), hints, nil, schema)
	if strings.Contains(hinted.Summary, "accepts only these parameters") {
		t.Fatalf("relocate-hinted rejection must not double-teach the list: %s", hinted.Summary)
	}

	// Schema-less legacy callers: byte-identical rejection (no list).
	plain, _ := failStrictDecodeWithError("trace_query", time.Now(), decodeErr(`{"view":"x","ignore_case":true}`), nil, nil)
	if strings.Contains(plain.Summary, "accepts only these parameters") {
		t.Fatalf("nil-schema caller must stay list-free: %s", plain.Summary)
	}
}

// ③ witness wiring: the production trace_query Execute path rejects the
// fabricated `ignore_case` WITH its real schema-reflected parameter list
// (first and last schema fields prove the whole list rode along).
func TestTraceQueryFabricatedFieldRejectionCarriesKnownFields(t *testing.T) {
	tool := &TraceQuery{}
	ctx := &types.BusContext{}
	res, err := tool.Execute(ctx, json.RawMessage(`{"view":"window_stats","ignore_case":true}`))
	if err == nil {
		t.Fatal("fabricated field must fail loud")
	}
	if res.Success {
		t.Fatal("fabricated field must reject")
	}
	if !strings.Contains(res.Summary, `unknown field "ignore_case"`) {
		t.Fatalf("summary must name the fabricated field: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "this tool accepts only these parameters:") {
		t.Fatalf("summary must teach the parameter list: %s", res.Summary)
	}
	for _, field := range []string{"source", "view", "pattern", "time_start", "bucket_ms"} {
		if !strings.Contains(res.Summary, field) {
			t.Fatalf("parameter list must include schema field %q: %s", field, res.Summary)
		}
	}
	if res.Repair == nil || res.Repair.Code != "tool_param_unknown_field" {
		t.Fatalf("typed repair must stay on the unknown-field code: %+v", res.Repair)
	}
}
