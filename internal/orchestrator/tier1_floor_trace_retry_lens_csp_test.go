package orchestrator

// CSP #63 回探臂 (2026-07-10) — pre-finalize localizer retry wording fork for
// pure-trace sessions.
//
// Witness: cust_trace_cmp_792.txt:77-79 (customer 792 回访, ledger
// real_trace_campaign_20260705.md §29.8 P2④). A two-trace comparison run
// ("对比分析 … bindApplication …", plain RequestModel — no external-observation
// policy) tripped the pre-finalize follow-up gate after its first
// emit_investigation_complete, and the retry directive said "Missing repo_map
// lenses: task_map, file_map, relation_map" — while the same prompt's
// artifact-only workflow section said NOT to run repo breadth search. The
// model called out the contradiction verbatim and converged through
// trace_query window_sweep → root_cause_rank → frame_root_cause_bundle drills.
//
// Ruling (§29.8): the repair loop itself is design-final (gate intercept →
// one more pass → convergence); ONLY the directive word-face was wrong.
//
// Arm attribution (probe-verified on the replay below, file:line on HEAD):
//   arm0 tier1_floor.go readLocalizerFollowupForTier1 →
//        types.RuntimeArtifactReadSourceNavigationNotRequiredForBusContext
//        declined: AllowsRuntimeEvidenceWithoutCurrentSource()==false because
//        KeepsCurrentSourceLaneLoadBearing()==true
//        (runtime_source_answer_authority_view.go — satisfied arm);
//   arm1 zero_current_source_repo declined: census counts ANY non-artifact
//        regular file (orchestrator/runtime_artifact_preflight.go) and the
//        customer cwd is not artifact-only — honest fail-closed absence;
//   arm2 runtime_observation_closure declined at the authority keep arm
//        (tier1_floor.go readLocalizerRuntimeSourceAuthorityKeepsFollowup)
//        BEFORE the sufficiency / deterministic-query / trace-count arms;
//   arm3 TraceQueryRuntimeObservationCount>0 was true but structurally
//        unreachable behind the keep arm.
// Shared root: CurrentSourceSatisfied minted from the model's own aggregate
// facts via the plain-RM terminal current_source fallback
// (types/answer_evidence_origin.go) — the CSP #63 pollution class in the
// plain/required-lane shape.
//
// EVOLUTION RECORD (CSP-RM, §29.21 ruling 2026-07-10; amended same day per
// CSP-RM review F-1): the ruling this file's conscious-flip marker was
// waiting for has landed. The plain-RM terminal fallback now mints the
// ADVISORY lane (system_inference / kind=model_claim); pure model claims can
// no longer set CurrentSourceSatisfied, so the witness shape's keep-arm veto
// is gone. The FIRST evolution of this file let the honest authority suppress
// the witness follow-up entirely — review F-1 rejected that as a quality
// regression (ledger :1815: the cmp retry was BENEFICIAL — the final
// projection's core evidence, the root_cause_rank hot windows and
// frame_root_cause_bundle rows, was all produced by the retry round; the
// first pass had only window_stats/window_sweep/wakeup_chain-level rows,
// heavy views truncated by index_event_limit). The closure criterion for the
// trace-drill shape is therefore the typed drill DEPTH
// (traceDrillRetryPending in tier1_floor.go — root_cause_rank-family
// deterministic observation count over the trace coverage taxonomy):
//   - Pin 1  (zero-drill retry): witness first-pass shape (depth==0) → the
//     follow-up still fires with the §29.20 trace word-face — the beneficial
//     retry is preserved, now riding a typed trace-side signal instead of
//     the satisfied pollution.
//   - Pin 1c (post-drill closure): a rank-family observation landed
//     (depth>0) → the follow-up is honestly suppressed — single-pass
//     convergence, matching the witness.
//   - Pin 1b (negative-grep keep): a REAL deterministic source witness keeps
//     the source lane satisfied and the retry directive stays trace-worded.
// The wording fork itself (lens + renderer) is unchanged and stays pinned at
// unit level (Pins 3/3b). Suppression-arm ORDER remains untouched; the
// pending gate is a typed carve-out after the keep arm, bounded by the
// existing retry budget.
//
// Review fixes (SHIP-WITH-FIXES 2026-07-10):
//   P1-1 the lens additionally requires the ABSENCE of a typed
//        current-source requirement (readLocalizerTier1CurrentSourceRequired
//        — the same typed signal the closure suppressor consumes): a mixed
//        run whose follow-up stays alive BECAUSE the request demands source
//        proof must never receive a directive claiming repository search is
//        not required, or the retry loop strands on an unsatisfiable
//        directive until the budget exhausts.
//   P2-1 runtime-artifact read_file entries (trace_query blob escape lane,
//        .codrax materializations, artifact-shaped paths, attached-trace
//        spelling) do not count as source work — the read_file tool already
//        types that decision (ToolRuntimeArtifactRead / nil ReadCoverage, so
//        such reads never enter TurnA.ReadFiles via the explorer walker);
//        the lens-side path filter is the consumption-side defense.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// cspCmp792Round1TraceToolResult mirrors the witness's FIRST-pass evidence
// level (review F-1 fidelity fix): window_stats/window_sweep/wakeup_chain
// class rows landed, while the heavy hot-window attribution family
// (root_cause_rank / frame_root_cause_bundle) was truncated by
// index_event_limit and produced no rows — the typed drill depth is zero.
// The Predicate is the trace coverage taxonomy's wakeup_chain lane
// (trace_observation_coverage.go), NOT a root_cause_* key.
func cspCmp792Round1TraceToolResult() types.ToolResult {
	return types.ToolResult{
		ToolName: "trace_query",
		Success:  true,
		// Distinct summary: the ledger tool-result merge dedupes on
		// ToolName+RawRef+Summary, and drilled twins append a second
		// trace_query result to the same context.
		Summary: "[trace_query: view=wakeup_chain window=5.000s-5.007s]",
		Observations: []types.ObservationRecord{{
			ID:              "trace_query:window#wakeup_chain:1",
			Origin:          types.AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact},
			Subject:         "com.baidu.tieba-59566",
			Predicate:       "wakeup_chain",
			Object:          "ThreadPoolForeg-60555",
		}},
	}
}

