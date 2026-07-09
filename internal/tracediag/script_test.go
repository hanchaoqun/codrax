package tracediag

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const validScriptYAML = `
version: 1
description: "test script"
defaults: { pid: 42591, window: "6793222.031..6793225.370" }
steps:
  - label: raw_events
    view: event_search
    window: "6793224.9..6793225.0"
    event_types: [sched_switch]
    line_start: 10
    line_end: 200
    max_lines: 400
  - label: rank
    view: root_cause_rank
`

func TestParseScriptValid(t *testing.T) {
	script, err := ParseScript([]byte(validScriptYAML))
	if err != nil {
		t.Fatalf("ParseScript: %v", err)
	}
	if len(script.Steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(script.Steps))
	}
	step := script.Steps[0]
	start, end, ok := step.WindowBounds()
	if !ok || start != 6793224.9 || end != 6793225.0 {
		t.Fatalf("step window = %v %v %v", start, end, ok)
	}
	if step.PID != 42591 {
		t.Fatalf("step pid must inherit defaults.pid, got %d", step.PID)
	}
	if step.EffectiveMaxLines() != 400 {
		t.Fatalf("effective max lines = %d, want 400", step.EffectiveMaxLines())
	}
	// Step 2 inherits the defaults window and default line cap.
	rank := script.Steps[1]
	start, end, ok = rank.WindowBounds()
	if !ok || start != 6793222.031 || end != 6793225.370 {
		t.Fatalf("rank window = %v %v %v", start, end, ok)
	}
	if rank.EffectiveMaxLines() != DefaultStepMaxLines {
		t.Fatalf("rank max lines = %d, want default %d", rank.EffectiveMaxLines(), DefaultStepMaxLines)
	}
}

// Thread-only defaults inheritance stays intact after the P0-1 ambiguity
// check (pid-less script, thread flows from defaults into every step).
func TestParseScriptThreadDefaultsInheritance(t *testing.T) {
	yamlText := `
version: 1
defaults: { thread: "#RxComputationT-16816", window: "33872.289161..33872.408222" }
steps:
  - {label: stats, view: window_stats}
`
	script, err := ParseScript([]byte(yamlText))
	if err != nil {
		t.Fatalf("ParseScript: %v", err)
	}
	if script.Steps[0].Thread != "#RxComputationT-16816" || script.Steps[0].PID != 0 {
		t.Fatalf("step selectors = thread %q pid %d", script.Steps[0].Thread, script.Steps[0].PID)
	}
}

// R2-family negative: an unknown key at ANY level must fail loud, never be
// silently ignored (自设最强突变: KnownFields(false) must redden this test).
func TestParseScriptUnknownKeysFailLoud(t *testing.T) {
	cases := map[string]string{
		"top-level": `
version: 1
bogus_key: true
steps:
  - {label: a, view: event_search}
`,
		"step-level": `
version: 1
steps:
  - {label: a, view: event_search, sneaky: 1}
`,
		"defaults-level": `
version: 1
defaults: {pid: 1, oops: 2}
steps:
  - {label: a, view: event_search}
`,
	}
	for name, yamlText := range cases {
		if _, err := ParseScript([]byte(yamlText)); err == nil {
			t.Fatalf("%s: unknown key must fail loud", name)
		}
	}
}

func TestParseScriptIllegalViewFailLoud(t *testing.T) {
	yamlText := `
version: 1
steps:
  - {label: a, view: state_churn}
`
	_, err := ParseScript([]byte(yamlText))
	if err == nil {
		t.Fatal("illegal view must fail loud (aliases like state_churn are deliberately rejected)")
	}
	if !strings.Contains(err.Error(), "state_churn") || !strings.Contains(err.Error(), "window_stats") {
		t.Fatalf("error must name the bad view and list supported views: %v", err)
	}
}

