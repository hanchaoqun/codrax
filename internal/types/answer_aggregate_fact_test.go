package types

import (
	"strings"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestNormalizeAnswerAggregateFacts_DedupesAndTrims(t *testing.T) {
	in := []AnswerAggregateFact{
		{
			Kind:  AnswerAggregateUniqueCount,
			Label: "  unique files  ",
			Value: " 2 ",
			Unit:  " files ",
			Dimensions: []AnswerAggregateDimension{
				{Name: " scope ", Value: " production "},
				{Name: "scope", Value: "production"},
			},
			Members: []string{" internal/a.go ", "internal/a.go", "internal/b.cpp"},
		},
		{
			Kind:  AnswerAggregateUniqueCount,
			Label: "unique files",
			Value: "2",
			Unit:  "files",
			Dimensions: []AnswerAggregateDimension{
				{Name: "scope", Value: "production"},
			},
			Members: []string{"internal/a.go", "internal/b.cpp"},
		},
	}
	got, err := NormalizeAnswerAggregateFacts(in)
	if err != nil {
		t.Fatalf("NormalizeAnswerAggregateFacts returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("deduped facts len=%d, want 1: %+v", len(got), got)
	}
	if got[0].Label != "unique files" || got[0].Value != "2" || got[0].Unit != "files" {
		t.Fatalf("fact not trimmed: %+v", got[0])
	}
	if len(got[0].Dimensions) != 1 || got[0].Dimensions[0].Name != "scope" || got[0].Dimensions[0].Value != "production" {
		t.Fatalf("dimensions not normalized: %+v", got[0].Dimensions)
	}
	if len(got[0].Members) != 2 || got[0].Members[1] != "internal/b.cpp" {
		t.Fatalf("members not normalized: %+v", got[0].Members)
	}
}

func TestNormalizeAnswerAggregateFacts_RejectsInvalid(t *testing.T) {
	cases := []struct {
		name string
		in   []AnswerAggregateFact
	}{
		{name: "kind", in: []AnswerAggregateFact{{Kind: "derived_guess", Label: "x", Value: "1"}}},
		{name: "label", in: []AnswerAggregateFact{{Kind: AnswerAggregateTotalCount, Value: "1"}}},
		{name: "value", in: []AnswerAggregateFact{{Kind: AnswerAggregateTotalCount, Label: "x"}}},
		{name: "count value unit drift", in: []AnswerAggregateFact{{Kind: AnswerAggregateTotalCount, Label: "x", Value: "3 files"}}},
		{name: "member cardinality", in: []AnswerAggregateFact{{Kind: AnswerAggregateBucketCount, Label: "runtime bucket", Value: "3", Members: []string{"a.go:1", "b.go:2"}}}},
		{name: "member set requires members", in: []AnswerAggregateFact{{Kind: AnswerAggregateMemberSet, Label: "enum types", Value: "2"}}},
		{name: "member set cardinality", in: []AnswerAggregateFact{{Kind: AnswerAggregateMemberSet, Label: "enum types", Value: "2", Members: []string{"Intent"}}}},
		{name: "excluded cardinality", in: []AnswerAggregateFact{{Kind: AnswerAggregateExcluded, Label: "tests", Value: "2", Excluded: []string{"a_test.go"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NormalizeAnswerAggregateFacts(tc.in); err == nil {
				t.Fatalf("expected validation error")
			}
		})
	}
}

func TestNormalizeAnswerAggregateFacts_AcceptsMemberSet(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:    AnswerAggregateMemberSet,
		Label:   "public string enum types",
		Value:   "4",
		Unit:    "types",
		Members: []string{"Intent", "Scenario", "Complexity", "QuestionFamily"},
		SupportRefs: []string{
			"internal/types/analysis_ir.go:642",
			"internal/types/facet_plan.go:9",
		},
	}})
	if err != nil {
		t.Fatalf("member_set aggregate should validate: %v", err)
	}
	if len(got) != 1 || got[0].Kind != AnswerAggregateMemberSet || len(got[0].Members) != 4 {
		t.Fatalf("member_set not preserved: %+v", got)
	}
}

func TestNormalizeAnswerAggregateFacts_PreservesRoleAndProvenance(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:       AnswerAggregateMemberSet,
		Label:      "files inspected",
		Value:      "1",
		Role:       "coverage",
		Provenance: " command:rg ",
		Members:    []string{"internal/agent/explorer.go"},
	}})
	if err != nil {
		t.Fatalf("member_set aggregate should validate: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d facts, want 1", len(got))
	}
	if got[0].Role != AnswerAggregateRoleSupportingCoverage {
		t.Fatalf("role = %q, want supporting_coverage", got[0].Role)
	}
	if got[0].Provenance != "command:rg" {
		t.Fatalf("provenance = %q, want trimmed command:rg", got[0].Provenance)
	}
}

