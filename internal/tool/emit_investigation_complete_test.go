package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEmitInvestigationCompleteSchemaDescribesClosureHandoff(t *testing.T) {
	tool := &EmitInvestigationComplete{}
	desc := tool.Description()
	for _, want := range []string{
		"concise terminal conclusion",
		"later answer writing",
	} {
		if !strings.Contains(desc, want) {
			t.Fatalf("Description missing %q:\n%s", want, desc)
		}
	}

	schema := string(tool.Parameters())
	for _, want := range []string{
		"Concise completion conclusion",
		"scope boundary",
		"no-hit/exclusion finding",
		"cross-repository or cross-component distinction",
		"Do not leave the conclusion only in free-form text",
		"aggregate_facts",
		"negative_search",
		"repo, query or pattern, scope, and searched_at",
		"absence_justification",
		"not as a citation",
	} {
		if !strings.Contains(schema, want) {
			t.Fatalf("schema missing %q:\n%s", want, schema)
		}
	}
}

// TestEmitInvestigationComplete_AbsenceWithGroundedEvidenceRejected
// pins the 2026-04-17 contradiction gate: when the explorer already
// buffered grounded/recovered evidence, an emit_investigation_complete
// call that carries absence_justification is rejected. This prevents
// the LLM from shortcutting the finalize citation-floor gate by
// tacking "this is an absence answer" onto every completion call.
func TestEmitInvestigationComplete_AbsenceWithGroundedEvidenceRejected(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: "a.go", LineStart: 10,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "Foo",
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
	}})
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"done","confidence":"high","result_kind":"absence","absence_justification":"answer is zero"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected rejection when grounded evidence exists and absence claimed; got success")
	}
	if !strings.Contains(res.Summary, "absence_justification") {
		t.Errorf("rejection summary must name the offending field: %q", res.Summary)
	}
	if mut.AbsenceJustification() != "" {
		t.Errorf("absence must NOT be stored on rejection, got %q", mut.AbsenceJustification())
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Errorf("completion flag must NOT fire on rejection")
	}
}

func TestEmitInvestigationComplete_AbsenceWithTypedEmptyMemberSetAllowsContextEvidence(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: "a.go", LineStart: 10,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "Foo",
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
	}})
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"the bounded inventory was checked and no requested members were found",
		"confidence":"high",
		"result_kind":"absence",
		"absence_justification":"the checked member universe is empty for the requested target",
		"aggregate_facts":[
			{
				"kind":"negative_search",
				"label":"requested member search",
				"value":"0",
				"unit":"matches",
				"dimensions":[
					{"name":"repo","value":"current repository"},
					{"name":"pattern","value":"requested target"},
					{"name":"scope","value":"bounded inventory"},
					{"name":"searched_at","value":"current_investigation"}
				]
			},
			{
				"kind":"member_set",
				"label":"requested members",
				"value":"0",
				"unit":"members",
				"role":"principal_answer",
				"members":[]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("typed empty member_set with no-hit support should allow contextual evidence: %s", res.Summary)
	}
	if mut.AbsenceJustification() == "" {
		t.Fatalf("absence justification should be stored after accepted typed empty handoff")
	}
}

// TestEmitInvestigationComplete_AbsenceWithoutEvidenceAccepted locks
// the legit honest-zero path: when no evidence was emitted, the LLM
// can still declare absence (e.g. "how many .py files?" → 0). The
// hasAnyInvestigationSuccess audit in contract_check still applies
// downstream.
func TestEmitInvestigationComplete_AbsenceWithoutEvidenceAccepted(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"none found","confidence":"high","result_kind":"absence","absence_justification":"no .py files exist"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("honest-zero absence must be accepted: %s", res.Summary)
	}
	assertToolRuntimeTimingPhases(t, res.RuntimeTimings,
		"strict_decode",
		"completion_preflight_view",
		"aggregate_normalization",
		"decorator_member_validation",
		"grounding_citation_floors",
		"pre_complete_gate_chain",
		"completion_state_write",
	)
	if mut.AbsenceJustification() == "" {
		t.Errorf("absence must be stored on acceptance")
	}
}

func TestCompletionPreflightViewCopiesEvidenceFactsAndTallies(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	evidence := []types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "a.go",
			LineStart:       10,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceRelationship,
			Source:          "b.go",
			LineStart:       20,
			GroundingStatus: types.GroundingRecovered,
		},
	}
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "members",
		Value:   "1",
		Members: []string{"A"},
	}}

	view := buildCompletionPreflightView(bus, "resolved", "", evidence, facts, nil)
	evidence[0].Source = "mutated.go"
	facts[0].Value = "99"

	if got := view.Evidence[0].Source; got != "a.go" {
		t.Fatalf("preflight view must snapshot evidence, got %q", got)
	}
	if got := view.EffectiveAggregateFacts[0].Value; got != "1" {
		t.Fatalf("preflight view must snapshot aggregate facts, got %q", got)
	}
	if got := view.StructuredRelationAuthorityFacts[0].Value; got != "1" {
		t.Fatalf("relation authority facts must default to effective facts, got %q", got)
	}
	if !view.GenericEvidenceTally.hasAny() {
		t.Fatalf("generic tally should see grounded/recovered evidence: %+v", view.GenericEvidenceTally)
	}
	if view.Tier1EvidenceTally.total != 2 || view.Tier1EvidenceTally.tier1 != 1 || view.Tier1EvidenceTally.recovered != 1 {
		t.Fatalf("tier1 tally mismatch: %+v", view.Tier1EvidenceTally)
	}
}

func TestEmitInvestigationComplete_AcceptsStructuredAggregateFacts(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"deterministic count and candidate classification complete",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"total_count",
				"label":"production assignment locations",
				"value":"4",
				"unit":"locations",
				"members":["internal/a.go:10","internal/a.go:20","src/native/foo.cpp:22","src/native/foo.cpp:23"]
			},
			{
				"kind":"unique_count",
				"label":"unique files",
				"value":"2",
				"unit":"files",
				"members":["internal/a.go","src/native/foo.cpp"]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("aggregate facts completion should be accepted: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "aggregate_facts: 2") {
		t.Fatalf("summary should report aggregate fact handoff: %s", res.Summary)
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) != 2 || got[0].Kind != types.AnswerAggregateTotalCount || got[1].Value != "2" {
		t.Fatalf("stable aggregate facts not stored: %+v", got)
	}
}

func TestEmitInvestigationComplete_RefreshesStaleMemberSetWithLaterCompleteSet(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	first := json.RawMessage(`{
		"reason":"fixture surface was enumerated first",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"foreign func 声明",
			"value":"1",
			"role":"principal_answer",
			"unit":"处",
			"members":["native_add(a: Int64, b: Int64): Int64 @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6"],
			"support_refs":["Bridge.cj:6"],
			"member_notes":["foreign func native_add(a: Int64, b: Int64): Int64，包 demo.bridge"]
		}]
	}`)
	if res, err := tool.Execute(bus, first); err != nil || !res.Success {
		t.Fatalf("first completion failed: res=%+v err=%v", res, err)
	}

	secondFact := types.AnswerAggregateFact{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "foreign func 声明",
		Value: "2",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Unit:  "处",
		Members: []string{
			"native_add @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6",
			"native_add @ internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6",
		},
		SupportRefs: []string{"Bridge.cj:6", "07_foreign_ffi.cj:6"},
		MemberNotes: []string{
			"foreign func native_add(a: Int64, b: Int64): Int64，包 demo.bridge",
			"foreign func native_add(a: Int64, b: Int64): Int64，包 demo.ffi",
		},
	}
	normalizedSecond, _, err := normalizeCompletionAggregateFacts(bus, "resolved", []types.AnswerAggregateFact{secondFact})
	if err != nil {
		t.Fatalf("normalize second facts failed: %v", err)
	}
	if len(normalizedSecond) != 1 || normalizedSecond[0].Value != "2" || len(normalizedSecond[0].Members) != 2 {
		t.Fatalf("normalized second facts should keep two rows, got %+v", normalizedSecond)
	}
	mergedProbe := types.MergeAnswerAggregateFacts(mut.StableInvestigationAggregateFacts(), normalizedSecond)
	if len(mergedProbe) != 1 || mergedProbe[0].Value != "2" || len(mergedProbe[0].Members) != 2 {
		t.Fatalf("types merge should prefer later complete set before preflight, got %+v", mergedProbe)
	}
	probeEffective := effectiveCompletionAggregateFactsForValidation(bus, normalizedSecond, nil)
	if len(probeEffective) != 1 || probeEffective[0].Value != "2" || len(probeEffective[0].Members) != 2 {
		t.Fatalf("preflight effective facts should prefer later complete set before Execute, got %+v", probeEffective)
	}

	second := json.RawMessage(`{
		"reason":"all source classes were enumerated after follow-up",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"foreign func 声明",
			"value":"2",
			"role":"principal_answer",
			"unit":"处",
			"members":[
				"native_add @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6",
				"native_add @ internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6"
			],
			"support_refs":["Bridge.cj:6","07_foreign_ffi.cj:6"],
			"member_notes":[
				"foreign func native_add(a: Int64, b: Int64): Int64，包 demo.bridge",
				"foreign func native_add(a: Int64, b: Int64): Int64，包 demo.ffi"
			]
		}]
	}`)
	secondRes, err := tool.Execute(bus, second)
	if err != nil || !secondRes.Success {
		t.Fatalf("second completion failed: res=%+v err=%v", secondRes, err)
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) != 1 || got[0].Kind != types.AnswerAggregateMemberSet {
		t.Fatalf("stable aggregate facts = %+v, want one member_set", got)
	}
	if got[0].Value != "2" || len(got[0].Members) != 2 {
		t.Fatalf("later complete member_set should refresh stale subset, got %+v; second summary=%s", got[0], secondRes.Summary)
	}
	visible := strings.Join(got[0].SupportRefs, "\n")
	for _, want := range []string{
		"native_add @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6",
		"native_add @ internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("stable support refs lost %q: %+v", want, got[0].SupportRefs)
		}
	}
}

func TestEmitInvestigationComplete_DropsPartialCountMembers(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"count fact has exact scalar value; listed members were only examples",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"total_count",
				"label":"test-file LoopController implementations",
				"value":"4",
				"unit":"types",
				"members":["protocolSoftStopEvaluator","isolatedPromptEvaluator","protocolSoftStopAcceptEvaluator"]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("partial count members should be structurally normalized, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "partial members omitted") {
		t.Fatalf("summary should disclose partial member omission, got: %s", res.Summary)
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) != 1 || got[0].Kind != types.AnswerAggregateTotalCount || got[0].Value != "4" {
		t.Fatalf("count fact not preserved: %+v", got)
	}
	if len(got[0].Members) != 0 {
		t.Fatalf("partial members should not be stored as exact count members: %+v", got[0].Members)
	}
}

func TestEmitInvestigationComplete_AcceptsNegativeSearchAggregateFact(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"frameworks/base had zero Ressched bridge hits while ressched contains the local IPC surface",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"negative_search",
				"label":"frameworks/base Ressched bridge search",
				"value":"0",
				"unit":"matches",
				"dimensions":[
					{"name":"repo","value":"frameworks/base"},
					{"name":"pattern","value":"ResSched|IRemoteBroker|SAMR"},
					{"name":"scope","value":"frameworks/base"},
					{"name":"searched_at","value":"explore iteration 20"}
				]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("negative_search aggregate completion should be accepted: %s", res.Summary)
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) != 1 || got[0].Kind != types.AnswerAggregateNegativeSearch || got[0].Value != "0" {
		t.Fatalf("negative_search aggregate not stored: %+v", got)
	}
}

func TestEmitInvestigationComplete_SynthesizesAbsenceJustificationFromNegativeSearchAggregate(t *testing.T) {
	const missingKey = "explore_xyz_phantom_unique_budget"
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:   types.SubjectConfigKey,
					TargetLabel:  "config key",
					Targets:      []string{missingKey},
					AllowAbsence: true,
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"the exact config key is absent from every requested layer",
		"confidence":"high",
		"result_kind":"absence",
		"aggregate_facts":[
			{
				"kind":"negative_search",
				"label":"explore_xyz_phantom_unique_budget in production code",
				"value":"0",
				"unit":"matches",
				"dimensions":[
					{"name":"repo","value":"hanchaoqun/codrax"},
					{"name":"pattern","value":"explore_xyz_phantom_unique_budget"},
					{"name":"scope","value":"production Go files + codrax.yaml.example + cmd/root.go"},
					{"name":"searched_at","value":"current_investigation"}
				]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("negative_search exact absence should synthesize justification instead of retrying: %s", res.Summary)
	}
	if got := mut.StableInvestigationResultKind(); got != "absence" {
		t.Fatalf("StableInvestigationResultKind = %q, want absence", got)
	}
	if got := mut.StableAbsenceJustification(); got == "" {
		t.Fatal("StableAbsenceJustification = empty, want synthesized justification")
	}
}

func TestEmitInvestigationComplete_SynthesizesAbsenceForConfigTraceSubjectDriftWithContextEvidence(t *testing.T) {
	const missingKey = "explore_xyz_phantom_unique_budget"
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/types/config.go",
		LineStart:       852,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "ExploreSettings",
		ContextRole:     types.EvidenceContextRoleRelatedContext,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest:    "explore_xyz_phantom_unique_budget 在三层覆盖（默认值 / codrax.yaml / CLI flag）里各自的有效值是什么？给我每一层的来源锚点。",
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectStructField},
				AnalyzerHints: types.AnalyzerHints{
					Kind:              "config_mapping",
					ExactTargets:      []string{missingKey},
					ExactContextRoles: []types.EvidenceDiagramRole{types.EvidenceDiagramRoleDefault, types.EvidenceDiagramRoleConfig, types.EvidenceDiagramRoleOverride},
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"explore_xyz_phantom_unique_budget 在三层覆盖中均不存在",
		"confidence":"high",
		"result_kind":"absence",
		"aggregate_facts":[
			{
				"kind":"negative_search",
				"label":"explore_xyz_phantom_unique_budget in ExploreSettings fields",
				"provenance":"grep_negative",
				"value":"0",
				"dimensions":[
					{"name":"repo","value":"."},
					{"name":"pattern","value":"explore_xyz_phantom_unique_budget"},
					{"name":"scope","value":"ExploreSettings struct fields (config.go:852-855)"},
					{"name":"searched_at","value":"current_investigation"}
				]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("config-trace subject drift with typed negative_search should not retry: %s", res.Summary)
	}
	if got := mut.StableInvestigationResultKind(); got != "absence" {
		t.Fatalf("StableInvestigationResultKind = %q, want absence", got)
	}
	if got := mut.StableAbsenceJustification(); got == "" {
		t.Fatal("StableAbsenceJustification = empty, want synthesized justification")
	}
}

func TestEmitInvestigationComplete_AbsenceDropsUnsupportedNonPrincipalDecoratedMemberSet(t *testing.T) {
	const missingKey = "explore_xyz_phantom_unique_budget"
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:   types.SubjectConfigKey,
					TargetLabel:  "config key",
					Targets:      []string{missingKey},
					AllowAbsence: true,
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"the exact config key is absent from every requested layer",
		"confidence":"high",
		"result_kind":"absence",
		"aggregate_facts":[
			{
				"kind":"negative_search",
				"label":"explore_xyz_phantom_unique_budget in production code",
				"value":"0",
				"unit":"matches",
				"dimensions":[
					{"name":"repo","value":"hanchaoqun/codrax"},
					{"name":"pattern","value":"explore_xyz_phantom_unique_budget"},
					{"name":"scope","value":"production Go files"},
					{"name":"searched_at","value":"current_investigation"}
				]
			},
			{
				"kind":"member_set",
				"label":"related explore config keys",
				"role":"supporting_coverage",
				"members":[
					"ExploreSettings.PerToolDefaultCap (config.go:852, yaml标签 per_tool_default_cap)"
				]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("unsupported supporting member_set should not hard-retry an absence closure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "dropped supporting_coverage member_set") {
		t.Fatalf("summary should record dropped support aggregate, got: %s", res.Summary)
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) != 1 || got[0].Kind != types.AnswerAggregateNegativeSearch {
		t.Fatalf("stored aggregate facts = %+v, want only negative_search", got)
	}
}

func TestEmitInvestigationComplete_AbsenceDoesNotDropUnknownDecoratedMemberSet(t *testing.T) {
	const missingKey = "explore_xyz_phantom_unique_budget"
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:   types.SubjectConfigKey,
					TargetLabel:  "config key",
					Targets:      []string{missingKey},
					AllowAbsence: true,
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"the exact config key is absent from every requested layer",
		"confidence":"high",
		"result_kind":"absence",
		"aggregate_facts":[
			{
				"kind":"negative_search",
				"label":"explore_xyz_phantom_unique_budget in production code",
				"value":"0",
				"unit":"matches",
				"dimensions":[
					{"name":"repo","value":"hanchaoqun/codrax"},
					{"name":"pattern","value":"explore_xyz_phantom_unique_budget"},
					{"name":"scope","value":"production Go files"},
					{"name":"searched_at","value":"current_investigation"}
				]
			},
			{
				"kind":"member_set",
				"label":"maybe principal related keys",
				"members":[
					"ExploreSettings.PerToolDefaultCap (config.go:852, yaml标签 per_tool_default_cap)"
				]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("unknown-role decorated member_set support_refs debt should downgrade instead of tool-failing: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("first support_refs downgrade must not complete an absence closure")
	}
	if !strings.Contains(res.Summary, EmitInvestigationCompleteDowngradePrefix) ||
		!strings.Contains(res.Summary, "support_refs is empty") {
		t.Fatalf("summary should preserve the support_refs contract as downgrade, got: %s", res.Summary)
	}
}

func TestEffectiveCompletionAggregateFacts_CurrentPayloadReplacesStaleRetainedFacts(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "stale bucket",
		Value:       "3",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"old-a", "old-b", "old-c"},
		SupportRefs: []string{"old.go:1", "old.go:2", "old.go:3"},
	}})
	mut.SetInvestigationComplete("previous accepted closure")
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{Mutable: mut}

	current := []types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "current bucket",
		Value:       "1",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"new-a"},
		SupportRefs: []string{"new.go:10"},
	}}
	got := effectiveCompletionAggregateFacts(ctx, current)
	if len(got) != 1 || got[0].Label != "current bucket" || got[0].Members[0] != "new-a" {
		t.Fatalf("current aggregate_facts must replace stale retained facts, got %+v", got)
	}
}

func TestEffectiveCompletionAggregateFacts_MonotonicMergesSamePrincipalMemberSet(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "foreign func 声明",
		Value: "2",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"native_add @ internal/thirdparty/tree-sitter-cangjie/corpus/sources/07_foreign_ffi.cj:6 (package demo.ffi)",
			"native_add @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6 (package demo.bridge)",
		},
	}})
	mut.SetInvestigationComplete("previous accepted closure")
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{Mutable: mut}

	current := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "foreign func 声明",
		Value:   "1",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"native_add @ eval/fixtures/testdata/cangjie_minimal/bridge/Bridge.cj:6 (package demo.bridge)"},
	}}
	got := effectiveCompletionAggregateFacts(ctx, current)
	if len(got) != 1 || got[0].Value != "2" || len(got[0].Members) != 2 {
		t.Fatalf("same principal member_set must merge monotonically instead of narrowing: %+v", got)
	}
	visible := strings.Join(append(append([]string{}, got[0].Members...), got[0].SupportRefs...), "\n")
	for _, want := range []string{"07_foreign_ffi.cj:6", "Bridge.cj:6"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("merged member_set lost %q: %+v", want, got[0].Members)
		}
	}
}

func TestEffectiveCompletionAggregateFacts_EmptyPayloadCarriesForwardRetainedFacts(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "accepted bucket",
		Value:   "1",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"accepted"},
	}})
	mut.SetInvestigationComplete("previous accepted closure")
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{Mutable: mut}

	got := effectiveCompletionAggregateFacts(ctx, nil)
	if len(got) != 1 || got[0].Label != "accepted bucket" || got[0].Members[0] != "accepted" {
		t.Fatalf("empty aggregate_facts should carry forward retained facts, got %+v", got)
	}
}

