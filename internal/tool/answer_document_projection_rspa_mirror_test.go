package tool

// answer_document_projection_rspa_mirror_test.go — 修复轮 (RSPA §29.61.10,
// 2026-07-14) production-shape pins on the donghu witness render:
//
//	件1  EVOLUTION RECORD (ONCHAIN-FIX-2 件2, Q5 已追认 2026-07-18): the
//	     original pin froze the full-window MIRROR row (critical_blocking
//	     lane face, 36.757 undecomposed) speaking 「同段镜像·全窗账=[⛓]+[◇]
//	     二分席之和」 with back-pointers on both halves. That undecomposed
//	     keep-⛓ face existed ONLY on the direct critical_blocking entrance,
//	     which swept anchor-less and skipped the HULL-CRED four-arm verdict
//	     — the two-entrance judgment fork Q5 retired. The direct entrance
//	     now runs the SAME chain-first verdict machine as the bundle: the
//	     CompThread D/IO view faces adjudicate per row (the cpu=1 group
//	     keeps ⛓ on its per-segment credential; the segment-disjoint /
//	     zero-credential groups demote to ◇ wearing the R4 word), so the
//	     single undecomposed 36.757 face — the mirror sentence's carrier —
//	     is structurally gone. The 二分还原 information is fully preserved
//	     on the rank pair's 同源二分对席 sentences (E13/E35 form). This pin
//	     now freezes the CONVERGED shape and the mirror sentence's absence.
//	件3  the re-anchored JankManager row's coverage tag reads the census
//	     full account (31.191, never the capped 16.687 donor) and speaks
//	     锚定合计 (1.759 is an anchored Σ, not a single largest fragment);
//	D3   decomposed rows' roster members wear 成员(全窗账);
//	D1   regression guard: the ◇ halves render at all (the classified dedupe
//	     once swallowed them behind their ⛓ siblings — the bipartition pair
//	     below is unresolvable without them).

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRSPAMirrorRowRelationOnDonghuWitness(t *testing.T) {
	idx, err := tracequery.BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatal(err)
	}
	q := tracequery.Query{PID: 17267, TimeStart: 13762.791708, TimeEnd: 13763.024898,
		TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace}
	var records []types.ObservationRecord
	for i, view := range []string{"window_stats", "root_cause_rank", "critical_blocking_calls", "thread_timeline"} {
		vq := q
		vq.View = view
		result := tracequery.Run(idx, vq)
		records = append(records, traceQueryTypedObservations(result, "donghu.ftrace",
			fmt.Sprintf("payload-%d", i), "raw-ref", "", time.Unix(1751600000, 0).UTC())...)
	}
	set := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: records})
	if len(set.Projections) == 0 {
		t.Fatal("no projection")
	}
	model := buildRuntimeTraceProjTreeModel(set.Projections[0], newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	// D1 guard: both ◇ remainder halves render.
	for _, want := range []string{"其余33.159ms(无链上凭证)", "其余29.432ms(无链上凭证)"} {
		if !strings.Contains(fence, want) {
			t.Fatalf("D1: ◇ remainder disclosure %q missing:\n%s", want, fence)
		}
	}
	// 件1 (EVOLUTION, ONCHAIN-FIX-2 件2): the undecomposed fake-⛓ mirror
	// face is retired — the direct entrance adjudicates the D/IO view faces
	// per row like the bundle, so neither the mirror sentence nor its
	// back-pointers can mint (their carrier row no longer exists).
	if got := strings.Count(fence, "同段镜像·全窗账=["); got != 0 {
		t.Fatalf("件1 evolution: the undecomposed mirror-row sentence must be retired, got %d:\n%s", got, fence)
	}
	if got := strings.Count(fence, "全窗账镜像行 ["); got != 0 {
		t.Fatalf("件1 evolution: the mirror back-pointers must be retired with their carrier, got %d:\n%s", got, fence)
	}
	// 件1 (converged shape): the per-row verdicts stand in — the CompThread
	// cpu=1 D-state view face keeps ⛓ on its per-segment credential while
	// its sibling groups ride ◇ wearing the R4 word (值零动,通道位归位).
	if !strings.Contains(fence, "⛓ CompThread_0-2955 · D-state") {
		t.Fatalf("件1 converged: the segment-verified CompThread D-state view face must keep ⛓:\n%s", fence)
	}
	if !strings.Contains(fence, "D-state(对端未解析) · 3次(3.774~16.064ms)") {
		t.Fatalf("件1 converged: the demoted CompThread D-state view faces must render on ◇:\n%s", fence)
	}
	// 件3: the coverage tag reads the census full account + the anchored-Σ
	// word; the capped 16.687 donor form is gone.
	if !strings.Contains(fence, "窗内 runnable 合计 31.191ms,链上锚定合计 1.759ms(6%)") {
		t.Fatalf("件3: census-basis anchored coverage tag missing:\n%s", fence)
	}
	if strings.Contains(fence, "合计 16.687ms,链上仅覆盖其中最大片段") {
		t.Fatalf("件3: capped donor coverage wording resurfaced:\n%s", fence)
	}
	// D3: decomposed rows' roster members wear the full-window qualifier.
	if !strings.Contains(fence, "成员(全窗账) ") {
		t.Fatalf("D3: 成员(全窗账) qualifier missing:\n%s", fence)
	}
}
