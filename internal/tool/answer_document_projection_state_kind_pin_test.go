package tool

// answer_document_projection_state_kind_pin_test.go — TSH batch StateKind pin,
// tool-side half (memory reaudit P1-1; the types-side twin pins the compile
// face). Scanner + rule checker are the SHARED implementation in
// internal/types/trace_state_kinds_scan.go (TSH review F6: the former private
// twin copies of the scanner had two unrecorded semantic forks — types-only
// classifier-func lane, tool-only silent skip of empty tagged switches — and
// a false "used verbatim" cross-reference; one implementation now serves both
// pins, with the forks surfaced as parameters/behavior). Three rules:
//
//  1. Switch coverage: every renderer switch whose tag derives from
//     .StateKind covers the word universe, carries a default, or declares its
//     fall-through words in the ledger with a rationale.
//  2. Literal registration: every case / comparison literal on a
//     StateKind-derived expression must be a registered word
//     (types.TraceStateKindUniverse), "" (absence checks), or a ledgered
//     consumer-only alias. Divergence record — "runnable_wait" at
//     runtimeTraceProjSymptomFamilyStateKind is KEPT verbatim (历史裁定 RN-6):
//     it is not producible as StateKind today (dominant lanes emit "runnable",
//     the Object fallback recognizes only the universe) but the defensive
//     branch stays; unifying it away would be the ping-pong this pin forbids.
//     TSH review F5: the alias ledger is orphan-guarded — deleting the RN-6
//     case (even with a matching golden refresh) leaves the ledger row
//     unconsumed and the pin goes red instead of letting it rot.
//  3. Bridge: every tracequery dominant-lane string (the dominant_state note
//     value set) is a registered word, and stopped/dead/unknown stay
//     UNREGISTERED until a ruling gives them a word lane — extending the
//     dominant lanes without extending the universe goes red here.
//
// matched==0 fatals on every rule; test files excluded (fixture literals).

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

var runtimeStateKindPinFiles = []string{
	"answer_document_mutation_runtime.go",
	"answer_document_mutation_runtime_tree.go",
	// PTV8-RCR-A §24.3: the glyph switch moved into the impact-form
	// classifier (answer_document_mutation_runtime_rcr.go) — the pin follows
	// the switch to its new site instead of losing coverage.
	"answer_document_mutation_runtime_rcr.go",
	// DSTATE-REFINE (CAL-1 件③, 2026-07-12): the arm-c tail-redundancy
	// predicate switches on StateKind in the typelabels home — scanned.
	"answer_document_mutation_runtime_typelabels.go",
}

// runtimeStateKindConsumerAliasLedger: consumer-only case words that are NOT
// universe members, kept verbatim with the divergence recorded (rule 2).
// Orphan-guarded (rule 2, TSH F5): every entry must still be consumed by a
// scanned case/comparison.
var runtimeStateKindConsumerAliasLedger = map[string]string{
	"runnable_wait": "RN-6 defensive alias at runtimeTraceProjSymptomFamilyStateKind: not producible as StateKind today (dominant lanes publish \"runnable\"; Object fallback admits only the universe); branch preserved byte-for-byte — do not \"clean up\" without a ruling",
}

// runtimeStateKindClassifierFuncs: functions whose switch tag is a
// StateKind-carrying PARAMETER (tag-text detection cannot see ".StateKind").
// PTV6-C #6 extracted the ActionCell state switch into the single wording
// home runtimeTraceCausalProjectionStateActionWord (callers pass
// node.StateKind / the ActionCell effective state) — the pin follows the
// switch to its new site instead of losing coverage.
var runtimeStateKindClassifierFuncs = map[string]bool{
	"runtimeTraceCausalProjectionStateActionWord": true,
}

