package repomap

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

// repoMapSchemaProp is the subset of JSON-schema fields the strict
// decode tests need to generate type-correct sample values.
type repoMapSchemaProp struct {
	Type  string          `json:"type"`
	Enum  []string        `json:"enum"`
	Items json.RawMessage `json:"items"`
}

func decodeRepoMapSchemaProps(t *testing.T) map[string]repoMapSchemaProp {
	t.Helper()
	var schema struct {
		Properties map[string]repoMapSchemaProp `json:"properties"`
	}
	if err := json.Unmarshal((&RepoMapV2{}).Parameters(), &schema); err != nil {
		t.Fatalf("repo_map parameters schema is invalid JSON: %v", err)
	}
	if len(schema.Properties) == 0 {
		t.Fatal("repo_map parameters schema has no properties")
	}
	return schema.Properties
}

func repoMapSampleSchemaValue(t *testing.T, prop repoMapSchemaProp) any {
	t.Helper()
	switch prop.Type {
	case "string":
		if len(prop.Enum) > 0 {
			return prop.Enum[0]
		}
		return "sample"
	case "integer":
		return 1
	case "number":
		return 1.5
	case "boolean":
		return true
	case "array":
		var item repoMapSchemaProp
		if len(prop.Items) > 0 {
			if err := json.Unmarshal(prop.Items, &item); err != nil {
				t.Fatalf("array items schema is invalid JSON: %v", err)
			}
		}
		return []any{repoMapSampleSchemaValue(t, item)}
	default:
		t.Fatalf("unhandled schema property type %q — extend repoMapSampleSchemaValue", prop.Type)
		return nil
	}
}

// Every documented schema property must survive the strict decode:
// onboarding DisallowUnknownFields must never reject a parameter the
// schema teaches the model to use.
func TestRepoMapStrictDecodeAcceptsEveryDocumentedParam(t *testing.T) {
	props := decodeRepoMapSchemaProps(t)
	payload := map[string]any{}
	for name, prop := range props {
		payload[name] = repoMapSampleSchemaValue(t, prop)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal sample payload: %v", err)
	}
	if _, err := decodeRepoMapParams(raw); err != nil {
		t.Fatalf("strict decode rejected documented params: %v\npayload: %s", err, raw)
	}

	// And the reverse direction: every params-struct field must be
	// documented in the schema, so the decode surface and the
	// model-facing schema cannot drift apart.
	rt := reflect.TypeOf(repoMapParams{})
	for i := 0; i < rt.NumField(); i++ {
		tag := strings.Split(rt.Field(i).Tag.Get("json"), ",")[0]
		if tag == "" || tag == "-" {
			t.Fatalf("repoMapParams field %s has no json tag", rt.Field(i).Name)
		}
		if _, ok := props[tag]; !ok {
			t.Errorf("repoMapParams field %q is not documented in the parameters schema", tag)
		}
	}
}

func TestRepoMapSupportedViewsMatchSchemaEnum(t *testing.T) {
	props := decodeRepoMapSchemaProps(t)
	schemaEnum := props["view"].Enum
	if len(schemaEnum) == 0 {
		t.Fatal("view enum missing from parameters schema")
	}
	if !reflect.DeepEqual(schemaEnum, repoMapSupportedViews) {
		t.Fatalf("view enum drift: schema=%v gate=%v", schemaEnum, repoMapSupportedViews)
	}
}