func TestParseScriptBadWindowSyntaxFailLoud(t *testing.T) {
	for _, window := range []string{"abc", "1.0..abc", "5.0..1.0", "3.0", "1.0..1.0", "-2.0..1.0"} {
		yamlText := `
version: 1
steps:
  - {label: a, view: event_search, window: "` + window + `"}
`
		if _, err := ParseScript([]byte(yamlText)); err == nil {
			t.Fatalf("window %q must fail loud", window)
		}
	}
}

func TestParseScriptBadEventTypeFailLoud(t *testing.T) {
	yamlText := `
version: 1
steps:
  - {label: a, view: event_search, event_types: [sched]}
`
	_, err := ParseScript([]byte(yamlText))
	if err == nil {
		t.Fatal("event type 'sched' matches nothing in the engine and must fail loud")
	}
	if !strings.Contains(err.Error(), "sched_switch") {
		t.Fatalf("error must list the supported tokens: %v", err)
	}
}

func TestParseScriptVersionAndStructureFailLoud(t *testing.T) {
	cases := map[string]string{
		"version 2":             "version: 2\nsteps:\n  - {label: a, view: event_search}\n",
		"missing version":       "steps:\n  - {label: a, view: event_search}\n",
		"no steps":              "version: 1\n",
		"missing label":         "version: 1\nsteps:\n  - {view: event_search}\n",
		"missing view":          "version: 1\nsteps:\n  - {label: a}\n",
		"duplicate label":       "version: 1\nsteps:\n  - {label: a, view: event_search}\n  - {label: a, view: window_stats}\n",
		"line_end < line_start": "version: 1\nsteps:\n  - {label: a, view: event_search, line_start: 10, line_end: 5}\n",
	}
	for name, yamlText := range cases {
		if _, err := ParseScript([]byte(yamlText)); err == nil {
			t.Fatalf("%s: must fail loud", name)
		}
	}
}

// Hard-cap discipline: max_lines above 1000 is CLAMPED with disclosure
// state, never rejected and never silently honored (§28.12 超设夹取+披露).
func TestParseScriptMaxLinesHardCapClamps(t *testing.T) {
	yamlText := `
version: 1
steps:
  - {label: a, view: event_search, max_lines: 5000}
`
	script, err := ParseScript([]byte(yamlText))
	if err != nil {
		t.Fatalf("ParseScript: %v", err)
	}
	step := script.Steps[0]
	if step.EffectiveMaxLines() != HardStepMaxLines {
		t.Fatalf("effective = %d, want hard cap %d", step.EffectiveMaxLines(), HardStepMaxLines)
	}
	requested, clamped := step.MaxLinesClamped()
	if !clamped || requested != 5000 {
		t.Fatalf("clamp disclosure state = (%d,%v), want (5000,true)", requested, clamped)
	}
}

// P0-1 pin (对抗复核 2026-07-09, 自设最强突变: removing the resolved-values
// ambiguity check reddens all three arms): the engine thread resolver is
// pid-first and silently ignores the thread selector, so pid+thread on one
// RESOLVED step is refused — including the defaults-pid-meets-step-thread
// form that made the original d10 script sample the main thread.
func TestParseScriptPIDPlusThreadFailLoud(t *testing.T) {
	cases := map[string]string{
		"same step": `
version: 1
steps:
  - {label: a, view: window_stats, pid: 16547, thread: "#RxComputationT-16816"}
`,
		"defaults pid meets step thread": `
version: 1
defaults: { pid: 16547 }
steps:
  - {label: a, view: window_stats, thread: "#RxComputationT-16816"}
`,
		"defaults thread meets step pid": `
version: 1
defaults: { thread: "#RxComputationT-16816" }
steps:
  - {label: a, view: window_stats, pid: 16547}
`,
	}
	for name, yamlText := range cases {
		_, err := ParseScript([]byte(yamlText))
		if err == nil {
			t.Fatalf("%s: pid+thread must fail loud", name)
		}
		if !strings.Contains(err.Error(), "pid-first") || !strings.Contains(err.Error(), "IGNORE") {
			t.Fatalf("%s: error must explain the engine pid-first semantics, got %v", name, err)
		}
	}
	// Single selectors stay legal.
	for name, yamlText := range map[string]string{
		"pid only":    "version: 1\nsteps:\n  - {label: a, view: window_stats, pid: 16547}\n",
		"thread only": "version: 1\nsteps:\n  - {label: a, view: window_stats, thread: \"#RxComputationT-16816\"}\n",
	} {
		if _, err := ParseScript([]byte(yamlText)); err != nil {
			t.Fatalf("%s must stay legal: %v", name, err)
		}
	}
}

