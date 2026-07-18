package tool

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// LENSBURN-FIX pins (§29.122 / §29.127, spec docs/design/lensburn_eval_20260717.md).
//
// 病A: HasObservationOnlyRuntimeArtifact only reads perf/log triage bundles;
// large traces (> perf_triage_llm_max_bytes, 512KiB default) skip bundle
// materialization entirely, so both source-inventory guards in emit_analysis
// went blind and trace-only runs white-minted a source_inventory_profile.
// 病B: an empty-shelf lens returned the zero observation, so lens execution
// was structurally unrecognizable and completion/scheduling gates burned
// rounds re-demanding an already-executed lens.

// lensburnLargeTracePreflight mirrors the Run-entry preflight the orchestrator
// builds for an attached large trace (runtimeArtifactPreflightProfileForRun:
// attachment carrier, deterministic byte count) — the production state chain
// after perf_triager's size gate skipped the bundle.
func lensburnLargeTracePreflight() types.RuntimeArtifactPreflightProfile {
	return types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
		Active:                   true,
		SourceNavigationOptional: true,
		ReasonCode:               types.RuntimeArtifactPreflightReasonDetected,
		Artifacts: []types.RuntimeArtifactPreflightArtifact{{
			Kind:    "trace",
			Source:  "/tmp/big.systrace",
			Bytes:   700 << 10, // > 512KiB perf_triage size gate: bundle never materializes
			Carrier: "attachment",
		}},
	})
}

func lensburnObservationOnlyPolicy() *types.ExternalObservationPolicy {
	return &types.ExternalObservationPolicy{
		CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
		ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:      []string{"only analyse the attached trace"},
		Confidence:        0.9,
	}
}

func lensburnTypedEnumerationRequestModel(policy *types.ExternalObservationPolicy) types.RequestModel {
	return types.RequestModel{
		Intent:                    types.IntentEnumerate,
		Predicates:                types.SemanticPredicates{IsCategoryEnumeration: true},
		ExternalObservationPolicy: policy,
	}
}

func TestLensburnSynthesizeGuard_LargeTracePreflightRestoresSight(t *testing.T) {
	rm := lensburnTypedEnumerationRequestModel(lensburnObservationOnlyPolicy())
	// Disease form documented: the bundle-only predicate is blind on exactly
	// this shape (no bundle materialized for the large trace), which is why
	// the guard needs the preflight carrier.
	if rm.HasObservationOnlyRuntimeArtifact() {
		t.Fatalf("fixture drift: bundle-only predicate should be blind on the bundle-less large-trace form")
	}
	ctx := &types.BusContext{RuntimeArtifactPreflight: lensburnLargeTracePreflight()}
	if warning := synthesizeSourceInventoryProfileForTypedEnumeration(ctx, &rm, "list every module in the trace", nil); warning != "" {
		t.Fatalf("sighted guard must suppress synthesis on trace-only observation-only run, got warning %q", warning)
	}
	if rm.SourceInventoryProfile != nil {
		t.Fatalf("sighted guard must not white-mint source_inventory_profile, got %+v", rm.SourceInventoryProfile)
	}
}

func TestLensburnSynthesizeGuard_UnrelatedFormStillSynthesizesIdentically(t *testing.T) {
	// 无关形负臂: no attached artifact (empty preflight) — the synthesis path
	// is byte-identical to the historic behavior, policy present or not.
	for _, policy := range []*types.ExternalObservationPolicy{nil, lensburnObservationOnlyPolicy()} {
		rm := lensburnTypedEnumerationRequestModel(policy)
		warning := synthesizeSourceInventoryProfileForTypedEnumeration(&types.BusContext{}, &rm, "list every module", nil)
		if warning != "synthesized source_inventory_profile from typed source-enumeration request shape" {
			t.Fatalf("unrelated form must keep the historic synthesis warning, got %q (policy=%v)", warning, policy != nil)
		}
		if rm.SourceInventoryProfile == nil || !rm.SourceInventoryProfile.IsSourceInventory ||
			rm.SourceInventoryProfile.Confidence != 0.45 ||
			rm.SourceInventoryProfile.Rationale != "synthesized from typed source-enumeration request shape" {
			t.Fatalf("unrelated form must keep the historic synthesized profile, got %+v", rm.SourceInventoryProfile)
		}
	}
}

