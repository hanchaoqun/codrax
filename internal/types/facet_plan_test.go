package types

import (
	"testing"
)

// ── ResolveQuestionFamily ──────────────────────────────────────────

func TestResolveQuestionFamily_RootCauseTraceWithLog(t *testing.T) {
	rm := RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{},
	}
	if got := ResolveQuestionFamily(rm); got != QFRootCauseTrace {
		t.Errorf("got %q, want QFRootCauseTrace", got)
	}
}

func TestResolveQuestionFamily_RootCauseTraceWithPerf(t *testing.T) {
	rm := RequestModel{
		Intent:    IntentTrace,
		PerfTrace: &PerfBundle{},
	}
	if got := ResolveQuestionFamily(rm); got != QFRootCauseTrace {
		t.Errorf("got %q, want QFRootCauseTrace", got)
	}
}

func TestResolveQuestionFamily_RootCauseWithoutArtifact(t *testing.T) {
	rm := RequestModel{
		Intent:     IntentRootCause,
		Scenario:   ScenarioRootCause,
		Predicates: SemanticPredicates{IsDiagnosticQuestion: true},
	}
	if got := ResolveQuestionFamily(rm); got != QFRootCauseTrace {
		t.Errorf("got %q, want QFRootCauseTrace", got)
	}
}

func TestResolveQuestionFamily_TraceWithoutArtifactGoesToCallChain(t *testing.T) {
	// Trace intent but NO log/perf attached → call chain, not
	// root cause. Distinguishes "trace this panic" from "how does X
	// reach Y".
	rm := RequestModel{Intent: IntentTrace}
	if got := ResolveQuestionFamily(rm); got != QFCallChain {
		t.Errorf("got %q, want QFCallChain", got)
	}
}

func TestResolveQuestionFamily_TraceFunctionSubjectStillGoesToCallChain(t *testing.T) {
	// Explicit trace intent must not be stolen by QFRoleLookup just
	// because the analyzer inferred a function-like AnswerSubject.
	rm := RequestModel{
		Intent:        IntentTrace,
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName, Confidence: 0.8},
	}
	if got := ResolveQuestionFamily(rm); got != QFCallChain {
		t.Errorf("got %q, want QFCallChain", got)
	}
}

func TestResolveQuestionFamily_ConfigPrecedence(t *testing.T) {
	cases := []struct {
		name string
		rm   RequestModel
	}{
		{"intent config_query", RequestModel{Intent: IntentConfigQuery}},
		{"scenario config_trace", RequestModel{Intent: IntentExplain, Scenario: ScenarioConfigTrace}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ResolveQuestionFamily(c.rm); got != QFConfigPrecedence {
				t.Errorf("got %q, want QFConfigPrecedence", got)
			}
		})
	}
}

func TestResolveQuestionFamily_RoleLookup(t *testing.T) {
	cases := []AnswerSubjectKind{
		SubjectFunctionName, SubjectHandlerRoute,
		SubjectConfigKey, SubjectStructField, SubjectInterface,
	}
	for _, kind := range cases {
		t.Run(string(kind), func(t *testing.T) {
			rm := RequestModel{
				Intent:        IntentExplain,
				AnswerSubject: AnswerSubject{Kind: kind, Confidence: 0.8},
			}
			if got := ResolveQuestionFamily(rm); got != QFRoleLookup {
				t.Errorf("got %q, want QFRoleLookup", got)
			}
		})
	}
}

func TestResolveQuestionFamily_ObligationOverridesRoleLookup(t *testing.T) {
	// AnswerSubject says FunctionName (which would go RoleLookup)
	// BUT EnumerationBoundary is set — obligation wins.
	rm := RequestModel{
		Intent:              IntentExplain,
		AnswerSubject:       AnswerSubject{Kind: SubjectFunctionName, Confidence: 0.8},
		EnumerationBoundary: &RequestedEnumerationBoundary{DeclaredCount: 5, SourceQuote: "5 X"},
	}
	if got := ResolveQuestionFamily(rm); got != QFEnumeration {
		t.Errorf("obligation must override role-lookup; got %q want QFEnumeration", got)
	}
}

// TestResolveQuestionFamily_BucketsRouteToComparison: buckets >= 2
// route to QFComparison (R4.4) — pre-R4.4 fell to QFEnumeration
// via the obligation rule + family_underrepresented telemetry.
func TestResolveQuestionFamily_BucketsRouteToComparison(t *testing.T) {
	rm := RequestModel{
		Intent: IntentExplain,
		Buckets: []QuestionBucket{
			{Label: "A", Index: 1},
			{Label: "B", Index: 2},
		},
	}
	if got := ResolveQuestionFamily(rm); got != QFComparison {
		t.Errorf("got %q, want QFComparison", got)
	}
}

func TestResolveQuestionFamily_InferredRequiredFileBucketsRouteToComparison(t *testing.T) {
	rm := RequestModel{
		RawRequest: "对比 explorer.go 和 extractor.go 的 ReAct 循环终止条件有何不同？逐条列出差异。",
		Intent:     IntentExplain,
		Scenario:   ScenarioArchitectureExplain,
		Predicates: SemanticPredicates{IsCrossComponent: true},
		SubTopics: []SubTopic{
			{Summary: "explorer side", Entities: []string{"explorerEvaluator", "ShouldStop"}},
			{Summary: "extractor side", Entities: []string{"extractorEvaluator", "ShouldStop"}},
		},
		AnalyzerHints: AnalyzerHints{
			RequiredFileHints: []RequiredFileHint{
				{Path: "internal/agent/explorer.go", Confidence: 0.95},
				{Path: "internal/agent/extractor.go", Confidence: 0.95},
				{Path: "internal/agent/iteration_cap.go", Confidence: 0.8},
			},
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "逐条"},
	}
	if got := ResolveQuestionFamily(rm); got != QFComparison {
		t.Errorf("got %q, want QFComparison", got)
	}
}

