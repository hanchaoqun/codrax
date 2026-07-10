package tracediag

import (
	"reflect"
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
	for _, window := range []string{"abc", "1.0..abc", "5.0..1.0", "3.0", "1.0..1.0", "-2.0..1.0", "NaN..2", "1..+Inf"} {
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

func TestParseScriptTraceMarkActionsClosedFilter(t *testing.T) {
	valid := `
version: 1
steps:
  - {label: async_rows, view: event_search, trace_mark_actions: [S, F], max_lines: 20}
`
	script, err := ParseScript([]byte(valid))
	if err != nil {
		t.Fatalf("valid action filter: %v", err)
	}
	if got := script.Steps[0].TraceMarkActions; len(got) != 2 || got[0] != "S" || got[1] != "F" {
		t.Fatalf("trace_mark_actions = %v", got)
	}
	invalid := map[string]string{
		"unknown":      `[X]`,
		"lowercase":    `[s]`,
		"duplicate":    `[S, S]`,
		"wrong view":   `[S]`,
		"foreign type": `[S]`,
	}
	for name, actions := range invalid {
		view := "event_search"
		eventTypes := ""
		if name == "wrong view" {
			view = "window_stats"
		}
		if name == "foreign type" {
			eventTypes = ", event_types: [sched_switch]"
		}
		yamlText := "version: 1\nsteps:\n  - {label: bad, view: " + view + ", trace_mark_actions: " + actions + eventTypes + "}\n"
		if _, err := ParseScript([]byte(yamlText)); err == nil {
			t.Errorf("%s action shape must fail loud", name)
		}
	}
	withExactType := `
version: 1
steps:
  - {label: async_rows, view: event_search, event_types: [trace_mark], trace_mark_actions: [S]}
`
	if _, err := ParseScript([]byte(withExactType)); err != nil {
		t.Fatalf("trace_mark + exact action must be valid: %v", err)
	}
}

func TestParseScriptRejectsSelectorsOnCPUGlobalEventSearch(t *testing.T) {
	for name, yamlText := range map[string]string{
		"inherited pid": `
version: 1
defaults: {pid: 20}
steps:
  - {label: freq, view: event_search, event_types: [cpu_frequency]}
`,
		"thread": `
version: 1
steps:
  - {label: idle, view: event_search, thread: app-20, event_types: [cpu_idle]}
`,
		"mixed": `
version: 1
steps:
  - {label: mixed, view: event_search, pid: 20, event_types: [sched_switch, clock_set_rate]}
`,
	} {
		_, err := ParseScript([]byte(yamlText))
		if err == nil || !strings.Contains(err.Error(), "CPU-global") || !strings.Contains(err.Error(), "incidental emitter") {
			t.Errorf("%s must fail with CPU ownership guidance, got %v", name, err)
		}
	}
	allowed := `
version: 1
steps:
  - {label: stats, view: window_stats, pid: 20, event_types: [cpu_frequency]}
  - {label: raw, view: event_search, event_types: [cpu_frequency]}
`
	if _, err := ParseScript([]byte(allowed)); err != nil {
		t.Fatalf("target-oriented compound view and unscoped raw CPU lane must remain valid: %v", err)
	}
}

func TestParseScriptVersionAndStructureFailLoud(t *testing.T) {
	cases := map[string]string{
		"version 3":             "version: 3\nsteps:\n  - {label: a, view: event_search}\n",
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

func TestParseScriptV2DiscoveryAndTypedFanout(t *testing.T) {
	yamlText := `
version: 2
description: typed fanout
defaults: {window: "1.000..1.500"}
limits: {max_generated_windows: 4, max_expanded_steps: 8, max_report_lines: 500}
discoveries:
  - label: io_pairing
    strategy: pairing_integrity
    families: [block, storage]
    max_windows: 2
    max_window_ms: 50
    padding_ms: 5
    max_lines: 20
steps:
  - {label: static_stats, view: window_stats, pid: 20, max_lines: 20}
  - label: raw_io
    view: event_search
    event_types: [block_rq_issue, block_rq_complete, storage_latency]
    windows_from: {discovery: io_pairing}
    max_lines: 20
`
	script, err := ParseScript([]byte(yamlText))
	if err != nil {
		t.Fatalf("ParseScript(v2): %v", err)
	}
	if script.Version != ScriptVersionV2 || len(script.Discoveries) != 1 || len(script.Steps) != 2 {
		t.Fatalf("v2 shape = %+v", script)
	}
	discovery := script.Discoveries[0]
	start, end, ok := discovery.WindowBounds()
	if !ok || start != 1 || end != 1.5 {
		t.Fatalf("discovery did not inherit the one parent window: %v..%v set=%v", start, end, ok)
	}
	dynamic := script.Steps[1]
	if dynamic.Window != "" {
		t.Fatalf("dynamic step must not inherit defaults.window: %q", dynamic.Window)
	}
	if _, _, set := dynamic.WindowBounds(); set || dynamic.WindowsFrom == nil || dynamic.WindowsFrom.Discovery != "io_pairing" {
		t.Fatalf("dynamic typed reference = %+v", dynamic)
	}
	if script.v2Limits.MaxGeneratedWindows != 4 || script.v2WorstReportLines <= 0 || script.v2WorstReportLines > 500 {
		t.Fatalf("resolved v2 budgets = %+v worst=%d", script.v2Limits, script.v2WorstReportLines)
	}
}

func TestParseScriptV2TraceMarkCarrySchemaAndAtomicBudget(t *testing.T) {
	yamlText := `
version: 2
defaults: {window: "1.000..1.100"}
limits: {max_generated_windows: 2, max_expanded_steps: 2, max_report_lines: 300}
discoveries:
  - label: marker_pairs
    strategy: trace_mark_carry
    families: [trace_sync, trace_async, trace_track]
    max_windows: 2
    max_window_ms: 50
    max_lines: 20
steps:
  - label: raw_marker_endpoints
    view: event_search
    event_types: [trace_mark]
    trace_mark_actions: [B, E, S, F, G, H]
    windows_from: {discovery: marker_pairs}
    max_lines: 20
`
	script, err := ParseScript([]byte(yamlText))
	if err != nil {
		t.Fatalf("ParseScript(trace_mark_carry): %v", err)
	}
	if len(script.Discoveries) != 1 || script.Discoveries[0].Strategy != string(tracequery.WindowDiscoveryTraceMarkCarry) ||
		!reflect.DeepEqual(script.Discoveries[0].Families, []string{"trace_sync", "trace_async", "trace_track"}) ||
		script.v2Limits.MaxGeneratedWindows != 2 || script.v2Limits.MaxExpandedSteps != 2 || script.v2WorstReportLines > 300 {
		t.Fatalf("trace_mark_carry schema/budget = discoveries:%+v limits:%+v worst:%d", script.Discoveries, script.v2Limits, script.v2WorstReportLines)
	}
	step := script.Steps[0]
	if step.WindowsFrom == nil || step.WindowsFrom.Discovery != "marker_pairs" ||
		!reflect.DeepEqual(step.TraceMarkActions, []string{"B", "E", "S", "F", "G", "H"}) {
		t.Fatalf("trace_mark_carry generated endpoint step = %+v", step)
	}

	foreignFamily := strings.Replace(yamlText, "trace_sync, trace_async, trace_track", "block", 1)
	if _, err := ParseScript([]byte(foreignFamily)); err == nil || !strings.Contains(err.Error(), `strategy "trace_mark_carry"`) {
		t.Fatalf("trace_mark_carry accepted a foreign family: %v", err)
	}
	unbounded := strings.Replace(yamlText, `defaults: {window: "1.000..1.100"}`, "", 1)
	if unbounded == yamlText {
		t.Fatal("test fixture failed to remove defaults.window")
	}
	if _, err := ParseScript([]byte(unbounded)); err == nil || !strings.Contains(err.Error(), "bounded parent") {
		t.Fatalf("trace_mark_carry accepted an unbounded parent: %v", err)
	}
}

func TestScriptWindowOverrideReplacesOnlyInheritedDefault(t *testing.T) {
	script, err := parseScript([]byte(validScriptYAML), ScriptOverrides{Window: "2.000..3.000"})
	if err != nil {
		t.Fatalf("parseScript override: %v", err)
	}
	// raw_events has an explicit step window and must remain unchanged.
	start, end, ok := script.Steps[0].WindowBounds()
	if !ok || start != 6793224.9 || end != 6793225.0 {
		t.Fatalf("explicit step window was overwritten: %v..%v set=%v", start, end, ok)
	}
	// rank inherited defaults.window and therefore takes the CLI override.
	start, end, ok = script.Steps[1].WindowBounds()
	if !ok || start != 2 || end != 3 || script.Defaults.Window != "2.000..3.000" {
		t.Fatalf("inherited window did not take override: %v..%v set=%v default=%q", start, end, ok, script.Defaults.Window)
	}
	if _, err := parseScript([]byte(validScriptYAML), ScriptOverrides{Window: "NaN..3"}); err == nil {
		t.Fatal("invalid CLI window override must fail through the same strict parser")
	}
}

func TestScriptWindowOverrideDrivesV2DiscoveryButNotExplicitWindows(t *testing.T) {
	yamlText := `
version: 2
defaults: {window: "1.000..1.500"}
limits: {max_generated_windows: 2, max_expanded_steps: 3, max_report_lines: 300}
discoveries:
  - {label: inherited, strategy: pairing_integrity, families: [block], max_windows: 1, max_lines: 10}
  - {label: explicit, strategy: pairing_integrity, families: [storage], window: "4.000..4.500", max_windows: 1, max_lines: 10}
steps:
  - {label: raw, view: event_search, event_types: [block_rq_issue, block_rq_complete], windows_from: {discovery: inherited}, max_lines: 10}
`
	script, err := parseScript([]byte(yamlText), ScriptOverrides{Window: "2.000..3.000"})
	if err != nil {
		t.Fatalf("parseScript v2 override: %v", err)
	}
	start, end, ok := script.Discoveries[0].WindowBounds()
	if !ok || start != 2 || end != 3 {
		t.Fatalf("inherited discovery did not take override: %v..%v set=%v", start, end, ok)
	}
	start, end, ok = script.Discoveries[1].WindowBounds()
	if !ok || start != 4 || end != 4.5 {
		t.Fatalf("explicit discovery window was overwritten: %v..%v set=%v", start, end, ok)
	}
	if script.Steps[0].Window != "" {
		t.Fatalf("dynamic step must keep typed discovery source, got window=%q", script.Steps[0].Window)
	}
}

func TestScriptRequiredWindowInputFailsLoudAndStaysTyped(t *testing.T) {
	yamlText := `
version: 2
inputs: {window: required}
steps:
  - {label: rows, view: event_search}
`
	if _, err := ParseScript([]byte(yamlText)); err == nil || !strings.Contains(err.Error(), "requires --trace-window") {
		t.Fatalf("missing required window override must fail with operator guidance, got %v", err)
	}
	script, err := parseScript([]byte(yamlText), ScriptOverrides{Window: "2.000..3.000"})
	if err != nil {
		t.Fatalf("required window with override: %v", err)
	}
	start, end, ok := script.Steps[0].WindowBounds()
	if !ok || start != 2 || end != 3 {
		t.Fatalf("required window was not consumed: %v..%v set=%v", start, end, ok)
	}
	if err := script.Validate(); err != nil {
		t.Fatalf("resolved script validation must remain idempotent: %v", err)
	}

	unsupported := strings.Replace(yamlText, "window: required", "window: optional", 1)
	if _, err := parseScript([]byte(unsupported), ScriptOverrides{Window: "2..3"}); err == nil || !strings.Contains(err.Error(), "supported: required") {
		t.Fatalf("input mode must remain a closed enum, got %v", err)
	}
	unused := `
version: 2
inputs: {window: required}
steps:
  - {label: rows, view: event_search, window: "4..5"}
`
	if _, err := parseScript([]byte(unused), ScriptOverrides{Window: "2..3"}); err == nil || !strings.Contains(err.Error(), "is unused") {
		t.Fatalf("declared required input must have an inherited consumer, got %v", err)
	}
}

func TestScriptTIDOverrideBindsOnlyDeclaredSteps(t *testing.T) {
	yamlText := `
version: 2
inputs: {window: required, tid: required}
limits: {max_generated_windows: 1, max_expanded_steps: 2, max_report_lines: 300}
steps:
  - {label: target, view: window_stats, pid_from: tid, max_lines: 20}
  - {label: raw, view: event_search, event_types: [block_rq_complete], max_lines: 20}
`
	if _, err := parseScript([]byte(yamlText), ScriptOverrides{Window: "2..3"}); err == nil || !strings.Contains(err.Error(), "requires --trace-tid") {
		t.Fatalf("missing required TID must fail with operator guidance, got %v", err)
	}
	script, err := parseScript([]byte(yamlText), ScriptOverrides{Window: "2..3", TID: "42591"})
	if err != nil {
		t.Fatalf("typed TID parse: %v", err)
	}
	if got := script.Steps[0]; got.PID != 42591 || got.PIDFrom != "tid" || got.Thread != "" {
		t.Fatalf("bound target selector = pid:%d pid_from:%q thread:%q", got.PID, got.PIDFrom, got.Thread)
	}
	if got := script.Steps[1]; got.PID != 0 || got.PIDFrom != "" || got.Thread != "" {
		t.Fatalf("unbound raw evidence lane was scoped: pid:%d pid_from:%q thread:%q", got.PID, got.PIDFrom, got.Thread)
	}
	if err := script.Validate(); err != nil {
		t.Fatalf("resolved TID binding validation must remain idempotent: %v", err)
	}
	script.Steps[0].PID = 9
	if err := script.Validate(); err == nil || !strings.Contains(err.Error(), "binding was modified") {
		t.Fatalf("mutated resolved TID binding must fail loud, got %v", err)
	}

	for _, bad := range []string{"0", "-1", "+20", "20x", "2147483648"} {
		if _, err := parseScript([]byte(yamlText), ScriptOverrides{Window: "2..3", TID: bad}); err == nil || !strings.Contains(err.Error(), "--trace-tid") {
			t.Errorf("invalid TID %q must fail loud, got %v", bad, err)
		}
	}
}

func TestParseTIDOverrideCanonicalBoundaries(t *testing.T) {
	for raw, want := range map[string]int{
		"1":          1,
		"00020":      20,
		" 20 ":       20,
		"2147483647": 2147483647,
	} {
		got, err := parseTIDOverride(raw)
		if err != nil || got != want {
			t.Errorf("parseTIDOverride(%q) = %d, %v; want %d", raw, got, err, want)
		}
	}
	for _, raw := range []string{"", " ", "0", "-1", "+20", "2147483648", "0x14", "20.0", "2e1", "20x", "２０"} {
		if got, err := parseTIDOverride(raw); err == nil {
			t.Errorf("parseTIDOverride(%q) = %d, want error", raw, got)
		}
	}
}

func TestScriptTIDBindingDoesNotInheritSelectorDefaults(t *testing.T) {
	cases := map[string]struct {
		defaults          string
		wantUnboundPID    int
		wantUnboundThread string
	}{
		"pid default":    {defaults: "pid: 99", wantUnboundPID: 99},
		"thread default": {defaults: `thread: "worker-99"`, wantUnboundThread: "worker-99"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			yamlText := `
version: 2
inputs: {tid: required}
defaults: {` + tc.defaults + `}
steps:
  - {label: bound, view: event_search, pid_from: tid}
  - {label: unbound, view: event_search}
`
			script, err := parseScript([]byte(yamlText), ScriptOverrides{TID: "7"})
			if err != nil {
				t.Fatal(err)
			}
			if got := script.Steps[0]; got.PID != 7 || got.Thread != "" {
				t.Fatalf("bound step inherited selector defaults: pid=%d thread=%q", got.PID, got.Thread)
			}
			if got := script.Steps[1]; got.PID != tc.wantUnboundPID || got.Thread != tc.wantUnboundThread {
				t.Fatalf("unbound defaults changed: pid=%d thread=%q", got.PID, got.Thread)
			}
		})
	}
}

func TestScriptTIDBindingRejectsAmbiguousOrUnusedShapes(t *testing.T) {
	cases := map[string]string{
		"empty inputs": `
version: 2
inputs: {}
steps: [{label: a, view: event_search}]
`,
		"unsupported tid mode": `
version: 2
inputs: {tid: optional}
steps: [{label: a, view: event_search, pid_from: tid}]
`,
		"unknown binding": `
version: 2
inputs: {tid: required}
steps: [{label: a, view: event_search, pid_from: target}]
`,
		"explicit pid": `
version: 2
inputs: {tid: required}
steps: [{label: a, view: event_search, pid_from: tid, pid: 7}]
`,
		"explicit thread": `
version: 2
inputs: {tid: required}
steps: [{label: a, view: event_search, pid_from: tid, thread: app-7}]
`,
		"undeclared input": `
version: 2
steps: [{label: a, view: event_search, pid_from: tid}]
`,
		"required but unused": `
version: 2
inputs: {tid: required}
steps: [{label: a, view: event_search, window: "1..2"}]
`,
	}
	for name, yamlText := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := parseScript([]byte(yamlText), ScriptOverrides{TID: "7"}); err == nil {
				t.Fatal("ambiguous or unused TID shape must fail")
			}
		})
	}
	noDeclaration := `
version: 2
steps: [{label: a, view: event_search, window: "1..2"}]
`
	if _, err := parseScript([]byte(noDeclaration), ScriptOverrides{TID: "7"}); err == nil || !strings.Contains(err.Error(), "does not declare inputs.tid") {
		t.Fatalf("unused CLI TID must not be silently ignored, got %v", err)
	}
	if _, err := parseScript([]byte(validScriptYAML), ScriptOverrides{TID: "7"}); err == nil || !strings.Contains(err.Error(), "version: 2") {
		t.Fatalf("v1 script must reject --trace-tid instead of silently ignoring it, got %v", err)
	}
}

