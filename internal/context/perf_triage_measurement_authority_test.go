package context

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Pin the actual assembled prompt, not an isolated skill string: the no-tool
// reasoning rule and the skill must give the same measurement instructions.
func TestPerfTriagePromptKeepsLiteralMeasurementsAndDefersArithmetic(t *testing.T) {
	prompt := perfTriageInlineMeasurementPrompt(t, "app-100 (100) [000] .... 1.000000: tracing_mark_write: B|100|OpenDocument\napp-100 (100) [000] .... 1.050000: tracing_mark_write: E|100\n")
	for _, want := range []string{
		"does NOT have permission to run counting tools",
		"copy only source-explicit measurements",
		"preserve their original value and unit",
		"Do not calculate total duration, B/E differences, full counts, or threshold absence claims",
		"frames[] and stalls[] require duration_ms",
		"use observations[] without numeric fields",
		"never substitute zero for an unknown duration",
		"both start_ts_ms and duration_ms are explicit millisecond values",
		"{ start_ts_ms (req), duration_ms (req), kind, symbol, file, line }",
		"same ftrace thread stack",
		"marker pid + tag + cookie",
		"keep `janky` false/omitted and omit janks[]",
		"deterministic trace_query results remain authoritative",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("actual perf-triage prompt lacks %q", want)
		}
	}
	for _, stale := range []string{
		"Compute meta.duration_ms",
		"The delta is the span duration.",
		"Always emit measured frame/span durations",
		"For main-thread blocking calls longer than 100 ms",
		"Over 1.2 s app_launch_ms = slow cold start",
		"a paired GC span duration is 8ms",
		"no paired GC span in the bounded excerpt exceeds 50ms",
	} {
		if strings.Contains(prompt, stale) {
			t.Errorf("actual perf-triage prompt retains uncomputed measurement instruction %q", stale)
		}
	}
	if got := strings.Count(prompt, skill.TraceResourceObservationContract); got != 1 {
		t.Errorf("actual perf-triage prompt must carry the shared resource contract once, got %d", got)
	}
}

func TestPerfTriagePromptUnparsedTextUsesObservationRatherThanResidueOnly(t *testing.T) {
	prompt := perfTriageInlineMeasurementPrompt(t, "custom-event payload: ???\nunrecognized-stream-entry\n")
	for _, want := range []string{
		"Never emit residue[] alone",
		"kind=unparsed",
		"evidence with verbatim source text",
		"line_start/line_end",
		"Do not invent event types, business meaning, timings, counts, or causes",
		"residue[] is supplementary",
		"{ start_ts_ms (req), duration_ms (req), trigger_span, reason, tags[] }",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("actual unparseable-trace prompt lacks %q", want)
		}
	}
	if strings.Contains(prompt, "Only emit residue-only when") {
		t.Fatal("unparseable-trace prompt teaches an emission that the public tool rejects")
	}
}

func perfTriageInlineMeasurementPrompt(t *testing.T, trace string) string {
	t.Helper()
	r := skill.NewRegistry()
	skill.RegisterDefaults(r)
	sk, err := r.Get("perf-triage-skill")
	if err != nil {
		t.Fatal(err)
	}
	// The small inline-attachment dispatch filters out pagination read_file
	// before prompt construction (BaseAgent.skillForPrompt).
	inlineSkill := *sk
	inlineSkill.ToolSuggestions = []string{"emit_perf_trace"}
	pc := BuildPromptContext(&types.AgentContext{
		AgentName:       types.AgentPerfTriager,
		Stage:           types.StagePerfTriage,
		Objective:       "Explain why opening a document was slow",
		AttachedHitrace: trace,
	}, &inlineSkill)
	var body strings.Builder
	for _, section := range append(pc.SystemSections, pc.UserSections...) {
		body.WriteString(section.Content)
		body.WriteByte('\n')
	}
	return body.String()
}
