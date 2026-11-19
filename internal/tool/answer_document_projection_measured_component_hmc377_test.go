package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 (thread-identity lane, D3/X3): an inversion composite's attribution
// sub-rows must print a MEASURED value as 原始 and the counted value as 计入.
// The Honeybird witness row (waker 64176 of main thread 64156, dependency
// window 11343.250704..11343.297396) measured 45.378 ms ready and 1.314 ms
// running; only 35.466 ms of the ready time has a proven lower priority (the
// first 9.912 ms piece starts at the thread's creation record, which carries
// no priority), and 0.845 ms is the converted running deficit of 1.314 ms.
// Before: "runnable 原始 35.466ms → 计入 35.466ms(全额)" and
// "running 原始未发布" although the same row publishes 1.314 ms.
func hmc377HoneybirdInversionNode() types.TraceCausalProjectionNode {
	node := elimChainNode("E-inv", "Signal Catcher-64176", "priority_inversion_candidate", "runnable", 1, 36.311, 20301)
	node.PriorityInversionCandidate = true
	node.GatedRunnableMS = 35.466
	node.GatedRunningDeficitMS = 0.845
	node.RunnableMS = 45.378
	node.RunningMS = 1.314
	node.PriorityRelationProvenLowerMS = 36.780
	node.PriorityRelationUnknownOrNonLowerMS = 9.912
	return node
}

func TestHMC377InversionComponentsPrintMeasuredRawAndCountedShare(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(gatedCalProjection(hmc377HoneybirdInversionNode()),
		newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if !strings.Contains(fence, "有效归因 36.311ms = runnable(全额) 35.466ms + running(折算) 0.845ms") {
		t.Fatalf("the counted equation must stay byte-identical:\n%s", fence)
	}
	if !strings.Contains(fence, "runnable 原始 45.378ms → 计入 35.466ms(全额计入已证明优先级低于其下游等待线程的部分;其余9.912ms未证明优先级更低,不计入)") {
		t.Fatalf("the runnable sub-row must print the measured ready time and the uncounted share:\n%s", fence)
	}
	if strings.Contains(fence, "runnable 原始 35.466ms") {
		t.Fatalf("a counted value must never wear the measured word:\n%s", fence)
	}
	if !strings.Contains(fence, "running 原始 1.314ms → 计入 0.845ms(折算,按") || strings.Contains(fence, "running 原始未发布") {
		t.Fatalf("the published measured running must not read as unpublished:\n%s", fence)
	}
	modelEN := buildRuntimeTraceProjTreeModel(gatedCalProjection(hmc377HoneybirdInversionNode()),
		newRuntimeTraceCausalProjectionEvidenceIndex(), false)
	fenceEN := rspaFenceJoined(runtimeTraceProjTreeFence(modelEN, false))
	if !rspaFenceContains(fenceEN, "runnable raw 45.378ms → counted 35.466ms (in full for the share whose priority is proven lower than its downstream waiting thread; the other 9.912ms has no proven lower priority and is not counted)") ||
		!rspaFenceContains(fenceEN, "running raw 1.314ms → counted 0.845ms") {
		t.Fatalf("en face must carry the same measured/counted split:\n%s", fenceEN)
	}
}

// The same-CPU overlap row counts only the overlap with a lower-priority
// thread running on that CPU; its measured ready time is the raw value and
// the rest is named, never shown as the overlap itself.
func TestHMC377RunnableOverlapComponentNamesMeasuredReadyAndOverlap(t *testing.T) {
	node := elimChainNode("E-ovl", "Signal Catcher-64176", "priority_inversion_runnable_wait", "runnable", 1, 8.408, 20301)
	node.GatedRunnableMS = 8.408
	node.RunnableMS = 45.878
	node.PriorityRelationProvenLowerMS = 8.408
	node.PriorityRelationUnknownOrNonLowerMS = 37.470
	model := buildRuntimeTraceProjTreeModel(gatedCalProjection(node), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := rspaFenceJoined(runtimeTraceProjTreeFence(model, true))
	if strings.Contains(fence, "runnable 原始 8.408ms") {
		t.Fatalf("the overlap estimate must not read as the measured ready time:\n%s", fence)
	}
	if strings.Contains(fence, "原始 45.878ms") && !strings.Contains(fence, "其余37.470ms同核没有更低优先级线程在运行,不计入") {
		t.Fatalf("a measured ready time beside the overlap must name the uncounted share:\n%s", fence)
	}
}

// Negative: rows whose measured and counted runnable agree, merged display
// nodes, and rows that publish no measured account keep the legacy grammar.
func TestHMC377InversionComponentsKeepLegacyWhenNoMeasuredSurplus(t *testing.T) {
	equal := hmc377HoneybirdInversionNode()
	equal.RunnableMS = equal.GatedRunnableMS
	merged := hmc377HoneybirdInversionNode()
	merged.MergedCount = 2
	unpublished := gatedCalCompositeNode(1)
	for name, node := range map[string]types.TraceCausalProjectionNode{"equal": equal, "merged": merged, "unpublished": unpublished} {
		measured, _, ok := runtimeTraceProjInversionMeasuredRunnable(node)
		if ok || measured != 0 {
			t.Fatalf("%s: no measured surplus may be claimed: %v", name, measured)
		}
	}
	if got := runtimeTraceProjInversionMeasuredQualifiedRunning(merged); got != 0 {
		t.Fatalf("merged node must not claim a qualified running raw: %v", got)
	}
	if got := runtimeTraceProjInversionMeasuredQualifiedRunning(unpublished); got != 0 {
		t.Fatalf("a row without the proven-lower note keeps 原始未发布: %v", got)
	}
}
