package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func hmc325BoundaryInput(ctx *types.AgentContext) types.ObservationLedgerInput {
	return types.ObservationLedgerInputFromAgentContext(ctx, 64)
}

func hmc325BoundaryJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func hmc325BoundaryNative(t *testing.T) (*types.AgentContext, types.ToolResult) {
	t.Helper()
	ctx, path := hmc323BusinessBudgetContext(t)
	result := hmc325PublicBusinessQuery(t, ctx, path, 5.2, 5.312)
	if ctx.Mutable.NativeFactRetentionStatus().RetainedQueries == 0 {
		t.Fatal("real native query did not enter the generation-local retention lane")
	}
	return ctx, result
}

func TestHMC325NativeRetentionCopyOutKeepsPublicationTimeAndFacts(t *testing.T) {
	ctx, original := hmc325BoundaryNative(t)
	before := types.CompileObservationLedger(hmc325BoundaryInput(ctx))
	ctx.Mutable.ResetDispatchToolResults()
	input := hmc325BoundaryInput(ctx)
	if len(input.ToolResults) != 1 {
		t.Fatalf("one absent whole publication should be restored: %d", len(input.ToolResults))
	}
	copy := input.ToolResults[0]
	if !copy.Timestamp.Equal(original.Timestamp) || copy.Timestamp.IsZero() ||
		hmc325BoundaryJSON(t, copy.Observations) != hmc325BoundaryJSON(t, original.Observations) ||
		hmc325BoundaryJSON(t, copy.TraceEvidenceAuthority) != hmc325BoundaryJSON(t, original.TraceEvidenceAuthority) ||
		hmc325BoundaryJSON(t, copy.EnumerationAuthority) != hmc325BoundaryJSON(t, original.EnumerationAuthority) {
		t.Fatal("frozen native copy lost publication time, facts, or authority boundary")
	}
	after := types.CompileObservationLedger(input)
	if hmc325BoundaryJSON(t, before.Records) != hmc325BoundaryJSON(t, after.Records) {
		t.Fatal("restored ledger changed native records, including ObservedAt fallback")
	}
	// Every mutable public body is copy-out owned. Changing this consumer's
	// representation must not change the frozen native publication.
	copy.Observations[0].Value = "changed by a consumer"
	for i := range copy.Observations {
		if len(copy.Observations[i].RichNotes) > 0 {
			copy.Observations[i].RichNotes[0] = "changed native notes"
		}
		if len(copy.Observations[i].SupportRefs) > 0 {
			copy.Observations[i].SupportRefs[0] = "changed source"
		}
		if len(copy.Observations[i].StateAccounting) > 0 {
			copy.Observations[i].StateAccounting[0].SegmentCount++
		}
	}
	if copy.TraceEvidenceAuthority != nil {
		copy.TraceEvidenceAuthority.CausalConclusion = "changed authority"
	}
	if copy.EnumerationAuthority != nil && len(copy.EnumerationAuthority.Boundaries) > 0 {
		copy.EnumerationAuthority.Boundaries[0].Total++
	}
	again := hmc325BoundaryInput(ctx)
	if len(again.ToolResults) != 1 || hmc325BoundaryJSON(t, types.CompileObservationLedger(again).Records) != hmc325BoundaryJSON(t, before.Records) ||
		hmc325BoundaryJSON(t, again.ToolResults[0].TraceEvidenceAuthority) != hmc325BoundaryJSON(t, original.TraceEvidenceAuthority) ||
		hmc325BoundaryJSON(t, again.ToolResults[0].EnumerationAuthority) != hmc325BoundaryJSON(t, original.EnumerationAuthority) {
		t.Fatal("consumer mutation aliased the frozen native core")
	}
}