var runtimeStateKindSwitchFallthroughLedger = map[string]types.TraceStateKindFallthroughDecl{
	"answer_document_mutation_runtime.go:runtimeTraceCausalProjectionStateActionWord#1": {
		Missing: "sleep,s_sleep,sleep_wait",
		Why:     "sleep family handled upstream by ActionCell's IsSleepState early return; remaining values (incl. the causeKind fallback ActionCell may pass) fall to \"\" and the caller keeps its generic candidate-cause wording",
	},
	"answer_document_mutation_runtime_tree.go:runtimeTraceProjWaitFamilyStateKind#1": {
		Missing: "running,runnable",
		Why:     "RN-6 (§7.9) split: running is occupancy and runnable is NOT '疑似空闲' — the wait family is sleep/D/blocked only; runnable joins the SYMPTOM family below instead",
	},
	"answer_document_mutation_runtime_tree.go:runtimeTraceProjSymptomFamilyStateKind#1": {
		Missing: "running,sleep,s_sleep,sleep_wait,d_sleep,d_state,io_wait,uninterruptible_sleep",
		Why:     "wait-family words already admitted by the runtimeTraceProjWaitFamilyStateKind short-circuit above; running/stateless rows stay excluded (occupancy, not wait time)",
	},
	"answer_document_mutation_runtime_tree.go:runtimeTraceProjNoDominantStateRow#1": {
		Missing: "sleep,s_sleep,sleep_wait",
		Why:     "PTV4 T4 ◦ 二义拆分: the sleep family is short-circuited by the IsSleepState early return above (☾ rows never render ◦); the switch only lists the remaining non-◦ states — anything else IS the no-dominant-state ◦ sense",
	},
	"answer_document_mutation_runtime_rcr.go:runtimeTraceProjImpactFormForNode#1": {
		Missing: "running,runnable,sleep,s_sleep,sleep_wait,io_wait",
		Why:     "DSTATE-REFINE arm b (件③, 2026-07-12): the refined-to-iowait gate matches ONLY a D-family dominant state paired with the typed io_wait TypeToken (96728 E14 shape); every other state falls through to the ordinary form ladder below (sleep short-circuited above, running/runnable/io_wait handled by site #2)",
	},
	"answer_document_mutation_runtime_rcr.go:runtimeTraceProjImpactFormForNode#2": {
		Missing: "sleep,s_sleep,sleep_wait",
		Why:     "PTV8-RCR-A §24.3: the sleep family is short-circuited by the IsSleepState early return above (☾ form); non-state rows fall through to the typed token-family lane, then ◦",
	},

	"answer_document_mutation_runtime_rcr.go:runtimeTraceProjInversionStateCompositionWord#1": {
		Missing: "sleep,s_sleep,sleep_wait,d_sleep,d_state,io_wait,uninterruptible_sleep",
		Why:     "GAP-B G7 词值同源 (§27.3, 2026-07-09): the inversion 行1 word-lane speaks only the two inversion COMPONENT states (runnable/running — the states the gated decomposition accounts); a sleep/D/IO-dominant window projection has no seat in the composition grammar, so the word falls back to the gated composition below (fail-open to legacy, never a fabricated component word)",
	},
}

