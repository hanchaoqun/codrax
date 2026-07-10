package types

// CSR #64 (2026-07-05) — current-source qualification rulings, two pins.
//
// Ruling 1 (mixed-run terminal fallback): the ledger-input RequestModel now
// goes through the SAME bundle-backfill helper as the authority snapshot
// (RuntimeSourceAuthorityRequestModelFrom*Context). Before the fix the two
// faces of ONE run disagreed on the source lane (ledger RM: PerfTrace=nil →
// lane=required; authority RM: backfilled → lane=allowed_optional) and every
// model aggregate fact with no origin-lane hit fell through to the terminal
// current_source fallback — in a mixed attached-artifact run the unverified
// model facts fake-satisfied the current-source lane (CurrentSourceSatisfied
// from model claims with zero landed reads; the mixed-run face of the CSP #63
// pollution class). Probed consumption face before the fix was chosen
// (2026-07-05): in the external-bundle mixed shape the reclassification is
// loosening-only (hardBlock false→false, allowsProceed true→true, keeps
// true→false, allows false→true); the strict flip (CanHardBlockCompletion
// false→true) is confined to runs with a PRECISE current-source ask where
// zero genuine source evidence landed — exactly the shape where the contract
// designed the block, with the required-file-hint counterfactual and soft
// downgrade as typed escape lanes.
//
// Ruling 2 (exclude-run negative search): under the typed
// explicit-user-exclusion boundary a negative repo search stays in the ledger
// as an advisory observation, but its SourceRef must not be qualified into the
// current-source proof lane (ObservationSourceKindForOrigin maps
// repo_negative_search → current_source, which set CurrentSourceSatisfied in
// excluded-lane runs). Non-exclude runs keep the historical current_source
// kind byte-stable — the negative search remains the legitimate satisfaction
// valve for "asked about a symbol that does not exist" questions, so an
// over-widened requalification flips the non-exclude pins red.

import "testing"

func csrMixedRunFacts() []AnswerAggregateFact {
	return []AnswerAggregateFact{
		{Kind: AnswerAggregateScalar, Label: "帧窗口总时长", Value: "114.94 ms", Role: AnswerAggregateRoleSupportingCoverage},
		{Kind: AnswerAggregateMemberSet, Label: "主线程唤醒链节点", Value: "2", Role: AnswerAggregateRolePrincipalAnswer,
			Members: []string{"ThreadPoolForeg-60555", "NetworkService-60595"}},
		{Kind: AnswerAggregateScalar, Label: "主线程最大单次唤醒延迟", Value: "11.103 ms", Role: AnswerAggregateRolePrincipalAnswer},
		{Kind: AnswerAggregateBucketCount, Label: "主线程被唤醒次数", Value: "34", Role: AnswerAggregateRoleSupportingCoverage},
	}
}

func csrExternalOnlyPerfBundle() *PerfBundle {
	return &PerfBundle{
		Frames: []PerfFrame{{FrameNo: 1, DurationMs: 32.5, Janky: true}},
		// ResolvedFiles deliberately empty: IsExternalSource() == true.
	}
}

// csrMixedRunBus mirrors the specimen shape: the analyzer RM carries NO
// bundle (PerfTrace only lives in Mutable) and NO external-observation
// policy — a mixed run without any exclude boundary.
func csrMixedRunBus(perf *PerfBundle, facts []AnswerAggregateFact, evidence []EvidenceItem) *BusContext {
	mut := NewMutableState("为什么这一帧超时了")
	mut.SetPerfTrace(perf)
	mut.SetInvestigationAggregateFacts(facts)
	mut.RetainInvestigationAggregateFacts()
	return &BusContext{
		Mutable: mut,
		AnalysisIR: &AnalysisIR{
			RequestModel: RequestModel{Intent: IntentRootCause, Scenario: ScenarioRootCause},
		},
		EvidenceItems: evidence,
	}
}

func csrCountLedgerLanes(ledger ObservationLedger) (currentSource, runtime int) {
	for _, r := range ledger.Records {
		if r.Origin == AnswerEvidenceOriginCurrentSource ||
			r.SourceRef.Kind == ObservationSourceCurrentSource {
			currentSource++
		}
		if r.Origin == AnswerEvidenceOriginRuntimeArtifact {
			runtime++
		}
	}
	return
}

