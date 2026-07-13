package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// 76684 行1 形态词回退修 pin (SMR-1 批 coordinator witness, 2026-07-12):
// `ThreadPoolForeg-60555 █ 13.418ms 12% 对端线程未解析 · [E9(+6)]` rendered
// 行1 with a BARE name (state word iowait demoted to 行2) — 违 PTV4 行1
// 三要素/零省略. 96728 对照形 kept `ThreadPoolForeg-60555 · iowait` in 行1.
// Post-fix: the generic-unresolved shape names 「主体 · <state>」 (状态词永在
// 行1); 「对端线程未解析」 rides its own demotable tag; the typed-kind forms
// (D-state/iowait(对端未解析)) stay byte-identical.

func smr1Row1GenericUnresolvedProjection() types.TraceCausalProjection {
	return types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "com.baidu.tieba-59566"},
		WindowStartTs: 34579.472865,
		WindowEndTs:   34579.587805,
		OnChainCauses: []types.TraceCausalProjectionNode{
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "hop",
				Subject: "waker-1", Predicate: "wakeup_causal_impact", Object: "s_sleep",
				StateKind: "s_sleep", ChainRelevance: "on_chain", ChainDepth: 1,
				ImpactMS: 20.0, CumulativeImpactMS: 20.0,
				Confidence: 0.8, LineStart: 50, LineEnd: 60},
			// The 76684 witness row: generic unresolved peer + iowait state.
			{Role: types.TraceCausalRoleCausalHop, EvidenceID: "e9",
				Subject: "ThreadPoolForeg-60555", Predicate: "critical_blocking",
				Object: "unknown-thread", StateKind: "io_wait",
				ChainRelevance: "on_chain",
				ImpactMS:       13.418, CumulativeImpactMS: 13.418,
				MergedCount: 3, MergedMinMS: 4.265, MergedMaxMS: 4.884,
				Confidence: 0.8, LineStart: 8712, LineEnd: 15131},
		},
	}
}

func TestSMR1Row1GenericUnresolvedKeepsStateWord(t *testing.T) {
	model := buildRuntimeTraceProjTreeModel(smr1Row1GenericUnresolvedProjection(), newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	found := false
	for _, line := range strings.Split(fence, "\n") {
		if !strings.Contains(line, "13.418ms") {
			continue
		}
		found = true
		if !strings.Contains(line, "ThreadPoolForeg-60555 · iowait") {
			t.Fatalf("行1 三要素: the state word must ride 行1 (96728 对照形):\n%s\nfence:\n%s", line, fence)
		}
	}
	if !found {
		t.Fatalf("witness row missing:\n%s", fence)
	}
	if !strings.Contains(fence, "对端线程未解析") {
		t.Fatalf("零省略: the unresolved-peer disclosure must stay on the row faces:\n%s", fence)
	}
	// The state word never double-speaks (name + tag on one row).
	for _, line := range strings.Split(fence, "\n") {
		if strings.Count(line, "iowait") > 1 {
			t.Fatalf("state word must appear once per line (双说灭):\n%s", line)
		}
	}
}

// 同病兄弟形 control: the typed-kind unresolved form keeps its 行1 word
// byte-identically (D-state/iowait(对端未解析) already speaks the family).
func TestSMR1Row1TypedKindUnresolvedUntouched(t *testing.T) {
	projection := smr1Row1GenericUnresolvedProjection()
	projection.OnChainCauses[1].TypeToken = "d_state_or_io_wait"
	projection.OnChainCauses[1].StateKind = "d_sleep"
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	if !strings.Contains(fence, "D-state/iowait(对端未解析)") {
		t.Fatalf("the typed-kind form keeps its 行1 family word:\n%s", fence)
	}
}

// 2609 复放形 control: the state parked on the TYPE lane (StateKind empty,
// TypeToken io_wait) takes the same 行1 word.
func TestSMR1Row1TypeLaneStateWord(t *testing.T) {
	projection := smr1Row1GenericUnresolvedProjection()
	projection.OnChainCauses[1].StateKind = ""
	projection.OnChainCauses[1].TypeToken = "io_wait"
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	fence := runtimeTraceProjTreeFence(model, true)
	for _, line := range strings.Split(fence, "\n") {
		if strings.Contains(line, "13.418ms") && !strings.Contains(line, "ThreadPoolForeg-60555 · iowait") {
			t.Fatalf("行1 三要素 (type-lane form):\n%s\nfence:\n%s", line, fence)
		}
	}
}
