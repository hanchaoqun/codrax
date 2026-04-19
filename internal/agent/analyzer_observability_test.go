package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestRecordReconcileObservation_DropsNilMutable proves the helper is
// safe to call on the unit-test bare BusContext (no Mutable).
// Without this, every reconcile call site would have to guard the
// Mutable check itself.
func TestRecordReconcileObservation_DropsNilMutable(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("recordReconcileObservation panicked on nil mutable: %v", r)
		}
	}()
	recordReconcileObservation(nil, types.ReconcileObservation{
		Field: "intent", Before: "explain", After: "explain",
	})
}

// TestRecordReconcileObservation_DropsEmptyField protects against the
// caller forgetting to fill Field — without this guard the per-Run
// summary would emit "(empty field)=N" entries which are noise.
func TestRecordReconcileObservation_DropsEmptyField(t *testing.T) {
	mu := types.NewMutableState("anything")
	recordReconcileObservation(mu, types.ReconcileObservation{
		Field: "", Before: "x", After: "y",
	})
	if got := mu.ReconcileObservations(); len(got) != 0 {
		t.Errorf("empty field should drop, got %v", got)
	}
}

// TestFormatReconcileSummary_QuietRun emits the "no event" line so
// the absence of any [reconcile-shadow] log is itself a signal.
func TestFormatReconcileSummary_QuietRun(t *testing.T) {
	got := formatReconcileSummary(nil)
	if !strings.Contains(got, "no reconcile event") {
		t.Errorf("expected quiet-run line, got %q", got)
	}
	if !strings.HasPrefix(got, "[reconcile-shadow] summary:") {
		t.Errorf("missing prefix, got %q", got)
	}
}

// TestFormatReconcileSummary_OverrideAndLowConf renders the per-field
// counters in stable order with override and low-confidence buckets.
func TestFormatReconcileSummary_OverrideAndLowConf(t *testing.T) {
	events := []types.ReconcileObservation{
		// 2 intent events: 1 override + 1 noop, both high confidence.
		reconcileEvent("intent", "enumerate", "return_value", 0.8, "count_verb", types.SemanticPredicates{IsCountQuestion: true}),
		reconcileEvent("intent", "explain", "explain", 0.9, "", types.SemanticPredicates{}),
		// 1 shape event, override + low confidence.
		reconcileEvent("shape", "config_value", "value", 0.3, "subject_kind", types.SemanticPredicates{}),
		// 1 axis event, no change (LLM and reconcile agree on "call").
		reconcileEvent("axis", "call", "call", 0, "", types.SemanticPredicates{}),
	}
	got := formatReconcileSummary(events)
	if !strings.Contains(got, "total=4") {
		t.Errorf("total counter wrong, got %q", got)
	}
	if !strings.Contains(got, "overrides=2") {
		t.Errorf("overrides counter wrong, got %q", got)
	}
	if !strings.Contains(got, "low_conf=1") {
		t.Errorf("low_conf counter wrong, got %q", got)
	}
	if !strings.Contains(got, "intent=2(override=1)") {
		t.Errorf("intent per-field rendering wrong, got %q", got)
	}
	if !strings.Contains(got, "shape=1(override=1)") {
		t.Errorf("shape per-field rendering wrong, got %q", got)
	}
	if !strings.Contains(got, "axis=1") {
		t.Errorf("axis missing, got %q", got)
	}
	if strings.Contains(got, "axis=1(override") {
		t.Errorf("axis should render without override marker, got %q", got)
	}
	// Stable order: intent must precede shape must precede axis.
	idxIntent := strings.Index(got, "intent=")
	idxShape := strings.Index(got, "shape=")
	idxAxis := strings.Index(got, "axis=")
	if !(idxIntent > 0 && idxIntent < idxShape && idxShape < idxAxis) {
		t.Errorf("unstable field order: intent=%d shape=%d axis=%d in %q", idxIntent, idxShape, idxAxis, got)
	}
}

// TestReconcileEvent_DefaultsRule normalises the empty rule string to
// "none" so the per-Run summary can grep on RuleFired without a bare
// empty string ambiguity.
func TestReconcileEvent_DefaultsRule(t *testing.T) {
	ev := reconcileEvent("intent", "a", "b", 0.5, "", types.SemanticPredicates{})
	if ev.RuleFired != "none" {
		t.Errorf("empty rule should default to 'none', got %q", ev.RuleFired)
	}
}

// TestRecordReconcileObservation_ThreadsToReadback validates the full
// write/read round-trip through MutableState so a reconcile site that
// records an event can trust ReconcileObservations() to surface it.
func TestRecordReconcileObservation_ThreadsToReadback(t *testing.T) {
	mu := types.NewMutableState("anything")
	recordReconcileObservation(mu, reconcileEvent(
		"intent", "enumerate", "return_value", 0.8,
		"count_verb",
		types.SemanticPredicates{IsCountQuestion: true},
	))
	got := mu.ReconcileObservations()
	if len(got) != 1 {
		t.Fatalf("want 1 observation, got %d", len(got))
	}
	if got[0].Field != "intent" || got[0].Before != "enumerate" || got[0].After != "return_value" {
		t.Errorf("event content not preserved, got %+v", got[0])
	}
	if !got[0].Predicates.IsCountQuestion {
		t.Errorf("predicates snapshot lost, got %+v", got[0].Predicates)
	}
}