func TestResolveQuestionFamily_EnumerationFromCompleteness(t *testing.T) {
	rm := RequestModel{
		Intent: IntentExplain,
		CompletenessObligation: &CompletenessObligation{
			Required: true, SourceQuote: "all",
		},
	}
	if got := ResolveQuestionFamily(rm); got != QFEnumeration {
		t.Errorf("got %q, want QFEnumeration", got)
	}
}

func TestResolveQuestionFamily_ErrorGranularityDecisionUsesGeneric(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioArchitectureExplain,
		AnalyzerHints: AnalyzerHints{
			Entities: []string{"emit_evidence", "anchor_kind"},
		},
		ErrorGranularityProfile: &ErrorGranularityProfile{
			IsGranularityQuestion: true,
			Confidence:            0.9,
		},
	}
	if got := ResolveQuestionFamily(rm); got != QFGeneric {
		t.Errorf("got %q, want QFGeneric", got)
	}
	rm.Intent = IntentRootCause
	rm.Scenario = ScenarioRootCause
	if got := ResolveQuestionFamily(rm); got != QFGeneric {
		t.Errorf("root-cause-labeled failure-scope decision should still route to generic, got %q", got)
	}
	rm.Intent = IntentExplain
	rm.Scenario = ScenarioArchitectureExplain
	rm.Predicates.IsCategoryEnumeration = true
	if got := ResolveQuestionFamily(rm); got != QFEnumeration {
		t.Errorf("explicit category predicate should still route to enumeration, got %q", got)
	}
}

func TestResolveQuestionFamily_Architecture(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioArchitectureExplain,
	}
	if got := ResolveQuestionFamily(rm); got != QFArchitecture {
		t.Errorf("got %q, want QFArchitecture", got)
	}
}

func TestResolveQuestionFamily_ArchitectureNarrativeBeatsEnumerationDrift(t *testing.T) {
	rm := RequestModel{
		Intent:     IntentExplain,
		Scenario:   ScenarioArchitectureExplain,
		Complexity: ComplexityComplex,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
			IsCrossComponent:      true,
		},
		DiagramHint: &DiagramHint{Kind: DiagramSequence},
		SubTopics: []SubTopic{
			{Summary: "orchestrator scheduling", Entities: []string{"Orchestrator", "PipelineStage"}},
			{Summary: "agent responsibilities", Entities: []string{"AnalyzerAgent", "ExplorerAgent"}},
			{Summary: "context propagation", Entities: []string{"BusContext", "AgentContext"}},
		},
		AnalyzerHints: AnalyzerHints{
			Entities: []string{"Orchestrator", "AnalyzerAgent", "ExplorerAgent", "AgentContext"},
		},
	}
	if got := ResolveQuestionFamily(rm); got != QFArchitecture {
		t.Errorf("architecture narrative with category drift got %q, want QFArchitecture", got)
	}

	rm.CompletenessObligation = &CompletenessObligation{Required: true, SourceQuote: "all components"}
	if got := ResolveQuestionFamily(rm); got != QFEnumeration {
		t.Errorf("explicit completeness obligation should still route to enumeration, got %q", got)
	}
}

func TestResolveQuestionFamily_SingleTopicMechanismExplainUsesGeneric(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		Scenario:      ScenarioArchitectureExplain,
		PredicateAxis: AxisCondition,
		AnalyzerHints: AnalyzerHints{
			Kind:     string(ReqMechanism),
			Entities: []string{"emit_evidence", "anchor_kind", "EmitEvidence"},
		},
		AnswerSubject: AnswerSubject{Kind: SubjectFunctionName, Confidence: 0.9},
	}
	if got := ResolveQuestionFamily(rm); got != QFGeneric {
		t.Errorf("single-topic mechanism explanation got %q, want QFGeneric", got)
	}

	rm.Complexity = ComplexityComplex
	if got := ResolveQuestionFamily(rm); got != QFGeneric {
		t.Errorf("post-reconcile complex single-topic mechanism got %q, want QFGeneric", got)
	}

	rm.Predicates.IsCrossComponent = true
	if got := ResolveQuestionFamily(rm); got != QFGeneric {
		t.Errorf("bare cross-component single-topic mechanism got %q, want QFGeneric", got)
	}
}

func TestResolveQuestionFamily_GenericFallthrough(t *testing.T) {
	// No intent / no scenario / no subject / no obligation → generic.
	rm := RequestModel{}
	if got := ResolveQuestionFamily(rm); got != QFGeneric {
		t.Errorf("got %q, want QFGeneric", got)
	}
}

// ── CompileFacetCoverage ───────────────────────────────────────────