func TestParseScriptFormatCensusViewAccepted(t *testing.T) {
	yamlText := `
version: 1
steps:
  - {label: census, view: format_census}
`
	script, err := ParseScript([]byte(yamlText))
	if err != nil {
		t.Fatalf("format_census must be a legal view: %v", err)
	}
	if script.Steps[0].View != ViewFormatCensus {
		t.Fatalf("view = %q", script.Steps[0].View)
	}
}

// EVOLUTION RECORD (TDIAG B1, §28.13, 2026-07-09) — the former cross-check
// pin ("every self-maintained view resolves to a capacity row") evolved: the
// self-maintained list is deleted and the script layer CONSUMES the engine's
// exported enumerator, so the pin now asserts 导出面=引擎容量表 — the accepted
// view set IS tracequery.CanonicalViewNames() (plus the tracediag-only
// format_census), every name is canonical/heavy-or-limited, and the known
// engine views are spot-pinned so an accidental table gutting cannot pass on
// shape alone.
func TestSupportedEngineViewsAreTheExportedEnumerator(t *testing.T) {
	names := tracequery.CanonicalViewNames()
	if len(supportedEngineViews) != len(names) {
		t.Fatalf("script layer must consume the exported enumerator verbatim: %v vs %v", supportedEngineViews, names)
	}
	for i, view := range supportedEngineViews {
		if view != names[i] {
			t.Fatalf("script view list diverged from the exported enumerator at %d: %q vs %q", i, view, names[i])
		}
		if tracequery.CanonicalViewName(view) != view {
			t.Errorf("view %q is not canonical (CanonicalViewName → %q)", view, tracequery.CanonicalViewName(view))
		}
		capacity := tracequery.ViewCapacityFor(view)
		if !capacity.HeavyView && capacity.DefaultLimit <= 0 {
			t.Errorf("view %q resolves to a zero capacity row — renamed or removed from the engine?", view)
		}
	}
	for _, must := range []string{"event_search", "window_sweep", "root_cause_rank", "wakeup_chain", "window_stats", "critical_blocking_calls", "recipe", "evidence_pack"} {
		if !viewSupported(must) {
			t.Errorf("engine view %q must stay accepted", must)
		}
	}
	if !viewSupported(ViewFormatCensus) {
		t.Errorf("the tracediag-only census view must stay accepted")
	}
	// Aliases stay out by construction (capacity-table keys are canonical).
	for _, reject := range []string{"state_churn", "frame_bundle", "causal_impact"} {
		if viewSupported(reject) {
			t.Errorf("alias %q must NOT be accepted (deterministic scripts name canonical views)", reject)
		}
	}
}

// The event_types closed set must stay canonical: spot-pin the wire tokens
// that scripts ship with, and pin that the noisy tool-layer aliases stay out.
func TestSupportedEventTypesPinned(t *testing.T) {
	for _, want := range []string{"sched_switch", "sched_blocked_reason", "trace_mark", "file_io", "page_cache", "storage_latency"} {
		if !eventTypeSupported(want) {
			t.Errorf("token %q must be supported", want)
		}
	}
	for _, reject := range []string{"sched", "filemap", "print", "perf", "interrupts"} {
		if eventTypeSupported(reject) {
			t.Errorf("noisy alias %q must NOT be accepted (deterministic scripts name exact tokens)", reject)
		}
	}
}
