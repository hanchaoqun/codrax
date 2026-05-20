package gate

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/normalizer"
	"github.com/hanchaoqun/codrax/internal/types"
)

// fakeSymbolResolver is a deterministic in-memory resolver used by the
// R1.4 / R1.5 tests so we can control "this entity resolves to domain
// X" without spinning up a real repomap graph. Returns hits ordered by
// the keys in the `byEntity` map (callers should pre-sort if order
// matters; no test today depends on iteration order beyond presence).
type fakeSymbolResolver struct {
	byEntity      map[string][]normalizer.SymbolHit
	byFile        map[string][]normalizer.SymbolHit
	byAlias       map[string][]normalizer.SymbolHit
	byRepoSurface map[string][]normalizer.SymbolHit
}

func (f *fakeSymbolResolver) LookupSymbol(surface string) []normalizer.SymbolHit {
	if f == nil {
		return nil
	}
	if hits, ok := f.byEntity[strings.TrimSpace(surface)]; ok {
		return hits
	}
	return nil
}

func (f *fakeSymbolResolver) LookupFileSurface(surface string) []normalizer.SymbolHit {
	if f == nil {
		return nil
	}
	if hits, ok := f.byFile[strings.TrimSpace(surface)]; ok {
		return hits
	}
	return nil
}

func (f *fakeSymbolResolver) LookupSymbolActionAlias(surface string) []normalizer.SymbolHit {
	if f == nil {
		return nil
	}
	if hits, ok := f.byAlias[strings.TrimSpace(surface)]; ok {
		return hits
	}
	return nil
}

func (f *fakeSymbolResolver) LookupRepoSurface(surface string) []normalizer.SymbolHit {
	if f == nil {
		return nil
	}
	if hits, ok := f.byRepoSurface[strings.TrimSpace(surface)]; ok {
		return hits
	}
	return nil
}

// coherenceFixtureIR builds a minimum AnalysisIR that already passes
// every other check in Run, so any rejection in a coherence test is
// guaranteed to come from one of the new gates and not collateral
// damage from coverage/dag/etc. The fixture is intentionally NOT
// composed via the real compiler — coherence checks read only
// RequestModel + view-derived signals, so a hand-rolled minimal IR
// is enough and lets tests isolate one signal at a time. The
// trailing variadic argument is accepted (and ignored) so existing
// callers don't have to drop their now-removed shape arg before
// the next refactor.
func coherenceFixtureIR(rm types.RequestModel, _ ...string) *types.AnalysisIR {
	return &types.AnalysisIR{
		Version:        types.AnalysisIRVersion,
		RequestModel:   rm,
		AnswerContract: types.AnswerContract{},
	}
}

// ── checkSubtopicCoherence ────────────────────────────────────────

func TestSubtopicCoherence_R1_1_DomainDivergence_SoftAdvisory(t *testing.T) {
	// 2026-05-08 R3 audit: R1.1 was demoted from hard fail to soft
	// advisory. Domain divergence is a NOISY heuristic (system-
	// inferred from TermGraph package distribution, not an LLM
	// self-contradiction), and per the R3 red line "noisy signals
	// only as soft guidance". The gate must now PASS with a
	// reduced score and an advisory detail naming R1.1; it must
	// NOT block the analyzer's emit loop.
	rm := types.RequestModel{
		TermGraph: types.TermGraph{
			Canonical: []types.CanonicalTerm{
				{ID: "code:a", Surface: "A", Kind: types.TermSymbol, Domain: "agent", Confidence: 0.9},
				{ID: "code:b", Surface: "B", Kind: types.TermSymbol, Domain: "orchestrator", Confidence: 0.9},
			},
		},
		// 0 sub-topics — even with no sub-topic, R1.1 alone does NOT
		// hard-fail any more.
	}
	ir := coherenceFixtureIR(rm)
	check := checkSubtopicCoherence(ir, nil)
	if !check.Passed {
		t.Fatalf("R1.1 alone must NOT fail the gate (soft advisory only); got %+v", check)
	}
	if !strings.Contains(check.Detail, "R1.1") || !strings.Contains(check.Detail, "agent") {
		t.Errorf("detail must cite R1.1 and the divergent domains as advisory; got %q", check.Detail)
	}
	if check.Score >= 1.0 {
		t.Errorf("R1.1 advisory must reduce Score below 1.0 to differentiate from clean pass; got %v", check.Score)
	}
}

