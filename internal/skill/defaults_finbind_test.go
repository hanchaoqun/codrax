package skill

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// defaults_finbind_test.go — CR-1 件④ FIN-BIND directive-group pins
// (§29.42.2② 教学先行 + §29.40.1 并存披露,
// docs/design/real_trace_campaign_20260705.md, 2026-07-12). Five
// trace-gated prose disciplines: (a) measurement-subject binding, (b)
// root-cause board order with the conscious-flip disclosure lane, (c) no
// cross-row duration sums, (d) channel words per row, (e) inversion×lock
// coexistence. All soft teaching — none of them lives in the
// always-rendered workflow, and none introduces a reject path.

func finBindTierBItem(t *testing.T, header string) TierBItem {
	t.Helper()
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("Get(answer-document-skill): %v", err)
	}
	if always := strings.Join(sk.Workflow, "\n"); strings.Contains(always, header) {
		t.Fatalf("%s must not live in the always-rendered workflow", header)
	}
	for _, item := range sk.WorkflowTierB {
		// Prefix match — bodies may cross-reference each other's headers
		// (SST: rules are referenced by their header names).
		if strings.HasPrefix(item.Body, header+":") {
			return item
		}
	}
	t.Fatalf("answer-document-skill WorkflowTierB missing %q", header)
	return TierBItem{}
}

func TestFinBindMeasurementSubjectBinding(t *testing.T) {
	item := finBindTierBItem(t, "MEASUREMENT-SUBJECT BINDING")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("subject-binding rule must be trace-gated: %+v", item.AppliesTo)
	}
	for _, want := range []string{
		"SAME thread or entity its measured evidence row publishes it for",
		"quoting a nearby value that was published for a DIFFERENT thread",
		"either name that owner explicitly beside the number or leave the number out",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("subject-binding rule missing %q:\n%s", want, item.Body)
		}
	}
}

func TestFinBindRootCauseBoardOrder(t *testing.T) {
	item := finBindTierBItem(t, "ROOT-CAUSE BOARD ORDER")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("board-order rule must be trace-gated: %+v", item.AppliesTo)
	}
	for _, want := range []string{
		"state the ranked causes in THAT order",
		"say explicitly that it differs from the measured ordering",
		"never reorder silently",
		"never present two different \"number one\" causes in one answer",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("board-order rule missing %q:\n%s", want, item.Body)
		}
	}
	found := false
	for _, kind := range item.OnViolation {
		if kind == types.ViolProseLexiconBoardInconsistent {
			found = true
		}
	}
	if !found {
		t.Fatalf("board-order rule must re-render when its lane's kind fired: %+v", item.OnViolation)
	}
}

func TestFinBindNoCrossRowDurationSums(t *testing.T) {
	item := finBindTierBItem(t, "NO CROSS-ROW DURATION SUMS")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("no-sum rule must be trace-gated: %+v", item.AppliesTo)
	}
	for _, want := range []string{
		"never add durations from different measured rows or different threads into a new total",
		"a self-made aggregate fabricates time",
		"must still be one published value, not your own sum",
		"Self-derived RATIOS stay allowed",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("no-sum rule missing %q:\n%s", want, item.Body)
		}
	}
	// R7 sync half: the core-numbers clause no longer licenses self-made
	// sums — its derivation license covers ratios only and defers duration
	// totals to this rule.
	core := finBindTierBItem(t, "WINDOW-STATS CORE NUMBERS")
	if strings.Contains(core.Body, "a ratio or a sum yourself") {
		t.Fatalf("core-numbers clause must not re-license self-made sums:\n%s", core.Body)
	}
	if !strings.Contains(core.Body, "governed by the NO CROSS-ROW DURATION SUMS rule") {
		t.Fatalf("core-numbers clause must defer duration totals to the no-sum rule:\n%s", core.Body)
	}
	// R7 sync half two (复核 P2-3, 2026-07-12): the PROSE NUMBER GROUNDING
	// derivation license likewise covers ratios/normalizations only and
	// defers duration totals to this rule.
	png := finBindTierBItem(t, "PROSE NUMBER GROUNDING")
	if strings.Contains(png.Body, "(a sum, a ratio, a normalization)") {
		t.Fatalf("prose-number-grounding clause must not re-license self-made sums:\n%s", png.Body)
	}
	if !strings.Contains(png.Body, "governed by the NO CROSS-ROW DURATION SUMS rule") {
		t.Fatalf("prose-number-grounding clause must defer duration totals to the no-sum rule:\n%s", png.Body)
	}
}

