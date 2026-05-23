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

func TestNormalize_SingletonObjectArrayAgainstSchema(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "items":{
	      "type":"array",
	      "items":{
	        "type":"object",
	        "properties":{
	          "label":{"type":"string"},
	          "citation_ref":{"type":"integer"}
	        }
	      }
	    }
	  }
	}`)
	raw := json.RawMessage(`{"items":{"label":"Eval","citation_ref":"3"}}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if !hasRepair(report, "$.items", "singleton_object_array") {
		t.Fatalf("expected singleton-object array repair, got %+v", report)
	}
	if !hasRepair(report, "$.items[0].citation_ref", "string_integer") {
		t.Fatalf("expected nested scalar repair after wrapping, got %+v", report)
	}
	var decoded struct {
		Items []struct {
			Label       string `json:"label"`
			CitationRef int    `json:"citation_ref"`
		} `json:"items"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("normalized payload must decode: %v\n%s", err, got)
	}
	if len(decoded.Items) != 1 || decoded.Items[0].Label != "Eval" || decoded.Items[0].CitationRef != 3 {
		t.Fatalf("unexpected normalized items: %+v", decoded.Items)
	}
}

func TestNormalize_DoesNotWrapObjectForStringArray(t *testing.T) {
	schema := json.RawMessage(`{"type":"object","properties":{"keywords":{"type":"array","items":{"type":"string"}}}}`)
	raw := json.RawMessage(`{"keywords":{"value":"agent"}}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if report.Changed() {
		t.Fatalf("object must not be wrapped for array-of-string fields: %+v", report)
	}
	if string(got) != string(raw) {
		t.Fatalf("payload must remain unchanged: got %s want %s", got, raw)
	}
}

func TestNormalize_StringScalarsWithStructuralNumericPrefix(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "count":{"type":"integer"},
	    "threshold":{"type":"number"}
	  }
	}`)
	raw := json.RawMessage(`{"count":": 7","threshold":": 0.95"}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if !report.Changed() {
		t.Fatal("expected scalar repairs")
	}
	var decoded struct {
		Count     int     `json:"count"`
		Threshold float64 `json:"threshold"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("normalized payload must decode: %v\n%s", err, got)
	}
	if decoded.Count != 7 || decoded.Threshold != 0.95 {
		t.Fatalf("unexpected normalized values: %+v", decoded)
	}
}

func TestNormalize_StringEnumJSONLiteralArtifacts(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "intent":{"type":"string","enum":["explain","root_cause"]},
	    "predicate_axis":{"type":"string","enum":["call","register",""]},
	    "free_text":{"type":"string"}
	  }
	}`)
	raw := json.RawMessage(`{
	  "intent":"\"explain\"",
	  "predicate_axis":"\"\"",
	  "free_text":"\"keep the user's literal quotes\""
	}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if !hasRepair(report, "$.intent", "json_string_enum") {
		t.Fatalf("expected intent enum-string repair, got %+v", report)
	}
	if !hasRepair(report, "$.predicate_axis", "json_string_enum") {
		t.Fatalf("expected predicate_axis enum-string repair, got %+v", report)
	}
	var decoded struct {
		Intent        string `json:"intent"`
		PredicateAxis string `json:"predicate_axis"`
		FreeText      string `json:"free_text"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("normalized payload must decode: %v\n%s", err, got)
	}
	if decoded.Intent != "explain" {
		t.Fatalf("intent = %q, want explain; raw=%s", decoded.Intent, got)
	}
	if decoded.PredicateAxis != "" {
		t.Fatalf("predicate_axis = %q, want empty enum sentinel; raw=%s", decoded.PredicateAxis, got)
	}
	if decoded.FreeText != `"keep the user's literal quotes"` {
		t.Fatalf("free_text should not be unwrapped without enum schema, got %q", decoded.FreeText)
	}
}