func TestParseScriptV1RejectsEveryV2Field(t *testing.T) {
	cases := map[string]string{
		"inputs":       "version: 1\ninputs: {window: required}\nsteps:\n  - {label: a, view: event_search}\n",
		"limits":       "version: 1\nlimits: {max_expanded_steps: 2}\nsteps:\n  - {label: a, view: event_search}\n",
		"discoveries":  "version: 1\ndiscoveries:\n  - {label: d, strategy: pairing_integrity}\nsteps:\n  - {label: a, view: event_search}\n",
		"windows_from": "version: 1\nsteps:\n  - {label: a, view: event_search, windows_from: {discovery: d}}\n",
		"pid_from":     "version: 1\nsteps:\n  - {label: a, view: event_search, pid_from: tid}\n",
	}
	for name, yamlText := range cases {
		_, err := ParseScript([]byte(yamlText))
		if err == nil || !strings.Contains(err.Error(), "v2-only") {
			t.Errorf("%s: v1 must reject recognized v2 fields explicitly, got %v", name, err)
		}
	}
}

func TestParseScriptV2StrictValidationAndBudgets(t *testing.T) {
	cases := map[string]string{
		"unknown strategy": `
version: 2
discoveries: [{label: d, strategy: guess}]
steps: [{label: a, view: event_search}]
`,
		"unknown family": `
version: 2
discoveries: [{label: d, strategy: pairing_integrity, families: [io]}]
steps: [{label: a, view: event_search}]
`,
		"duplicate global label": `
version: 2
discoveries: [{label: same, strategy: pairing_integrity}]
steps: [{label: same, view: event_search}]
`,
		"unsafe label": `
version: 2
steps: [{label: "bad label", view: event_search}]
`,
		"unknown discovery ref": `
version: 2
steps: [{label: a, view: event_search, windows_from: {discovery: missing}}]
`,
		"explicit and generated window": `
version: 2
discoveries: [{label: d, strategy: pairing_integrity}]
steps: [{label: a, view: event_search, window: "1..2", windows_from: {discovery: d}}]
`,
		"line disables generated time": `
version: 2
discoveries: [{label: d, strategy: pairing_integrity}]
steps: [{label: a, view: event_search, line_start: 1, windows_from: {discovery: d}}]
`,
		"generated window budget": `
version: 2
limits: {max_generated_windows: 1}
discoveries: [{label: d, strategy: pairing_integrity, max_windows: 2}]
steps: [{label: a, view: event_search}]
`,
		"expanded step budget": `
version: 2
limits: {max_expanded_steps: 1}
discoveries: [{label: d, strategy: pairing_integrity, max_windows: 2}]
steps: [{label: a, view: event_search, windows_from: {discovery: d}, max_lines: 1}]
`,
		"report budget": `
version: 2
limits: {max_report_lines: 100}
steps: [{label: a, view: event_search, max_lines: 50}]
`,
		"discovery hard width": `
version: 2
discoveries: [{label: d, strategy: pairing_integrity, max_window_ms: 51}]
steps: [{label: a, view: event_search}]
`,
		"cohort limit one": `
version: 2
discoveries: [{label: d, strategy: pairing_integrity, cohort_event_limit: 1}]
steps: [{label: a, view: event_search}]
`,
		"generated event body too small": `
version: 2
discoveries: [{label: d, strategy: pairing_integrity}]
steps: [{label: a, view: event_search, windows_from: {discovery: d}, max_lines: 4}]
`,
	}
	for name, yamlText := range cases {
		if _, err := ParseScript([]byte(yamlText)); err == nil {
			t.Errorf("%s must fail loud", name)
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