func TestPrincipalAggregateMemberSetFactRefsForRequest_DemotesPathCoverageForArchitecture(t *testing.T) {
	facts := []AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "files inspected",
			Value:   "2",
			Members: []string{"internal/agent/explorer.go", "internal/agent/agent.go"},
		},
		{
			Kind:    AnswerAggregateTotalCount,
			Label:   "core files",
			Value:   "2",
			Unit:    "files",
			Members: []string{"internal/agent/explorer.go", "internal/agent/agent.go"},
		},
	}
	arch := RequestModel{
		Intent:      IntentExplain,
		Scenario:    ScenarioArchitectureExplain,
		Complexity:  ComplexityComplex,
		Predicates:  SemanticPredicates{IsCrossComponent: true},
		DiagramHint: &DiagramHint{Kind: DiagramArchitecture},
	}
	if got := PrincipalAggregateMemberSetFactRefsForRequest(facts, &arch); len(got) != 0 {
		t.Fatalf("architecture path coverage set should not be principal, got %+v", got)
	}

	enum := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all files"},
	}
	if got := PrincipalAggregateMemberSetFactRefsForRequest(facts, &enum); len(got) != 2 {
		t.Fatalf("enumeration path set should remain principal, got %+v", got)
	}
}

func TestPrincipalAggregateMemberSetFactRefsForRequest_ExplicitRoleOverridesHeuristic(t *testing.T) {
	arch := RequestModel{
		Intent:      IntentExplain,
		Scenario:    ScenarioArchitectureExplain,
		Complexity:  ComplexityComplex,
		Predicates:  SemanticPredicates{IsCrossComponent: true},
		DiagramHint: &DiagramHint{Kind: DiagramArchitecture},
	}
	principal := []AnswerAggregateFact{{
		Kind:    AnswerAggregateMemberSet,
		Label:   "files requested by the answer",
		Value:   "1",
		Role:    AnswerAggregateRolePrincipalAnswer,
		Members: []string{"internal/agent/explorer.go"},
	}}
	if got := PrincipalAggregateMemberSetFactRefsForRequest(principal, &arch); len(got) != 1 || got[0].Role != AnswerAggregateRolePrincipalAnswer {
		t.Fatalf("explicit principal role should override path-coverage heuristic, got %+v", got)
	}

	support := []AnswerAggregateFact{{
		Kind:    AnswerAggregateMemberSet,
		Label:   "supporting files",
		Value:   "1",
		Role:    AnswerAggregateRoleSupportingCoverage,
		Members: []string{"internal/agent/explorer.go"},
	}}
	enum := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "all files"},
	}
	if got := PrincipalAggregateMemberSetFactRefsForRequest(support, &enum); len(got) != 0 {
		t.Fatalf("explicit supporting_coverage role must not satisfy principal enumeration, got %+v", got)
	}
}

