package types

import "testing"

// TestCompletenessObligation_IsActive pins the gate predicate.
func TestCompletenessObligation_IsActive(t *testing.T) {
	cases := []struct {
		name string
		c    *CompletenessObligation
		want bool
	}{
		{"nil → false", nil, false},
		{"required+quote → true", &CompletenessObligation{Required: true, SourceQuote: "all the X"}, true},
		{"required no quote → false", &CompletenessObligation{Required: true}, false},
		{"quote no required → false", &CompletenessObligation{SourceQuote: "all the X"}, false},
	}
	for _, c := range cases {
		if got := c.c.IsActive(); got != c.want {
			t.Errorf("%s: IsActive=%v, want %v", c.name, got, c.want)
		}
	}
}

// TestNormalizeCompletenessObligation_QuoteGrounding pins the
// verbatim-quote-must-appear-in-raw invariant. Mirrors
// NormalizeRequestedEnumerationBoundary's grounding semantic.
func TestNormalizeCompletenessObligation_QuoteGrounding(t *testing.T) {
	raw := "列出本仓库里所有实现了 LoopController 接口的具体类型"
	cases := []struct {
		name string
		in   *CompletenessObligation
		want bool // non-nil result?
	}{
		{"nil input → nil", nil, false},
		{"!Required → nil", &CompletenessObligation{Required: false, SourceQuote: "所有"}, false},
		{"empty quote → nil", &CompletenessObligation{Required: true}, false},
		{"quote not in raw → nil", &CompletenessObligation{Required: true, SourceQuote: "every single"}, false},
		{"quote verbatim in raw → kept", &CompletenessObligation{Required: true, SourceQuote: "所有"}, true},
	}
	for _, c := range cases {
		got := NormalizeCompletenessObligation(raw, c.in)
		if (got != nil) != c.want {
			t.Errorf("%s: got %v, want non-nil=%v", c.name, got, c.want)
		}
	}
}

// TestNormalizeBuckets_VerbatimAndDedup pins the bucket
// normalization contract: verbatim-presence in raw, case-insensitive
// dedup, sequential 1-based indexing.
func TestNormalizeBuckets_VerbatimAndDedup(t *testing.T) {
	raw := "分别列出 Turn A 和 Turn B 产出的 emit_* 工具"
	in := []QuestionBucket{
		{Label: "Turn A", Anchors: []string{"explorer"}},
		{Label: "Turn B", Anchors: []string{"extractor"}},
		{Label: "turn a", Anchors: []string{"dup"}},  // dedup target
		{Label: "Turn C", Anchors: []string{"none"}}, // not in raw → drop
		{Label: "", Anchors: nil},                    // empty → drop
	}
	got := NormalizeBuckets(raw, in)
	if len(got) != 2 {
		t.Fatalf("got %d buckets, want 2 (Turn A + Turn B; turn a dedup'd, Turn C dropped, empty dropped)", len(got))
	}
	if got[0].Label != "Turn A" || got[0].Index != 1 {
		t.Errorf("bucket[0]: Label=%q Index=%d, want (Turn A, 1)", got[0].Label, got[0].Index)
	}
	if got[1].Label != "Turn B" || got[1].Index != 2 {
		t.Errorf("bucket[1]: Label=%q Index=%d, want (Turn B, 2)", got[1].Label, got[1].Index)
	}
	// Anchors propagated.
	if len(got[0].Anchors) != 1 || got[0].Anchors[0] != "explorer" {
		t.Errorf("bucket[0].Anchors lost: %v", got[0].Anchors)
	}
}

// TestNormalizeBuckets_EmptyAndNil pins the nil-safe contract.
func TestNormalizeBuckets_EmptyAndNil(t *testing.T) {
	if got := NormalizeBuckets("any", nil); got != nil {
		t.Errorf("nil input → got %v, want nil", got)
	}
	if got := NormalizeBuckets("any", []QuestionBucket{}); got != nil {
		t.Errorf("empty slice → got %v, want nil", got)
	}
	if got := NormalizeBuckets("raw", []QuestionBucket{{Label: "not-in-raw"}}); got != nil {
		t.Errorf("all-dropped → got %v, want nil", got)
	}
}