func TestEmitInvestigationComplete_AcceptsStringEncodedAggregateFacts(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}
	encodedFacts, err := json.Marshal([]map[string]any{{
		"kind":    "member_set",
		"label":   "enum type names",
		"value":   "2",
		"members": []string{"Intent", "Scenario"},
	}})
	if err != nil {
		t.Fatalf("marshal facts: %v", err)
	}
	paramsBytes, err := json.Marshal(map[string]any{
		"reason":          "model emitted valid aggregate_facts JSON inside a string field",
		"confidence":      "high",
		"result_kind":     "resolved",
		"aggregate_facts": string(encodedFacts),
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	res, err := tool.Execute(bus, paramsBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("string-encoded aggregate facts should be accepted when the string is a complete JSON array: %s", res.Summary)
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) != 1 || got[0].Kind != types.AnswerAggregateMemberSet || got[0].Members[1] != "Scenario" {
		t.Fatalf("string-encoded aggregate facts not retained: %+v", got)
	}
}

func TestEmitInvestigationComplete_RepairsStringEncodedAggregateFactsWithMisplacedResolvedTail(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	paramsBytes, err := json.Marshal(map[string]any{
		"reason":      "model emitted aggregate_facts as a JSON string with a misplaced sibling field tail",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": `[
			{"kind":"member_set","label":"read mode stages","value":"2","members":["StageAnalyze","StageExplore"]}
		], "absence_justification": "No write-mode stages execute in read mode."`,
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	res, err := tool.Execute(bus, paramsBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("misplaced aggregate_facts tail should not reject a resolved completion: %s", res.Summary)
	}
	if got := mut.AbsenceJustification(); got != "" {
		t.Fatalf("resolved completion must not promote misplaced absence_justification, got %q", got)
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) != 1 || got[0].Kind != types.AnswerAggregateMemberSet || got[0].Value != "2" {
		t.Fatalf("aggregate facts not recovered from leading array: %+v", got)
	}
}

func TestEmitInvestigationComplete_RepairsStringEncodedAggregateFactsWithMisplacedTopLevelFields(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	paramsBytes, err := json.Marshal(map[string]any{
		"aggregate_facts": `[
			{"kind":"scalar_value","label":"类型数量","value":"3"},
			{"kind":"member_set","label":"类型完整列表","value":"3","members":["Kind","Env","Result"]}
		], "confidence": "high", "reason": "complete typed handoff", "result_kind": "resolved"`,
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	res, err := tool.Execute(bus, paramsBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("misplaced confidence/reason/result_kind tail should be recovered: %s", res.Summary)
	}
	if got := mut.InvestigationResultKind(); got != "resolved" {
		t.Fatalf("result_kind = %q, want recovered resolved", got)
	}
	if got := mut.InvestigationCompleteReason(); got != "complete typed handoff" {
		t.Fatalf("reason = %q, want recovered misplaced reason", got)
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) != 2 || got[1].Kind != types.AnswerAggregateMemberSet || got[1].Value != "3" {
		t.Fatalf("aggregate facts not recovered with misplaced top-level fields: %+v", got)
	}
}

func TestEmitInvestigationComplete_RepairsReasonSuffixMisplacedTopLevelFields(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	paramsBytes, err := json.Marshal(map[string]any{
		"reason": `LoopController implementations are fully enumerated.
confidence: high, result_kind: resolved, aggregate_facts: [
  {"kind":"member_set","label":"production LoopController implementations","value":"2","role":"principal_answer","members":["analyzerEvaluator","explorerEvaluator"]}
]`,
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	res, err := tool.Execute(bus, paramsBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("reason-suffix payload should be recovered: %s", res.Summary)
	}
	if got := mut.InvestigationResultKind(); got != "resolved" {
		t.Fatalf("result_kind = %q, want recovered resolved", got)
	}
	if got := mut.InvestigationCompleteReason(); got != "LoopController implementations are fully enumerated." {
		t.Fatalf("reason = %q, want clean conclusion", got)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 || facts[0].Kind != types.AnswerAggregateMemberSet || len(facts[0].Members) != 2 {
		t.Fatalf("aggregate facts not recovered from reason suffix: %+v", facts)
	}
}

func TestEmitInvestigationComplete_DoesNotPromoteOrdinaryReasonConfidenceProse(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	paramsBytes, err := json.Marshal(map[string]any{
		"reason": "confidence: high appears here as ordinary rationale text, not a misplaced tool payload.",
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	res, err := tool.Execute(bus, paramsBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("single schema-looking word in prose must not be promoted into a completion")
	}
	if !strings.Contains(res.Summary, "confidence") {
		t.Fatalf("rejection should still ask for the missing confidence field, got: %s", res.Summary)
	}
	if got := strings.TrimSpace(mut.InvestigationCompleteReason()); got != "" {
		t.Fatalf("ordinary prose must not mark investigation complete, got reason %q", got)
	}
}

func TestEmitInvestigationComplete_PromotesMisplacedAbsenceTailForAbsenceResult(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	paramsBytes, err := json.Marshal(map[string]any{
		"reason":          "searched and found none",
		"confidence":      "high",
		"result_kind":     "absence",
		"aggregate_facts": `[], "absence_justification": "no .py files exist"`,
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	res, err := tool.Execute(bus, paramsBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("absence completion should promote misplaced absence_justification: %s", res.Summary)
	}
	if got := mut.AbsenceJustification(); got != "no .py files exist" {
		t.Fatalf("absence justification = %q, want promoted misplaced string", got)
	}
}

func TestEmitInvestigationComplete_RepairsStringEncodedAggregateFactsWithDanglingTailCloser(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	paramsBytes, err := json.Marshal(map[string]any{
		"reason":      "model emitted aggregate_facts as a JSON string copied from an object member",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": `[
			{"kind":"member_set","label":"core types","value":"2","members":["SubAgentValidator","SubAgentReducer"],}
		], "absence_justification": "not an absence answer"}`,
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}

	res, err := tool.Execute(bus, paramsBytes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("dangling object-tail closer and trailing commas should be repaired: %s", res.Summary)
	}
	if got := mut.AbsenceJustification(); got != "" {
		t.Fatalf("resolved completion must not promote misplaced absence_justification, got %q", got)
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) != 1 || got[0].Kind != types.AnswerAggregateMemberSet || len(got[0].Members) != 2 {
		t.Fatalf("aggregate facts not recovered: %+v", got)
	}
}

func TestEmitInvestigationComplete_RejectsInvalidAggregateFacts(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"done",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{"kind":"guess","label":"x","value":"1"}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("invalid aggregate kind should reject")
	}
	if !strings.Contains(res.Summary, "aggregate_facts[0]") {
		t.Fatalf("rejection should name aggregate fact path: %s", res.Summary)
	}
	if got := mut.StableInvestigationAggregateFacts(); len(got) != 0 {
		t.Fatalf("rejected aggregate facts must not be retained: %+v", got)
	}
}

func TestEmitInvestigationComplete_DropsOptionalInvalidAggregateFactsOnNarrativeRequest(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: "从历史提交里找一次相关改动，再结合当前代码解释现在链路怎么工作",
			Intent:     types.IntentExplain,
			Scenario:   types.ScenarioArchitectureExplain,
			Complexity: types.ComplexityComplex,
			Predicates: types.SemanticPredicates{IsHistoryLookup: true},
			AnalyzerHints: types.AnalyzerHints{
				Kind: string(types.ReqMechanism),
			},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"history evidence plus current implementation evidence are sufficient",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{"kind":"negative_search","label":"not actually negative","value":"3"}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("optional invalid aggregate fact should not force a retry: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "aggregate_facts normalized") {
		t.Fatalf("summary should disclose the dropped optional aggregate fact: %s", res.Summary)
	}
	if got := mut.StableInvestigationAggregateFacts(); len(got) != 0 {
		t.Fatalf("invalid optional aggregate fact must not be retained: %+v", got)
	}
}

func TestEmitInvestigationComplete_DerivesBucketCountValueFromMembers(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"bucket members fully describe the analyze success conditions",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"bucket_count",
			"label":"runAnalyzePhase 成功条件",
			"unit":"条件项",
			"members":["err == nil","out != nil","out.Error == \"\"","out.AnalysisIR != nil"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("bucket_count with exact members should not force JSON repair: %s", res.Summary)
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) != 1 || got[0].Value != "4" {
		t.Fatalf("stable bucket_count = %+v, want value derived from 4 members", got)
	}
}

func TestEmitInvestigationComplete_RepairsMalformedMemberSetValueFromMembers(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"member set is structurally complete even though value has a suffix",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"callable agents",
			"value":"1+",
			"members":["Orchestrator"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("malformed member_set value should be repaired from exact members, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "aggregate_facts normalized") {
		t.Fatalf("summary should disclose aggregate value repair: %s", res.Summary)
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) != 1 || got[0].Value != "1" || len(got[0].Members) != 1 {
		t.Fatalf("stable member_set = %+v, want value=1 from members", got)
	}
}

func TestEmitInvestigationComplete_DropsOptionalUnsupportedDecoratedMemberSetAfterDemotion(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: "从历史提交里找一次相关改动，再结合当前代码解释现在链路怎么工作",
			Intent:     types.IntentExplain,
			Scenario:   types.ScenarioArchitectureExplain,
			Complexity: types.ComplexityComplex,
			Predicates: types.SemanticPredicates{IsHistoryLookup: true},
			AnalyzerHints: types.AnalyzerHints{
				Kind: string(types.ReqMechanism),
			},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"the prose conclusion and grounded evidence carry the answer; the member_set is only support",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"supporting modes",
			"role":"principal_answer",
			"value":"3",
			"members":["ScalarSourceLiteral (IsScalarAnswer=true)", "ScalarRoleLocate (IsRoleLocateLookup=true)", "ScalarCount (IsCountQuestion=true)"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("optional demoted decorated member_set should not force a retry: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "dropped supporting_coverage member_set") {
		t.Fatalf("summary should disclose dropped optional member_set: %s", res.Summary)
	}
	if got := mut.StableInvestigationAggregateFacts(); len(got) != 0 {
		t.Fatalf("unsupported optional member_set must be dropped, got %+v", got)
	}
}

func TestEmitInvestigationComplete_DropsOptionalPrincipalDecoratedMemberSetForMechanismNarrative(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "run-analyze-phase",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/orchestrator/orchestrator.go",
		LineStart:       2225,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "runAnalyzePhase",
		Subject:         "runAnalyzePhase",
		Summary:         "runAnalyzePhase owns the analyzer retry loop and records degraded recovery when emit_analysis cannot be obtained.",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: "说明 runTaskGraph 下 analyze 阶段如果一直没有调用 emit_analysis 会怎样重试",
			Intent:     types.IntentExplain,
			Scenario:   types.ScenarioArchitectureExplain,
			Complexity: types.ComplexityComplex,
			Predicates: types.SemanticPredicates{
				IsScalarAnswer:        false,
				IsCountQuestion:       false,
				IsCategoryEnumeration: false,
				IsRelationalLookup:    false,
			},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"grounded prose evidence already explains the retry mechanism; aggregate_facts is only an optional row scaffold",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"analyze 阶段重试关键点",
			"role":"principal_answer",
			"value":"2",
			"members":["MaxRetriesPerStage (配置上限)", "dynamicAnalyzeRetries (动态扩展)"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("optional decorated principal member_set should not force mechanism exploration retry: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "dropped principal_answer member_set") {
		t.Fatalf("summary should disclose dropped optional principal member_set: %s", res.Summary)
	}
	if got := mut.StableInvestigationAggregateFacts(); len(got) != 0 {
		t.Fatalf("unsupported optional principal member_set must be dropped, got %+v", got)
	}
}

func TestEmitInvestigationComplete_RuntimeNegativeObservationCompat(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			LogTriage: &types.LogBundle{Observations: []types.LogObservation{{
				Kind:       types.LogObservationRuntimeEvent,
				Summary:    "FATAL is absent",
				Confidence: 1,
			}}},
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
				ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
				SourceQuotes:      []string{"只分析日志"},
				Confidence:        0.9,
			},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"FATAL absent from attached log",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"negative_observation",
			"label":"FATAL absence",
			"value":"0",
			"excluded":["FATAL"],
			"dimensions":[
				{"name":"scope","value":"attached log"},
				{"name":"searched_at","value":"current_investigation"}
			]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("runtime negative observation should be normalized instead of rejected: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 {
		t.Fatalf("expected one aggregate fact, got %+v", facts)
	}
	var dimParts []string
	for _, dim := range facts[0].Dimensions {
		dimParts = append(dimParts, dim.Name+"="+dim.Value)
	}
	joined := strings.Join(dimParts, ",")
	if !strings.Contains(joined, "origin=runtime_artifact") || !strings.Contains(joined, "target=FATAL") {
		t.Fatalf("negative observation dimensions not normalized: %s", joined)
	}
}

func TestEmitInvestigationComplete_RuntimeNegativeObservationMissingSignalCompat(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			PerfTrace: &types.PerfBundle{Observations: []types.PerfObservation{{
				Kind:       "attached_trace",
				Subject:    "attached trace",
				Summary:    "trace contains sched_blocked_reason but no storage rows",
				Confidence: 1,
			}}},
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
				CurrentSourceMode:    types.ExternalObservationCurrentSourceExclude,
				ExclusionKind:        types.ExternalObservationSourceExclusionExplicitUserBoundary,
				Confidence:           0.9,
			},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"trace rows are sufficient to answer the runtime-only question",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"negative_observation",
			"label":"IO 层证据缺失",
			"value":"0",
			"dimensions":[
				{"name":"missing_signal","value":"inode/block_dev/file_bytes/storage_latency"},
				{"name":"reason","value":"trace has sched_blocked_reason and no block_rq/filesystem rows"}
			]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("runtime missing-signal negative observation should normalize without retry: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 {
		t.Fatalf("expected one aggregate fact, got %+v", facts)
	}
	var dimParts []string
	for _, dim := range facts[0].Dimensions {
		dimParts = append(dimParts, dim.Name+"="+dim.Value)
	}
	joined := strings.Join(dimParts, ",")
	for _, want := range []string{
		"origin=runtime_artifact",
		"target=inode/block_dev/file_bytes/storage_latency",
		"scope=attached_runtime_trace",
		"result_count=0",
		"searched_at=current_investigation",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("normalized dimensions missing %q: %s", want, joined)
		}
	}
}

func TestEmitInvestigationComplete_ExplicitAllowWithoutPreciseSourceRequirementAcceptsExternalOnlyWaiver(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentRootCause,
			Scenario: types.ScenarioRootCause,
			Predicates: types.SemanticPredicates{
				IsDiagnosticQuestion: true,
			},
			LogTriage: &types.LogBundle{Observations: []types.LogObservation{{
				Kind:       types.LogObservationRuntimeEvent,
				Summary:    "finalizer attempt failed after LLM stream timeout",
				Confidence: 0.95,
			}}},
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
				CurrentSourceMode:    types.ExternalObservationCurrentSourceAllow,
				Confidence:           0.95,
			},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"runtime log explains the observed timeout and finalizer retry",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{
			"reason":"external_only_log",
			"rationale":"the log lines are external observations"
		},
		"aggregate_facts":[{
			"kind":"behavior_outcome",
			"label":"runtime outcome",
			"value":"finalizer retry after stream timeout",
			"role":"principal_answer",
			"dimensions":[{"name":"origin","value":"runtime_artifact"}]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("external-only waiver should be accepted without precise current-source requirement: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("completion should close when allow has no precise current-source proof obligation; summary=%s", res.Summary)
	}
	if strings.Contains(res.Summary, "current-source") {
		t.Fatalf("allow-only runtime completion must not reopen current-source repair, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_RequiredCurrentKeyCodeStillBlocksExternalOnlyWaiver(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentRootCause,
			Scenario: types.ScenarioPerformanceBottleneck,
			Predicates: types.SemanticPredicates{
				IsDiagnosticQuestion: true,
			},
			PerfTrace: &types.PerfBundle{Observations: []types.PerfObservation{{
				Kind:       "state_churn",
				Subject:    "app-20",
				Summary:    "runtime trace state_churn metrics",
				LineStart:  3,
				LineEnd:    23,
				DurationMs: 8,
			}}},
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
				CurrentSourceMode:    types.ExternalObservationCurrentSourceAllow,
				Confidence:           0.95,
			},
			SourceScopeProfile: &types.SourceScopeProfile{
				RequestedScope: types.SourceScopeProduction,
				Confidence:     0.8,
			},
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{
					Label:    "current implementation",
					Role:     types.RequestedAnswerDimensionCurrentKeyCode,
					Required: true,
					Index:    1,
				}},
				Confidence: 0.9,
			},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"runtime trace shows the observed churn",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{
			"reason":"external_only_trace",
			"rationale":"the trace rows are external observations"
		},
		"aggregate_facts":[{
			"kind":"behavior_outcome",
			"label":"runtime outcome",
			"value":"state churn",
			"role":"principal_answer",
			"dimensions":[{"name":"origin","value":"runtime_artifact"}]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("current-source downgrade should remain soft, got hard failure: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("required current_key_code dimension must not close through external-only waiver")
	}
	if !strings.Contains(res.Summary, "current-source") {
		t.Fatalf("summary should point at the precise current-source requirement, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_DroppedCurrentSourceObligationCompletesWithCaveat(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentRootCause,
			Scenario: types.ScenarioPerformanceBottleneck,
			Predicates: types.SemanticPredicates{
				IsDiagnosticQuestion: true,
			},
			PerfTrace: &types.PerfBundle{Observations: []types.PerfObservation{{
				Kind:       "trace_mark",
				Subject:    "DoFrame",
				Summary:    "runtime trace span is janky",
				LineStart:  5,
				LineEnd:    6,
				DurationMs: 86.111,
			}}},
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
				CurrentSourceMode:    types.ExternalObservationCurrentSourceDefault,
				Confidence:           0.95,
			},
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{
					Label:    "evidence boundary",
					Role:     types.RequestedAnswerDimensionBoundary,
					Required: true,
					Index:    3,
				}},
				Confidence: 0.9,
			},
			CurrentSourceObligationSignals: []types.CurrentSourceObligationSignal{{
				Kind:  types.CurrentSourceObligationSignalDroppedRequestedDimension,
				Role:  types.RequestedAnswerDimensionCurrentKeyCode,
				Index: 2,
			}},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"runtime trace shows the observed span",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{
			"reason":"external_only_trace",
			"rationale":"the trace rows are external observations"
		},
		"aggregate_facts":[{
			"kind":"behavior_outcome",
			"label":"runtime outcome",
			"value":"janky span",
			"role":"principal_answer",
			"dimensions":[{"name":"origin","value":"runtime_artifact"}]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("dropped-obligation downgrade should remain soft, got hard failure: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("dropped current-source obligation without a precise source anchor should close with caveat")
	}
	caveats := mut.EvidenceClosure().CompletionCaveats()
	if len(caveats) != 1 || caveats[0].Lane != types.DowngradeLaneCurrentSourceLane {
		t.Fatalf("dropped current-source obligation should be recorded as a current-source caveat, got: %#v", caveats)
	}
}

func TestEmitInvestigationComplete_NormalizesNegativeObservationAliasPayload(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"runtime zero-result observation is complete",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"negative_observation",
			"label":"IO/binder/IRQ absence",
			"value":"0",
			"unit":"events",
			"origin":"trace_query window_stats.counts",
			"checked_types":["binder","irq","softirq"],
			"window":"11.000s-11.008s cpu=1",
			"matches":0
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("alias negative_observation should normalize without retry: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 {
		t.Fatalf("expected one aggregate fact, got %+v", facts)
	}
	var dimParts []string
	for _, dim := range facts[0].Dimensions {
		dimParts = append(dimParts, dim.Name+"="+dim.Value)
	}
	joined := strings.Join(dimParts, ",")
	for _, want := range []string{
		"origin=runtime_artifact",
		"target=binder, irq, softirq",
		"scope=11.000s-11.008s cpu=1",
		"result_count=0",
		"searched_at=current_investigation",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("normalized dimensions missing %q: %s", want, joined)
		}
	}
}

func TestEmitInvestigationComplete_RejectsNegativeObservationAliasPayloadWithCurrentSourceOrigin(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"invalid current-source negative observation",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"negative_observation",
			"label":"missing runtime row",
			"value":"0",
			"origin":"current_source",
			"target":"binder",
			"scope":"internal",
			"matches":0
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}
	if res.Success {
		t.Fatalf("invalid origin must not be accepted: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "requires a non-repo origin dimension") {
		t.Fatalf("rejection should preserve non-repo origin hard gate, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_NormalizesRuntimeOriginAliasesForPositiveAggregate(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"trace query measured the blocked main-thread duration",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"scalar_value",
			"label":"main-thread running slice",
			"value":"281.940",
			"unit":"ms",
			"role":"principal_answer",
			"evidence_origin":"runtime_artifact",
			"artifact_id":"trace_query",
			"trace_window":"62380.172..62380.455",
			"scope":"current trace selection"
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("positive runtime aggregate aliases should normalize without retry: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 {
		t.Fatalf("expected one aggregate fact, got %+v", facts)
	}
	assertAggregateDimension(t, facts[0], "evidence_origin", "runtime_artifact")
	assertAggregateDimension(t, facts[0], "artifact_id", "trace_query")
	assertAggregateDimension(t, facts[0], "trace_window", "62380.172..62380.455")
	assertAggregateDimension(t, facts[0], "scope", "current trace selection")
}

func TestEmitInvestigationComplete_RuntimeAliasCompatDoesNotAllowUnknownAggregateFields(t *testing.T) {
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"trace query measured the blocked main-thread duration",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"scalar_value",
			"label":"main-thread running slice",
			"value":"281.940",
			"evidence_origin":"runtime_artifact",
			"bogus_axis":"should still fail"
		}]
	}`)
	_, failure, err := decodeEmitInvestigationCompleteParamsStrict(tool.Name(), params, tool.Parameters())
	if err == nil {
		t.Fatalf("expected unknown non-alias aggregate field to remain rejected")
	}
	if failure == nil || failure.Success {
		t.Fatalf("expected strict decode failure result, got %+v", failure)
	}
	if !strings.Contains(err.Error(), `unknown field "bogus_axis"`) {
		t.Fatalf("expected bogus_axis unknown-field error, got: %v", err)
	}
}