func TestPrincipalAggregateMemberSetFactRefsForRequest_DemotesNarrativeGroupedCountsForArchitecture(t *testing.T) {
	facts := []AnswerAggregateFact{{
		Kind:    AnswerAggregateGroupedCount,
		Label:   "codrax 四层架构成员",
		Value:   "4",
		Unit:    "layers",
		Members: []string{"Ground (ground/ground.go)", "Gate (analysis/gate/gate.go)", "Reviewer (orchestrator/)", "Contract (orchestrator/contract_check.go)"},
	}}
	arch := RequestModel{
		Intent:     IntentExplain,
		Scenario:   ScenarioArchitectureExplain,
		Complexity: ComplexityComplex,
		Predicates: SemanticPredicates{IsCrossComponent: true},
	}
	if got := PrincipalAggregateMemberSetFactRefsForRequest(facts, &arch); len(got) != 0 {
		t.Fatalf("architecture grouped_count should remain support context, got %+v", got)
	}

	enum := RequestModel{
		Intent: IntentEnumerate,
		Predicates: SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		CompletenessObligation: &CompletenessObligation{Required: true, SourceQuote: "全部架构成员"},
	}
	if got := PrincipalAggregateMemberSetFactRefsForRequest(facts, &enum); len(got) != 1 {
		t.Fatalf("enumeration grouped_count should remain principal, got %+v", got)
	}
}

func TestNormalizeAnswerAggregateFacts_CanonicalizesMemberSetValueFromMembers(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:    AnswerAggregateMemberSet,
		Label:   "pipeline stages",
		Unit:    "stages",
		Members: []string{"analyze", "explore", "extract", "finalize"},
	}})
	if err != nil {
		t.Fatalf("member_set without value should validate from structured members: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d facts, want 1: %+v", len(got), got)
	}
	if got[0].Value != "4" {
		t.Fatalf("member_set value = %q, want canonical len(members)=4", got[0].Value)
	}
	if len(got[0].Members) != 4 {
		t.Fatalf("members not preserved: %+v", got[0].Members)
	}
}

func TestNormalizeAnswerAggregateFacts_DedupesEquivalentMemberSetDisplayVariants(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "subpackage directory and entry function",
			Value:   "3",
			Members: []string{"aggregator/Aggregate", "compiler/Compile", "Type::Member"},
		},
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "all subpackages and entry functions",
			Value:   "3",
			Members: []string{"compiler: Compile", "aggregator -> Aggregate", "Type/Member"},
		},
	})
	if err != nil {
		t.Fatalf("equivalent member_set display variants should validate: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("equivalent member sets should dedupe to one fact, got %+v", got)
	}
	if got[0].Label != "subpackage directory and entry function" {
		t.Fatalf("dedupe should preserve first model-authored display fact, got %+v", got[0])
	}
	candidates := AnswerAggregateMemberDisplayCandidates("compiler: Compile")
	if !stringSliceContains(candidates, "compiler → Compile") || !stringSliceContains(candidates, "compiler/Compile") {
		t.Fatalf("colon relation display should expose equivalent renderings, got %+v", candidates)
	}
}

func TestNormalizeAnswerAggregateFacts_DedupesQualifiedRelationMemberSetVariants(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "subpackage directory and entry function",
			Value:   "3",
			Members: []string{"aggregator → Aggregate", "declarative → Classify", "hint → Compose"},
		},
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "all subpackages and entry functions",
			Value:   "3",
			Members: []string{"hint → Composer.Compose", "aggregator → Aggregator.Aggregate", "declarative → Classifier.Classify"},
		},
	})
	if err != nil {
		t.Fatalf("qualified relation member variants should validate: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("qualified relation variants should dedupe to one fact, got %+v", got)
	}
	candidates := AnswerAggregateMemberDisplayCandidates("aggregator → Aggregator.Aggregate")
	if !stringSliceContains(candidates, "aggregator → Aggregate") ||
		!stringSliceContains(candidates, "aggregator/Aggregate") {
		t.Fatalf("qualified relation display should expose tail renderings, got %+v", candidates)
	}
}

func TestPrincipalAggregateMemberSetFactRefs_PrefersSupportedRelationAlternateSameLeftAxis(t *testing.T) {
	refs := PrincipalAggregateMemberSetFactRefs([]AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "subpackage entry functions",
			Value:   "2",
			Members: []string{"aggregator → Aggregator.Aggregate", "compiler → Compile"},
			SupportRefs: []string{
				"internal/analysis/aggregator/aggregator.go:132",
				"internal/analysis/compiler/compile.go:37",
			},
		},
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "internal/analysis package entries",
			Value:   "2",
			Members: []string{"aggregator.Aggregate", "compiler.Compile"},
		},
	})
	if len(refs) != 1 {
		t.Fatalf("unsupported alternate relation set with same left axis should be support-only, got %+v", refs)
	}
	if refs[0].Index != 0 || refs[0].Fact.Label != "subpackage entry functions" {
		t.Fatalf("complete support-ref relation set should remain principal, got %+v", refs[0])
	}
}

