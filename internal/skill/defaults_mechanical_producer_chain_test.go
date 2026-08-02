package skill

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestMechanicalProducerChainSeparationDirectiveSharedAndMechanismGated(t *testing.T) {
	registry := NewRegistry()
	RegisterDefaults(registry)

	for _, skillName := range []string{"explore-skill", "answer-document-skill"} {
		config, err := registry.Get(skillName)
		if err != nil {
			t.Fatalf("Get(%s): %v", skillName, err)
		}
		found := 0
		for _, item := range config.WorkflowTierB {
			if item.Body != mechanicalProducerChainSeparationDirective {
				continue
			}
			found++
			if !item.ShouldRender(AppliesToContext{HasMechanism: true, Intent: types.IntentRootCause}) {
				t.Fatalf("%s directive must render for typed mechanism questions", skillName)
			}
			if item.ShouldRender(AppliesToContext{Intent: types.IntentExplain}) {
				t.Fatalf("%s directive must not use broad explain intent as a mechanism substitute", skillName)
			}
		}
		if found != 1 {
			t.Fatalf("%s directive count=%d, want 1", skillName, found)
		}
	}

	for _, required := range []string{
		"each visible fragment to its own producer",
		"progress ordinal",
		"retry/loop policy",
		"direct call, assignment, parameter flow, or returned value",
		"Equal numbers",
		"never proof",
		"formatter/composer",
	} {
		if !strings.Contains(mechanicalProducerChainSeparationDirective, required) {
			t.Fatalf("directive missing %q: %s", required, mechanicalProducerChainSeparationDirective)
		}
	}
}

func TestIndependentMechanismContrastDirectiveSharedAndMechanismGated(t *testing.T) {
	registry := NewRegistry()
	RegisterDefaults(registry)

	for _, skillName := range []string{"explore-skill", "answer-document-skill"} {
		config, err := registry.Get(skillName)
		if err != nil {
			t.Fatalf("Get(%s): %v", skillName, err)
		}
		found := 0
		for _, item := range config.WorkflowTierB {
			if item.Body != independentMechanismContrastDirective {
				continue
			}
			found++
			if !item.ShouldRender(AppliesToContext{HasMechanism: true, Intent: types.IntentExplain}) {
				t.Fatalf("%s directive must render for typed mechanism comparisons", skillName)
			}
			if item.ShouldRender(AppliesToContext{Intent: types.IntentExplain}) {
				t.Fatalf("%s directive must not infer mechanism shape from broad explain intent", skillName)
			}
		}
		if found != 1 {
			t.Fatalf("%s directive count=%d, want 1", skillName, found)
		}
	}

	for _, required := range []string{
		"each side through its own producer and control path",
		"false branch or logical complement is not evidence",
		"explicit handoff, shared decision, or return-value flow",
		"every side",
		"state that evidence boundary",
	} {
		if !strings.Contains(independentMechanismContrastDirective, required) {
			t.Fatalf("directive missing %q: %s", required, independentMechanismContrastDirective)
		}
	}
}

func TestLogTriageComposedOutputInterpretationStaysAdvisory(t *testing.T) {
	registry := NewRegistry()
	RegisterDefaults(registry)
	config, err := registry.Get("log-triage-skill")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(append(append([]string{}, config.Workflow...), config.Prohibitions...), "\n")
	for _, want := range []string{
		"Evidence is the observed artifact text",
		"Never infer the meaning or producer of a numeric prefix",
		"progress ordinals, retry counters, status payloads",
		"until current source establishes their data flow",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("log triage observation boundary missing %q", want)
		}
	}
}