func TestLensburnSynthesizeGuard_MaterializedBundleKeepsAuthority(t *testing.T) {
	// A materialized perf bundle keeps full authority over its lane: the
	// preflight arm only covers the bundle-ABSENT blindness, so a small trace
	// whose bundle resolved into current-source files still synthesizes.
	rm := lensburnTypedEnumerationRequestModel(lensburnObservationOnlyPolicy())
	rm.PerfTrace = &types.PerfBundle{}
	ctx := &types.BusContext{RuntimeArtifactPreflight: lensburnLargeTracePreflight()}
	if warning := synthesizeSourceInventoryProfileForTypedEnumeration(ctx, &rm, "list every module", nil); warning == "" {
		t.Fatalf("bundle-present form must keep bundle authority (no preflight override)")
	}
	if rm.SourceInventoryProfile == nil {
		t.Fatalf("bundle-present form must keep the historic synthesis")
	}
}

func TestLensburnSynthesizeGuard_ZeroCurrentSourceRepoCensusArm(t *testing.T) {
	// A2: the deterministic run-entry census alone (no policy needed) proves
	// the checkout has zero current-source files — the obligation is
	// structurally unsatisfiable, so nothing is synthesized.
	preflight := lensburnLargeTracePreflight()
	preflight.RepoSourceCensus = types.RuntimeArtifactRepoSourceCensus{
		Completed:     true,
		SourceFiles:   0,
		ArtifactFiles: 1,
	}
	rm := lensburnTypedEnumerationRequestModel(nil)
	ctx := &types.BusContext{RuntimeArtifactPreflight: preflight}
	if warning := synthesizeSourceInventoryProfileForTypedEnumeration(ctx, &rm, "list every module", nil); warning != "" || rm.SourceInventoryProfile != nil {
		t.Fatalf("zero-current-source census must suppress synthesis, got warning=%q profile=%+v", warning, rm.SourceInventoryProfile)
	}
	// Census gate stays precise: an incomplete walk keeps the arm inert.
	preflight.RepoSourceCensus.Completed = false
	rm = lensburnTypedEnumerationRequestModel(nil)
	ctx = &types.BusContext{RuntimeArtifactPreflight: preflight}
	if warning := synthesizeSourceInventoryProfileForTypedEnumeration(ctx, &rm, "list every module", nil); warning == "" || rm.SourceInventoryProfile == nil {
		t.Fatalf("incomplete census must stay inert (synthesis proceeds)")
	}
}

func TestLensburnDropGuard_LargeTracePreflightWithdrawsModelProfile(t *testing.T) {
	// h6 double-source form: the model self-emitted a source_inventory_profile
	// on a trace-only observation-only run; the withdrawal guard was equally
	// blind without a bundle.
	rm := lensburnTypedEnumerationRequestModel(lensburnObservationOnlyPolicy())
	rm.SourceInventoryProfile = &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		Confidence:        0.9,
	}
	ctx := &types.BusContext{RuntimeArtifactPreflight: lensburnLargeTracePreflight()}
	dropped, warning := dropSourceInventoryProfileForObservationOnlyRuntime(ctx, &rm)
	if !dropped || warning == "" || rm.SourceInventoryProfile != nil {
		t.Fatalf("sighted withdrawal guard must drop the model-emitted profile, got dropped=%v warning=%q profile=%+v", dropped, warning, rm.SourceInventoryProfile)
	}
	// 无关形负臂: without the preflight artifact the guard behaves exactly as
	// before (no drop on the bundle-less form).
	rm = lensburnTypedEnumerationRequestModel(lensburnObservationOnlyPolicy())
	rm.SourceInventoryProfile = &types.SourceInventoryProfile{IsSourceInventory: true, Confidence: 0.9}
	if dropped, _ := dropSourceInventoryProfileForObservationOnlyRuntime(&types.BusContext{}, &rm); dropped || rm.SourceInventoryProfile == nil {
		t.Fatalf("empty-preflight form must keep historic no-drop behavior")
	}
}

func lensburnEmptyShelfContext(t *testing.T, stage types.PipelineStage) *types.BusContext {
	t.Helper()
	ctx := sourceInventoryTestContext(t.TempDir(), testGraphWithFiles(nil), ".", &types.SourceInventoryProfile{
		IsSourceInventory: true,
		TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		Confidence:        0.9,
	})
	ctx.PipelineStage = stage
	return ctx
}