func TestSubtopicCoherence_R1_1_DomainDivergence_PassesWithSubTopics(t *testing.T) {
	rm := types.RequestModel{
		TermGraph: types.TermGraph{
			Canonical: []types.CanonicalTerm{
				{ID: "code:a", Surface: "A", Kind: types.TermSymbol, Domain: "agent", Confidence: 0.9},
				{ID: "code:b", Surface: "B", Kind: types.TermSymbol, Domain: "orchestrator", Confidence: 0.9},
			},
		},
		SubTopics: []types.SubTopic{
			{Summary: "agent half"},
			{Summary: "orchestrator half"},
		},
	}
	ir := coherenceFixtureIR(rm)
	if check := checkSubtopicCoherence(ir, nil); !check.Passed {
		t.Fatalf("R1.1 must pass when sub-topic count matches domain count; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_1_LowConfidenceTermsExcluded(t *testing.T) {
	// One Domain at high confidence, one at low confidence — the low-
	// confidence term should be excluded so the count stays at 1 and
	// R1.1 does NOT fire.
	rm := types.RequestModel{
		TermGraph: types.TermGraph{
			Canonical: []types.CanonicalTerm{
				{ID: "code:a", Surface: "A", Kind: types.TermSymbol, Domain: "agent", Confidence: 0.9},
				{ID: "code:b", Surface: "B", Kind: types.TermSymbol, Domain: "noisy", Confidence: 0.4},
			},
		},
	}
	ir := coherenceFixtureIR(rm)
	if check := checkSubtopicCoherence(ir, nil); !check.Passed {
		t.Fatalf("low-confidence terms must not contribute to domain count; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_2_CrossComponentWithoutPartition_Advisory(t *testing.T) {
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true},
		// no sub-topics
	}
	ir := coherenceFixtureIR(rm)
	check := checkSubtopicCoherence(ir, nil)
	if !check.Passed {
		t.Fatalf("R1.2 must be advisory when IsCrossComponent=true and no explicit partition exists; got %+v", check)
	}
	if !strings.Contains(check.Detail, "R1.2") {
		t.Errorf("detail must cite R1.2; got %q", check.Detail)
	}
	if check.Score >= 1.0 {
		t.Errorf("advisory should lower score without rejecting; got score=%f detail=%q", check.Score, check.Detail)
	}
}

func TestSubtopicCoherence_R1_3_EntityOrphan_Fails(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"PrimaryX", "PrimaryY"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "drifted", Entities: []string{"Unrelated1", "Unrelated2"}},
		},
	}
	ir := coherenceFixtureIR(rm)
	check := checkSubtopicCoherence(ir, nil)
	if check.Passed {
		t.Fatalf("R1.3 must fail when sub-topic entities share no element with primary entities; got %+v", check)
	}
	if !strings.Contains(check.Detail, "R1.3") {
		t.Errorf("detail must cite R1.3; got %q", check.Detail)
	}
}

func TestSubtopicCoherence_R1_3_EntityOrphan_SinglePrimarySkipped(t *testing.T) {
	// Only 1 PrimaryEntity — orphan rule must NOT fire (a single
	// primary often legitimately fans out into different sub-topic
	// surfaces).
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"X"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "topic", Entities: []string{"Y"}},
		},
	}
	ir := coherenceFixtureIR(rm)
	if check := checkSubtopicCoherence(ir, nil); !check.Passed {
		t.Fatalf("R1.3 must pass when PrimaryEntities < 2; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_3_EntityOrphan_OverlapPasses(t *testing.T) {
	rm := types.RequestModel{
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"Shared", "Other"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "topic1", Entities: []string{"Shared"}},
			{Summary: "topic2", Entities: []string{"AlsoOther"}},
		},
	}
	ir := coherenceFixtureIR(rm)
	if check := checkSubtopicCoherence(ir, nil); !check.Passed {
		t.Fatalf("R1.3 must pass when sub-topics share at least one entity with primary; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_3_MultiScopeFacetDecompositionIsAdvisory(t *testing.T) {
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"codrax", "opencode"},
			PrimaryScopes:   []string{"codrax", "opencode"},
		},
		SubTopics: []types.SubTopic{
			{
				Summary:  "codrax grounding and contract checks",
				Entities: []string{"ground.go", "claim_citation.go", "contract_check.go"},
			},
			{
				Summary:  "opencode read path",
				Entities: []string{"read.ts", "agent.ts"},
			},
		},
	}
	ir := coherenceFixtureIR(rm)
	check := checkSubtopicCoherence(ir, nil)
	if !check.Passed {
		t.Fatalf("R1.3 must be advisory for multi-scope facet/file decompositions; got %+v", check)
	}
	if !strings.Contains(check.Detail, "R1.3 entity_orphan (advisory)") {
		t.Fatalf("detail should preserve R1.3 advisory telemetry; got %q", check.Detail)
	}
}

// ── R1.5 entity_unresolvable ──────────────────────────────────────

func TestSubtopicCoherence_R1_5_AllEntitiesUnresolved_Fails(t *testing.T) {
	// 3 sub-topics; sub-topic 3's entities all return 0 resolver hits
	// (the canonical "PlanMode / mode_dispatch hallucination" pattern).
	// Sub-topics 1 and 2 each have at least one resolvable entity so
	// the failure is isolated to sub-topic 3.
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"PipelineMode": {{Canonical: "PipelineMode", Domain: "types"}},
			"analyze":      {{Canonical: "analyze", Domain: "agent"}},
			"plan":         {{Canonical: "plan", Domain: "agent"}},
			// PlanMode and mode_dispatch deliberately absent → 0 hits.
		},
	}
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true}, // bypass R1.4
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"PipelineMode", "PlanMode", "orchestrator"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "Analyze pipeline", Entities: []string{"PipelineMode", "analyze"}},
			{Summary: "Plan pipeline", Entities: []string{"PipelineMode", "plan"}},
			{Summary: "模式分发机制", Entities: []string{"PlanMode", "mode_dispatch"}},
		},
	}
	ir := coherenceFixtureIR(rm)
	check := checkSubtopicCoherence(ir, resolver)
	if check.Passed {
		t.Fatalf("R1.5 must fail when a sub-topic has zero resolvable entities; got %+v", check)
	}
	if !strings.Contains(check.Detail, "R1.5") {
		t.Errorf("detail must cite R1.5; got %q", check.Detail)
	}
	if !strings.Contains(check.Detail, "sub-topic 3") {
		t.Errorf("detail must name the failing sub-topic index; got %q", check.Detail)
	}
}