func TestNormalize_StringEnumJSONLiteralKeepsInvalidEnumForToolGate(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "predicate_axis":{"type":"string","enum":["call","register",""]}
	  }
	}`)
	raw := json.RawMessage(`{"predicate_axis":"\"ponder\""}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if report.Changed() {
		t.Fatalf("invalid enum must remain for the tool's canonical gate, got report %+v", report)
	}
	if string(got) != string(raw) {
		t.Fatalf("invalid enum payload changed: got %s want %s", got, raw)
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

func TestNormalize_RepairsTrailingCommasBeforeJSONClosers(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "items":{
	      "type":"array",
	      "items":{
	        "type":"object",
	        "properties":{
	          "name":{"type":"string"},
	          "line":{"type":"integer"}
	        }
	      }
	    }
	  }
	}`)
	raw := json.RawMessage(`{"items":[{"name":"SubExplorer","line":"32",},],}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if !hasRepair(report, "$", "json_trailing_comma") {
		t.Fatalf("expected trailing-comma repair, got %+v", report)
	}
	if !hasRepair(report, "$.items[0].line", "string_integer") {
		t.Fatalf("expected nested scalar repair after syntax repair, got %+v", report)
	}
	var decoded struct {
		Items []struct {
			Name string `json:"name"`
			Line int    `json:"line"`
		} `json:"items"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("normalized payload must decode: %v\n%s", err, got)
	}
	if len(decoded.Items) != 1 || decoded.Items[0].Name != "SubExplorer" || decoded.Items[0].Line != 32 {
		t.Fatalf("unexpected normalized payload: %+v", decoded)
	}
}

func TestJSONRepairCandidatesRepairsMissingTrailingClosers(t *testing.T) {
	input := `{"blocks":[{"id":"summary","text":"draft"}`

	var decoded struct {
		Blocks []struct {
			ID string `json:"id"`
		} `json:"blocks"`
	}
	var parsed bool
	for _, candidate := range JSONRepairCandidates(input) {
		if err := json.Unmarshal([]byte(candidate), &decoded); err == nil {
			parsed = true
			break
		}
	}
	if !parsed {
		t.Fatalf("expected a parseable repaired candidate for %q", input)
	}
	if len(decoded.Blocks) != 1 || decoded.Blocks[0].ID != "summary" {
		t.Fatalf("unexpected decoded value after repair: %+v", decoded)
	}
}

func TestJSONRepairCandidatesRepairsParentCloseBeforeChild(t *testing.T) {
	input := `[{"id":"diagram","diagram":{"body":"flowchart TD\n    A --> B"}]`

	var decoded []struct {
		ID      string `json:"id"`
		Diagram struct {
			Body string `json:"body"`
		} `json:"diagram"`
	}
	var parsed bool
	for _, candidate := range JSONRepairCandidates(input) {
		if err := json.Unmarshal([]byte(candidate), &decoded); err == nil {
			parsed = true
			break
		}
	}
	if !parsed {
		t.Fatalf("expected a parseable repaired candidate for %q", input)
	}
	if len(decoded) != 1 || decoded[0].ID != "diagram" || decoded[0].Diagram.Body == "" {
		t.Fatalf("unexpected decoded value after repair: %+v", decoded)
	}
}

func TestRepairMissingTrailingJSONClosersRejectsUnterminatedStrings(t *testing.T) {
	if repaired, ok := RepairMissingTrailingJSONClosers(`{"text":"unterminated}`); ok {
		t.Fatalf("unterminated string must not be repaired, got %q", repaired)
	}
}

func TestNormalize_StringWrappedArrayWithTrailingCommas(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "items":{
	      "type":"array",
	      "items":{
	        "type":"object",
	        "properties":{"name":{"type":"string"}}
	      }
	    }
	  }
	}`)
	raw := json.RawMessage(`{"items":"[{\"name\":\"SubExplorer\",},]"}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if !hasRepair(report, "$.items", "json_string_array_trailing_comma") {
		t.Fatalf("expected trailing-comma string array repair, got %+v", report)
	}
	var decoded struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("normalized payload must decode: %v\n%s", err, got)
	}
	if len(decoded.Items) != 1 || decoded.Items[0].Name != "SubExplorer" {
		t.Fatalf("unexpected normalized payload: %+v", decoded)
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

func TestNormalize_RepairsSchemaPropertyKeyArtifactsBeforeValueNormalization(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "items":{
	      "type":"array",
	      "items":{
	        "type":"object",
	        "properties":{
	          "line_start":{"type":"integer"},
	          "anchor_symbol":{"type":"string"},
	          "summary":{"type":"string"}
	        }
	      }
	    }
	  }
	}`)
	raw := json.RawMessage(`{
	  "items\"":"[{\"lineStart\":\"969\",\"anchorSymbol\":\"Execute\",\"summary\":\"BaseAgent.Execute\"}]"
	}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if !hasRepair(report, `$.items"`, "property_key_quote_artifact") {
		t.Fatalf("expected top-level key quote repair, got %+v", report)
	}
	if !hasRepair(report, "$.items", "json_string_array") {
		t.Fatalf("expected repaired key to unlock array-value repair, got %+v", report)
	}
	if !hasRepair(report, "$.items[0].anchorSymbol", "property_key_case_style") {
		t.Fatalf("expected nested camelCase key repair, got %+v", report)
	}
	if !hasRepair(report, "$.items[0].line_start", "string_integer") {
		t.Fatalf("expected nested scalar repair after key repair, got %+v", report)
	}
	var decoded struct {
		Items []struct {
			LineStart    int    `json:"line_start"`
			AnchorSymbol string `json:"anchor_symbol"`
			Summary      string `json:"summary"`
		} `json:"items"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("normalized payload must decode: %v\n%s", err, got)
	}
	if len(decoded.Items) != 1 || decoded.Items[0].LineStart != 969 || decoded.Items[0].AnchorSymbol != "Execute" {
		t.Fatalf("unexpected normalized payload: %+v\n%s", decoded, got)
	}
}

func TestNormalize_RepairsGeneralSchemaPropertyKeyVariants(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "replace_blocks":{
	      "type":"array",
	      "items":{
	        "type":"object",
	        "properties":{
	          "claim_uses":{
	            "type":"array",
	            "items":{
	              "type":"object",
	              "properties":{
	                "citation_ref":{"type":"integer"}
	              }
	            }
	          },
	          "facet_ids":{"type":"array","items":{"type":"string"}},
	          "edge_anchors":{"type":"array","items":{"type":"object"}}
	        }
	      }
	    },
	    "unchanged_block_ids":{"type":"array","items":{"type":"string"}}
	  }
	}`)
	raw := json.RawMessage(`{
	  "replaceBlocks":[{
	    "claimUses":[{"citationRef":"7"}],
	    "facet ids":["diagram_spine"],
	    "edgeAnchors":[{"from_node":"A","to_node":"B"}]
	  }],
	  "unchangedBlockID":["summary"]
	}`)

	got, report := Normalize(raw, schema, repairPolicy)
	for _, want := range []struct {
		path string
		rule string
	}{
		{`$.replaceBlocks`, "property_key_case_style"},
		{`$.replace_blocks[0].claimUses`, "property_key_case_style"},
		{`$.replace_blocks[0].facet ids`, "property_key_case_style"},
		{`$.replace_blocks[0].edgeAnchors`, "property_key_case_style"},
		{`$.unchangedBlockID`, "property_key_id_plural"},
		{`$.replace_blocks[0].claim_uses[0].citationRef`, "property_key_case_style"},
		{`$.replace_blocks[0].claim_uses[0].citation_ref`, "string_integer"},
	} {
		if !hasRepair(report, want.path, want.rule) {
			t.Fatalf("expected repair %s via %s, got %+v", want.path, want.rule, report)
		}
	}
	var decoded struct {
		ReplaceBlocks []struct {
			ClaimUses []struct {
				CitationRef int `json:"citation_ref"`
			} `json:"claim_uses"`
			FacetIDs    []string         `json:"facet_ids"`
			EdgeAnchors []map[string]any `json:"edge_anchors"`
		} `json:"replace_blocks"`
		UnchangedBlockIDs []string `json:"unchanged_block_ids"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("normalized payload must decode: %v\n%s", err, got)
	}
	if len(decoded.ReplaceBlocks) != 1 ||
		len(decoded.ReplaceBlocks[0].ClaimUses) != 1 ||
		decoded.ReplaceBlocks[0].ClaimUses[0].CitationRef != 7 ||
		len(decoded.ReplaceBlocks[0].FacetIDs) != 1 ||
		len(decoded.ReplaceBlocks[0].EdgeAnchors) != 1 ||
		len(decoded.UnchangedBlockIDs) != 1 {
		t.Fatalf("unexpected normalized payload: %+v\n%s", decoded, got)
	}
}

func TestNormalize_RepairsStringSuffixPropertyKeyVariant(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "path":{"type":"string"},
	    "offset":{"type":"integer"},
	    "limit":{"type":"integer"}
	  }
	}`)
	raw := json.RawMessage(`{"path":"internal/agent/analyzer.go","offset_str":"2310","limit_str":"50"}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if !hasRepair(report, "$.offset_str", "property_key_string_suffix") {
		t.Fatalf("expected offset_str key repair, got %+v", report)
	}
	if !hasRepair(report, "$.limit_str", "property_key_string_suffix") {
		t.Fatalf("expected limit_str key repair, got %+v", report)
	}
	if !hasRepair(report, "$.offset", "string_integer") {
		t.Fatalf("expected offset scalar repair after key repair, got %+v", report)
	}
	if !hasRepair(report, "$.limit", "string_integer") {
		t.Fatalf("expected limit scalar repair after key repair, got %+v", report)
	}
	var decoded struct {
		Offset int `json:"offset"`
		Limit  int `json:"limit"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("normalized payload must decode: %v\n%s", err, got)
	}
	if decoded.Offset != 2310 || decoded.Limit != 50 {
		t.Fatalf("offset/limit = %d/%d, want 2310/50; raw=%s", decoded.Offset, decoded.Limit, got)
	}
}