func TestCompileFacetCoverage_HardDegradesToSoftWhenNoCandidate(t *testing.T) {
	// QFRootCauseTrace template has FacetObservedArtifactFact as
	// HARD with AcceptableForms=[ClaimExternalObservation]. Provide
	// non-empty surface evidence whose ClaimForm is NOT
	// ClaimExternalObservation → no candidate matches → HARD must
	// degrade to SOFT.
	//
	// R3.1 (2026-05-04): empty surface is now short-circuited as
	// "inconclusive" (no evidence collected yet ≠ uncoverable), so
	// the downgrade test must pass NON-empty surface that fails
	// the AcceptableForms match — the real "evidence collected
	// but doesn't fit" case the rule targets.
	rm := RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{},
	}
	// AnchorDefinition → ClaimDefinitionFact, NOT acceptable for
	// FacetObservedArtifactFact (which requires
	// ClaimExternalObservation).
	surface := []EvidenceItem{
		{ID: "e1", Source: "x.go", AnchorKind: AnchorDefinition},
	}
	plan := CompileFacetCoverage(rm, surface)
	if plan == nil {
		t.Fatal("plan must be non-nil for QFRootCauseTrace")
	}
	if plan.Family != QFRootCauseTrace {
		t.Errorf("family = %q, want QFRootCauseTrace", plan.Family)
	}
	for _, req := range plan.Required {
		if req.Kind == FacetObservedArtifactFact && req.Required == FacetHardRequired {
			t.Errorf("FacetObservedArtifactFact must degrade to SOFT when no candidate; got HARD")
		}
	}
}

func TestCompileFacetCoverage_UncertaintyBoundaryIgnoresOrdinaryDefinitionEvidence(t *testing.T) {
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}
	surface := []EvidenceItem{
		{ID: "def", Source: "internal/analysis/criterion/grammar.go", LineStart: 25, Scope: ScopeLine, AnchorKind: AnchorDefinition},
	}
	plan := CompileFacetCoverage(rm, surface)
	if plan == nil {
		t.Fatal("plan must be non-nil")
	}
	var boundary *FacetRequirement
	for i := range plan.Required {
		if plan.Required[i].Kind == FacetUncertaintyBoundary {
			boundary = &plan.Required[i]
			break
		}
	}
	if boundary == nil {
		t.Fatal("enumeration should still carry the advisory uncertainty boundary facet")
	}
	if len(boundary.SourceCandidate) != 0 {
		t.Fatalf("ordinary definition evidence must not promote uncertainty boundary, got candidates %+v", boundary.SourceCandidate)
	}
	if boundary.IsPromoted() {
		t.Fatalf("uncertainty boundary must stay unpromoted without absence/external/text-reference evidence: %+v", boundary)
	}
}

func TestCompileFacetCoverage_UncertaintyBoundaryPromotesOnNegativeEvidence(t *testing.T) {
	rm := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}
	surface := []EvidenceItem{
		{
			ID:            "absent",
			Source:        "internal/analysis/criterion",
			Scope:         ScopeNegative,
			NegativeScope: NegativeScopeFile,
			NegativeQuery: &NegativeQuery{File: "internal/analysis/criterion", Pattern: "DeprecatedKind"},
		},
	}
	plan := CompileFacetCoverage(rm, surface)
	if plan == nil {
		t.Fatal("plan must be non-nil")
	}
	var boundary *FacetRequirement
	for i := range plan.Required {
		if plan.Required[i].Kind == FacetUncertaintyBoundary {
			boundary = &plan.Required[i]
			break
		}
	}
	if boundary == nil {
		t.Fatal("enumeration should carry the advisory uncertainty boundary facet")
	}
	if len(boundary.SourceCandidate) != 1 || boundary.SourceCandidate[0] != "absent" {
		t.Fatalf("negative search evidence should promote uncertainty boundary, got %+v", boundary)
	}
	if !boundary.IsPromoted() {
		t.Fatalf("uncertainty boundary should promote when absence evidence exists: %+v", boundary)
	}
}

func TestCompileFacetCoverage_ExternalOnlyRuntimeDoesNotPromoteCurrentCodePath(t *testing.T) {
	rm := RequestModel{
		Intent: IntentRootCause,
		LogTriage: &LogBundle{
			Errors: []LogError{{Type: "panic", Frames: []LogFrame{{
				File: "src/bridge/Bridge.cj",
				Line: 18,
				Func: "demo.bridge.ohSum",
			}}}},
			ResolvedFiles: nil,
		},
	}
	surface := []EvidenceItem{
		{ID: "repo-helper", Source: "internal/analysis/logtriage/resolver_harmonyos.go", LineStart: 27, AnchorKind: AnchorDefinition},
	}
	plan := CompileFacetCoverage(rm, surface)
	if plan == nil {
		t.Fatal("plan must be non-nil")
	}
	for _, req := range plan.Required {
		if req.Kind == FacetCurrentCodePath {
			t.Fatalf("external-only runtime artifact must not keep current_code_path in Required facets: %+v", req)
		}
	}
	var current *FacetRequirement
	for i := range plan.Optional {
		if plan.Optional[i].Kind == FacetCurrentCodePath {
			current = &plan.Optional[i]
			break
		}
	}
	if current == nil {
		t.Fatal("current_code_path should remain optional enrichment for external-only artifacts")
	}
	if current.EffectivePromotionPolicy() != PromotionAdvisoryOnly || current.IsPromoted() {
		t.Fatalf("external-only current_code_path should be advisory-only, got policy=%s promoted=%v", current.EffectivePromotionPolicy(), current.IsPromoted())
	}
	if len(current.SourceCandidate) != 0 {
		t.Fatalf("external-only observation answers must not bind unrelated current-repo evidence as current_code_path candidates: %+v", current.SourceCandidate)
	}
}

