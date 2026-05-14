package toolparam

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

var repairPolicy = types.ToolParamCompatConfig{Mode: types.ToolParamCompatRepair}

func splitArrayRepairPolicy() types.ToolParamCompatConfig {
	enabled := true
	return types.ToolParamCompatConfig{
		Mode:              types.ToolParamCompatRepair,
		SplitStringArrays: &enabled,
	}
}

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

	got, report := Normalize(raw, schema, splitArrayRepairPolicy())
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

func TestNormalize_StringWrappedArrayEscapesBareQuotesInTextValue(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "items":{
	      "type":"array",
	      "items":{
	        "type":"object",
	        "properties":{
	          "summary":{"type":"string"},
	          "source":{"type":"string"},
	          "line_start":{"type":"integer"}
	        }
	      }
	    }
	  }
	}`)
	raw := json.RawMessage(`{
	  "items":"[{\"summary\":\"EvidenceNodeSpec says IDPrefix appends \"_tN\" and may describe a \"key\": \"value\" pair in prose\",\"source\":\"internal/analysis/compiler/templates.go\",\"line_start\":\"82\"}]"
	}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if !hasRepair(report, "$.items", "json_string_array_quote_escape") {
		t.Fatalf("expected quote-escape array repair, got %+v", report)
	}
	if !hasRepair(report, "$.items[0].line_start", "string_integer") {
		t.Fatalf("expected nested scalar repair after array unwrap, got %+v", report)
	}
	var decoded struct {
		Items []struct {
			Summary   string `json:"summary"`
			Source    string `json:"source"`
			LineStart int    `json:"line_start"`
		} `json:"items"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("normalized payload must decode: %v\n%s", err, got)
	}
	if len(decoded.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(decoded.Items))
	}
	item := decoded.Items[0]
	if item.LineStart != 82 || item.Source != "internal/analysis/compiler/templates.go" {
		t.Fatalf("unexpected item: %+v", item)
	}
	for _, want := range []string{`"_tN"`, `"key": "value"`} {
		if !strings.Contains(item.Summary, want) {
			t.Fatalf("summary lost quoted text %q: %q", want, item.Summary)
		}
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

func TestNormalize_UnwrapsToolArgumentEnvelope(t *testing.T) {
	schema := envelopeCompatTestSchema()
	raw := json.RawMessage(`{
	  "name":"emit_analysis",
	  "arguments":"{\"intent\":\"explain\",\"keywords\":[\"agent\"],\"limit\":\"25\"}"
	}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if !hasRepair(report, "$", "tool_argument_envelope_arguments_json_string_object") {
		t.Fatalf("expected argument-envelope repair, got %+v", report)
	}
	if !hasRepair(report, "$.limit", "string_integer") {
		t.Fatalf("expected nested scalar repair after envelope unwrap, got %+v", report)
	}
	var decoded struct {
		Intent   string   `json:"intent"`
		Keywords []string `json:"keywords"`
		Limit    int      `json:"limit"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("normalized payload must decode: %v\n%s", err, got)
	}
	if decoded.Intent != "explain" || strings.Join(decoded.Keywords, "|") != "agent" || decoded.Limit != 25 {
		t.Fatalf("unexpected normalized payload: %+v", decoded)
	}
}

func TestNormalize_UnwrapsNestedFunctionArgumentEnvelope(t *testing.T) {
	schema := envelopeCompatTestSchema()
	raw := json.RawMessage(`{
	  "id":"call_1",
	  "type":"function",
	  "function":{
	    "name":"emit_analysis",
	    "arguments":{"intent":"explain","keywords":["agent"],"limit":"25"}
	  }
	}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if !hasRepair(report, "$", "tool_function_envelope_arguments_object") {
		t.Fatalf("expected nested function-envelope repair, got %+v", report)
	}
	var decoded struct {
		Intent string `json:"intent"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("normalized payload must decode: %v\n%s", err, got)
	}
	if decoded.Intent != "explain" || decoded.Limit != 25 {
		t.Fatalf("unexpected normalized payload: %+v", decoded)
	}
}

func TestNormalize_UnwrapsDoubleEncodedToolArgumentEnvelope(t *testing.T) {
	schema := envelopeCompatTestSchema()
	raw := json.RawMessage(`{
	  "arguments":"\"{\\\"intent\\\":\\\"explain\\\",\\\"keywords\\\":[\\\"agent\\\"],\\\"limit\\\":\\\"25\\\"}\""
	}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if !hasRepair(report, "$", "tool_argument_envelope_arguments_json_string_object_nested") {
		t.Fatalf("expected nested string argument-envelope repair, got %+v", report)
	}
	var decoded struct {
		Intent string `json:"intent"`
		Limit  int    `json:"limit"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("normalized payload must decode: %v\n%s", err, got)
	}
	if decoded.Intent != "explain" || decoded.Limit != 25 {
		t.Fatalf("unexpected normalized payload: %+v", decoded)
	}
}

func TestNormalize_DoesNotUnwrapSchemaArgumentsProperty(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "arguments":{"type":"string"},
	    "intent":{"type":"string"}
	  }
	}`)
	raw := json.RawMessage(`{"arguments":"{\"intent\":\"explain\"}"}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if report.Changed() {
		t.Fatalf("schema-owned arguments field must not be unwrapped: %+v", report)
	}
	if string(got) != string(raw) {
		t.Fatalf("payload must remain unchanged: got %s want %s", got, raw)
	}
}

func TestNormalize_DoesNotDropUnknownEnvelopeFields(t *testing.T) {
	schema := envelopeCompatTestSchema()
	raw := json.RawMessage(`{"arguments":"{\"intent\":\"explain\"}","unexpected":"keep me"}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if report.Changed() {
		t.Fatalf("ambiguous envelope with unknown outer fields must not be repaired: %+v", report)
	}
	if string(got) != string(raw) {
		t.Fatalf("payload must remain unchanged: got %s want %s", got, raw)
	}
}

func TestNormalize_DoesNotUnwrapEnvelopeWithoutSchemaOverlap(t *testing.T) {
	schema := envelopeCompatTestSchema()
	raw := json.RawMessage(`{"arguments":"{\"path\":\"internal/types/enums.go\"}"}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if report.Changed() {
		t.Fatalf("payload with no schema-property overlap must not be repaired: %+v", report)
	}
	if string(got) != string(raw) {
		t.Fatalf("payload must remain unchanged: got %s want %s", got, raw)
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

func TestNormalize_StringArraySplitRequiresExplicitEnable(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"keywords":{"type":"array","items":{"type":"string"}}}}`)
	raw := json.RawMessage(`{"keywords":"agent,count"}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if report.Changed() {
		t.Fatalf("split_string_arrays must be explicit for delimited string repair: %+v", report)
	}
	if string(got) != string(raw) {
		t.Fatalf("payload must remain unchanged when split is not enabled: got %s want %s", got, raw)
	}

	got, report = Normalize(raw, schema, splitArrayRepairPolicy())
	if !report.Changed() {
		t.Fatal("split_string_arrays=true should repair delimited string arrays")
	}
	var decoded struct {
		Keywords []string `json:"keywords"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("normalized payload must decode: %v\n%s", err, got)
	}
	if strings.Join(decoded.Keywords, "|") != "agent|count" {
		t.Fatalf("keywords not split: %+v", decoded.Keywords)
	}
}

func hasRepair(report Report, path, rule string) bool {
	for _, repair := range report.Repairs {
		if repair.Path == path && repair.Rule == rule {
			return true
		}
	}
	return false
}

func envelopeCompatTestSchema() json.RawMessage {
	return json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "intent":{"type":"string"},
	    "keywords":{"type":"array","items":{"type":"string"}},
	    "limit":{"type":"integer"}
	  }
	}`)
}