// cspCmp792TraceBusContext mirrors the witness run shape: two preflight trace
// artifacts referenced in the request, a non-artifact-only cwd census, one
// deterministic FIRST-PASS-level trace_query runtime observation (wakeup
// chain — zero drill depth), a completed first investigation pass whose
// model-authored aggregate facts are retained, and zero source reads.
func cspCmp792TraceBusContext() *types.BusContext {
	mu := types.NewMutableState("对比分析两个 systrace 的 bindApplication 耗时差异主要原因")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{})
	mu.AppendDispatchToolResult(cspCmp792Round1TraceToolResult())
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{Kind: types.AnswerAggregateScalar, Label: "7.0 sleep", Value: "1430.101 ms"},
		{Kind: types.AnswerAggregateScalar, Label: "6.0 sleep", Value: "974.469 ms"},
		{Kind: types.AnswerAggregateScalar, Label: "sleep difference", Value: "455.632 ms"},
	})
	mu.RetainInvestigationAggregateFacts()
	return &types.BusContext{
		Mutable: mu,
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{
			Active:                   true,
			SourceNavigationOptional: true,
			ReasonCode:               types.RuntimeArtifactPreflightReasonDetected,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{
				{Kind: "trace", Source: "7.0B30SP22_7315.systrace", Bytes: 408555520, Carrier: "request_path"},
				{Kind: "trace", Source: "6.0B138_3900.sys.systrace", Bytes: 499779584, Carrier: "request_path"},
			},
			RepoSourceCensus: types.RuntimeArtifactRepoSourceCensus{Completed: true, SourceFiles: 1, ArtifactFiles: 2},
		},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentRootCause,
			Scenario: types.ScenarioPerformanceBottleneck,
			AnalyzerHints: types.AnalyzerHints{
				Entities: []string{"7.0B30SP22_7315.systrace", "6.0B138_3900.sys.systrace", "bindApplication"},
			},
		}},
	}
}

