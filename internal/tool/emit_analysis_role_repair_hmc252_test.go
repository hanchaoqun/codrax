package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC252DimensionRoleTeachingHasOneSchemaOwner(t *testing.T) {
	var schema map[string]any
	if err := json.Unmarshal((&EmitAnalysis{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	dimensions := schema["properties"].(map[string]any)["requested_answer_dimensions"].(map[string]any)
	fields := dimensions["properties"].(map[string]any)["dimensions"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	role := fields["role"].(map[string]any)["description"].(string)
	encoded, err := json.Marshal(dimensions)
	if err != nil {
		t.Fatal(err)
	}
	// Count decoded descriptions, not escaped JSON strings.
	var descriptions strings.Builder
	var visit func(any)
	visit = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			for k, child := range v {
				if k == "description" {
					descriptions.WriteString(child.(string))
				} else {
					visit(child)
				}
			}
		case []any:
			for _, child := range v {
				visit(child)
			}
		}
	}
	visit(dimensions)
	if strings.Count(descriptions.String(), skill.AnalysisMultiSurfaceDimensionTeaching) != 1 || !strings.Contains(role, skill.AnalysisMultiSurfaceDimensionTeaching) {
		t.Errorf("role semantics need one schema owner, not repeated parent/array descriptions: %s", encoded)
	}
	for _, surface := range []string{role, skill.BuildAnalysisSkill().OutputFormat} {
		for _, want := range []string{"information requested", "output carrier", "delivery action", "not a visual surface", "provenance"} {
			if !strings.Contains(surface, want) {
				t.Errorf("dimension axes are ambiguous: missing %q", want)
			}
		}
	}
}

func TestHMC252RejectedDimensionCanBeReclassifiedWithoutInventingDiagram(t *testing.T) {
	const question = "检查采集记录的调用位置，然后导出审计包。"
	bus := &types.BusContext{Mutable: types.NewMutableState(question)}
	payload := func(role string) json.RawMessage {
		return json.RawMessage(withV4Required(`{
			"intent":"explain","scenario":"generic","complexity":"moderate",
			"keywords":["采集","记录"],"entities":["采集记录"],
			"question_kind":"mechanism","predicate_axis":"",
			"requested_answer_dimensions":{"is_dimensioned_answer":true,"confidence":0.9,"dimensions":[
				{"label":"调用位置","role":"observed_value","source_quote":"调用位置","required":true,"index":1},
				{"label":"审计包","role":"` + role + `","source_quote":"导出审计包","required":true,"index":2}
			]}
		}`))
	}
	bad, err := (&EmitAnalysis{}).Execute(bus, payload("diagram"))
	if err != nil || bad.Success || !strings.Contains(bad.Summary, "missing required top-level field(s): diagram_hint") {
		t.Fatalf("typed diagram without carrier must still reject: %+v, %v", bad, err)
	}
	if bus.Mutable.RequestModel() != nil {
		t.Fatal("rejected classification must not publish an accepted request")
	}
	for _, want := range []string{"Your submitted", "reclassify", "current request", "presentation authority and does not prove any edge"} {
		if !strings.Contains(bad.Summary, want) {
			t.Errorf("repair must permit model-owned semantic correction: missing %q in %s", want, bad.Summary)
		}
	}
	if strings.Contains(bad.Summary, "accepted current-turn diagram dimension remains required") {
		t.Error("a rejected role must not be described as a permanently accepted obligation")
	}
	good, err := (&EmitAnalysis{}).Execute(bus, payload("other"))
	if err != nil || !good.Success {
		t.Fatalf("complete model-owned correction must remain possible: %+v, %v", good, err)
	}
	got := bus.Mutable.RequestModel()
	if got == nil || got.DiagramHint != nil || got.RequestedAnswerDimensions == nil {
		t.Fatalf("delivery correction minted or lost a surface: %+v", got)
	}
	if d := got.RequestedAnswerDimensions.Dimensions; len(d) != 2 || d[0].Role != types.RequestedAnswerDimensionObservedValue || d[1].Role != types.RequestedAnswerDimensionOther {
		t.Fatalf("model-owned roles were rewritten: %+v", d)
	}
}

func TestHMC252RealDiagramRoleRetainsExplicitCarrierRequirement(t *testing.T) {
	question := "检查采集记录的调用位置，然后用时序图展示唤醒关系，并导出审计包。"
	bus := &types.BusContext{Mutable: types.NewMutableState(question), PresentationDiagramRequired: true}
	payload := json.RawMessage(withV4Required(`{
		"intent":"explain","scenario":"generic","complexity":"moderate",
		"keywords":["采集","唤醒"],"entities":["采集记录"],
		"question_kind":"mechanism","predicate_axis":"flow",
		"requested_answer_dimensions":{"is_dimensioned_answer":true,"confidence":0.9,"dimensions":[
			{"label":"时序图","role":"diagram","source_quote":"用时序图展示唤醒关系","required":true,"index":1},
			{"label":"审计包","role":"other","source_quote":"导出审计包","required":true,"index":2}
		]}
	}`))
	result, err := (&EmitAnalysis{}).Execute(bus, payload)
	if err != nil || result.Success || !strings.Contains(result.Summary, "diagram_hint") || bus.Mutable.RequestModel() != nil {
		t.Fatalf("delivery sibling must not waive the actual visual: %+v %v", result, err)
	}
	var corrected map[string]json.RawMessage
	if err := json.Unmarshal(payload, &corrected); err != nil {
		t.Fatal(err)
	}
	corrected["diagram_hint"] = json.RawMessage(`{"kind":"sequence","required":true,"relation_scope_quote":"用时序图展示唤醒关系","participants":[]}`)
	full, err := json.Marshal(corrected)
	if err != nil {
		t.Fatal(err)
	}
	result, err = (&EmitAnalysis{}).Execute(bus, full)
	if err != nil || !result.Success || bus.Mutable.RequestModel() == nil || bus.Mutable.RequestModel().DiagramHint == nil || !bus.Mutable.RequestModel().DiagramHint.Required {
		t.Fatalf("explicit visual must remain available after complete repair: %+v %v", result, err)
	}
}