func TestPrincipalAggregateMemberSetFactRefs_KeepsFullySupportedSameLeftAxisSets(t *testing.T) {
	refs := PrincipalAggregateMemberSetFactRefs([]AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "subpackage entry functions",
			Value:   "2",
			Members: []string{"aggregator → Aggregator.Aggregate", "compiler → Compile"},
			SupportRefs: []string{
				"internal/analysis/aggregator/aggregator.go:132",
				"internal/analysis/compiler/compile.go:37",
			},
		},
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "subpackage constructors",
			Value:   "2",
			Members: []string{"aggregator → New", "compiler → New"},
			SupportRefs: []string{
				"internal/analysis/aggregator/aggregator.go:112",
				"internal/analysis/compiler/compiler.go:20",
			},
		},
	})
	if len(refs) != 2 {
		t.Fatalf("two fully supported relation attributes over the same left axis should both stay principal, got %+v", refs)
	}
}

func TestAnswerAggregateMemberDisplayCandidates_RelationDecoratorVariants(t *testing.T) {
	candidates := AnswerAggregateMemberDisplayCandidates("declarative → New (Classifier)")
	if !stringSliceContains(candidates, "declarative → New (Classifier)") ||
		!stringSliceContains(candidates, "declarative → New(Classifier)") ||
		!stringSliceContains(candidates, "declarative/New(Classifier)") {
		t.Fatalf("decorated relation member should expose spaced and compact displays, got %+v", candidates)
	}
	left, right, ok := AnswerAggregateMemberRelationParts("priority → Score(priority)")
	if !ok || left != "priority" || right != "Score(priority)" {
		t.Fatalf("decorated relation should parse as relation parts, got left=%q right=%q ok=%v", left, right, ok)
	}
	left, right, ok = AnswerAggregateMemberRelationParts("patcher → Engine (New + Submit/Apply)")
	if !ok || left != "patcher" || right != "Engine (New + Submit/Apply)" {
		t.Fatalf("decorated relation with qualifier list should parse, got left=%q right=%q ok=%v", left, right, ok)
	}
	candidates = AnswerAggregateMemberDisplayCandidates("patcher → Engine (New + Submit/Apply)")
	if !stringSliceContains(candidates, "patcher → Engine (New + Submit/Apply)") ||
		!stringSliceContains(candidates, "patcher → Engine(New + Submit/Apply)") ||
		!stringSliceContains(candidates, "patcher/Engine(New + Submit/Apply)") {
		t.Fatalf("decorated relation with slash qualifier should expose display variants, got %+v", candidates)
	}
	base, qualifier, ok := AnswerAggregateDecoratedLabelParts("Score (subject)")
	if !ok || base != "Score" || qualifier != "subject" {
		t.Fatalf("standalone decorated label should expose qualifier, got base=%q qualifier=%q ok=%v", base, qualifier, ok)
	}
}

func TestAnswerAggregateMemberDisplayCandidates_SourceLineDecoratorIsSupportSurface(t *testing.T) {
	candidates := AnswerAggregateMemberDisplayCandidates("aggregator → Aggregate (line 132)")
	if !stringSliceContains(candidates, "aggregator → Aggregate") ||
		!stringSliceContains(candidates, "aggregator/Aggregate") {
		t.Fatalf("source-line decorator should expose relation identity without the line support surface, got %+v", candidates)
	}
	if stringSliceContains(AnswerAggregateMemberDisplayCandidates("declarative → New (Classifier)"), "declarative → New") {
		t.Fatal("non-location relation decorators must remain identity-bearing")
	}
	if !stringSliceContains(AnswerAggregateMemberDisplayCandidates("prescan → ClassifyToken (行 29)"), "prescan → ClassifyToken") {
		t.Fatal("Chinese source-line decorators should also be treated as support surfaces")
	}
	if !stringSliceContains(AnswerAggregateMemberDisplayCandidates("compiler → Compile (lines 37-39)"), "compiler → Compile") {
		t.Fatal("source-line ranges should also be treated as support surfaces")
	}
	if !stringSliceContains(AnswerAggregateMemberDisplayCandidates("risk → Evaluate (L22)"), "risk → Evaluate") {
		t.Fatal("compact line decorators should also be treated as support surfaces")
	}
}