func TestEmitInvestigationComplete_AcceptsCategoricalBehaviorAggregate(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"invalid emit_evidence rows are rejected per item while the batch can still accept grounded siblings",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"error_granularity_verdict",
			"label":"emit_evidence invalid item failure scope",
			"value":"per_item_rejection",
			"role":"principal_answer",
			"dimensions":[
				{"name":"target","value":"emit_evidence invalid item"},
				{"name":"predicate","value":"failure_scope"}
			],
			"support_refs":["internal/tool/emit_evidence.go:560"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("categorical aggregate should not be forced into count/member schemas: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 {
		t.Fatalf("expected one aggregate fact, got %+v", facts)
	}
	if facts[0].Kind != types.AnswerAggregateErrorGranularity || facts[0].Value != "per_item_rejection" {
		t.Fatalf("categorical aggregate not preserved: %+v", facts[0])
	}
}

func TestEmitInvestigationComplete_RuntimeArtifactDropsInvalidOptionalAggregateFact(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentReturnValue,
			Predicates: types.SemanticPredicates{
				IsScalarAnswer: true,
			},
			LogTriage: &types.LogBundle{Observations: []types.LogObservation{{
				Kind:       types.LogObservationRuntimeEvent,
				Summary:    "FATAL is absent",
				Confidence: 1,
			}}},
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
				ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
				SourceQuotes:      []string{"只分析日志"},
				Confidence:        0.9,
			},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"WARN line and FATAL absence are visible in the attached log observations",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{"kind":"scalar_value","label":"WARN line","value":"3","provenance":"runtime_artifact"},
			{"kind":"negative_observation","label":"FATAL absence","value":"0","dimensions":[{"name":"scope","value":"attached log"}]}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("invalid optional runtime aggregate fact should not force retry: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "dropped optional aggregate_facts") {
		t.Fatalf("summary should disclose optional aggregate drop: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 || facts[0].Kind != types.AnswerAggregateScalar {
		t.Fatalf("valid scalar aggregate should survive while invalid negative observation drops: %+v", facts)
	}
}

func TestEmitInvestigationComplete_DropsSupportingRepoSourceNegativeObservationWhenPrincipalSurvives(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"principal member_set is complete; EntryAbility is only related context",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"@Entry 页面入口",
				"value":"1",
				"role":"principal_answer",
				"members":["Index"]
			},
			{
				"kind":"negative_observation",
				"label":"EntryAbility 无 @Entry 装饰器",
				"value":"0",
				"role":"supporting_coverage",
				"provenance":"read_file",
				"dimensions":[
					{"name":"scope","value":"internal/thirdparty/tree-sitter-arkts/corpus/sources/06_entry_ability_stage_model.ets"},
					{"name":"target","value":"@Entry decorator"},
					{"name":"searched_at","value":"current_investigation"}
				]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("supporting repo-source negative context should drop locally, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "dropped contextual aggregate_facts") {
		t.Fatalf("summary should disclose contextual aggregate drop: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 || facts[0].Kind != types.AnswerAggregateMemberSet || facts[0].Members[0] != "Index" {
		t.Fatalf("principal member_set should survive while contextual negative drops: %+v", facts)
	}
}

func TestEmitInvestigationComplete_DropsImpreciseRepoSourceNegativeObservationWhenPrincipalSurvives(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"principal member_set is complete; the zero row is only a non-precise source-inventory coverage note",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"@Entry 页面入口",
				"value":"1",
				"role":"principal_answer",
				"members":["Index"]
			},
			{
				"kind":"negative_observation",
				"label":"04_styles_extend.ets zero source-inventory rows",
				"value":"0",
				"origin":"source_inventory",
				"dimensions":[
					{"name":"scope","value":"internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets"},
					{"name":"searched_at","value":"current_investigation"}
				]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("imprecise contextual repo negative observation should drop locally, got: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "dropped contextual aggregate_facts") {
		t.Fatalf("summary should disclose contextual aggregate drop: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 || facts[0].Kind != types.AnswerAggregateMemberSet || facts[0].Members[0] != "Index" {
		t.Fatalf("principal member_set should survive while imprecise contextual negative drops: %+v", facts)
	}
}

func TestEmitInvestigationComplete_DropsFileBoundRepoNegativeObservationSidecar(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"principal member sets are complete; one bounded file merely lacks the decorator",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"@Entry 页面入口",
				"value":"1",
				"role":"principal_answer",
				"members":["Index"]
			},
			{
				"kind":"negative_observation",
				"label":"@Entry 装饰器不存在",
				"value":"0",
				"provenance":"read_file full file + scope=negative cross-check",
				"dimensions":[
					{"name":"file","value":"src/pages/EntryAbility.ets"},
					{"name":"reason","value":"EntryAbility 使用 UIAbility，无 @Entry 装饰器"}
				]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("file-bound repo negative sidecar should drop locally, got: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 || facts[0].Kind != types.AnswerAggregateMemberSet || facts[0].Members[0] != "Index" {
		t.Fatalf("principal member_set should survive while file-bound negative sidecar drops: %+v", facts)
	}
}

func TestEmitInvestigationComplete_DoesNotDropOnlyImpreciseRepoSourceNegativeObservation(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"invalid lone current-source negative observation",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"negative_observation",
			"label":"repo absence without target",
			"value":"0",
			"origin":"source_inventory",
			"dimensions":[
				{"name":"scope","value":"internal"},
				{"name":"searched_at","value":"current_investigation"}
			]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}
	if res.Success {
		t.Fatalf("lone imprecise repo-source negative_observation must still reject: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "requires a non-repo origin dimension") {
		t.Fatalf("rejection should preserve non-repo origin hard gate, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_DoesNotDropPrincipalRepoSourceNegativeObservation(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"invalid principal current-source negative observation",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"context",
				"value":"1",
				"role":"principal_answer",
				"members":["Index"]
			},
			{
				"kind":"negative_observation",
				"label":"principal repo absence",
				"value":"0",
				"role":"principal_answer",
				"origin":"current_source",
				"target":"binder",
				"scope":"internal",
				"matches":0
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected execution error: %v", err)
	}
	if res.Success {
		t.Fatalf("principal repo-source negative_observation must still reject: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "requires a non-repo origin dimension") {
		t.Fatalf("rejection should preserve non-repo origin hard gate, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_RejectsInconsistentAggregateFacts(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"done",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"total_count",
			"label":"production locations",
			"value":"2",
			"unit":"locations",
			"members":["internal/a.go:10","src/native/foo.cpp:22"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("missing unique_count companion should reject")
	}
	if !strings.Contains(res.Summary, "unique_count") {
		t.Fatalf("rejection should explain missing unique_count companion: %s", res.Summary)
	}
	if got := mut.StableInvestigationAggregateFacts(); len(got) != 0 {
		t.Fatalf("rejected aggregate facts must not be retained: %+v", got)
	}
}

func TestEmitInvestigationComplete_RejectsDecoratorMemberSetMismatchedToEvidence(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			AnalyzerHints: types.AnalyzerHints{Entities: []string{"@Builder"}},
		}},
	}
	seedReadFileHistory(bus, "internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets", 1,
		"// Source: openharmony @Styles and @Extend decorators",
		"",
		"@Styles function commonCardStyle() {",
		"  .width('100%')",
		"}",
	)
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceRegistration,
		Source:          "internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets",
		LineStart:       3,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "commonCardStyle",
		Object:          "Styles",
		SurfaceTerms:    []string{"@Styles", "commonCardStyle"},
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"member set carried through aggregate_facts",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"@Builder reusable fragments",
			"value":"1",
			"members":["commonCardStyle"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("expected decorator aggregate mismatch rejection, got success")
	}
	if !strings.Contains(res.Summary, "requested decorator \"@Builder\"") || !strings.Contains(res.Summary, "@Styles") {
		t.Fatalf("rejection should explain aggregate member mismatch, got %q", res.Summary)
	}
}

func TestEmitInvestigationComplete_NormalizesLogSourceDriftReasonToBoundedSurface(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{
			Frames: []types.LogFrame{
				{File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR"},
				{File: "internal/agent/analyzer.go", Line: 320, Func: "(*analyzerEvaluator).ParseOutput"},
			},
		}},
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceRelationship,
			Source:          "internal/agent/analyzer.go",
			LineStart:       651,
			AnchorKind:      types.AnchorCall,
			AnchorSymbol:    "buildAnalysisIR",
			Subject:         "ParseOutput",
			Object:          "buildAnalysisIR",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceConditional,
			Source:          "internal/agent/analyzer.go",
			LineStart:       861,
			AnchorKind:      types.AnchorCondition,
			AnchorSymbol:    "buildAnalysisIR",
			Condition:       "ctx == nil || ctx.Mutable == nil",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioRootCause,
				Intent:   types.IntentRootCause,
			},
			AnswerContract: types.AnswerContract{},
		},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"the nil receiver path proves ParseOutput panicked before entering the method body","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("bounded drift completion should still succeed: %s", res.Summary)
	}
	got := mut.InvestigationCompleteReason()
	if strings.Contains(strings.ToLower(got), "nil receiver") {
		t.Fatalf("completion reason should be normalized to bounded drift surface, got %q", got)
	}
	for _, want := range []string{"ParseOutput", "buildAnalysisIR"} {
		if !strings.Contains(got, want) {
			t.Fatalf("completion reason missing %q: %q", want, got)
		}
	}
}

func TestEmitInvestigationComplete_AbsenceRequiresHonestZeroPhrasing(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"all necessary evidence has been collected","confidence":"high","result_kind":"resolved","absence_justification":"already found enough evidence to explain it"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("positive completion text must not be accepted as absence: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "result_kind=absence") {
		t.Errorf("rejection should explain structured absence contract: %s", res.Summary)
	}
	if mut.AbsenceJustification() != "" {
		t.Errorf("absence must NOT be stored on rejection")
	}
}

func TestEmitInvestigationComplete_ConfigTraceAbsenceRejectsGroundedSameScopeContextBeforeValidatedPrecedenceRole(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/types/config.go",
		LineStart:       707,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "DefaultExploreHeuristics",
		ContextRole:     types.EvidenceContextRoleRelatedContext,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{"explore_mid_loop_hint_budget"},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"done","confidence":"high","result_kind":"absence","absence_justification":"no config key named explore_mid_loop_hint_budget exists"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("config-trace absence context debt should soft-downgrade, not hard-reject: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatal("config-trace absence context debt must not mark completion before convergence")
	}
	if !strings.Contains(res.Summary, "precedence-capable lineage anchor") || !strings.Contains(res.Summary, "diagram_role_hint") {
		t.Fatalf("rejection should explain the missing precedence-role requirement, got %q", res.Summary)
	}
	if res.Repair == nil {
		t.Fatal("missing structured repair on precedence-anchor rejection")
	}
	if got := res.Repair.Code; got != "exact_absence_precedence_anchor" {
		t.Fatalf("repair code = %q, want exact_absence_precedence_anchor", got)
	}
	if len(res.Repair.Targets) != 1 || res.Repair.Targets[0].File != "internal/types/config.go" || res.Repair.Targets[0].Action != string(types.RepairReadFile) {
		t.Fatalf("repair targets = %+v, want read_file on internal/types/config.go", res.Repair.Targets)
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) == 0 || repairs[0].Kind != types.RepairReadFile {
		t.Fatalf("closure repairs = %+v, want queued read repair", repairs)
	}
}

func TestEmitInvestigationComplete_ConfigTraceAbsenceAlreadyReadScopeQueuesEmitEvidenceRepair(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.EvidenceClosure().SetReadSet(map[string]bool{"internal/types/config.go": true})
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/types/config.go",
		LineStart:       707,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "DefaultExploreHeuristics",
		ContextRole:     types.EvidenceContextRoleRelatedContext,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{"explore_mid_loop_hint_budget"},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"done","confidence":"high","result_kind":"absence","absence_justification":"no config key named explore_mid_loop_hint_budget exists"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("config-trace absence evidence debt should soft-downgrade, not hard-reject: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatal("config-trace absence evidence debt must not mark completion before convergence")
	}
	if !strings.Contains(res.Summary, "already in read coverage") || !strings.Contains(res.Summary, "Do NOT widen scope") {
		t.Fatalf("rejection should explain that already-read scope needs evidence materialization, got %q", res.Summary)
	}
	if res.Repair == nil {
		t.Fatal("missing structured repair on already-read precedence-anchor rejection")
	}
	if got := res.Repair.Code; got != "exact_absence_precedence_evidence" {
		t.Fatalf("repair code = %q, want exact_absence_precedence_evidence", got)
	}
	if len(res.Repair.Targets) != 1 || res.Repair.Targets[0].File != "internal/types/config.go" || res.Repair.Targets[0].Action != string(types.RepairEmitEvidence) {
		t.Fatalf("repair targets = %+v, want emit_evidence on internal/types/config.go", res.Repair.Targets)
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) == 0 || repairs[0].Kind != types.RepairEmitEvidence {
		t.Fatalf("closure repairs = %+v, want queued emit_evidence repair", repairs)
	}
}

// TestEmitInvestigationComplete_CompletionWithoutAbsenceOnEvidenceAccepted
// — the normal happy path: grounded evidence exists, LLM signals
// completion WITHOUT absence_justification. Must succeed.
func TestEmitInvestigationComplete_ConfigTraceAbsencePrefersMaterializeOverUnreadSibling(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go", "internal/types/context.go"})
	mut.EvidenceClosure().SetReadSet(map[string]bool{"internal/types/config.go": true})
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/types/config.go",
		LineStart:       707,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "DefaultExploreHeuristics",
		ContextRole:     types.EvidenceContextRoleRelatedContext,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{"explore_mid_loop_hint_budget"},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"done","confidence":"high","result_kind":"absence","absence_justification":"no config key named explore_mid_loop_hint_budget exists"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("config-trace absence evidence debt should soft-downgrade, not hard-reject: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatal("config-trace absence evidence debt must not mark completion before convergence")
	}
	if !strings.Contains(res.Summary, "already in read coverage") || !strings.Contains(res.Summary, "Do NOT widen scope") {
		t.Fatalf("rejection should keep the repair on already-read scope before unread siblings, got %q", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != "exact_absence_precedence_evidence" {
		t.Fatalf("repair = %+v, want exact_absence_precedence_evidence", res.Repair)
	}
	if len(res.Repair.Targets) != 1 || res.Repair.Targets[0].File != "internal/types/config.go" || res.Repair.Targets[0].Action != string(types.RepairEmitEvidence) {
		t.Fatalf("repair targets = %+v, want emit_evidence on internal/types/config.go", res.Repair.Targets)
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) == 0 || repairs[0].Kind != types.RepairEmitEvidence || len(repairs[0].Files) != 1 || repairs[0].Files[0] != "internal/types/config.go" {
		t.Fatalf("closure repairs = %+v, want queued emit_evidence on internal/types/config.go", repairs)
	}
}

func TestAllowsContextualEvidenceForAbsence_UsesTypedTargetsNotCompletionProse(t *testing.T) {
	bus := &types.BusContext{
		Mutable: types.NewMutableState("q"),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Scenario: types.ScenarioConfigTrace},
			AnswerContract: types.AnswerContract{ExactResolution: &types.ExactResolutionContract{
				TargetKind:   types.SubjectConfigKey,
				TargetLabel:  "config key",
				Targets:      []string{"existing_key", "missing_key"},
				AllowAbsence: true,
			}},
		},
	}

	if allowsContextualEvidenceForAbsence(bus, []types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/config/runtime.go",
		LineStart:       10,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "existing_key",
		GroundingStatus: types.GroundingGrounded,
	}}) {
		t.Fatal("typed defining proof for any exact target must block absence; completion prose must not choose another target")
	}

	if !allowsContextualEvidenceForAbsence(bus, []types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/config/runtime.go",
		LineStart:       20,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "DefaultExploreHeuristics",
		ContextRole:     types.EvidenceContextRoleRelatedContext,
		GroundingStatus: types.GroundingGrounded,
	}}) {
		t.Fatal("typed related-context evidence should remain compatible with exact absence")
	}
}

func TestEmitInvestigationComplete_ConfigTraceContextOnlyEvidenceRequiresValidatedPrecedenceRole(t *testing.T) {
	missingKey := "explore_mid_loop_hint_budget"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       32,
			Subject:         "RuntimeSettings",
			Object:          missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "RuntimeSettings",
			Summary:         "RuntimeSettings does not define " + missingKey,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       707,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "Grounded same-family defaults context for nearby explore settings.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in production code","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("config-trace context-only evidence debt should soft-downgrade, not hard-reject: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatal("config-trace context-only evidence debt must not mark completion before convergence")
	}
	if !strings.Contains(res.Summary, "precedence-capable lineage anchor") || !strings.Contains(res.Summary, "diagram_role_hint") {
		t.Fatalf("rejection should explain the validated-precedence requirement, got: %s", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != "exact_absence_precedence_anchor" {
		t.Fatalf("repair = %+v, want exact_absence_precedence_anchor", res.Repair)
	}
	if len(res.Repair.Targets) != 1 || res.Repair.Targets[0].File != "internal/types/config.go" {
		t.Fatalf("repair targets = %+v, want internal/types/config.go", res.Repair.Targets)
	}
}

func TestEmitInvestigationComplete_CompletionWithoutAbsenceOnEvidenceAccepted(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: "a.go", LineStart: 10,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "Foo",
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
	}})
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"evidence collected","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("normal completion must succeed: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_CallChainRejectsLatePrincipalSpanGap(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind: types.EvidenceDirect, Source: "internal/agent/analyzer.go", LineStart: 100,
			Subject: "buildAnalysisIR", AnchorKind: types.AnchorDefinition, AnchorSymbol: "buildAnalysisIR",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/agent/analyzer.go", LineStart: 160,
			Subject: "buildAnalysisIR", Object: "normalizer.Normalize", AnchorKind: types.AnchorCall, AnchorSymbol: "normalizer.Normalize",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/agent/analyzer.go", LineStart: 300,
			Subject: "buildAnalysisIR", Object: "gate.RunWith", AnchorKind: types.AnchorCall, AnchorSymbol: "gate.RunWith",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:              "call_chain",
				MentionedEntities: []string{"buildAnalysisIR", "gate.Run", "analyzer.go"},
			},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"source and sink are covered","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("pre-complete downgrades should be tool-success keepalives, got failure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "call-chain principal span") || !strings.Contains(res.Summary, "250-299") {
		t.Fatalf("downgrade should name the missing late span, got %q", res.Summary)
	}
	if got := strings.TrimSpace(mut.InvestigationCompleteReason()); got != "" {
		t.Fatalf("completion flag must not be set on span downgrade, got %q", got)
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) != 1 || repairs[0].Kind != types.RepairReadFile {
		t.Fatalf("repair = %+v, want one read_file repair", repairs)
	}
	if len(repairs[0].LineRanges) != 1 || repairs[0].LineRanges[0] != (types.LineRange{Start: 250, End: 299}) {
		t.Fatalf("repair range = %+v, want 250-299", repairs[0].LineRanges)
	}
}

func TestEmitInvestigationComplete_CallChainAlreadyReadGapAsksForEvidenceMaterialization(t *testing.T) {
	mut := types.NewMutableState("q")
	closure := mut.EvidenceClosure()
	closure.SetReadSet(map[string]bool{"internal/agent/analyzer.go": true})
	closure.SetReadRanges(map[string][]types.LineRange{
		"internal/agent/analyzer.go": {{Start: 1, End: 320}},
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind: types.EvidenceDirect, Source: "internal/agent/analyzer.go", LineStart: 100,
			Subject: "buildAnalysisIR", AnchorKind: types.AnchorDefinition, AnchorSymbol: "buildAnalysisIR",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/agent/analyzer.go", LineStart: 160,
			Subject: "buildAnalysisIR", Object: "normalizer.Normalize", AnchorKind: types.AnchorCall, AnchorSymbol: "normalizer.Normalize",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/agent/analyzer.go", LineStart: 300,
			Subject: "buildAnalysisIR", Object: "gate.RunWith", AnchorKind: types.AnchorCall, AnchorSymbol: "gate.RunWith",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:              "call_chain",
				MentionedEntities: []string{"buildAnalysisIR", "gate.Run"},
			},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"source and sink are covered","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(res.Summary, "already in read coverage") {
		t.Fatalf("downgrade should keep the explorer on already-read evidence materialization, got %q", res.Summary)
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) != 1 || repairs[0].Kind != types.RepairEmitEvidence {
		t.Fatalf("repair = %+v, want one emit_evidence repair", repairs)
	}
}

func TestEmitInvestigationComplete_CallChainAcceptsLatePrincipalEvidence(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind: types.EvidenceDirect, Source: "internal/agent/analyzer.go", LineStart: 100,
			Subject: "buildAnalysisIR", AnchorKind: types.AnchorDefinition, AnchorSymbol: "buildAnalysisIR",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/agent/analyzer.go", LineStart: 160,
			Subject: "buildAnalysisIR", Object: "normalizer.Normalize", AnchorKind: types.AnchorCall, AnchorSymbol: "normalizer.Normalize",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/agent/analyzer.go", LineStart: 260,
			Subject: "buildAnalysisIR", Object: "compiler.Compile", AnchorKind: types.AnchorCall, AnchorSymbol: "compiler.Compile",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/agent/analyzer.go", LineStart: 300,
			Subject: "buildAnalysisIR", Object: "gate.RunWith", AnchorKind: types.AnchorCall, AnchorSymbol: "gate.RunWith",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:              "call_chain",
				MentionedEntities: []string{"buildAnalysisIR", "gate.Run", "analyzer.go"},
			},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"source, interior, and sink are covered","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("completion with late principal evidence should succeed: %s", res.Summary)
	}
	if got := strings.TrimSpace(mut.InvestigationCompleteReason()); got == "" {
		t.Fatalf("completion flag should be set")
	}
}

func TestRelationMemberSetCoverageGaps_UsesEvidenceDrivenRegistrationCarrier(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceRegistration,
		Subject:         "RegisterFeature",
		Object:          "FeatureA",
		Source:          "internal/registry.go",
		LineStart:       42,
		AnchorKind:      types.AnchorAssignment,
		AnchorSymbol:    "FeatureA",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
		Scope:           types.ScopeLine,
		Salience:        types.SalienceExhaustListed,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:        types.IntentEnumerate,
			PredicateAxis: types.AxisRegister,
			Predicates: types.SemanticPredicates{
				IsRelationalLookup:    true,
				IsCategoryEnumeration: true,
			},
			AnalyzerHints: types.AnalyzerHints{
				PrimaryEntities: []string{"FeatureA"},
				Entities:        []string{"FeatureA"},
			},
		}},
	}
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "FeatureA registrations",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"OtherRegistrar"},
	}}
	gaps := relationMemberSetCoverageGaps(bus, facts)
	if len(gaps) != 1 {
		t.Fatalf("gaps = %+v, want one omitted registration member", gaps)
	}
	if gaps[0].Relation != types.TypedRelationRegisters || gaps[0].Name != "RegisterFeature" || gaps[0].File != "internal/registry.go" {
		t.Fatalf("unexpected gap: %+v", gaps[0])
	}
}

func TestRelationMemberSetCoverageGaps_DoesNotForceSupportingRegistrationEvidence(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceRegistration,
		Subject:         "RegisterFeature",
		Object:          "FeatureA",
		Source:          "internal/registry.go",
		LineStart:       42,
		AnchorKind:      types.AnchorAssignment,
		AnchorSymbol:    "FeatureA",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
		Scope:           types.ScopeLine,
		Salience:        types.SalienceSupporting,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:        types.IntentEnumerate,
			PredicateAxis: types.AxisRegister,
			Predicates: types.SemanticPredicates{
				IsRelationalLookup:    true,
				IsCategoryEnumeration: true,
			},
			AnalyzerHints: types.AnalyzerHints{Entities: []string{"FeatureA"}},
		}},
	}
	facts := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "FeatureA registrations",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"OtherRegistrar"},
	}}
	if gaps := relationMemberSetCoverageGaps(bus, facts); len(gaps) != 0 {
		t.Fatalf("supporting evidence must not force a missing relation member: %+v", gaps)
	}
}

func TestGenericForcedReadBoundarySatisfiedByCompleteRelationMemberSet(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceRegistration,
		Subject:         "RegisterFeature",
		Object:          "FeatureA",
		Source:          "internal/registry.go",
		LineStart:       42,
		AnchorKind:      types.AnchorAssignment,
		AnchorSymbol:    "FeatureA",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
		Scope:           types.ScopeLine,
		Salience:        types.SalienceExhaustListed,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:        types.IntentEnumerate,
			PredicateAxis: types.AxisRegister,
			Predicates: types.SemanticPredicates{
				IsRelationalLookup:    true,
				IsCategoryEnumeration: true,
			},
			AnalyzerHints: types.AnalyzerHints{Entities: []string{"FeatureA"}},
		}},
	}
	facts := []types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "FeatureA registrations",
		Role:        types.AnswerAggregateRolePrincipalAnswer,
		Members:     []string{"RegisterFeature"},
		SupportRefs: []string{"RegisterFeature: internal/registry.go:42"},
	}}
	if !genericForcedReadBoundarySatisfied(bus, facts, mut.EmittedEvidence()) {
		t.Fatal("complete typed relation member_set should satisfy generic forced-read boundary")
	}
}

func TestGenericForcedReadBoundarySatisfiedByDiagnosticMechanismGroundedEvidence(t *testing.T) {
	mut := types.NewMutableState("why did finalization retry")
	mut.AppendEvidence([]types.EvidenceItem{
		diagnosticMechanismEvidenceForTest(types.EvidenceMechanism, "internal/llm/openai.go", 374),
		diagnosticMechanismEvidenceForTest(types.EvidenceRelationship, "internal/orchestrator/orchestrator.go", 4612),
		diagnosticMechanismEvidenceForTest(types.EvidenceConditional, "internal/orchestrator/contract_check.go", 72),
	})
	bus := diagnosticMechanismBoundaryBusForTest(mut)
	if !genericForcedReadBoundarySatisfied(bus, nil, mut.EmittedEvidence()) {
		t.Fatal("diagnostic mechanism answer with grounded multi-file current-source evidence should satisfy generic forced-read boundary")
	}
	bus.AnalysisIR.RequestModel.Intent = types.IntentRootCause
	if !genericForcedReadBoundarySatisfied(bus, nil, mut.EmittedEvidence()) {
		t.Fatal("root-cause diagnostic mechanism answer should use the same grounded forced-read boundary")
	}

	mut = types.NewMutableState("why did finalization retry")
	mut.AppendEvidence([]types.EvidenceItem{
		diagnosticMechanismEvidenceForTest(types.EvidenceMechanism, "internal/llm/openai.go", 374),
		diagnosticMechanismEvidenceForTest(types.EvidenceRelationship, "internal/llm/openai.go", 390),
		diagnosticMechanismEvidenceForTest(types.EvidenceConditional, "internal/llm/openai.go", 412),
	})
	bus = diagnosticMechanismBoundaryBusForTest(mut)
	if genericForcedReadBoundarySatisfied(bus, nil, mut.EmittedEvidence()) {
		t.Fatal("single-file diagnostic evidence should not bypass cross-file forced-read coverage")
	}

	mut = types.NewMutableState("list all agents")
	mut.AppendEvidence([]types.EvidenceItem{
		diagnosticMechanismEvidenceForTest(types.EvidenceMechanism, "internal/llm/openai.go", 374),
		diagnosticMechanismEvidenceForTest(types.EvidenceRelationship, "internal/orchestrator/orchestrator.go", 4612),
		diagnosticMechanismEvidenceForTest(types.EvidenceConditional, "internal/orchestrator/contract_check.go", 72),
	})
	bus = diagnosticMechanismBoundaryBusForTest(mut)
	bus.AnalysisIR.RequestModel.Predicates.IsCategoryEnumeration = true
	if genericForcedReadBoundarySatisfied(bus, nil, mut.EmittedEvidence()) {
		t.Fatal("diagnostic evidence shortcut must not weaken exhaustive enumeration obligations")
	}
}