func TestSubtopicCoherence_R1_5_ArchitectureDesignAxisAdvisoryOnly(t *testing.T) {
	// Broad architecture/design-document questions legitimately use
	// subsystem or directory axes as exploration leads. Even when one
	// axis does not resolve as a symbol, the analyzer should not burn
	// retries trying to force every design-doc sub-topic into the symbol
	// table; exploration/finalization can ground or caveat the axis.
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"OHOS":                {{Canonical: "OHOS", Domain: "OHOS"}},
			"ResourceSchedule":    {{Canonical: "ResourceSchedule", Domain: "OHOS::ResourceSchedule"}},
			"socperf_plugin":      {{Canonical: "SocPerfPlugin", Domain: "plugins"}},
			"cgroup_sched_plugin": {{Canonical: "CgroupSchedPlugin", Domain: "plugins"}},
		},
	}
	rm := types.RequestModel{
		Intent:   types.IntentExplain,
		Scenario: types.ScenarioArchitectureExplain,
		AnswerSubject: types.AnswerSubject{
			Kind:       types.SubjectGeneric,
			Confidence: 0.9,
		},
		Predicates: types.SemanticPredicates{
			IsCrossComponent: true,
		},
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"OHOS", "ResourceSchedule", "socperf_plugin", "cgroup_sched_plugin", "resched", "resched_executor"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "OHOS ResourceSchedule 核心调度框架与模块架构", Entities: []string{"OHOS", "ResourceSchedule"}},
			{Summary: "插件生态体系", Entities: []string{"socperf_plugin", "cgroup_sched_plugin"}},
			{Summary: "调度服务与执行器的运行流程与线程模型", Entities: []string{"resched", "resched_executor"}},
		},
	}
	check := checkSubtopicCoherence(coherenceFixtureIR(rm), resolver)
	if !check.Passed {
		t.Fatalf("architecture design axes should be advisory, got %+v", check)
	}
	if !strings.Contains(check.Detail, "R1.5 entity_unresolvable (advisory)") {
		t.Fatalf("expected advisory R1.5 detail, got %q", check.Detail)
	}
}

func TestSubtopicCoherence_R1_5_HistoryBackedTraceDiagramAdvisoryOnly(t *testing.T) {
	// History-backed explanation/trace requests often decompose into
	// VCS lookup, changed-code analysis, and requested presentation
	// shape. The VCS/output-shape axes are user intent, not repo
	// declarations, so mixed resolver hits must not force a retry.
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"LoopPolicy": {{Canonical: "LoopPolicy", Domain: "agent"}},
		},
	}
	rm := types.RequestModel{
		Intent: types.IntentTrace,
		Predicates: types.SemanticPredicates{
			IsHistoryLookup: true,
		},
		DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow},
		SubTopics: []types.SubTopic{
			{Summary: "查找最近一次 merge commit", Entities: []string{"git log", "merge commit"}},
			{Summary: "定位对应代码", Entities: []string{"LoopPolicy"}},
			{Summary: "绘制逻辑图", Entities: []string{"architecture diagram", "flow diagram"}},
		},
	}
	check := checkSubtopicCoherence(coherenceFixtureIR(rm), resolver)
	if !check.Passed {
		t.Fatalf("history-backed trace/diagram axes should be advisory, got %+v", check)
	}
	if !strings.Contains(check.Detail, "R1.5 entity_unresolvable (advisory)") {
		t.Fatalf("expected advisory R1.5 detail, got %q", check.Detail)
	}
}

func TestSubtopicCoherence_R1_5_DiagramPresentationAxisAdvisoryOnly(t *testing.T) {
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"BuildAgentContext": {{Canonical: "BuildAgentContext", Domain: "context"}},
		},
	}
	rm := types.RequestModel{
		Intent:      types.IntentExplain,
		DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture},
		SubTopics: []types.SubTopic{
			{Summary: "上下文构建", Entities: []string{"BuildAgentContext"}},
			{Summary: "架构视图呈现", Entities: []string{"architecture diagram"}},
		},
	}
	check := checkSubtopicCoherence(coherenceFixtureIR(rm), resolver)
	if !check.Passed {
		t.Fatalf("diagram presentation axes should be advisory, got %+v", check)
	}
	if !strings.Contains(check.Detail, "R1.5 entity_unresolvable (advisory)") {
		t.Fatalf("expected advisory R1.5 detail, got %q", check.Detail)
	}
}

func TestSubtopicCoherence_R1_5_CategoryEnumerationStaysHardDespiteDiagram(t *testing.T) {
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"KnownKind": {{Canonical: "KnownKind", Domain: "types"}},
		},
	}
	rm := types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow},
		SubTopics: []types.SubTopic{
			{Summary: "known enum member", Entities: []string{"KnownKind"}},
			{Summary: "invented enum member", Entities: []string{"InventedKind"}},
		},
	}
	check := checkSubtopicCoherence(coherenceFixtureIR(rm), resolver)
	if check.Passed {
		t.Fatalf("enumeration entity asymmetry should remain hard despite diagram hint, got %+v", check)
	}
	if !strings.Contains(check.Detail, "R1.5") {
		t.Fatalf("expected R1.5 hard failure, got %q", check.Detail)
	}
}