func TestAnswerAggregateMemberDisplayCandidates_CompactSourceSupportRef(t *testing.T) {
	candidates := AnswerAggregateMemberDisplayCandidates("analyzerEvaluator@internal/agent/analyzer.go:887")
	if !stringSliceContains(candidates, "analyzerEvaluator@internal/agent/analyzer.go:887") {
		t.Fatalf("compact support-ref member should preserve the model-authored surface, got %+v", candidates)
	}
	if !stringSliceContains(candidates, "analyzerEvaluator") {
		t.Fatalf("compact support-ref member should expose its principal label for downstream display, got %+v", candidates)
	}
}

func TestAnswerAggregateMemberDisplayCandidates_ViaSourceSupportRef(t *testing.T) {
	candidates := AnswerAggregateMemberDisplayCandidates("explorer (via SubExplorer.Name @ internal/agent/sub_explorer.go:31)")
	if !stringSliceContains(candidates, "explorer") {
		t.Fatalf("via support-ref member should expose its principal label for downstream display, got %+v", candidates)
	}
	if !stringSliceContains(candidates, "explorer (via SubExplorer.Name @ internal/agent/sub_explorer.go:31)") {
		t.Fatalf("via support-ref member should preserve the model-authored surface, got %+v", candidates)
	}
}

func TestAnswerAggregateMemberRelationParts_CompactDotAcrossSupportedLanguages(t *testing.T) {
	samples := map[string]string{
		repotypes.LangGo:         "compiler.Compile",
		repotypes.LangPython:     "module.run",
		repotypes.LangJavaScript: "Controller.handle",
		repotypes.LangTypeScript: "Service.resolve",
		repotypes.LangJava:       "Service.handle",
		repotypes.LangKotlin:     "Service.handle",
		repotypes.LangRust:       "crate::run",
		repotypes.LangC:          "module.init_module",
		repotypes.LangCpp:        "Parser::Parse",
		repotypes.LangRuby:       "Admin::call",
		repotypes.LangSwift:      "Service.start",
		repotypes.LangLua:        "M.render",
		repotypes.LangProto:      "UserService.ListUsers",
		repotypes.LangArkTS:      "EntryComponent.build",
		repotypes.LangCangjie:    "Service::run",
	}
	for _, lang := range repotypes.SupportedReadLanguages() {
		sample, ok := samples[lang]
		if !ok {
			t.Fatalf("missing compact relation sample for supported language %q", lang)
		}
		left, right, parsed := AnswerAggregateMemberRelationParts(sample)
		if !parsed {
			t.Fatalf("%s compact sample did not parse as relation member: %q", lang, sample)
		}
		if left == "" || right == "" {
			t.Fatalf("%s compact sample parsed empty relation part: left=%q right=%q", lang, left, right)
		}
	}
	for _, sample := range []string{"package.submodule.run", "internal/types/foo.go", "github.com/acme/pkg"} {
		if left, right, ok := AnswerAggregateMemberRelationParts(sample); ok {
			t.Fatalf("ambiguous/path-like compact surface %q must stay literal, got left=%q right=%q", sample, left, right)
		}
	}
	candidates := AnswerAggregateMemberDisplayCandidates("compiler.Compile")
	if !stringSliceContains(candidates, "compiler → Compile") ||
		!stringSliceContains(candidates, "compiler/Compile") {
		t.Fatalf("compact relation should expose split display candidates, got %+v", candidates)
	}
}

