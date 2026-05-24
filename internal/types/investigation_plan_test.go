package types

import "testing"

func TestCompileInvestigationPlan_SubTopicsAreAnalyzerUnits(t *testing.T) {
	rm := RequestModel{
		Intent: IntentExplain,
		SubTopics: []SubTopic{
			{Summary: "分析 analyzer 重试", Entities: []string{"analyzer"}},
			{Summary: "分析 finalizer 重写", Entities: []string{"finalizer"}},
		},
	}
	got := CompileInvestigationPlan(rm, nil)
	if got.HasUserBuckets {
		t.Fatal("sub-topics must not become user buckets")
	}
	if got.Coupling != InvestigationCouplingIndependent {
		t.Fatalf("coupling=%q want %q", got.Coupling, InvestigationCouplingIndependent)
	}
	if len(got.Units) != 2 {
		t.Fatalf("units=%d want 2: %+v", len(got.Units), got.Units)
	}
	if got.Units[0].AnswerPartition != InvestigationAnswerPartitionAnalyzerDecomposition ||
		got.Units[0].Source != InvestigationUnitSourceSubTopic {
		t.Fatalf("unit[0] classification wrong: %+v", got.Units[0])
	}
	if got.Units[0].Label != "分析 analyzer 重试" || got.Units[0].Entities[0] != "analyzer" {
		t.Fatalf("unit[0] text/provenance lost: %+v", got.Units[0])
	}
}

func TestCompileInvestigationPlan_BucketsOutrankSubTopics(t *testing.T) {
	rm := RequestModel{
		RawRequest: "对比 codrax 和 opencode 的防幻觉机制",
		Predicates: SemanticPredicates{
			IsCrossComponent: true,
		},
		AnalyzerHints: AnalyzerHints{
			MentionedEntities: []string{"codrax", "opencode"},
		},
		SubTopics: []SubTopic{
			{Summary: "codrax", Entities: []string{"codrax"}},
			{Summary: "opencode", Entities: []string{"opencode"}},
		},
		Buckets: []QuestionBucket{
			{Label: "codrax", Index: 1},
			{Label: "opencode", Index: 2},
		},
	}
	got := CompileInvestigationPlan(rm, nil)
	if !got.HasUserBuckets {
		t.Fatal("explicit buckets should be marked as user buckets")
	}
	if got.Coupling != InvestigationCouplingComparative {
		t.Fatalf("coupling=%q want comparative", got.Coupling)
	}
	if len(got.Units) != 2 || got.Units[0].Source != InvestigationUnitSourceBucket ||
		got.Units[0].AnswerPartition != InvestigationAnswerPartitionUserBucket {
		t.Fatalf("bucket units not preserved: %+v", got.Units)
	}
}

func TestCompileInvestigationPlan_SequentialTraceAndOriginContract(t *testing.T) {
	rm := RequestModel{
		Intent: IntentTrace,
		AnalyzerHints: AnalyzerHints{
			Kind: string(ReqCallChain),
		},
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
		},
		SubTopics: []SubTopic{
			{Summary: "diff 入口", Entities: []string{"git"}},
			{Summary: "当前代码验证", Entities: []string{"Run"}},
		},
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes: []CurrentSourceExplanationMode{
				CurrentSourceExplanationLocateCurrentCode,
			},
			Confidence: 0.9,
		},
	}
	got := CompileInvestigationPlan(rm, nil)
	if got.Coupling != InvestigationCouplingSequential {
		t.Fatalf("coupling=%q want sequential", got.Coupling)
	}
	unit, ok := got.InvestigationUnitForEvidenceNode("ev_t1")
	if !ok || unit.Index != 2 || unit.Label != "当前代码验证" {
		t.Fatalf("node→unit mapping failed: ok=%v unit=%+v", ok, unit)
	}
	if !containsAnswerEvidenceOrigin(unit.EvidenceOrigins, AnswerEvidenceOriginVCSMetadata) ||
		!containsAnswerEvidenceOrigin(unit.EvidenceOrigins, AnswerEvidenceOriginCurrentSource) {
		t.Fatalf("origins should preserve VCS + current source lanes: %+v", unit.EvidenceOrigins)
	}
}

func containsAnswerEvidenceOrigin(in []AnswerEvidenceOrigin, want AnswerEvidenceOrigin) bool {
	for _, got := range in {
		if got == want {
			return true
		}
	}
	return false
}
