package skill

// PSG batch P-s pins (§25 ruling b assertion half,
// docs/design/real_trace_campaign_20260705.md, 2026-07-08; huadong_01 §22
// C-P1): the answer-side skill carries the trace-gated discipline
// sentences — prose numeral grounding (locate-or-remove; the G14 §28.1
// evolution retired the view+window escape) and object identity assertions
// (thread names and object names are never interchangeable; holder claims
// need typed evidence rows). The Wave-3.1 GAP-C batch (§27.4/§28.1,
// 2026-07-09) adds three more: primary-cause entity consistency (G13a),
// lock-wait site quotation (G13b), and hop citation-assertion alignment
// (G16). All bodies are also swept by TestNoInternalTermsInPrompts, so a
// jargon regression fails there; these pins hold the SUBSTANCE.

import (
	"strings"
	"testing"
)

func psgAnswerSkillTierBBody(t *testing.T, marker string) TierBItem {
	t.Helper()
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("Get(answer-document-skill) returned error: %v", err)
	}
	// Prefix match, not Contains: clause bodies may NAME sibling clauses
	// (the P2-3 single-authority pointer inside BACKGROUND AGGREGATE
	// HEADLINE names TRACE PRIMARY-CAUSE ENTITY CONSISTENCY), and every
	// marker is the clause's own leading token.
	for _, item := range sk.WorkflowTierB {
		if strings.HasPrefix(item.Body, marker) {
			return item
		}
	}
	t.Fatalf("answer-document-skill WorkflowTierB missing the %q item", marker)
	return TierBItem{}
}

func TestPSGFinalizerSkillProseNumberGrounding(t *testing.T) {
	item := psgAnswerSkillTierBBody(t, "PROSE NUMBER GROUNDING")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("the prose-number rule is trace-gated; AppliesTo=%+v", item.AppliesTo)
	}
	for _, want := range []string{
		"must be locatable",
		// G14 (§27.4/§28.1 ruling, 2026-07-09): exploration-phase
		// intermediates that never entered the final evidence surfaces are
		// prohibited outright — the former same-sentence view+window escape
		// is retired (five opendir_79_01 witnesses rode it into the body).
		"must be removed from the prose",
		"never entered those final evidence surfaces",
		"prohibited in the body",
		"does not license them",
		"name the published values it was derived from",
		// PSG-2 binding clause (§24.14 B-3/D-2): the window/thread named
		// next to a number must be its publishing row's, and cross-window
		// comparisons normalize per side first.
		"must be the ones the number's evidence row was published under",
		"normalize each side by its own window length",
		// PSG-2H thread-identity clause (§29.10-2, 2026-07-10): thread
		// identity tokens are copied verbatim from an evidence surface —
		// never assembled, adjusted, or recalled from memory (792 witness:
		// a fabricated spelling one character from the only real thread).
		"copied verbatim from an evidence surface",
		"never assemble, adjust, or recall a thread name or id",
		"drop the token or use the published spelling",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("prose-number rule missing %q:\n%s", want, item.Body)
		}
	}
	// Negative pin (G14): the retired escape must not come back — a prose
	// numeral outside the final evidence surfaces cannot be licensed by
	// naming its source view and window inline.
	if strings.Contains(item.Body, "exact source view and time window") {
		t.Fatalf("prose-number rule must not re-open the same-sentence view+window escape:\n%s", item.Body)
	}
}

// TestG13FinalizerSkillPrimaryCauseEntityConsistency pins the §27.4 G13 /
// §28.1 ruling (2026-07-09) headline half: the prose's primary-cause entity
// follows the ranked ordering (discounts/demotions included), and a genuine
// divergence must be stated explicitly instead of silently rewriting the
// main cause (huadong_79_01 witness: prose led with a cadence-discounted
// periodic chain while the largest attributable ranked cause was IO).
func TestG13FinalizerSkillPrimaryCauseEntityConsistency(t *testing.T) {
	item := psgAnswerSkillTierBBody(t, "TRACE PRIMARY-CAUSE ENTITY CONSISTENCY")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("the primary-cause consistency rule is trace-gated; AppliesTo=%+v", item.AppliesTo)
	}
	for _, want := range []string{
		"must be the SAME entity the ranked root-cause evidence puts first",
		"discounted attribution",
		"combined total",
		"tier=target_self_state",
		// GAP-A review P3-3 (2026-07-09): data_gap rows joined the demotion
		// enumeration when the engine gained the data_gap tier (G2).
		"tier=data_gap",
		// G1 batch (2026-07-09): absorbed critical_blocking rows are the
		// same events as the family row — never an additional cause.
		"absorbed_by_rank_family=true",
		"never as an additional separate cause",
		"Do not promote a lower-attribution cause to the headline",
		"state explicitly that your conclusion diverges from the ranked ordering",
		"silently substituting a different entity as the main cause",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("primary-cause consistency rule missing %q:\n%s", want, item.Body)
		}
	}
}

