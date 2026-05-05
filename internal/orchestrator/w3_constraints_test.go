package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// W3 tests pin the schema-driven constraint preview + locus
// routing fixable-by-agent validation per
// docs/design/iteration_inflation_remediation.md W3.

// TestW3_SchemaDescriptionFragmentsReachToolDescription pins W3.2
// wiring: every ViolKindSpec.SchemaDescriptionFragment that's
// non-empty appears in the emit_answer_document tool description.
// Without this, the LLM-facing surface drifts from the validator
// and structural rules are learned post-rejection only.
func TestW3_SchemaDescriptionFragmentsReachToolDescription(t *testing.T) {
	desc := tool.BuildAnswerDocumentSemanticContractDescription()
	if !strings.Contains(desc, "PRE-EMIT CONSTRAINTS") {
		t.Fatal("schema description missing PRE-EMIT CONSTRAINTS section — W3.2 wiring lost")
	}
	for _, spec := range types.AllViolKindSpecs() {
		fragment := strings.TrimSpace(spec.SchemaDescriptionFragment)
		if fragment == "" {
			continue
		}
		if !strings.Contains(desc, fragment) {
			t.Errorf("ViolKindSpec for %q has fragment %q not in tool description",
				spec.Kind, fragment)
		}
	}
}

// TestW3_NoInternalJargonInFragments enforces the LLM-facing
// surface red line: no ViolKind names, no Go type names, no
// orchestration jargon in fragments.
func TestW3_NoInternalJargonInFragments(t *testing.T) {
	forbidden := []string{
		"ViolKind", "IRField", "RepairLocus", "FallbackTarget",
		"ViolBlock", "ViolDiagram", "ViolFacet", "ViolEnumeration",
		"AgentFinalizer", "AgentExtractor", "AgentExplorer",
		"yield kill", "Pipeline terminated",
	}
	for _, spec := range types.AllViolKindSpecs() {
		fragment := spec.SchemaDescriptionFragment
		if fragment == "" {
			continue
		}
		for _, token := range forbidden {
			if strings.Contains(fragment, token) {
				t.Errorf("fragment for %q leaks forbidden token %q: %q",
					spec.Kind, token, fragment)
			}
		}
	}
}

// TestW3_FixableByAgents_BlocksUnsoundDowngrade pins W3.5: when a
// violation's FixableByAgents excludes AgentFinalizer, the R2.2
// downgrade must NOT route to FallbackFinalizerOnly even when
// finalizer-local budget allows it.
//
// qfa-mr3 forensic case: ViolEnumerationLabelUngrounded (owner=
// extract, FixableByAgents=[extractor, explorer]) was routed to
// finalizer_only via R2.2 downgrade -> finalizer dispatched -> no
// file tools -> guaranteed fail round.
func TestW3_FixableByAgents_BlocksUnsoundDowngrade(t *testing.T) {
	violations := []types.Violation{
		{Kind: types.ViolEnumerationLabelUngrounded, Detail: "items[0].label not in evidence pool"},
	}
	// finalizerLocalUsed=0 means budget is fully available — pre-W3
	// would unconditionally downgrade to finalizer_only.
	target := FallbackTargetForViolationsWithBudget(violations, 0)
	if target == FallbackFinalizerOnly {
		t.Errorf("FixableByAgents=[extractor, explorer] must block downgrade to finalizer_only; got %s",
			target)
	}
}

// TestW3_ViolationsFixableByAgent_PerKindSemantics pins the helper
// directly: when ALL violations' FixableByAgents lists include the
// candidate agent (or are empty / unconstrained), the helper
// returns true; when any is missing, returns false.
func TestW3_ViolationsFixableByAgent_PerKindSemantics(t *testing.T) {
	// EnumerationLabelUngrounded.FixableByAgents = [extractor, explorer]
	// — finalizer NOT in list.
	if violationsFixableByAgent(
		[]types.Violation{{Kind: types.ViolEnumerationLabelUngrounded}},
		types.AgentFinalizer,
	) {
		t.Error("finalizer should not be marked fixer for label-ungrounded")
	}
	// BlockCoverageMissing.FixableByAgents = [finalizer, extractor]
	// — finalizer IS in list.
	if !violationsFixableByAgent(
		[]types.Violation{{Kind: types.ViolBlockCoverageMissing}},
		types.AgentFinalizer,
	) {
		t.Error("finalizer should be marked fixer for block-coverage-missing")
	}
	// Mixed set with one violation requiring extractor only -> false.
	if violationsFixableByAgent(
		[]types.Violation{
			{Kind: types.ViolBlockCoverageMissing},
			{Kind: types.ViolEnumerationLabelUngrounded},
		},
		types.AgentFinalizer,
	) {
		t.Error("mixed set with non-finalizer-fixable kind must return false")
	}
}

// TestW3_FixableByAgents_EmptyListIsUnconstrained — when a
// violation has empty FixableByAgents, the validation passes
// (legacy behaviour preserved for kinds that haven't been
// classified yet).
func TestW3_FixableByAgents_EmptyListIsUnconstrained(t *testing.T) {
	// ViolGhostAnchor doesn't have FixableByAgents set yet (no
	// W3.4 entry). Should pass through.
	if !violationsFixableByAgent([]types.Violation{{Kind: types.ViolGhostAnchor}}, types.AgentFinalizer) {
		t.Error("empty FixableByAgents should be treated as unconstrained")
	}
}
