package agent

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

type hmc364RefLimit struct {
	Refs []int  `json:"refs"`
	Time string `json:"query_window"`
	TID  int    `json:"tid"`
	Use  string `json:"use_limit"`
}

func hmc364MeasureContext(t *testing.T, path string, start, end float64) *types.AgentContext {
	t.Helper()
	rm := types.RequestModel{Language: "zh", PerfTrace: &types.PerfBundle{},
		RuntimeQuestionProfile:      &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}},
		RuntimeTargetProfile:        &types.RuntimeTargetProfile{Declaration: types.RuntimeTargetDeclarationNoNamedTarget},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end}}
	ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", AgentName: types.AgentFinalizer, Stage: types.StageFinalize,
		Mutable: types.NewMutableState("Compare confirmed scheduler counts and observed thread state in the window"), AnalysisIR: &types.AnalysisIR{RequestModel: rm}}
	ctx.Mutable.SetRequestModel(rm)
	hmc320BusinessQuery(t, ctx, path, map[string]any{"time_start": start, "time_end": end})
	return ctx
}

func hmc364RefsFor(t *testing.T, ctx *types.AgentContext, predicate string, view types.RuntimeMeasurementView) ([]types.AnswerRuntimeMeasurementReceipt, []types.InvestigationNativeRowReference) {
	t.Helper()
	contract := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
	var selections []types.AnswerRuntimeMeasurementReceipt
	var refs []types.InvestigationNativeRowReference
	for _, table := range contract.Choices() {
		matchesProducer := strings.HasSuffix(table.ObservationID, "#"+predicate) || strings.Contains(table.ObservationID, "#"+predicate+":")
		if !matchesProducer || table.View != view || len(table.Rows) == 0 {
			continue
		}
		selections = append(selections, types.AnswerRuntimeMeasurementReceipt{ObservationID: table.ObservationID, View: table.View})
		refs = append(refs, types.InvestigationNativeRowReference{ObservationID: table.ObservationID, View: table.View, RowIndices: []int{0}})
	}
	return selections, refs
}