func TestHMC325NativeRetentionNeverRepairsChangedCurrentIncarnation(t *testing.T) {
	for _, mutation := range []string{"value", "query_scope", "query_window", "missing_tree_member", "empty_rows"} {
		t.Run(mutation, func(t *testing.T) {
			ctx, original := hmc325BoundaryNative(t)
			bad := original
			bad.Observations = append([]types.ObservationRecord(nil), original.Observations...)
			index := -1
			for i, r := range bad.Observations {
				if _, ok := tool.DecodeTraceBusinessTreeFact(r); ok {
					index = i
					break
				}
			}
			if index < 0 {
				t.Fatal("real producer omitted business-tree prerequisite")
			}
			source := bad.Observations[index].SourceRef
			switch mutation {
			case "value":
				bad.Observations[index].Value = "999999"
			case "query_scope":
				bad.Observations[index].SourceRef.QueryScopeID += "-changed"
			case "query_window":
				bad.Observations[index].SourceRef.QueryWindowEndTs += .001
			case "missing_tree_member":
				bad.Observations = append(bad.Observations[:index], bad.Observations[index+1:]...)
			case "empty_rows":
				bad.Observations = nil
			}
			ctx.Mutable.ResetDispatchToolResults()
			ctx.Mutable.AppendDispatchToolResult(bad)
			input := hmc325BoundaryInput(ctx)
			if len(input.ToolResults) != 1 || hmc325BoundaryJSON(t, input.ToolResults[0].Observations) != hmc325BoundaryJSON(t, bad.Observations) {
				t.Fatal("retention silently repaired/replaced the current producer incarnation")
			}
			ledger := types.CompileObservationLedger(input)
			var rows []types.ObservationRecord
			for _, r := range ledger.Records {
				if r.Predicate == types.TraceBusinessTreePredicate && r.SourceRef.QueryScopeID == source.QueryScopeID {
					rows = append(rows, r)
				}
			}
			if ledger.CurrentRuntimeQueryPopulation(source, types.TraceBusinessTreePredicate, rows) {
				t.Fatal("changed/partial current rows borrowed the retained original census")
			}
		})
	}
}

func TestHMC325NativeRetentionMemoOrFailureCannotHideRealPublication(t *testing.T) {
	for _, state := range []string{"memo", "failed", "cancelled"} {
		t.Run(state, func(t *testing.T) {
			ctx, original := hmc325BoundaryNative(t)
			current := original
			switch state {
			case "memo":
				current.ReusedFromRunMemo = true
			case "failed":
				current.Success = false
			case "cancelled":
				current.TraceViewCancellation = &types.TraceViewCancellation{}
			}
			ctx.Mutable.ResetDispatchToolResults()
			ctx.Mutable.AppendDispatchToolResult(current)
			input := hmc325BoundaryInput(ctx)
			if len(input.ToolResults) != 2 {
				t.Fatalf("nonfresh/non-success wrapper concealed the real retained publication: %d", len(input.ToolResults))
			}
			fresh := 0
			for _, r := range input.ToolResults {
				if r.Success && !r.ReusedFromRunMemo && r.TraceViewCancellation == nil {
					fresh++
					if hmc325BoundaryJSON(t, r.Observations) != hmc325BoundaryJSON(t, original.Observations) {
						t.Fatal("retained original was replaced by wrapper contents")
					}
				}
			}
			if fresh != 1 {
				t.Fatalf("expected one actual original, not a newly signed replay: %d", fresh)
			}
		})
	}
}

func TestHMC325NativeRetentionCannotRebuildFromJSONOrRetiredGeneration(t *testing.T) {
	ctx, original := hmc325BoundaryNative(t)
	var restored types.ToolResult
	if err := json.Unmarshal([]byte(hmc325BoundaryJSON(t, original)), &restored); err != nil {
		t.Fatal(err)
	}
	fresh := ctx.ShallowClone()
	fresh.Mutable = types.NewMutableState("same source is not the same producer generation")
	fresh.Mutable.SetRequestModel(fresh.AnalysisIR.RequestModel)
	fresh.Mutable.AppendDispatchToolResult(restored)
	if fresh.Mutable.NativeFactRetentionStatus().RetainedQueries != 0 {
		t.Fatal("JSON recreated private retention authority in a fresh mutable state")
	}
	fresh.Mutable.ResetDispatchToolResults()
	if len(hmc325BoundaryInput(fresh).ToolResults) != 0 {
		t.Fatal("JSON body returned as a retained native original")
	}
	ctx.Mutable.ResetTurnAArtifacts()
	ctx.Mutable.ResetDispatchToolResults()
	ctx.Mutable.AppendDispatchToolResult(original)
	if ctx.Mutable.NativeFactRetentionStatus().RetainedQueries != 0 {
		t.Fatal("old Go receipt revived a retired generation")
	}
	ctx.Mutable.ResetDispatchToolResults()
	if len(hmc325BoundaryInput(ctx).ToolResults) != 0 {
		t.Fatal("old generation leaked after history and dispatch were cleared")
	}
}