func TestAnswerAggregateMemberDisplayCandidates_AllSupportedLanguageQualifiers(t *testing.T) {
	samples := map[string]string{
		repotypes.LangGo:         "compiler → compiler.Compile",
		repotypes.LangPython:     "module → package.submodule.run",
		repotypes.LangJavaScript: "module → Controller.handle",
		repotypes.LangTypeScript: "service → App.Service.resolve",
		repotypes.LangJava:       "service → com.example.Service.handle",
		repotypes.LangKotlin:     "service → com.example.Service.handle",
		repotypes.LangRust:       "crate → crate::module::run",
		repotypes.LangC:          "module → init_module",
		repotypes.LangCpp:        "namespace → core::Parser::Parse",
		repotypes.LangRuby:       "module → Admin::Users.call",
		repotypes.LangSwift:      "module → App.Service.start",
		repotypes.LangLua:        "module → M.render",
		repotypes.LangProto:      "service → api.v1.UserService.ListUsers",
		repotypes.LangArkTS:      "component → EntryComponent.build",
		repotypes.LangCangjie:    "package → pkg::Service::run",
	}
	for _, lang := range repotypes.SupportedReadLanguages() {
		sample, ok := samples[lang]
		if !ok {
			t.Fatalf("missing aggregate relation qualifier sample for supported language %q", lang)
		}
		left, right, parsed := AnswerAggregateMemberRelationParts(sample)
		if !parsed {
			t.Fatalf("%s sample did not parse as relation member: %q", lang, sample)
		}
		if left == "" || right == "" {
			t.Fatalf("%s sample parsed empty relation part: left=%q right=%q", lang, left, right)
		}
		candidates := AnswerAggregateMemberDisplayCandidates(sample)
		if len(candidates) == 0 {
			t.Fatalf("%s sample produced no display candidates: %q", lang, sample)
		}
		if tail := NormalizedSurfaceSymbolTail(right); tail != "" && tail != strings.ToLower(right) {
			if !stringSliceContainsFold(candidates, left+" → "+tail) {
				t.Fatalf("%s sample should expose tail relation candidate %q, got %+v", lang, left+" → "+tail, candidates)
			}
		}
	}
}

func TestAnswerAggregateMemberRelationParts_AllSupportedLanguageSignatures(t *testing.T) {
	samples := map[string]struct {
		member string
		base   string
	}{
		repotypes.LangGo:         {"analysis → New(cfg Config) *Aggregator", "New"},
		repotypes.LangPython:     {"module -> run(config: Config) -> Result", "run"},
		repotypes.LangJavaScript: {"service → resolve(input): Promise<Result>", "resolve"},
		repotypes.LangTypeScript: {"service → resolve<T>(input: T): Promise<T>", "resolve"},
		repotypes.LangJava:       {"service → handle(Request req): Response", "handle"},
		repotypes.LangKotlin:     {"service → handle(request: Request): Response", "handle"},
		repotypes.LangRust:       {"crate -> run<T>(input: T) -> Result<()>", "run"},
		repotypes.LangC:          {"module → init_module(int argc, char **argv)", "init_module"},
		repotypes.LangCpp:        {"parser → Parser::Parse(std::string_view input) -> Result", "Parser::Parse"},
		repotypes.LangRuby:       {"service → call(payload)", "call"},
		repotypes.LangSwift:      {"service → start(request: Request) async throws -> Response", "start"},
		repotypes.LangLua:        {"module → render(ctx)", "render"},
		repotypes.LangProto:      {"service → ListUsers(ListUsersRequest) returns (ListUsersResponse)", "ListUsers"},
		repotypes.LangArkTS:      {"component → build(): void", "build"},
		repotypes.LangCangjie:    {"service → run(input: String): Result<Unit>", "run"},
	}
	for _, lang := range repotypes.SupportedReadLanguages() {
		sample, ok := samples[lang]
		if !ok {
			t.Fatalf("missing signature relation sample for supported language %q", lang)
		}
		left, right, parsed := AnswerAggregateMemberRelationParts(sample.member)
		if !parsed {
			t.Fatalf("%s signature sample did not parse as relation member: %q", lang, sample.member)
		}
		if left == "" || right == "" {
			t.Fatalf("%s signature sample parsed empty relation part: left=%q right=%q", lang, left, right)
		}
		candidates := AnswerAggregateMemberDisplayCandidates(sample.member)
		if !stringSliceContainsFold(candidates, left+" → "+sample.base) {
			t.Fatalf("%s signature sample should expose callable base candidate %q, got %+v", lang, left+" → "+sample.base, candidates)
		}
	}
}