// TestCompileFacetCoverage_EmptySurfaceInconclusiveNoSoftening
// pins R3.1 (post_shape_residual_audit.md, 2026-05-04): when
// CompileFacetCoverage runs at analyzer-time (before any emit_evidence
// call) surface is empty and we MUST NOT silently downgrade
// HARD facets — the lack of evidence is "not yet collected", not
// "facet uncoverable". Telemetry MUST also stay silent.
func TestCompileFacetCoverage_EmptySurfaceInconclusiveNoSoftening(t *testing.T) {
	rm := RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{},
	}
	sink := &fakeRichnessSink{}
	// nil surface — analyzer-time / pre-evidence call shape.
	plan := CompileFacetCoverage(rm, nil, sink)
	if plan == nil {
		t.Fatal("plan must be non-nil")
	}
	hardKept := false
	for _, req := range plan.Required {
		if req.Kind == FacetObservedArtifactFact && req.Required == FacetHardRequired {
			hardKept = true
		}
	}
	if !hardKept {
		t.Errorf("HARD facet must remain HARD on empty surface (R3.1 inconclusive); got softened")
	}
	for _, sig := range sink.signals {
		if sig.Kind == "facet_softened" {
			t.Errorf("R3.1 must skip facet_softened telemetry on empty surface; got %+v", sig)
		}
	}
}

func TestCompileFacetCoverage_RootCauseWithoutArtifactDoesNotRequireObservedArtifact(t *testing.T) {
	rm := RequestModel{
		Intent:     IntentRootCause,
		Scenario:   ScenarioRootCause,
		Predicates: SemanticPredicates{IsDiagnosticQuestion: true},
	}
	plan := CompileFacetCoverage(rm, nil)
	if plan == nil {
		t.Fatal("plan must be non-nil")
	}
	if plan.Family != QFRootCauseTrace {
		t.Fatalf("family = %q, want QFRootCauseTrace", plan.Family)
	}
	for _, req := range plan.Required {
		if req.Kind == FacetObservedArtifactFact && req.Required == FacetHardRequired {
			t.Fatalf("no-attachment root cause must not hard-require observed artifact; got %+v", req)
		}
	}
}

func TestCompileFacetCoverage_HardKeptWhenCandidateExists(t *testing.T) {
	// Provide a LogFrame-origin EvidenceItem so
	// FacetObservedArtifactFact has a SourceCandidate.
	rm := RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{},
	}
	surface := []EvidenceItem{
		{ID: "ev-log-1", Origin: ClaimOriginLog, Source: "panic.log"},
	}
	plan := CompileFacetCoverage(rm, surface)
	if plan == nil {
		t.Fatal("plan must be non-nil")
	}
	foundHard := false
	for _, req := range plan.Required {
		if req.Kind == FacetObservedArtifactFact &&
			req.Required == FacetHardRequired &&
			contains(req.SourceCandidate, "ev-log-1") {
			foundHard = true
		}
	}
	if !foundHard {
		t.Errorf("FacetObservedArtifactFact must keep HARD with bound candidate; got %+v", plan.Required)
	}
}

