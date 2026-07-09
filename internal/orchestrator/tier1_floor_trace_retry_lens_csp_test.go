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
// plain/required-lane shape, whose byte-stability is pinned by CSR #64
// (csr_current_source_qualification_test.go P6 =
// TestCompileObservationLedger_PlainNegativeSearchKeepsCurrentSourceKind;
// the plain aggregate-fact terminal fallback itself is mutation-pinned in
// csp_current_source_exclude_test.go). Changing that fallback or the
// keep-arm order is an authority-semantics change that needs its own
// ruling; the fix here forks retry WORDING only.
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

// cspCmp792TraceBusContext mirrors the witness run shape: two preflight trace
// artifacts referenced in the request, a non-artifact-only cwd census, one
// deterministic trace_query runtime observation, a completed first
// investigation pass whose model-authored aggregate facts are retained, and
// zero source reads.
func cspCmp792TraceBusContext() *types.BusContext {
	mu := types.NewMutableState("对比分析两个 systrace 的 bindApplication 耗时差异主要原因")
	mu.SetTurnAArtifacts(types.TurnAArtifacts{})
	mu.AppendDispatchToolResult(tier1TraceQueryRuntimeToolResult())
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

// Pin 1 — witness replay: the retry still fires (the repair loop is
// design-final and stays), but the directive names the trace drill-down views
// the session can actually execute; every repo-navigation demand word is
// stripped.
func TestCheckTier1Floor_Cmp792PureTraceRetryNamesTraceDrillLenses(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	busCtx := cspCmp792TraceBusContext()
	o := &Orchestrator{busCtx: busCtx}

	// Arm-attribution sub-pins (conscious-flip markers): the keep-arm veto in
	// this shape rides on the polluted satisfied flag. A future root fix of
	// the plain-shape current_source fallback flips these first — re-read the
	// header comment before "fixing" this test.
	authority := types.BuildRuntimeSourceAnswerAuthoritySnapshotForBusContext(busCtx, types.ObservationLedger{})
	if !authority.CurrentSourceSatisfied || !authority.KeepsCurrentSourceLaneLoadBearing() {
		t.Fatalf("witness precondition drifted: keep-arm veto no longer rides on satisfied pollution: %+v", authority)
	}
	if busCtx.RuntimeArtifactPreflight.ZeroCurrentSourceRepo() {
		t.Fatal("witness precondition drifted: census must not be artifact-only")
	}
	if busCtx.Mutable.TraceQueryRuntimeObservationCount() == 0 {
		t.Fatal("witness precondition drifted: deterministic trace observation missing")
	}

	state := newGraphState(types.TaskGraph{ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 4}})
	msg, proceed, exhausted := o.checkTier1Floor(busCtx.AnalysisIR, state)
	if proceed || exhausted {
		t.Fatalf("repair loop must keep firing for the witness shape (design-final), proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
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
func TestCheckTier1Floor_SourceReadSessionKeepsRepoMapLensWording(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	busCtx := cspCmp792TraceBusContext()
	busCtx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles: []string{"pkg/handler.py"},
	})
	o := &Orchestrator{busCtx: busCtx}
	state := newGraphState(types.TaskGraph{ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 4}})

	msg, proceed, exhausted := o.checkTier1Floor(busCtx.AnalysisIR, state)
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
func TestCheckTier1Floor_CurrentRepoEvidenceKeepsSourceWording(t *testing.T) {
	prev := tool.CurrentGroundingPolicy()
	tool.SetGroundingPolicy(tool.DefaultGroundingPolicy())
	t.Cleanup(func() { tool.SetGroundingPolicy(prev) })

	busCtx := cspCmp792TraceBusContext()
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
	o := &Orchestrator{busCtx: busCtx}
	state := newGraphState(types.TaskGraph{ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 4}})

	msg, proceed, exhausted := o.checkTier1Floor(busCtx.AnalysisIR, state)
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

	msg, proceed, exhausted := o.checkTier1Floor(busCtx.AnalysisIR, state)
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

	// (b) Defense: artifact path in ReadFiles anyway → trace wording stays.
	busCtx := cspCmp792TraceBusContext()
	busCtx.RepoRoot = repoRoot
	busCtx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles:   []string{"attached_trace.txt"},
		ToolResults: []types.ToolResult{artifactRead},
	})
	o := &Orchestrator{busCtx: busCtx}
	state := newGraphState(types.TaskGraph{ExecutionPolicy: types.ExecutionPolicy{RetryBudget: 4}})
	msg, proceed, exhausted := o.checkTier1Floor(busCtx.AnalysisIR, state)
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
	msg, proceed, exhausted = o.checkTier1Floor(sourceCtx.AnalysisIR, state)
	if proceed || exhausted {
		t.Fatalf("expected localizer retry, proceed=%v exhausted=%v msg=%q", proceed, exhausted, msg)
	}
	if !strings.Contains(msg, "repo_map") || strings.Contains(msg, "window_sweep") {
		t.Fatalf("genuine source read must keep source wording:\n%s", msg)
	}
}
