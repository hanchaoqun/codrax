package agent

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the unchanged natural resource fixture through actual native query,
// accepted tool handoff, shared projection and actual Finalizer messages. A
// numeric fallback is still a legitimate estimate, never a measured zero-loss
// result just because the optional note preview ran out of space.
func TestHMC352PublicValueContextKeepsFallbackInActualFinalizer(t *testing.T) {
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_native_resource_identity/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	ctx, results := hmc346QueryContext(t, path, 100, .0005, .0055)
	before, _ := json.Marshal(results)
	causal := hmc218CausalProjection(t, ctx)
	var record *types.ObservationRecord
	for i := range results[0].Observations {
		if results[0].Observations[i].ClaimKey == "compute_supply_balance" {
			record = &results[0].Observations[i]
		}
	}
	if record == nil || record.Value != "1.000" {
		t.Fatal("actual numeric fallback prerequisite absent")
	}
	raw, _ := json.Marshal(record)
	var wire map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire["value_context"]) == 0 {
		t.Error("native numeric estimate lost its method/availability context")
	}
	opts := types.DefaultObservationPromptProjectionOptions(1)
	opts.NoteLimit, opts.PrincipalNoteLimit, opts.OriginSpecificSupportingNoteLimit = 1, 1, 1
	opts.NoteMaxLen, opts.SummaryMaxLen = 1, 1
	compact, _ := json.Marshal(types.ProjectObservationPromptRecords([]types.ObservationRecord{*record}, nil, nil, opts))
	_, actual := finalizerReaderFieldMessages(t, ctx)
	for name, text := range map[string]string{"native": string(raw), "tiny_projection": string(compact), "actual_finalizer": actual} {
		for _, want := range []string{"frequency_weighted_estimate", "frequency_unknown_running_ms=", "unmeasured"} {
			if !strings.Contains(text, want) {
				t.Errorf("%s dropped complete value qualification %q", name, want)
			}
		}
	}
	if complete := types.FormatObservationValueContexts(record.ValueContext); complete == "" || !strings.Contains(actual, complete) {
		t.Error("actual Finalizer did not retain the native value context as one complete unit")
	}
	after, _ := json.Marshal(results)
	if !bytes.Equal(before, after) || !bytes.Equal(causal, hmc218CausalProjection(t, ctx)) {
		t.Fatal("context rendering changed native results or causal projection")
	}
}

func TestHMC352CheckpointAndWholeRowPlanKeepSummaryValueContext(t *testing.T) {
	r := types.ObservationRecord{ID: "native:q1", Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", ClaimKey: "cost", Summary: "estimated total 0", SourceRef: types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "input.trace"}, ValueContext: []types.ObservationValueContext{{Fields: "total", Method: "model", Detail: strings.Repeat("complete source limit ", 40)}}}
	ctx := &types.AgentContext{Mutable: types.NewMutableState("explain")}
	ctx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, Observations: []types.ObservationRecord{r}}}})
	want := types.FormatObservationValueContexts(r.ValueContext)
	if text := renderToolHistoryObservationCheckpoint(ctx, 4); !strings.Contains(text, want) {
		t.Fatal("checkpoint truncated the qualification or required a scalar Value")
	}
	projected := types.ProjectObservationPromptRecords([]types.ObservationRecord{r}, nil, nil, types.DefaultObservationPromptProjectionOptions(1))
	plan := planAnswerDocObservationRecord(projected[0], "", "")
	var b strings.Builder
	plan.write(&b, plan.source)
	if !strings.Contains(b.String(), want) || plan.bytes(plan.source) != b.Len() {
		t.Fatal("record budget omitted qualification bytes or summary-only context")
	}
}