func hmc364CompleteUnit(t *testing.T, ctx *types.AgentContext, selectors []types.AnswerRuntimeMeasurementReceipt, unit types.InvestigationInterpretationUnit) *types.InvestigationNativeFactHandoff {
	t.Helper()
	params, err := json.Marshal(map[string]any{"reason": "Native facts are ready.", "confidence": "high", "result_kind": "resolved", "native_fact_handoff": map[string]any{
		"runtime_measurements": selectors, "interpretation_role": "independent_analysis", "interpretation_units": []types.InvestigationInterpretationUnit{unit},
	}})
	if err != nil {
		t.Fatal(err)
	}
	r, err := (&tool.EmitInvestigationComplete{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !r.Success {
		t.Fatalf("public completion: %v %s", err, r.Summary)
	}
	h := ctx.Mutable.AcceptedInvestigationNativeFactHandoff()
	if h == nil || len(h.InterpretationUnits) != 1 || !reflect.DeepEqual(h.InterpretationUnits[0], unit) {
		t.Fatal("the producer changed model-owned interpretation or its original addresses")
	}
	return h
}

func hmc364UnitLimits(t *testing.T, prompt string, unit types.InvestigationInterpretationUnit) []hmc364RefLimit {
	t.Helper()
	raw, _ := json.Marshal(unit)
	_, tail, ok := strings.Cut(prompt, "- unit[0]: "+string(raw)+"\n\n")
	if !ok {
		t.Fatalf("whole accepted unit (%d JSON bytes) and its original boundaries did not reach the actual request", len(raw))
	}
	_, nearby, _ := strings.Cut(tail, "\n\n")
	nearby, adjacent := strings.CutPrefix(nearby, "  native_reference_limits=")
	line, _, _ := strings.Cut(nearby, "\n")
	var limits []hmc364RefLimit
	if !adjacent || json.Unmarshal([]byte(line), &limits) != nil || len(limits) == 0 || !strings.HasPrefix(tail, "  native_reference_status=current_exact_rows;") {
		t.Fatal("actual adapter received a valid address but no source-owned interpretation limits beside the unit")
	}
	return limits
}

func hmc364AssertLimits(t *testing.T, ctx *types.AgentContext, selections []types.AnswerRuntimeMeasurementReceipt, unit types.InvestigationInterpretationUnit, refCount, tid int, window, meaning string) {
	t.Helper()
	before := string(hmc218CausalProjection(t, ctx))
	h := hmc364CompleteUnit(t, ctx, selections, unit)
	assert := func(prompt string) {
		t.Helper()
		got := hmc364UnitLimits(t, prompt, unit)
		if len(got) != 1 || len(got[0].Refs) != refCount || got[0].Time != window || got[0].TID != tid || !strings.Contains(got[0].Use, meaning) {
			t.Fatalf("wrong local boundary: %+v", got)
		}
		for i, n := range got[0].Refs {
			if i != n {
				t.Fatalf("boundary lost reference indices: %+v", got)
			}
		}
	}
	assert(dependencyObservationMessages(t, ctx.ShallowClone()))
	extractor := (&extractorEvaluator{}).BuildInitialInstruction(ctx.ShallowClone(), nil)
	if raw, _ := json.Marshal(unit); len(raw) > 900 {
		if !strings.Contains(extractor, "0 of 1 declared units shown whole") || strings.Contains(extractor, "- unit[0]:") || strings.Contains(extractor, "native_reference_limits=") {
			t.Fatal("oversize unit did not remain explicitly and atomically omitted in Extractor")
		}
	} else {
		assert(extractor)
	}
	view := types.BuildAnswerSemanticViewForAgentContext(ctx)
	contexts := investigationNativeReferenceContexts(h, view.RuntimeMeasurementContract)
	preview := previewInvestigationInterpretationWithReferenceContexts(h, 1800, func(s string) string { return s }, true, contexts)
	if preview.publishedBytes > 1800 || !strings.Contains(preview.text, "native_reference_limits=") {
		t.Fatal("bounded native unit lost its atomic applicability limit")
	}
	stale := h.Clone()
	stale.InterpretationUnits[0].NativeRefs[0].RowIndices[0]++
	if refs := investigationNativeReferenceContexts(stale, view.RuntimeMeasurementContract); len(refs) != 0 {
		t.Fatal("changed address inherited the original reference limit")
	}
	if before != string(hmc218CausalProjection(t, ctx)) {
		t.Fatal("reference teaching changed Trace causal projection")
	}
}

func TestHMC364ActualReferenceLimitsForCountsAndThreadStates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "scheduler.systrace")
	if err := os.WriteFile(path, []byte(schedulerConcurrencyContextTrace), 0600); err != nil {
		t.Fatal(err)
	}
	const window = "[1,1.01) s"
	t.Run("system_population", func(t *testing.T) {
		ctx := hmc364MeasureContext(t, path, 1, 1.01)
		sel, refs := hmc364RefsFor(t, ctx, "scheduler_concurrency", types.RuntimeMeasurementSummary)
		if len(refs) != 2 {
			t.Fatalf("expected two independent scheduler views: %+v", refs)
		}
		unit := types.InvestigationInterpretationUnit{Text: "The running level may indicate saturation.",
			Boundaries: []string{"Capture completeness is unproven."}, NativeRefs: refs}
		hmc364AssertLimits(t, ctx, sel, unit, 2, 0, window, "Zero contribution")
	})
	t.Run("thread_states", func(t *testing.T) {
		ctx := hmc364MeasureContext(t, path, 1, 1.01)
		hmc320BusinessQuery(t, ctx, path, map[string]any{"view": "thread_timeline", "pid": 300, "time_start": 1.0, "time_end": 1.01})
		sel, refs := hmc364RefsFor(t, ctx, tool.TraceThreadTimelinePredicate, types.RuntimeMeasurementTimeline)
		if len(refs) != 1 {
			t.Fatalf("expected a native thread timeline: %+v", refs)
		}
		unit := types.InvestigationInterpretationUnit{Text: "This might indicate a delay before execution.", Boundaries: []string{"Do not infer a cause from the state alone."}, NativeRefs: refs}
		hmc364AssertLimits(t, ctx, sel, unit, 1, 300, window, "cannot rule out another")
	})
}

