package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The shared native selector teaching and the system skill reach the same
// finalizer request. Neither surface may revoke evidence-qualified arithmetic
// or turn a computed value into a physical/causal relationship.
func TestHMC322NativeSelectorArithmeticHandoff(t *testing.T) {
	for _, tc := range []struct {
		name, fixture string
		start, end    float64
	}{
		{"business_self", "hmosperf_marker_tree", 5, 5.012},
		{"independent_io_populations", "hmosperf_io_inflight", 1, 1.01},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, _ := hmc277MeasurementContext(t, tc.fixture, "window_stats", tc.start, tc.end)
			before := readerFieldScopePublicSnapshot(t, ctx)
			system, user := finalizerReaderFieldMessages(t, ctx)
			if !strings.Contains(user, types.RuntimeMeasurementFactDisplayTeaching) ||
				!strings.Contains(system, "prohibit unsupported sums, not evidence-qualified arithmetic") {
				t.Fatal("HARNESS: actual finalizer omitted the native selector or scoped system rule")
			}
			if strings.Contains(user, "no cross-group total or physical correspondence is implied unless the producer explicitly supplies it") {
				t.Error("ARITHMETIC_HANDOFF_CONFLICT: native selector conflates compatible derived totals with physical relations")
			}
			for _, want := range []string{
				"Derived arithmetic follows the shared scoped-arithmetic rule",
				"compatible units and proven population membership",
				"A calculation does not establish physical correspondence",
				"Fact diagrams have no relationship edges",
				"do not prove hardware saturation, execution state, thread waiting or a response cause",
			} {
				if !strings.Contains(user, want) {
					t.Errorf("ARITHMETIC_HANDOFF_BOUNDARY: missing %q", want)
				}
			}
			if readerFieldScopePublicSnapshot(t, ctx) != before {
				t.Fatal("teaching changed the native ledger, measured operands, source scope or causal projection")
			}
		})
	}
}