func TestSubtopicCoherence_R1_5_OneEntityResolves_Passes(t *testing.T) {
	// A sub-topic with one unresolvable + one resolvable entity must
	// PASS R1.5 — the rule requires at least ONE hit per sub-topic,
	// not all entities.
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"PipelineMode": {{Canonical: "PipelineMode", Domain: "types"}},
			"PlanMode":     {{Canonical: "PlanMode", Domain: "types"}},
		},
	}
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true}, // bypass R1.4
		SubTopics: []types.SubTopic{
			{Summary: "first", Entities: []string{"PipelineMode", "garbage"}},
			{Summary: "second", Entities: []string{"PlanMode", "also_garbage"}},
		},
	}
	ir := coherenceFixtureIR(rm)
	if check := checkSubtopicCoherence(ir, resolver); !check.Passed {
		t.Fatalf("R1.5 must pass when at least one entity per sub-topic resolves; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_5_UniqueActionAliasResolves(t *testing.T) {
	resolver := &fakeSymbolResolver{
		byAlias: map[string][]normalizer.SymbolHit{
			"read_scheduler_loop": {{Canonical: "runReadSchedulerLoop", Domain: "orchestrator"}},
		},
	}
	if !subTopicEntityResolvedForCoherence("read_scheduler_loop", types.RequestModel{}, resolver) {
		t.Fatal("unique action-prefix alias must resolve for coherence")
	}
	resolver.byAlias["read_scheduler_loop"] = []normalizer.SymbolHit{
		{Canonical: "runReadSchedulerLoop", Domain: "orchestrator"},
		{Canonical: "buildReadSchedulerLoop", Domain: "orchestrator"},
	}
	if subTopicEntityResolvedForCoherence("read_scheduler_loop", types.RequestModel{}, resolver) {
		t.Fatal("ambiguous action-prefix alias must not satisfy hard coherence")
	}
}

func TestSubtopicCoherence_R1_5_UserMentionedFieldAndLocalSurfacesPass(t *testing.T) {
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"callChainPrincipalSpanDemandForEvidence": {{Canonical: "callChainPrincipalSpanDemandForEvidence", Domain: "tool"}},
			"canonicalCallChainSource":                {{Canonical: "canonicalCallChainSource", Domain: "tool"}},
		},
	}
	rm := types.RequestModel{
		RawRequest: "在 callChainPrincipalSpanDemandForEvidence 里，EvidenceItem.Source 是如何经过 canonicalCallChainSource 再被放入 bySource map 的？",
		Predicates: types.SemanticPredicates{
			IsCrossComponent: true,
		},
		SubTopics: []types.SubTopic{
			{Summary: "source normalizer", Entities: []string{"canonicalCallChainSource"}},
			{Summary: "field surface", Entities: []string{"EvidenceItem.Source"}},
			{Summary: "local aggregation map", Entities: []string{"bySource"}},
		},
	}
	check := checkSubtopicCoherence(coherenceFixtureIR(rm), resolver)
	if !check.Passed {
		t.Fatalf("R1.5 must not reject exact user-mentioned field/local-variable search leads; got %+v", check)
	}
	if !subTopicEntityResolvedForCoherence("EvidenceItem.Source", rm, resolver) {
		t.Fatal("user-mentioned field path should satisfy coherence as a search lead")
	}
	if !subTopicEntityResolvedForCoherence("bySource", rm, resolver) {
		t.Fatal("user-mentioned local variable should satisfy coherence as a search lead")
	}
}

func TestSubtopicCoherence_R1_5_NilResolver_NoOp(t *testing.T) {
	// Pre-RunOptions callers (Run, tests, write mode) pass nil resolver
	// → R1.5 must be a no-op so existing R1.1/R1.2/R1.3 behaviour is
	// preserved byte-for-byte.
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true}, // bypass R1.4
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"X", "Y"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "first", Entities: []string{"X"}},
			{Summary: "second", Entities: []string{"never-resolves"}},
		},
	}
	ir := coherenceFixtureIR(rm)
	if check := checkSubtopicCoherence(ir, nil); !check.Passed {
		t.Fatalf("nil resolver must disable R1.5; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_5_AllConceptual_NoOp(t *testing.T) {
	// Audit run 2026-05-02 07:06 ("对比 read 模式的 explorer 阶段
	// 和 write 模式的 verify 阶段") emitted 2 sub-topics with
	// conceptual entities (`explorer` / `retry` / `read`) — none of
	// which the resolver could match because they are stage / phase
	// names, not Tier 1-2 symbols. Pre-2026-05-02-2 the rule fired
	// here and burned the analyzer retry budget. The refined rule
	// requires ASYMMETRY: only fire when some sub-topics resolve and
	// others don't. A uniformly-conceptual IR is the legitimate
	// cross-component case and must pass.
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			// no entries — every entity returns 0 hits
		},
	}
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true},
		SubTopics: []types.SubTopic{
			{Summary: "explorer 阶段（read 模式）的 retry 机制", Entities: []string{"explorer", "retry", "read"}},
			{Summary: "verify 阶段（write 模式）的 retry 机制", Entities: []string{"verifier", "retry", "write"}},
		},
	}
	ir := coherenceFixtureIR(rm)
	if check := checkSubtopicCoherence(ir, resolver); !check.Passed {
		t.Fatalf("R1.5 must pass when all sub-topics are uniformly unresolved (no asymmetry); got %+v", check)
	}
}

