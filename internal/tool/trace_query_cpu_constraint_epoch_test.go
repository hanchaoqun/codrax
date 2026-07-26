package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceQueryCPUConstraintEpochHandoffKeepsVersionTailAndCausalValue(t *testing.T) {
	item := tracequery.CPUConstraintSummary{
		Thread:                   tracequery.ThreadRef{PID: 100, Comm: "app"},
		AllowedCPUsAuthority:     tracequery.CPUConstraintAllowedCPUsAuthorityKernelNextInfo,
		RestrictionProof:         tracequery.CPUConstraintRestrictionProofEpochScoped,
		RunnableWaitMs:           30,
		RestrictedRunnableWaitMs: 20,
		EpochTotal:               2,
		EpochEmitted:             2,
		EpochComplete:            true,
		RestrictionEpochCount:    2,
		Epochs: []tracequery.CPUConstraintEpoch{
			{
				Ordinal: 1, SourceAuthority: tracequery.CPUConstraintAllowedCPUsAuthorityKernelNextInfo,
				StartTs: 1, EndTs: 1.01, FieldCount: 5, Affinity: "1",
				AllowedCPUs: []int{0}, RunnableWaitMs: 10,
			},
			{
				Ordinal: 2, SourceAuthority: tracequery.CPUConstraintAllowedCPUsAuthorityKernelNextInfo,
				StartTs: 1.01, EndTs: 1.02, FieldCount: 8, Affinity: "10",
				AllowedCPUs: []int{4}, ExtensionFields: []string{"8", "9"}, RunnableWaitMs: 10,
				Load: 20, LoadKnown: true, SchedGroup: 3, SchedGroupKnown: true,
				ICESBoost: true, ICESBoostKnown: true,
			},
		},
	}
	got := traceQueryTypedCPUConstraintSummary(item)
	for _, want := range []string{
		"restricted_runnable=20.000",
		"constraint_epoch_total=2",
		"constraint_epoch_emitted=2",
		"constraint_epoch_status=complete",
		"f=5",
		"f=8",
		"tail=8,9",
		"boost=true",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed epoch handoff lost %q:\n%s", want, got)
		}
	}
}