var runtimeStateKindSwitchSiteGolden = map[string]string{
	"answer_document_mutation_runtime.go:runtimeTraceCausalProjectionStateActionWord#1": "running,runnable,d_sleep,d_state,io_wait,uninterruptible_sleep",
	// PTV6-D (b): ImpactShapeCell → ImpactShapeCellTyped rename only (the
	// typed generic-arm second return) — the case set is byte-identical.
	"answer_document_mutation_runtime.go:runtimeTraceCausalProjectionImpactShapeCellTyped#1": "running,runnable,sleep,s_sleep,sleep_wait,d_sleep,d_state,io_wait,uninterruptible_sleep",
	"answer_document_mutation_runtime_tree.go:runtimeTraceProjNoDominantStateRow#1":          "running,runnable,d_sleep,d_state,io_wait,uninterruptible_sleep",
	// PTV8-RCR-A §24.3 EVOLUTION RECORD: runtimeTraceProjStateIcon's switch
	// moved into the single-source impact-form classifier (rcr.go) — same
	// case set, no default arm (the classifier falls through to the typed
	// token-family lane, then ◦).
	"answer_document_mutation_runtime_rcr.go:runtimeTraceProjImpactFormForNode#1": "d_sleep,d_state,uninterruptible_sleep",
	"answer_document_mutation_runtime_rcr.go:runtimeTraceProjImpactFormForNode#2": "running,runnable,d_sleep,d_state,io_wait,uninterruptible_sleep",
	// DSTATE-REFINE arm c (件③): the D-family tail-redundancy predicate —
	// the explicit default arm keeps every non-D/IO state on its
	// informative tail (96728 E14/E16 witness fork lives in the D family).
	"answer_document_mutation_runtime_typelabels.go:runtimeTraceProjDFamilyTailRedundant#1": "d_sleep,d_state,io_wait,uninterruptible_sleep|default",
	// GAP-B G7 (§27.3, 2026-07-09): the 词值同源 window-lane word switch — the
	// two inversion component states only (fall-through declared above).
	"answer_document_mutation_runtime_rcr.go:runtimeTraceProjInversionStateCompositionWord#1": "running,runnable",
	"answer_document_mutation_runtime_tree.go:runtimeTraceProjStateKindLabel#1":               "running,runnable,sleep,s_sleep,sleep_wait,d_sleep,d_state,io_wait,uninterruptible_sleep",
	"answer_document_mutation_runtime_tree.go:runtimeTraceProjWaitFamilyStateKind#1":          "sleep,s_sleep,sleep_wait,d_sleep,d_state,io_wait,uninterruptible_sleep",
	"answer_document_mutation_runtime_tree.go:runtimeTraceProjSymptomFamilyStateKind#1":       "runnable+alias:runnable_wait",
}

func TestRuntimeStateKindSwitchConsumerCoverage(t *testing.T) {
	scan, err := types.ScanTraceStateKindConsumers(runtimeStateKindPinFiles, runtimeStateKindClassifierFuncs, runtimeStateKindConsumerAliasLedger)
	if err != nil {
		t.Fatal(err)
	}
	for _, issue := range scan.Issues {
		t.Error(issue)
	}
	if len(scan.Sites) == 0 {
		t.Fatal("no StateKind consumer switches matched — the pin is checking nothing; update the scan alongside the refactor")
	}
	if scan.Comparisons == 0 {
		t.Fatal("no StateKind comparison literals matched — the pin is checking nothing; update the scan alongside the refactor")
	}
	for _, issue := range types.CheckTraceStateKindConsumerRules(scan, runtimeStateKindSwitchSiteGolden, runtimeStateKindSwitchFallthroughLedger, runtimeStateKindConsumerAliasLedger) {
		t.Error(issue)
	}
}

// TestRuntimeStateKindDominantLaneBridge is rule 3: the producer-side
// dominant_state note values (tracequery dominant lanes) and the display-side
// word universe must stay one set — in BOTH directions.
func TestRuntimeStateKindDominantLaneBridge(t *testing.T) {
	lanes := tracequery.ThreadStateDominantLaneUniverse()
	if len(lanes) == 0 {
		t.Fatal("empty dominant-lane universe — the bridge is checking nothing")
	}
	for _, lane := range lanes {
		if !types.TraceStateKindRegistered(string(lane)) {
			t.Errorf("tracequery dominant lane %q can be published as a dominant_state note but is NOT a registered state-kind word — extend types.TraceStateKindUniverse in the same change", lane)
		}
	}
	// §7.11 B-1 current reality, pinned on purpose: stopped/dead own no
	// dominant lane and no word lane. If a future ruling gives them one,
	// BOTH sides must move together — flipping only this list is red above.
	for _, word := range []string{"stopped", "dead", "unknown"} {
		if types.TraceStateKindRegistered(word) {
			t.Errorf("state-kind word %q is registered but has NO dominant-lane/Object producer — universe ⊆ producible is violated (add the producer lane in the same change)", word)
		}
	}
}