func TestSubtopicCoherence_R1_5_DiagnosticFacetSubTopicsBypassResolverAsymmetry(t *testing.T) {
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"ExecCommand": {{Canonical: "ExecCommand", Domain: "tool"}},
			// source/sink/protection facet labels and stdlib names may
			// legitimately have zero repo-symbol hits.
		},
	}
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{
			IsCrossComponent:     true,
			IsDiagnosticQuestion: true,
		},
		DiagnosticProfile: types.DiagnosticIntentProfile{IsDiagnostic: true, CurrentRisk: true},
		SubTopics: []types.SubTopic{
			{Summary: "taint source", Entities: []string{"ExecCommand"}},
			{Summary: "sink", Entities: []string{"exec.Command", "shell sink"}},
			{Summary: "protection", Entities: []string{"allowlist", "sandbox"}},
		},
	}
	ir := coherenceFixtureIR(rm)
	if check := checkSubtopicCoherence(ir, resolver); !check.Passed {
		t.Fatalf("diagnostic facet sub-topics should not hard-fail on resolver asymmetry; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_5_SingleTopic_NoOp(t *testing.T) {
	// Single-topic IR doesn't go through the multi-topic anchor
	// backbone, so R1.5 should not fire even with an unresolvable
	// entity.
	resolver := &fakeSymbolResolver{byEntity: map[string][]normalizer.SymbolHit{}}
	rm := types.RequestModel{
		SubTopics: []types.SubTopic{
			{Summary: "only", Entities: []string{"never-resolves"}},
		},
	}
	ir := coherenceFixtureIR(rm)
	if check := checkSubtopicCoherence(ir, resolver); !check.Passed {
		t.Fatalf("R1.5 must skip when nSub<2; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_5_ExternalOnlyArtifactEntities_BypassRepoResolver(t *testing.T) {
	// External log entities are typed observation surfaces, not current
	// checkout symbols. A local repo may accidentally resolve one Java
	// class name while leaving IOException / endpoint strings unresolved;
	// that mixed resolver pattern must not hard-fail the analyzer.
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"UserService": {{Canonical: "UserService", Domain: "fixture"}},
		},
	}
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsDiagnosticQuestion: true},
		Intent:     types.IntentRootCause,
		Scenario:   types.ScenarioRootCause,
		LogTriage: &types.LogBundle{
			Errors: []types.LogError{{
				Type:    "java.lang.RuntimeException",
				Message: "Failed to process request",
			}},
			ResolvedFiles: nil,
		},
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"RuntimeException", "NullPointerException", "IOException", "UserService"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "surface wrapper", Entities: []string{"RuntimeException", "UserService"}},
			{Summary: "immediate trigger", Entities: []string{"NullPointerException", "UserService"}},
			{Summary: "external root cause", Entities: []string{"IOException", "10.0.0.5:5432"}},
		},
	}
	ir := coherenceFixtureIR(rm)
	if check := checkSubtopicCoherence(ir, resolver); !check.Passed {
		t.Fatalf("external-only artifact entities must bypass repo-symbol R1.5, got %+v", check)
	}
}

func TestSubtopicCoherence_R1_5_FileSurfaceEntitiesResolve(t *testing.T) {
	// Architecture / trace planning often uses files as principal
	// surfaces: `Index.ets`, `foo.cj`, `orchestrator.go`, or
	// `src/lib.cpp` are valid anchors when FileIndex resolves them,
	// even though they are not symbols. R1.5 must consume the typed
	// file-surface lane instead of forcing the LLM to invent symbol
	// names for a file-shaped sub-topic.
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"Orchestrator": {{Canonical: "Orchestrator", Domain: "orchestrator"}},
		},
		byFile: map[string][]normalizer.SymbolHit{
			"orchestrator.go": {{Canonical: "internal/orchestrator/orchestrator.go", Domain: "orchestrator"}},
			"Index.ets":       {{Canonical: "entry/src/main/ets/pages/Index.ets", Domain: "pages"}},
			"pkg/foo.cj":      {{Canonical: "pkg/foo.cj", Domain: "pkg"}},
			"src/lib.cpp":     {{Canonical: "src/lib.cpp", Domain: "src"}},
		},
	}
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"Orchestrator", "logical view"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "symbol-backed control flow", Entities: []string{"Orchestrator"}},
			{Summary: "file-backed sequence diagram", Entities: []string{"orchestrator.go", "Index.ets", "pkg/foo.cj", "src/lib.cpp"}},
		},
	}
	ir := coherenceFixtureIR(rm)
	check := checkSubtopicCoherence(ir, resolver)
	if !check.Passed {
		t.Fatalf("R1.5 must accept file-surface entities resolved by FileIndex; got %+v", check)
	}
	if strings.Contains(check.Detail, "R1.5") {
		t.Fatalf("detail must not include an R1.5 hard-fail; got %q", check.Detail)
	}
}

