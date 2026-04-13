package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This file is a DETERMINISTIC REPRODUCER for the "reverse-reference
// enumeration" failure class observed on 2026-04-12 against the
// question "有多少个agent可以调用subagent" (how many agents can call
// sub-agents?).
//
// Full debug trace lives in /tmp/repro_run.log at repro time. The
// failing pipeline:
//
//   1. Explorer grepped for `subagent`, `SubAgent`, `AgentRegistry`,
//      read internal/agent/subagent.go + sub_explorer.go + agent.go,
//      and (correctly) surfaced the concrete chain
//        `RegisterDefaultSubAgents()` binds ONLY NewSubExplorer(deps)
//        → `SubExplorer.Name()` returns "explorer"
//      into strict evidence. So far so good.
//
//   2. extractAnswerSymbols picked the registration-shape item, fell
//      into the `isRegistrationShape` branch, and extracted
//      stripNewPrefix(firstUppercaseIdent("NewSubExplorer(deps)"))
//      = "SubExplorer" as the sole AnswerSymbol.
//
//   3. The finalizer, in translation mode, dutifully rendered the one
//      allowed symbol:
//        **Answer:** 唯一可以调用的是 SubExplorer。
//
// That answer is WRONG for the question actually asked. The question
// asks for the *callers* of the sub-agent subsystem — i.e., which
// BaseAgent(s) get `propose_sub_agents` auto-injected by the check at
// internal/agent/agent.go:601:
//
//     if _, err := b.deps.SubAgents.Get(string(b.name)); err == nil {
//         // ... inject propose_sub_agents tool ...
//     }
//
// The answer is the set of agent names `b.name` for which
// `SubAgents.Get(name)` succeeds — i.e., the agents whose name
// matches a registered SubAgent's Name(). With the current
// registration (`NewSubExplorer`, whose Name() returns "explorer"),
// exactly one agent satisfies this: the "explorer" agent.
//
// So the correct AnswerSymbol set is `{"explorer"}` (or at least
// includes it), NOT `{"SubExplorer"}` — SubExplorer is the *callee*
// (the registered sub-agent type), not the caller.
//
// These tests lock both sides of that gap:
//
//   - TestReverseRefExtraction_CurrentBrokenOutput passes today by
//     asserting the current (wrong) extraction. It is a
//     characterization test: breaking it means the extraction
//     behavior has changed, for better or worse — re-check it.
//
//   - TestReverseRefExtraction_TargetCorrectOutput is skipped today
//     via t.Skip. It documents the assertion a correct fix must
//     pass. When the fix lands, remove the Skip to promote this to
//     a green-path regression guard.
//
// See MEMORY.md entry for broader context on reverse-reference
// enumeration as a class of questions the pipeline currently fumbles.

// evidenceFromFailingRun returns the exact EvidenceItem that
// extractAnswerSymbols consumed in the 2026-04-12 repro. Reconstructed
// from the finalizer prompt's "Ground Truth" section in the debug log:
//
//   - `RegisterDefaultSubAgents()` binds ONLY NewSubExplorer(deps)
//     → `SubExplorer.Name()` returns "explorer"
//     (internal/agent/subagent.go:63)
//
// This is an EvidenceConcrete + `binds ONLY` predicate, which
// `isRegistrationShape` matches and routes through the registration
// extraction branch.
func evidenceFromFailingRun() types.EvidenceItem {
	return types.EvidenceItem{
		ID:        "ev-repro-reverse-ref",
		Kind:      types.EvidenceConcrete,
		Subject:   "RegisterDefaultSubAgents()",
		Predicate: "binds ONLY",
		Object:    "NewSubExplorer(deps)",
		Summary:   "`RegisterDefaultSubAgents()` binds ONLY NewSubExplorer(deps) → `SubExplorer.Name()` returns \"explorer\"",
		Source:    "internal/agent/subagent.go",
		LineStart: 63,
		Producer:  "concrete_extractor",
	}
}

// TestReverseRefExtraction_Phase2Fixed verifies the 2026-04-12 repro
// is fixed by the Phase 2 direction-aware extraction.
//
// The question "有多少个agent可以调用subagent" contains 多少 + 调用,
// so classifyAnswerRole routes it to RoleAnchor. pickHop then walks
// the evidence chain right-to-left looking for a string literal and
// finds "explorer" in the terminal `returns "explorer"` hop.
//
// Before Phase 2 this returned "SubExplorer" (the callee class name
// at the registration site); the pre-flip characterization text is
// preserved in git history if needed for archaeology.
func TestReverseRefExtraction_Phase2Fixed(t *testing.T) {
	items := []types.EvidenceItem{evidenceFromFailingRun()}
	question := "有多少个agent可以调用subagent"

	syms := extractAnswerSymbols(items, "enumeration", question, "", nil)

	if len(syms) != 1 {
		t.Fatalf("expected 1 extracted symbol, got %d: %+v", len(syms), syms)
	}
	got := syms[0].Name
	if got != "explorer" {
		t.Errorf("Phase 2: RoleAnchor should surface the terminal literal %q, got %q", "explorer", got)
	}
	if got == "SubExplorer" {
		t.Errorf("regression: extraction still returns the callee class name — classifier or pickHop broken")
	}
}

// TestReverseRefExtraction_NoQuestionStaysLegacy verifies that when
// the caller supplies an empty question string (the code path
// existing unit tests in erm_test.go exercise), the classifier
// defaults to RoleTerminal and pickHop falls through to the legacy
// pickTerminalLegacy. This is the backward-compat guarantee: Phase 2
// does not change any output that was correct before.
func TestReverseRefExtraction_NoQuestionStaysLegacy(t *testing.T) {
	items := []types.EvidenceItem{evidenceFromFailingRun()}

	syms := extractAnswerSymbols(items, "enumeration", "", "", nil)

	if len(syms) != 1 {
		t.Fatalf("expected 1 symbol, got %d: %+v", len(syms), syms)
	}
	if syms[0].Name != "SubExplorer" {
		t.Errorf("empty question should follow the legacy RoleTerminal path and produce %q, got %q",
			"SubExplorer", syms[0].Name)
	}
}
