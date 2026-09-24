package tool

import (
	"encoding/json"
	"strings"
	"testing"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPerfTriageMarkerEndpointActualPromptAndSchemaAgree(t *testing.T) {
	registry := skill.NewRegistry()
	skill.RegisterDefaults(registry)
	perfSkill, err := registry.Get("perf-triage-skill")
	if err != nil {
		t.Fatal(err)
	}
	inline := *perfSkill
	inline.ToolSuggestions = []string{"emit_perf_trace"}
	pc := promptctx.BuildPromptContext(&types.AgentContext{
		AgentName: types.AgentPerfTriager, Stage: types.StagePerfTriage,
		Objective: "Locate nested work and async markers",
		AttachedHitrace: "worker-7 (7) [001] .... 5.001000: tracing_mark_write: B|7|Business\n" +
			"worker-7 (7) [001] .... 5.002000: tracing_mark_write: E|7\n",
	}, &inline)
	var prompt strings.Builder
	for _, section := range append(pc.SystemSections, pc.UserSections...) {
		prompt.WriteString(section.Content)
		prompt.WriteByte('\n')
	}
	for _, want := range []string{
		"Preserve relevant marker rows independently as navigation anchors",
		"do not assign an unnamed end to a begin or build parent/child relationships in this pre-stage",
		"Deterministic queries perform B/E pairing on the same ftrace thread stack",
		"async pairing by marker pid + tag + cookie within the same source",
		"An async marker is not a synchronous child or proof of CPU work",
		"an unnamed end is retained at its own line without assigning a begin or duration",
		"copy only source-explicit measurements", "preserve their original value and unit",
	} {
		if !strings.Contains(prompt.String(), want) {
			t.Errorf("actual dispatch prompt lacks %q", want)
		}
	}
	for _, stale := range []string{
		"begin rows and pair them", "Pair async", "a matched B/E pair has raw endpoint rows",
		"e.g. span, threshold_check, line_anchor, duration, absence",
	} {
		if strings.Contains(prompt.String(), stale) {
			t.Errorf("actual dispatch still assigns pairing to the model: %q", stale)
		}
	}
	var schema struct {
		Properties map[string]struct {
			Items struct {
				Required   []string `json:"required"`
				Properties map[string]struct {
					Description string `json:"description"`
				} `json:"properties"`
			} `json:"items"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&EmitPerfTrace{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	observation := schema.Properties["observations"].Items
	if strings.Join(observation.Required, ",") != "subject,summary" {
		t.Fatalf("navigation must not require derived values: %+v", observation.Required)
	}
	for _, key := range []string{"start_ts_ms", "end_ts_ms", "duration_ms"} {
		if !strings.Contains(observation.Properties[key].Description, "Optional: copy only a source-explicit") ||
			!strings.Contains(observation.Properties[key].Description, "already in milliseconds") {
			t.Errorf("published %s schema encourages unit conversion: %+v", key, observation.Properties[key])
		}
	}
	if !strings.Contains(observation.Properties["kind"].Description, "pairing, parent/child relationships and derived measurements belong to deterministic queries") ||
		!strings.Contains(observation.Properties["summary"].Description, "not a model-computed pair") {
		t.Fatal("public schema and actual dispatch disagree about computation ownership")
	}
	if _, present := observation.Properties["authority"]; present {
		t.Fatal("model must not be allowed to mint validator authority")
	}
}

func TestEmitPerfTracePublicKeepsIndependentMarkerEndpoints(t *testing.T) {
	rows := []string{
		"worker-7 (7) [001] .... 5.001000: tracing_mark_write: B|7|Business",
		"worker-7 (7) [001] .... 5.002000: tracing_mark_write: B|7|Child",
		"worker-7 (7) [001] .... 5.002500: tracing_mark_write: S|7|AsyncWork|41",
		"worker-7 (7) [001] .... 5.003000: tracing_mark_write: E|7",
		"worker-9 (9) [002] .... 5.004000: tracing_mark_write: B|9|Business",
		"worker-7 (7) [001] .... 5.005000: tracing_mark_write: F|7|AsyncWork|41",
	}
	var observations []map[string]any
	for i, row := range rows {
		observations = append(observations, map[string]any{
			"kind": "marker_endpoint", "subject": "raw marker", "summary": "One independently observed endpoint.",
			"evidence": row, "line_start": i + 1, "line_end": i + 1, "tags": []string{"Business"},
		})
	}
	params, _ := json.Marshal(map[string]any{"meta": map[string]string{"source": "systrace"}, "observations": observations})
	ctx := &types.BusContext{Mutable: types.NewMutableState("endpoints"), AttachedHitrace: strings.Join(rows, "\n") + "\n"}
	result, err := (&EmitPerfTrace{}).Execute(ctx, params)
	if err != nil || !result.Success {
		t.Fatalf("independent endpoint payload failed: %v %+v", err, result)
	}
	bundle := ctx.Mutable.PerfTrace()
	if bundle == nil || len(bundle.Frames)+len(bundle.Janks)+len(bundle.Stalls) != 0 {
		t.Fatalf("endpoint payload invented span/causal measurements: %+v", bundle)
	}
	var retained int
	for _, obs := range bundle.Observations {
		if obs.Authority == types.PerfObservationAuthorityDeterministicValidator {
			continue // Existing unit/priority normalization retains its own authority.
		}
		if retained >= len(rows) || obs.Evidence != rows[retained] || obs.LineStart != retained+1 || obs.LineEnd != retained+1 ||
			!obs.IsNavigationOnly() || obs.StartTsMs != 0 || obs.EndTsMs != 0 || obs.DurationMs != 0 ||
			len(obs.Tags) != 1 || obs.Tags[0] != "Business" {
			t.Fatalf("independent business locator changed: %+v", obs)
		}
		retained++
	}
	if retained != len(rows) {
		t.Fatalf("source business clues lost: got %d want %d", retained, len(rows))
	}
}
