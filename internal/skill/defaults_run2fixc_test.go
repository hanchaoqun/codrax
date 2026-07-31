package skill

import (
	"strings"
	"testing"
)

// defaults_run2fixc_test.go — RUN2FIX-C 批 pins (§29.174 RUN2AUDIT-1 处置④,
// witness /Users/han/opt/customlogs/runnable_2.txt, 2026-07-20). Four
// drafting-side trace teachings plus one explore-side carry companion — all
// soft guidance, zero hard arms (§29.42.4 答案出厂权属 / §29.104.13 非致命不
// 硬拦): none lives in the always-rendered workflow, none introduces a
// reject path, every item is RequiresTrace-gated.

// run2FixCExploreTierBItem finds an explore-skill Tier B item by its
// ALL-CAPS header prefix (same prefix-matching convention as
// finBindTierBItem for the answer-document skill).
func run2FixCExploreTierBItem(t *testing.T, header string) TierBItem {
	t.Helper()
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("explore-skill")
	if err != nil {
		t.Fatalf("Get(explore-skill): %v", err)
	}
	if always := strings.Join(sk.Workflow, "\n"); strings.Contains(always, header) {
		t.Fatalf("%s must not live in the always-rendered workflow", header)
	}
	for _, item := range sk.WorkflowTierB {
		if strings.HasPrefix(item.Body, header+":") {
			return item
		}
	}
	t.Fatalf("explore-skill WorkflowTierB missing %q", header)
	return TierBItem{}
}

// TestRun2FixCUserNamedQuantityCoverage — 件1 drafting duty (fork (b) of the
// mechanical map: the vsync→doFrame boundary pair has no typed carrier on
// the finalize evidence surfaces, so the duty is teaching + fact fence).
// Witness: 119.320ms derived six times during investigation (:20/:28/:84),
// absent from the shipped answer (:115) — only a neighboring 22.214ms
// wakeup-edge latency shipped.
func TestRun2FixCUserNamedQuantityCoverage(t *testing.T) {
	item := finBindTierBItem(t, "USER-NAMED END-TO-END QUANTITY COVERAGE")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("quantity-coverage duty must be trace-gated: %+v", item.AppliesTo)
	}
	for _, want := range []string{
		// The duty itself: the named object's own end-to-end quantity.
		"the answer's lead must state that object's OWN end-to-end quantity",
		"whenever the investigation established it",
		// The stand-in ban (the witness shipped one wakeup edge instead).
		"Do not let a nearby smaller number silently stand in for it",
		"it never replaces the named object's own delay",
		// The derivation lane: a boundary difference is not a cross-row sum,
		// and both anchors ride beside the value.
		"a boundary difference, not a cross-row duration sum",
		"name BOTH boundary timestamps beside the derived value",
		// The fact fence stays with PSG-1 (R6: referenced by header, not
		// re-taught).
		"PROSE NUMBER GROUNDING governs the numbers themselves",
		// The honest-gap lane: disclosure, never fabrication.
		"never invent, estimate, or substitute a replacement number",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("quantity-coverage duty missing %q:\n%s", want, item.Body)
		}
	}
	if len(item.OnViolation) != 0 {
		t.Fatalf("quantity-coverage duty is soft teaching — no violation lane: %+v", item.OnViolation)
	}
}

// TestRun2FixCUserNamedLatencyAnchorCarry — 件1 explore companion: the carry
// duty that makes the drafting duty satisfiable (the witness's boundary
// event rows never entered emitted evidence, so the drafting side could not
// state the delay under the evidence fence).
func TestRun2FixCUserNamedLatencyAnchorCarry(t *testing.T) {
	item := run2FixCExploreTierBItem(t, "USER-NAMED LATENCY ANCHOR CARRY")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("anchor-carry duty must be trace-gated: %+v", item.AppliesTo)
	}
	for _, want := range []string{
		"emit those boundary rows as evidence with their timestamps verbatim",
		"carry the derived end-to-end quantity, with both boundary timestamps, into the completion `reason`",
		"never reaches the final evidence surfaces",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("anchor-carry duty missing %q:\n%s", want, item.Body)
		}
	}
	if len(item.OnViolation) != 0 {
		t.Fatalf("anchor-carry duty is soft teaching — no violation lane: %+v", item.OnViolation)
	}
}

