package types

// CSP #63 (2026-07-05) — CurrentSourceSatisfied pollution root fix pins.
//
// Specimen: eval/results/trace_query_donghu_real_frame_multicausal-20260703-111818
// ("只分析这份 trace，不分析代码", typed ExcludesCurrentSource). The explorer made
// only trace_query calls (0 evidence, 0 readFiles) yet the authority logged
// `current_source_lane=excluded ... current_source_satisfied=true source=10`.
// Attribution: each of the 10 model-emitted aggregate facts from
// emit_investigation_complete compiled into one current-source ledger record —
// AnswerAggregateFactEvidenceOrigins returned empty (the ledger-input
// RequestModel copy lacks the Mutable perf bundle, so
// HasExternalOnlyRuntimeArtifact()==false) and the terminal kind-shaped
// fallback stamped AnswerEvidenceOriginCurrentSource. CurrentSourceRecordCount
// then set CurrentSourceSatisfied (runtime_source_answer_authority_view.go:181),
// KeepsCurrentSourceLaneLoadBearing vetoed runtime citation cleanup, and 4
// blob-path pseudo-citations rendered as a source bibliography (CPD #58 added
// the display-layer arm; this file pins the producer-side root fix).
//
// Pinned here:
//  1. Producer zero-emission: under the typed explicit-user-exclusion boundary,
//     model-authored aggregate facts NEVER project onto the current-source
//     evidence lane — not via the terminal fallback, and not via a
//     model-emitted origin dimension token (user boundary outranks model
//     claims). They classify as runtime-artifact observations instead (no
//     ledger data loss).
//  2. Mutation pin (EVOLVED by CSP-RM, §29.21 ruling 2026-07-10): the same
//     facts WITHOUT the exclude boundary now classify system_inference — the
//     plain-run terminal current_source fallback was retired as the residual
//     satisfied-pollution source (cmp_792). The exclude/plain delta remains
//     pinned (runtime_artifact vs system_inference), so reverting the exclude
//     gate still flips pin 1 red while this pin stays green.
//  3. Authority projection on the donghu shape: zero current-source records,
//     CurrentSourceSatisfied=false, KeepsCurrentSourceLaneLoadBearing=false,
//     AllowsRuntimeEvidenceWithoutCurrentSource=true — the runtime citation
//     cleanup gate opens from the authority itself, making the CPD #58
//     display arm a redundant defense line for this shape (the arm stays; an
//     explicit user boundary must keep outranking derived authority).

import "testing"

func cspTypedExcludeRequestModel() RequestModel {
	return RequestModel{
		Intent: IntentRootCause,
		ExternalObservationPolicy: &ExternalObservationPolicy{
			CurrentSourceMode:    ExternalObservationCurrentSourceExclude,
			ExclusionKind:        ExternalObservationSourceExclusionExplicitUserBoundary,
			ArtifactCitationMode: ExternalObservationArtifactCitationExternalOnly,
			SourceQuotes:         []string{"只分析这份 trace，不分析代码"},
		},
	}
}

// cspDonghuAggregateFacts mirrors the specimen's emit_investigation_complete
// handoff shape: trace-derived scalar/member_set/bucket_count facts with no
// origin dimensions, no support refs, no provenance.
func cspDonghuAggregateFacts() []AnswerAggregateFact {
	return []AnswerAggregateFact{
		{Kind: AnswerAggregateScalar, Label: "帧窗口总时长", Value: "114.94 ms", Role: AnswerAggregateRoleSupportingCoverage},
		{Kind: AnswerAggregateMemberSet, Label: "主线程唤醒链节点", Value: "4", Role: AnswerAggregateRolePrincipalAnswer,
			Members: []string{"ThreadPoolForeg-60555", "NetworkService-60595", "CookieMonsterCl-59843", "com.baidu.tieba-59566"}},
		{Kind: AnswerAggregateScalar, Label: "主线程最大单次唤醒延迟", Value: "11.103 ms", Role: AnswerAggregateRolePrincipalAnswer},
		{Kind: AnswerAggregateScalar, Label: "CPU 0 runnable 等待总时长", Value: "389.746 ms", Role: AnswerAggregateRoleSupportingCoverage},
		{Kind: AnswerAggregateScalar, Label: "IO 最长延迟", Value: "110.660 ms", Role: AnswerAggregateRoleSupportingCoverage},
		{Kind: AnswerAggregateMemberSet, Label: "优先级反转候选", Value: "2", Role: AnswerAggregateRolePrincipalAnswer,
			Members: []string{"CookieMonsterCl-59843 (prio=20)", "NetworkService-60595 (prio=20)"}},
		{Kind: AnswerAggregateBucketCount, Label: "主线程在窗口内的状态段数量", Value: "12", Role: AnswerAggregateRoleSupportingCoverage},
		{Kind: AnswerAggregateScalar, Label: "ThreadPoolForeg-60555 D-sleep 时长", Value: "6.768 ms", Role: AnswerAggregateRoleSupportingCoverage},
		{Kind: AnswerAggregateScalar, Label: "主线程与 Binder 的同步 binder 等待", Value: "11.103 ms", Role: AnswerAggregateRoleSupportingCoverage},
		{Kind: AnswerAggregateBucketCount, Label: "主线程被唤醒次数", Value: "34", Role: AnswerAggregateRoleSupportingCoverage},
	}
}