func TestCompileFacetCoverage_BucketLabelInjectedFromQuestionStructure(t *testing.T) {
	rm := RequestModel{
		Intent: IntentExplain,
		Buckets: []QuestionBucket{
			{Label: "A", Index: 1}, {Label: "B", Index: 2},
		},
	}
	plan := CompileFacetCoverage(rm, nil)
	if plan == nil {
		t.Fatal("plan must be non-nil")
	}
	// Bucket label is HARD-required regardless of candidate presence
	// (bucket labels are user-named partitions, not evidence-bound)
	// — but the Phase-1 fallback rule degrades to SOFT when no
	// candidate exists. We check that the requirement APPEARS
	// somewhere (HARD or SOFT) so finalizer prompt can surface it.
	found := false
	for _, req := range append(plan.Required, plan.Optional...) {
		if req.Kind == FacetBucketLabel {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("FacetBucketLabel must appear when Buckets >= 2; got %+v", plan)
	}
}

func TestCompileFacetCoverage_EnumerationItemFromCount(t *testing.T) {
	rm := RequestModel{
		Intent:              IntentExplain,
		EnumerationBoundary: &RequestedEnumerationBoundary{DeclaredCount: 7, SourceQuote: "7 X"},
	}
	plan := CompileFacetCoverage(rm, nil)
	if plan == nil {
		t.Fatal("plan must be non-nil")
	}
	found := false
	for _, req := range append(plan.Required, plan.Optional...) {
		if req.Kind == FacetEnumerationItem {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("FacetEnumerationItem must appear when EnumerationBoundary set; got %+v", plan)
	}
}

func TestCompileFacetCoverage_GenericMinimal(t *testing.T) {
	plan := CompileFacetCoverage(RequestModel{}, nil)
	if plan == nil {
		t.Fatal("plan must be non-nil for QFGeneric")
	}
	if plan.Family != QFGeneric {
		t.Errorf("family = %q, want QFGeneric", plan.Family)
	}
}

func TestCompileFacetCoverage_NilOnNoTemplate(t *testing.T) {
	// Sanity: family resolver must not return an unmapped family
	// that yields nil template — guards against future
	// QuestionFamily additions without template definitions.
	for _, f := range []QuestionFamily{
		QFRootCauseTrace, QFConfigPrecedence, QFRoleLookup,
		QFCallChain, QFEnumeration, QFArchitecture, QFGeneric,
	} {
		t.Run(string(f), func(t *testing.T) {
			rm := familyToRequestModel(f)
			plan := CompileFacetCoverage(rm, nil)
			if plan == nil {
				t.Errorf("family %q yields nil plan — template missing", f)
			}
		})
	}
}

// ── claimFormMatches helper ────────────────────────────────────────

func TestClaimFormMatches_EmptyAllowedAcceptsNonUnknown(t *testing.T) {
	if !claimFormMatches(nil, ClaimDefinitionFact) {
		t.Error("empty allowed must accept any non-unknown")
	}
	if claimFormMatches(nil, ClaimUnknown) {
		t.Error("empty allowed must reject unknown")
	}
}

func TestClaimFormMatches_WhitelistFiltering(t *testing.T) {
	allowed := []ClaimForm{ClaimDefinitionFact, ClaimCallEdge}
	if !claimFormMatches(allowed, ClaimDefinitionFact) {
		t.Error("whitelist must accept listed form")
	}
	if !claimFormMatches(allowed, ClaimCallEdge) {
		t.Error("whitelist must accept listed form")
	}
	if claimFormMatches(allowed, ClaimGuardCondition) {
		t.Error("whitelist must reject non-listed form")
	}
}

// ── helpers ────────────────────────────────────────────────────────

func contains(s []string, target string) bool {
	for _, v := range s {
		if v == target {
			return true
		}
	}
	return false
}

// ── B5-F2 / B5-F3 richness telemetry tests ─────────────────────────

// fakeRichnessSink captures every signal that flows through the
// CompileFacetCoverage / ResolveQuestionFamily channel for a single
// test case. Mirrors MutableState.AppendRichnessTelemetry's surface
// (the production sink) so the production deduper does not interfere
// with assertions.
type fakeRichnessSink struct {
	signals []RichnessTelemetrySignal
}

func (f *fakeRichnessSink) AppendRichnessTelemetry(sig RichnessTelemetrySignal) {
	f.signals = append(f.signals, sig)
}

func TestCompileFacetCoverage_FacetSofteningTelemetryFires(t *testing.T) {
	// QFRootCauseTrace + non-empty surface evidence that DOESN'T
	// match the FacetObservedArtifactFact AcceptableForms (which
	// requires ClaimExternalObservation only). With a sink wired,
	// we expect at least one "facet_softened" signal.
	//
	// R3.1 (2026-05-04): emptySurface=nil now short-circuits the
	// downgrade path (treat as inconclusive — analyzer-time call
	// before any emit_evidence). The test must pass NON-empty
	// surface that fails to satisfy the facet AcceptableForms,
	// which is the genuine "evidence collected but doesn't match"
	// case the softening rule was designed for.
	rm := RequestModel{
		Intent:    IntentRootCause,
		LogTriage: &LogBundle{},
	}
	// AnchorDefinition → ClaimDefinitionFact, NOT in
	// FacetObservedArtifactFact AcceptableForms (which is
	// ClaimExternalObservation only).
	surface := []EvidenceItem{
		{ID: "e1", Source: "x.go", AnchorKind: AnchorDefinition},
	}
	sink := &fakeRichnessSink{}
	plan := CompileFacetCoverage(rm, surface, sink)
	if plan == nil {
		t.Fatal("plan must be non-nil")
	}
	gotSoftened := false
	for _, sig := range sink.signals {
		if sig.Kind == "facet_softened" {
			gotSoftened = true
			if sig.Family != string(QFRootCauseTrace) {
				t.Errorf("family on signal = %q, want QFRootCauseTrace", sig.Family)
			}
			if sig.FacetKind == "" {
				t.Errorf("facet_kind must be populated; got empty signal %+v", sig)
			}
			if sig.Reason == "" {
				t.Errorf("reason must be populated; got empty signal %+v", sig)
			}
		}
	}
	if !gotSoftened {
		t.Errorf("no facet_softened signal recorded; got %+v", sink.signals)
	}
}

func TestCompileFacetCoverage_NoSofteningWhenAllCovered(t *testing.T) {
	// QFRoleLookup + a candidate that satisfies the facet → no
	// HARD→SOFT degradation, no signal.
	rm := RequestModel{
		Intent: IntentExplain,
		AnswerSubject: AnswerSubject{
			Kind: SubjectFunctionName, Confidence: 0.8,
		},
	}
	surface := []EvidenceItem{
		{ID: "e1", Source: "x.go", AnchorKind: AnchorDefinition, Summary: "definition"},
	}
	sink := &fakeRichnessSink{}
	_ = CompileFacetCoverage(rm, surface, sink)
	for _, sig := range sink.signals {
		if sig.Kind == "facet_softened" {
			t.Errorf("unexpected softening signal on satisfied requirement: %+v", sig)
		}
	}
}

// TestResolveQuestionFamily_BucketedRoutesToComparison pins the
// R4.4 QFComparison routing: any RequestModel with Buckets >= 2
// routes to QFComparison regardless of Intent (comparison takes
// priority over enumeration / call-chain / generic so the user's
// mental partition survives end-to-end).
func TestResolveQuestionFamily_BucketedRoutesToComparison(t *testing.T) {
	cases := []struct {
		name   string
		intent Intent
	}{
		{"with_enumerate_intent", IntentEnumerate},
		{"with_explain_intent", IntentExplain},
		{"with_trace_intent", IntentTrace},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rm := RequestModel{
				Intent: tc.intent,
				Buckets: []QuestionBucket{
					{Label: "read mode", Index: 1},
					{Label: "write mode", Index: 2},
				},
			}
			got := ResolveQuestionFamily(rm)
			if got != QFComparison {
				t.Errorf("intent=%s: family = %q, want QFComparison", tc.intent, got)
			}
		})
	}
}

// TestResolveQuestionFamily_SingleBucketDoesNotRouteToComparison
// confirms only Buckets >= 2 triggers QFComparison;
// single-bucket / no-bucket questions route by Intent as before.
func TestResolveQuestionFamily_SingleBucketDoesNotRouteToComparison(t *testing.T) {
	rm := RequestModel{
		Intent: IntentEnumerate,
		Buckets: []QuestionBucket{
			{Label: "only one", Index: 1},
		},
	}
	got := ResolveQuestionFamily(rm)
	if got == QFComparison {
		t.Errorf("single-bucket must NOT route to QFComparison; got %q", got)
	}
	if got != QFEnumeration {
		t.Errorf("single-bucket enumeration: got %q, want QFEnumeration", got)
	}
}

