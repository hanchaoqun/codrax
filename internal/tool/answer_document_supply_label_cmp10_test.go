package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// CMP-10 (§7.4) display-relabel pins: the supply_pressure wire token measures
// demand-side backlog, so every DISPLAY surface names it 调度压力(需求积压) /
// "scheduling pressure (demand backlog)" via the single label helper, while
// the wire token itself and the audit-fidelity raw-type column stay
// untouched (token migration is a separate R2' adjudication).
func TestSupplyPressureDisplayLabelHelper(t *testing.T) {
	if got := runtimeTraceSupplyPressureDisplayLabel(true); got != "调度压力(需求积压)" {
		t.Fatalf("zh label = %q", got)
	}
	if got := runtimeTraceSupplyPressureDisplayLabel(false); got != "scheduling pressure (demand backlog)" {
		t.Fatalf("en label = %q", got)
	}
	// The zh tree-label map routes through the helper (single home).
	if got := runtimeTraceRootCauseTypeZHLabel("supply_pressure"); got != runtimeTraceSupplyPressureDisplayLabel(true) {
		t.Fatalf("zh map label = %q, want helper value", got)
	}
}

func TestSupplyPressureDisplayCauseLanesRelabelBothSurfaces(t *testing.T) {
	// Tree/cause display lane: BOTH surfaces relabel (the EN raw-token rule
	// is deliberately overridden for this one token).
	if got := runtimeTraceCausalProjectionDisplayCauseName("supply_pressure", true); got != "调度压力(需求积压)" {
		t.Fatalf("zh display cause = %q", got)
	}
	if got := runtimeTraceCausalProjectionDisplayCauseName("supply_pressure", false); got != "scheduling pressure (demand backlog)" {
		t.Fatalf("en display cause = %q", got)
	}
	// Narrative lane keeps the raw wire token for audit fidelity.
	if got := runtimeTraceCausalProjectionNarrativeCauseName("supply_pressure", true); got != "调度压力(需求积压)（supply_pressure）" {
		t.Fatalf("zh narrative cause = %q", got)
	}
	if got := runtimeTraceCausalProjectionNarrativeCauseName("supply_pressure", false); got != "scheduling pressure (demand backlog) (supply_pressure)" {
		t.Fatalf("en narrative cause = %q", got)
	}
	// Other tokens keep the EN raw-token rule — the relabel is a typed
	// single-token exception, not a policy change.
	if got := runtimeTraceCausalProjectionDisplayCauseName("cpu_pressure", false); got != "cpu_pressure" {
		t.Fatalf("en cpu_pressure display = %q, want raw token", got)
	}
}

func TestSupplyPressureRawTypeColumnKeepsWireToken(t *testing.T) {
	node := types.TraceCausalProjectionNode{Object: "supply_pressure", TypeToken: "supply_pressure"}
	if got := runtimeTraceCausalProjectionRawTypeToken(node); got != "supply_pressure" {
		t.Fatalf("detail-table raw type = %q, want verbatim wire token", got)
	}
}