func TestNormalizeAnswerAggregateFacts_DoesNotCollapseAmbiguousQualifiedRelationTails(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "overloaded handlers",
			Value:   "2",
			Members: []string{"api → HTTP.Handle", "api → GRPC.Handle"},
		},
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "ambiguous handler surface",
			Value:   "1",
			Members: []string{"api → Handle"},
		},
	})
	if err != nil {
		t.Fatalf("ambiguous qualified relation members should still validate: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ambiguous qualified tails must not collapse into one member set, got %+v", got)
	}
}

func stringSliceContains(in []string, want string) bool {
	for _, got := range in {
		if got == want {
			return true
		}
	}
	return false
}

func stringSliceContainsFold(in []string, want string) bool {
	for _, got := range in {
		if strings.EqualFold(got, want) {
			return true
		}
	}
	return false
}

func TestNormalizeAnswerAggregateFacts_DoesNotCollapseDistinctPathMembers(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "production files",
			Value:   "1",
			Members: []string{"src/main.cpp"},
		},
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "production files alternate",
			Value:   "1",
			Members: []string{"src → main.cpp"},
		},
	})
	if err != nil {
		t.Fatalf("path-like member sets should still validate: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("path-like and relation-like members must remain distinct, got %+v", got)
	}
}

func TestNormalizeAnswerAggregateFacts_NonMemberSetStillRequiresValue(t *testing.T) {
	_, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:    AnswerAggregateTotalCount,
		Label:   "pipeline stages",
		Members: []string{"analyze", "explore", "extract", "finalize"},
	}})
	if err == nil {
		t.Fatal("total_count without value must still reject; members may be samples")
	}
}

func TestNormalizeAnswerAggregateFacts_AcceptsGroupedAndBucketCounts(t *testing.T) {
	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{
		{
			Kind:  AnswerAggregateGroupedCount,
			Label: "language=arkts",
			Value: "2",
			Unit:  "locations",
			Dimensions: []AnswerAggregateDimension{
				{Name: "language", Value: "ArkTS"},
				{Name: "bucket", Value: "runtime"},
			},
			Members: []string{"entry/src/main/ets/pages/Index.ets:12", "entry/src/main/ets/pages/Index.ets:18"},
		},
		{
			Kind:  AnswerAggregateBucketCount,
			Label: "native bucket",
			Value: "2",
			Unit:  "locations",
			Dimensions: []AnswerAggregateDimension{
				{Name: "language", Value: "C++"},
				{Name: "bucket", Value: "native"},
			},
			Members: []string{"src/native/foo.cpp:20", "src/native/foo.h:8"},
		},
		{
			Kind:  AnswerAggregateUniqueCount,
			Label: "native bucket files",
			Value: "2",
			Unit:  "files",
			Dimensions: []AnswerAggregateDimension{
				{Name: "bucket", Value: "native"},
			},
			Members: []string{"src/native/foo.cpp", "src/native/foo.h"},
		},
	})
	if err != nil {
		t.Fatalf("grouped/bucket aggregate facts should validate: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d facts, want 3: %+v", len(got), got)
	}
}