func TestHMC364NativeReferenceLimitsShareBudgetWithCompleteUnit(t *testing.T) {
	i, j, k := 0, 1, 2
	const limit = "Sample points do not prove time running."
	contexts := map[int][]investigationNativeReferenceContext{0: {
		{ReferenceIndex: &i, QueryWindow: "[1,2) s", UseLimit: limit, Label: "first"},
		{ReferenceIndex: &j, QueryWindow: "[1,2) s", UseLimit: limit, Label: "second"},
		{ReferenceIndex: &k, QueryWindow: "[2,3) s", UseLimit: limit, Label: "third"},
	}}
	unit := types.InvestigationInterpretationUnit{Text: "Sample locations suggest a follow-up.", Boundaries: []string{"No conclusion about execution."}}
	for _, id := range []string{"one", "two", "three"} {
		unit.NativeRefs = append(unit.NativeRefs, types.InvestigationNativeRowReference{ObservationID: id, View: types.RuntimeMeasurementSummary, RowIndices: []int{0}})
	}
	h := &types.InvestigationNativeFactHandoff{InterpretationRole: types.InvestigationIndependentAnalysis, InterpretationUnits: []types.InvestigationInterpretationUnit{unit}}
	preview := func(budget int) investigationInterpretationPreview {
		return previewInvestigationInterpretationWithReferenceContexts(h, budget, func(s string) string { return s }, true, contexts)
	}
	full := preview(1800)
	base, _, ok := strings.Cut(full.text, "  native_reference_context=")
	if !ok {
		t.Fatal("complete optional navigation missing from wide budget")
	}
	tight := preview(len(base))
	got := hmc364UnitLimits(t, tight.text, unit)
	if tight.publishedBytes != len(base) || strings.Contains(tight.text, "  native_reference_context=") || len(got) != 2 ||
		!reflect.DeepEqual(got[0].Refs, []int{0, 1}) || !reflect.DeepEqual(got[1].Refs, []int{2}) || got[0].Time == got[1].Time {
		t.Fatalf("same-scope deduplication or tight budget lost the unit's reference boundaries: %+v", got)
	}
	oneByteShort := preview(len(base) - 1)
	if oneByteShort.publishedBytes != 0 || strings.Contains(oneByteShort.text, "native_reference_status=") || !strings.Contains(oneByteShort.text, "0 of 1 declared units shown whole") {
		t.Fatal("the unit was shown without its required limits or was silently truncated")
	}
}

func TestHMC364WholeCapturePreservesArchivedUnitLimits(t *testing.T) {
	path, archived := os.Getenv("CODRAX_HMC364_NETEASE"), os.Getenv("CODRAX_HMC364_ARCHIVED_UNIT")
	if path == "" || archived == "" {
		t.Skip("requires pinned complete NetEase trace and archived independent unit")
	}
	if sum, n := hmc333FullSourceHash(t, path); sum != "12349db5bbdc5a4343360d9633c7d6c36f18c8be5f39fb64e78b53c9e345bfe4" || n != 82838676 {
		t.Fatal("not the pinned original capture")
	}
	if sum, _ := hmc333FullSourceHash(t, archived); sum != "4f5015c2e8ec9ddc0ee00d42c17809ef4314e51a6a833fb463815707d35bb5f0" {
		t.Fatal("not the archived Finalizer INIT")
	}
	data, err := os.ReadFile(archived)
	if err != nil {
		t.Fatal(err)
	}
	_, rest, _ := strings.Cut(string(data), "- unit[0]: ")
	line, _, _ := strings.Cut(rest, "\n")
	var unit types.InvestigationInterpretationUnit
	if json.Unmarshal([]byte(line), &unit) != nil || len(unit.NativeRefs) != 2 || len(unit.SupportRefs) != 2 {
		t.Fatal("archived accepted unit not found")
	}
	const start, end, window = 5969.994941, 5970.622586, "[5969.994941,5970.622586) s"
	ctx := hmc364MeasureContext(t, path, start, end)
	timeline := hmc320BusinessQuery(t, ctx, path, map[string]any{"view": "thread_timeline", "pid": 36435, "time_start": start, "time_end": end})
	var native tracequery.Result
	for _, r := range timeline.Observations {
		if r.SourceRef.PayloadRef != "" {
			data, err := os.ReadFile(r.SourceRef.PayloadRef)
			if err != nil || json.Unmarshal(data, &native) != nil {
				t.Fatalf("cannot inspect native timeline: %v", err)
			}
			break
		}
	}
	if state := native.TargetWindowStates; state == nil || state.Thread.PID != 36435 || math.Abs(state.RunnableMs-128.273) > .0001 {
		t.Fatalf("real runnable duration changed or was confused with another state: %+v", native.TargetWindowStates)
	}
	sel, refs := hmc364RefsFor(t, ctx, "scheduler_concurrency", types.RuntimeMeasurementSummary)
	if len(refs) != 2 {
		t.Fatal("exact native groups are missing")
	}
	// Rebind the original selectors to the current source receipt; never edit
	// the archived model text/boundaries or imply that its words passed audit.
	unit.NativeRefs = refs
	for i := range refs {
		unit.SupportRefs[i] = refs[i].ObservationID
	}
	hmc364AssertLimits(t, ctx, sel, unit, 2, 0, window, "Zero contribution")
}
