package toolparam

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

var repairPolicy = types.ToolParamCompatConfig{Mode: types.ToolParamCompatRepair}

func TestNormalize_StringScalarsAgainstSchema(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "offset":{"type":"integer"},
	    "limit":{"type":"integer"},
	    "ignore_case":{"type":"boolean"},
	    "threshold":{"type":"number"}
	  }
	}`)
	raw := json.RawMessage(`{"offset":"140","limit":"30","ignore_case":"true","threshold":"0.75"}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if !report.Changed() {
		t.Fatal("expected scalar repairs")
	}
	var decoded struct {
		Offset     int     `json:"offset"`
		Limit      int     `json:"limit"`
		IgnoreCase bool    `json:"ignore_case"`
		Threshold  float64 `json:"threshold"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("normalized payload must decode: %v\n%s", err, got)
	}
	if decoded.Offset != 140 || decoded.Limit != 30 || !decoded.IgnoreCase || decoded.Threshold != 0.75 {
		t.Fatalf("unexpected normalized values: %+v", decoded)
	}
}

func TestNormalize_StringWrappedArrays(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "items":{"type":"array","items":{"type":"object"}},
	    "keywords":{"type":"array","items":{"type":"string"}}
	  }
	}`)
	raw := json.RawMessage(`{
	  "items":"[{\"source\":\"internal/types/enums.go\",\"line_start\":146}]",
	  "keywords":"agent, count，enum"
	}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if len(report.Repairs) != 2 {
		t.Fatalf("expected two repairs, got %+v", report)
	}
	var decoded struct {
		Items    []map[string]any `json:"items"`
		Keywords []string         `json:"keywords"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("normalized payload must decode: %v\n%s", err, got)
	}
	if len(decoded.Items) != 1 || decoded.Items[0]["source"] != "internal/types/enums.go" {
		t.Fatalf("items not recovered: %+v", decoded.Items)
	}
	if strings.Join(decoded.Keywords, "|") != "agent|count|enum" {
		t.Fatalf("keywords not split conservatively: %+v", decoded.Keywords)
	}
}

func TestNormalize_StringWrappedRootObject(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "path":{"type":"string"},
	    "offset":{"type":"integer"}
	  }
	}`)
	raw := json.RawMessage(`"{\"path\":\"internal/types/enums.go\",\"offset\":\"146\"}"`)

	got, report := Normalize(raw, schema, repairPolicy)
	if len(report.Repairs) != 2 {
		t.Fatalf("expected root object and nested scalar repairs, got %+v", report)
	}
	var decoded struct {
		Path   string `json:"path"`
		Offset int    `json:"offset"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("normalized payload must decode: %v\n%s", err, got)
	}
	if decoded.Path != "internal/types/enums.go" || decoded.Offset != 146 {
		t.Fatalf("unexpected normalized values: %+v", decoded)
	}
}

func TestNormalize_AuditReportsButDoesNotMutate(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"offset":{"type":"integer"}}}`)
	raw := json.RawMessage(`{"offset":"140"}`)

	got, report := Normalize(raw, schema, types.ToolParamCompatConfig{Mode: types.ToolParamCompatAudit})
	if !report.Changed() {
		t.Fatal("audit mode should report repairable payload")
	}
	if string(got) != string(raw) {
		t.Fatalf("audit mode must not mutate payload: got %s want %s", got, raw)
	}
}

func TestNormalize_UnionNumberFallsThroughAfterIntegerMiss(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"score":{"type":["integer","number"]}}}`)
	raw := json.RawMessage(`{"score":"1.5"}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if !report.Changed() {
		t.Fatal("expected number repair for integer|number union")
	}
	var decoded struct {
		Score float64 `json:"score"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("normalized payload must decode: %v\n%s", err, got)
	}
	if decoded.Score != 1.5 {
		t.Fatalf("score = %v, want 1.5; raw=%s", decoded.Score, got)
	}
}

func TestNormalize_OffIsByteIdentical(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"offset":{"type":"integer"}}}`)
	raw := json.RawMessage(`{"offset":"140"}`)

	got, report := Normalize(raw, schema, types.ToolParamCompatConfig{})
	if report.Changed() {
		t.Fatalf("off mode should not report repairs: %+v", report)
	}
	if string(got) != string(raw) {
		t.Fatalf("off mode must be byte-identical: got %s want %s", got, raw)
	}
}

func TestNormalize_DoesNotInventOrGuess(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "path":{"type":"string"},
	    "offset":{"type":"integer"},
	    "enabled":{"type":"boolean"},
	    "keywords":{"type":"array","items":{"type":"string"}}
	  },
	  "required":["path"]
	}`)
	raw := json.RawMessage(`{"offset":"one","enabled":"yes","unknown":"keep me"}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if report.Changed() {
		t.Fatalf("non-mechanical values must not be repaired: %+v", report)
	}
	if string(got) != string(raw) {
		t.Fatalf("unrepairable payload must pass through unchanged: got %s want %s", got, raw)
	}
}

func TestNormalize_StringArraySplitCanBeDisabled(t *testing.T) {
	disabled := false
	schema := json.RawMessage(`{"type":"object","properties":{"keywords":{"type":"array","items":{"type":"string"}}}}`)
	raw := json.RawMessage(`{"keywords":"agent,count"}`)

	got, report := Normalize(raw, schema, types.ToolParamCompatConfig{
		Mode:              types.ToolParamCompatRepair,
		SplitStringArrays: &disabled,
	})
	if report.Changed() {
		t.Fatalf("split_string_arrays=false should skip delimited string repair: %+v", report)
	}
	if string(got) != string(raw) {
		t.Fatalf("payload must remain unchanged when split disabled: got %s want %s", got, raw)
	}
}