// Pin 1 (RE-EVOLVED per CSP-RM review F-1) — witness first-pass replay,
// zero-drill retry: the authority is honest (satisfied=false, keeps=false —
// the §29.21 root fix), the keep-arm veto is gone, and the follow-up STILL
// fires because the typed drill depth is zero (no root_cause_rank-family
// deterministic observation yet — exactly the witness first pass whose heavy
// views were truncated). The directive carries the §29.20 trace word-face.
// The beneficial retry the witness converged through is preserved, now
// riding a typed trace-side signal instead of the satisfied pollution.
func TestCheckTier1Floor_Cmp792ZeroDrillDepthFiresTraceDrillRetry(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	busCtx := cspCmp792TraceBusContext()
	o := &Orchestrator{busCtx: busCtx}

	// Witness preconditions (fixture drift guards).
	if busCtx.RuntimeArtifactPreflight.ZeroCurrentSourceRepo() {
		t.Fatal("witness precondition drifted: census must not be artifact-only")
	}
	if busCtx.Mutable.TraceQueryRuntimeObservationCount() == 0 {
		t.Fatal("witness precondition drifted: deterministic trace observation missing")
	}
	if depth := traceDrillDepthObservationCount(busCtx); depth != 0 {
		t.Fatalf("witness precondition drifted: first pass must have zero drill depth, got %d", depth)
	}

	// §29.21 root-fix face: the model facts no longer mint current-source
	// proof — they live losslessly on the advisory lane, the keep arm is
	// honest, and the pending gate (not pollution) is what keeps the retry.
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(busCtx, types.ObservationLedger{})
	if authority.CurrentSourceSatisfied || authority.CurrentSourceRecordCount != 0 {
		t.Fatalf("model aggregate facts fake-satisfied the source lane again (§29.21 regression): %+v", authority)
	}
	if authority.KeepsCurrentSourceLaneLoadBearing() {
		t.Fatalf("keep arm rides pollution again (§29.21 regression): %+v", authority)
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(busCtx, types.ObservationExtractLedgerEvidenceLimit))
	advisory := 0
	for _, record := range ledger.Records {
		if record.Origin == types.AnswerEvidenceOriginSystemInference &&
			record.SourceRef.Kind == types.ObservationSourceModelClaim {
			advisory++
		}
	}
	if advisory != 3 {
		t.Fatalf("model facts must stay lossless on the advisory lane: got %d, want 3; records=%+v", advisory, ledger.Records)
	}

	// F-1 pending-gate attribution: the trace-drill retry is pending, so
	// neither arm0 (source-navigation waiver) nor the arm2 closure may
	// suppress on source-lane-advisory reasoning.
	if !traceDrillRetryPending(busCtx, busCtx.AnalysisIR) {
		t.Fatal("zero-drill witness shape must report a pending trace-drill retry")
	}
	if runtimeObservationClosureSuppressesReadLocalizerFollowup(busCtx, busCtx.AnalysisIR) {
		t.Fatal("arm2 closure must yield while the drill retry is pending (F-1 regression)")
	}

	state := newGraphState(types.TaskGraph{ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 4}})
	msg, _, proceed, exhausted := o.checkTier1Floor(busCtx.AnalysisIR, state)
	if proceed || exhausted {
		t.Fatalf("zero-drill witness shape must fire the beneficial retry (F-1), proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
	for _, want := range []string{
		"trace_query", "window_sweep", "root_cause_rank", "frame_root_cause_bundle",
		"time_start", "emit_investigation_complete",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("trace drill retry directive missing %q:\n%s", want, msg)
		}
	}
	for _, banned := range []string{"repo_map", "read_file", "Source localization"} {
		if strings.Contains(msg, banned) {
			t.Fatalf("pure-trace retry directive still carries repo-navigation word-face %q (cmp_792 regression):\n%s", banned, msg)
		}
	}
}

