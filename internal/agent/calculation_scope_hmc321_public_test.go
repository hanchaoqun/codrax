package agent

import (
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the same system skill plus dynamic native handoff sent by the
// public finalizer. Capture stops locally before any model response; prose
// arithmetic is neither executed nor promoted into a native measurement.
func TestHMC321ScopedArithmeticActualFinalizer(t *testing.T) {
	for _, tc := range []struct {
		name, fixture string
		start, end    float64
	}{
		{"business_self", "hmosperf_marker_tree", 5, 5.012},
		{"io_populations", "hmosperf_io_inflight", 1, 1.01},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, path := hmc277MeasurementContext(t, tc.fixture, "window_stats", tc.start, tc.end)
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			ctx.AttachedHitrace = string(original)
			before := readerFieldScopePublicSnapshot(t, ctx)
			view := types.BuildAnswerSemanticViewForAgentContext(ctx)
			system, user := finalizerReaderFieldMessages(t, ctx)
			for _, heading := range []string{"WINDOW-STATS CORE NUMBERS:", "PROSE NUMBER GROUNDING:", "NO CROSS-ROW DURATION SUMS:"} {
				if strings.Count(system, heading) != 1 {
					t.Fatalf("HARNESS: actual system did not publish %q exactly once", heading)
				}
			}
			if tc.name == "business_self" {
				count := 0
				for _, row := range answerDocObservationLedger(ctx).Records {
					if fact, ok := tool.DecodeTraceBusinessTreeFact(row); ok {
						count++
						if !strings.Contains(user, tool.TraceBusinessTreeNodeText(fact.Node)) {
							t.Fatalf("HARNESS: actual finalizer lost native self/state values for %s", row.ID)
						}
					}
				}
				if count != 4 || !strings.Contains(user, "互斥自身分段及其同状态交集可合计") {
					t.Fatalf("HARNESS: business self permission/account missing: records=%d", count)
				}
			} else {
				matched := false
				for _, table := range view.RuntimeMeasurementContract.Choices() {
					if table.View == types.RuntimeMeasurementSummary && table.HasNativeMeasurementFacts() &&
						reflect.DeepEqual(table.Rows, [][]string{{"2", "1.4", "10", "14", "4", "6"}}) {
						matched = true
					}
				}
				if !matched || !strings.Contains(user, "request·ms") {
					t.Fatal("HARNESS: independent IO populations, exact values or request-time unit missing")
				}
			}
			for _, old := range []string{
				"never add durations from different rows into a new figure",
				"never add durations from different measured rows or different threads into a new total of your own",
				"duration totals do not",
				"when comparing across windows, normalize each side by its own window length first",
			} {
				if strings.Contains(system, old) {
					t.Errorf("SCOPED_ARITHMETIC_CONFLICT: actual system contradicts native quantity permission: %q", old)
				}
			}
			for _, want := range []string{
				"prohibit unsupported sums, not evidence-qualified arithmetic",
				"source and generation, subject, time domain and window endpoints, unit and measured population",
				"the evidence proves they are mutually disjoint and additive",
				"business self segments and their same-state intersections",
				"only an evidenced exhaustive partition licenses a whole-population total",
				"same-measure subtraction across proven nested scopes",
				"Never add durations across threads as one wall-clock interval",
				"top N root-cause rank seats",
				"licenses only the listed members of that same board and direction",
				"sampling weights are not milliseconds",
				"A calculation creates no new dependency edge, on-chain attribution, completion claim or root-cause authority",
			} {
				if !strings.Contains(system, want) {
					t.Errorf("SCOPED_ARITHMETIC_BOUNDARY: actual system missing %q", want)
				}
			}
			if readerFieldScopePublicSnapshot(t, ctx) != before {
				t.Fatal("teaching changed native operands, query/source scope, observations or causal projection")
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(original, after) {
				t.Fatal("prompt capture modified the original trace")
			}
		})
	}
}
