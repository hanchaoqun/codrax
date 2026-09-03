package tracefinding

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/types"
)

// SIDECAR-EVID-1 (customer report 2026-09-02 → §40.32): the public evidence
// is rendered from the system-owned typed facts as customer-readable
// sentences — quantified value, chain relation + credential, mechanism +
// boundary, trace locator — and never publishes internal artifact paths or
// trace_query result ids (the customer cannot open them).
func TestBoundRootCauseEvidenceRendersTypedFactsWithoutInternalReferences(t *testing.T) {
	candidate := packingRootCauseCandidate([]string{
		"/workspace/.codrax/blob/20260902-005125-000-78634/attached_trace.txt:2892-13060",
		"trace_query:trace-query-result-28bd29e5.json#root_cause_rank:1",
	})
	candidate.Decision.CausalQualifier = types.TraceCausalQualifierFrameUnproven
	candidate.Decision.EvidenceFacts = &types.TraceCauseEvidenceFacts{
		WindowStartTs: 34579.472865, WindowEndTs: 34579.587805, SeatStartTs: 34579.4801, SeatEndTs: 34579.5210,
		LineStart: 2892, LineEnd: 13060, TargetSubject: "com.baidu.tieba-59566",
		ChainRelevance: "on_chain", Causality: "on_wakeup_chain", ChainDepth: 1, ChainBranch: 2,
		WakeupPath: []string{"worker-9", "com.baidu.tieba-59566"}, StateKind: "running",
		Lane: "compute_delivery", FixDirection: "scheduling_supply",
	}
	before := append([]string(nil), candidate.Decision.EvidenceRefs...)
	report, err := BindRootCauseReportSelection(&types.TraceRootCauseReportV2{SchemaVersion: 2,
		RootCauses: []*types.TraceRootCauseItemV2{{CandidateID: candidate.Decision.CandidateID}}},
		&types.TraceFindingContract{RootCauseReportEnabled: true, Candidates: []types.TraceFindingCandidateV1{candidate}})
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	item := report.RootCauses[0]
	if len(item.Evidence) != 4 {
		t.Fatalf("four typed sentences expected: %+v", item.Evidence)
	}
	joined := strings.Join(item.Evidence, "\n")
	for _, want := range []string{
		"worker-9 在目标窗口内的链上有效归因为 58.320 ms", RootCauseValueDescription(candidate.Decision),
		"链路关系：位于目标 com.baidu.tieba-59566 唤醒依赖链第 1 级（分支 2）", "凭证=唤醒链成员", "唤醒链：worker-9 → com.baidu.tieba-59566",
		"机理与边界：状态=running", "修向=调度供给", "帧因果未证：本席位引用的 trace 证据中没有帧证据",
		"trace 定位：附件 trace 第 2892–13060 行，发生 34579.480100–34579.521000 s，分析窗 34579.472865–34579.587805 s",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("typed fact missing %q in:\n%s", want, joined)
		}
	}
	for _, forbidden := range []string{".codrax", "trace_query:", ".json#", "attached_trace.txt", "证据 "} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("internal reference leaked to the customer face (%q):\n%s", forbidden, joined)
		}
	}
	for _, line := range item.Evidence {
		if utf8.RuneCountInString(line) > types.TraceRootCauseEvidenceMaxRunes {
			t.Fatalf("overlong evidence: %q", line)
		}
	}
	if !reflect.DeepEqual(candidate.Decision.EvidenceRefs, before) || item.CandidateID != "" ||
		item.Category != types.TraceRootCauseComputeSupplyShortage || *item.ImpactSeconds != .05832 {
		t.Fatalf("rendering changed source or semantics: %+v", item)
	}
}