// Pin 1c (NEW per CSP-RM review F-1) — post-drill closure: once a
// root_cause_rank-family deterministic observation lands (the retry round's
// product), the drill depth flips >0, the pending gate turns off, and the
// follow-up is honestly suppressed through the authority arms — single-pass
// convergence, matching the witness. The authority stays honest
// (satisfied=false: drill rows are trace-side, not source proof).
func TestCheckTier1Floor_Cmp792DrilledRankObservationSuppressesRetry(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	busCtx := cspCmp792TraceBusContext()
	busCtx.Mutable.AppendDispatchToolResult(tier1TraceQueryRuntimeToolResult())
	if depth := traceDrillDepthObservationCount(busCtx); depth == 0 {
		t.Fatal("fixture drifted: rank-family observation must register drill depth")
	}
	if traceDrillRetryPending(busCtx, busCtx.AnalysisIR) {
		t.Fatal("pending gate must turn off once the drill round landed")
	}
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(busCtx, types.ObservationLedger{})
	if authority.CurrentSourceSatisfied || authority.KeepsCurrentSourceLaneLoadBearing() {
		t.Fatalf("drill rows must not fake source-lane proof: %+v", authority)
	}

	o := &Orchestrator{busCtx: busCtx}
	state := newGraphState(types.TaskGraph{ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 4}})
	msg, _, proceed, exhausted := o.checkTier1Floor(busCtx.AnalysisIR, state)
	if !proceed || exhausted || msg != "" {
		t.Fatalf("drilled shape must complete without a further retry (single-pass convergence), proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
}

// Pin 1b (NEW with CSP-RM §29.21; wording corrected per review F-3 — the
// trigger here is the negative-grep WITNESS keeping the source lane, not an
// "insufficiency" verdict): when the source lane is satisfied by a REAL
// deterministic witness (a negative repo grep — the §29.21-sanctioned
// grep-family valve) and trace localization is still incomplete, the
// follow-up stays alive through the honest keep arm and the directive
// carries the §29.20 trace word-face verbatim — riding a witness instead of
// pollution.
func TestCheckTier1Floor_HonestKeepNegativeGrepFiresTraceDrillRetry(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	busCtx := cspCmp792TraceBusContext()
	busCtx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Observations: []types.ObservationRecord{{
			ID:        "neg-row",
			Origin:    types.AnswerEvidenceOriginRepoNegativeSearch,
			SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceCurrentSource, Path: "src/"},
			Summary:   "no bindApplication match",
			Negative:  true,
		}},
	})
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(busCtx, types.ObservationLedger{})
	if !authority.CurrentSourceSatisfied || !authority.KeepsCurrentSourceLaneLoadBearing() {
		t.Fatalf("negative-grep witness must satisfy and keep the source lane (deterministic valve): %+v", authority)
	}

	o := &Orchestrator{busCtx: busCtx}
	state := newGraphState(types.TaskGraph{ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 4}})
	msg, _, proceed, exhausted := o.checkTier1Floor(busCtx.AnalysisIR, state)
	if proceed || exhausted {
		t.Fatalf("honest keep must fire the trace-drill retry, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
	for _, want := range []string{
		"trace_query", "window_sweep", "root_cause_rank", "frame_root_cause_bundle",
		"time_start", "emit_investigation_complete",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("trace drill retry directive missing %q:\n%s", want, msg)
		}
	}
	for _, banned := range []string{"repo_map", "read_file", "Source localization"} {
		if strings.Contains(msg, banned) {
			t.Fatalf("pure-trace retry directive still carries repo-navigation word-face %q (cmp_792 regression):\n%s", banned, msg)
		}
	}
}