// Ruling 1 pin (bus face + split-brain closure): in a mixed run whose attached
// bundle is external-source, model aggregate facts classify runtime_artifact
// on the ledger face — never current_source — and the authority stops being
// fake-satisfied. The ledger-input RM and the authority RM must agree on the
// source lane (they are the same helper now).
func TestObservationLedgerInputFromBusContext_MixedExternalBundleFactsNeverCurrentSource(t *testing.T) {
	facts := csrMixedRunFacts()
	bus := csrMixedRunBus(csrExternalOnlyPerfBundle(), facts, nil)

	input := ObservationLedgerInputFromBusContext(bus, 64)
	if input.RequestModel == nil || input.RequestModel.PerfTrace == nil {
		t.Fatalf("ledger-input RM must carry the backfilled bundle (CSR #64 ruling 1): %+v", input.RequestModel)
	}
	authorityRM := RuntimeSourceAuthorityRequestModelFromBusContext(bus)
	if got, want := input.RequestModel.CurrentSourceLaneDecision(), authorityRM.CurrentSourceLaneDecision(); got != want {
		t.Fatalf("ledger face and authority face disagree on the source lane again: ledger=%q authority=%q", got, want)
	}

	ledger := CompileObservationLedger(input)
	cs, runtime := csrCountLedgerLanes(ledger)
	if cs != 0 {
		t.Fatalf("mixed external-bundle run stamped %d model fact(s) current_source (CSR #64 ruling 1 regression): %+v", cs, ledger.Records)
	}
	if runtime != len(facts)+1 { // facts + the perf frame record
		t.Fatalf("facts must stay in the ledger as runtime observations: got %d, want %d", runtime, len(facts)+1)
	}

	authority := BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(bus, ObservationLedger{})
	if authority.CurrentSourceRecordCount != 0 || authority.CurrentSourceSatisfied {
		t.Fatalf("model facts fake-satisfied the source lane again: %+v", authority)
	}
	if authority.KeepsCurrentSourceLaneLoadBearing() {
		t.Fatalf("polluted satisfied flag kept the source lane load-bearing: %+v", authority)
	}
	if !authority.AllowsRuntimeEvidenceWithoutCurrentSource() {
		t.Fatalf("runtime-only lane must open on truthful classification: %+v", authority)
	}
}

// Ruling 1 pin (agent-context twin): both builders route through the shared
// backfill helper; reverting only one of them flips this pin red.
func TestObservationLedgerInputFromAgentContext_MixedExternalBundleFactsNeverCurrentSource(t *testing.T) {
	facts := csrMixedRunFacts()
	mut := NewMutableState("为什么这一帧超时了")
	mut.SetPerfTrace(csrExternalOnlyPerfBundle())
	mut.SetInvestigationAggregateFacts(facts)
	mut.RetainInvestigationAggregateFacts()
	ctx := &AgentContext{
		Mutable: mut,
		AnalysisIR: &AnalysisIR{
			RequestModel: RequestModel{Intent: IntentRootCause, Scenario: ScenarioRootCause},
		},
	}

	input := ObservationLedgerInputFromAgentContext(ctx, 64)
	if input.RequestModel == nil || input.RequestModel.PerfTrace == nil {
		t.Fatalf("agent ledger-input RM must carry the backfilled bundle (CSR #64 ruling 1): %+v", input.RequestModel)
	}
	cs, runtime := csrCountLedgerLanes(CompileObservationLedger(input))
	if cs != 0 || runtime != len(facts)+1 {
		t.Fatalf("agent-context ledger lanes wrong: currentSource=%d runtime=%d, want 0/%d", cs, runtime, len(facts)+1)
	}
}