func TestSubtopicCoherence_R1_5_RepoDirectorySurfaceEntitiesResolve(t *testing.T) {
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"notifyProcessStateChanged": {{Canonical: "notifyProcessStateChanged", Domain: "am"}},
			"HwServiceExFactory":        {{Canonical: "HwServiceExFactory", Domain: "server"}},
		},
		byRepoSurface: map[string][]normalizer.SymbolHit{
			"ancoproxy": {{Canonical: "frameworks/base/services/core/java/com/android/server/am/ancoproxy", Domain: "ancoproxy"}},
		},
	}
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsCrossComponent: true},
		AnalyzerHints: types.AnalyzerHints{
			PrimaryEntities: []string{"IAncoActivityManager", "AncoProcessData", "ancoproxy", "HwServiceExFactory"},
		},
		SubTopics: []types.SubTopic{
			{
				Summary:  "东湖感知机制的核心接口和数据载体分析",
				Entities: []string{"IAncoActivityManager", "ancoproxy", "AncoProcessData"},
			},
			{
				Summary:  "callback dispatch",
				Entities: []string{"notifyProcessStateChanged"},
			},
			{
				Summary:  "service factory integration",
				Entities: []string{"HwServiceExFactory"},
			},
		},
	}
	ir := coherenceFixtureIR(rm)
	check := checkSubtopicCoherence(ir, resolver)
	if !check.Passed {
		t.Fatalf("R1.5 must accept repo directory/package surfaces as typed grounding, got %+v", check)
	}
	if strings.Contains(check.Detail, "R1.5") {
		t.Fatalf("detail must not include an R1.5 hard-fail; got %q", check.Detail)
	}
}

func TestSubtopicCoherence_R1_5_CategoryEnumerationPackageMembersBypassSymbolResolver(t *testing.T) {
	// Cross-language package/module/directory members are often the
	// answer's principal objects but not symbol-table entries. If the
	// analyzer has already emitted a typed bounded enumeration member
	// lane, R1.5 must not hard-fail merely because some package-shaped
	// members do not resolve as symbols while a sibling happens to.
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"criterion": {{Canonical: "criterion", Domain: "analysis"}},
			// findings_validator / @scope/pkg / com.example.api
			// deliberately absent: valid package surfaces, not symbols.
		},
	}
	rm := types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
			IsCrossComponent:      true, // isolate R1.5 from R1.4 here
		},
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"criterion", "findings_validator", "@scope/pkg", "com.example.api"},
			RequiredFileHints: []types.RequiredFileHint{
				{Path: "internal/analysis/criterion/doc.go", Confidence: 0.9},
				{Path: "internal/analysis/findings_validator/doc.go", Confidence: 0.9},
			},
		},
		SubTopics: []types.SubTopic{
			{Summary: "criterion", Entities: []string{"criterion"}},
			{Summary: "other package members", Entities: []string{"findings_validator", "@scope/pkg", "com.example.api"}},
		},
	}
	ir := coherenceFixtureIR(rm)
	check := checkSubtopicCoherence(ir, resolver)
	if !check.Passed {
		t.Fatalf("R1.5 must accept typed cross-language package members as grounded enumeration members; got %+v", check)
	}
	if strings.Contains(check.Detail, "R1.5") {
		t.Fatalf("detail must not include an R1.5 hard-fail; got %q", check.Detail)
	}
}

func TestSubtopicCoherence_R1_5_UnboundedCategoryEnumerationMembersBypassSymbolResolver(t *testing.T) {
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"criterion": {{Canonical: "criterion", Domain: "analysis"}},
		},
	}
	rm := types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
			IsCrossComponent:      true,
		},
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"criterion", "priority", "risk", "sourcemix", "stopcond", "subject"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "first package group", Entities: []string{"criterion"}},
			{Summary: "last package group", Entities: []string{"priority", "risk", "sourcemix", "stopcond", "subject"}},
		},
	}
	ir := coherenceFixtureIR(rm)
	check := checkSubtopicCoherence(ir, resolver)
	if !check.Passed {
		t.Fatalf("R1.5 must not require package/module members to resolve as symbols in typed category enumerations; got %+v", check)
	}
}

// ── R1.4 axis_collapse ────────────────────────────────────────────

func TestSubtopicCoherence_R1_4_EnumerationCollapsesToSingleDomain_Fails(t *testing.T) {
	// Replicates the failing run: 3 sub-topics, every entity resolves
	// to the same Domain, IsCategoryEnumeration=true, IsCrossComponent
	// =false. The LLM split a single enumeration axis (mode value) into
	// per-value sub-topics — collapse repair required.
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"PipelineMode": {{Canonical: "PipelineMode", Domain: "types"}},
			"ModeRead":     {{Canonical: "ModeRead", Domain: "types"}},
			"ModePlan":     {{Canonical: "ModePlan", Domain: "types"}},
			"ModeApply":    {{Canonical: "ModeApply", Domain: "types"}},
		},
	}
	rm := types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
			IsCrossComponent:      false,
		},
		SubTopics: []types.SubTopic{
			{Summary: "ModeRead", Entities: []string{"PipelineMode", "ModeRead"}},
			{Summary: "ModePlan", Entities: []string{"PipelineMode", "ModePlan"}},
			{Summary: "ModeApply", Entities: []string{"PipelineMode", "ModeApply"}},
		},
	}
	ir := coherenceFixtureIR(rm)
	check := checkSubtopicCoherence(ir, resolver)
	if check.Passed {
		t.Fatalf("R1.4 must fail when enumeration sub-topics all collapse to one domain; got %+v", check)
	}
	if !strings.Contains(check.Detail, "R1.4") {
		t.Errorf("detail must cite R1.4; got %q", check.Detail)
	}
	if !strings.Contains(check.Detail, "ordered_list item") {
		t.Errorf("detail must propose the V2 ordered_list repair; got %q", check.Detail)
	}
}