// Pin 2 — source-session negative control: a run that actually read source
// files keeps the repository-navigation wording byte-for-byte, even with an
// active trace surface and deterministic trace observations. Both fork
// directions are pinned; over-broadening the trace lens flips this red.
//
// EVOLUTION RECORD (CSP-RM §29.21): the fixture previously kept the follow-up
// alive through the satisfied POLLUTION (bare ReadFiles never reach the
// ledger); post-ruling the keep must ride a real witness, so the fixture now
// carries the read_file tool result with its typed ReadCoverage — the exact
// deterministic read witness §29.21 names as the accepted proof lane.
func TestCheckTier1Floor_SourceReadSessionKeepsRepoMapLensWording(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	busCtx := cspCmp792TraceBusContext()
	busCtx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"pkg/handler.py"},
	})
	busCtx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName: "read_file",
		Success:  true,
		ReadCoverage: &types.ToolReadCoverage{
			Path:      "pkg/handler.py",
			LineStart: 1,
			LineEnd:   40,
		},
	})
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(busCtx, types.ObservationLedger{})
	if !authority.CurrentSourceSatisfied {
		t.Fatalf("read coverage witness must satisfy the source lane (§29.21 accepted proof): %+v", authority)
	}
	o := &Orchestrator{busCtx: busCtx}
	state := newGraphState(types.TaskGraph{ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 4}})

	msg, _, proceed, exhausted := o.checkTier1Floor(busCtx.AnalysisIR, state)
	if proceed || exhausted {
		t.Fatalf("expected localizer retry, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
	for _, want := range []string{"Source localization", "repo_map", "read_file"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("source-session retry wording lost %q (trace lens over-broadened):\n%s", want, msg)
		}
	}
}

// Pin 3 — lens condition axes (direct unit pins; the no-observation twin of
// the witness shape gets suppressed upstream by the observation-only-surface
// arm before the wording fork is reached, so the helper is pinned directly):
// every condition is required, and each false condition falls back to the
// source-navigation wording.
func TestTraceObservationDrillRetryLensActive_ConditionAxes(t *testing.T) {
	lens := func(ctx *types.BusContext) bool {
		return traceObservationDrillRetryLensActive(ctx, ctx.AnalysisIR)
	}
	if !lens(cspCmp792TraceBusContext()) {
		t.Fatal("witness shape must enable the trace drill lens")
	}

	// B41: runtime relation and runtime breadth are orthogonal. A finite
	// relation lookup must not inherit the heavy causal drill merely because
	// its legacy axes say call-chain; the shared bounded profile is the
	// stronger typed authority.
	boundedRelation := cspCmp792TraceBusContext()
	boundedRelation.AnalysisIR.RequestModel.PredicateAxis = types.AxisCall
	boundedRelation.AnalysisIR.RequestModel.AnalyzerHints.Kind = string(types.ReqCallChain)
	boundedRelation.AnalysisIR.RequestModel.Predicates.IsRelationalLookup = true
	boundedRelation.AnalysisIR.RequestModel.RuntimeQuestionProfile = &types.RuntimeQuestionProfile{
		Scope: types.RuntimeQuestionScopeBoundedFactSet,
	}
	if lens(boundedRelation) {
		t.Fatal("bounded relation must not be upgraded to the heavy causal drill")
	}
	start, end := 10.0, 10.1
	boundedRelation.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile = &types.RuntimeArtifactScopeProfile{
		RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &start,
		TimeEnd:        &end,
		SourceQuote:    "10.0..10.1",
	}
	if !lens(boundedRelation) {
		t.Fatal("explicit user window must retain the heavy causal drill")
	}

	// Axis (P1-1): typed current-source requirement on the request → lens
	// off. PerfTrace with in-repo resolved files is the standard perf-triage
	// product and readLocalizerTier1CurrentSourceRequired's typed carrier.
	preciseAsk := cspCmp792TraceBusContext()
	preciseAsk.AnalysisIR.RequestModel.PerfTrace = &types.PerfBundle{ResolvedFiles: []string{"pkg/handler.py"}}
	if lens(preciseAsk) {
		t.Fatal("lens must stay off while the request typed-requires current-source proof (P1-1)")
	}

	// Axis: no deterministic trace_query observation → lens off.
	noObs := cspCmp792TraceBusContext()
	mu := types.NewMutableState("no observation twin")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{})
	noObs.Mutable = mu
	if lens(noObs) {
		t.Fatal("lens must require a deterministic trace_query observation")
	}

	// Axis: no typed trace surface → lens off.
	noSurface := cspCmp792TraceBusContext()
	noSurface.RuntimeArtifactPreflight = types.RuntimeArtifactPreflightProfile{}
	noSurface.AnalysisIR.RequestModel.AnalyzerHints.Entities = nil
	if lens(noSurface) {
		t.Fatal("lens must require a typed trace-query surface")
	}

	// Axis: current-source reads happened → lens off.
	readSession := cspCmp792TraceBusContext()
	readSession.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ReadFiles: []string{"pkg/handler.py"}})
	if lens(readSession) {
		t.Fatal("lens must stay off once source files were read")
	}

	// Axis (P2-1): runtime-artifact reads are NOT source work — blob escape
	// lane, .codrax materializations, artifact-shaped paths, and the
	// attached-trace spelling (case-folded, even without an artifact-shaped
	// extension) all keep the lens on; one genuine source read among them
	// still turns it off.
	artifactReads := cspCmp792TraceBusContext()
	artifactReads.AttachedHitraceSource = "../../CustomLogs/Customer_Trace.data"
	if types.RuntimeArtifactPathKind(artifactReads.AttachedHitraceSource) != "" {
		t.Fatal("fixture drifted: spelling-arm fixture must not be artifact-shaped")
	}
	artifactReads.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ReadFiles: []string{
		".codrax/blob/20260703-111820-000-5208/trace_query-ab12cd34.txt",
		"attached_trace.txt",
		"7.0B30SP22_7315.systrace",
		"customer_trace.data",
	}})
	if !lens(artifactReads) {
		t.Fatal("runtime-artifact reads must not disable the trace lens (P2-1)")
	}
	mixedReads := cspCmp792TraceBusContext()
	mixedReads.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ReadFiles: []string{
		"attached_trace.txt",
		"pkg/handler.py",
	}})
	if lens(mixedReads) {
		t.Fatal("one genuine source read must still turn the lens off")
	}

	// Axis: current-repo (or legacy empty-origin) evidence exists → lens off;
	// runtime log/perf-origin evidence keeps it on.
	repoEvidence := cspCmp792TraceBusContext()
	repoEvidence.Mutable.AppendEvidence([]types.EvidenceItem{{
		ID: "repo-row", Source: "pkg/handler.py", LineStart: 1, Origin: types.ClaimOriginCurrentRepo,
	}})
	if lens(repoEvidence) {
		t.Fatal("lens must stay off with current-repo evidence in the buffer")
	}
	perfEvidence := cspCmp792TraceBusContext()
	perfEvidence.Mutable.AppendEvidence([]types.EvidenceItem{{
		ID: "perf-row", Source: "/tmp/app.systrace", LineStart: 10, Origin: types.ClaimOriginPerf,
	}})
	if !lens(perfEvidence) {
		t.Fatal("runtime perf-origin evidence must not disable the trace lens")
	}
}