func TestResolveQuestionFamily_NoSignalWhenSinkNil(t *testing.T) {
	// Calling without a sink must not panic and must not invoke any
	// telemetry side-effect — back-compat for tests / non-orchestrator
	// callers.
	rm := RequestModel{
		Intent: IntentEnumerate,
		Buckets: []QuestionBucket{
			{Label: "read mode", Index: 1},
			{Label: "write mode", Index: 2},
		},
	}
	_ = ResolveQuestionFamily(rm)
	_ = CompileFacetCoverage(rm, nil)
}

// TestMutableState_AppendRichnessTelemetry_Dedups confirms identical
// signals only land once even when many call sites fire.
func TestMutableState_AppendRichnessTelemetry_Dedups(t *testing.T) {
	mut := &MutableState{}
	sig := RichnessTelemetrySignal{Kind: "facet_softened", FacetKind: "X", Family: "QFGeneric", Reason: "r"}
	mut.AppendRichnessTelemetry(sig)
	mut.AppendRichnessTelemetry(sig)
	mut.AppendRichnessTelemetry(sig)
	got := mut.RichnessTelemetry()
	if len(got) != 1 {
		t.Errorf("expected dedup → len=1; got %d (%+v)", len(got), got)
	}

	// A signal that differs in any field is recorded separately.
	mut.AppendRichnessTelemetry(RichnessTelemetrySignal{
		Kind: "facet_softened", FacetKind: "Y", Family: "QFGeneric", Reason: "r",
	})
	if got := mut.RichnessTelemetry(); len(got) != 2 {
		t.Errorf("expected len=2 after distinct signal; got %d", len(got))
	}
}

// ───────────────────────────────────────────────────────────────────

func familyToRequestModel(f QuestionFamily) RequestModel {
	switch f {
	case QFRootCauseTrace:
		return RequestModel{Intent: IntentRootCause, LogTriage: &LogBundle{}}
	case QFConfigPrecedence:
		return RequestModel{Intent: IntentConfigQuery}
	case QFRoleLookup:
		return RequestModel{
			Intent:        IntentExplain,
			AnswerSubject: AnswerSubject{Kind: SubjectFunctionName, Confidence: 0.8},
		}
	case QFCallChain:
		return RequestModel{Intent: IntentTrace}
	case QFEnumeration:
		return RequestModel{Intent: IntentEnumerate}
	case QFArchitecture:
		return RequestModel{
			Intent: IntentExplain, Scenario: ScenarioArchitectureExplain,
		}
	case QFGeneric:
		return RequestModel{}
	}
	return RequestModel{}
}

// ── Phase 5-E0: SourceCandidate provenance lock ──────────────────

// TestBindSourceCandidates_FiltersByTypedClaimForm pins R3:
// bindSourceCandidates uses claimFormMatches over the typed
// ClaimForm enum, NOT similarity / frequency / heuristic. An
// evidence item whose projected ClaimForm is not in AcceptableForms
// MUST be excluded — even if its other fields look "close".
func TestBindSourceCandidates_FiltersByTypedClaimForm(t *testing.T) {
	req := FacetRequirement{
		Kind:            FacetCurrentCodePath,
		AcceptableForms: []ClaimForm{ClaimCallEdge}, // narrow whitelist
	}
	surface := []EvidenceItem{
		{ID: "ev-call-1", AnchorKind: AnchorCall, Subject: "fn1"},      // → ClaimCallEdge
		{ID: "ev-def-1", AnchorKind: AnchorDefinition, Subject: "fn1"}, // → ClaimDefinitionFact (rejected)
	}
	bound := bindSourceCandidates(req, surface)
	if len(bound.SourceCandidate) != 1 || bound.SourceCandidate[0] != "ev-call-1" {
		t.Errorf("AcceptableForms=ClaimCallEdge must accept exactly ev-call-1; got %v", bound.SourceCandidate)
	}
}

// TestBindSourceCandidates_FiltersByTypedSubKind pins the typed
// LogPerfSubKind filter (not similarity). When RequiredSubKind is
// set, only evidence with the matching enum value passes.
func TestBindSourceCandidates_FiltersByTypedSubKind(t *testing.T) {
	req := FacetRequirement{
		Kind:            FacetObservedArtifactFact,
		AcceptableForms: []ClaimForm{ClaimExternalObservation},
		RequiredSubKind: LogPanicFrame,
	}
	surface := []EvidenceItem{
		{ID: "ev-panic", Origin: ClaimOriginLog, LogPerfSubKind: LogPanicFrame},
		{ID: "ev-noise", Origin: ClaimOriginLog, LogPerfSubKind: LogNoiseFrame},
	}
	bound := bindSourceCandidates(req, surface)
	if len(bound.SourceCandidate) != 1 || bound.SourceCandidate[0] != "ev-panic" {
		t.Errorf("RequiredSubKind=LogPanicFrame must accept only ev-panic; got %v", bound.SourceCandidate)
	}
}

// TestBindSourceCandidates_OutputAllInputIDs pins the provenance
// invariant: every SourceCandidate entry MUST be the ID of an
// evidence item in the input surface. No fabrication, no derived
// IDs from prose.
func TestBindSourceCandidates_OutputAllInputIDs(t *testing.T) {
	req := FacetRequirement{
		Kind:            FacetCurrentCodePath,
		AcceptableForms: nil, // empty = any non-Unknown claim form
	}
	surface := []EvidenceItem{
		{ID: "ev-1", AnchorKind: AnchorCall},
		{ID: "ev-2", AnchorKind: AnchorDefinition},
		{ID: "ev-3", AnchorKind: AnchorReturn},
	}
	bound := bindSourceCandidates(req, surface)
	inputIDs := map[string]bool{"ev-1": true, "ev-2": true, "ev-3": true}
	for _, id := range bound.SourceCandidate {
		if !inputIDs[id] {
			t.Errorf("SourceCandidate %q is NOT in input surface — fabrication / drift", id)
		}
	}
}