func diagnosticMechanismBoundaryBusForTest(mut *types.MutableState) *types.BusContext {
	return &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentExplain,
				Scenario:      types.ScenarioRootCause,
				PredicateAxis: types.AxisCondition,
				Predicates: types.SemanticPredicates{
					IsDiagnosticQuestion: true,
					IsCrossComponent:     true,
				},
				DiagnosticProfile: types.DiagnosticIntentProfile{
					IsDiagnostic:        true,
					CurrentVersionCheck: true,
				},
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, MinCitations: 3},
			},
		},
	}
}

func diagnosticMechanismEvidenceForTest(kind types.EvidenceKind, source string, line int) types.EvidenceItem {
	return types.EvidenceItem{
		Kind:            kind,
		Subject:         "finalizer observation",
		Object:          "current source mechanism",
		Source:          source,
		LineStart:       line,
		LineEnd:         line,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "boundary",
		Snippet:         "func boundary() {}",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
		Scope:           types.ScopeLine,
		Origin:          types.ClaimOriginCurrentRepo,
		Authority:       types.AuthorityFactual,
	}
}

func TestEmitInvestigationComplete_CallChainRejectsLargeTailGapAfterLateEvidence(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind: types.EvidenceDirect, Source: "internal/agent/analyzer.go", LineStart: 100,
			Subject: "buildAnalysisIR", AnchorKind: types.AnchorDefinition, AnchorSymbol: "buildAnalysisIR",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/agent/analyzer.go", LineStart: 180,
			Subject: "buildAnalysisIR", Object: "normalizer.Normalize", AnchorKind: types.AnchorCall, AnchorSymbol: "normalizer.Normalize",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/agent/analyzer.go", LineStart: 530,
			Subject: "buildAnalysisIR", Object: "compiler.Compile", AnchorKind: types.AnchorCall, AnchorSymbol: "compiler.Compile",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceRelationship, Source: "internal/agent/analyzer.go", LineStart: 660,
			Subject: "buildAnalysisIR", Object: "gate.RunWith", AnchorKind: types.AnchorCall, AnchorSymbol: "gate.RunWith",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:              "call_chain",
				MentionedEntities: []string{"buildAnalysisIR", "gate.Run"},
			},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"source, one late interior, and sink are covered","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("pre-complete downgrades should be tool-success keepalives, got failure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "531-659") {
		t.Fatalf("downgrade should name the large late tail gap, got %q", res.Summary)
	}
	if got := strings.TrimSpace(mut.InvestigationCompleteReason()); got != "" {
		t.Fatalf("completion flag must not be set on tail-gap downgrade, got %q", got)
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) != 1 || repairs[0].Kind != types.RepairReadFile {
		t.Fatalf("repair = %+v, want one read_file repair", repairs)
	}
	if len(repairs[0].LineRanges) != 1 || repairs[0].LineRanges[0] != (types.LineRange{Start: 531, End: 659}) {
		t.Fatalf("repair range = %+v, want 531-659", repairs[0].LineRanges)
	}
}

func TestEmitInvestigationComplete_NormalizesResolvedToAbsenceForExactConfigTraceClosure(t *testing.T) {
	missingKey := "explore_mid_loop_hint_budget"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       32,
			Subject:         "RuntimeSettings",
			Object:          missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "RuntimeSettings",
			Summary:         "RuntimeSettings does not define " + missingKey,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       815,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "Code defaults for the surviving explore heuristics fields.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: missingKey + " 的最终有效值是怎么计算出来的？",
				Scenario:   types.ScenarioConfigTrace,
				AnalyzerHints: types.AnalyzerHints{
					Kind:              "config_mapping",
					PrimaryEntities:   []string{missingKey},
					Entities:          []string{missingKey},
					MentionedEntities: []string{missingKey},
				},
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{missingKey},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key explore_mid_loop_hint_budget in any layer","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected structured exact-absence closure to auto-normalize, got: %s", res.Summary)
	}
	if got := mut.StableInvestigationResultKind(); got != "absence" {
		t.Fatalf("StableInvestigationResultKind = %q, want absence", got)
	}
	if got := mut.StableAbsenceJustification(); got == "" {
		t.Fatal("StableAbsenceJustification = empty, want synthesized justification")
	}
	if !strings.Contains(res.Summary, "result_kind=absence") {
		t.Fatalf("summary should reflect normalized absence result, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_SynthesizesAbsenceForRelatedContextOnlyExactClosure(t *testing.T) {
	missingKey := "explore_mid_loop_hint_budget"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       815,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "Code defaults for the surviving explore heuristics fields.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: missingKey + " 的最终有效值是怎么计算出来的？",
				Scenario:   types.ScenarioConfigTrace,
				AnalyzerHints: types.AnalyzerHints{
					Kind:              "config_mapping",
					PrimaryEntities:   []string{missingKey},
					Entities:          []string{missingKey},
					MentionedEntities: []string{missingKey},
					ExactTargets:      []string{missingKey},
				},
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{missingKey},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
					RelatedContextTerms:  []string{"explore"},
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key explore_mid_loop_hint_budget in any layer","confidence":"high","result_kind":"absence"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected exact-absence closure with validated related context to auto-synthesize justification, got: %s", res.Summary)
	}
	if got := mut.StableInvestigationResultKind(); got != "absence" {
		t.Fatalf("StableInvestigationResultKind = %q, want absence", got)
	}
	if got := mut.StableAbsenceJustification(); got == "" {
		t.Fatal("StableAbsenceJustification = empty, want synthesized justification")
	}
	if !strings.Contains(res.Summary, "result_kind=absence") {
		t.Fatalf("summary should reflect synthesized absence result, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_AbsenceAllowsProductionNegativeSummaryMentions(t *testing.T) {
	missingKey := "explore_mid_loop_hint_budget"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       839,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "DefaultExploreHeuristics defines the surviving defaults, and there is no explore_mid_loop_hint_budget field here.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: missingKey + " 的最终有效值是怎么计算出来的？",
				Scenario:   types.ScenarioConfigTrace,
				AnalyzerHints: types.AnalyzerHints{
					Kind:              "config_mapping",
					PrimaryEntities:   []string{missingKey},
					Entities:          []string{missingKey},
					MentionedEntities: []string{missingKey},
					ExactTargets:      []string{missingKey},
				},
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{missingKey},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
					RelatedContextTerms:  []string{"explore"},
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key explore_mid_loop_hint_budget in any layer","confidence":"high","result_kind":"absence","absence_justification":"the exact config key explore_mid_loop_hint_budget is absent from the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("negative summary mentions should not be upgraded into defining exact proof, got: %s", res.Summary)
	}
	if got := mut.StableInvestigationResultKind(); got != "absence" {
		t.Fatalf("StableInvestigationResultKind = %q, want absence", got)
	}
}

func TestEmitInvestigationComplete_PreservesPriorAbsenceOverResolvedRetryWithoutNewProof(t *testing.T) {
	missingKey := "explore_mid_loop_hint_budget"
	mut := types.NewMutableState("q")
	mut.SetAbsenceJustification("the exact config key explore_mid_loop_hint_budget is absent from the repo")
	mut.SetInvestigationResultKind("absence")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       839,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "DefaultExploreHeuristics defines the surviving defaults, and there is no explore_mid_loop_hint_budget field here.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: missingKey + " 的最终有效值是怎么计算出来的？",
				Scenario:   types.ScenarioConfigTrace,
				AnalyzerHints: types.AnalyzerHints{
					Kind:              "config_mapping",
					PrimaryEntities:   []string{missingKey},
					Entities:          []string{missingKey},
					MentionedEntities: []string{missingKey},
					ExactTargets:      []string{missingKey},
				},
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{missingKey},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
					RelatedContextTerms:  []string{"explore"},
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{"reason":"the three-layer precedence chain is understood and the key is still absent","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("resolved retry without new exact proof should preserve prior absence closure, got: %s", res.Summary)
	}
	if got := mut.StableInvestigationResultKind(); got != "absence" {
		t.Fatalf("StableInvestigationResultKind = %q, want absence", got)
	}
	if !strings.Contains(res.Summary, "result_kind=absence") {
		t.Fatalf("summary should reflect preserved absence result, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_DowngradesResolvedExactClosureWithoutDefiningProof(t *testing.T) {
	missingKey := "explore_mid_loop_hint_budget"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go", "cmd/root.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       848,
			Subject:         "DefaultExploreHeuristics",
			Object:          "16 hard-coded defaults; " + missingKey + " is not one of them",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       768,
			Subject:         "ExploreHeuristics",
			Object:          missingKey,
			Predicate:       "absent_from",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "ExploreHeuristics",
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: missingKey + " 的最终有效值是怎么计算出来的？",
				Scenario:   types.ScenarioConfigTrace,
				AnalyzerHints: types.AnalyzerHints{
					Kind:              "config_mapping",
					PrimaryEntities:   []string{missingKey},
					Entities:          []string{missingKey},
					MentionedEntities: []string{missingKey},
					ExactTargets:      []string{missingKey},
				},
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{missingKey},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
					RelatedContextTerms:  []string{"explore"},
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{"reason":"the general precedence mechanism is understood and the key still does not exist","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("resolved completion without defining proof should downgrade instead of tool-rejecting: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("downgraded resolved exact completion must not mark investigation complete")
	}
	if !strings.Contains(res.Summary, "still has no grounded defining proof") || !strings.Contains(res.Summary, "result_kind=\"absence\"") {
		t.Fatalf("downgrade should steer either toward real defining proof or exact absence closure, got: %s", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != "exact_resolved_defining_proof" {
		t.Fatalf("downgrade should carry exact resolved defining-proof repair, got %+v", res.Repair)
	}
	if repairs := mut.EvidenceClosure().ActiveRepairs(); len(repairs) != 1 || repairs[0].Kind != types.RepairEmitEvidence {
		t.Fatalf("downgrade should record typed repair directive, got %+v", repairs)
	}
}

func TestEmitInvestigationComplete_ResolvedExactDefiningProofForceCompletesWithCaveatAfterNoProgress(t *testing.T) {
	SetDowngradeConvergenceHardThreshold(3)
	t.Cleanup(func() { SetDowngradeConvergenceHardThreshold(0) })
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/types/config.go",
		LineStart:       848,
		Subject:         "DefaultExploreHeuristics",
		Object:          "missing_key is not present in defaults",
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "DefaultExploreHeuristics",
		ContextRole:     types.EvidenceContextRoleRelatedContext,
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
				AnalyzerHints: types.AnalyzerHints{
					ExactTargets: []string{"missing_key"},
				},
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:   types.SubjectConfigKey,
					TargetLabel:  "config key",
					Targets:      []string{"missing_key"},
					AllowAbsence: true,
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{"reason":"the general mechanism is understood","confidence":"high","result_kind":"resolved"}`)

	for i := 1; i <= 2; i++ {
		res, _ := tool.Execute(bus, params)
		if !res.Success {
			t.Fatalf("attempt %d should downgrade without tool failure: %s", i, res.Summary)
		}
		if mut.IsInvestigationComplete() {
			t.Fatalf("attempt %d should not complete before convergence threshold", i)
		}
	}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("threshold attempt should complete with caveat, got failure: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("threshold attempt should force-complete with typed caveat")
	}
	caveats := mut.EvidenceClosure().CompletionCaveats()
	if len(caveats) != 1 || caveats[0].Lane != types.DowngradeLaneExactResolvedDefiningProof {
		t.Fatalf("completion caveats = %+v, want exact_resolved_defining_proof", caveats)
	}
}

// TestEmitInvestigationComplete_Tier1FloorDowngradesPureRecovery pins the
// session-8 upstream-intercept plus the 2026-06-23 hard-gate discipline: when
// every item is Recovered (the LLM never read_file'd any of the cited sources),
// the Tier-1 floor delays completion and names the read_file repair without
// surfacing a tool error. The mutable completion flag remains false, so the
// scheduler keeps exploring.
// Matches the trace 1776444788929246456 failure mode where the
// finalizer dropped all 4 citations because none were read-file proven.
func TestEmitInvestigationComplete_Tier1FloorDowngradesPureRecovery(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0.5, Tier1Floor: 0.3})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("q")
	closure := mut.EvidenceClosure()
	// 3 recovered items — grounded+recovered ratio = 100% (passes
	// GroundingFloor) but Tier-1 ratio = 0% (fails Tier1Floor).
	for i := 0; i < 3; i++ {
		mut.AppendEvidence([]types.EvidenceItem{{
			Kind: types.EvidenceDirect, Source: "a.go", LineStart: 10 + i,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "Foo",
			GroundingStatus: types.GroundingRecovered,
			GroundingTier:   types.TierFQNameSameFile,
		}})
	}
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"done","confidence":"high","result_kind":"resolved"}`)
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("pure-recovery investigation should be downgraded, not tool-rejected: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("downgraded tier1-floor completion must not flip investigationComplete")
	}
	if !strings.Contains(res.Summary, "line-text-grounded ratio") {
		t.Errorf("downgrade must name the line-text grounding gate: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "read_file") {
		t.Errorf("downgrade must suggest read_file repair: %q", res.Summary)
	}
	if res.Repair == nil || res.Repair.Code != "tier1_citation_floor" {
		t.Fatalf("downgrade should carry typed repair metadata, got %+v", res.Repair)
	}
	if repairs := closure.PendingRepairs(); len(repairs) != 1 {
		t.Fatalf("expected one deduped RepairReadFile directive, got %d: %+v", len(repairs), repairs)
	} else if repairs[0].Kind != types.RepairReadFile || len(repairs[0].Files) != 1 || repairs[0].Files[0] != "a.go" {
		t.Fatalf("unexpected repair payload: %+v", repairs[0])
	}
	if pending := closure.PendingReads(); len(pending) != 1 {
		t.Fatalf("expected one mirrored PendingRead, got %d: %+v", len(pending), pending)
	} else if pending[0].File != "a.go" {
		t.Fatalf("pending read file = %q, want a.go", pending[0].File)
	}
	if !strings.Contains(res.Summary, "10, 11, 12") {
		t.Errorf("summary should collapse same-file line hints, got: %q", res.Summary)
	}
}

func TestEmitInvestigationComplete_Tier1FloorForceCompletesWithCaveatAfterNoProgress(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0.5, Tier1Floor: 0.3})
	t.Cleanup(func() { SetGroundingPolicy(prev) })
	SetDowngradeConvergenceHardThreshold(3)
	t.Cleanup(func() { SetDowngradeConvergenceHardThreshold(0) })

	mut := types.NewMutableState("q")
	for i := 0; i < 2; i++ {
		mut.AppendEvidence([]types.EvidenceItem{{
			Kind: types.EvidenceDirect, Source: "a.go", LineStart: 10 + i,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "Foo",
			GroundingStatus: types.GroundingRecovered,
			GroundingTier:   types.TierFQNameSameFile,
		}})
	}
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{"reason":"done","confidence":"high","result_kind":"resolved"}`)

	for i := 1; i <= 2; i++ {
		res, _ := tool.Execute(bus, params)
		if !res.Success {
			t.Fatalf("attempt %d should downgrade without tool failure: %s", i, res.Summary)
		}
		if mut.IsInvestigationComplete() {
			t.Fatalf("attempt %d should not complete before convergence threshold", i)
		}
	}
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("threshold attempt should complete with caveat, got failure: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("threshold attempt should force-complete with typed caveat")
	}
	caveats := mut.EvidenceClosure().CompletionCaveats()
	if len(caveats) != 1 || caveats[0].Lane != types.DowngradeLaneGroundingCitationFloor {
		t.Fatalf("completion caveats = %+v, want grounding_citation_floor", caveats)
	}
}

// TestEmitInvestigationComplete_Tier1FloorAcceptsMixed — 30% Tier-1
// threshold met by 1 Tier-1 + 2 Recovered items (1/3 = 33%). Gate
// passes, completion accepted.
func TestEmitInvestigationComplete_Tier1FloorAcceptsMixed(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0.5, Tier1Floor: 0.3})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind: types.EvidenceDirect, Source: "a.go", LineStart: 10,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "Foo",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceDirect, Source: "b.go", LineStart: 20,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "Bar",
			GroundingStatus: types.GroundingRecovered, GroundingTier: types.TierFQNameSameFile,
		},
		{
			Kind: types.EvidenceDirect, Source: "c.go", LineStart: 30,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "Baz",
			GroundingStatus: types.GroundingRecovered, GroundingTier: types.TierPackageSymbol,
		},
	})
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"done","confidence":"high","result_kind":"resolved"}`)
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("33%% Tier-1 ratio must pass 30%% floor, got rejection: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_DowngradesWhenPrincipalSupportLaneMissing(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("Which agents can call subagents?")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "context-guard",
		Kind:            types.EvidenceConditional,
		Scope:           types.ScopeLine,
		Source:          "internal/agent/subagent_runtime.go",
		LineStart:       42,
		AnchorKind:      types.AnchorCondition,
		AnchorSymbol:    "Validate",
		Subject:         "SubAgentRuntime",
		Summary:         "SubAgentRuntime validates proposals before execution",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable:    mut,
		AnalysisIR: enumerationPrincipalGateIR(),
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"found enough context","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("principal support downgrade is a soft pre-complete result, got hard failure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "typed principal evidence handoff is missing") {
		t.Fatalf("summary should explain missing principal handoff, got: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("downgraded completion must not mark investigation complete")
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) == 0 || repairs[len(repairs)-1].Kind != types.RepairEmitEvidence {
		t.Fatalf("expected RepairEmitEvidence queued, got %+v", repairs)
	}
}

func TestEmitInvestigationComplete_AllowsFacetBoundPrincipalSupport(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("Which agents can call subagents?")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "agent-explorer",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/enums.go",
		LineStart:       117,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "AgentExplorer",
		Subject:         "AgentExplorer",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable:    mut,
		AnalysisIR: enumerationPrincipalGateIR(),
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"principal member is grounded","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("facet-bound principal support should pass: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("completion should be stored after principal support passes")
	}
}

func TestEmitInvestigationComplete_DowngradesExhaustiveEnumerationWithoutMemberSet(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all public enum types")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "intent",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       642,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Intent",
		Subject:         "Intent",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all public enum types",
	}
	bus := &types.BusContext{
		Mutable:    mut,
		AnalysisIR: ir,
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"the command output listed all members","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("exhaustive member-set downgrade is a soft pre-complete result, got hard failure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "exhaustive member-set handoff is missing") {
		t.Fatalf("summary should ask for structured member_set handoff, got: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("downgraded completion must not mark investigation complete")
	}
}

func TestEmitInvestigationComplete_DowngradesDeclaredEnumerationWithoutMemberSet(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List the three public enum types")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "intent",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       642,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Intent",
		Subject:         "Intent",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.AnalyzerHints = types.AnalyzerHints{
		Kind:     string(types.ReqEnumeration),
		Entities: []string{"Intent", "Scenario", "Complexity"},
	}
	ir.RequestModel.EnumerationBoundary = &types.RequestedEnumerationBoundary{
		DeclaredCount: 3,
		SourceQuote:   "three public enum types",
	}
	bus := &types.BusContext{
		Mutable:    mut,
		AnalysisIR: ir,
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"the command output listed all members","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("exhaustive member-set downgrade is a soft pre-complete result, got hard failure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "exhaustive member-set handoff is missing") {
		t.Fatalf("summary should ask for structured member_set handoff, got: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("downgraded completion must not mark investigation complete")
	}
}

func TestExhaustiveEnumerationMemberSetDowngrade_SourceInventoryUsesStructuredHandoffRepair(t *testing.T) {
	mut := types.NewMutableState("仓库里有哪些 extend 块、哪些 foreign func 声明、哪些 public class？")
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.AnalyzerHints = types.AnalyzerHints{
		Kind:     string(types.ReqEnumeration),
		Entities: []string{"extend 块", "foreign func 声明", "public class"},
	}
	ir.RequestModel.SourceInventoryProfile = &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleType, types.AnswerCandidateRoleFunction},
		SourceQuotes:      []string{"extend 块", "foreign func 声明", "public class"},
		Confidence:        0.9,
	}
	ir.RequestModel.SourceScopeProfile = &types.SourceScopeProfile{RequestedScope: types.SourceScopeAll}
	bus := &types.BusContext{
		Mutable:    mut,
		AnalysisIR: ir,
	}

	summary := exhaustiveEnumerationMemberSetDowngrade(bus, mut.EvidenceClosure(), nil)
	if !strings.Contains(summary, "source-inventory") {
		t.Fatalf("expected source-inventory-specific structured handoff guidance, got: %s", summary)
	}
	for _, notWant := range []string{"emit_answer_symbol", "emit_evidence"} {
		if strings.Contains(summary, notWant) {
			t.Fatalf("source-inventory completion-form debt must not ask for unavailable %s: %s", notWant, summary)
		}
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) == 0 {
		t.Fatalf("expected structured handoff repair directive")
	}
	last := repairs[len(repairs)-1]
	if last.Kind != types.RepairStructuredHandoff ||
		last.Origin != types.RepairOriginCompletionFormPrefix+"source_inventory_member_set" {
		t.Fatalf("expected completion-form structured_handoff repair, got %+v", last)
	}
	if tools := types.RepairDirectiveRequiredTools(last); len(tools) != 0 {
		t.Fatalf("completion-form source-inventory repair must not require extra tools, got %+v", tools)
	}
}