// Pin 1 + 2: origin projection — typed exclude boundary keeps model facts out
// of the current-source lane; without the boundary the historical fallback is
// byte-stable.
func TestAnswerAggregateFactEvidenceOrigins_TypedExcludeNeverCurrentSource(t *testing.T) {
	excludeRM := cspTypedExcludeRequestModel()
	plainRM := RequestModel{Intent: IntentRootCause}

	for _, fact := range cspDonghuAggregateFacts() {
		origins := AnswerAggregateFactEvidenceOrigins(fact, &excludeRM)
		if len(origins) == 0 {
			t.Fatalf("exclude run must not drop fact %q from every origin lane", fact.Label)
		}
		for _, origin := range origins {
			if origin == AnswerEvidenceOriginCurrentSource {
				t.Fatalf("typed-exclude run stamped fact %q current_source (CSP #63 regression): %v", fact.Label, origins)
			}
		}
		if origins[0] != AnswerEvidenceOriginRuntimeArtifact {
			t.Fatalf("typed-exclude trace fact %q should classify runtime_artifact, got %v", fact.Label, origins)
		}

		// EVOLUTION RECORD (CSP-RM, §29.21 ruling 2026-07-10): this mutation
		// arm previously froze the plain-run terminal fallback at
		// [current_source] — the very byte-stability §29.20 flagged as the
		// residual satisfied-pollution source (cmp_792: three model facts →
		// satisfied=true → all three retry-suppression arms dead). The ruling
		// closed it: pure model claims now project onto the ADVISORY lane in
		// every run shape, so the plain side asserts [system_inference]. The
		// exclude/plain DELTA this arm guards is preserved: exclude runs
		// classify runtime_artifact (external restatement), plain runs
		// classify system_inference (bare model claim) — reverting the exclude
		// gate still flips pin 1 red while this stays green.
		plain := AnswerAggregateFactEvidenceOrigins(fact, &plainRM)
		if len(plain) != 1 || plain[0] != AnswerEvidenceOriginSystemInference {
			t.Fatalf("non-exclude model-claim fallback changed for fact %q: got %v, want [system_inference]", fact.Label, plain)
		}
	}

	// User boundary outranks a model-emitted origin dimension token.
	tokenFact := AnswerAggregateFact{
		Kind:  AnswerAggregateScalar,
		Label: "model claims current source",
		Value: "1",
		Dimensions: []AnswerAggregateDimension{
			{Name: "origin", Value: "current_source"},
		},
	}
	origins := AnswerAggregateFactEvidenceOrigins(tokenFact, &excludeRM)
	for _, origin := range origins {
		if origin == AnswerEvidenceOriginCurrentSource {
			t.Fatalf("model origin token must not beat the typed user boundary: %v", origins)
		}
	}
	// The classification projection remains available to the support validator;
	// A1 closes the proof lane later, at ledger compilation, so mixed-origin
	// validation cannot be weakened by an early deletion.
	if plain := AnswerAggregateFactEvidenceOrigins(tokenFact, &plainRM); len(plain) != 1 ||
		plain[0] != AnswerEvidenceOriginCurrentSource {
		t.Fatalf("explicit token classification changed before ledger compilation: %v", plain)
	}
}