func TestCompileFacetCoverage_ConfigPrecedenceRoleUsesCoverageRoles(t *testing.T) {
	rm := RequestModel{
		RawRequest: "explore_mid_loop_hint_budget 的 code default / codrax.yaml / CLI 覆盖优先级是什么？",
		Intent:     IntentConfigQuery,
		Scenario:   ScenarioConfigTrace,
		AnswerSubject: AnswerSubject{
			Kind: SubjectConfigKey,
		},
		AnalyzerHints: AnalyzerHints{
			ExactTargets:      []string{"explore_mid_loop_hint_budget"},
			ExactContextRoles: []EvidenceDiagramRole{EvidenceDiagramRoleDefault, EvidenceDiagramRoleConfig, EvidenceDiagramRoleOverride},
		},
	}
	surface := []EvidenceItem{
		{
			ID:              "ev-default",
			Source:          "internal/types/config.go",
			LineStart:       943,
			Scope:           ScopeLine,
			GroundingStatus: GroundingGrounded,
			AnchorKind:      AnchorDefinition,
			DiagramRole:     EvidenceDiagramRoleDefault,
			Subject:         "ExploreMidLoopMinIteration",
			Summary:         "default midloop heuristics",
		},
		{
			ID:              "ev-config",
			Source:          "codrax.yaml.example",
			LineStart:       22,
			Scope:           ScopeLine,
			GroundingStatus: GroundingGrounded,
			AnchorKind:      AnchorDefinition,
			DiagramRole:     EvidenceDiagramRoleConfig,
		},
		{
			ID:              "ev-cli",
			Source:          "cmd/root.go",
			Scope:           ScopeFile,
			GroundingStatus: GroundingGrounded,
			FileRoleLabel:   FileRoleCLIRegistration,
		},
		{
			ID:              "ev-noise",
			Source:          "internal/skill/glossary.go",
			LineStart:       35,
			Scope:           ScopeLine,
			GroundingStatus: GroundingGrounded,
			AnchorKind:      AnchorDefinition,
			DiagramRole:     EvidenceDiagramRoleDefault,
		},
	}
	plan := CompileFacetCoverage(rm, surface)
	if plan == nil {
		t.Fatal("CompileFacetCoverage returned nil")
	}
	var got *FacetRequirement
	for i := range plan.Required {
		if plan.Required[i].Kind == FacetConfigPrecedenceRole {
			got = &plan.Required[i]
			break
		}
	}
	if got == nil {
		t.Fatal("FacetConfigPrecedenceRole missing from required set")
	}
	for _, want := range []string{"ev-default", "ev-config", "ev-cli"} {
		if !contains(got.SourceCandidate, want) {
			t.Fatalf("config precedence facet missing source candidate %q: %v", want, got.SourceCandidate)
		}
	}
	if contains(got.SourceCandidate, "ev-noise") {
		t.Fatalf("auxiliary evidence must not become precedence source candidate: %v", got.SourceCandidate)
	}
}

// TestStableEvidenceID_DeterministicOverTypedFields pins the
// EvidenceItem.ID provenance: identical typed fields produce
// identical IDs (deterministic projection, suitable as hard gate
// input). Different typed fields produce different IDs.
func TestStableEvidenceID_DeterministicOverTypedFields(t *testing.T) {
	a := EvidenceItem{Kind: EvidenceDirect, Subject: "X", Source: "f.go", LineStart: 10, LineEnd: 10}
	b := EvidenceItem{Kind: EvidenceDirect, Subject: "X", Source: "f.go", LineStart: 10, LineEnd: 10}
	c := EvidenceItem{Kind: EvidenceDirect, Subject: "Y", Source: "f.go", LineStart: 10, LineEnd: 10}
	if StableEvidenceID(a) != StableEvidenceID(b) {
		t.Error("identical typed fields must produce identical IDs (deterministic)")
	}
	if StableEvidenceID(a) == StableEvidenceID(c) {
		t.Error("different Subject must produce different IDs")
	}
}

// ── G6 (post_v2_runtime_gap_remediation, 2026-05-04) — promotion policy

func TestFacetPromotionPolicy_FromRequiredness_OneToOne(t *testing.T) {
	cases := []struct {
		req  FacetRequiredness
		want FacetPromotionPolicy
	}{
		{FacetHardRequired, PromotionAlwaysHard},
		{FacetSoftRequired, PromotionWhenEvidenceSufficient},
		{FacetOptional, PromotionAdvisoryOnly},
		// Unknown / zero-value defaults to AdvisoryOnly (safe — no
		// surprise hard gating on a hand-constructed FacetRequirement).
		{FacetRequiredness(""), PromotionAdvisoryOnly},
	}
	for _, c := range cases {
		if got := PromotionPolicyFromRequiredness(c.req); got != c.want {
			t.Errorf("PromotionPolicyFromRequiredness(%q) = %q, want %q", c.req, got, c.want)
		}
	}
}