func TestEmitInvestigationComplete_AllowsExhaustiveEnumerationMemberSet(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all public enum types")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "intent",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       642,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Intent",
		Subject:         "Intent",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all public enum types",
	}
	bus := &types.BusContext{
		Mutable:    mut,
		AnalysisIR: ir,
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"member set carried through aggregate_facts",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"public enum types",
			"value":"1",
			"unit":"types",
			"members":["Intent"],
			"support_refs":["internal/types/analysis_ir.go:642"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("member_set aggregate should satisfy exhaustive handoff: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("completion should be stored after member_set aggregate passes")
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 || facts[0].Kind != types.AnswerAggregateMemberSet || len(facts[0].Members) != 1 {
		t.Fatalf("member_set aggregate should be retained, got %+v", facts)
	}
}

func TestEmitInvestigationComplete_AutoFillsBareMemberSupportFromUniqueReadLine(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("Which subagent is registered by default?")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "subexplorer-name",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/agent/sub_explorer.go",
		LineStart:       32,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Name",
		Subject:         "Name",
		Snippet:         "func (s *SubExplorer) Name() string {",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	mut.AppendDispatchToolResult(readFileRawRefResultForTest(
		t,
		"internal/agent/sub_explorer.go",
		32,
		34,
		"func (s *SubExplorer) Name() string {",
		"\treturn \"explorer\"",
		"}",
	))
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "default subagent names",
	}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"default subagent registry resolves to the explorer subagent",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"default subagent names",
			"value":"1",
			"unit":"subagents",
			"role":"principal_answer",
			"members":["explorer"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("unique read_file member line should auto-fill support: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 || len(facts[0].SupportRefs) != 1 {
		t.Fatalf("expected enriched support refs, got %+v; summary: %s", facts, res.Summary)
	}
	if got := facts[0].SupportRefs[0]; got != "internal/agent/sub_explorer.go:33" {
		t.Fatalf("auto-filled support ref should be positional and member-aligned, got %+v", facts[0].SupportRefs)
	}
}

func TestEmitInvestigationComplete_DoesNotVerifyMemberSupportFromNonReadFileSummary(t *testing.T) {
	mut := types.NewMutableState("Which gates run?")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary:  "src/render/handler.go:42: func (h *RenderHandler) processFrame() error { return nil }\n",
	})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "render handlers",
	}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"gate list collected",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"gates",
			"value":"1",
			"members":["RenderHandler.processFrame (current implementation)"],
			"support_refs":["RenderHandler.processFrame: src/render/handler.go:42"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("summary-only support debt should downgrade, not tool-fail: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("grep summary text must not verify decorated member support_refs")
	}
	if !strings.Contains(res.Summary, EmitInvestigationCompleteDowngradePrefix) {
		t.Fatalf("expected a typed pre-complete downgrade, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_DoesNotVerifyMemberSupportFromEvidenceSummary(t *testing.T) {
	mut := types.NewMutableState("Which handlers run?")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "handler-return",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "src/render/handler.go",
		LineStart:       42,
		AnchorKind:      types.AnchorReturn,
		AnchorSymbol:    "returnValue",
		Subject:         "returnValue",
		Snippet:         "return h.delegate(ctx)",
		Summary:         "RenderHandler.processFrame returns through delegate",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "render handlers",
	}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"handler list collected",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"handlers",
			"value":"1",
			"members":["RenderHandler.processFrame (current implementation)"],
			"support_refs":["RenderHandler.processFrame: src/render/handler.go:42"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("summary-only support debt should downgrade, not tool-fail: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("ordinary evidence summary must not verify decorated member support_refs; summary=%s facts=%+v", res.Summary, mut.StableInvestigationAggregateFacts())
	}
	if !strings.Contains(res.Summary, EmitInvestigationCompleteDowngradePrefix) {
		t.Fatalf("expected a typed pre-complete downgrade, got: %s", res.Summary)
	}
}

func TestAggregateEvidenceMemberSupportText_OnlyLoadBearingSummaryCarriesSummary(t *testing.T) {
	ev := types.EvidenceItem{
		AnchorKind:      types.AnchorReturn,
		Snippet:         "return value",
		Summary:         "opaque runtime scalar abc123",
		GroundingStatus: types.GroundingGrounded,
	}
	if got := aggregateEvidenceMemberSupportText(ev); strings.Contains(got, ev.Summary) {
		t.Fatalf("ordinary summaries must not enter member support text, got %q", got)
	}
	ev.LoadBearingSummary = true
	if got := aggregateEvidenceMemberSupportText(ev); !strings.Contains(got, ev.Summary) {
		t.Fatalf("load-bearing summaries should remain explicit support carriers, got %q", got)
	}
}

func TestEmitInvestigationComplete_AutoFillsMemberSupportFromUniqueGroundedEvidence(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List the read-mode agents")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			ID:              "agent-analyzer",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/types/enums.go",
			LineStart:       130,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "AgentAnalyzer",
			Subject:         "AgentAnalyzer",
			Producer:        "explorer.emit_evidence",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			ID:              "agent-explorer",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/types/enums.go",
			LineStart:       131,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "AgentExplorer",
			Subject:         "AgentExplorer",
			Producer:        "explorer.emit_evidence",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "read-mode agents",
	}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"read-mode principal agents are grounded by AgentName evidence",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"read-mode agents",
			"value":"2",
			"unit":"agents",
			"role":"principal_answer",
			"members":["AgentAnalyzer","AgentExplorer"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("unique grounded evidence should auto-fill support: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 {
		t.Fatalf("expected one retained fact, got %+v", facts)
	}
	want := []string{"internal/types/enums.go:130", "internal/types/enums.go:131"}
	if !reflect.DeepEqual(facts[0].SupportRefs, want) {
		t.Fatalf("support_refs should be precise positional evidence refs, got %+v want %+v", facts[0].SupportRefs, want)
	}
}

func TestEmitInvestigationComplete_RepairsDecoratedMemberSupportRefFromUniqueGroundedEvidence(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List decorator declarations")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "arkts-extend-highlight",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets",
		LineStart:       11,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "highlight",
		Subject:         "@Extend decorator",
		Snippet:         "@Extend(Text) function highlight(color: string) {",
		SurfaceTerms:    []string{"@Extend(Text)"},
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"decorator declaration is fully grounded",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"extend decorators",
			"value":"1",
			"role":"principal_answer",
			"members":["@Extend(Text) @ 04_styles_extend.ets:11 (ArkTS decorator)"],
			"support_refs":["04_styles_extend.ets:11"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("unique grounded decorated member support_ref should be repaired locally: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 {
		t.Fatalf("stable aggregate facts = %+v, want one repaired member_set; summary=%s complete=%v reason=%q",
			facts, res.Summary, mut.IsInvestigationComplete(), mut.InvestigationCompleteReason())
	}
	want := "internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets:11"
	if len(facts[0].SupportRefs) != 1 || facts[0].SupportRefs[0] != want {
		t.Fatalf("support_refs = %+v, want %q", facts[0].SupportRefs, want)
	}
}

func TestEnrichCompletionAggregateFactsWithMemberSupport_SkipsAmbiguousGroundedEvidence(t *testing.T) {
	mut := types.NewMutableState("List the matching agents")
	evidence := []types.EvidenceItem{
		{
			ID:              "agent-planner-a",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/types/enums.go",
			LineStart:       155,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "AgentPlanner",
			Subject:         "AgentPlanner",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			ID:              "agent-planner-b",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/agent/planner.go",
			LineStart:       17,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "AgentPlanner",
			Subject:         "AgentPlanner",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	}
	mut.AppendEvidence(evidence)
	bus := &types.BusContext{Mutable: mut}
	got := enrichCompletionAggregateFactsWithMemberSupportWithEvidence(bus, []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "matching agents",
		Value:   "1",
		Members: []string{"AgentPlanner"},
	}}, evidence)
	if len(got) != 1 {
		t.Fatalf("expected one fact, got %+v", got)
	}
	if len(got[0].SupportRefs) != 0 {
		t.Fatalf("ambiguous evidence must not be auto-filled, got %+v", got[0].SupportRefs)
	}
}

func TestEmitInvestigationComplete_DoesNotRepairAmbiguousDecoratedMemberSupportRefs(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List decorator declarations")
	for _, source := range []string{
		"internal/thirdparty/tree-sitter-arkts/corpus/sources/04_styles_extend.ets",
		"examples/arkts/04_styles_extend.ets",
	} {
		mut.AppendEvidence([]types.EvidenceItem{{
			ID:              source,
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          source,
			LineStart:       11,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "highlight",
			Subject:         "@Extend decorator",
			Snippet:         "@Extend(Text) function highlight(color: string) {",
			SurfaceTerms:    []string{"@Extend(Text)"},
			Producer:        "explorer.emit_evidence",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		}})
	}
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"decorator declarations collected",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"extend decorators",
			"value":"1",
			"role":"principal_answer",
			"members":["@Extend(Text) @ 04_styles_extend.ets:11 (ArkTS decorator)"],
			"support_refs":["04_styles_extend.ets:11"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ambiguous support debt should downgrade instead of tool-failing: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("ambiguous decorated member support_refs must not be repaired into accepted completion")
	}
	if !strings.Contains(res.Summary, EmitInvestigationCompleteDowngradePrefix) ||
		!strings.Contains(res.Summary, "support_refs") {
		t.Fatalf("expected bounded completion-form downgrade, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_DoesNotAutoFillBareMemberSupportFromAmbiguousReadLines(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("Which subagent is registered by default?")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "subexplorer-name",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/agent/sub_explorer.go",
		LineStart:       32,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Name",
		Subject:         "Name",
		Snippet:         "func (s *SubExplorer) Name() string {",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		Summary: "[internal/agent/sub_explorer.go: showing lines 33-35]\n" +
			"   33 │ \treturn \"explorer\"\n" +
			"   34 │ \tname := \"explorer\"\n" +
			"   35 │ \t_ = name\n",
	})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "default subagent names",
	}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"default subagent registry resolves to the explorer subagent",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"default subagent names",
			"value":"1",
			"unit":"subagents",
			"role":"principal_answer",
			"members":["explorer"],
			"support_refs":["Name: internal/agent/sub_explorer.go:32"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("ambiguous support should be a soft downgrade, not hard failure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "typed evidence") {
		t.Fatalf("summary should ask for typed support after ambiguous read lines, got: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("ambiguous support must not close investigation")
	}
}

func TestEmitInvestigationComplete_AllowsExhaustiveRuntimeArtifactMemberSet(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all trace resource and plugin observations")
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all trace resource and plugin observations",
	}
	ir.RequestModel.ExternalObservationPolicy = &types.ExternalObservationPolicy{
		ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
		CurrentSourceMode:    types.ExternalObservationCurrentSourceDefault,
		Confidence:           0.9,
	}
	bus := &types.BusContext{
		Mutable:         mut,
		AnalysisIR:      ir,
		AttachedHitrace: "8.000000: bio_latency: op=R path=/data/app/base.db latency_us=2500 bytes=4096",
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"trace_query window_stats enumerated the external artifact rows",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{
			"reason":"external_only_trace",
			"rationale":"the principal members are artifact-local trace rows, not current-repo source lines"
		},
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"trace observations",
			"value":"6",
			"role":"principal_answer",
			"provenance":"trace_query.window_stats",
			"dimensions":[
				{"name":"origin","value":"runtime_artifact"},
				{"name":"artifact_id","value":"attached_trace"},
				{"name":"artifact_kind","value":"trace"}
			],
			"members":[
				"bio_latency @ line 3",
				"file_system @ line 4",
				"page_fault_user @ line 5",
				"ability_monitor @ line 6",
				"xpower_cpu @ line 7",
				"hi_sysevent @ line 8"
			]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("runtime artifact member_set should close without repo support_refs: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("completion should be stored after origin-specific runtime member_set passes")
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 || facts[0].Kind != types.AnswerAggregateMemberSet || len(facts[0].Members) != 6 {
		t.Fatalf("runtime member_set aggregate should be retained, got %+v", facts)
	}
}

func TestEmitInvestigationComplete_SameCallTraceWaiverBypassesDecoratedRuntimeSupportRefs(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("Analyze the attached trace bottleneck")
	bus := &types.BusContext{
		Mutable:         mut,
		AnalysisIR:      &types.AnalysisIR{RequestModel: types.RequestModel{}},
		AttachedHitrace: "173971.000000: tracing_mark_write: OHOS::AppExecFwk::MainThread blocked",
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"the attached trace artifact identifies the runtime stack segment",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{
			"reason":"external_only_trace",
			"rationale":"the member labels are runtime artifact frames and not current checkout source symbols"
		},
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"runtime stack segment",
			"value":"1",
			"role":"principal_answer",
			"members":["OHOS::AppExecFwk::MainThread (应用框架)"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("same-call trace waiver should not be hard-rejected: %s", res.Summary)
	}
	if strings.Contains(res.Summary, "support_refs") {
		t.Fatalf("runtime artifact waiver should prevent support_refs repair loop, got: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("completion should be stored after same-call trace waiver")
	}
}

func TestEmitInvestigationComplete_DoesNotBypassMixedOriginExhaustiveMemberSet(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all trace rows and current source anchors")
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all trace rows and current source anchors",
	}
	ir.RequestModel.ExternalObservationPolicy = &types.ExternalObservationPolicy{
		ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
		CurrentSourceMode:    types.ExternalObservationCurrentSourceDefault,
		Confidence:           0.9,
	}
	bus := &types.BusContext{
		Mutable:         mut,
		AnalysisIR:      ir,
		AttachedHitrace: "8.000000: bio_latency: op=R path=/data/app/base.db latency_us=2500 bytes=4096",
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"mixed-origin rows are not fully repo-grounded",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{
			"reason":"external_only_trace",
			"rationale":"the trace members are external but the fact also declares current-source origin"
		},
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"mixed trace and source observations",
			"value":"1",
			"role":"principal_answer",
			"dimensions":[
				{"name":"origin","value":"runtime_artifact"},
				{"name":"secondary_origin","value":"current_source"}
			],
			"members":["bio_latency @ line 3"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("mixed-origin member_set downgrade should be soft, got hard failure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "member_set") || !strings.Contains(res.Summary, "no typed evidence") {
		t.Fatalf("summary should preserve member support requirement for mixed origins, got: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("mixed-origin ungrounded member_set must not mark investigation complete")
	}
}

func TestEmitInvestigationComplete_OriginSpecificMemberSetHintAvoidsRepoEvidenceRepair(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all external trace observations")
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all external trace observations",
	}
	ir.RequestModel.ExternalObservationPolicy = &types.ExternalObservationPolicy{
		ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
		CurrentSourceMode:    types.ExternalObservationCurrentSourceDefault,
		Confidence:           0.9,
	}
	bus := &types.BusContext{
		Mutable:         mut,
		AnalysisIR:      ir,
		AttachedHitrace: "8.000000: bio_latency: op=R path=/data/app/base.db latency_us=2500 bytes=4096",
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"trace rows are known but the handoff role is wrong",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{
			"reason":"external_only_trace",
			"rationale":"trace rows are artifact-local"
		},
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"trace observations",
			"value":"1",
			"role":"supporting_coverage",
			"provenance":"trace_query.window_stats",
			"dimensions":[{"name":"origin","value":"runtime_artifact"}],
			"members":["bio_latency @ line 3"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("origin-specific handoff downgrade should be soft, got hard failure: %s", res.Summary)
	}
	for _, want := range []string{
		"origin-specific external-observation `member_set`",
		"role=`principal_answer`",
		"Do not call `emit_evidence`",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Fatalf("summary should include origin-specific repair guidance %q, got: %s", want, res.Summary)
		}
	}
	if strings.Contains(res.Summary, "For code symbols, paths, routes") {
		t.Fatalf("origin-specific hint must not render source-code support_refs guidance: %s", res.Summary)
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) == 0 {
		t.Fatalf("expected structured handoff repair directive")
	}
	last := repairs[len(repairs)-1]
	if last.Kind != types.RepairStructuredHandoff {
		t.Fatalf("expected structured_handoff repair, got %+v", last)
	}
	if last.Origin != "pre_complete.exhaustive_member_set.origin_specific" {
		t.Fatalf("unexpected repair origin: %+v", last)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("structurally invalid origin-specific member_set must not mark investigation complete")
	}
}

func TestEmitInvestigationComplete_DowngradesPrincipalGroupedCountWithoutMembersUnderExhaustiveEnumeration(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all public symbols by category")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "intent",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       642,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Intent",
		Subject:         "Intent",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all public symbols by category",
	}
	bus := &types.BusContext{
		Mutable:    mut,
		AnalysisIR: ir,
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"one category has a complete member_set, another category only has a count",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"public enum types",
			"value":"1",
			"unit":"types",
			"members":["Intent"],
			"support_refs":["internal/types/analysis_ir.go:642"]
		},{
			"kind":"grouped_count",
			"label":"public functions",
			"value":"2",
			"unit":"functions"
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("grouped-count member gap is a soft pre-complete result, got hard failure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "exhaustive member-set handoff is missing") ||
		!strings.Contains(res.Summary, "grouped_count") ||
		!strings.Contains(res.Summary, "public functions") {
		t.Fatalf("summary should ask for members on positive principal grouped_count, got: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("downgraded completion must not mark investigation complete")
	}
}

func TestEmitInvestigationComplete_AllowsSingletonCountBasisWhenMemberSetExists(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all public symbols by category and explain count basis")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "intent",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       642,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Intent",
		Subject:         "Intent",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all public symbols by category",
	}
	bus := &types.BusContext{
		Mutable:    mut,
		AnalysisIR: ir,
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"one complete member_set plus one singleton count-basis scalar",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"public enum types",
			"value":"1",
			"unit":"types",
			"members":["Intent"],
			"support_refs":["internal/types/analysis_ir.go:642"]
		},{
			"kind":"grouped_count",
			"label":"declaration block count basis",
			"value":"1",
			"unit":"blocks"
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("singleton count-basis metadata should not force synthetic members: %s", res.Summary)
	}
	if strings.Contains(res.Summary, "grouped_count") && strings.Contains(res.Summary, "has no members") {
		t.Fatalf("summary should not ask for synthetic singleton count-basis members: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("completion should be stored when real member_set exists and singleton count is metadata")
	}
}

func TestEmitInvestigationComplete_SupportingCoverageMemberSetDoesNotSatisfyExhaustiveEnumeration(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all public enum types")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "intent",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       642,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Intent",
		Subject:         "Intent",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all public enum types",
	}
	bus := &types.BusContext{
		Mutable:    mut,
		AnalysisIR: ir,
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"member set is only an investigation coverage ledger",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"files inspected for enum evidence",
			"value":"1",
			"role":"supporting_coverage",
			"provenance":"command:rg",
			"members":["Intent"],
			"support_refs":["internal/types/analysis_ir.go:642"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("role mismatch should downgrade softly, got hard failure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "exhaustive member-set handoff is missing") ||
		!strings.Contains(res.Summary, "supporting_coverage") {
		t.Fatalf("supporting coverage member_set must not satisfy principal handoff, got: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("downgraded completion must not mark investigation complete")
	}
}

func TestMergeCompletionAggregateFacts_DedupesEquivalentMemberSetRetries(t *testing.T) {
	current := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "all subpackages and entry functions",
		Value:   "2",
		Members: []string{"aggregator → Aggregate", "compiler → Compile"},
	}}
	stable := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "subpackage directory and entry function",
		Value:   "2",
		Members: []string{"aggregator/Aggregate", "compiler/Compile"},
	}}

	got := mergeCompletionAggregateFacts(current, stable)
	if len(got) != 1 {
		t.Fatalf("equivalent retry member_set facts should merge to one authoritative set, got %+v", got)
	}
	if got[0].Label != "all subpackages and entry functions" {
		t.Fatalf("current retry aggregate should win over retained display metadata, got %+v", got[0])
	}
}

func TestMergeCompletionAggregateFacts_RetainsStableSupersetMemberSet(t *testing.T) {
	current := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "internal/analysis subpackages and entry functions",
		Value: "2",
		Unit:  "rows",
		Members: []string{
			"aggregator: Aggregate (aggregator.go:132)",
			"compiler: Compile (compile.go:37)",
		},
	}}
	stable := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "internal/analysis subpackages and entry functions",
		Value: "3",
		Unit:  "rows",
		Members: []string{
			"aggregator -> Aggregate (line 132)",
			"compiler -> Compile (line 37)",
			"subject -> Score (line 40)",
		},
	}}

	got := mergeCompletionAggregateFacts(current, stable)
	if len(got) != 1 {
		t.Fatalf("same labeled member_set facts should collapse to the fullest typed set, got %+v", got)
	}
	if got[0].Value != "3" || len(got[0].Members) != 3 {
		t.Fatalf("stable superset should survive a later narrowed retry, got %+v", got[0])
	}
	if !strings.Contains(strings.Join(got[0].Members, "\n"), "subject") {
		t.Fatalf("stable-only member should be retained, got %+v", got[0].Members)
	}
}

func TestMergeCompletionAggregateFacts_CurrentSupersetWinsStableSubset(t *testing.T) {
	current := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "internal/analysis subpackages and entry functions",
		Value: "3",
		Unit:  "rows",
		Members: []string{
			"aggregator: Aggregate (aggregator.go:132)",
			"compiler: Compile (compile.go:37)",
			"subject: Score (taxonomy.go:40)",
		},
	}}
	stable := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "internal/analysis subpackages and entry functions",
		Value: "2",
		Unit:  "rows",
		Members: []string{
			"aggregator -> Aggregate (line 132)",
			"compiler -> Compile (line 37)",
		},
	}}

	got := mergeCompletionAggregateFacts(current, stable)
	if len(got) != 1 {
		t.Fatalf("same labeled member_set facts should collapse when current is the typed superset, got %+v", got)
	}
	if got[0].Value != "3" || len(got[0].Members) != 3 {
		t.Fatalf("current superset should win over retained narrower set, got %+v", got[0])
	}
}

func TestMergeCompletionAggregateFacts_RetainsPrincipalRoleOverSupportingCoverageRetry(t *testing.T) {
	current := []types.AnswerAggregateFact{{
		Kind:       types.AnswerAggregateMemberSet,
		Label:      "files inspected",
		Value:      "2",
		Role:       types.AnswerAggregateRoleSupportingCoverage,
		Provenance: "command:rg",
		Members:    []string{"internal/agent/explorer.go", "internal/agent/finalizer.go"},
	}}
	stable := []types.AnswerAggregateFact{{
		Kind:       types.AnswerAggregateMemberSet,
		Label:      "principal implementation files",
		Value:      "2",
		Provenance: "previous_verified_completion",
		Members:    []string{"internal/agent/explorer.go", "internal/agent/finalizer.go"},
	}}

	got := mergeCompletionAggregateFacts(current, stable)
	if len(got) != 1 {
		t.Fatalf("equivalent member_set role variants should merge to one fact, got %+v", got)
	}
	if got[0].Label != "principal implementation files" || got[0].Role != "" {
		t.Fatalf("legacy principal stable fact should survive supporting coverage retry, got %+v", got[0])
	}
	if got[0].Provenance != "previous_verified_completion" {
		t.Fatalf("principal provenance should be retained, got %+v", got[0])
	}
}

func TestMergeCompletionAggregateFacts_KeepsDisjointSameLabelMemberSetsSeparate(t *testing.T) {
	current := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "public API",
		Value:   "1",
		Members: []string{"functions: Eval"},
	}}
	stable := []types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "public API",
		Value:   "1",
		Members: []string{"types: Kind"},
	}}

	got := mergeCompletionAggregateFacts(current, stable)
	if len(got) != 2 {
		t.Fatalf("disjoint same-label member sets should not be collapsed as a false superset, got %+v", got)
	}
}