// TestQuestionStructureView_HasAnyObligation pins the fast-path.
func TestQuestionStructureView_HasAnyObligation(t *testing.T) {
	cases := []struct {
		name string
		v    QuestionStructureView
		want bool
	}{
		{"empty → false", QuestionStructureView{}, false},
		{"only count → true", QuestionStructureView{
			EnumerationBoundary: &RequestedEnumerationBoundary{DeclaredCount: 7, SourceQuote: "7 X"},
		}, true},
		{"only completeness → true", QuestionStructureView{
			CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all"},
		}, true},
		{"only 1 bucket → false (need >=2 to partition)", QuestionStructureView{
			Buckets: []QuestionBucket{{Label: "A", Index: 1}},
		}, false},
		{"2 buckets → true", QuestionStructureView{
			Buckets: []QuestionBucket{{Label: "A", Index: 1}, {Label: "B", Index: 2}},
		}, true},
	}
	for _, c := range cases {
		if got := c.v.HasAnyObligation(); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// TestRequestModel_QuestionStructure_Synthesizes pins the accessor.
func TestRequestModel_QuestionStructure_Synthesizes(t *testing.T) {
	rm := RequestModel{
		EnumerationBoundary:    &RequestedEnumerationBoundary{DeclaredCount: 9, SourceQuote: "9 项"},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all"},
		Buckets:                []QuestionBucket{{Label: "A", Index: 1}},
	}
	v := rm.QuestionStructure()
	if v.EnumerationBoundary != rm.EnumerationBoundary {
		t.Errorf("EnumerationBoundary mismatch")
	}
	if v.CompletenessObligation != rm.CompletenessObligation {
		t.Errorf("CompletenessObligation mismatch")
	}
	if len(v.Buckets) != 1 {
		t.Errorf("Buckets len=%d, want 1", len(v.Buckets))
	}
	if v.ResolvedDeclaredCount() != 9 {
		t.Errorf("ResolvedDeclaredCount=%d, want 9", v.ResolvedDeclaredCount())
	}
}

func TestEffectiveQuestionBuckets_InferFromTypedRequiredFiles(t *testing.T) {
	rm := RequestModel{
		RawRequest: "对比 explorer.go 和 extractor.go 的 ReAct 循环终止条件有何不同？",
		Predicates: SemanticPredicates{
			IsCrossComponent: true,
		},
		SubTopics: []SubTopic{
			{Summary: "explorer side", Entities: []string{"explorerEvaluator"}},
			{Summary: "extractor side", Entities: []string{"extractorEvaluator"}},
		},
		AnalyzerHints: AnalyzerHints{
			RequiredFileHints: []RequiredFileHint{
				{Path: "internal/agent/extractor.go", Confidence: 0.95},
				{Path: "internal/agent/iteration_cap.go", Confidence: 0.95},
				{Path: "internal/agent/explorer.go", Confidence: 0.95},
			},
		},
	}
	buckets := rm.QuestionStructure().Buckets
	if len(buckets) != 2 {
		t.Fatalf("buckets len=%d, want 2: %+v", len(buckets), buckets)
	}
	if buckets[0].Label != "explorer.go" || buckets[1].Label != "extractor.go" {
		t.Fatalf("bucket order/labels = %+v, want explorer.go then extractor.go", buckets)
	}
	if !rm.QuestionStructure().HasAnyObligation() {
		t.Fatal("inferred comparison buckets should make QuestionStructure obligation-bearing")
	}
}

func TestEffectiveQuestionBuckets_NoInferWithoutTypedParallelShape(t *testing.T) {
	rm := RequestModel{
		RawRequest: "explorer.go 和 extractor.go 如何协作？",
		Predicates: SemanticPredicates{
			IsCrossComponent: true,
		},
		AnalyzerHints: AnalyzerHints{
			RequiredFileHints: []RequiredFileHint{
				{Path: "internal/agent/explorer.go", Confidence: 0.95},
				{Path: "internal/agent/extractor.go", Confidence: 0.95},
			},
		},
	}
	if buckets := rm.QuestionStructure().Buckets; len(buckets) != 0 {
		t.Fatalf("single-topic typed shape must not infer buckets: %+v", buckets)
	}
}

// TestEffectiveQuestionBuckets_InferFromMentionedEntities is the F3
// regression for the 2026-05-16 architectural-comparison finalize
// loop. The analyzer correctly identifies "codrax" and "opencode"
// as MentionedEntities but emits required_files=[], so the
// inferRequiredFileComparisonBuckets fallback is silent. Without
// inferEntityComparisonBuckets, QuestionStructure.Buckets stays
// empty, the enumeration-label oracle has no vouch for entity
// labels in the auto-injected required_mechanism_anchors block, and
// the run loops on the same error class until the budget is burned.
func TestEffectiveQuestionBuckets_InferFromMentionedEntities(t *testing.T) {
	rm := RequestModel{
		RawRequest: "对比一下 codrax 和 opencode 在防止模型幻觉方面各有什么优缺点？",
		Predicates: SemanticPredicates{
			IsCrossComponent: true,
		},
		SubTopics: []SubTopic{
			{Summary: "codrax side", Entities: []string{"codrax"}},
			{Summary: "opencode side", Entities: []string{"opencode"}},
		},
		AnalyzerHints: AnalyzerHints{
			// Mirroring the production trace: analyzer emits entities
			// but NO required_file_hints — the file-based fallback
			// is dormant here.
			MentionedEntities: []string{"codrax", "opencode"},
		},
	}
	buckets := rm.QuestionStructure().Buckets
	if len(buckets) != 2 {
		t.Fatalf("buckets len=%d, want 2: %+v", len(buckets), buckets)
	}
	// Order follows RawRequest order, not slice order.
	if buckets[0].Label != "codrax" || buckets[1].Label != "opencode" {
		t.Fatalf("bucket labels = %+v, want codrax then opencode", buckets)
	}
}

func TestEffectiveQuestionBuckets_EntityFallback_RequiresCrossComponent(t *testing.T) {
	rm := RequestModel{
		RawRequest: "codrax 和 opencode 各是什么？",
		AnalyzerHints: AnalyzerHints{
			MentionedEntities: []string{"codrax", "opencode"},
		},
	}
	if buckets := rm.QuestionStructure().Buckets; len(buckets) != 0 {
		t.Fatalf("non-cross-component must not infer entity buckets: %+v", buckets)
	}
}

func TestEffectiveQuestionBuckets_EntityFallback_RequiresVerbatim(t *testing.T) {
	rm := RequestModel{
		RawRequest: "对比 A 和 B 在防幻方面差异",
		Predicates: SemanticPredicates{IsCrossComponent: true},
		SubTopics: []SubTopic{
			{Summary: "x"}, {Summary: "y"},
		},
		AnalyzerHints: AnalyzerHints{
			// Entities NOT verbatim in RawRequest — must be filtered.
			MentionedEntities: []string{"codrax", "opencode"},
		},
	}
	if buckets := rm.QuestionStructure().Buckets; len(buckets) != 0 {
		t.Fatalf("non-verbatim entities must not become buckets: %+v", buckets)
	}
}

func TestEffectiveQuestionBuckets_RequiredFilePathTakesPriorityOverEntity(t *testing.T) {
	rm := RequestModel{
		RawRequest: "对比 codrax 的 explorer.go 和 extractor.go",
		Predicates: SemanticPredicates{IsCrossComponent: true},
		SubTopics: []SubTopic{
			{Summary: "x"}, {Summary: "y"},
		},
		AnalyzerHints: AnalyzerHints{
			RequiredFileHints: []RequiredFileHint{
				{Path: "internal/agent/explorer.go", Confidence: 0.95},
				{Path: "internal/agent/extractor.go", Confidence: 0.95},
			},
			MentionedEntities: []string{"codrax"},
		},
	}
	buckets := rm.QuestionStructure().Buckets
	if len(buckets) != 2 {
		t.Fatalf("file fallback should win when both available: %+v", buckets)
	}
	if buckets[0].Label != "explorer.go" {
		t.Fatalf("expected file-derived buckets, got %+v", buckets)
	}
}

// TestRequestModel_BackCompat_ZeroValueQuestionStructure pins that
// pre-Plan-E callers (no QuestionStructure fields set) get a
// safe-zero view that all gates should short-circuit on.
func TestRequestModel_BackCompat_ZeroValueQuestionStructure(t *testing.T) {
	var rm RequestModel
	v := rm.QuestionStructure()
	if v.HasAnyObligation() {
		t.Errorf("zero-value RM must have no obligations")
	}
	if v.ResolvedDeclaredCount() != 0 {
		t.Errorf("zero-value count=%d, want 0", v.ResolvedDeclaredCount())
	}
}
