package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeTraceProjDetailWakeupPointUsesTypedEdgeAuthority(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Subject:                   "app-100",
		DrilldownTarget:           "cookie-200",
		DrilldownWakeupPointKnown: true,
		DrilldownWakeupTs:         2.020,
		DrilldownWakeupLine:       15,
	}

	if got := runtimeTraceProjDetailWakeupPoint(node, true); got != "cookie-200 → app-100 @ 2.020000s（trace 行15）" {
		t.Fatalf("unexpected zh exact wakeup point: %q", got)
	}
	if got := runtimeTraceProjDetailWakeupPoint(node, false); got != "cookie-200 → app-100 @ 2.020000s (trace line 15)" {
		t.Fatalf("unexpected en exact wakeup point: %q", got)
	}

	model := runtimeTraceProjTreeModel{SelfRows: []runtimeTraceProjTreeRow{{
		Node: node, Kind: runtimeTraceProjTreeRowSelf, HasData: true,
	}}}
	detail := runtimeTraceProjDetailFullText(model, true)
	if !strings.Contains(detail, "- 直接上游唤醒点: cookie-200 → app-100 @ 2.020000s（trace 行15）") {
		t.Fatalf("lossless detail must publish the typed edge point, got:\n%s", detail)
	}
}

func TestRuntimeTraceProjDetailWakeupPointDoesNotFabricateZero(t *testing.T) {
	node := types.TraceCausalProjectionNode{
		Subject:         "app-100",
		DrilldownTarget: "cookie-200",
	}
	if got := runtimeTraceProjDetailWakeupPoint(node, true); got != "" {
		t.Fatalf("unknown point must stay absent, got %q", got)
	}

	node.DrilldownWakeupPointKnown = true
	node.DrilldownWakeupTs = 0
	node.DrilldownWakeupLine = 1
	if got := runtimeTraceProjDetailWakeupPoint(node, true); !strings.Contains(got, "@ 0.000000s") {
		t.Fatalf("known trace-zero point must remain representable, got %q", got)
	}
}