// Ruling 1 negative control — EVOLUTION RECORD (CSP-RM, §29.21 ruling
// 2026-07-10): this pin used to freeze the RESOLVED-bundle mixed run on the
// historical terminal current_source fallback (satisfied=true from bare model
// facts). §29.21 retired that fallback in every run shape: satisfied now only
// comes from deterministic tool witnesses, so the resolved-bundle shape with
// ZERO landed reads honestly reports satisfied=false. What this pin still
// guards is the CSR #64 ruling-1 boundary in its evolved form: a RESOLVED
// (non-external) bundle must NOT classify the model facts runtime_artifact —
// they are bare model claims (system_inference), not external restatements.
// An over-widened external reclassification flips THIS pin red while the
// external-shape pin stays green. Note: this shape keeps a PRECISE
// current-source requirement (PerfTrace.ResolvedFiles), so the source lane
// stays load-bearing through the requirement itself — not through fake
// satisfaction (the run is pushed toward真读, the anti-hallucination
// direction).
func TestObservationLedgerInputFromBusContext_ResolvedBundleFactsAreModelClaimsNotRuntime(t *testing.T) {
	facts := csrMixedRunFacts()
	perf := csrExternalOnlyPerfBundle()
	perf.ResolvedFiles = []string{"internal/render/frame.go"}
	bus := csrMixedRunBus(perf, facts, nil)

	ledger := CompileObservationLedger(ObservationLedgerInputFromBusContext(bus, 64))
	cs, runtime := csrCountLedgerLanes(ledger)
	if cs != 0 {
		t.Fatalf("resolved-bundle model facts re-entered the current-source lane (CSP-RM regression): got %d; records=%+v", cs, ledger.Records)
	}
	// The perf frame record stays runtime; the model facts must NOT join it
	// (resolved bundle ⇒ non-external ⇒ facts are bare model claims).
	if runtime != 1 {
		t.Fatalf("resolved-bundle facts must not classify runtime_artifact (ruling-1 boundary): got %d, want 1; records=%+v", runtime, ledger.Records)
	}
	modelClaims := 0
	for _, r := range ledger.Records {
		if r.Origin == AnswerEvidenceOriginSystemInference && r.SourceRef.Kind == ObservationSourceModelClaim {
			modelClaims++
		}
	}
	if modelClaims != len(facts) {
		t.Fatalf("resolved-bundle model facts must stay lossless on the advisory lane: got %d, want %d", modelClaims, len(facts))
	}
	authority := BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(bus, ObservationLedger{})
	if authority.CurrentSourceSatisfied {
		t.Fatalf("bare model facts still fake-satisfy the resolved-bundle source lane (CSP-RM regression): %+v", authority)
	}
	if authority.CurrentSourceRequirement != RuntimeSourceRequirementPrecise {
		t.Fatalf("resolved-bundle precise requirement lost (fixture drift): %+v", authority)
	}
	if !authority.KeepsCurrentSourceLaneLoadBearing() {
		t.Fatalf("resolved-bundle source lane must stay load-bearing through the PRECISE requirement: %+v", authority)
	}
}

// Ruling 1 supply pin: one genuine landed read keeps the completion supply —
// the satisfied flag survives through the evidence lane, so mixed runs that
// actually read source are untouched by the reclassification.
func TestObservationLedgerInputFromBusContext_MixedExternalBundleRealEvidenceKeepsSatisfied(t *testing.T) {
	evidence := []EvidenceItem{{
		ID:        "real-src",
		Source:    "internal/render/frame.go",
		LineStart: 42,
		LineEnd:   48,
		Summary:   "frame budget check",
		Salience:  SalienceLoadBearing,
	}}
	bus := csrMixedRunBus(csrExternalOnlyPerfBundle(), csrMixedRunFacts(), evidence)

	authority := BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(bus, ObservationLedger{})
	if !authority.CurrentSourceSatisfied || authority.CurrentSourceRecordCount != 1 {
		t.Fatalf("genuine evidence must keep the source lane satisfied: %+v", authority)
	}
	if authority.ExactCurrentSourceSupportCount != 1 {
		t.Fatalf("landed read lost its exact-support qualification: %+v", authority)
	}
}

func csrNegativeSearchFact() AnswerAggregateFact {
	return AnswerAggregateFact{
		Kind:  AnswerAggregateNegativeSearch,
		Label: "仓库中不存在 FooBar 符号",
		Value: "0",
		Dimensions: []AnswerAggregateDimension{
			{Name: "pattern", Value: "FooBar"},
			{Name: "scope", Value: "internal/"},
		},
	}
}