// Pin 3b — renderer fork is exactly the lens flag: same followup, both
// wordings pinned (repo-navigation demand words never leak into the trace
// directive; the trace views never leak into the source directive).
func TestRenderReadLocalizerFollowupRetryMessage_ForkByTraceLens(t *testing.T) {
	followup := &types.ReadLocalizerFollowup{
		State:         types.ReadLocalizerFollowupNeeded,
		ReasonCode:    "read_localizer_navigation_missing",
		MissingRoutes: []types.RepoMapNavigationRoute{types.RepoMapNavigationRouteTaskMap, types.RepoMapNavigationRouteFileMap},
	}
	source := renderReadLocalizerFollowupRetryMessage(followup, false)
	if !strings.Contains(source, "Missing repo_map lenses: task_map, file_map") ||
		!strings.Contains(source, "use repo_map for the missing lenses") {
		t.Fatalf("source wording changed:\n%s", source)
	}
	trace := renderReadLocalizerFollowupRetryMessage(followup, true)
	for _, banned := range []string{"repo_map", "read_file", "Source localization", "task_map", "file_map"} {
		if strings.Contains(trace, banned) {
			t.Fatalf("trace directive carries repo-navigation word-face %q:\n%s", banned, trace)
		}
	}
	for _, want := range []string{"trace_query", "window_sweep", "time_start", "emit_investigation_complete"} {
		if !strings.Contains(trace, want) {
			t.Fatalf("trace directive missing %q:\n%s", want, trace)
		}
	}
}