func TestSubtopicCoherence_R1_4_TwoDomains_Passes(t *testing.T) {
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"Explorer": {{Canonical: "Explorer", Domain: "agent"}},
			"Resolver": {{Canonical: "Resolver", Domain: "tool"}},
		},
	}
	rm := types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		SubTopics: []types.SubTopic{
			{Summary: "explorer half", Entities: []string{"Explorer"}},
			{Summary: "resolver half", Entities: []string{"Resolver"}},
		},
	}
	ir := coherenceFixtureIR(rm)
	if check := checkSubtopicCoherence(ir, resolver); !check.Passed {
		t.Fatalf("R1.4 must pass when sub-topic entities span ≥2 domains; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_4_CrossComponent_NoOp(t *testing.T) {
	// IsCrossComponent=true is the LLM's explicit "this question spans
	// subsystems" judgment. Per the user-intent-over-system-gates red
	// line, we never force-collapse such a split even if domains
	// happen to overlap in the resolver.
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"PipelineMode": {{Canonical: "PipelineMode", Domain: "types"}},
			"ModePlan":     {{Canonical: "ModePlan", Domain: "types"}},
		},
	}
	rm := types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
			IsCrossComponent:      true, // load-bearing — bypasses R1.4
		},
		SubTopics: []types.SubTopic{
			{Summary: "first", Entities: []string{"PipelineMode"}},
			{Summary: "second", Entities: []string{"ModePlan"}},
		},
	}
	ir := coherenceFixtureIR(rm)
	if check := checkSubtopicCoherence(ir, resolver); !check.Passed {
		t.Fatalf("R1.4 must respect IsCrossComponent=true; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_4_NotEnumeration_NoOp(t *testing.T) {
	// Without IsCategoryEnumeration AND without Intent=enumerate, R1.4
	// must NOT fire — multi-aspect explanations of one component are
	// legitimate and not over-decomposition.
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"Explorer": {{Canonical: "Explorer", Domain: "agent"}},
		},
	}
	rm := types.RequestModel{
		Intent: types.IntentExplain,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: false,
		},
		SubTopics: []types.SubTopic{
			{Summary: "behaviour", Entities: []string{"Explorer"}},
			{Summary: "implementation", Entities: []string{"Explorer"}},
		},
	}
	ir := coherenceFixtureIR(rm)
	if check := checkSubtopicCoherence(ir, resolver); !check.Passed {
		t.Fatalf("R1.4 must skip non-enumeration intent; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_4_NilResolver_NoOp(t *testing.T) {
	rm := types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		SubTopics: []types.SubTopic{
			{Summary: "first", Entities: []string{"X"}},
			{Summary: "second", Entities: []string{"Y"}},
		},
	}
	ir := coherenceFixtureIR(rm)
	if check := checkSubtopicCoherence(ir, nil); !check.Passed {
		t.Fatalf("nil resolver must disable R1.4; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_4_ZeroResolvedDomains_NoOp(t *testing.T) {
	resolver := &fakeSymbolResolver{byEntity: map[string][]normalizer.SymbolHit{}}
	rm := types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
			IsCrossComponent:      false,
		},
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"priority", "risk", "sourcemix", "stopcond", "subject"},
		},
		SubTopics: []types.SubTopic{
			{Summary: "priority", Entities: []string{"priority"}},
			{Summary: "risk", Entities: []string{"risk"}},
			{Summary: "sourcemix", Entities: []string{"sourcemix"}},
		},
	}
	ir := coherenceFixtureIR(rm)
	if check := checkSubtopicCoherence(ir, resolver); !check.Passed {
		t.Fatalf("R1.4 must not infer axis collapse from zero resolver domains; got %+v", check)
	}
}

func TestSubtopicCoherence_R1_4_AttributeBearingPackageEnumeration_NoOp(t *testing.T) {
	// The "list every package and each package's entry point" shape is
	// a two-axis enumeration: principal members plus per-member
	// attribute. Package members may all sit in one code area or miss
	// symbol resolution entirely; collapsing that shape in analyzer
	// retries loses the attribute lane and causes downstream loops.
	resolver := &fakeSymbolResolver{
		byEntity: map[string][]normalizer.SymbolHit{
			"criterion": {{Canonical: "criterion", Domain: "analysis"}},
			// compiler is intentionally unresolved as a package member.
		},
	}
	rm := types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
			IsCrossComponent:      false,
		},
		AnalyzerHints: types.AnalyzerHints{
			Entities: []string{"criterion", "compiler"},
			RequiredFileHints: []types.RequiredFileHint{
				{Path: "internal/analysis/criterion/doc.go", Confidence: 0.9},
				{Path: "internal/analysis/compiler/doc.go", Confidence: 0.9},
			},
		},
		SubTopics: []types.SubTopic{
			{Summary: "principal members", Entities: []string{"criterion"}},
			{Summary: "per-member entry points", Entities: []string{"compiler"}},
		},
	}
	ir := coherenceFixtureIR(rm)
	check := checkSubtopicCoherence(ir, resolver)
	if !check.Passed {
		t.Fatalf("R1.4 must skip typed attribute-bearing package enumeration; got %+v", check)
	}
	if strings.Contains(check.Detail, "R1.4") {
		t.Fatalf("detail must not include an R1.4 hard-fail; got %q", check.Detail)
	}
}

// ── checkShapeSubjectCoherence ────────────────────────────────────

