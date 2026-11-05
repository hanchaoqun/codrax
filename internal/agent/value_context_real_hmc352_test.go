package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC352WholeCapturesValueMeaning(t *testing.T) {
	input, out := os.Getenv("CODRAX_HMC352_REAL_INPUTS"), os.Getenv("CODRAX_HMC352_REAL_OUT")
	if input == "" {
		t.Skip("whole pinned captures not supplied")
	}
	if !filepath.IsAbs(input) || !filepath.IsAbs(out) || os.Mkdir(out, 0700) != nil {
		t.Fatal("absolute pinned sources and fresh output required")
	}
	var captures []struct {
		Name, Path, SHA256, BaselinePath, BaselineSHA256 string
		Bytes                                            int64
		PID                                              int
		Start, End                                       float64
	}
	raw, err := os.ReadFile(input)
	if err != nil || json.Unmarshal(raw, &captures) != nil || len(captures) != 2 {
		t.Fatal("two complete source pins required", err)
	}
	for _, capture := range captures {
		t.Run(capture.Name, func(t *testing.T) {
			sha, size := hmc333FullSourceHash(t, capture.Path)
			bsha, _ := hmc333FullSourceHash(t, capture.BaselinePath)
			if sha != capture.SHA256 || size != capture.Bytes || bsha != capture.BaselineSHA256 || filepath.Base(capture.Name) != capture.Name {
				t.Fatal("source or previous native baseline changed")
			}
			dir := filepath.Join(out, capture.Name)
			if os.Mkdir(dir, 0700) != nil {
				t.Fatal("fresh output required")
			}
			write := func(name string, value any) {
				raw, err := json.MarshalIndent(value, "", "  ")
				if err != nil || os.WriteFile(filepath.Join(dir, name), append(raw, '\n'), 0600) != nil {
					t.Fatal("retain native evidence", err)
				}
			}
			ctx, results := hmc346QueryContext(t, capture.Path, capture.PID, capture.Start, capture.End)
			write("actual-tool-results.json", results)
			var current map[string]any
			var observed *types.ObservationRecord
			for _, r := range results {
				for _, record := range r.Observations {
					if record.ClaimKey == "compute_supply_balance" {
						copy := record
						observed = &copy
						raw, err := os.ReadFile(record.SourceRef.PayloadRef)
						if err != nil || json.Unmarshal(raw, &current) != nil {
							t.Fatal("complete raw native payload", err)
						}
					}
				}
			}
			if observed == nil || len(observed.ValueContext) == 0 {
				t.Fatal("qualified source estimate absent")
			}
			write("current-native.json", current)
			var previous map[string]any
			prior, err := os.ReadFile(capture.BaselinePath)
			if err != nil || json.Unmarshal(prior, &previous) != nil {
				t.Fatal("previous whole-source native baseline", err)
			}
			oldBalance := previous["window_stats"].(map[string]any)["compute_supply_balance"]
			balance := current["window_stats"].(map[string]any)["compute_supply_balance"].(map[string]any)
			for _, item := range balance["per_cpu"].([]any) {
				cpu := item.(map[string]any)
				delete(cpu, "frequency_coverage_known")
				delete(cpu, "frequency_known_running_ms")
				delete(cpu, "frequency_unknown_running_ms")
			}
			if !reflect.DeepEqual(balance, oldBalance) {
				t.Fatal("qualification changed the existing numeric supply calculation")
			}
			before, _ := json.Marshal(results)
			causal := hmc218CausalProjection(t, ctx)
			_, message := finalizerReaderFieldMessages(t, ctx)
			if os.WriteFile(filepath.Join(dir, "actual-finalizer-context.txt"), []byte(message), 0600) != nil {
				t.Fatal("save actual composing context")
			}
			qualification := types.FormatObservationValueContexts(observed.ValueContext)
			unfocusedSelected := strings.Contains(message, observed.ID)
			if unfocusedSelected && !strings.Contains(message, qualification) {
				t.Fatal("whole-source Finalizer showed the selected calculation without its complete meaning")
			}
			// The state-only hmc346 request may legitimately leave this whole
			// supporting calculation unselected (the previous baseline does
			// too). Do not promote it into every task. Independently exercise
			// the actual composing consumer of this complete selected native
			// calculation, keeping original source/query coordinates.
			selected := ctx.ShallowClone()
			selected.Mutable = types.NewMutableState("说明这条已查询计算的数值和适用范围")
			selected.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
			selected.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{ToolResults: []types.ToolResult{{ToolName: "trace_query", Success: true, RawRef: observed.SourceRef.RawRef, Observations: []types.ObservationRecord{*observed}}}})
			_, selectedMessage := finalizerReaderFieldMessages(t, selected)
			if os.WriteFile(filepath.Join(dir, "selected-calculation-finalizer-context.txt"), []byte(selectedMessage), 0600) != nil || !strings.Contains(selectedMessage, qualification) {
				t.Fatal("actual selected-calculation Finalizer dropped or clipped its original value context")
			}
			after, _ := json.Marshal(results)
			afterSHA, afterSize := hmc333FullSourceHash(t, capture.Path)
			if !bytes.Equal(before, after) || !bytes.Equal(causal, hmc218CausalProjection(t, ctx)) || sha != afterSHA || size != afterSize {
				t.Fatal("rendering changed source, native results or causal projection")
			}
			write("scope-audit.json", map[string]any{"source": capture.Path, "source_bytes": size, "source_sha256": sha, "source_unchanged": true, "baseline_path": capture.BaselinePath, "baseline_sha256": bsha, "numeric_supply_equal_previous": true, "native_value_context_entries": len(observed.ValueContext), "unfocused_calculation_selected": unfocusedSelected, "selected_calculation_context_complete": true, "causal_projection_unchanged": true, "model_calls": 0, "scope": "Actual queries over whole originals, prior native numeric baseline and actual selected-calculation Finalizer. Unselected supporting rows are not promoted into the state-only task. No independent scheduler oracle or paid model evaluation."})
		})
	}
}