func lensburnEmptyShelfQuery() types.SourceInventoryLensQuery {
	return types.SourceInventoryLensQuery{
		Path:          ".",
		Scopes:        []string{"."},
		Roles:         []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
		IncludeCounts: true,
	}
}

func TestLensburnEmptyShelfLens_MintsExecutedEmptyCredentialAndStopsBurn(t *testing.T) {
	ctx := lensburnEmptyShelfContext(t, types.StageExplore)

	// 修前病形: before the lens runs the gate correctly demands it, and the
	// SAME downgrade is re-issued on every attempt (the burn loop's round
	// shape — nothing in the pre-fix state could ever change the verdict).
	if gap := SourceInventoryLensExecutionGapForContext(ctx); !gap.Blocking {
		t.Fatalf("before any lens run the execution gap must block, got %+v", gap)
	}
	first := sourceInventoryLensExecutionDowngrade(ctx, nil)
	second := sourceInventoryLensExecutionDowngrade(ctx, nil)
	if first == "" || first != second {
		t.Fatalf("pre-lens downgrade must be stable and non-empty (burn shape), first=%q second=%q", first, second)
	}

	// The lens executes over the empty shelf.
	obs := PublishSourceInventoryObservationFromLens(ctx, lensburnEmptyShelfQuery())
	if !obs.LensExecutedEmpty() {
		t.Fatalf("empty-shelf lens must mint the typed executed-empty carrier, got %+v", obs)
	}
	if obs.IsActive() {
		t.Fatalf("executed-empty carrier must not fabricate row content, got %+v", obs)
	}
	if !types.SourceInventoryLensExecuted(obs) {
		t.Fatalf("successfully executed empty lens must count as executed, got %+v", obs)
	}

	// 完成门: the execution gap is satisfied — the downgrade is not re-issued.
	if gap := SourceInventoryLensExecutionGapForContext(ctx); gap.Blocking {
		t.Fatalf("executed-empty lens must satisfy the execution gap, got %+v", gap)
	}
	if downgrade := sourceInventoryLensExecutionDowngrade(ctx, nil); downgrade != "" {
		t.Fatalf("completion gate must stop re-issuing the lens downgrade, got %q", downgrade)
	}

	// 调度侧: env.SourceInventoryLensExecuted (orchestrator.go read-scheduler
	// env) is exactly this expression — the lens window stops re-forming.
	if !types.SourceInventoryLensExecuted(types.SourceInventoryObservationFromMutable(ctx.Mutable)) {
		t.Fatalf("scheduler-side lens predicate must heal from the persisted executed-empty carrier")
	}
}

func TestLensburnEmptyShelfLens_FailedLensDoesNotImpersonateExecution(t *testing.T) {
	// 负臂③: a FAILED repo_map call carrying the typed executed-empty
	// observation is not an execution credential; a successful one is.
	carrier := types.SourceInventoryObservation{
		Active:     true,
		Complete:   true,
		Provenance: []string{"repo_lens:tool_query", "repo_lens:stage:explore"},
		Lens:       []string{"lens_executed_empty"},
		Execution:  &types.SourceInventoryExecutionState{LensExecutedEmpty: true},
	}
	ctx := lensburnEmptyShelfContext(t, types.StageExplore)
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName:        "repo_map",
		Success:         false,
		Summary:         "repo_map failed",
		SourceInventory: &carrier,
	})
	if gap := SourceInventoryLensExecutionGapForContext(ctx); !gap.Blocking {
		t.Fatalf("failed lens tool result must not satisfy the execution gap, got %+v", gap)
	}
	ctx.Mutable.ResetDispatchToolResults()
	ctx.Mutable.AppendDispatchToolResult(types.ToolResult{
		ToolName:        "repo_map",
		Success:         true,
		Summary:         "Repo Lens: no source-inventory observation is available for the requested typed scope/role slice.",
		SourceInventory: &carrier,
	})
	if gap := SourceInventoryLensExecutionGapForContext(ctx); gap.Blocking {
		t.Fatalf("successful executed-empty lens tool result must satisfy the execution gap, got %+v", gap)
	}
}

