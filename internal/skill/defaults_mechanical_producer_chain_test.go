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
		"repo_map/grep only to locate each side",
		"carrier-only enum, constant, type, schema, or event-name declaration proves identity only",
		"actual producer/callsite and consumer/handler branch",
		"aggregate_facts.member_set",
		"one `members[]` entry per compared mechanism",
		"index-aligned `member_notes[]`",
		"index-aligned `support_refs[]`",
		"ungrounded grouped_count dimensions",
		"independently supported members",
		"state that evidence boundary",
	} {
		if !strings.Contains(independentMechanismContrastDirective, required) {
			t.Fatalf("directive missing %q: %s", required, independentMechanismContrastDirective)
		}
	}
}

func TestRuntimeRuleInstantiationDirectiveSharedAndLogMechanismGated(t *testing.T) {
	registry := NewRegistry()
	RegisterDefaults(registry)

	for _, skillName := range []string{"explore-skill", "answer-document-skill"} {
		config, err := registry.Get(skillName)
		if err != nil {
			t.Fatalf("Get(%s): %v", skillName, err)
		}
		found := 0
		for _, item := range config.WorkflowTierB {
			if item.Body != runtimeRuleInstantiationDirective {
				continue
			}
			found++
			if !item.ShouldRender(AppliesToContext{HasLog: true, HasMechanism: true, Intent: types.IntentRootCause}) {
				t.Fatalf("%s directive must render for typed log plus mechanism questions", skillName)
			}
			if item.ShouldRender(AppliesToContext{HasLog: true, Intent: types.IntentRootCause}) {
				t.Fatalf("%s directive must require typed mechanism shape", skillName)
			}
			if item.ShouldRender(AppliesToContext{HasMechanism: true, Intent: types.IntentRootCause}) {
				t.Fatalf("%s directive must require an attached log", skillName)
			}
		}
		if found != 1 {
			t.Fatalf("%s directive count=%d, want 1", skillName, found)
		}
	}

	for _, required := range []string{
		"defines a predicate, threshold, classifier, advisory, or routing rule",
		"does not by itself prove that an attached runtime event satisfied the rule",
		"bind every load-bearing operand",
		"A declaration or soft-warning predicate is not the enforcement path",
		"uninstantiated advisory",
		"Do not upgrade source plausibility into runtime causality",
	} {
		if !strings.Contains(runtimeRuleInstantiationDirective, required) {
			t.Fatalf("directive missing %q: %s", required, runtimeRuleInstantiationDirective)
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
