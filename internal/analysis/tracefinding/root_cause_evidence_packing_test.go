package tracefinding

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBoundRootCauseEvidencePacksCompleteFactsAndReferences(t *testing.T) {
	for _, tc := range []struct {
		name string
		refs []string
	}{
		{"live length shape", []string{
			"/workspace/.codrax/blob/20260902-005125-000-78634/attached_trace_with_a_long_capture_name.txt:2-27625",
			"trace_query:trace-query-result-28bd29e5.json#root_cause_rank:1",
		}},
		{"four bounded entries", []string{strings.Repeat("甲", 175), strings.Repeat("乙", 175), strings.Repeat("丙", 175)}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			candidate := packingRootCauseCandidate(tc.refs)
			before := append([]string(nil), candidate.Decision.EvidenceRefs...)
			report, err := BindRootCauseReportSelection(&types.TraceRootCauseReportV2{SchemaVersion: 2,
				RootCauses: []*types.TraceRootCauseItemV2{{CandidateID: candidate.Decision.CandidateID}}},
				&types.TraceFindingContract{RootCauseReportEnabled: true, Candidates: []types.TraceFindingCandidateV1{candidate}})
			if err != nil {
				t.Fatalf("representable frozen evidence was rejected: %v", err)
			}
			item := report.RootCauses[0]
			if len(item.Evidence) < 2 || len(item.Evidence) > 4 {
				t.Fatalf("evidence was not packed into the existing wire limits: %+v", item.Evidence)
			}
			joined := strings.Join(item.Evidence, "\n")
			for _, fact := range append([]string{"worker-9", "58.320 ms", RootCauseValueDescription(candidate.Decision)}, tc.refs...) {
				if strings.Count(joined, fact) != 1 {
					t.Fatalf("fact/reference lost, split or duplicated: %q in %q", fact, joined)
				}
			}
			last := -1
			for _, ref := range tc.refs {
				at := strings.Index(joined, ref)
				if at <= last {
					t.Fatalf("reference order changed: %q", joined)
				}
				last = at
			}
			for _, line := range item.Evidence {
				if utf8.RuneCountInString(line) > 240 {
					t.Fatalf("overlong evidence: %q", line)
				}
			}
			if !reflect.DeepEqual(candidate.Decision.EvidenceRefs, before) || item.CandidateID != "" ||
				item.Category != types.TraceRootCauseComputeSupplyShortage || *item.ImpactSeconds != .05832 {
				t.Fatalf("packing changed source or semantics: %+v", item)
			}
		})
	}
}

func TestBoundRootCauseEvidenceDoesNotTruncateUnrepresentableFacts(t *testing.T) {
	for _, refs := range [][]string{
		{strings.Repeat("长", 241)},
		{strings.Repeat("a", 200), strings.Repeat("b", 200), strings.Repeat("c", 200), strings.Repeat("d", 200)},
	} {
		candidate := packingRootCauseCandidate(refs)
		item, ok := boundRootCauseItem(candidate)
		if !ok {
			t.Fatal("source candidate unexpectedly excluded")
		}
		joined := strings.Join(item.Evidence, "\n")
		for _, ref := range refs {
			if !strings.Contains(joined, ref) {
				t.Fatal("reference silently truncated or dropped")
			}
		}
		if _, err := types.NormalizeAndValidateTraceRootCauseReport(&types.TraceRootCauseReportV2{SchemaVersion: 2,
			RootCauses: []*types.TraceRootCauseItemV2{item}}); err == nil {
			t.Fatal("packing bypassed public v2 limits")
		}
	}
}

func TestBoundRootCauseEvidenceKeepsCompactShapeWhenItFits(t *testing.T) {
	candidate := packingRootCauseCandidate([]string{"E1"})
	item, ok := boundRootCauseItem(candidate)
	want := fmt.Sprintf("worker-9 在目标窗口内的链上有效归因为 58.320 ms（证据 E1）；%s", RootCauseValueDescription(candidate.Decision))
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