func TestLensburnEmptyShelfLens_AnalyzeStageLensDoesNotCount(t *testing.T) {
	// 负臂③: the analyzer-stage provenance discipline is unchanged — an
	// empty lens minted during analyze never counts as an executed lens.
	ctx := lensburnEmptyShelfContext(t, types.StageAnalyze)
	obs := PublishSourceInventoryObservationFromLens(ctx, lensburnEmptyShelfQuery())
	if !obs.LensExecutedEmpty() {
		t.Fatalf("analyze-stage empty lens still mints the shape carrier, got %+v", obs)
	}
	if types.SourceInventoryLensExecuted(obs) {
		t.Fatalf("analyze-stage empty lens must not count as executed")
	}
	if gap := SourceInventoryLensExecutionGapForContext(ctx); !gap.Blocking {
		t.Fatalf("analyze-stage empty lens must not satisfy the execution gap, got %+v", gap)
	}
}

func TestLensburnEmptyShelfLens_RenderBytesUnchanged(t *testing.T) {
	// 无关形字节恒等: the executed-empty carrier renders byte-identically to
	// the historic zero observation — the fix lives entirely on the typed
	// lane, never on the model-visible surface.
	ctx := lensburnEmptyShelfContext(t, types.StageExplore)
	obs := PublishSourceInventoryObservationFromLens(ctx, lensburnEmptyShelfQuery())
	if !obs.LensExecutedEmpty() {
		t.Fatalf("fixture drift: expected executed-empty carrier, got %+v", obs)
	}
	got := RenderSourceInventoryObservationView(obs, lensburnEmptyShelfQuery())
	want := RenderSourceInventoryObservationView(types.SourceInventoryObservation{}, lensburnEmptyShelfQuery())
	if got != want {
		t.Fatalf("executed-empty render must be byte-identical to the zero observation:\n got: %q\nwant: %q", got, want)
	}
}

// Incorporated adversarial-review probe (LENSBURN-FIX fix round, item 1): the
// "seventh loss point". Production chain: the empty-shelf lens mints the
// executed-empty carrier, then the model runs one successful non-recursive
// list_files over a real artifact file. The historic
// PublishSourceInventoryObservationFromToolObservation arm replaced the durable
// carrier wholesale whenever current was not IsActive() — and the
// executed-empty carrier is never IsActive() — so that single list_files
// destroyed the execution credential, env.SourceInventoryLensExecuted flipped
// back to false across rounds, and the lens window / completion gap re-formed
// (the exact burn loop this batch exists to kill).
func TestLensburnEmptyShelfLens_ListFilesAfterEmptyLensKeepsCredential(t *testing.T) {
	ctx := lensburnEmptyShelfContext(t, types.StageExplore)
	// Mint the executed-empty carrier through the production path.
	obs := PublishSourceInventoryObservationFromLens(ctx, lensburnEmptyShelfQuery())
	if !obs.LensExecutedEmpty() || !types.SourceInventoryLensExecuted(obs) {
		t.Fatalf("fixture drift: expected executed-empty credential, got %+v", obs)
	}
	if !types.SourceInventoryLensExecuted(types.SourceInventoryObservationFromMutable(ctx.Mutable)) {
		t.Fatalf("fixture drift: durable credential missing right after mint")
	}

	// A real artifact file exists in the trace-only checkout (e.g. the trace
	// itself, or any non-source file); the model runs a plain list_files.
	if err := os.WriteFile(filepath.Join(ctx.RepoRoot, "big.systrace"), []byte("trace"), 0o644); err != nil {
		t.Fatal(err)
	}
	ok := PublishSourceInventoryObservationFromToolObservation(ctx, types.ToolResult{
		ToolName: "list_files",
		Success:  true,
		PathDiscovery: &types.ToolPathDiscovery{
			Kind:           types.ToolPathDiscoveryKindListFiles,
			Path:           ".",
			CandidateFiles: []string{"big.systrace"},
		},
	})
	if !ok {
		t.Fatalf("probe drift: list_files observation did not publish")
	}
	durable := types.SourceInventoryObservationFromMutable(ctx.Mutable)
	if !durable.IsActive() {
		t.Fatalf("list_files direct-child universe must survive the merge, got %+v", durable)
	}
	if !types.SourceInventoryLensExecuted(durable) {
		t.Fatalf("a successful list_files after the empty lens must not destroy the execution credential (historic replace arm in PublishSourceInventoryObservationFromToolObservation), got %+v", durable)
	}
	// And the completion gap stays satisfied — the burn loop does not return.
	if gap := SourceInventoryLensExecutionGapForContext(ctx); gap.Blocking {
		t.Fatalf("execution gap must stay satisfied after the list_files publication, got %+v", gap)
	}
}