func TestCompileObservationLedger_ExplicitCurrentSourceTokenCannotMintProofLane(t *testing.T) {
	rm := RequestModel{Intent: IntentRootCause}
	ledger := CompileObservationLedger(ObservationLedgerInput{
		AggregateFacts: []AnswerAggregateFact{{
			Kind:  AnswerAggregateScalar,
			Label: "model-declared source fact",
			Value: "1",
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: "current_source"},
			},
		}},
		RequestModel: &rm,
	})
	for _, record := range ledger.Records {
		if record.Origin == AnswerEvidenceOriginCurrentSource || record.SourceRef.Kind == ObservationSourceCurrentSource {
			t.Fatalf("model origin token minted current-source proof record: %+v", record)
		}
	}
	if len(ledger.Records) != 1 || ledger.Records[0].Origin != AnswerEvidenceOriginSystemInference || ledger.Records[0].SourceRef.Kind == ObservationSourceCurrentSource {
		t.Fatalf("model origin token must compile losslessly onto advisory lane: %+v", ledger.Records)
	}
}

func TestCompileObservationLedger_RequiredExactSupportRefRetainsCurrentSourceWitness(t *testing.T) {
	rm := RequestModel{
		Intent:   IntentExplain,
		Scenario: ScenarioArchitectureExplain,
		CurrentSourceExplanationProfile: &CurrentSourceExplanationProfile{
			IsCurrentSourceExplanationRequested: true,
			Modes:                               []CurrentSourceExplanationMode{CurrentSourceExplanationExplainCurrentMechanism},
			SourceQuotes:                        []string{"结合当前源码解释"},
			Confidence:                          0.9,
		},
	}
	ledger := CompileObservationLedger(ObservationLedgerInput{
		EvidenceItems: []EvidenceItem{{
			ID:              "verified-coordinate",
			Kind:            EvidenceDirect,
			Scope:           ScopeLine,
			Source:          "internal/tracequery/parse.go",
			LineStart:       22,
			GroundingStatus: GroundingGrounded,
		}},
		AggregateFacts: []AnswerAggregateFact{{
			Kind:        AnswerAggregateScalar,
			Label:       "verified source coordinate",
			Value:       "1",
			SupportRefs: []string{"internal/tracequery/parse.go:22"},
			Dimensions: []AnswerAggregateDimension{
				{Name: "origin", Value: "current_source"},
			},
		}},
		RequestModel: &rm,
	})
	for _, record := range ledger.Records {
		if record.Origin == AnswerEvidenceOriginCurrentSource && record.SourceRef.Kind == ObservationSourceCurrentSource &&
			len(record.SupportRefs) == 1 && record.SupportRefs[0] == "internal/tracequery/parse.go:22" &&
			record.SourceRef.Path == "internal/tracequery/parse.go" && record.Span.LineStart == 22 &&
			record.GroundingStatus == GroundingGrounded {
			return
		}
	}
	t.Fatalf("required exact file:line witness was over-demoted: %+v", ledger.Records)
}