func TestEmitInvestigationComplete_RejectsUnsupportedExhaustiveEnumerationMemberSet(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all public enum types")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "intent",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       642,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Intent",
		Subject:         "Intent",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all public enum types",
	}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"member set contains one grounded member and one nearby helper",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"public enum types",
			"value":"2",
			"unit":"types",
			"members":["Intent","NearbyHelper"],
			"support_refs":["Intent @ internal/types/analysis_ir.go:642"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("unsupported member-set downgrade is a soft pre-complete result, got hard failure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "member_set") ||
		!strings.Contains(res.Summary, "NearbyHelper") ||
		!strings.Contains(res.Summary, "typed evidence") {
		t.Fatalf("summary should point at unsupported principal members, got: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("downgraded completion must not mark investigation complete")
	}
}

func TestEmitInvestigationComplete_MemberSetNarrowsAnalyzerEntityCandidates(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all public enum types with const sets")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "intent",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       642,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Intent",
		Subject:         "Intent",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.AnalyzerHints.Entities = []string{"Intent", "CandidateWithoutConstSet"}
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all public enum types with const sets",
	}
	ir.AnswerContract.MustIncludeTerms = []types.ContractTerm{
		{Text: "Intent", Kind: types.ContractTermSymbol, Source: types.ContractTermSourceAnalyzerEntity},
		{Text: "CandidateWithoutConstSet", Kind: types.ContractTermSymbol, Source: types.ContractTermSourceAnalyzerEntity},
	}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"member set contains the verified filtered members",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"public enum types with const sets",
			"value":"1",
			"unit":"types",
			"members":["Intent"],
			"support_refs":["internal/types/analysis_ir.go:642"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("verified member_set should supersede analyzer-entity candidates: %s", res.Summary)
	}
	if strings.Contains(res.Summary, "required principal members lack typed handoff") {
		t.Fatalf("analyzer-entity candidates absent from member_set must not block completion: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("completion should be stored after analyzer candidates are narrowed by member_set")
	}
}

func TestEmitInvestigationComplete_AcceptsExactEmptyMemberSetWithNegativeSearch(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all ArkTS entry components")
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all ArkTS entry components",
	}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir}
	tool := &EmitInvestigationComplete{}

	res, err := tool.Execute(bus, exactEmptyArkTSAggregateCompletionParams())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("empty member_set with typed negative support should pass: %s", res.Summary)
	}
	if strings.Contains(res.Summary, "exhaustive member-set handoff is missing") ||
		strings.Contains(res.Summary, "empty member_set without typed zero-result support") {
		t.Fatalf("empty supported member_set should not downgrade: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 2 || !types.AnswerAggregateFactIsExactEmptyMemberSet(facts[1]) {
		t.Fatalf("stored aggregate facts = %+v, want negative_search plus exact empty member_set", facts)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("completion should be stored after exact empty member_set")
	}
}

func exactEmptyArkTSAggregateCompletionParams() json.RawMessage {
	return json.RawMessage(`{
		"reason":"the bounded repository search found no ArkTS entry components",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"negative_search",
				"label":"ArkTS @Entry component search",
				"value":"0",
				"unit":"matches",
				"dimensions":[
					{"name":"repo","value":"current repository"},
					{"name":"pattern","value":"@Entry|@Builder"},
					{"name":"scope","value":"ArkTS source files"},
					{"name":"searched_at","value":"current_investigation"}
				]
			},
			{
				"kind":"member_set",
				"label":"ArkTS entry components",
				"value":"0",
				"unit":"components",
				"role":"principal_answer",
				"members":[]
			}
		]
	}`)
}

func arkTSPathFilteredGrepNoMatchResult() types.ToolResult {
	return types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary: "[grep params: pattern=@Entry|@Builder path=. include=*.ets files_only=true]\n" +
			"no matches found\n" +
			"path_discovery_advisory=grep searches file contents, not file names.\n",
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:      types.ToolPathDiscoveryKindGrep,
			Path:      ".",
			Include:   "*.ets",
			FilesOnly: true,
			NoMatches: true,
		},
	}
}

func arkTSRecursiveListFilesResult(includeAuxiliary bool) types.ToolResult {
	summary := "[list_files: path=. recursive=true include=*.ets]\n"
	if includeAuxiliary {
		summary = "[list_files: path=. recursive=true include=*.ets include_auxiliary=true]\n"
	}
	return types.ToolResult{
		ToolName: "list_files",
		Success:  true,
		Summary:  summary,
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:             types.ToolPathDiscoveryKindListFiles,
			Path:             ".",
			Recursive:        true,
			Include:          "*.ets",
			IncludeAuxiliary: includeAuxiliary,
		},
	}
}

func TestEmitInvestigationComplete_RejectsPathFamilyAbsenceAfterPathFilteredGrepOnly(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all ArkTS entry components")
	mut.AppendDispatchToolResult(arkTSPathFilteredGrepNoMatchResult())
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all ArkTS entry components",
	}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir, RepoRoot: t.TempDir()}
	tool := &EmitInvestigationComplete{}

	res, err := tool.Execute(bus, exactEmptyArkTSAggregateCompletionParams())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("path-filtered grep-only empty proof should soft-downgrade, not hard-reject: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("path-filtered grep-only empty proof must not mark completion before convergence")
	}
	if !strings.Contains(res.Summary, "repo-wide file-family absence is not proven") ||
		!strings.Contains(res.Summary, "list_files") {
		t.Fatalf("summary should steer to typed path discovery, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_IgnoresSummaryOnlyPathDiscoveryBanners(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all ArkTS entry components")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary: "[grep params: pattern=@Entry|@Builder path=. include=*.ets files_only=true]\n" +
			"no matches found\n" +
			"path_discovery_advisory=grep searches file contents, not file names.\n",
	})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all ArkTS entry components",
	}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir, RepoRoot: t.TempDir()}
	tool := &EmitInvestigationComplete{}

	res, err := tool.Execute(bus, exactEmptyArkTSAggregateCompletionParams())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("summary-only path-discovery banner must not drive completion gate: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("summary-only banner should not create path-discovery debt")
	}
	if strings.Contains(res.Summary, "repo-wide file-family absence is not proven") {
		t.Fatalf("summary-only banner should not trigger path-family repair: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_AcceptsPathFamilyAbsenceAfterRecursiveListFiles(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all ArkTS entry components")
	mut.AppendDispatchToolResult(arkTSPathFilteredGrepNoMatchResult())
	mut.AppendDispatchToolResult(arkTSRecursiveListFilesResult(false))
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all ArkTS entry components",
	}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir, RepoRoot: t.TempDir()}
	tool := &EmitInvestigationComplete{}

	res, err := tool.Execute(bus, exactEmptyArkTSAggregateCompletionParams())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("recursive path-family discovery should satisfy empty proof: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_PathFamilyAbsenceDebtForceCompletesWithCaveatAfterConvergence(t *testing.T) {
	prevPolicy := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prevPolicy) })
	SetDowngradeConvergenceHardThreshold(2)
	t.Cleanup(func() { SetDowngradeConvergenceHardThreshold(0) })

	mut := types.NewMutableState("List all ArkTS entry components")
	mut.AppendDispatchToolResult(arkTSPathFilteredGrepNoMatchResult())
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all ArkTS entry components",
	}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir, RepoRoot: t.TempDir()}
	tool := &EmitInvestigationComplete{}

	first, err := tool.Execute(bus, exactEmptyArkTSAggregateCompletionParams())
	if err != nil {
		t.Fatalf("first Execute returned error: %v", err)
	}
	if !first.Success || mut.IsInvestigationComplete() {
		t.Fatalf("first path-family debt should soft-downgrade without completion: success=%t summary=%s", first.Success, first.Summary)
	}
	second, err := tool.Execute(bus, exactEmptyArkTSAggregateCompletionParams())
	if err != nil {
		t.Fatalf("second Execute returned error: %v", err)
	}
	if !second.Success || !mut.IsInvestigationComplete() {
		t.Fatalf("repeated identical path-family debt should force-complete with caveat: success=%t summary=%s", second.Success, second.Summary)
	}
	caveats := mut.EvidenceClosure().CompletionCaveats()
	if len(caveats) != 1 || caveats[0].Lane != types.DowngradeLanePathDiscoveryAbsence {
		t.Fatalf("completion caveats = %+v, want path_discovery_absence", caveats)
	}
}

func TestEmitInvestigationComplete_RequiresAuxiliaryPathDiscoveryWhenRepoHasAuxiliaryTrees(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "internal", "thirdparty", "corpus"), 0o755); err != nil {
		t.Fatalf("mkdir auxiliary corpus: %v", err)
	}
	mut := types.NewMutableState("List all ArkTS entry components")
	mut.AppendDispatchToolResult(arkTSPathFilteredGrepNoMatchResult())
	mut.AppendDispatchToolResult(arkTSRecursiveListFilesResult(false))
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all ArkTS entry components",
	}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir, RepoRoot: repo}
	tool := &EmitInvestigationComplete{}

	res, err := tool.Execute(bus, exactEmptyArkTSAggregateCompletionParams())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("auxiliary tree path proof should soft-downgrade, not hard-reject: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("auxiliary tree path proof must not mark completion before convergence")
	}
	if !strings.Contains(res.Summary, "include_auxiliary=true") {
		t.Fatalf("summary should request auxiliary discovery, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_AcceptsAuxiliaryPathDiscoveryWhenRepoHasAuxiliaryTrees(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "internal", "thirdparty", "corpus"), 0o755); err != nil {
		t.Fatalf("mkdir auxiliary corpus: %v", err)
	}
	mut := types.NewMutableState("List all ArkTS entry components")
	mut.AppendDispatchToolResult(arkTSPathFilteredGrepNoMatchResult())
	mut.AppendDispatchToolResult(arkTSRecursiveListFilesResult(true))
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all ArkTS entry components",
	}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir, RepoRoot: repo}
	tool := &EmitInvestigationComplete{}

	res, err := tool.Execute(bus, exactEmptyArkTSAggregateCompletionParams())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("auxiliary-aware recursive path discovery should satisfy empty proof: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_RejectsExactEmptyMemberSetWithoutTypedNoHitSupport(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all ArkTS entry components")
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all ArkTS entry components",
	}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"empty set without typed no-hit support",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"ArkTS entry components",
			"value":"0",
			"unit":"components",
			"role":"principal_answer",
			"members":[]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("unsupported empty member_set should be a soft downgrade, not hard failure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "empty member_set without typed zero-result support") {
		t.Fatalf("summary should explain typed no-hit support requirement, got: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("downgraded empty member_set must not mark investigation complete")
	}
}

func TestEmitInvestigationComplete_BucketCountMembersCoverPrincipalTerms(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List public criterion symbols by bucket")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "criterion",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       1075,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Criterion",
		Subject:         "Criterion",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "complete public symbols",
	}
	ir.AnswerContract.MustIncludeTerms = []types.ContractTerm{{
		Text:   "Criterion",
		Kind:   types.ContractTermSymbol,
		Source: types.ContractTermSourceAnalyzerEntity,
	}}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"bucket count carries the exact typed principal member list",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"bucket_count",
			"label":"public types",
			"value":"1",
			"unit":"types",
			"members":["Criterion"],
			"role":"principal_answer",
			"support_refs":["internal/types/analysis_ir.go:1075"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("complete bucket_count members should satisfy principal handoff: %s", res.Summary)
	}
	if strings.Contains(res.Summary, "required principal members lack typed handoff") ||
		strings.Contains(res.Summary, "exhaustive member-set handoff is missing") {
		t.Fatalf("bucket_count exact members must not trigger member handoff retry: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) == "" {
		t.Fatalf("completion should be stored after exact bucket_count member carrier")
	}
}

func TestEmitInvestigationComplete_MemberSetDoesNotWaiveExplicitRequiredTerms(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all agents")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "agent-explorer",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/enums.go",
		LineStart:       117,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "AgentExplorer",
		Subject:         "AgentExplorer",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.AnalyzerHints.Entities = []string{"AgentExplorer", "AgentFinalizer"}
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all agents",
	}
	ir.AnswerContract.MustIncludeTerms = []types.ContractTerm{
		{Text: "AgentExplorer", Kind: types.ContractTermSymbol},
		{Text: "AgentFinalizer", Kind: types.ContractTermSymbol},
	}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"member set carried one member only",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"agents",
			"value":"1",
			"unit":"agents",
			"members":["AgentExplorer"],
			"support_refs":["internal/types/enums.go:117"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("required-term handoff downgrade is a soft pre-complete result, got hard failure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "required principal members lack typed handoff") ||
		!strings.Contains(res.Summary, "AgentFinalizer") {
		t.Fatalf("explicit required terms must still block when absent from member_set, got: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("downgraded completion must not mark investigation complete")
	}
}

func TestEmitInvestigationComplete_CanonicalizesMemberSetValue(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all public enum types")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "intent",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       642,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Intent",
		Subject:         "Intent",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all public enum types",
	}
	bus := &types.BusContext{Mutable: mut, AnalysisIR: ir}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{
		"reason":"member set carried through aggregate_facts",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"member_set",
			"label":"public enum types",
			"unit":"types",
			"members":["Intent"],
			"support_refs":["internal/types/analysis_ir.go:642"]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("member_set value should be canonicalized from members: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 || facts[0].Value != "1" {
		t.Fatalf("member_set value not canonicalized: %+v", facts)
	}
}

func TestEmitInvestigationComplete_DowngradesWhenRequiredEnumerationTermsLackTypedHandoff(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("List all agents")
	mut.AppendEvidence([]types.EvidenceItem{{
		ID:              "agent-explorer",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/types/enums.go",
		LineStart:       117,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "AgentExplorer",
		Subject:         "AgentExplorer",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	ir := enumerationPrincipalGateIR()
	ir.RequestModel.AnalyzerHints.Entities = []string{"AgentExplorer", "AgentFinalizer"}
	ir.RequestModel.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "all agents",
	}
	ir.AnswerContract.MustIncludeTerms = []types.ContractTerm{
		{Text: "AgentExplorer", Kind: types.ContractTermSymbol},
		{Text: "AgentFinalizer", Kind: types.ContractTermSymbol},
	}
	bus := &types.BusContext{
		Mutable:    mut,
		AnalysisIR: ir,
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"only one member grounded","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("required-term handoff downgrade is a soft pre-complete result, got hard failure: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "required principal members lack typed handoff") ||
		!strings.Contains(res.Summary, "AgentFinalizer") {
		t.Fatalf("summary should name missing typed handoff member, got: %s", res.Summary)
	}
	if strings.TrimSpace(mut.InvestigationCompleteReason()) != "" {
		t.Fatalf("downgraded completion must not mark investigation complete")
	}
	repairs := mut.EvidenceClosure().PendingRepairs()
	if len(repairs) == 0 || repairs[len(repairs)-1].Origin != "pre_complete.principal_required_term_handoff" {
		t.Fatalf("expected principal required-term handoff repair, got %+v", repairs)
	}
}

func TestCompletionPrincipalHandoffTerms_SoftensAnalyzerEntitiesForGroupedEnumeration(t *testing.T) {
	contract := types.AnswerContract{MustIncludeTerms: []types.ContractTerm{
		{Text: "Criterion", Kind: types.ContractTermSymbol, Source: types.ContractTermSourceAnalyzerEntity},
		{Text: "block", Kind: types.ContractTermFileStem, Source: types.ContractTermSourceAnalyzerEntity},
		{Text: "ExplicitSymbol", Kind: types.ContractTermSymbol},
	}}
	rm := types.RequestModel{
		Intent: types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		SubTopics: []types.SubTopic{
			{Summary: "types"},
			{Summary: "functions"},
			{Summary: "constants"},
		},
	}

	got := completionPrincipalHandoffTerms(contract, rm)
	if len(got) != 1 || got[0].Text != "ExplicitSymbol" {
		t.Fatalf("grouped enumeration should soften analyzer-entity terms only, got %+v", got)
	}

	rm.SubTopics = nil
	got = completionPrincipalHandoffTerms(contract, rm)
	if len(got) != 2 {
		t.Fatalf("ungrouped enumeration should preserve analyzer symbol guard terms while softening scope file-stems, got %+v", got)
	}
	for _, term := range got {
		if term.Kind == types.ContractTermFileStem {
			t.Fatalf("analyzer file-stem terms name the scoped package/directory, not principal members: %+v", got)
		}
	}

	rm.CompletenessObligation = &types.CompletenessObligation{
		Required:    true,
		SourceQuote: "complete members",
	}
	got = completionPrincipalHandoffTerms(contract, rm)
	if len(got) != 1 || got[0].Text != "ExplicitSymbol" {
		t.Fatalf("exhaustive enumeration should get hard members from aggregate_facts/evidence, not analyzer entities, got %+v", got)
	}
}

func TestCompletionMissingPrincipalHandoffTerms_GroundedEvidenceCoversAnalyzerSymbols(t *testing.T) {
	terms := []types.ContractTerm{
		{Text: "Eval", Kind: types.ContractTermSymbol, Source: types.ContractTermSourceAnalyzerEntity},
		{Text: "EvalAll", Kind: types.ContractTermSymbol, Source: types.ContractTermSourceAnalyzerEntity},
		{Text: "SetExternalArtifactFloor", Kind: types.ContractTermSymbol, Source: types.ContractTermSourceAnalyzerEntity},
	}
	evidence := []types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/analysis/criterion/eval.go",
			LineStart:       15,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "Eval",
			Subject:         "Eval",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/analysis/criterion/eval.go",
			LineStart:       36,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "EvalAll",
			Subject:         "EvalAll",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/analysis/criterion/eval.go",
			LineStart:       982,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "SetExternalArtifactFloor",
			Subject:         "SetExternalArtifactFloor",
			GroundingStatus: types.GroundingGrounded,
		},
	}
	if missing := completionMissingPrincipalHandoffTerms(terms, evidence, nil, nil); len(missing) != 0 {
		t.Fatalf("grounded direct definition evidence should satisfy analyzer symbol handoff terms, got %+v", missing)
	}
}

func enumerationPrincipalGateIR() *types.AnalysisIR {
	return &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent:     types.IntentEnumerate,
			Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
		},
		AnswerContract: types.AnswerContract{
			CitationReq: types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 1},
		},
	}
}

// TestEmitInvestigationComplete_Tier1FloorDisabledWhenZero — floor=0
// preserves session-7 backward-compat behaviour (no Tier-1 gate).
func TestEmitInvestigationComplete_Tier1FloorDisabledWhenZero(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0.5, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: "a.go", LineStart: 10,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "Foo",
		GroundingStatus: types.GroundingRecovered, GroundingTier: types.TierFQNameSameFile,
	}})
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"done","confidence":"high","result_kind":"resolved"}`)
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Errorf("Tier1Floor=0 must disable the gate; got rejection: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_Tier1FloorQueuesTier2GroundedReads(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0.5, Tier1Floor: 0.3})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("q")
	closure := mut.EvidenceClosure()
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceMechanism, Source: "internal/tool/repomap/tool.go", LineStart: 133,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "buildOrLoadGraph",
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierFQNameSameFile,
	}})
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"done","confidence":"high","result_kind":"resolved"}`)
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("tier2-only investigation should be downgraded, not tool-rejected: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("tier2-only downgrade must not mark investigation complete before convergence")
	}
	if repairs := closure.PendingRepairs(); len(repairs) != 1 {
		t.Fatalf("expected one RepairReadFile for tier2 grounded item, got %d: %+v", len(repairs), repairs)
	} else if repairs[0].Files[0] != "internal/tool/repomap/tool.go" {
		t.Fatalf("repair file = %q, want internal/tool/repomap/tool.go", repairs[0].Files[0])
	}
}

