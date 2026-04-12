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

// TestReverseRefExtraction_CurrentBrokenOutput characterizes the
// current extractAnswerSymbols behavior on the failing fixture.
// Passes today; failing this test means the extraction changed and
// the change needs to be reviewed (ideally toward the correct output
// documented in TestReverseRefExtraction_TargetCorrectOutput below).
func TestReverseRefExtraction_CurrentBrokenOutput(t *testing.T) {
	items := []types.EvidenceItem{evidenceFromFailingRun()}

	syms := extractAnswerSymbols(items, "enumeration", nil)

	if len(syms) != 1 {
		t.Fatalf("expected exactly 1 extracted symbol in current (broken) behavior, got %d: %+v", len(syms), syms)
	}
	got := syms[0].Name
	if got != "SubExplorer" {
		t.Errorf("characterization: current broken extraction should yield %q, got %q — if this is now %q the fix has landed, remove t.Skip in the target-behavior test",
			"SubExplorer", got, "explorer")
	}
	// Belt-and-braces: SubExplorer is the CALLEE symbol. The correct
	// answer for a "who can call" question should not even look like
	// a sub-agent class name. This assertion is redundant with the
	// one above but documents the semantic gap for readers.
	if got == "explorer" || got == "ExplorerAgent" {
		t.Errorf("unexpected improvement: extraction now returns %q — flip this characterization test and un-skip the target test", got)
	}
}

// TestReverseRefExtraction_TargetCorrectOutput documents the
// assertion the fix must satisfy. SKIPPED today because the fix has
// not landed. When unskipped, it must pass on the same fixture.
//
// The target behavior: for a registration-shape evidence item whose
// Summary chains forward to a `returns "literal"` terminal, and where
// the question is a reverse-reference enumeration (`who can call X`
// / `who uses X`), extraction should prefer the terminal literal as
// the caller-name anchor instead of the registered class name.
//
// Concretely, from
//
//     `RegisterDefaultSubAgents()` binds ONLY NewSubExplorer(deps)
//     → `SubExplorer.Name()` returns "explorer"
//
// the caller identity the question asks for is "explorer" — the name
// under which the SubAgent is registered, which is also the agent
// name any BaseAgent must have for SubAgents.Get(b.name) to succeed
// at internal/agent/agent.go:601.
func TestReverseRefExtraction_TargetCorrectOutput(t *testing.T) {
	t.Skip("reproducer for known bug: reverse-reference enumeration mis-extraction. " +
		"Un-skip when internal/agent/erm.go:extractAnswerSymbols learns to prefer " +
		"the terminal `returns \"literal\"` hop for reverse-ref question kinds.")

	items := []types.EvidenceItem{evidenceFromFailingRun()}

	syms := extractAnswerSymbols(items, "enumeration", nil)

	// Must produce at least one symbol; the first one must anchor on
	// the caller identity, not the callee class name.
	if len(syms) == 0 {
		t.Fatalf("expected at least one extracted symbol, got 0")
	}

	var hasExplorerAnchor bool
	var hasCalleeClassName bool
	for _, s := range syms {
		switch s.Name {
		case "explorer", "ExplorerAgent", "Explorer":
			hasExplorerAnchor = true
		case "SubExplorer":
			hasCalleeClassName = true
		}
	}
	if !hasExplorerAnchor {
		t.Errorf("extraction should include the caller-side anchor (explorer / ExplorerAgent), got symbols: %+v", syms)
	}
	if hasCalleeClassName {
		t.Errorf("extraction should NOT include the callee class name SubExplorer as the primary answer for a reverse-reference question, got: %+v", syms)
	}
}