// Ruling 2 pin: in a typed-exclude run the negative-search record is retained
// (advisory, lossless) but its SourceRef leaves the current-source proof lane,
// so it can never set CurrentSourceSatisfied. The requalification sits at the
// single compile throat: a producer-emitted row with an explicit
// current_source kind is requalified the same way as the aggregate-fact
// default.
func TestCompileObservationLedger_ExcludeNegativeSearchNeverCurrentSourceKind(t *testing.T) {
	rm := cspTypedExcludeRequestModel()
	ledger := CompileObservationLedger(ObservationLedgerInput{
		AggregateFacts: []AnswerAggregateFact{csrNegativeSearchFact()},
		ToolResults: []ToolResult{{
			ToolName: "grep",
			Success:  true,
			Observations: []ObservationRecord{{
				ID:        "neg-row",
				Origin:    AnswerEvidenceOriginRepoNegativeSearch,
				SourceRef: ObservationSourceRef{Kind: ObservationSourceCurrentSource, Path: "internal/"},
				Summary:   "no FooBar match",
				Negative:  true,
			}},
		}},
		RequestModel: &rm,
	})

	negative := 0
	for _, record := range ledger.Records {
		if record.Origin != AnswerEvidenceOriginRepoNegativeSearch {
			continue
		}
		negative++
		if record.SourceRef.Kind == ObservationSourceCurrentSource {
			t.Fatalf("exclude-run negative search re-entered the current-source lane (CSR #64 ruling 2 regression): %+v", record)
		}
		if record.SourceRef.Kind != ObservationSourceCommand {
			t.Fatalf("exclude-run negative search must requalify as a command observation: %+v", record)
		}
	}
	if negative != 2 {
		t.Fatalf("negative-search records must stay in the ledger (advisory, lossless): got %d, want 2; records=%+v", negative, ledger.Records)
	}

	authority := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		RequestModel: &rm,
		Ledger:       ledger,
	})
	if authority.CurrentSourceRecordCount != 0 || authority.CurrentSourceSatisfied {
		t.Fatalf("exclude-run negative search still feeds satisfied (CSR #64 ruling 2 regression): %+v", authority)
	}
	if authority.KeepsCurrentSourceLaneLoadBearing() {
		t.Fatalf("excluded lane kept load-bearing by a negative search: %+v", authority)
	}
}

// Ruling 2 mutation pin (non-exclude side): without the exclude boundary the
// negative search keeps its historical current_source qualification and stays
// the satisfaction valve — an over-widened requalification flips this red
// while the exclude pin stays green.
//
// EVOLUTION RECORD (CSP-RM, §29.21 ruling 2026-07-10): reviewed against the
// plain-fallback retirement and kept BYTE-STABLE on purpose. §29.21 ranks the
// lanes explicitly: satisfied only accepts deterministic tool witnesses
// (read_file coverage / grep / trace_query typed observations), and a
// negative repo search is a grep-family witness — "模型断言比负向搜索更弱,
// 不应享更高定性" is the ruling's own ordering. The pollution class the
// ruling closed is the TERMINAL fallback for bare model claims (now
// system_inference, see csp_current_source_exclude_test.go); the negative
// search valve for "asked about a symbol that does not exist" questions is
// the legitimate satisfaction path and survives unchanged.
func TestCompileObservationLedger_PlainNegativeSearchKeepsCurrentSourceKind(t *testing.T) {
	plainRM := RequestModel{Intent: IntentRootCause}
	ledger := CompileObservationLedger(ObservationLedgerInput{
		AggregateFacts: []AnswerAggregateFact{csrNegativeSearchFact()},
		RequestModel:   &plainRM,
	})
	record := findObservationRecord(t, ledger, "aggregate:0#repo_negative_search")
	if record.SourceRef.Kind != ObservationSourceCurrentSource {
		t.Fatalf("non-exclude negative search lost its historical current_source kind: %+v", record)
	}
	authority := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		RequestModel: &plainRM,
		Ledger:       ledger,
	})
	if !authority.CurrentSourceSatisfied || authority.CurrentSourceRecordCount != 1 {
		t.Fatalf("non-exclude negative-search satisfaction valve changed: %+v", authority)
	}
}
