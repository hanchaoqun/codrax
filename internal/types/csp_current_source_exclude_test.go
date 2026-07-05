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
//  2. Mutation pin: the same facts WITHOUT the exclude boundary keep the
//     historical current-source fallback — reverting the exclude gate flips
//     pin 1 red while this pin stays green, so old behavior cannot silently
//     return in either direction.
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

		// Mutation pin: the same fact WITHOUT the exclude boundary keeps the
		// historical current-source fallback.
		plain := AnswerAggregateFactEvidenceOrigins(fact, &plainRM)
		if len(plain) != 1 || plain[0] != AnswerEvidenceOriginCurrentSource {
			t.Fatalf("non-exclude fallback changed for fact %q: got %v, want [current_source]", fact.Label, plain)
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
	if plain := AnswerAggregateFactEvidenceOrigins(tokenFact, &plainRM); len(plain) == 0 ||
		plain[0] != AnswerEvidenceOriginCurrentSource {
		t.Fatalf("explicit token lane changed outside the exclude boundary: %v", plain)
	}
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

	// Mutation pin: dropping the boundary restores the historical
	// current-source projection — the exclude gate is exactly the delta.
	plainRM := RequestModel{Intent: IntentRootCause}
	plainLedger := CompileObservationLedger(ObservationLedgerInput{
		AggregateFacts: facts,
		RequestModel:   &plainRM,
	})
	plainCurrentSource := 0
	for _, record := range plainLedger.Records {
		if record.Origin == AnswerEvidenceOriginCurrentSource {
			plainCurrentSource++
		}
	}
	if plainCurrentSource != len(facts) {
		t.Fatalf("non-exclude ledger projection changed: got %d current-source records, want %d", plainCurrentSource, len(facts))
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