func TestEmitInvestigationComplete_Tier1FloorIgnoresAuxiliaryContextOnlyItems(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0.5, Tier1Floor: 0.3})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceUnresolved,
			Source:          "docs/example.go",
			LineStart:       10,
			ContextRole:     types.EvidenceContextRoleIllustrativeOnly,
			GroundingStatus: types.GroundingUngrounded,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       214,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "explore_mid_loop_hint_budget",
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingUngrounded,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       707,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"done","confidence":"high","result_kind":"resolved"}`)
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("auxiliary illustrative/absence-support items should not force a tier1-floor retry, got rejection: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_Tier1FloorIgnoresRelatedContextForScalarRoleLocate(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0.5, Tier1Floor: 0.6})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       883,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "buildAnalysisIR",
			ContextRole:     types.EvidenceContextRoleDefining,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/analysis_ir.go",
			LineStart:       25,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "AnalysisIR",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingRecovered,
			GroundingTier:   types.TierFQNameSameFile,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentExplain,
				Complexity:    types.ComplexitySimple,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
				Predicates: types.SemanticPredicates{
					IsScalarAnswer:     true,
					IsRoleLocateLookup: true,
				},
				AnalyzerHints: types.AnalyzerHints{
					Kind:            "mechanism",
					PrimaryEntities: []string{"AnalysisIR"},
					Entities:        []string{"AnalysisIR"},
				},
			},
		},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"the defining function line is grounded and the nearby type name is only explanatory context","confidence":"high","result_kind":"resolved"}`)
	res, _ := tool.Execute(bus, params)
	if !res.Success {
		t.Fatalf("scalar role-locate related_context should not dilute the tier1 floor, got rejection: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceAllowsContextualEvidence(t *testing.T) {
	missingKey := "zz_absent_config_" + "knob"
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: "cmd/root.go", LineStart: 889,
		Subject: "explore_midloop_min_iteration", AnchorKind: types.AnchorAssignment, AnchorSymbol: "ExploreMidLoopMinIteration",
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 在哪里定义？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + `; related budget keys are only context","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("config-key absence with contextual related evidence should be accepted: %s", res.Summary)
	}
	if mut.AbsenceJustification() == "" {
		t.Errorf("absence must be stored on acceptance")
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceAllowsTestOnlyExactMentions(t *testing.T) {
	missingKey := "zz_absent_config_" + "knob"
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind: types.EvidenceDirect, Source: "cmd/root.go", LineStart: 889,
			Subject: "explore_midloop_min_iteration", AnchorKind: types.AnchorAssignment, AnchorSymbol: "ExploreMidLoopMinIteration",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceDirect, Source: "internal/tool/emit_answer_document_test.go", LineStart: 813,
			Subject: missingKey, AnchorKind: types.AnchorDefinition, AnchorSymbol: "hint_budget",
			Snippet:         "no config key named `" + missingKey + "` exists in the repo",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 鍦ㄥ摢閲屽畾涔夛紵",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + `; related explore defaults are only context","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("test-only exact mentions should not block absence closure: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("investigation should be marked complete")
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceAllowsAbsenceSupportProductionMentions(t *testing.T) {
	missingKey := "zz_absent_config_" + "knob"
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       207,
			LineEnd:         221,
			Subject:         missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    missingKey,
			Summary:         "RuntimeSettings does not define " + missingKey,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       707,
			LineEnd:         724,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "Related context only: nearby explore defaults do not define " + missingKey,
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 的最终值怎么计算？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in production code","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("absence-support production mentions should still allow exact absence closure: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceIgnoresNegativeProbeDefiningHints(t *testing.T) {
	missingKey := "zz_absent_config_" + "knob"
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       231,
			Subject:         "RuntimeSettings",
			Predicate:       "does not bind",
			Object:          missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "RuntimeSettings",
			Summary:         "RuntimeSettings does not bind the missing exact config key.",
			ContextRole:     types.EvidenceContextRoleDefining,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       707,
			Subject:         "DefaultExploreHeuristics",
			Predicate:       "does not provide",
			Object:          "mid_loop_hint_budget",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "Same-family defaults code does not provide a mid_loop_hint_budget field.",
			ContextRole:     types.EvidenceContextRoleDefining,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 的最终有效值怎么计算？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in production code","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("negative exact-target probes should not block honest absence closure even if their context_role is stale/defining: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceAllowsRelatedContextThatKeepsTargetInSubjectOnly(t *testing.T) {
	missingKey := "zz_absent_config_" + "knob"
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/explore_budget.go",
			LineStart:       40,
			LineEnd:         48,
			Subject:         missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "ExploreBudget",
			Object:          "internal/types/explore_budget.go",
			Summary:         "Nearby runtime budget struct does not define the missing exact config key.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 鐨勬渶缁堝€兼€庝箞璁＄畻锛?",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in production code","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("related-context evidence that keeps the exact target only in subject text must not block exact absence closure: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceAllowsClosureReadyMixedContextRoles(t *testing.T) {
	missingKey := "zz_absent_config_mixed_roles"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{
		"internal/types/config.go",
		"codrax.yaml.example",
		"cmd/root.go",
	})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       848,
			LineEnd:         866,
			Subject:         "DefaultExploreHeuristics",
			Predicate:       "does not provide",
			Object:          missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "DefaultExploreHeuristics does not provide the missing exact config key.",
			ContextRole:     types.EvidenceContextRoleDefining,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "codrax.yaml.example",
			LineStart:       410,
			LineEnd:         410,
			Subject:         "explore_midloop_min_iteration",
			AnchorKind:      types.AnchorAssignment,
			AnchorSymbol:    "explore_midloop_min_iteration",
			Summary:         "Config file layer lists supported explore_* keys, but not the missing exact key.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleConfig,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "cmd/root.go",
			LineStart:       1618,
			LineEnd:         1620,
			Subject:         "ExploreMidLoopMinIteration",
			AnchorKind:      types.AnchorAssignment,
			AnchorSymbol:    "ExploreMidLoopMinIteration",
			Summary:         "Operator override layer only merges supported Explore_* fields, not the missing exact key.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleOverride,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 的最终有效值怎么计算？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in any supported precedence layer","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("closure-ready mixed context roles should still allow exact absence closure: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceRejectsUngroundedRequiredContext(t *testing.T) {
	missingKey := "explore_mid_loop_missing_knob"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       207,
			LineEnd:         221,
			Subject:         missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "RuntimeSettings",
			Summary:         "RuntimeSettings does not define " + missingKey,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 的默认值在哪定义？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in production code","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("absence closure with missing required context should soft-downgrade, not hard-reject: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("missing required context must not mark completion before convergence")
	}
	if !strings.Contains(res.Summary, "precedence-capable lineage anchor") || !strings.Contains(res.Summary, "diagram_role_hint") || !strings.Contains(res.Summary, "internal/types/config.go") {
		t.Fatalf("rejection should name the missing related-context requirement, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ExactAbsenceContextDebtForceCompletesWithCaveatAfterConvergence(t *testing.T) {
	SetDowngradeConvergenceHardThreshold(2)
	t.Cleanup(func() { SetDowngradeConvergenceHardThreshold(0) })

	missingKey := "explore_mid_loop_missing_knob"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       207,
			LineEnd:         221,
			Subject:         missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "RuntimeSettings",
			Summary:         "RuntimeSettings does not define " + missingKey,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 的默认值在哪定义？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in production code","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)

	first, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("first Execute returned error: %v", err)
	}
	if !first.Success || mut.IsInvestigationComplete() {
		t.Fatalf("first exact-absence context debt should soft-downgrade without completion: success=%t summary=%s", first.Success, first.Summary)
	}
	second, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("second Execute returned error: %v", err)
	}
	if !second.Success || !mut.IsInvestigationComplete() {
		t.Fatalf("repeated exact-absence context debt should force-complete with caveat: success=%t summary=%s", second.Success, second.Summary)
	}
	caveats := mut.EvidenceClosure().CompletionCaveats()
	if len(caveats) != 1 || caveats[0].Lane != types.DowngradeLaneExactAbsenceContext {
		t.Fatalf("completion caveats = %+v, want exact_absence_context", caveats)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceAllowsGroundedRequiredContext(t *testing.T) {
	missingKey := "explore_mid_loop_missing_knob"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       207,
			LineEnd:         221,
			Subject:         missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "RuntimeSettings",
			Summary:         "RuntimeSettings does not define " + missingKey,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       707,
			LineEnd:         724,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "Grounded same-family defaults context for nearby explore settings.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 的默认值在哪定义？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in production code","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("grounded required related-context anchor should allow absence closure: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceAllowsNearbyGroundedContextThatMentionsMissingKeyInObject(t *testing.T) {
	missingKey := "explore_mid_loop_hint_budget"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go", "codrax.yaml.example", "cmd/root.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       296,
			Subject:         "RuntimeSettings",
			Object:          missingKey,
			AnchorKind:      types.AnchorAssignment,
			AnchorSymbol:    "ExploreMidLoopMinIteration",
			Summary:         "RuntimeSettings does not define " + missingKey,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       848,
			Subject:         "DefaultExploreHeuristics",
			Object:          missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "DefaultExploreHeuristics defines nearby explore defaults but not " + missingKey,
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       296,
			Subject:         "ExploreMidLoopMinIteration",
			AnchorKind:      types.AnchorAssignment,
			AnchorSymbol:    "ExploreMidLoopMinIteration",
			Summary:         "Runtime binding for nearby explore defaults.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleRuntime,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in production code","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("nearby grounded context that names the missing key only in explanatory object text should still allow absence closure: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceRejectsGroundedRequiredContextWithoutDiagramRole(t *testing.T) {
	missingKey := "explore_mid_loop_missing_knob"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       207,
			LineEnd:         221,
			Subject:         missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "RuntimeSettings",
			Summary:         "RuntimeSettings does not define " + missingKey,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       707,
			LineEnd:         724,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "Grounded same-family defaults context for nearby explore settings.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 的默认值在哪定义？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in production code","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("config-trace absence context without validated diagram role should soft-downgrade, not hard-reject: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatal("context without validated diagram role must not mark completion before convergence")
	}
	if !strings.Contains(res.Summary, "precedence-capable lineage anchor") || !strings.Contains(res.Summary, "diagram_role_hint") {
		t.Fatalf("rejection should explain the missing precedence anchor requirement, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceRejectsRecoveredRequiredContext(t *testing.T) {
	missingKey := "explore_mid_loop_missing_knob"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       207,
			LineEnd:         221,
			Subject:         missingKey,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "RuntimeSettings",
			Summary:         "RuntimeSettings does not define " + missingKey,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       707,
			LineEnd:         724,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "Grounded same-family defaults context for nearby explore settings.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingRecovered,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 的默认值在哪定义？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in production code","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("recovered related-context evidence should soft-downgrade, not hard-reject: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatal("recovered related-context evidence must not mark completion before convergence")
	}
	if !strings.Contains(res.Summary, "grounded precedence-capable lineage anchor") && !strings.Contains(res.Summary, "precedence-capable lineage anchor") {
		t.Fatalf("rejection should preserve the missing-grounded-anchor guidance, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceIgnoresAuxiliaryRecoveredContextInTier1Floor(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0.5, Tier1Floor: 0.3})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	missingKey := "explore_mid_loop_missing_knob"
	mut := types.NewMutableState("q")
	mut.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/config/runtime.go",
			LineStart:       296,
			LineEnd:         310,
			Subject:         missingKey,
			AnchorKind:      types.AnchorAssignment,
			AnchorSymbol:    "ExploreMidLoopMinIteration",
			Summary:         "RuntimeSettings does not define " + missingKey,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       848,
			LineEnd:         865,
			Subject:         "DefaultExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "DefaultExploreHeuristics",
			Summary:         "Grounded same-family defaults context for nearby explore settings.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		},
		{
			Kind:            types.EvidenceDirect,
			Source:          "internal/types/config.go",
			LineStart:       768,
			LineEnd:         784,
			Subject:         "ExploreHeuristics",
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "ExploreHeuristics",
			Summary:         "Nearby same-family struct context.",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingRecovered,
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 的最终有效值是怎么计算出来的？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"searched the repo and found no exact config key ` + missingKey + ` in production code","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("auxiliary recovered nearby-context items should not force a tier1-floor retry once exact-absence closure already has a grounded same-scope anchor: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceBypassesGroundingFloorsForContextOnlyEvidence(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0.5, Tier1Floor: 0.3})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	missingKey := "zz_absent_config_" + "knob"
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			Kind: types.EvidenceDirect, Source: "cmd/root.go", LineStart: 889,
			Subject: "explore_midloop_min_iteration", AnchorKind: types.AnchorAssignment, AnchorSymbol: "ExploreMidLoopMinIteration",
			GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
		},
		{
			Kind: types.EvidenceUnresolved, Source: "internal/config/runtime.go",
			Subject: "AgentLoopMaxMidLoopInjects", AnchorKind: types.AnchorDefinition, AnchorSymbol: "AgentLoopMaxMidLoopInjects",
			GroundingStatus: types.GroundingUngrounded,
		},
		{
			Kind: types.EvidenceUnresolved, Source: "cmd/root.go",
			Subject: "zz_absent_context_knob", AnchorKind: types.AnchorAssignment, AnchorSymbol: "zz_absent_context_knob",
			GroundingStatus: types.GroundingUngrounded,
		},
		{
			Kind: types.EvidenceUnresolved, Source: "internal/config/runtime.go",
			Subject: "AgentLoopMaxDocBytes", AnchorKind: types.AnchorDefinition, AnchorSymbol: "AgentLoopMaxDocBytes",
			GroundingStatus: types.GroundingUngrounded,
		},
		{
			Kind: types.EvidenceUnresolved, Source: "codrax.yaml",
			Subject: "zz_absent_config_knob_context", AnchorKind: types.AnchorAssignment, AnchorSymbol: "zz_absent_config_knob_context",
			GroundingStatus: types.GroundingUngrounded,
		},
		{
			Kind: types.EvidenceUnresolved, Source: "internal/agent/explorer.go",
			Subject: "context budget knob", AnchorKind: types.AnchorDefinition, AnchorSymbol: "contextBudgetKnob",
			GroundingStatus: types.GroundingUngrounded,
		},
	})
	mut.EvidenceClosure().AppendUnverifiedFinding(types.UnverifiedFinding{
		Token:  missingKey,
		Kind:   "symbol",
		Reason: "exact target has no current production-defining prescan hit",
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 在哪里定义？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"found no exact config key ` + missingKey + `; related budget keys are context only","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("honest config-key absence should bypass generic grounding floors: %s", res.Summary)
	}
	if mut.AbsenceJustification() == "" {
		t.Fatalf("absence justification must be stored")
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("investigation should be marked complete")
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceRejectsExactEvidence(t *testing.T) {
	missingKey := "zz_absent_config_" + "knob"
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: "codrax.yaml", LineStart: 12,
		Subject: missingKey, AnchorKind: types.AnchorAssignment, AnchorSymbol: missingKey,
		Snippet:         missingKey + ": 2",
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 在哪里定义？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"no exact config key ` + missingKey + ` exists","confidence":"high","result_kind":"absence","absence_justification":"no config key named ` + missingKey + ` exists in the repo"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("absence must still be rejected when exact config-key evidence exists")
	}
	if mut.AbsenceJustification() != "" {
		t.Errorf("absence must NOT be stored on rejection")
	}
}

func TestEmitInvestigationComplete_ConfigAbsenceRejectsPositiveSubstituteFromTypedUnverifiedFinding(t *testing.T) {
	missingKey := "zz_absent_config_" + "knob"
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: "internal/config/runtime.go", LineStart: 184,
		Subject: "AgentLoopMaxMidLoopInjects", AnchorKind: types.AnchorDefinition, AnchorSymbol: "AgentLoopMaxMidLoopInjects",
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
	}})
	mut.EvidenceClosure().AppendUnverifiedFinding(types.UnverifiedFinding{
		Token:  missingKey,
		Kind:   "symbol",
		Reason: "exact target has no current production-defining prescan hit",
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 在哪里定义？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
	}
	tool := &EmitInvestigationComplete{}

	params := json.RawMessage(`{"reason":"positive chain is fully traced through AgentLoopMaxMidLoopInjects","confidence":"high","result_kind":"resolved"}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("positive substitute completion should downgrade instead of tool-rejecting: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "primary exact config key") {
		t.Fatalf("downgrade should explain exact-key guard: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("completion flag must not fire on positive substitute downgrade")
	}
	if res.Repair == nil || res.Repair.Code != "exact_resolved_defining_proof" {
		t.Fatalf("downgrade should carry exact resolved defining-proof repair, got %+v", res.Repair)
	}
}

func TestEmitInvestigationComplete_AnnotatedStageReportDoesNotOverrideExactDefiningProof(t *testing.T) {
	missingKey := "zz_absent_config_" + "knob"
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind: types.EvidenceDirect, Source: "codrax.yaml", LineStart: 12,
		Subject: missingKey, AnchorKind: types.AnchorAssignment, AnchorSymbol: missingKey,
		Snippet:         missingKey + ": 2",
		GroundingStatus: types.GroundingGrounded, GroundingTier: types.TierLineText,
	}})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: missingKey + " 在哪里定义？",
			Scenario:   types.ScenarioConfigTrace,
			AnalyzerHints: types.AnalyzerHints{
				Kind:            "config_mapping",
				PrimaryEntities: []string{missingKey},
				Entities:        []string{missingKey},
				ExactTargets:    []string{missingKey},
			},
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
		}},
		StageReports: []types.StageReport{{
			Stage:    types.StageAnalyze,
			Agent:    types.AgentAnalyzer,
			Findings: "The key ~~`" + missingKey + "`~~ [unverified: symbol not in repo graph] was not found exactly.",
		}},
	}

	contract := exactResolutionContractForCompletion(bus)
	if contract == nil {
		t.Fatal("test setup must produce exact-resolution contract")
	}
	if pending := completionPendingExactTargets(bus, contract, mut.EmittedEvidence()); len(pending) != 0 {
		t.Fatalf("StageReport prose must not override exact defining proof, pending=%v", pending)
	}

	mut.EvidenceClosure().AppendUnverifiedFinding(types.UnverifiedFinding{
		Token:  missingKey,
		Kind:   "symbol",
		Reason: "typed verifier still marks target unresolved",
	})
	if pending := completionPendingExactTargets(bus, contract, mut.EmittedEvidence()); len(pending) != 1 || pending[0] != missingKey {
		t.Fatalf("typed closure unverified finding should still drive exact target blocker, pending=%v", pending)
	}
}

// TestEmitInvestigationComplete_RepairsDecoratedMemberSetWithoutSupportRefs
// pins the completion-form debt policy: when a member_set uses
// "<code identifier> (<qualifier>)" labels but omits support_refs, the tool
// repairs the form locally by keeping the bare citable member surface and moving
// the qualifier into member_notes. This avoids turning answer-presentation debt
// into another exploration round.
func TestEmitInvestigationComplete_RepairsDecoratedMemberSetWithoutSupportRefs(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"comparison investigation complete",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"codrax 防幻觉组件",
				"value":"3",
				"members":[
					"Gate.Run (8个独立检查)",
					"Ground (3层验证)",
					"Orchestrator (4阶段管道整合)"
				]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("decorated member_set without support_refs should be repaired locally: %s", res.Summary)
	}
	for _, want := range []string{
		"aggregate_facts normalized",
		"decorated member labels",
	} {
		if !strings.Contains(res.Summary, want) {
			t.Errorf("repair summary missing %q in:\n%s", want, res.Summary)
		}
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) != 1 {
		t.Fatalf("stable aggregate facts = %+v, want one repaired member_set", got)
	}
	if got[0].Members[0] != "Gate.Run" || got[0].Members[2] != "Orchestrator" {
		t.Fatalf("decorated members were not canonicalized to bare citable surfaces: %+v", got[0].Members)
	}
	if len(got[0].MemberNotes) < 3 || got[0].MemberNotes[0] != "8个独立检查" || got[0].MemberNotes[2] != "4阶段管道整合" {
		t.Fatalf("decorator qualifiers not preserved in member_notes: %+v", got[0].MemberNotes)
	}
}

func TestEmitInvestigationComplete_AllowsDecoratedCommitHashMemberSetForHistoryLookup(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
		}},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"history commits collected",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"相关提交列表",
				"value":"2",
				"members":[
					"916511448b3b4046d1b265e818b1bde4d07ae133 (最早)",
					"e4f15cd2074c0eead7a6f835cf9f0c0000012345 (最近)"
				]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("decorated commit-hash member_set should rely on VCS provenance, not file:line support_refs: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_AllowsDecoratedCommitHashMemberSetWithVCSOrigin(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"history commits collected",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"相关提交列表",
				"value":"1",
				"provenance":"git_history_search",
				"dimensions":[{"name":"proof_source","value":"git_history_search"}],
				"members":["916511448b3b4046d1b265e818b1bde4d07ae133 (最早)"]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("decorated commit-hash member_set should rely on typed VCS origin projection: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_DecoratedCodeMemberSupportRefsDowngradesWithVCSOrigin(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"code members collected",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"code members",
				"value":"1",
				"provenance":"git_history_search",
				"dimensions":[{"name":"proof_source","value":"git_history_search"}],
				"members":["Gate.Run (8个独立检查)"]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("decorated code member support_refs debt should downgrade instead of tool-failing: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("first completion-form downgrade must not mark investigation complete")
	}
	if !strings.Contains(res.Summary, EmitInvestigationCompleteDowngradePrefix) ||
		!strings.Contains(res.Summary, "support_refs is empty") {
		t.Fatalf("summary should preserve support_refs contract as downgrade, got: %s", res.Summary)
	}
	if res.Repair == nil ||
		res.Repair.Metadata["lane"] != string(types.DowngradeLaneCompletionForm) ||
		res.Repair.Metadata["repair_origin"] != types.RepairOriginCompletionFormPrefix+"member_set_support_refs" {
		t.Fatalf("downgrade should carry completion-form repair metadata, got %+v", res.Repair)
	}
}

func TestEmitInvestigationComplete_AllowsDecoratedRuntimeMemberSetForObservationOnlyLog(t *testing.T) {
	logBundle := &types.LogBundle{
		Errors: []types.LogError{{
			Type: "panic",
			Frames: []types.LogFrame{{
				Func: "Cart.itemAt",
				File: "src/cart/Cart.cj",
				Line: 78,
			}},
		}},
	}
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:    types.IntentRootCause,
			Scenario:  types.ScenarioRootCause,
			LogTriage: logBundle,
			ExternalObservationPolicy: &types.ExternalObservationPolicy{
				CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
				ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
				SourceQuotes:      []string{"只分析日志"},
				Confidence:        0.9,
			},
		}},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"runtime call chain collected",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{
			"reason":"external_only_log",
			"rationale":"the log frames point to external paths and do not resolve in this checkout"
		},
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"call_chain",
				"value":"3",
				"members":[
					"Cart.itemAt (origin)",
					"Cart.checkout (trigger)",
					"demo.app.entry (entry)"
				]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("observation-only runtime member_set should not require repo support_refs: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_AllowsDecoratedRuntimeMemberSetForDefaultMixedRuntime(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:    types.IntentRootCause,
			Scenario:  types.ScenarioRootCause,
			LogTriage: &types.LogBundle{Errors: []types.LogError{{Type: "runtime trace"}}},
		}},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"runtime trace facts are enough for the observed sleep chain; source analysis remains optional",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"runtime blocking candidates",
				"value":"2",
				"provenance":"trace_query.wakeup_chain",
				"members":[
					"binder_wait (synchronous-looking)",
					"InternTable lock (owner tid: 32094)"
				]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("default mixed runtime member_set should rely on runtime provenance instead of repo support_refs: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_AllowsDecoratedRuntimeMemberSetAfterTraceQueryObservation(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{{
			ID:              "trace_query:payload#perf_stats:1",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef: types.ObservationSourceRef{
				Kind:         types.ObservationSourceRuntimeArtifact,
				Path:         "hiprofiler_data_20260624_17.04.05.htrace.systrace",
				ArtifactID:   "attached_trace",
				ArtifactKind: "trace",
				PayloadRef:   "/tmp/trace-query.json",
			},
			Subject:   "wp.wattpad-47216",
			Predicate: "cpu_hot_symbol",
			Object:    "android::PaintGlue::getRunCharacterAdvance___CIIIIZI_FI_F",
			Summary:   "perf samples identify libhwui text measurement as the top running-state hotspot",
		}},
	})
	bus := &types.BusContext{
		Mutable:         mut,
		AttachedHitrace: "sched_switch prev=wp.wattpad-47216 next=worker",
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentRootCause,
			Scenario: types.ScenarioRootCause,
		}},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"trace_query has isolated the observed running-state hotspot",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"热点函数调用栈",
				"value":"1",
				"members":["android::PaintGlue::getRunCharacterAdvance___CIIIIZI_FI_F (libhwui.so)"]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("trace_query runtime member_set should not require repo support_refs: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("trace_query runtime member_set should close the investigation, summary=%s", res.Summary)
	}
}

func TestEmitInvestigationComplete_CurrentSourceTraceRequestStillRequiresDecoratedSupportRefs(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		Observations: []types.ObservationRecord{{
			ID:              "trace_query:payload#root_cause_rank:1",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			Role:            types.AnswerAggregateRolePrincipalAnswer,
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef: types.ObservationSourceRef{
				Kind:         types.ObservationSourceRuntimeArtifact,
				Path:         "app.systrace",
				ArtifactID:   "attached_trace",
				ArtifactKind: "trace",
			},
			Subject:   "app-1",
			Predicate: "root_cause_primary",
			Object:    "source handler",
			Summary:   "trace points at a handler that the user asked to verify in current source",
		}},
	})
	bus := &types.BusContext{
		Mutable:         mut,
		AttachedHitrace: "sched_switch prev=app-1 next=worker",
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentRootCause,
			Scenario: types.ScenarioRootCause,
			CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
				IsCurrentSourceExplanationRequested: true,
				Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationVerifyCurrentStatus},
				SourceQuotes:                        []string{"结合当前源码确认"},
				Confidence:                          0.9,
			},
		}},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"current source verification still needs source-member grounding",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"source handlers",
				"value":"1",
				"members":["RenderHandler.processFrame (current implementation)"]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("completion-form support_refs debt should downgrade, not tool-fail: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatal("explicit current-source trace request must not close a decorated source member_set without support_refs")
	}
	if !strings.Contains(res.Summary, "support_refs is empty") {
		t.Fatalf("expected support_refs downgrade, got: %s", res.Summary)
	}
}

func TestEmitInvestigationCompleteSchema_DocumentsRuntimeDirectObservationBoundary(t *testing.T) {
	params := string((&EmitInvestigationComplete{}).Parameters())
	for _, want := range []string{
		"external runtime/log/trace artifacts",
		"direct observations separate from inferred upstream causes",
		"variable/parameter/caller",
		"possible upstream investigation direction",
	} {
		if !strings.Contains(params, want) {
			t.Fatalf("emit_investigation_complete schema should teach runtime direct-observation boundary; missing %q in:\n%s", want, params)
		}
	}
}

func TestEmitInvestigationCompleteSchema_DocumentsMemberSetSupportRefsBoundary(t *testing.T) {
	params := string((&EmitInvestigationComplete{}).Parameters())
	for _, want := range []string{
		"current-source principal member_set rows",
		"display aliases that may not exactly match an emitted evidence anchor",
		"positional refs",
		"one already-read grounded location per members[] entry",
		"member_notes are explanatory row notes",
		"do not replace support_refs",
	} {
		if !strings.Contains(params, want) {
			t.Fatalf("emit_investigation_complete schema should front-load member_set support_refs contract; missing %q in:\n%s", want, params)
		}
	}
}

func TestEmitInvestigationComplete_DecimalTotalCountNormalizesToScalar(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:    types.IntentRootCause,
			Scenario:  types.ScenarioRootCause,
			LogTriage: &types.LogBundle{Errors: []types.LogError{{Type: "runtime trace"}}},
		}},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"frame duration is a scalar runtime measurement",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"total_count",
			"label":"frame duration",
			"value":"119.227",
			"unit":"ms",
			"provenance":"trace_query.window_stats"
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("decimal count measurement should be normalized instead of rejected: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 || facts[0].Kind != types.AnswerAggregateScalar || facts[0].Value != "119.227" {
		t.Fatalf("expected scalar runtime measurement, got %+v", facts)
	}
	if !strings.Contains(res.Summary, "scalar_value") {
		t.Fatalf("summary should disclose scalar normalization: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_DecimalTotalCountStillRejectsCodeCount(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCountQuestion: true,
			},
		}},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"count answer must remain an integer",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"total_count",
			"label":"source files",
			"value":"3.5",
			"unit":"files",
			"provenance":"repo_map"
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Success {
		t.Fatalf("code count decimal should remain a structural error, got success: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "non-integer") {
		t.Fatalf("rejection should explain count integer constraint: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_RuntimeAggregateFactsOverLimitCompacts(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:    types.IntentRootCause,
			Scenario:  types.ScenarioRootCause,
			LogTriage: &types.LogBundle{Errors: []types.LogError{{Type: "runtime trace"}}},
		}},
	}
	tool := &EmitInvestigationComplete{}
	facts := make([]map[string]any, 0, 17)
	for i := 0; i < 17; i++ {
		role := "audit_ledger"
		if i == 0 {
			role = "principal_answer"
		}
		facts = append(facts, map[string]any{
			"kind":       "scalar_value",
			"label":      fmt.Sprintf("runtime fact %02d", i),
			"value":      fmt.Sprintf("%d", i),
			"unit":       "ms",
			"role":       role,
			"provenance": "trace_query.window_stats",
		})
	}
	payload := map[string]any{
		"reason":          "runtime facts collected",
		"confidence":      "high",
		"result_kind":     "resolved",
		"aggregate_facts": facts,
	}
	params, _ := json.Marshal(payload)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("runtime aggregate over-limit should compact instead of forcing retry: %s", res.Summary)
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) != 16 {
		t.Fatalf("expected compacted 16 facts, got %d: %+v", len(got), got)
	}
	if got[0].Label != "runtime fact 00" {
		t.Fatalf("principal fact should be preserved first, got %+v", got[0])
	}
	if !strings.Contains(res.Summary, "compacted from 17 to 16") {
		t.Fatalf("summary should disclose compaction: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_DecoratedRuntimeMemberSetDowngradesForCurrentVerification(t *testing.T) {
	logBundle := &types.LogBundle{
		Errors: []types.LogError{{Type: "panic"}},
	}
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentRootCause,
			DiagnosticProfile: types.DiagnosticIntentProfile{
				IsDiagnostic:        true,
				CurrentVersionCheck: true,
			},
			AnalyzerHints: types.AnalyzerHints{
				RequiredFileHints: []types.RequiredFileHint{{Path: "src/cart/Cart.cj"}},
			},
			LogTriage: logBundle,
		}},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"current code verification requires source support",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"call_chain",
				"value":"1",
				"members":["Cart.itemAt (origin)"]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("current-code verification support_refs debt should downgrade instead of tool-failing: %s", res.Summary)
	}
	if mut.IsInvestigationComplete() {
		t.Fatalf("first completion-form downgrade must not mark investigation complete")
	}
	if !strings.Contains(res.Summary, EmitInvestigationCompleteDowngradePrefix) ||
		!strings.Contains(res.Summary, "support_refs is empty") {
		t.Fatalf("summary should preserve support_refs contract as downgrade, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_CompletionFormSupportRefsConvergesWithCaveat(t *testing.T) {
	logBundle := &types.LogBundle{
		Errors: []types.LogError{{Type: "panic"}},
	}
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentRootCause,
			DiagnosticProfile: types.DiagnosticIntentProfile{
				IsDiagnostic:        true,
				CurrentVersionCheck: true,
			},
			AnalyzerHints: types.AnalyzerHints{
				RequiredFileHints: []types.RequiredFileHint{{Path: "src/cart/Cart.cj"}},
			},
			LogTriage: logBundle,
		}},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"current code verification has enough substantive evidence but the handoff uses decorated labels",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"call_chain",
				"value":"1",
				"members":["Cart.itemAt (origin)"]
			}
		]
	}`)

	for i := 1; i <= 2; i++ {
		res, err := tool.Execute(bus, params)
		if err != nil {
			t.Fatalf("attempt %d unexpected error: %v", i, err)
		}
		if !res.Success || !strings.Contains(res.Summary, EmitInvestigationCompleteDowngradePrefix) {
			t.Fatalf("attempt %d should be a tool-success downgrade, got success=%t summary=%s", i, res.Success, res.Summary)
		}
		if mut.IsInvestigationComplete() {
			t.Fatalf("attempt %d must not complete before convergence", i)
		}
	}

	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("third attempt unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("third attempt should complete with caveat, got: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("completion-form convergence should mark investigation complete")
	}
	caveats := mut.EvidenceClosure().CompletionCaveats()
	if len(caveats) != 1 || caveats[0].Lane != types.DowngradeLaneCompletionForm {
		t.Fatalf("completion caveat = %+v, want completion_form lane", caveats)
	}
	if got := mut.StableInvestigationAggregateFacts(); len(got) != 1 || got[0].Members[0] != "Cart.itemAt (origin)" {
		t.Fatalf("substantive aggregate handoff should be retained under caveat, got %+v", got)
	}
}

func TestPreCompleteDowngradeCompletionFormConvergesAcrossFormDebtChurn(t *testing.T) {
	mut := types.NewMutableState("completion form churn")
	bus := &types.BusContext{Mutable: mut}
	reasons := []string{"member_set_value", "member_set_support_refs", "aggregate_shape"}
	for i, reason := range reasons {
		queueCompletionFormRepair(bus, reason, "completion form detail changed")
		converged := preCompleteDowngradeConverges(bus, types.DowngradeLaneCompletionForm)
		if i < len(reasons)-1 && converged {
			t.Fatalf("attempt %d should still request local form repair", i+1)
		}
		if i == len(reasons)-1 && !converged {
			t.Fatalf("completion-form lane churn should converge on attempt %d", i+1)
		}
	}
	caveats := mut.EvidenceClosure().CompletionCaveats()
	if len(caveats) != 1 || caveats[0].Lane != types.DowngradeLaneCompletionForm {
		t.Fatalf("completion-form churn caveat = %+v", caveats)
	}
}

func TestEmitInvestigationComplete_AllowsDecoratedExternalOriginMemberSet(t *testing.T) {
	for _, origin := range []types.AnswerEvidenceOrigin{
		types.AnswerEvidenceOriginCommandMeasurement,
		types.AnswerEvidenceOriginCrossRepoIndex,
		types.AnswerEvidenceOriginExternalDocument,
		types.AnswerEvidenceOriginWebPage,
		types.AnswerEvidenceOriginMCPResource,
		types.AnswerEvidenceOriginConnectorResource,
	} {
		mut := types.NewMutableState("q")
		bus := &types.BusContext{Mutable: mut}
		tool := &EmitInvestigationComplete{}
		params := json.RawMessage(fmt.Sprintf(`{
			"reason":"external observation rows collected",
			"confidence":"high",
			"result_kind":"resolved",
			"aggregate_facts":[
				{
					"kind":"member_set",
					"label":"external rows",
					"value":"1",
					"role":"principal_answer",
					"dimensions":[{"name":"origin","value":%q}],
					"members":["ServiceA (observed externally)"]
				}
			]
		}`, string(origin)))
		res, err := tool.Execute(bus, params)
		if err != nil {
			t.Fatalf("origin %q: unexpected error: %v", origin, err)
		}
		if !res.Success {
			t.Fatalf("origin %q decorated member_set should rely on origin-specific provenance: %s", origin, res.Summary)
		}
	}
}

func TestAppendDeterministicCountAggregateFactTagsUnifiedOrigin(t *testing.T) {
	execFacts := appendDeterministicCountAggregateFact(nil, 7, "count", "system", "exec_command", nil)
	if len(execFacts) != 1 {
		t.Fatalf("expected one fact, got %+v", execFacts)
	}
	assertAggregateDimension(t, execFacts[0], "origin", string(types.AnswerEvidenceOriginCommandMeasurement))
	assertAggregateDimension(t, execFacts[0], "proof_source", "exec_command")
	if origins := types.AnswerAggregateFactEvidenceOrigins(execFacts[0], &types.RequestModel{
		Predicates: types.SemanticPredicates{IsCountQuestion: true, IsScalarAnswer: true},
	}); !answerOriginsContain(origins, types.AnswerEvidenceOriginCommandMeasurement) {
		t.Fatalf("exec count should project command_measurement origin, got %+v", origins)
	}

	gitFacts := appendDeterministicCountAggregateFact(nil, 2, "history count", "system", "git_history_search", []types.AnswerAggregateDimension{
		{Name: "measurement_kind", Value: "vcs_history_count"},
	})
	if len(gitFacts) != 1 {
		t.Fatalf("expected one fact, got %+v", gitFacts)
	}
	assertAggregateDimension(t, gitFacts[0], "origin", string(types.AnswerEvidenceOriginVCSMetadata))
	assertAggregateDimension(t, gitFacts[0], "proof_source", "git_history_search")
	if origins := types.AnswerAggregateFactEvidenceOrigins(gitFacts[0], &types.RequestModel{
		Predicates: types.SemanticPredicates{IsHistoryLookup: true, IsCountQuestion: true, IsScalarAnswer: true},
	}); !answerOriginsContain(origins, types.AnswerEvidenceOriginVCSMetadata) {
		t.Fatalf("git history count should project VCS metadata origin, got %+v", origins)
	}
}

func TestDeterministicCountAggregateFactMeasurementSurfaceRequiresTypedOrigin(t *testing.T) {
	commandShapedProse := types.AnswerAggregateFact{
		Kind:  types.AnswerAggregateScalar,
		Value: "7",
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "proof_source", Value: "find . -name '*.go' | wc -l"},
		},
	}
	if deterministicCountAggregateFactHasCommandMeasurementSurface(commandShapedProse) {
		t.Fatalf("command-shaped model field text must not be accepted as command measurement surface")
	}

	for _, fact := range []types.AnswerAggregateFact{
		{
			Kind:  types.AnswerAggregateScalar,
			Value: "7",
			Dimensions: []types.AnswerAggregateDimension{
				{Name: "origin", Value: string(types.AnswerEvidenceOriginCommandMeasurement)},
			},
		},
		{
			Kind:  types.AnswerAggregateScalar,
			Value: "7",
			Dimensions: []types.AnswerAggregateDimension{
				{Name: "proof_source", Value: "exec_command"},
			},
		},
	} {
		if !deterministicCountAggregateFactHasCommandMeasurementSurface(fact) {
			t.Fatalf("typed command measurement token should be accepted: %+v", fact)
		}
	}
}

func TestEmitInvestigationComplete_ReconcilesSinglePrincipalCountWithCommandMeasurement(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "exec_command",
		Success:  true,
		Summary:  "[exec_command: $ find internal/tool -name \"*.go\" ! -name \"*_test.go\" | wc -l]\n[exec_command: evidence_origin=command_measurement measurement=count]\n     140\n",
		CommandMeasurement: &types.ToolCommandMeasurement{
			Kind:        types.ToolCommandMeasurementKindCount,
			Value:       140,
			Origin:      types.AnswerEvidenceOriginCommandMeasurement,
			ProofSource: "exec_command",
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Predicates: types.SemanticPredicates{IsCountQuestion: true, IsScalarAnswer: true},
		}},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"count collected from a command",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"total_count",
			"label":"non-test source files",
			"value":"13",
			"role":"principal_answer",
			"dimensions":[
				{"name":"scope","value":"internal/tool"},
				{"name":"proof_source","value":"exec_command"}
			]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected corrected completion to succeed: %s", res.Summary)
	}
	if !strings.Contains(res.Summary, "13->140") {
		t.Fatalf("normalization note should disclose command reconciliation, got: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 {
		t.Fatalf("stable aggregate facts = %+v", facts)
	}
	if facts[0].Value != "140" {
		t.Fatalf("aggregate value = %q, want 140", facts[0].Value)
	}
	assertAggregateDimension(t, facts[0], "origin", string(types.AnswerEvidenceOriginCommandMeasurement))
	assertAggregateDimension(t, facts[0], "answer_axis", "count")
}

func TestEmitInvestigationComplete_DoesNotReconcileSummaryOnlyCommandMeasurement(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "exec_command",
		Success:  true,
		Summary:  "[exec_command: $ find internal/tool -name \"*.go\" ! -name \"*_test.go\" | wc -l]\n[exec_command: evidence_origin=command_measurement measurement=count]\n     140\n",
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Predicates: types.SemanticPredicates{IsCountQuestion: true, IsScalarAnswer: true},
		}},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"count collected from a command",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"total_count",
			"label":"non-test source files",
			"value":"13",
			"role":"principal_answer",
			"dimensions":[
				{"name":"scope","value":"internal/tool"},
				{"name":"proof_source","value":"exec_command"}
			]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("summary-only command output should not break completion: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 || facts[0].Value != "13" {
		t.Fatalf("summary-only command output must not rewrite model count, got %+v", facts)
	}
}

func TestEmitInvestigationComplete_DoesNotReconcileAmbiguousGroupedCounts(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendDispatchToolResult(types.ToolResult{
		ToolName: "exec_command",
		Success:  true,
		Summary:  "[exec_command: $ find . -name '*.go' | wc -l]\n[exec_command: evidence_origin=command_measurement measurement=count]\n10\n",
		CommandMeasurement: &types.ToolCommandMeasurement{
			Kind:        types.ToolCommandMeasurementKindCount,
			Value:       10,
			Origin:      types.AnswerEvidenceOriginCommandMeasurement,
			ProofSource: "exec_command",
		},
	})
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Predicates: types.SemanticPredicates{IsCountQuestion: true, IsScalarAnswer: true},
		}},
	}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"group buckets are separate from the shell total",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[{
			"kind":"grouped_count",
			"label":"category A",
			"value":"4",
			"role":"principal_answer",
			"dimensions":[{"name":"group","value":"A"}]
		}]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected grouped count completion to succeed: %s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 || facts[0].Value != "4" {
		t.Fatalf("grouped count should not be rewritten by shell total, got %+v", facts)
	}
}