func TestNormalizeAnswerAggregateFacts_FileLineMembersRequireUniqueFileFact(t *testing.T) {
	_, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{{
		Kind:  AnswerAggregateTotalCount,
		Label: "production locations",
		Value: "4",
		Members: []string{
			"internal/agent/analyzer.go:1903",
			"internal/orchestrator/contract_check.go:63",
			"internal/orchestrator/orchestrator.go:6362",
			"internal/orchestrator/orchestrator.go:6494",
		},
	}})
	if err == nil {
		t.Fatal("expected missing unique_count companion to reject")
	}

	got, err := NormalizeAnswerAggregateFacts([]AnswerAggregateFact{
		{
			Kind:  AnswerAggregateTotalCount,
			Label: "production locations",
			Value: "4",
			Members: []string{
				"internal/agent/analyzer.go:1903",
				"internal/orchestrator/contract_check.go:63",
				"internal/orchestrator/orchestrator.go:6362",
				"internal/orchestrator/orchestrator.go:6494",
			},
		},
		{
			Kind:  AnswerAggregateUniqueCount,
			Label: "distinct files",
			Value: "3",
			Unit:  "files",
			Members: []string{
				"internal/agent/analyzer.go",
				"internal/orchestrator/contract_check.go",
				"internal/orchestrator/orchestrator.go",
			},
		},
	})
	if err != nil {
		t.Fatalf("unique_count companion should satisfy file-set aggregate: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d facts, want 2: %+v", len(got), got)
	}
}

func TestPrincipalAggregateMemberSetFactRefs_RelationSetSubsumesLeftAxis(t *testing.T) {
	facts := []AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "packages",
			Value:   "3",
			Members: []string{"aggregator", "declarative", "priority"},
		},
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "package entry functions",
			Value:   "3",
			Members: []string{"aggregator → Aggregate", "declarative → New (Classifier)", "priority → Score(priority)"},
		},
	}

	got := PrincipalAggregateMemberSetFactRefs(facts)
	if len(got) != 1 {
		t.Fatalf("left-axis member set should be coverage context, got %+v", got)
	}
	if got[0].Index != 1 || got[0].Fact.Label != "package entry functions" {
		t.Fatalf("principal slate should preserve the richer relation fact and original index, got %+v", got[0])
	}
}

func TestPrincipalAggregateMemberSetFactRefs_DoesNotSuppressRightAxisOrDifferentDimensions(t *testing.T) {
	facts := []AnswerAggregateFact{
		{
			Kind:    AnswerAggregateMemberSet,
			Label:   "entry functions",
			Value:   "2",
			Members: []string{"Aggregate", "Compile"},
		},
		{
			Kind:  AnswerAggregateMemberSet,
			Label: "production package entries",
			Value: "2",
			Dimensions: []AnswerAggregateDimension{{
				Name:  "scope",
				Value: "production",
			}},
			Members: []string{"aggregator → Aggregate", "compiler → Compile"},
		},
		{
			Kind:  AnswerAggregateMemberSet,
			Label: "test packages",
			Value: "2",
			Dimensions: []AnswerAggregateDimension{{
				Name:  "scope",
				Value: "tests",
			}},
			Members: []string{"aggregator", "compiler"},
		},
	}

	got := PrincipalAggregateMemberSetFactRefs(facts)
	if len(got) != 3 {
		t.Fatalf("right-axis and different-dimension sets should remain principal, got %+v", got)
	}
}

func TestMutableState_StableInvestigationAggregateFactsRetention(t *testing.T) {
	mu := NewMutableState("q")
	facts := []AnswerAggregateFact{{
		Kind:    AnswerAggregateBucketCount,
		Label:   "CLI bucket",
		Value:   "2",
		Unit:    "items",
		Members: []string{"--foo", "--bar"},
	}}
	mu.SetInvestigationAggregateFacts(facts)
	if got := mu.StableInvestigationAggregateFacts(); len(got) != 0 {
		t.Fatalf("downgraded/current facts must not be stable before completion: %+v", got)
	}
	mu.SetInvestigationComplete("done")
	mu.RetainInvestigationAggregateFacts()
	mu.ResetInvestigationComplete()

	got := mu.StableInvestigationAggregateFacts()
	if len(got) != 1 || got[0].Label != "CLI bucket" || len(got[0].Members) != 2 {
		t.Fatalf("stable aggregate facts not retained across reset: %+v", got)
	}
	got[0].Members[0] = "mutated"
	again := mu.StableInvestigationAggregateFacts()
	if again[0].Members[0] != "--foo" {
		t.Fatalf("StableInvestigationAggregateFacts must return a defensive copy: %+v", again)
	}
}