func TestShapeSubjectCoherence_R2_1_ScalarMultiTopic_Fails(t *testing.T) {
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsScalarAnswer: true},
		SubTopics: []types.SubTopic{
			{Summary: "first"},
			{Summary: "second"},
		},
	}
	ir := coherenceFixtureIR(rm)
	check := checkShapeSubjectCoherence(ir)
	if check.Passed {
		t.Fatalf("R2.1 must fail when IsScalarAnswer=true and 2+ sub-topics; got %+v", check)
	}
	if !strings.Contains(check.Detail, "R2.1") {
		t.Errorf("detail must cite R2.1; got %q", check.Detail)
	}
}

func TestShapeSubjectCoherence_R2_1_SingleTopicScalarPasses(t *testing.T) {
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsScalarAnswer: true},
	}
	ir := coherenceFixtureIR(rm)
	if check := checkShapeSubjectCoherence(ir); !check.Passed {
		t.Fatalf("R2.1 must pass for single-topic scalar; got %+v", check)
	}
}

func TestShapeSubjectCoherence_R2_2_ExplanationScalarSubject_Fails(t *testing.T) {
	rm := types.RequestModel{
		AnswerSubject: types.AnswerSubject{
			Kind:       types.SubjectNumeric,
			Confidence: 0.85,
		},
	}
	ir := coherenceFixtureIR(rm)
	check := checkShapeSubjectCoherence(ir)
	if check.Passed {
		t.Fatalf("R2.2 must fail when Explanation shape but high-confidence Numeric subject; got %+v", check)
	}
	if !strings.Contains(check.Detail, "R2.2") {
		t.Errorf("detail must cite R2.2; got %q", check.Detail)
	}
}

func TestShapeSubjectCoherence_R2_2_LowConfidencePasses(t *testing.T) {
	// Below the 0.6 confidence floor — R2.2 must NOT fire to avoid
	// noisy retries on uncertain subject inference.
	rm := types.RequestModel{
		AnswerSubject: types.AnswerSubject{
			Kind:       types.SubjectNumeric,
			Confidence: 0.40,
		},
	}
	ir := coherenceFixtureIR(rm)
	if check := checkShapeSubjectCoherence(ir); !check.Passed {
		t.Fatalf("R2.2 must pass below confidence floor; got %+v", check)
	}
}

func TestShapeSubjectCoherence_ScalarFamilyNotApplicable(t *testing.T) {
	// Role-lookup family (compileRoleLookup → BlockScalar at
	// SurfacePrincipal → NeedsPrincipalScalar=true) is the only
	// place a scalar subject naturally fits. R2.2 must NOT fire
	// for a scalar-payload family even at high subject confidence.
	rm := types.RequestModel{
		AnswerSubject: types.AnswerSubject{
			Kind:       types.SubjectFunctionName,
			Confidence: 0.95,
		},
	}
	ir := coherenceFixtureIR(rm, "")
	if check := checkShapeSubjectCoherence(ir); !check.Passed {
		t.Fatalf("scalar-payload family must not trigger R2.2; got %+v", check)
	}
}

// ── extractDistinctTermDomains ────────────────────────────────────

func TestExtractDistinctTermDomains_DropsConcepts(t *testing.T) {
	tg := types.TermGraph{
		Canonical: []types.CanonicalTerm{
			{Kind: types.TermSymbol, Domain: "agent", Confidence: 0.9},
			{Kind: types.TermConcept, Domain: "should-not-count", Confidence: 0.9},
			{Kind: types.TermSymbol, Domain: "orchestrator", Confidence: 0.9},
		},
	}
	got := extractDistinctTermDomains(tg)
	if len(got) != 2 {
		t.Fatalf("expected 2 distinct symbol-domains; got %v", got)
	}
}

func TestExtractDistinctTermDomains_DropsBlankDomains(t *testing.T) {
	tg := types.TermGraph{
		Canonical: []types.CanonicalTerm{
			{Kind: types.TermSymbol, Domain: "", Confidence: 0.9},
			{Kind: types.TermSymbol, Domain: "  ", Confidence: 0.9},
		},
	}
	if got := extractDistinctTermDomains(tg); len(got) != 0 {
		t.Errorf("blank/whitespace domains must be excluded; got %v", got)
	}
}

func TestExtractDistinctTermDomains_Sorted(t *testing.T) {
	tg := types.TermGraph{
		Canonical: []types.CanonicalTerm{
			{Kind: types.TermSymbol, Domain: "z", Confidence: 0.9},
			{Kind: types.TermSymbol, Domain: "a", Confidence: 0.9},
			{Kind: types.TermSymbol, Domain: "m", Confidence: 0.9},
		},
	}
	got := extractDistinctTermDomains(tg)
	for i := 1; i < len(got); i++ {
		if got[i-1] > got[i] {
			t.Errorf("output must be sorted; got %v", got)
		}
	}
}

// ── Run integration ───────────────────────────────────────────────

func TestRun_CoherenceGatesIntegrate_ReadModeOnly(t *testing.T) {
	// Build a write-mode dispatch: coherence gates must be skipped
	// even when the IR carries a structurally inconsistent shape,
	// because write mode has its own SuccessCriteria suite.
	rm := types.RequestModel{
		Predicates: types.SemanticPredicates{IsScalarAnswer: true},
		SubTopics: []types.SubTopic{
			{Summary: "first"},
			{Summary: "second"},
		},
	}
	ir := coherenceFixtureIR(rm)
	// Write-mode bypass — pass any non-empty non-"read" mode.
	report := Run(ir, Thresholds{}, "apply")
	if c := findCheck(report, "shape_subject_coherence"); c != nil {
		t.Errorf("coherence gates must not run in write mode; got %+v", c)
	}
}