func TestDeterministicHistoryCountExecCommandUsesVCSOrigin(t *testing.T) {
	measurement := execCommandMeasurement(
		"git -C . log --format=%H -20 -- internal/orchestrator | awk 'END { print \"answer_count=3\" }'",
		"answer_count=3\n",
	)
	if measurement == nil || !measurement.History || measurement.Value != 3 || measurement.ProofSource != "exec_command_git_history" {
		t.Fatalf("git history command should publish typed history measurement, got %+v", measurement)
	}
	if got := deterministicCountAggregateOrigin("exec_command_git_history"); got != string(types.AnswerEvidenceOriginVCSMetadata) {
		t.Fatalf("exec git history origin = %q, want %q", got, types.AnswerEvidenceOriginVCSMetadata)
	}
}

func assertAggregateDimension(t *testing.T, fact types.AnswerAggregateFact, name, value string) {
	t.Helper()
	for _, dim := range fact.Dimensions {
		if strings.EqualFold(dim.Name, name) && strings.EqualFold(dim.Value, value) {
			return
		}
	}
	t.Fatalf("dimension %s=%s missing from %+v", name, value, fact.Dimensions)
}

func answerOriginsContain(origins []types.AnswerEvidenceOrigin, want types.AnswerEvidenceOrigin) bool {
	for _, origin := range origins {
		if origin == want {
			return true
		}
	}
	return false
}

// TestEmitInvestigationComplete_AcceptsBareCodeMemberSetWithoutSupportRefs
// is the negative-control: bare code-identity members (no decorator)
// can still be auto-grounded by the enrichment pass against verbatim
// evidence anchors. The narrowed B check fires only on decorated
// members so callers and tests with bare-symbol member_sets keep
// working without source-level changes.
func TestEmitInvestigationComplete_AcceptsBareCodeMemberSetWithoutSupportRefs(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"plain symbol enum collected",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"read mode stages",
				"value":"2",
				"members":["StageAnalyze","StageExplore"]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("bare-symbol member_set without support_refs should pass through B: %s", res.Summary)
	}
}

// TestEmitInvestigationComplete_AcceptsDecoratedMemberSetWithSupportRefs
// is the happy path: when the model attaches support_refs alongside
// decorated members, the B gate is satisfied and the emit proceeds.
func TestEmitInvestigationComplete_AcceptsDecoratedMemberSetWithSupportRefs(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"comparison with explicit per-member grounding",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"codrax 防幻觉组件",
				"value":"2",
				"members":[
					"Gate.Run (8个独立检查)",
					"Orchestrator (4阶段管道整合)"
				],
				"support_refs":[
					"Gate.Run: internal/analysis/gate/gate.go:128",
					"Orchestrator: internal/orchestrator/orchestrator.go:42"
				]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("decorated member_set WITH support_refs should pass: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_RepairsSwappedDecoratedSupportRefsFromReadFile(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendDispatchToolResult(readFileRawRefResultForTest(t,
		"internal/llm/stream_errors.go",
		35,
		120,
		"type StreamStalledError struct {",
	))
	mut.AppendDispatchToolResult(readFileRawRefResultForTest(t,
		"internal/llm/stream_errors.go",
		97,
		120,
		"type StreamFirstByteTimeoutError struct {",
	))
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}
	params := json.RawMessage(`{
		"reason":"stream timeout variants are grounded",
		"confidence":"high",
		"result_kind":"resolved",
		"aggregate_facts":[
			{
				"kind":"member_set",
				"label":"模型响应超时信号类型",
				"value":"2",
				"role":"principal_answer",
				"members":[
					"StreamFirstByteTimeoutError (首字节超时)",
					"StreamStalledError (流停滞)"
				],
				"support_refs":[
					"internal/llm/stream_errors.go:35",
					"internal/llm/stream_errors.go:97"
				]
			}
		]
	}`)
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("swapped support_refs should be repaired without model retry: %s", res.Summary)
	}
	if !mut.IsInvestigationComplete() {
		t.Fatalf("repaired completion should mark investigation complete, summary=%s", res.Summary)
	}
	facts := mut.StableInvestigationAggregateFacts()
	if len(facts) != 1 {
		t.Fatalf("aggregate facts = %+v, want one", facts)
	}
	want := []string{
		"internal/llm/stream_errors.go:97",
		"internal/llm/stream_errors.go:35",
	}
	if !reflect.DeepEqual(facts[0].SupportRefs, want) {
		t.Fatalf("support_refs = %+v, want %+v", facts[0].SupportRefs, want)
	}
}

func TestCompletionAggregateSupportRepair_EmbeddedSourceLocationsFromReadFile(t *testing.T) {
	mut := types.NewMutableState("trace current-source mechanism")
	mut.AppendDispatchToolResult(readFileRawRefResultForTest(t,
		"internal/tracequery/parse.go",
		22,
		2296,
		"var ftraceLineRE = regexp.MustCompile(`trace row`)",
	))
	mut.AppendDispatchToolResult(readFileRawRefResultForTest(t,
		"internal/tracequery/flavor.go",
		25,
		286,
		"func flavorVote(line string) traceFlavor {",
	))
	mut.AppendDispatchToolResult(readFileRawRefResultForTest(t,
		"internal/tracequery/query.go",
		4254,
		4428,
		"func computeTraceMarks(events []Event) []Span {",
	))
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Scenario: types.ScenarioArchitectureExplain,
			CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
				IsCurrentSourceExplanationRequested: true,
				Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationExplainCurrentMechanism},
				SourceQuotes:                        []string{"结合当前源码解释"},
				Confidence:                          0.9,
			},
		}},
	}
	facts := enrichCompletionAggregateFactsWithMemberSupport(bus, []types.AnswerAggregateFact{{
		Kind:       types.AnswerAggregateMemberSet,
		Label:      "parse layers for HiTrace span",
		Value:      "3",
		Role:       types.AnswerAggregateRoleSupportingCoverage,
		Provenance: "demoted:runtime_observation_advisory_aggregate",
		Dimensions: []types.AnswerAggregateDimension{{Name: "origin", Value: "runtime_artifact"}},
		Members: []string{
			"ftraceLineRE 行解析 parse.go:22",
			"flavorVote 特征检测 flavor.go:25-49",
			"B|E 配对 + duration 计算 query.go:4254-4267",
		},
	}})
	if len(facts) != 1 {
		t.Fatalf("aggregate facts = %+v, want one", facts)
	}
	wantRefs := []string{
		"internal/tracequery/parse.go:22",
		"internal/tracequery/flavor.go:25",
		"internal/tracequery/query.go:4254",
	}
	if !reflect.DeepEqual(facts[0].SupportRefs, wantRefs) {
		t.Fatalf("support_refs = %+v, want %+v", facts[0].SupportRefs, wantRefs)
	}
	origins := types.AnswerAggregateFactEvidenceOrigins(facts[0], &bus.AnalysisIR.RequestModel)
	if !answerOriginsContain(origins, types.AnswerEvidenceOriginRuntimeArtifact) ||
		!answerOriginsContain(origins, types.AnswerEvidenceOriginCurrentSource) {
		t.Fatalf("repaired aggregate should carry runtime and current-source origins, got %+v", origins)
	}
}
