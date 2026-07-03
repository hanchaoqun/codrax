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
		types.SourceInventoryMechanicalFactBoundary,
		"reserve attribute_roles for narrowed scopes",
		"top-level architecture",
		"semantic_subgraph",
		"edit_impact",
		"call_path",
		"context or a tool refusal lists multiple active sub-repos",
		"active sub-repo",
		"relative to that selected sub-repo",
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
		!strings.Contains(schema.Properties["sources"].Description, "concrete source") ||
		!strings.Contains(schema.Properties["sources"].Description, "relative to that sub-repo") {
		t.Fatalf("relation_map parameter teaching incomplete: relation_kinds=%+v sources_desc=%q",
			schema.Properties["relation_kinds"].Items.Enum,
			schema.Properties["sources"].Description)
	}
	if !strings.Contains(schema.Properties["path"].Description, "active sub-repo") ||
		!strings.Contains(schema.Properties["target_file"].Description, "do not repeat the sub-repo prefix") ||
		!strings.Contains(schema.Properties["entry_point"].Description, "do not repeat the sub-repo prefix") {
		t.Fatalf("multi-repo path teaching incomplete: path=%q target_file=%q entry_point=%q",
			schema.Properties["path"].Description,
			schema.Properties["target_file"].Description,
			schema.Properties["entry_point"].Description)
	}
	if !strings.Contains(schema.Properties["include_attributes"].Description, "Use false for broad member/count passes") ||
		!strings.Contains(schema.Properties["attribute_roles"].Description, "Use only for bounded scopes") {
		t.Fatalf("source_inventory efficiency teaching incomplete: include_attributes=%q attribute_roles=%q",
			schema.Properties["include_attributes"].Description,
			schema.Properties["attribute_roles"].Description)
	}
	if !strings.Contains(schema.Properties["query"].Description, "exact code surfaces") ||
		!strings.Contains(schema.Properties["query"].Description, "do not paste a natural-language sentence") {
		t.Fatalf("query parameter should teach exact code-surface search, got %q", schema.Properties["query"].Description)
	}
}

func TestRepoMapLensParamsStripSelectedSubRepoPrefix(t *testing.T) {
	params := repoMapParams{
		TargetFile: "repo-a/internal/app.go",
		EntryPoint: "./repo-a/cmd/main.go",
		Scope:      "repo-a/internal",
		Scopes:     []string{"repo-a/pkg", "other/pkg", "repo-a"},
		Sources:    []string{"repo-a/internal/app.go", "Service", "repo-a"},
	}

	advisories := normalizeRepoMapLensParamsForSelectedSubRepo(&params, "repo-a")

	if params.TargetFile != "internal/app.go" ||
		params.EntryPoint != "cmd/main.go" ||
		params.Scope != "internal" {
		t.Fatalf("single-value prefix strip failed: %+v", params)
	}
	if got := strings.Join(params.Scopes, "|"); got != "pkg|other/pkg|." {
		t.Fatalf("scopes prefix strip failed: %q", got)
	}
	if got := strings.Join(params.Sources, "|"); got != "internal/app.go|Service|." {
		t.Fatalf("sources prefix strip failed: %q", got)
	}
	advisory := strings.Join(advisories, "\n")
	for _, want := range []string{
		"target_file: `repo-a/internal/app.go` → `internal/app.go`",
		"entry_point: `./repo-a/cmd/main.go` → `cmd/main.go`",
		"sources[0]: `repo-a/internal/app.go` → `internal/app.go`",
	} {
		if !strings.Contains(advisory, want) {
			t.Fatalf("normalization advisory missing %q:\n%s", want, advisory)
		}
	}
	if strings.Contains(advisory, "other/pkg") || strings.Contains(advisory, "Service") {
		t.Fatalf("advisory should not mention untouched parameters:\n%s", advisory)
	}
}

func TestRepoMapLensParamsNoAdvisoryWhenAlreadyRelative(t *testing.T) {
	params := repoMapParams{
		TargetFile: "internal/app.go",
		EntryPoint: "cmd/main.go",
		Scope:      "internal",
		Scopes:     []string{"pkg"},
		Sources:    []string{"internal/app.go", "Service"},
	}

	advisories := normalizeRepoMapLensParamsForSelectedSubRepo(&params, "repo-a")

	if len(advisories) != 0 {
		t.Fatalf("already-relative params should not produce correction advisory: %+v", advisories)
	}
	if params.TargetFile != "internal/app.go" ||
		params.EntryPoint != "cmd/main.go" ||
		params.Scope != "internal" ||
		strings.Join(params.Scopes, "|") != "pkg" ||
		strings.Join(params.Sources, "|") != "internal/app.go|Service" {
		t.Fatalf("already-relative params should remain unchanged: %+v", params)
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

// TestSourceInventoryRoleEnumSingleSourced pins that every surface listing
// the source_inventory lens roles quotes types.SourceInventoryLensRoleNames
// — the roles/attribute_roles schema enums (membership AND order, because
// the toolparam style-alias layer canonicalizes against these exact bytes)
// and the roles=["file"] refusal prose (every semantic role, i.e. all but
// "file", must be named so the model is steered with the real accepted set).
func TestSourceInventoryRoleEnumSingleSourced(t *testing.T) {
	want := types.SourceInventoryLensRoleNames()
	var schema struct {
		Properties map[string]struct {
			Items struct {
				Enum []string `json:"enum"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&RepoMapV2{}).Parameters(), &schema); err != nil {
		t.Fatalf("Parameters() is not valid JSON: %v", err)
	}
	for _, param := range []string{"roles", "attribute_roles"} {
		got := schema.Properties[param].Items.Enum
		if len(got) != len(want) {
			t.Fatalf("%s enum has %d entries, want %d: %v", param, len(got), len(want), got)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s enum[%d] = %q, want %q (order is load-bearing)", param, i, got[i], want[i])
			}
		}
	}
	refusal, ok := repoMapSourceInventoryParamPreflight(repoMapParams{
		View:  "source_inventory",
		Roles: []types.AnswerCandidateRole{types.AnswerCandidateRoleFile},
	})
	if ok || refusal == "" {
		t.Fatalf("roles=[file] must refuse; got ok=%v refusal=%q", ok, refusal)
	}
	for _, role := range want {
		if role == string(types.AnswerCandidateRoleFile) {
			continue
		}
		if !strings.Contains(refusal, role) {
			t.Fatalf("refusal prose missing semantic role %q:\n%s", role, refusal)
		}
	}
}
