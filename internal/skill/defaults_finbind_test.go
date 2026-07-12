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