func TestFinBindChannelWordsPerRow(t *testing.T) {
	item := finBindTierBItem(t, "CHANNEL WORDS PER ROW")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("channel-words rule must be trace-gated: %+v", item.AppliesTo)
	}
	for _, want := range []string{
		"never narrate it as background noise",
		"never promote it into the direct chain",
		"the published channel wins",
		"say so explicitly as your own assessment",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("channel-words rule missing %q:\n%s", want, item.Body)
		}
	}
}

func TestFinBindInversionLockCoexistence(t *testing.T) {
	item := finBindTierBItem(t, "PRIORITY-INVERSION AND LOCK-HOLD COEXISTENCE")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("coexistence rule must be trace-gated: %+v", item.AppliesTo)
	}
	for _, want := range []string{
		"state both facts",
		"they coexist; neither cancels the other",
		"priority inheritance or through decoupling the lock",
		"optimization / next-step surface",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("coexistence rule missing %q:\n%s", want, item.Body)
		}
	}
}

// --- CR-3 件⑦ extension (2026-07-12): three more trace-gated disciplines ---

// TestFinBindIOLatencyRoleWords — CR-3 件⑦a (CAL-1 冷读 F-9): the
// initiator's blocked segment and the request's device-side latency are
// different roles with different values.
func TestFinBindIOLatencyRoleWords(t *testing.T) {
	item := finBindTierBItem(t, "IO-LATENCY ROLE WORDS")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("IO-role rule must be trace-gated: %+v", item.AppliesTo)
	}
	for _, want := range []string{
		"the REQUEST's own latency",
		"DIFFERENT measurement with its own value",
		"Never restate one side's value as the other's",
		"say which side it is",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("IO-role rule missing %q:\n%s", want, item.Body)
		}
	}
}

// TestFinBindStateDurationCaliberSeparation — CR-3 件⑦b (P6 教学半场): the
// per-thread state partition respects the window; activity-slice and
// full-window calibers never mix in one breakdown.
func TestFinBindStateDurationCaliberSeparation(t *testing.T) {
	item := finBindTierBItem(t, "STATE-DURATION CALIBER SEPARATION")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("caliber-separation rule must be trace-gated: %+v", item.AppliesTo)
	}
	for _, want := range []string{
		"can never exceed the analysis window",
		"DIFFERENT calibers",
		"never mix calibers inside one per-thread breakdown",
		"sanity-check that the durations can fit the window together",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("caliber-separation rule missing %q:\n%s", want, item.Body)
		}
	}
}

// TestFinBindNoSilentSourceFallback — CR-3 件⑦c (§29.47.7): the empty
// trace result discloses first; the narrowed criteria (both typed
// conditions) are spoken verbatim, and mixed analysis stays untouched.
func TestFinBindNoSilentSourceFallback(t *testing.T) {
	item := finBindTierBItem(t, "NO SILENT SOURCE FALLBACK ON AN EMPTY TRACE RESULT")
	if !item.AppliesTo.RequiresTrace {
		t.Fatalf("source-fallback rule must be trace-gated: %+v", item.AppliesTo)
	}
	for _, want := range []string{
		// condition (a): trace-led question ∧ zero root-cause findings.
		"a runtime trace is attached and the question asks about that trace",
		"produced ZERO root-cause findings",
		// condition (b): principal claims rest on source citations alone.
		"rest on source citations alone",
		// disclosure-first duty + the non-degradation of mixed analysis.
		"disclose that FIRST",
		"An empty trace result is not a source-code question",
		"Mixed analysis stays normal",
	} {
		if !strings.Contains(item.Body, want) {
			t.Fatalf("source-fallback rule missing %q:\n%s", want, item.Body)
		}
	}
	if len(item.OnViolation) != 0 {
		t.Fatalf("the source-fallback rule is disclosure teaching — no violation lane: %+v", item.OnViolation)
	}
}