// Pin 1 (ledger face): the donghu replay compiles ZERO current-source records
// and keeps every fact in the ledger as a runtime observation.
func TestCompileObservationLedger_TypedExcludeAggregateFactsZeroCurrentSourceRecords(t *testing.T) {
	rm := cspTypedExcludeRequestModel()
	facts := cspDonghuAggregateFacts()
	ledger := CompileObservationLedger(ObservationLedgerInput{
		AggregateFacts: facts,
		RequestModel:   &rm,
	})
	currentSource := 0
	runtime := 0
	for _, record := range ledger.Records {
		if record.Origin == AnswerEvidenceOriginCurrentSource ||
			record.SourceRef.Kind == ObservationSourceCurrentSource {
			currentSource++
		}
		if record.Origin == AnswerEvidenceOriginRuntimeArtifact {
			runtime++
		}
	}
	if currentSource != 0 {
		t.Fatalf("typed-exclude run compiled %d current-source record(s) from model aggregate facts (CSP #63 regression); records=%+v", currentSource, ledger.Records)
	}
	if runtime != len(facts) {
		t.Fatalf("exclude-run facts must stay in the ledger as runtime observations: got %d, want %d", runtime, len(facts))
	}

	// EVOLUTION RECORD (CSP-RM, §29.21 ruling 2026-07-10): the plain side of
	// this mutation pin used to freeze the ledger projection at
	// len(facts) current_source records — the donghu/cmp satisfied-pollution
	// source in the plain/required-lane shape. Post-ruling the same facts
	// compile as ADVISORY system_inference records (kind=model_claim), stay
	// lossless in the ledger, and never reach the current-source proof lane.
	// The exclude/plain delta survives as runtime_artifact vs system_inference.
	plainRM := RequestModel{Intent: IntentRootCause}
	plainLedger := CompileObservationLedger(ObservationLedgerInput{
		AggregateFacts: facts,
		RequestModel:   &plainRM,
	})
	plainCurrentSource := 0
	plainModelClaim := 0
	for _, record := range plainLedger.Records {
		if record.Origin == AnswerEvidenceOriginCurrentSource ||
			record.SourceRef.Kind == ObservationSourceCurrentSource {
			plainCurrentSource++
		}
		if record.Origin == AnswerEvidenceOriginSystemInference &&
			record.SourceRef.Kind == ObservationSourceModelClaim {
			plainModelClaim++
		}
	}
	if plainCurrentSource != 0 {
		t.Fatalf("plain-run model facts re-entered the current-source lane (CSP-RM regression): got %d records", plainCurrentSource)
	}
	if plainModelClaim != len(facts) {
		t.Fatalf("plain-run model facts must stay lossless on the advisory lane: got %d, want %d", plainModelClaim, len(facts))
	}
	plainAuthority := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		RequestModel: &plainRM,
		Ledger:       plainLedger,
	})
	if plainAuthority.CurrentSourceSatisfied || plainAuthority.CurrentSourceRecordCount != 0 {
		t.Fatalf("plain-run model facts still fake-satisfy the source lane (CSP-RM regression): %+v", plainAuthority)
	}
}

// Pin 4 (NEW with CSP-RM, §29.21): the LEDGER-SIDE second copy of the
// terminal fallback (compileAggregateFactObservations re-stamps facts whose
// origin projection came back empty). Before §29.21 it re-stamped
// current_source ABOVE the zero-emission chokepoint, so it was a live bypass
// in BOTH boundary shapes (probe 2026-07-10):
//   - typed-exclude run + non-runtime-carryable kind (AnswerAggregateExcluded)
//     → current_source record → satisfied=true in a 不分析代码 run;
//   - plain run + NegativeObservation kind (projection empty by design)
//     → current_source record → satisfied=true from a bare model claim.
//
// Both shapes must now land on the advisory lane, lossless, satisfied=false.
func TestCompileObservationLedger_LedgerSideFallbackNeverCurrentSource(t *testing.T) {
	excludeRM := cspTypedExcludeRequestModel()
	plainRM := RequestModel{Intent: IntentRootCause}
	for name, tc := range map[string]struct {
		rm   *RequestModel
		fact AnswerAggregateFact
	}{
		"exclude excluded-kind": {
			rm:   &excludeRM,
			fact: AnswerAggregateFact{Kind: AnswerAggregateExcluded, Label: "排除项", Value: "2", Excluded: []string{"a", "b"}},
		},
		"plain negative-observation": {
			rm:   &plainRM,
			fact: AnswerAggregateFact{Kind: AnswerAggregateNegativeObservation, Label: "no X observed", Value: "0"},
		},
	} {
		ledger := CompileObservationLedger(ObservationLedgerInput{
			AggregateFacts: []AnswerAggregateFact{tc.fact},
			RequestModel:   tc.rm,
		})
		advisory := 0
		for _, record := range ledger.Records {
			if record.Origin == AnswerEvidenceOriginCurrentSource ||
				record.SourceRef.Kind == ObservationSourceCurrentSource {
				t.Fatalf("%s: ledger-side fallback re-entered the current-source lane (CSP-RM regression): %+v", name, record)
			}
			if record.Origin == AnswerEvidenceOriginSystemInference &&
				record.SourceRef.Kind == ObservationSourceModelClaim {
				advisory++
			}
		}
		if advisory != 1 {
			t.Fatalf("%s: fact must stay lossless on the advisory lane: got %d advisory records; records=%+v", name, advisory, ledger.Records)
		}
		authority := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
			RequestModel: tc.rm,
			Ledger:       ledger,
		})
		if authority.CurrentSourceSatisfied || authority.CurrentSourceRecordCount != 0 {
			t.Fatalf("%s: ledger-side fallback still feeds satisfied (CSP-RM regression): %+v", name, authority)
		}
	}
}

