package orchestrator

import (
	"strings"
	"testing"
)

// TestFinalizeRepairHardCapDefault pins the constant baseline.
// P6 (2026-05-10) ships with default 2 — bumping it loosens the
// chokepoint behaviour, so any future change should land here
// with a clear rationale and updated forensic anchor.
func TestFinalizeRepairHardCapDefault(t *testing.T) {
	if FinalizeRepairHardCapDefault != 2 {
		t.Errorf("P6 default cap should be 2, got %d", FinalizeRepairHardCapDefault)
	}
}

// TestSetFinalizeRepairHardCap_Bounds — the setter must clamp
// non-positive values to the default and accept positive values
// verbatim. Defensive against yaml mistakes (negative / 0).
func TestSetFinalizeRepairHardCap_Bounds(t *testing.T) {
	o := &Orchestrator{}
	// Positive value sticks.
	o.SetFinalizeRepairHardCap(5)
	if got := o.finalizeRepairHardCapValue(); got != 5 {
		t.Errorf("setter(5) → %d, want 5", got)
	}
	// 0 / negative falls back to default.
	o.SetFinalizeRepairHardCap(0)
	if got := o.finalizeRepairHardCapValue(); got != FinalizeRepairHardCapDefault {
		t.Errorf("setter(0) → %d, want default %d", got, FinalizeRepairHardCapDefault)
	}
	o.SetFinalizeRepairHardCap(-3)
	if got := o.finalizeRepairHardCapValue(); got != FinalizeRepairHardCapDefault {
		t.Errorf("setter(-3) → %d, want default %d", got, FinalizeRepairHardCapDefault)
	}
}

// TestFinalizeRepairHardCapValue_Defaults — when the field is
// unset (zero-value Orchestrator), the helper returns the default,
// not 0. Guards against the orchestrator entering a "no cap" state.
func TestFinalizeRepairHardCapValue_Defaults(t *testing.T) {
	o := &Orchestrator{} // unset finalizeRepairHardCap
	if got := o.finalizeRepairHardCapValue(); got != FinalizeRepairHardCapDefault {
		t.Errorf("unset cap → %d, want default %d", got, FinalizeRepairHardCapDefault)
	}
	// Nil receiver is also defensive (not a likely production path,
	// but tests should not panic).
	var nilOrch *Orchestrator
	if got := nilOrch.finalizeRepairHardCapValue(); got != FinalizeRepairHardCapDefault {
		t.Errorf("nil orchestrator → %d, want default %d", got, FinalizeRepairHardCapDefault)
	}
}

// TestSoftFinalizeRepairCapMessage_RedlineAudit pins the user-facing
// caveat string — covers R6 (no internal vocab), R4 (generic, no
// case names), and the CN+EN purity rule (no mid-sentence
// natural-language mixing). Each language branch is checked in
// isolation: a CN message must contain ZERO English prose words
// (CLI command names like `/repos` are exempted because they're
// code identifiers, not English prose).
func TestSoftFinalizeRepairCapMessage_RedlineAudit(t *testing.T) {
	zhMulti := softFinalizeRepairCapMessage("zh", 3, true)
	zhSingle := softFinalizeRepairCapMessage("zh", 3, false)
	enMulti := softFinalizeRepairCapMessage("en", 3, true)
	enSingle := softFinalizeRepairCapMessage("en", 3, false)
	emptyZh := softFinalizeRepairCapMessage("zh", 0, true)
	emptyEn := softFinalizeRepairCapMessage("en", 0, true)

	allMessages := []string{zhMulti, zhSingle, enMulti, enSingle, emptyZh, emptyEn}

	for _, msg := range allMessages {
		// R6: must not leak ViolKind / contract field names.
		for _, banned := range []string{
			"ViolKind", "ViolationKind", "ViolFacet", "ViolBlock",
			"FacetCoverage", "AnchoredCount", "DeclaredCount",
			"BuildRepairPlan", "RepairCluster", "AnswerDocumentV2",
			"finalizer", "extractor", "explorer", "analyzer",
			"orchestrator", "TaskGraph", "BusContext",
		} {
			if strings.Contains(msg, banned) {
				t.Errorf("caveat must not leak internal token %q; got %q", banned, msg)
			}
		}
		// R4: no third natural language tokens beyond CN+EN.
		for _, tok := range []string{"diese", "todos", "tutti", "cette", "esto", "هذا", "это"} {
			if strings.Contains(msg, tok) {
				t.Errorf("caveat must be CN+EN only; banned token %q in %q", tok, msg)
			}
		}
	}

	// CN-purity check (per 2026-05-10 user direction): the CN branch
	// must not contain English prose words. Allowed exceptions: CLI
	// command names like `/repos` (code identifier, not prose).
	englishProseInCN := []string{"REPL", "active sub-repo", "active 子仓"}
	for _, banned := range englishProseInCN {
		for _, msg := range []string{zhMulti, zhSingle, emptyZh} {
			if strings.Contains(msg, banned) {
				t.Errorf("CN branch must not mix English prose %q; got %q", banned, msg)
			}
		}
	}

	// Multi-repo branch references /repos; single-repo branch does not
	// (avoid misleading the operator about non-existent options).
	if !strings.Contains(zhMulti, "/repos") {
		t.Errorf("CN multi-repo should reference /repos; got %q", zhMulti)
	}
	if strings.Contains(zhSingle, "/repos") {
		t.Errorf("CN single-repo MUST NOT reference /repos (misleading); got %q", zhSingle)
	}
	if !strings.Contains(enMulti, "/repos") {
		t.Errorf("EN multi-repo should reference /repos; got %q", enMulti)
	}
	if strings.Contains(enSingle, "/repos") {
		t.Errorf("EN single-repo MUST NOT reference /repos (misleading); got %q", enSingle)
	}

	// Concern-count is rendered.
	if !strings.Contains(zhMulti, "3") || !strings.Contains(zhSingle, "3") {
		t.Errorf("CN should render concern count 3; got multi=%q single=%q", zhMulti, zhSingle)
	}
	if !strings.Contains(enMulti, "3") || !strings.Contains(enSingle, "3") {
		t.Errorf("EN should render concern count 3; got multi=%q single=%q", enMulti, enSingle)
	}

	// Zero-count branch: no stray "0 " before "concern".
	if strings.Contains(emptyEn, "0 residual") {
		t.Errorf("EN concernCount=0 should not render a stray '0 residual'; got %q", emptyEn)
	}
}

// TestSoftFinalizeRepairCapMessage_DefaultLanguageEN — empty / unknown
// language should fall through to English (matches sibling helpers).
func TestSoftFinalizeRepairCapMessage_DefaultLanguageEN(t *testing.T) {
	got := softFinalizeRepairCapMessage("", 1, true)
	if !strings.Contains(got, "Answer delivered") {
		t.Errorf("unknown lang should default to EN; got %q", got)
	}
	got = softFinalizeRepairCapMessage("fr", 2, false)
	if !strings.Contains(got, "Answer delivered") {
		t.Errorf("fr should not match preferZhMessage and default to EN; got %q", got)
	}
}