func TestHMC325NativeRetentionSamePathReplacementKeepsNewSourceVersion(t *testing.T) {
	ctx, path := hmc323BusinessBudgetContext(t)
	old := hmc325PublicBusinessQuery(t, ctx, path, 5.2, 5.312)
	before := ctx.Mutable.NativeFactRetentionStatus()
	if before.RetainedQueries == 0 {
		t.Fatal("old real publication was not retained")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// Same canonical path and exact bytes, different physical source version.
	// This is an isolated test input, never a replacement of supplied evidence.
	replacement := filepath.Join(filepath.Dir(path), "replacement-native-generation.systrace")
	if err := os.WriteFile(replacement, body, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if _, _, current := ctx.Mutable.ResolveTraceQueryWindowReplay(old.TraceQueryWindowReplay); current {
		t.Fatal("old physical source receipt survived same-path replacement")
	}
	ctx.Mutable.ResetDispatchToolResults()
	next := hmc325PublicBusinessQuery(t, ctx, path, 5.2, 5.312)
	if _, _, current := ctx.Mutable.ResolveTraceQueryWindowReplay(next.TraceQueryWindowReplay); !current {
		t.Fatal("fresh native query did not bind replacement source")
	}
	if ctx.Mutable.NativeFactRetentionStatus().RetainedQueries <= before.RetainedQueries {
		t.Fatal("same bytes/path caused new physical source version to collide with stale retention key")
	}
	ctx.Mutable.ResetDispatchToolResults()
	input := hmc325BoundaryInput(ctx)
	if len(input.ToolResults) != 1 {
		t.Fatalf("expected only replacement's current original, not stale or missing: %d", len(input.ToolResults))
	}
	if _, _, current := ctx.Mutable.ResolveTraceQueryWindowReplay(input.ToolResults[0].TraceQueryWindowReplay); !current ||
		!input.ToolResults[0].Timestamp.Equal(next.Timestamp) || hmc325BoundaryJSON(t, input.ToolResults[0].Observations) != hmc325BoundaryJSON(t, next.Observations) {
		t.Fatal("new generation cannot be consumed after public history disappears")
	}
}

func TestHMC325NativeRetentionKeepsPrivateIOIdentityAndRejectsEndpointMutation(t *testing.T) {
	ctx, path := hmc320BusinessFinalizer(t)
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	original := hmc320BusinessQuery(t, ctx, path, map[string]any{"view": "root_cause_rank", "pid": 100,
		"time_start": .999, "time_end": 1.051, "platform": "harmony", "trace_flavor": "harmony_hitrace"})
	// This helper also made a normal history snapshot. Remove only its public
	// contents, not the generation: the native lane must now stand on its own.
	ctx.Mutable.PublishTurnAArtifacts(types.TurnAArtifacts{}, types.TurnAHandoffHistoryCompacted)
	ctx.Mutable.ResetDispatchToolResults()
	input := hmc325BoundaryInput(ctx)
	if len(input.ToolResults) != 1 {
		t.Fatalf("native IO result was not retained independently: %d", len(input.ToolResults))
	}
	want := map[string]string{}
	for _, row := range original.Observations {
		if key, handled := types.TraceObservationIOPublicationKey(row); handled && key != "" {
			want[row.ID] = key
		}
	}
	if len(want) == 0 {
		t.Fatal("public native rank did not publish authenticated IO endpoints")
	}
	checked := 0
	for _, row := range input.ToolResults[0].Observations {
		if key := want[row.ID]; key != "" {
			got, handled := types.TraceObservationIOPublicationKey(row)
			if !handled || got != key || row.IORequestEndpoints == nil {
				t.Fatalf("retention dropped private IO request or resolved-thread identity: %s", row.ID)
			}
			row.IORequestEndpoints.CompleteTs += .5
			if changed, _ := types.TraceObservationIOPublicationKey(row); changed != "" {
				t.Fatal("changed physical request endpoint reused its private original seal")
			}
			checked++
		}
	}
	if checked != len(want) {
		t.Fatalf("lost native IO identities: got=%d want=%d", checked, len(want))
	}
	again := hmc325BoundaryInput(ctx)
	for _, row := range again.ToolResults[0].Observations {
		if key := want[row.ID]; key != "" {
			got, _ := types.TraceObservationIOPublicationKey(row)
			if got != key {
				t.Fatal("IO endpoint copy-out mutation aliased retained original")
			}
		}
	}
}

func TestHMC325NativeRetentionRejectsPrivateIOProjectionTransplant(t *testing.T) {
	ctx, path := hmc320BusinessFinalizer(t)
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	params, err := json.Marshal(map[string]any{"source": "path", "path": path, "view": "root_cause_rank", "pid": 100,
		"time_start": .999, "time_end": 1.051, "platform": "harmony", "trace_flavor": "harmony_hitrace"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), params)
	if err != nil || !result.Success || ctx.Mutable.NativeFactRetentionStatus().RetainedQueries != 0 {
		t.Fatalf("direct native call must precede admission: err=%v success=%v", err, result.Success)
	}
	index := -1
	for i, row := range result.Observations {
		if key, handled := types.TraceObservationIOPublicationKey(row); handled && key != "" && row.IORequestEndpoints != nil {
			index = i
			break
		}
	}
	if index < 0 {
		t.Fatal("public IO identity prerequisite absent")
	}
	originalEndpoint := result.Observations[index].IORequestEndpoints
	result.Observations[index].IORequestEndpoints = originalEndpoint.WithProjectionTarget(333, 444, "unrelated-thread")
	if key, _ := types.TraceObservationIOPublicationKey(result.Observations[index]); key != "" {
		t.Fatal("changed private projection unexpectedly retained native identity")
	}
	ctx.Mutable.AppendDispatchToolResult(result)
	if ctx.Mutable.NativeFactRetentionStatus().RetainedQueries != 0 {
		t.Fatal("JSON-identical endpoint with changed private projection entered the original fact pool")
	}
	ctx.Mutable.ResetDispatchToolResults()
	if len(hmc325BoundaryInput(ctx).ToolResults) != 0 {
		t.Fatal("private projection transplant returned as retained native publication")
	}
	// The untouched original receipt remains usable; rejection is not a
	// permanent source ban and no new signing operation is performed here.
	result.Observations[index].IORequestEndpoints = originalEndpoint
	ctx.Mutable.AppendDispatchToolResult(result)
	if ctx.Mutable.NativeFactRetentionStatus().RetainedQueries == 0 {
		t.Fatal("original native publication was permanently poisoned by a rejected transplant")
	}
}

// Explicit small field ownership inventories force a review when either
// shared envelope evolves. They do not infer authority from field names.
func TestHMC325NativeRetentionFieldOwnershipInventory(t *testing.T) {
	for _, shape := range []struct {
		value  any
		groups map[string][]string
	}{
		{types.ToolResult{}, map[string][]string{
			"native encoded core":                                              {"ToolName", "RawRef", "Observations", "TraceEvidenceAuthority", "EnumerationAuthority", "Timestamp", "Success"},
			"explicit native wire/private witness":                             {"TraceQuerySourceRead", "TraceQueryWindowReplay", "TraceEventSelectionAcquisition", "TraceStatistics", "RuntimeMeasurementPair", "TraceBusinessSpanCandidates", "TraceBusinessSpanRefs"},
			"admission rejection flag, not fresh facts":                        {"TraceViewCancellation", "ReusedFromRunMemo"},
			"other-tool/navigation/advice/telemetry, outside native fact core": {"Summary", "Repair", "Refinement", "Handoff", "PathDiscovery", "CommandMeasurement", "VCSHistory", "ReadCoverage", "RuntimeArtifactRead", "SourceInventory", "ArtifactReadNavigation", "TraceCatalogIndependentFailure", "RuntimeTimings", "OptionalCarrierOutcomes"},
		}},
		{types.ObservationRecord{}, map[string][]string{
			"native encoded core":                                 {"EventSearchInventory", "MeasurementSources", "StateAccounting", "ID", "Origin", "Producer", "Role", "GroundingPolicy", "ProvenanceLane", "ClaimAuthority", "SourceRef", "Span", "EvidenceKind", "AnchorKind", "EvidenceScope", "GroundingStatus", "ClaimKey", "Subject", "Predicate", "Object", "Value", "Unit", "Negative", "ResultCount", "Enumeration", "Summary", "RawExcerpt", "RichNotes", "ModelNotes", "SupportRefs", "SurfaceTerms", "ObservedAt", "Scope", "Confidence", "SystemSupplement"},
			"private immutable witness":                           {"runtimeRawMeasurementPublication", "runtimeMeasurementFactIdentity", "aggregateKind", "runtimeSamplingPublication", "runtimeResourcePublication", "ioRequestIdentity"},
			"explicit native wire plus private projection target": {"IORequestEndpoints"},
		}},
	} {
		t.Run(reflect.TypeOf(shape.value).Name(), func(t *testing.T) {
			classified := map[string]string{}
			for owner, fields := range shape.groups {
				for _, field := range fields {
					if classified[field] != "" {
						t.Fatalf("field %s has multiple owners", field)
					}
					classified[field] = owner
				}
			}
			typ := reflect.TypeOf(shape.value)
			for i := 0; i < typ.NumField(); i++ {
				name := typ.Field(i).Name
				if classified[name] == "" {
					t.Errorf("shared field %s requires an explicit retention/witness/exclusion decision", name)
				}
				delete(classified, name)
			}
			for name := range classified {
				t.Errorf("ownership inventory has stale field %s", name)
			}
		})
	}
}
