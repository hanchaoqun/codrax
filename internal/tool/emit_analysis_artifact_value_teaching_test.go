package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
)

func TestEmitAnalysisArtifactValueTeachingMatchesSchema(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&EmitAnalysis{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	description := schema.Properties["artifact_value_profile"].Description
	prompt := skill.BuildAnalysisSkill().OutputFormat
	_, paragraph, found := strings.Cut(prompt, "artifact_value_profile is OPTIONAL. ")
	if !found {
		t.Fatal("artifact profile teaching is missing")
	}
	paragraph, _, _ = strings.Cut(paragraph, "\n\n")
	for _, fragment := range []string{
		"scalar answer explicitly requested",
		"only with predicates.is_scalar_answer=true",
		"soft lookup lane that later stages must verify",
		"never transcribe a pre-triage model summary, stall guess, or inferred root-cause value",
	} {
		if !strings.Contains(description, fragment) || !strings.Contains(paragraph, fragment) {
			t.Errorf("both schema and local artifact paragraph must teach %q", fragment)
		}
	}
	if description == "" || !strings.Contains(paragraph, description) {
		t.Error("artifact profile schema and teaching must share one applicability/provenance contract")
	}
	for _, field := range []string{"`target`", "`value`", "`unit`", "`literal_kind`", "`artifact_refs[]`", "`observation_refs[]`", "`field_value_profile.source_quote`"} {
		if !strings.Contains(paragraph, field) {
			t.Errorf("artifact value field guidance lost %s", field)
		}
	}
}
