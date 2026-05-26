package repomap

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/toolparam"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRepoMapSchemaTeachesLensParameters(t *testing.T) {
	tl := &RepoMapV2{}
	desc := tl.Description()
	for _, want := range []string{
		"source_inventory",
		"relation_map",
		"sources",
		"scopes",
		"relation_kinds",
		"verified navigation",
		"not a semantic source citation",
	} {
		if !strings.Contains(desc, want) {
			t.Fatalf("repo_map description missing %q:\n%s", want, desc)
		}
	}

	var schema struct {
		Properties map[string]struct {
			Description string   `json:"description"`
			Enum        []string `json:"enum"`
			Items       struct {
				Enum []string `json:"enum"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(tl.Parameters(), &schema); err != nil {
		t.Fatalf("repo_map parameters schema is invalid JSON: %v", err)
	}
	for _, field := range []string{"view", "scope", "scopes", "sources", "relation_kinds", "roles", "attribute_roles"} {
		if _, ok := schema.Properties[field]; !ok {
			t.Fatalf("repo_map parameters missing field %q", field)
		}
	}
	for _, internalOnly := range []string{"show_source_inventory_hint", "ShowSourceInventoryHint", "source_inventory_hint"} {
		if _, ok := schema.Properties[internalOnly]; ok || strings.Contains(string(tl.Parameters()), internalOnly) {
			t.Fatalf("repo_map parameters leaked internal render switch %q:\n%s", internalOnly, tl.Parameters())
		}
	}
	if !containsString(schema.Properties["view"].Enum, "source_inventory") ||
		!containsString(schema.Properties["view"].Enum, "relation_map") {
		t.Fatalf("repo_map view enum missing lens views: %+v", schema.Properties["view"].Enum)
	}
	if !containsString(schema.Properties["relation_kinds"].Items.Enum, "type_usage") ||
		!strings.Contains(schema.Properties["sources"].Description, "concrete source") {
		t.Fatalf("relation_map parameter teaching incomplete: relation_kinds=%+v sources_desc=%q",
			schema.Properties["relation_kinds"].Items.Enum,
			schema.Properties["sources"].Description)
	}
}

func TestRepoMapToolDescriptionCoversLensViews(t *testing.T) {
	if got := ToolDescription("source_inventory", ""); !strings.Contains(got, "source inventory") {
		t.Fatalf("source_inventory status description = %q", got)
	}
	if got := ToolDescription("relation_map", ""); !strings.Contains(got, "relation map") {
		t.Fatalf("relation_map status description = %q", got)
	}
}

func TestRepoMapSchemaAwareParamCompatRepairsLensParameterShapes(t *testing.T) {
	tl := &RepoMapV2{}
	raw := json.RawMessage(`{
		"path": ".",
		"view": "relation_map",
		"sources": "Explorer",
		"scopes": "internal/agent",
		"relationKinds": "call",
		"topN": "3",
		"includeCounts": "true"
	}`)

	got, report := toolparam.Normalize(raw, tl.Parameters(), types.DefaultToolParamCompatConfig())
	if !report.Changed() {
		t.Fatal("expected schema-aware repo_map lens parameter repairs")
	}
	var decoded repoMapParams
	if err := json.Unmarshal(got, &decoded); err != nil {
		t.Fatalf("normalized repo_map params must decode: %v\n%s", err, got)
	}
	if decoded.Path != "." || decoded.View != "relation_map" || decoded.TopN != 3 ||
		decoded.IncludeCounts == nil || !*decoded.IncludeCounts {
		t.Fatalf("scalar/key repairs failed: %+v raw=%s", decoded, got)
	}
	if strings.Join(decoded.Sources, "|") != "Explorer" ||
		strings.Join(decoded.Scopes, "|") != "internal/agent" ||
		strings.Join(decoded.RelationKinds, "|") != "call" {
		t.Fatalf("array lens params not repaired losslessly: %+v raw=%s", decoded, got)
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