func TestFacetPromotionPolicy_IsValid(t *testing.T) {
	for _, p := range []FacetPromotionPolicy{
		PromotionAlwaysHard, PromotionWhenEvidenceSufficient, PromotionAdvisoryOnly,
	} {
		if !p.IsValid() {
			t.Errorf("%q must be valid", p)
		}
	}
	if FacetPromotionPolicy("").IsValid() {
		t.Error("empty must be invalid")
	}
	if FacetPromotionPolicy("foo").IsValid() {
		t.Error("bogus must be invalid")
	}
}

// EssentialAlwaysPromoted: TierEssential / PromotionAlwaysHard
// promotes to HARD regardless of evidence (analyzer template
// pinned the facet as essential).
func TestFacetRequirement_IsPromoted_EssentialAlwaysHard(t *testing.T) {
	req := FacetRequirement{
		Required:        FacetHardRequired,
		PromotionPolicy: PromotionAlwaysHard,
		// SourceCandidate empty
	}
	if !req.IsPromoted() {
		t.Errorf("Essential must promote regardless of evidence")
	}
}

func TestFacetRequirement_IsPromoted_ExpectedWithEvidencePromotes(t *testing.T) {
	req := FacetRequirement{
		Required:                FacetSoftRequired,
		PromotionPolicy:         PromotionWhenEvidenceSufficient,
		MinEvidenceForPromotion: 1,
		SourceCandidate:         []string{"ev-1", "ev-2"},
	}
	if !req.IsPromoted() {
		t.Errorf("Expected with len(SourceCandidate)=2 ≥ Min=1 must promote")
	}
}

func TestFacetRequirement_IsPromoted_ExpectedWithoutEvidenceSkips(t *testing.T) {
	req := FacetRequirement{
		Required:                FacetSoftRequired,
		PromotionPolicy:         PromotionWhenEvidenceSufficient,
		MinEvidenceForPromotion: 1,
		SourceCandidate:         nil,
	}
	if req.IsPromoted() {
		t.Errorf("Expected with empty SourceCandidate must NOT promote (avoid forcing fabrication)")
	}
}

func TestFacetRequirement_IsPromoted_EnrichmentNeverPromotes(t *testing.T) {
	req := FacetRequirement{
		Required:        FacetOptional,
		PromotionPolicy: PromotionAdvisoryOnly,
		// Even with abundant evidence:
		SourceCandidate: []string{"ev-1", "ev-2", "ev-3", "ev-4", "ev-5", "ev-6", "ev-7", "ev-8", "ev-9", "ev-10"},
	}
	if req.IsPromoted() {
		t.Errorf("Enrichment must NEVER promote regardless of evidence (R3 noisy-signal protection)")
	}
}

// EffectiveMinEvidenceForPromotion defaults to
// DefaultMinEvidenceForPromotion when zero (back-compat for
// hand-constructed FacetRequirements).
func TestFacetRequirement_EffectiveMinEvidenceForPromotion_Default(t *testing.T) {
	req := FacetRequirement{} // all zero
	if got := req.EffectiveMinEvidenceForPromotion(); got != DefaultMinEvidenceForPromotion {
		t.Errorf("zero MinEvidenceForPromotion → %d, want %d (default)", got, DefaultMinEvidenceForPromotion)
	}
	req2 := FacetRequirement{MinEvidenceForPromotion: 3}
	if got := req2.EffectiveMinEvidenceForPromotion(); got != 3 {
		t.Errorf("explicit Min=3 → %d, want 3", got)
	}
}

// CompileFacetCoverage MUST fill PromotionPolicy + MinEvidenceForPromotion
// 1:1 from Requiredness on every emitted FacetRequirement (Required
// list AND Optional list).
func TestCompileFacetCoverage_FillsPromotionPolicy(t *testing.T) {
	rm := RequestModel{
		Intent: IntentTrace,
	}
	plan := CompileFacetCoverage(rm, []EvidenceItem{
		{Kind: EvidenceDirect, Subject: "X", Source: "f.go", LineStart: 10, LineEnd: 10},
	})
	if plan == nil {
		t.Fatal("plan is nil — fixture mis-configured")
	}
	for _, req := range plan.Required {
		want := PromotionPolicyFromRequiredness(req.Required)
		if req.PromotionPolicy != want {
			t.Errorf("Required facet %q: PromotionPolicy=%q, want %q", req.Kind, req.PromotionPolicy, want)
		}
		if req.MinEvidenceForPromotion <= 0 {
			t.Errorf("Required facet %q: MinEvidenceForPromotion=%d, want >=1 (default fill)", req.Kind, req.MinEvidenceForPromotion)
		}
	}
	for _, req := range plan.Optional {
		if req.PromotionPolicy != PromotionAdvisoryOnly {
			t.Errorf("Optional facet %q: PromotionPolicy=%q, want PromotionAdvisoryOnly", req.Kind, req.PromotionPolicy)
		}
	}
}

// EffectivePromotionPolicy back-compat accessor: hand-constructed
// FacetRequirement (no compile) still surfaces the right policy.
func TestFacetRequirement_EffectivePromotionPolicy_BackCompat(t *testing.T) {
	cases := []struct {
		req  FacetRequirement
		want FacetPromotionPolicy
	}{
		{FacetRequirement{Required: FacetHardRequired}, PromotionAlwaysHard},
		{FacetRequirement{Required: FacetSoftRequired}, PromotionWhenEvidenceSufficient},
		{FacetRequirement{Required: FacetOptional}, PromotionAdvisoryOnly},
		// Explicit override stays:
		{FacetRequirement{Required: FacetHardRequired, PromotionPolicy: PromotionAdvisoryOnly}, PromotionAdvisoryOnly},
	}
	for i, c := range cases {
		if got := c.req.EffectivePromotionPolicy(); got != c.want {
			t.Errorf("case %d: got %q, want %q", i, got, c.want)
		}
	}
}