func TestNormalize_RepairsCompositePropertyKeyFragment(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "items":{
	      "type":"array",
	      "items":{
	        "type":"object",
	        "properties":{
	          "file":{"type":"string"},
	          "kind":{"type":"string"},
	          "line":{"type":"integer"},
	          "name":{"type":"string"}
	        }
	      }
	    }
	  }
	}`)
	raw := json.RawMessage(`{
	  "items":[{
	    "file":"internal/analysis/criterion/grammar.go",
	    "kind":"const",
	    "const\", \"line":37,
	    "name":"KindNoRelevantEvidence"
	  }]
	}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if !hasRepair(report, `$.items[0].const", "line`, "property_key_composite_fragment") {
		t.Fatalf("expected composite key fragment repair, got %+v", report)
	}
	var decoded struct {
		Items []struct {
			File string `json:"file"`
			Kind string `json:"kind"`
			Line int    `json:"line"`
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("normalized payload must decode: %v\n%s", err, got)
	}
	if len(decoded.Items) != 1 || decoded.Items[0].Line != 37 || decoded.Items[0].Kind != "const" {
		t.Fatalf("unexpected normalized payload: %+v\n%s", decoded, got)
	}
}

func TestNormalize_DoesNotRepairAmbiguousCompositePropertyKeyFragment(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "items":{
	      "type":"array",
	      "items":{
	        "type":"object",
	        "properties":{
	          "kind":{"type":"string"},
	          "line":{"type":"integer"}
	        }
	      }
	    }
	  }
	}`)
	raw := json.RawMessage(`{"items":[{"kind\", \"line":37}]}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if report.Changed() {
		t.Fatalf("ambiguous composite key must not be repaired: %+v", report)
	}
	if string(got) != string(raw) {
		t.Fatalf("ambiguous payload changed: got %s want %s", got, raw)
	}
}

func TestNormalize_DoesNotRepairAmbiguousSchemaPropertyKeyCollision(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "items":{"type":"array","items":{"type":"object"}}
	  }
	}`)
	raw := json.RawMessage(`{"items":[{"ok":true}],"items\"":"[{\"ok\":false}]"}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if report.Changed() {
		t.Fatalf("colliding canonical and malformed keys must not be repaired: %+v", report)
	}
	if string(got) != string(raw) {
		t.Fatalf("ambiguous payload must pass through unchanged: got %s want %s", got, raw)
	}
}

func TestNormalize_DoesNotRepairAmbiguousFuzzyPropertyKey(t *testing.T) {
	schema := json.RawMessage(`{
	  "type":"object",
	  "properties":{
	    "line_id":{"type":"string"},
	    "lineid":{"type":"string"}
	  }
	}`)
	raw := json.RawMessage(`{"line.id":"42"}`)

	got, report := Normalize(raw, schema, repairPolicy)
	if report.Changed() {
		t.Fatalf("fuzzy key that matches multiple schema fields must not be repaired: %+v", report)
	}
	if string(got) != string(raw) {
		t.Fatalf("ambiguous payload must pass through unchanged: got %s want %s", got, raw)
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