// Every x-codrax-enum-aliases target for view must be a member of the
// view enum, so the deterministic alias table can never introduce a
// view name the gate would then reject.
func TestRepoMapViewAliasTargetsAreEnumMembers(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Enum    []string          `json:"enum"`
			Aliases map[string]string `json:"x-codrax-enum-aliases"`
			Style   bool              `json:"x-codrax-enum-style-alias"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&RepoMapV2{}).Parameters(), &schema); err != nil {
		t.Fatalf("repo_map parameters schema is invalid JSON: %v", err)
	}
	view := schema.Properties["view"]
	if !view.Style {
		t.Fatal("view must declare x-codrax-enum-style-alias")
	}
	if len(view.Aliases) == 0 {
		t.Fatal("view must declare a deterministic x-codrax-enum-aliases table")
	}
	for alias, target := range view.Aliases {
		if !containsString(view.Enum, target) {
			t.Errorf("view alias %q targets %q which is not in the view enum", alias, target)
		}
		if containsString(view.Enum, alias) {
			t.Errorf("view alias %q shadows a canonical enum value", alias)
		}
	}
}

func TestRepoMapUnknownFieldRejectedWithTypedRepair(t *testing.T) {
	res, err := (&RepoMapV2{}).Execute(nil, json.RawMessage(`{"path":".","bogus_field":1}`))
	if err == nil {
		t.Fatal("strict decode of an unknown field must surface an error")
	}
	if res.Success {
		t.Fatalf("unknown field must fail the call, got success: %s", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != "tool_param_unknown_field" {
		t.Fatalf("unknown field must carry the typed repair code, got %+v", res.Repair)
	}
	if !strings.Contains(res.Summary, "bogus_field") {
		t.Fatalf("rejection must name the offending field: %s", res.Summary)
	}
}

func TestRepoMapUnknownViewTypedRejectionNamesEnum(t *testing.T) {
	res, err := (&RepoMapV2{}).Execute(nil, json.RawMessage(`{"path":".","view":"nonsense_view"}`))
	if err != nil {
		t.Fatalf("unknown view is a typed refusal, not a transport error: %v", err)
	}
	if res.Success {
		t.Fatalf("unknown view must fail the call, got success: %s", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != "tool_param_invalid_enum_value" {
		t.Fatalf("unknown view must carry the typed repair code, got %+v", res.Repair)
	}
	if got := strings.Join(res.Repair.Fields, ","); got != "view" {
		t.Fatalf("repair fields must name view, got %q", got)
	}
	for _, view := range repoMapSupportedViews {
		if !strings.Contains(res.Summary, view) {
			t.Fatalf("rejection must name valid view %q:\n%s", view, res.Summary)
		}
	}
	if !strings.Contains(res.Summary, `"nonsense_view"`) {
		t.Fatalf("rejection must echo the offending view: %s", res.Summary)
	}
	// The gate runs before any index work — a silent overview render
	// would carry the index banner marker.
	if strings.Contains(res.Summary, "Repo Map Index") {
		t.Fatalf("unknown view must not fall back to a rendered view:\n%s", res.Summary)
	}
}

// The deterministic alias table and style normalization must
// canonicalize common view-name drift BEFORE the gate, via the same
// schema-driven layer the agent runtime uses.
func TestRepoMapViewAliasesNormalizeDeterministically(t *testing.T) {
	tl := &RepoMapV2{}
	cases := map[string]string{
		"RelationMap":      "relation_map",
		"relation-map":     "relation_map",
		"relationmap":      "relation_map",
		"Relation Map":     "relation_map",
		"sourceinventory":  "source_inventory",
		"source-inventory": "source_inventory",
		"FileMap":          "file_map",
		"taskmap":          "task_map",
		"callpath":         "call_path",
		"editimpact":       "edit_impact",
		"semanticsubgraph": "semantic_subgraph",
		"implementors":     "implementers",
		" overview ":       "overview",
	}
	for given, want := range cases {
		raw, _ := json.Marshal(map[string]any{"path": ".", "view": given})
		got, _ := toolparam.Normalize(raw, tl.Parameters(), types.DefaultToolParamCompatConfig())
		p, err := decodeRepoMapParams(got)
		if err != nil {
			t.Fatalf("view %q: normalized params must strict-decode: %v", given, err)
		}
		if p.View != want {
			t.Errorf("view %q normalized to %q, want %q", given, p.View, want)
		}
	}
}

// In-tool compat must run before the gate even when the agent-layer
// normalization was bypassed (direct Execute callers): an aliased view
// with a missing path fails on the PATH preflight, not on the view.
func TestRepoMapExecuteNormalizesViewAliasBeforeGate(t *testing.T) {
	res, err := (&RepoMapV2{}).Execute(nil, json.RawMessage(`{"path":"definitely/missing/dir","view":"RelationMap"}`))
	if err != nil {
		t.Fatalf("aliased view must not error at decode: %v", err)
	}
	if res.Success {
		t.Fatalf("missing path must fail, got success: %s", res.Summary)
	}
	if res.Repair != nil && res.Repair.Code == "tool_param_invalid_enum_value" {
		t.Fatalf("aliased view must be normalized before the enum gate: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "was not found") {
		t.Fatalf("expected the path preflight refusal, got: %s", res.Summary)
	}
}

// String-carried arrays split deterministically via the schema
// annotation, keeping comma-joined model output decodable.
func TestRepoMapSplitStringArrayAnnotationsRepairLensParams(t *testing.T) {
	tl := &RepoMapV2{}
	raw := json.RawMessage(`{
		"path": ".",
		"view": "relation_map",
		"scopes": "internal/agent, internal/tool",
		"sources": "Explorer; Finalizer",
		"relation_kinds": "call, imports",
		"roles": "function, method"
	}`)
	got, report := toolparam.Normalize(raw, tl.Parameters(), types.DefaultToolParamCompatConfig())
	if !report.Changed() {
		t.Fatal("expected schema-driven split-string-array repairs")
	}
	p, err := decodeRepoMapParams(got)
	if err != nil {
		t.Fatalf("normalized params must strict-decode: %v\n%s", err, got)
	}
	if strings.Join(p.Scopes, "|") != "internal/agent|internal/tool" {
		t.Fatalf("scopes split failed: %+v", p.Scopes)
	}
	if strings.Join(p.Sources, "|") != "Explorer|Finalizer" {
		t.Fatalf("sources split failed: %+v", p.Sources)
	}
	if strings.Join(p.RelationKinds, "|") != "call|imports" {
		t.Fatalf("relation_kinds split failed: %+v", p.RelationKinds)
	}
	if len(p.Roles) != 2 || string(p.Roles[0]) != "function" || string(p.Roles[1]) != "method" {
		t.Fatalf("roles split failed: %+v", p.Roles)
	}
}