// TestG13FinalizerSkillLockWaitSiteQuotation pins the §27.4 G13 wait-point
// half (2026-07-09): the wait/blocking site may only quote the contention
// span's own recorded text (the `blocking from` segment), never a call site
// inferred from framework knowledge or thread role (opendir_79_01 witness:
// a message-queue entry was named while the span said the wait happened in
// an asset resource read).
func TestG13FinalizerSkillLockWaitSiteQuotation(t *testing.T) {
	item := psgAnswerSkillTierBBody(t, "LOCK-WAIT SITE QUOTATION")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("the wait-site quotation rule is trace-gated; AppliesTo=%+v", item.AppliesTo)
	}
	for _, want := range []string{
		"ONLY from the contention span's own recorded text",
		"`blocking from`",
		"verbatim",
		"Never infer the call site from framework knowledge",
		"is a fabrication",
		"without naming a code site",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("wait-site quotation rule missing %q:\n%s", want, item.Body)
		}
	}
}

// TestG13FinalizerSkillHeadlineSingleAuthority pins the GAP-C 复核 P2-3
// evolution (2026-07-09): the BACKGROUND AGGREGATE HEADLINE clause no longer
// binds the headline to the literal rank=1 row (a semantic optimization span
// or the target's own symptom row can wear rank=1 verbatim after the rank
// re-numbering) — it excepts those two row kinds and defers WHICH entity the
// headline names to the TRACE PRIMARY-CAUSE ENTITY CONSISTENCY rule as the
// single authority. The retired double-authority sentence must not return.
func TestG13FinalizerSkillHeadlineSingleAuthority(t *testing.T) {
	item := psgAnswerSkillTierBBody(t, "BACKGROUND AGGREGATE HEADLINE")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("the aggregate-headline rule is trace-gated; AppliesTo=%+v", item.AppliesTo)
	}
	for _, want := range []string{
		"not a deterministic-optimization span",
		"not the target's own symptom row",
		"decided solely by the TRACE PRIMARY-CAUSE ENTITY CONSISTENCY rule",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("aggregate-headline rule missing %q:\n%s", want, item.Body)
		}
	}
	if strings.Contains(item.Body, "MUST name that on-chain cause") {
		t.Fatalf("the retired double-authority sentence must not return:\n%s", item.Body)
	}
}

// TestG16FinalizerSkillHopCitationAssertionAlignment pins §27.4 G16
// (2026-07-09): a hop's citation must match the hop's own assertion kind,
// and the one-position citation drift (opendir_79_01: the priority-inversion
// hop carried an IO-latency reference) is called out as the failure shape.
func TestG16FinalizerSkillHopCitationAssertionAlignment(t *testing.T) {
	item := psgAnswerSkillTierBBody(t, "HOP CITATION-ASSERTION ALIGNMENT")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("the citation-alignment rule is trace-gated; AppliesTo=%+v", item.AppliesTo)
	}
	for _, want := range []string{
		"SAME kind as that item's own assertion",
		"priority inversion must not carry an IO-latency row's reference",
		"re-check each item's citation_ref",
		"off-by-one drift",
		"leave that item uncited and state the boundary",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("citation-alignment rule missing %q:\n%s", want, item.Body)
		}
	}
}

func TestPSGFinalizerSkillObjectIdentityAssertions(t *testing.T) {
	item := psgAnswerSkillTierBBody(t, "OBJECT IDENTITY ASSERTIONS")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("the identity-assertion rule is trace-gated; AppliesTo=%+v", item.AppliesTo)
	}
	for _, want := range []string{
		"must be backed by an evidence row",
		"thread names and object names are never interchangeable",
		"does not hold that object by virtue of its name",
		"without asserting a holder",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("identity-assertion rule missing %q:\n%s", want, item.Body)
		}
	}
}