// Pin 5 (NEW with CSP-RM review F-2, 2026-07-10): the THIRD bypass — an
// evidence item whose grounding pass VERIFIED FAILURE (GroundingUngrounded:
// the model-emitted file:line did not match the checkout) used to compile a
// current_source record regardless and feed CurrentSourceSatisfied (the
// donghu pseudo-citation shape on the evidence lane). It now lands on the
// advisory lane, lossless (path/span retained); the verified twin
// (GroundingGrounded) and the deterministic index recovery
// (GroundingRecovered) keep the historical current-source qualification —
// and the empty legacy status stays current_source too (a grounding pass
// that never ran is not a verified failure; absence never guesses).
func TestCompileObservationLedger_UngroundedEvidenceNeverCurrentSource(t *testing.T) {
	plainRM := RequestModel{Intent: IntentRootCause}
	pseudo := EvidenceItem{
		ID:              "pseudo",
		Source:          "internal/render/frame.go",
		LineStart:       42,
		Summary:         "model-claimed file:line that failed grounding",
		Salience:        SalienceLoadBearing,
		GroundingStatus: GroundingUngrounded,
	}
	ledger := CompileObservationLedger(ObservationLedgerInput{
		EvidenceItems: []EvidenceItem{pseudo},
		RequestModel:  &plainRM,
	})
	record := findObservationRecord(t, ledger, "evidence:pseudo")
	if record.Origin != AnswerEvidenceOriginSystemInference || record.SourceRef.Kind != ObservationSourceModelClaim {
		t.Fatalf("ungrounded evidence must requalify onto the advisory lane (F-2 regression): %+v", record)
	}
	if record.SourceRef.Path != "internal/render/frame.go" || record.Span.LineStart != 42 {
		t.Fatalf("advisory requalification must stay lossless: %+v", record)
	}
	authority := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		RequestModel: &plainRM,
		Ledger:       ledger,
	})
	if authority.CurrentSourceSatisfied || authority.CurrentSourceRecordCount != 0 {
		t.Fatalf("ungrounded pseudo-evidence still feeds satisfied (F-2 regression): %+v", authority)
	}

	// Verified / recovered / legacy-empty twins keep the historical lane —
	// an over-widened requalification flips these red.
	for name, status := range map[string]GroundingStatus{
		"grounded":     GroundingGrounded,
		"recovered":    GroundingRecovered,
		"legacy-empty": "",
	} {
		twin := pseudo
		twin.ID = "twin-" + name
		twin.GroundingStatus = status
		twinLedger := CompileObservationLedger(ObservationLedgerInput{
			EvidenceItems: []EvidenceItem{twin},
			RequestModel:  &plainRM,
		})
		twinRecord := findObservationRecord(t, twinLedger, "evidence:twin-"+name)
		if twinRecord.Origin != AnswerEvidenceOriginCurrentSource ||
			twinRecord.SourceRef.Kind != ObservationSourceCurrentSource {
			t.Fatalf("%s evidence lost its current-source qualification (over-widening): %+v", name, twinRecord)
		}
		twinAuthority := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
			RequestModel: &plainRM,
			Ledger:       twinLedger,
		})
		if !twinAuthority.CurrentSourceSatisfied {
			t.Fatalf("%s evidence must keep satisfying the source lane: %+v", name, twinAuthority)
		}
	}
}

