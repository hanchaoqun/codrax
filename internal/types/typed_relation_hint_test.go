package types

import "testing"

// TestTypedRelationAnchorKindCoverage is the structural gate: every
// value in AllTypedRelations() MUST have a non-empty AnchorKind
// mapping in TypedRelationAnchorKind. Adding a new relation forces
// updating both surfaces.
func TestTypedRelationAnchorKindCoverage(t *testing.T) {
	for _, rel := range AllTypedRelations() {
		ak := TypedRelationAnchorKind(rel)
		if ak == "" {
			t.Errorf("relation %q has empty AnchorKind mapping; add it to TypedRelationAnchorKind switch", rel)
			continue
		}
		// Validate against the closed AnchorKind taxonomy.
		valid := false
		for _, declared := range AllAnchorKinds() {
			if ak == declared {
				valid = true
				break
			}
		}
		if !valid {
			t.Errorf("relation %q maps to AnchorKind %q which is not in AllAnchorKinds; pick a declared kind", rel, ak)
		}
	}
}

// TestTypedRelationAnchorKindUnknownReturnsEmpty pins the fallback:
// unknown relation strings must return "" so dedup never accidentally
// matches against a typo'd value.
func TestTypedRelationAnchorKindUnknownReturnsEmpty(t *testing.T) {
	if got := TypedRelationAnchorKind("not_a_relation"); got != "" {
		t.Errorf("expected empty AnchorKind for unknown relation; got %q", got)
	}
	if got := TypedRelationAnchorKind(""); got != "" {
		t.Errorf("expected empty AnchorKind for empty relation; got %q", got)
	}
}