// Credential forms: the relation sentence names the host-edge credential with
// its timestamp and via word, the interval credential, and the target-self form.
func TestBoundRootCauseEvidenceRelationSentenceSpeaksTheCredential(t *testing.T) {
	base := packingRootCauseCandidate([]string{"E1"})
	for _, tc := range []struct {
		name  string
		facts types.TraceCauseEvidenceFacts
		want  []string
	}{
		{"host direct edge", types.TraceCauseEvidenceFacts{ChainRelevance: "on_chain", OnChainBasis: "host_wakeup_edge_pre_state",
			HostWakeupEdgeAnchorTs: 34579.576675, HostWakeupEdgeVia: "direct", TargetSubject: "app-100"},
			[]string{"凭证=唤醒锚定：该线程于 34579.576675 s 通过直接唤醒边唤醒目标", "边=凭证/边前=有效/边后=解除"}},
		{"interval credential", types.TraceCauseEvidenceFacts{ChainRelevance: "on_chain", OnChainBasis: types.TraceCausalOnChainBasisSemanticChainIntervalRelation,
			SemanticClass: "class_verification", SpanName: "VerifyClass Foo"},
			[]string{"凭证=交集证明", "确定性语义工作 VerifyClass Foo（class_verification）", "语义完成机理未证（仅披露"}},
		{"target self", types.TraceCauseEvidenceFacts{ChainRelevance: "on_chain", OnChainBasis: types.TraceCausalOnChainBasisSelfDeterministicSpan, TargetSubject: "worker-9"},
			[]string{"该线程即分析目标自身", "凭证=目标自身"}},
	} {
		candidate := base
		facts := tc.facts
		candidate.Decision.EvidenceFacts = &facts
		item, ok := boundRootCauseItem(candidate)
		if !ok {
			t.Fatalf("%s: excluded", tc.name)
		}
		joined := strings.Join(item.Evidence, "\n")
		for _, want := range tc.want {
			if !strings.Contains(joined, want) {
				t.Fatalf("%s: missing %q in:\n%s", tc.name, want, joined)
			}
		}
	}
}

// A single oversized atom is fitted on a semantic boundary (or a rune boundary
// with an ellipsis) — a system-rendered sentence never fails the strict
// validator; references are not published so they cannot overflow it.
func TestBoundRootCauseEvidenceFitsOversizedAtoms(t *testing.T) {
	candidate := packingRootCauseCandidate([]string{strings.Repeat("长", 241)})
	candidate.Decision.EvidenceFacts = &types.TraceCauseEvidenceFacts{WakeupPath: []string{strings.Repeat("甲", 300), "worker-9"}, ChainRelevance: "on_chain", ChainDepth: 1}
	item, ok := boundRootCauseItem(candidate)
	if !ok {
		t.Fatal("candidate unexpectedly excluded")
	}
	for _, line := range item.Evidence {
		if utf8.RuneCountInString(line) > types.TraceRootCauseEvidenceMaxRunes {
			t.Fatalf("overlong evidence survived the fit: %q", line)
		}
		if strings.Contains(line, strings.Repeat("长", 241)) {
			t.Fatal("an internal reference must never be published")
		}
	}
	if _, err := types.NormalizeAndValidateTraceRootCauseReport(&types.TraceRootCauseReportV2{SchemaVersion: 2,
		RootCauses: []*types.TraceRootCauseItemV2{item}}); err != nil {
		t.Fatalf("fitted evidence must validate: %v", err)
	}
}

// A legacy candidate without typed facts keeps the quantified sentence only —
// and still publishes no reference.
func TestBoundRootCauseEvidenceKeepsCompactShapeWhenItFits(t *testing.T) {
	candidate := packingRootCauseCandidate([]string{"E1"})
	item, ok := boundRootCauseItem(candidate)
	want := fmt.Sprintf("worker-9 在目标窗口内的链上有效归因为 58.320 ms；%s", RootCauseValueDescription(candidate.Decision))
	if !ok || !reflect.DeepEqual(item.Evidence, []string{want}) {
		t.Fatalf("short evidence needlessly changed: %+v", item)
	}
}

func packingRootCauseCandidate(refs []string) types.TraceFindingCandidateV1 {
	return types.TraceFindingCandidateV1{PrimaryEligible: true, Decision: types.TraceCauseDecision{
		CandidateID: "packing-candidate", SubjectName: "worker-9",
		Token: types.TraceCausalTokenSnapshot{Token: "running", Lane: "cpu_work"},
		Magnitude: &types.TypedMagnitude{Value: 58.320, Unit: "ms", Additivity: "wall_clock_per_thread", Caliber: "effective_attribution",
			Components: &types.TraceMagnitudeComponents{SupplyFoldComputed: true, SupplyFoldKnownMS: 157.248,
				SupplyFoldCapabilitySource: "default_table"}}, EvidenceRefs: refs,
		CausalQualifier: types.TraceCausalQualifierProven,
	}}
}