// Pin 6 (NEW with CSP-RM review F-5): kind-side twin of the producer-row
// origin guard. A tool-published typed row may not smuggle current-source
// proof through an explicit SourceRef.Kind its origin cannot mint — the
// mismatched badge is requalified to the origin's canonical kind, row
// content lossless. Origins whose canonical kind IS current_source
// (repo_negative_search, the satisfaction valve) pass unchanged.
func TestCompileObservationLedger_ProducerRowKindSmuggleRequalified(t *testing.T) {
	plainRM := RequestModel{Intent: IntentRootCause}
	ledger := CompileObservationLedger(ObservationLedgerInput{
		ToolResults: []ToolResult{{
			ToolName: "trace_query",
			Success:  true,
			Observations: []ObservationRecord{
				{
					ID:        "smuggle-row",
					Origin:    AnswerEvidenceOriginRuntimeArtifact,
					SourceRef: ObservationSourceRef{Kind: ObservationSourceCurrentSource, Path: "internal/render/frame.go"},
					Summary:   "runtime row wearing a current_source badge",
				},
				{
					ID:        "neg-row",
					Origin:    AnswerEvidenceOriginRepoNegativeSearch,
					SourceRef: ObservationSourceRef{Kind: ObservationSourceCurrentSource, Path: "src/"},
					Summary:   "no FooBar match",
					Negative:  true,
				},
			},
		}},
		RequestModel: &plainRM,
	})
	smuggle := findObservationRecord(t, ledger, "smuggle-row")
	if smuggle.SourceRef.Kind != ObservationSourceRuntimeArtifact {
		t.Fatalf("mismatched current_source kind badge survived the producer guard (F-5 regression): %+v", smuggle)
	}
	neg := findObservationRecord(t, ledger, "neg-row")
	if neg.SourceRef.Kind != ObservationSourceCurrentSource {
		t.Fatalf("canonical negative-search kind must pass unchanged (valve intact): %+v", neg)
	}
	authority := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		RequestModel: &plainRM,
		Ledger:       ledger,
	})
	// satisfied stays true ONLY through the negative-search valve.
	if !authority.CurrentSourceSatisfied || authority.CurrentSourceRecordCount != 1 {
		t.Fatalf("authority counting drifted: want exactly the valve record, got %+v", authority)
	}
	smuggleOnly := CompileObservationLedger(ObservationLedgerInput{
		ToolResults: []ToolResult{{
			ToolName: "trace_query",
			Success:  true,
			Observations: []ObservationRecord{{
				ID:        "smuggle-row",
				Origin:    AnswerEvidenceOriginRuntimeArtifact,
				SourceRef: ObservationSourceRef{Kind: ObservationSourceCurrentSource, Path: "internal/render/frame.go"},
				Summary:   "runtime row wearing a current_source badge",
			}},
		}},
		RequestModel: &plainRM,
	})
	smuggleAuthority := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		RequestModel: &plainRM,
		Ledger:       smuggleOnly,
	})
	if smuggleAuthority.CurrentSourceSatisfied || smuggleAuthority.CurrentSourceRecordCount != 0 {
		t.Fatalf("kind smuggle alone still feeds satisfied (F-5 regression): %+v", smuggleAuthority)
	}
}

// Pin 3: the authority over the donghu shape — the exact projection whose
// run-log line was `lane=excluded ... satisfied=true source=10` — now reports
// zero satisfied and opens the runtime-only cleanup lane by itself.
func TestBuildRuntimeSourceAnswerAuthoritySnapshot_TypedExcludeDonghuShapeNoSatisfied(t *testing.T) {
	rm := cspTypedExcludeRequestModel()
	ledger := CompileObservationLedger(ObservationLedgerInput{
		AggregateFacts: cspDonghuAggregateFacts(),
		RequestModel:   &rm,
	})
	authority := BuildRuntimeSourceAnswerAuthoritySnapshot(RuntimeSourceAnswerAuthorityInput{
		RequestModel: &rm,
		Ledger:       ledger,
	})
	if !authority.Active {
		t.Fatalf("donghu shape must keep the authority active via runtime observations: %+v", authority)
	}
	if authority.CurrentSourceLane != CurrentSourceLaneExcluded {
		t.Fatalf("lane = %q, want excluded", authority.CurrentSourceLane)
	}
	if authority.CurrentSourceRecordCount != 0 || authority.CurrentSourceSatisfied {
		t.Fatalf("exclude-run authority still satisfied (CSP #63 regression): %+v", authority)
	}
	if authority.KeepsCurrentSourceLaneLoadBearing() {
		t.Fatalf("polluted satisfied flag kept the source lane load-bearing: %+v", authority)
	}
	if !authority.AllowsRuntimeEvidenceWithoutCurrentSource() {
		t.Fatalf("runtime-only cleanup lane must open on the root fix alone: %+v", authority)
	}
	for _, code := range authority.ReasonCodes {
		if code == RuntimeSourceAuthorityCurrentSourceSatisfied {
			t.Fatalf("reason codes must not carry current_source_satisfied in the donghu shape: %v", authority.ReasonCodes)
		}
	}
}