func TestTypedRelationPrecisionCoverageGateEligibility(t *testing.T) {
	tests := []struct {
		name      string
		precision TypedRelationPrecision
		want      bool
	}{
		{"exact symbol id", TypedRelationPrecisionExactSymbolID, true},
		{"exact file", TypedRelationPrecisionExactFile, true},
		{"exact evidence", TypedRelationPrecisionExactEvidence, true},
		{"name only", TypedRelationPrecisionNameOnly, false},
		{"heuristic", TypedRelationPrecisionHeuristic, false},
		{"empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.precision.CoverageGateEligible(); got != tt.want {
				t.Fatalf("CoverageGateEligible()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestTypedRelationQueryAllowsKind(t *testing.T) {
	if !((TypedRelationQuery{}).AllowsKind(TypedRelationImplements)) {
		t.Fatalf("empty kind filter should allow known relations")
	}
	q := TypedRelationQuery{Kinds: []TypedRelationKind{TypedRelationImports}}
	if !q.AllowsKind(TypedRelationImports) {
		t.Fatalf("explicit kind filter should allow listed relation")
	}
	if q.AllowsKind(TypedRelationImplements) {
		t.Fatalf("explicit kind filter should reject unlisted relation")
	}
	if q.AllowsKind("") {
		t.Fatalf("empty relation kind must never be allowed")
	}
}

func TestBuildTypedRelationQuery_SelectsKindsFromTypedFields(t *testing.T) {
	rm := RequestModel{
		PredicateAxis: AxisImplement,
		AnalyzerHints: AnalyzerHints{PrimaryEntities: []string{"Looper"}},
	}
	q := BuildTypedRelationQuery(rm, TypedRelationPurposePromptHint, 17)
	assertTypedRelationKinds(t, q.Kinds, TypedRelationImplements)
	assertStringSlice(t, q.Sources, []string{"Looper"})
	if q.Purpose != TypedRelationPurposePromptHint || q.MaxMembers != 17 || q.Request == nil {
		t.Fatalf("query metadata not preserved: %+v", q)
	}

	rm.PredicateAxis = AxisCall
	q = BuildTypedRelationQuery(rm, TypedRelationPurposePromptHint, 0)
	assertTypedRelationKinds(t, q.Kinds, TypedRelationCalledBy)

	rm.PredicateAxis = AxisRegister
	q = BuildTypedRelationQuery(rm, TypedRelationPurposePromptHint, 0)
	assertTypedRelationKinds(t, q.Kinds, TypedRelationRegisters)

	rm.PredicateAxis = AxisConfigure
	q = BuildTypedRelationQuery(rm, TypedRelationPurposePromptHint, 0)
	assertTypedRelationKinds(t, q.Kinds, TypedRelationConfigures)

	q = BuildTypedRelationQuery(rm, TypedRelationPurposeCoverageGate, 0)
	if len(q.Kinds) != 0 {
		t.Fatalf("configure relation is prompt guidance until an exact hard-gate contract exists, got %+v", q.Kinds)
	}
}

func TestBuildTypedRelationQuery_SelectsCallRelationFromRequirementKind(t *testing.T) {
	rm := RequestModel{
		AnalyzerHints: AnalyzerHints{
			Kind:            string(ReqCallChain),
			PrimaryEntities: []string{"appendTypedRelationKinds"},
		},
	}
	q := BuildTypedRelationQuery(rm, TypedRelationPurposePromptHint, 0)
	assertTypedRelationKinds(t, q.Kinds, TypedRelationCalledBy)
	assertStringSlice(t, q.Sources, []string{"appendTypedRelationKinds"})
}

func TestBuildTypedRelationQuery_SelectsConfigRelationFromRequirementKindPromptOnly(t *testing.T) {
	rm := RequestModel{
		AnalyzerHints: AnalyzerHints{
			Kind:            string(ReqConfigMapping),
			PrimaryEntities: []string{"providers_config"},
		},
		AnswerSubject: AnswerSubject{Kind: SubjectConfigKey},
	}
	q := BuildTypedRelationQuery(rm, TypedRelationPurposePromptHint, 0)
	assertTypedRelationKinds(t, q.Kinds, TypedRelationConfigures)
	assertStringSlice(t, q.Sources, []string{"providers_config"})

	q = BuildTypedRelationQuery(rm, TypedRelationPurposeCoverageGate, 0)
	if len(q.Kinds) != 0 {
		t.Fatalf("config mapping relation must not activate hard coverage from request shape alone, got %+v", q.Kinds)
	}
}

func TestBuildTypedRelationQuery_InterfaceDiagramSelectsImplementAndExtends(t *testing.T) {
	rm := RequestModel{
		Intent:        IntentExplain,
		Scenario:      ScenarioArchitectureExplain,
		PredicateAxis: AxisDefine,
		AnswerSubject: AnswerSubject{Kind: SubjectInterface},
		DiagramHint:   &DiagramHint{Kind: DiagramArchitecture},
		AnalyzerHints: AnalyzerHints{Entities: []string{"Looper", "Looper"}},
	}
	q := BuildTypedRelationQuery(rm, TypedRelationPurposePromptHint, 50)
	assertTypedRelationKinds(t, q.Kinds, TypedRelationImplements, TypedRelationExtends)
	assertStringSlice(t, q.Sources, []string{"Looper"})
	if !ShouldSurfaceTypedRelationHints(rm) {
		t.Fatal("interface diagram shape should surface typed relation hints through the central selector")
	}
}

func TestBuildTypedRelationQuery_ResolvedInterfaceDiagramSelectsImplementAndExtends(t *testing.T) {
	rm := RequestModel{
		Intent:      IntentExplain,
		Scenario:    ScenarioArchitectureExplain,
		DiagramHint: &DiagramHint{Kind: DiagramArchitecture},
		AnalyzerHints: AnalyzerHints{
			Entities: []string{"LoopController"},
		},
	}
	q := BuildTypedRelationQuery(rm, TypedRelationPurposePromptHint, 50)
	if len(q.Kinds) != 0 {
		t.Fatalf("without typed subject/axis or resolved source facts, architecture diagram should not select relation kinds: %+v", q.Kinds)
	}
	resolved := []TypedRelationSourceFact{{
		Name:      "LoopController",
		Kind:      "interface",
		File:      "internal/agent/agent.go",
		Line:      430,
		Carrier:   TypedRelationCarrierGraph,
		Precision: TypedRelationPrecisionExactSymbolID,
	}}
	q = BuildTypedRelationQueryWithResolvedSources(rm, TypedRelationPurposePromptHint, 50, resolved)
	assertTypedRelationKinds(t, q.Kinds, TypedRelationImplements, TypedRelationExtends)
	assertStringSlice(t, q.Sources, []string{"LoopController"})

	q = BuildTypedRelationQueryWithResolvedSources(rm, TypedRelationPurposeCoverageGate, 0, resolved)
	if len(q.Kinds) != 0 {
		t.Fatalf("resolved interface diagram should remain prompt-only unless the request has a typed relation member-set shape, got %+v", q.Kinds)
	}
}

func TestBuildTypedRelationQuery_ResolvedSourceFactsNeedExactPrecision(t *testing.T) {
	rm := RequestModel{
		DiagramHint:   &DiagramHint{Kind: DiagramArchitecture},
		AnalyzerHints: AnalyzerHints{Entities: []string{"LoopController"}},
	}
	resolved := []TypedRelationSourceFact{{
		Name:      "LoopController",
		Kind:      "interface",
		Precision: TypedRelationPrecisionNameOnly,
	}}
	q := BuildTypedRelationQueryWithResolvedSources(rm, TypedRelationPurposePromptHint, 50, resolved)
	if len(q.Kinds) != 0 {
		t.Fatalf("name-only source facts must not select relation kinds, got %+v", q.Kinds)
	}
}

func TestBuildTypedRelationQuery_ImportPathProfileSelectsImportExport(t *testing.T) {
	rm := RequestModel{
		SourceInventoryProfile: &SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []AnswerCandidateRole{AnswerCandidateRoleImportPath},
		},
		AnalyzerHints: AnalyzerHints{PrimaryEntities: []string{"internal/agent"}},
	}
	q := BuildTypedRelationQuery(rm, TypedRelationPurposePromptHint, 25)
	assertTypedRelationKinds(t, q.Kinds, TypedRelationImports, TypedRelationExports)
	assertStringSlice(t, q.Sources, []string{"internal/agent"})
}

func TestBuildTypedRelationQuery_MixedExternalCurrentSourceSelectsSourceAnchor(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioArchitectureExplain,
		Predicates: SemanticPredicates{
			IsHistoryLookup: true,
		},
		AnalyzerHints: AnalyzerHints{PrimaryEntities: []string{"QueueProactiveCallChainClosureRepairs"}},
	}
	q := BuildTypedRelationQuery(rm, TypedRelationPurposePromptHint, 25)
	assertTypedRelationKinds(t, q.Kinds, TypedRelationSourceAnchor)

	rm.Predicates.IsHistoryLookup = false
	rm.LogTriage = &LogBundle{Errors: []LogError{{Type: "timeout"}}}
	rm.CurrentSourceExplanationProfile = &CurrentSourceExplanationProfile{
		IsCurrentSourceExplanationRequested: true,
		Modes:                               []CurrentSourceExplanationMode{CurrentSourceExplanationExplainCurrentMechanism},
		SourceQuotes:                        []string{"结合当前代码"},
	}
	q = BuildTypedRelationQuery(rm, TypedRelationPurposePromptHint, 25)
	assertTypedRelationKinds(t, q.Kinds, TypedRelationSourceAnchor)
}

func TestBuildTypedRelationQuery_CurrentSourceCountDoesNotSelectSourceAnchor(t *testing.T) {
	rm := RequestModel{
		Intent: IntentReturnValue,
		Predicates: SemanticPredicates{
			IsCountQuestion: true,
			IsScalarAnswer:  true,
		},
		AnalyzerHints: AnalyzerHints{PrimaryEntities: []string{"internal/tool"}},
	}
	q := BuildTypedRelationQuery(rm, TypedRelationPurposePromptHint, 25)
	for _, kind := range q.Kinds {
		if kind == TypedRelationSourceAnchor {
			t.Fatalf("plain current-source count should not select external source-anchor relation: %+v", q.Kinds)
		}
	}
}

func TestBuildTypedRelationQuery_ChangeImpactSelectsReferencesPromptOnly(t *testing.T) {
	rm := RequestModel{
		ChangeImpactProfile: &ChangeImpactProfile{
			IsChangeImpact:  true,
			Target:          "RuntimeConfig",
			RequestedOutput: ImpactOutputSites,
			AffectedSiteKinds: []ImpactAffectedSiteKind{
				ImpactSiteRead,
				ImpactSiteValidation,
			},
		},
		AnalyzerHints: AnalyzerHints{PrimaryEntities: []string{"RuntimeConfig"}},
	}
	q := BuildTypedRelationQuery(rm, TypedRelationPurposePromptHint, 25)
	assertTypedRelationKinds(t, q.Kinds, TypedRelationReferences)
	assertStringSlice(t, q.Sources, []string{"RuntimeConfig"})

	q = BuildTypedRelationQuery(rm, TypedRelationPurposeCoverageGate, 25)
	for _, kind := range q.Kinds {
		if kind == TypedRelationReferences {
			t.Fatalf("change-impact reference relation is prompt guidance only, not a coverage-gate selector: %+v", q.Kinds)
		}
	}
}

func TestBuildTypedRelationQuery_ChangeImpactWithoutTargetDoesNotSelectReferences(t *testing.T) {
	rm := RequestModel{
		ChangeImpactProfile: &ChangeImpactProfile{
			IsChangeImpact:  true,
			RequestedOutput: ImpactOutputSites,
			AffectedSiteKinds: []ImpactAffectedSiteKind{
				ImpactSiteRead,
			},
		},
		AnalyzerHints: AnalyzerHints{PrimaryEntities: []string{"RuntimeConfig"}},
	}
	q := BuildTypedRelationQuery(rm, TypedRelationPurposePromptHint, 25)
	for _, kind := range q.Kinds {
		if kind == TypedRelationReferences {
			t.Fatalf("change-impact references need an analyzer-emitted exact target, got %+v", q.Kinds)
		}
	}
}

func TestBuildTypedRelationQuery_DoesNotSelectFromRawProseOrEntityAxes(t *testing.T) {
	rm := RequestModel{
		RawRequest: "list everything that implements Looper and imports net/http",
		AnswerSubject: AnswerSubject{
			Kind:       SubjectGeneric,
			EntityAxes: []string{"interface -> implementation", "module -> import"},
		},
		AnalyzerHints: AnalyzerHints{Entities: []string{"Looper"}},
	}
	q := BuildTypedRelationQuery(rm, TypedRelationPurposePromptHint, 10)
	if len(q.Kinds) != 0 {
		t.Fatalf("raw prose / entity_axes must not select relation kinds, got %+v", q.Kinds)
	}
	if ShouldSurfaceTypedRelationHints(rm) {
		t.Fatal("raw prose / entity_axes alone must not surface typed relation hints")
	}
}

func TestTypedRelationQuerySources_PromptAndCoverageFallbacks(t *testing.T) {
	rm := RequestModel{
		AnalyzerHints: AnalyzerHints{
			PrimaryEntities: []string{"Primary"},
			Entities:        []string{"Primary", "Secondary", "secondary"},
		},
	}
	assertStringSlice(t,
		TypedRelationQuerySources(rm, TypedRelationPurposePromptHint),
		[]string{"Primary"},
	)
	rm.AnalyzerHints.PrimaryEntities = nil
	assertStringSlice(t,
		TypedRelationQuerySources(rm, TypedRelationPurposePromptHint),
		[]string{"Primary", "Secondary"},
	)
	assertStringSlice(t,
		TypedRelationQuerySources(rm, TypedRelationPurposeCoverageGate),
		[]string{"Primary", "Secondary"},
	)
}

func TestTypedRelationCandidateCoverageGateEligibility(t *testing.T) {
	c := TypedRelationCandidate{
		Relation:  TypedRelationImplements,
		Precision: TypedRelationPrecisionExactSymbolID,
		Member:    TypedRelationMember{Name: "looperImpl", File: "impl.go", Line: 12},
	}
	if !c.CoverageGateEligible() {
		t.Fatalf("exact typed relation candidate should be gate-eligible before evidence/scope checks")
	}
	c.Precision = TypedRelationPrecisionNameOnly
	if c.CoverageGateEligible() {
		t.Fatalf("name-only candidate must not be hard-gate eligible")
	}
	c.Precision = TypedRelationPrecisionExactSymbolID
	c.Relation = ""
	if c.CoverageGateEligible() {
		t.Fatalf("candidate without relation kind must not be hard-gate eligible")
	}
}

func assertTypedRelationKinds(t *testing.T, got []TypedRelationKind, want ...TypedRelationKind) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("relation kinds=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("relation kinds=%v, want %v", got, want)
		}
	}
}

func assertStringSlice(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("strings=%v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("strings=%v, want %v", got, want)
		}
	}
}
