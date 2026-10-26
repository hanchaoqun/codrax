package agent

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// This exercises the actual public query -> NewFinalizerAgent adapter request,
// not a stand-alone prompt fragment. The adapter captures locally; no model is
// invoked and no authored answer is treated as an arithmetic oracle.
func TestHMC296CalculationScopeActualFinalizer(t *testing.T) {
	ctx, path := hmc277MeasurementContext(t, "hmosperf_io_inflight", "window_stats", 1, 1.01)
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx.AttachedHitrace = string(original)
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	var controlID string
	for _, observation := range ctx.Mutable.TurnAArtifacts().ToolResults[0].Observations {
		if observation.Predicate == "io_inflight" && strings.Contains(strings.Join(observation.RichNotes, "\n"), "endpoint_family=block_rq dev=8,0 op=R") {
			controlID = observation.ID
		}
	}
	var control types.RuntimeMeasurementTable
	for _, table := range view.RuntimeMeasurementContract.Choices() {
		if table.View == types.RuntimeMeasurementSummary && table.ObservationID == controlID {
			control = table
		}
	}
	if !control.HasNativeMeasurementFacts() || !reflect.DeepEqual(control.Rows, [][]string{{"2", "1.4", "10", "14", "4", "6"}}) {
		t.Fatalf("HARNESS: missing original IO quantities/populations: %+v", control)
	}
	before := readerFieldScopePublicSnapshot(t, ctx)
	system, user := finalizerReaderFieldMessages(t, ctx)
	if readerFieldScopePublicSnapshot(t, ctx) != before {
		t.Fatal("teaching changed observations, source scope, ledger or causal projection")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(original, after) {
		t.Fatal("teaching mutated the original trace")
	}
	// Preserve the independent native population/quantity boundary that this
	// stage already receives; the correction must not replace data with advice.
	for _, want := range []string{"request·ms", "accepted", "starts", "10"} {
		if !strings.Contains(user, want) {
			t.Errorf("native handoff omitted %q", want)
		}
	}
	for _, old := range []string{
		"if a specific number is not already present in the evidence above, omit the claim or mark it uncertain rather than producing one",
		"when the published parts are what you have, the total IS their sum",
	} {
		if strings.Contains(system, old) {
			t.Errorf("CALCULATION_SCOPE_CONFLICT: actual adapter still has blanket rule %q", old)
		}
	}
	for _, want := range []string{
		"exact, published operands",
		"source, subject, scope, units and denominator",
		"derived calculation, not a new tool result",
		"Do not recover a whole population by recounting a preview",
		"mutually exclusive, exhaustive and additive parts of the same population",
		"different layers or endpoint populations",
		"A numerical difference does not identify missing members",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("CALCULATION_SCOPE_MISSING: actual adapter omitted %q", want)
		}
	}
	// These valid, already-supported operations and their narrower guards must
	// survive. We do not grant a new cross-row duration or causal capability.
	for _, want := range []string{
		"Self-derived RATIOS stay allowed",
		"two published boundary timestamps of the SAME named object",
		"never add durations from different measured rows or different threads",
		"never borrow it as a wakeup total",
		"「wakeup往来共65次(唤醒31次+被唤醒34次)」",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("existing evidence-bound arithmetic or protection lost: %q", want)
		}
	}
}