// Pin 4 — current-repo evidence in the buffer: source-lane work exists, so
// the source wording stays even though the session never called read_file.
//
// EVOLUTION RECORD (CSP-RM §29.21): the follow-up previously stayed alive
// through the satisfied pollution while the evidence only steered the LENS
// (Mutable evidence is a lens input; the BUS ledger reads bus.EvidenceItems).
// Post-ruling the keep must ride the real evidence witness, so the fixture
// carries the item on BOTH faces: bus.EvidenceItems (ledger/authority) and
// Mutable evidence (lens).
func TestCheckTier1Floor_CurrentRepoEvidenceKeepsSourceWording(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	busCtx := cspCmp792TraceBusContext()
	busCtx.EvidenceItems = []types.EvidenceItem{{
		ID:              "current-source-recovered-only",
		Kind:            types.EvidenceMechanism,
		Subject:         "handler.py",
		Predicate:       "mechanism",
		Object:          "dispatch",
		Source:          "pkg/handler.py",
		LineStart:       42,
		Scope:           types.ScopeLine,
		GroundingStatus: types.GroundingRecovered,
		GroundingTier:   types.TierSymbolTable,
		Origin:          types.ClaimOriginCurrentRepo,
	}}
	busCtx.Mutable.AppendEvidence([]types.EvidenceItem{{
		ID:              "current-source-recovered-only",
		Kind:            types.EvidenceMechanism,
		Subject:         "handler.py",
		Predicate:       "mechanism",
		Object:          "dispatch",
		Source:          "pkg/handler.py",
		LineStart:       42,
		Scope:           types.ScopeLine,
		GroundingStatus: types.GroundingRecovered,
		GroundingTier:   types.TierSymbolTable,
		Origin:          types.ClaimOriginCurrentRepo,
	}})
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(busCtx, types.ObservationLedger{})
	if !authority.CurrentSourceSatisfied {
		t.Fatalf("evidence witness must satisfy the source lane (§29.21 accepted proof): %+v", authority)
	}
	o := &Orchestrator{busCtx: busCtx}
	state := newGraphState(types.TaskGraph{ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 4}})

	msg, _, proceed, exhausted := o.checkTier1Floor(busCtx.AnalysisIR, state)
	if proceed || exhausted {
		t.Fatalf("expected localizer retry, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
	if !strings.Contains(msg, "repo_map") || strings.Contains(msg, "window_sweep") {
		t.Fatalf("current-repo evidence must keep source wording:\n%s", msg)
	}
}

// Pin 5 (P1-1) — mixed run with a typed current-source requirement: the
// follow-up stays alive BECAUSE the request demands source proof (healthy
// authority, no pollution needed), so the directive must keep the source
// wording — "repository search is not required" here would contradict the
// keep-alive reason and strand the retry loop until the budget exhausts.
func TestCheckTier1Floor_MixedPreciseCurrentSourceAskKeepsSourceWording(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	busCtx := cspCmp792TraceBusContext()
	busCtx.AnalysisIR.RequestModel.PerfTrace = &types.PerfBundle{ResolvedFiles: []string{"pkg/handler.py"}}
	o := &Orchestrator{busCtx: busCtx}
	state := newGraphState(types.TaskGraph{ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 4}})

	msg, _, proceed, exhausted := o.checkTier1Floor(busCtx.AnalysisIR, state)
	if proceed || exhausted {
		t.Fatalf("expected localizer retry, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
	if !strings.Contains(msg, "repo_map") {
		t.Fatalf("typed current-source ask lost the source wording (P1-1 regression):\n%s", msg)
	}
	if strings.Contains(msg, "not required") || strings.Contains(msg, "window_sweep") {
		t.Fatalf("typed current-source ask received a contradiction-shaped trace directive:\n%s", msg)
	}
}

// Pin 6 (P2-1) — artifact reads end-to-end through the REAL read_file tool:
// (a) upstream covenant: a runtime-artifact read carries the typed
// ToolRuntimeArtifactRead marker and NO ReadCoverage, so it never enters
// TurnA.ReadFiles via the explorer coverage walker in the first place;
// (b) consumption-side defense: even if such a path reaches ReadFiles (merge
// paths, future producers), the lens stays on; (c) a genuine source read
// produces coverage, lands in ReadFiles, and turns the lens off.
func TestCheckTier1Floor_ArtifactReadKeepsTraceDrillWording(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	repoRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(repoRoot, "attached_trace.txt"), []byte("sched_switch prev=app-100 prev_state=S next=worker-200\n"), 0o644); err != nil {
		t.Fatalf("setup artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "handler.py"), []byte("def dispatch():\n    return 1\n"), 0o644); err != nil {
		t.Fatalf("setup source: %v", err)
	}

	// (a) Covenant: artifact read → typed marker, no coverage.
	artifactRead, err := (&tool.ReadFile{}).Execute(&types.BusContext{RepoRoot: repoRoot}, mustJSONRawMessage(t, map[string]string{"path": "attached_trace.txt"}))
	if err != nil {
		t.Fatalf("read_file artifact: %v", err)
	}
	if artifactRead.RuntimeArtifactRead == nil || artifactRead.ReadCoverage != nil {
		t.Fatalf("artifact read must carry the typed marker and no read coverage: marker=%+v coverage=%+v",
			artifactRead.RuntimeArtifactRead, artifactRead.ReadCoverage)
	}

	// (b) Defense (re-evolved per CSP-RM review F-1): an artifact read must
	// never mint source-lane proof (authority face pinned below), and with
	// the zero-drill pending gate restored the follow-up FIRES with the
	// trace wording — reading the attached artifact is a design-internal
	// escape lane and must not flip the directive back to source navigation
	// (the original P2-1 assertion, now riding the honest authority + typed
	// depth signal instead of the satisfied pollution).
	busCtx := cspCmp792TraceBusContext()
	busCtx.RepoRoot = repoRoot
	busCtx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles:   []string{"attached_trace.txt"},
		ToolResults: []types.ToolResult{artifactRead},
	})
	if !traceObservationDrillRetryLensActive(busCtx, busCtx.AnalysisIR) {
		t.Fatal("artifact read disabled the trace lens (P2-1 regression)")
	}
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(busCtx, types.ObservationLedger{})
	if authority.CurrentSourceSatisfied {
		t.Fatalf("artifact read minted source-lane proof (§29.21 regression): %+v", authority)
	}
	o := &Orchestrator{busCtx: busCtx}
	state := newGraphState(types.TaskGraph{ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 4}})
	msg, _, proceed, exhausted := o.checkTier1Floor(busCtx.AnalysisIR, state)
	if proceed || exhausted {
		t.Fatalf("expected localizer retry, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
	if !strings.Contains(msg, "window_sweep") || strings.Contains(msg, "repo_map") {
		t.Fatalf("artifact read flipped the directive back to source wording (P2-1 regression):\n%s", msg)
	}

	// (c) Genuine source read → coverage minted → lens off, source wording.
	sourceRead, err := (&tool.ReadFile{}).Execute(&types.BusContext{RepoRoot: repoRoot}, mustJSONRawMessage(t, map[string]string{"path": "handler.py"}))
	if err != nil {
		t.Fatalf("read_file source: %v", err)
	}
	if sourceRead.ReadCoverage == nil || sourceRead.RuntimeArtifactRead != nil {
		t.Fatalf("source read must mint read coverage and no artifact marker: marker=%+v coverage=%+v",
			sourceRead.RuntimeArtifactRead, sourceRead.ReadCoverage)
	}
	sourceCtx := cspCmp792TraceBusContext()
	sourceCtx.RepoRoot = repoRoot
	sourceCtx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles:   []string{sourceRead.ReadCoverage.Path},
		ToolResults: []types.ToolResult{sourceRead},
	})
	o = &Orchestrator{busCtx: sourceCtx}
	msg, _, proceed, exhausted = o.checkTier1Floor(sourceCtx.AnalysisIR, state)
	if proceed || exhausted {
		t.Fatalf("expected localizer retry, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
	if !strings.Contains(msg, "repo_map") || strings.Contains(msg, "window_sweep") {
		t.Fatalf("genuine source read must keep source wording:\n%s", msg)
	}
}
