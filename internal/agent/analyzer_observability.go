package agent

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
)

// ctxMutable extracts the MutableState pointer from an AgentContext,
// returning nil when the context itself is nil. Used by reconcile
// callers to pass observability targets through one helper instead of
// repeating the nil check at each call site.
func ctxMutable(ctx *types.AgentContext) *types.MutableState {
	if ctx == nil {
		return nil
	}
	return ctx.Mutable
}

// recordReconcileObservation funnels every reconcile/inference decision
// onto a single observability channel. Each reconcile site (intent /
// complexity / shape / subject / axis) calls this so the per-Run
// summary in EmitReconcileSummary can surface confidence distribution
// and rule firing rate without each call site reaching into Mutable
// directly.
//
// The function is intentionally lenient on missing inputs: a nil
// MutableState (unit tests with a zero-value BusContext) silently
// drops the event so reconcile callers don't have to guard each call.
func recordReconcileObservation(mu *types.MutableState, ev types.ReconcileObservation) {
	if mu == nil {
		return
	}
	mu.AppendReconcileObservation(ev)
}

// formatReconcileSummary builds the summary line as a pure string so
// tests can assert content without coupling to the logger sink. The
// log-emitting wrapper EmitReconcileSummary delegates here.
func formatReconcileSummary(events []types.ReconcileObservation) string {
	if len(events) == 0 {
		return "[reconcile-shadow] summary: no reconcile event recorded (clean LLM classification)"
	}
	total := len(events)
	overrides := 0
	lowConf := 0
	perField := make(map[string]int, 5)
	overridesPerField := make(map[string]int, 5)
	for _, ev := range events {
		perField[ev.Field]++
		if ev.Before != ev.After {
			overrides++
			overridesPerField[ev.Field]++
		}
		if ev.Confidence > 0 && ev.Confidence < 0.5 {
			lowConf++
		}
	}
	fieldOrder := []string{"intent", "complexity", "shape", "subject", "axis"}
	var fieldParts []string
	for _, f := range fieldOrder {
		if perField[f] == 0 {
			continue
		}
		if overridesPerField[f] > 0 {
			fieldParts = append(fieldParts, fmt.Sprintf("%s=%d(override=%d)", f, perField[f], overridesPerField[f]))
		} else {
			fieldParts = append(fieldParts, fmt.Sprintf("%s=%d", f, perField[f]))
		}
	}
	return fmt.Sprintf(
		"[reconcile-shadow] summary: total=%d overrides=%d low_conf=%d %s",
		total, overrides, lowConf, strings.Join(fieldParts, " "),
	)
}

// EmitReconcileSummary renders a one-line operator-grep summary of
// the reconcile events recorded during this analyze dispatch. Mirrors
// the orchestrator's [CGEC] summary line so the same trace consumer
// can surface analyzer + closure activity in one search. Always
// emits — even on a quiet run where no reconcile rule fired — so the
// absence of the line is itself diagnostic ("analyzer never recorded
// observability — Mutable threading is broken").
//
// Counters surfaced:
//   - total: every recorded event
//   - overrides: events where Before != After (a rule replaced the
//     LLM's pick)
//   - low_conf: events whose Confidence < 0.5 (LLM was guessing)
//   - per-field: short field=N counts so a spike on a single field
//     is grep-able without parsing the full event slice
func EmitReconcileSummary(mu *types.MutableState) {
	if mu == nil {
		return
	}
	logging.Info("%s", formatReconcileSummary(mu.ReconcileObservations()))
}

// reconcileEvent is the internal helper that callers in the reconcile
// chain use to construct a ReconcileObservation. Keeps the field-name
// strings in one place (callers pass the constant) so a typo is one
// edit away from being grep-able.
func reconcileEvent(field, before, after string, conf float64, rule string, preds types.SemanticPredicates) types.ReconcileObservation {
	if rule == "" {
		rule = "none"
	}
	return types.ReconcileObservation{
		Field:      field,
		Before:     before,
		After:      after,
		Confidence: conf,
		RuleFired:  rule,
		Predicates: preds,
	}
}