// TestRun2FixCReaderWordsOverFieldSpellings — 件2: wire k=v field spellings
// and underscore enum tokens stay off user-facing prose; the published
// display words replace them; the fact fence (values / caliber words /
// state words / [E#]) is untouched. Witness: tier=primary ×2,
// tier=secondary ×2, tier=tertiary, 「runnable_wait dominant_state=
// runnable」, 「d_state_or_io_wait有效归因」, 「根因排名on-chain」 ×2
// (:115/:119/:120/:121).
func TestRun2FixCReaderWordsOverFieldSpellings(t *testing.T) {
	item := finBindTierBItem(t, "READER WORDS OVER FIELD SPELLINGS")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("reader-words rule must be trace-gated: %+v", item.AppliesTo)
	}
	for _, want := range []string{
		// The witnessed wire families are named verbatim.
		"`tier=primary`",
		"`dominant_state=runnable`",
		"`chain_relevance=on_chain`",
		"`d_state_or_io_wait`",
		// The rule: field names are addressing, not reader words.
		"Those spellings are data field names, not reader words",
		"A field name may appear only as a quoted key beside its cited evidence row",
		// The published display words offered as replacements.
		"链上/邻近",
		"D状态/IO候选",
		"根因排序#N",
		// The fact-fence carve: only the spelling is replaced, never the
		// values / caliber words / state words / [E#].
		"The fact fence is unchanged",
		"only the field-name/enum spelling wrapped around them is replaced by the published word",
		// R5: one negative and one positive shape.
		"Negative shape (do not ship): 「…d_state_or_io_wait有效归因10.433ms(tier=tertiary)」",
		"Positive shape: 「…的D状态/IO候选,有效归因10.433ms,根因排序#3,修向 IO/内核/依赖 (IO / kernel / dependency)」",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("reader-words rule missing %q:\n%s", want, item.Body)
		}
	}
	if len(item.OnViolation) != 0 {
		t.Fatalf("reader-words rule is soft teaching — no violation lane: %+v", item.OnViolation)
	}
}

// TestRun2FixCTraceAnswerSkeleton — 件3: the four-move answer skeleton plus
// the short-paragraph duty. Witness: a ~1100-character single opening
// paragraph (:115) followed by a seven-item per-seat value re-read
// (:117-:123), with the top eliminable causes 40+ lines below.
func TestRun2FixCTraceAnswerSkeleton(t *testing.T) {
	item := finBindTierBItem(t, "TRACE ANSWER SKELETON")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("skeleton rule must be trace-gated: %+v", item.AppliesTo)
	}
	for _, want := range []string{
		"organize a trace root-cause answer in four moves",
		// ① quantified conclusion first.
		"① Open with ONE quantified conclusion",
		"one or two short sentences, nothing in front of them",
		// ② own-account split (designed-in wait vs the real bottleneck).
		"② Then split the target's own account",
		"which part is the real bottleneck",
		// ③ top eliminable + repair directions, bounded, [E#]-anchored.
		"③ Then the top eliminable causes with their repair directions",
		"never generic template steps",
		// ④ appendix authority faces; no per-seat prose re-read.
		"④ Point everything else at the report's own deterministic faces",
		"prose does NOT re-read every seat's value account row by row",
		// The short-paragraph duty.
		"split a long conclusion into short sentences or bullets",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("skeleton rule missing %q:\n%s", want, item.Body)
		}
	}
	if len(item.OnViolation) != 0 {
		t.Fatalf("skeleton rule is soft teaching — no violation lane: %+v", item.OnViolation)
	}
}

// TestRun2FixCTotalsMatchTheirParts — 件4 (F4①): a stated total equals the
// exact sum of its listed parts; wakeup exchange totals come from the
// per-direction census counts, never from a state-switch count. Witness:
// 「wakeup往来共计62次(31次+34次)」 ×3 (:115/:118/:122) — 31+34=65 (the
// investigation computed 65 at :67); 62 was the state_churn face's 62
// 次切换 (:509).
func TestRun2FixCTotalsMatchTheirParts(t *testing.T) {
	item := finBindTierBItem(t, "TOTALS MATCH THEIR PARTS")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("totals rule must be trace-gated: %+v", item.AppliesTo)
	}
	for _, want := range []string{
		"the total must equal the exact sum of the listed parts",
		"never a nearby number remembered from a different measurement",
		// The census lane for wakeup totals.
		"the per-pair census counts",
		// The state-churn borrow ban (the witness's actual disease).
		"counts scheduler state transitions, not wakeup exchanges",
		"never borrow it as a wakeup total",
		// The honest-partial lane.
		"instead of presenting a partial list as the full decomposition",
		// R5: arithmetic negative/positive shapes.
		"「wakeup往来共62次(唤醒31次+被唤醒34次)」 — 31+34=65",
		"「wakeup往来共65次(唤醒31次+被唤醒34次)」",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("totals rule missing %q:\n%s", want, item.Body)
		}
	}
	if len(item.OnViolation) != 0 {
		t.Fatalf("totals rule is soft teaching — no violation lane: %+v", item.OnViolation)
	}
}

func TestWakeupCensusDirectionAndStateTeaching(t *testing.T) {
	item := finBindTierBItem(t, "WAKEUP CENSUS DIRECTION AND STATE")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("wakeup census rule must be trace-gated: %+v", item.AppliesTo)
	}
	for _, want := range []string{
		"waker -> wakee",
		"state the WAKEE LEFT",
		"pre-wakeup state",
		"A wakeup makes the target runnable",
		"Never turn sleep_exit=N into “after each wake it immediately slept”",
		"requires a separately complete paired transition census",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("wakeup census rule missing %q:\n%s", want, item.Body)
		}
	}
	if len(item.OnViolation) != 0 {
		t.Fatalf("wakeup census rule is soft teaching — no violation lane: %+v", item.OnViolation)
	}
}
