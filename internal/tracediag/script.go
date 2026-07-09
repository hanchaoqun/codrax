// Package tracediag implements the TDIAG (§28.12, user ruling 2026-07-09)
// deterministic trace diagnostic collector: a zero-LLM, read-only CLI mode
// that runs a YAML collection script of tracequery steps against a trace file
// and renders a single evidence-faithful text report for customer round-trip
// collection (single result ≤1000 lines per step).
//
// Boundary rules (pinned by TestTraceDiagImportBoundary):
//   - consumes ONLY the exported tracequery engine API (Run / StreamEventSearch
//     / StreamWindowSweep / BuildIndex* / Index.Events); never internal/tool
//     (its rendering face is coupled to the LLM pipeline), never
//     internal/llm / internal/agent / internal/orchestrator (zero LLM path);
//   - all rendering is built here, in the "系统补充" verbatim-token style;
//   - values and engine Summary strings pass through verbatim — this is an
//     evidence collection face, fidelity over beauty.
package tracediag

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const (
	// ScriptVersion is the only accepted collection-script schema version.
	ScriptVersion = 1
	// DefaultStepMaxLines is the per-step body line cap when a step (and the
	// script defaults) set none.
	DefaultStepMaxLines = 800
	// HardStepMaxLines is the per-step hard cap: larger requested values are
	// CLAMPED (not rejected) and the clamp is disclosed in the report
	// (§28.12: 每步行数帽默认 800 硬帽 1000, 超设夹取+披露).
	HardStepMaxLines = 1000
	// DefaultTotalMaxLines bounds the whole report so a runaway script cannot
	// produce an unbounded file; hitting it is disclosed, never silent.
	DefaultTotalMaxLines = 5000
	// ViewFormatCensus is the tracediag-only census step (§28.12 补充裁定:
	// 格式盲点普查): whole-window format census computed in THIS package from
	// the engine's exported Index face (event names / marker forms / clock
	// tracks / sched domain / FS-IO kv coverage / power events / spans /
	// line-level quality). It is not a tracequery view.
	ViewFormatCensus = "format_census"
)

// supportedEngineViews is the canonical tracequery view enum accepted by the
// step "view" field. tracequery exports no view-name enumerator (missing-API
// candidate), so the list is maintained here and cross-checked against the
// engine capacity table by TestSupportedEngineViewsExistInCapacityTable:
// every entry must resolve to a real capacity row (HeavyView or a positive
// DefaultLimit), which catches renames/typos mechanically. Aliases the LLM
// tool schema tolerates (state_churn→window_stats, …) are deliberately NOT
// accepted: collection scripts are deterministic artifacts, fail-loud beats
// silent normalization (R2 discipline).
var supportedEngineViews = []string{
	"event_search",
	"window_sweep",
	"span_window",
	"frame_window",
	"render_pipeline",
	"frame_timeline",
	"frame_flow",
	"thread_timeline",
	"window_stats",
	"perf_stats",
	"perf_timeline",
	"trace_perf_bundle",
	"scheduler_latency_stats",
	"ipc_graph",
	"wakeup_chain",
	"root_cause_rank",
	"frame_root_cause_bundle",
	"critical_blocking_calls",
	"interaction_stats",
	"recipe",
	"evidence_pack",
}

// supportedEventTypes is the closed event_types token set: the canonical
// tracequery.EventType wire tokens plus the family filter tokens
// eventTypeMatches consumes (file_io / page_cache / android_fs / f2fs / scsi
// / mmc / storage_latency / io_pressure). The LLM tool layer normalizes noisy
// aliases ("sched", "filemap", …); tracediag scripts must name the exact
// token — an unknown token would otherwise SILENTLY match zero events, which
// is the exact class of quiet lie this mode exists to remove.
var supportedEventTypes = []string{
	string(tracequery.EventUnknown),
	string(tracequery.EventSchedSwitch),
	string(tracequery.EventSchedWakeup),
	string(tracequery.EventSchedWaking),
	string(tracequery.EventSchedBlockedReason),
	string(tracequery.EventSchedStat),
	string(tracequery.EventCPUIdle),
	string(tracequery.EventCPUFrequency),
	string(tracequery.EventCPUFrequencyLimit),
	string(tracequery.EventCPUConstraint),
	string(tracequery.EventClockSetRate),
	string(tracequery.EventBlockIssue),
	string(tracequery.EventBlockRemap),
	string(tracequery.EventBlockComplete),
	string(tracequery.EventBinderTransaction),
	string(tracequery.EventBinderReceived),
	string(tracequery.EventBinderAllocBuf),
	string(tracequery.EventBinderLock),
	string(tracequery.EventBinderLocked),
	string(tracequery.EventBinderUnlock),
	string(tracequery.EventBinderReply),
	string(tracequery.EventIRQ),
	string(tracequery.EventSoftIRQ),
	string(tracequery.EventIPI),
	string(tracequery.EventTraceMark),
	string(tracequery.EventMemory),
	string(tracequery.EventStorage),
	string(tracequery.EventFilesystem),
	string(tracequery.EventPower),
	string(tracequery.EventAbilityMonitor),
	string(tracequery.EventXPower),
	string(tracequery.EventHiSystemEvent),
	string(tracequery.EventWorkqueue),
	string(tracequery.EventDMAFence),
	string(tracequery.EventPerfSample),
	// family filter tokens (engine eventTypeMatches families):
	"file_io",
	"page_cache",
	"android_fs",
	"f2fs",
	"scsi",
	"mmc",
	"storage_latency",
	"io_pressure",
}

// Script is the strict-decoded collection script (R2 family discipline:
// KnownFields(true), every unknown key fails loud).
type Script struct {
	Version     int      `yaml:"version"`
	Description string   `yaml:"description"`
	Defaults    Defaults `yaml:"defaults"`
	Steps       []Step   `yaml:"steps"`
}

// Defaults are optional script-wide step defaults; a step field overrides.
type Defaults struct {
	PID      int    `yaml:"pid"`
	Thread   string `yaml:"thread"`
	Window   string `yaml:"window"`
	MaxLines int    `yaml:"max_lines"`
}

// Step is one collection step. Field set is pinned to the §28.12 ruling:
// label / view / pid / thread / window / line range / pattern / event_types /
// max_lines — nothing more.
type Step struct {
	Label      string   `yaml:"label"`
	View       string   `yaml:"view"`
	PID        int      `yaml:"pid"`
	Thread     string   `yaml:"thread"`
	Window     string   `yaml:"window"`
	LineStart  int      `yaml:"line_start"`
	LineEnd    int      `yaml:"line_end"`
	Pattern    string   `yaml:"pattern"`
	EventTypes []string `yaml:"event_types"`
	MaxLines   int      `yaml:"max_lines"`

	// Resolved fields (populated by Validate; not part of the YAML schema).
	windowStart     float64
	windowEnd       float64
	windowSet       bool
	effMaxLines     int
	maxLinesClamped bool
	requestedMaxRaw int
}

// WindowBounds returns the parsed window endpoints (valid only when set).
func (s *Step) WindowBounds() (start, end float64, ok bool) {
	return s.windowStart, s.windowEnd, s.windowSet
}

// EffectiveMaxLines is the per-step body line cap after default/hard-cap
// resolution; Clamped reports whether the script asked for more than the hard
// cap (disclosed in the report).
func (s *Step) EffectiveMaxLines() int { return s.effMaxLines }

// MaxLinesClamped reports the hard-cap clamp together with the raw request.
func (s *Step) MaxLinesClamped() (requested int, clamped bool) {
	return s.requestedMaxRaw, s.maxLinesClamped
}

// LoadScript reads and strict-decodes a collection script, then validates it.
func LoadScript(path string) (*Script, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("tracediag: read script: %w", err)
	}
	return ParseScript(data)
}

// ParseScript strict-decodes a collection script from bytes and validates it.
func ParseScript(data []byte) (*Script, error) {
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	// R2 family discipline: the schema IS the decoder — unknown keys fail
	// loud instead of being silently dropped.
	dec.KnownFields(true)
	var script Script
	if err := dec.Decode(&script); err != nil {
		return nil, fmt.Errorf("tracediag: script decode failed (unknown keys are rejected; schema fields: version/description/defaults{pid,thread,window,max_lines}/steps[label,view,pid,thread,window,line_start,line_end,pattern,event_types,max_lines]): %w", err)
	}
	if err := script.Validate(); err != nil {
		return nil, err
	}
	return &script, nil
}

// Validate applies the fail-loud schema rules and resolves defaults, window
// bounds and line caps onto each step.
func (s *Script) Validate() error {
	if s.Version != ScriptVersion {
		return fmt.Errorf("tracediag: unsupported script version %d (want %d)", s.Version, ScriptVersion)
	}
	if len(s.Steps) == 0 {
		return fmt.Errorf("tracediag: script has no steps")
	}
	if s.Defaults.Window != "" {
		if _, _, err := parseWindow(s.Defaults.Window); err != nil {
			return fmt.Errorf("tracediag: defaults.window: %w", err)
		}
	}
	if s.Defaults.PID < 0 {
		return fmt.Errorf("tracediag: defaults.pid must be >= 0, got %d", s.Defaults.PID)
	}
	if s.Defaults.MaxLines < 0 {
		return fmt.Errorf("tracediag: defaults.max_lines must be >= 0, got %d", s.Defaults.MaxLines)
	}
	seen := map[string]bool{}
	for i := range s.Steps {
		step := &s.Steps[i]
		if err := s.validateStep(i, step, seen); err != nil {
			return err
		}
	}
	return nil
}

func (s *Script) validateStep(i int, step *Step, seen map[string]bool) error {
	at := fmt.Sprintf("tracediag: steps[%d]", i)
	step.Label = strings.TrimSpace(step.Label)
	if step.Label == "" {
		return fmt.Errorf("%s: label is required", at)
	}
	if seen[step.Label] {
		return fmt.Errorf("%s: duplicate label %q", at, step.Label)
	}
	seen[step.Label] = true
	step.View = strings.TrimSpace(step.View)
	if step.View == "" {
		return fmt.Errorf("%s (%s): view is required; supported: %s", at, step.Label, strings.Join(allSupportedViews(), ", "))
	}
	if !viewSupported(step.View) {
		return fmt.Errorf("%s (%s): unknown view %q; supported: %s", at, step.Label, step.View, strings.Join(allSupportedViews(), ", "))
	}
	// Defaults inheritance: a step-zero field inherits the script default.
	if step.PID == 0 {
		step.PID = s.Defaults.PID
	}
	if step.PID < 0 {
		return fmt.Errorf("%s (%s): pid must be >= 0, got %d", at, step.Label, step.PID)
	}
	if strings.TrimSpace(step.Thread) == "" {
		step.Thread = s.Defaults.Thread
	}
	step.Thread = strings.TrimSpace(step.Thread)
	// P0-1 (对抗复核 2026-07-09): the engine thread resolver is PID-first —
	// with Query.PID > 0 the thread selector is silently IGNORED (tracequery
	// resolveThread, query.go:1373), so pid+thread together collects the
	// WRONG thread (the original d10 script sampled the main thread this
	// way). A deterministic collection script refuses ambiguous selectors.
	// Checked on the RESOLVED values so a defaults-level pid meeting a
	// step-level thread is caught exactly like a same-step pair.
	if step.PID > 0 && step.Thread != "" {
		return fmt.Errorf("%s (%s): pid=%d and thread=%q are both set (after defaults) — the engine is pid-first and would IGNORE the thread selector; keep exactly one (a \"comm-pid\" thread selector already carries the tid)", at, step.Label, step.PID, step.Thread)
	}
	if strings.TrimSpace(step.Window) == "" {
		step.Window = s.Defaults.Window
	}
	step.Window = strings.TrimSpace(step.Window)
	if step.Window != "" {
		start, end, err := parseWindow(step.Window)
		if err != nil {
			return fmt.Errorf("%s (%s): %w", at, step.Label, err)
		}
		step.windowStart, step.windowEnd, step.windowSet = start, end, true
	}
	if step.LineStart < 0 || step.LineEnd < 0 {
		return fmt.Errorf("%s (%s): line_start/line_end must be >= 0", at, step.Label)
	}
	if step.LineStart > 0 && step.LineEnd > 0 && step.LineEnd < step.LineStart {
		return fmt.Errorf("%s (%s): line_end %d < line_start %d", at, step.Label, step.LineEnd, step.LineStart)
	}
	for _, et := range step.EventTypes {
		if !eventTypeSupported(et) {
			return fmt.Errorf("%s (%s): unknown event type %q; supported: %s", at, step.Label, et, strings.Join(supportedEventTypes, ", "))
		}
	}
	// Per-step line cap: step value > defaults value > DefaultStepMaxLines,
	// then the hard cap clamps WITH disclosure (never silently).
	requested := step.MaxLines
	if requested == 0 {
		requested = s.Defaults.MaxLines
	}
	if requested < 0 {
		return fmt.Errorf("%s (%s): max_lines must be >= 0, got %d", at, step.Label, step.MaxLines)
	}
	if requested == 0 {
		requested = DefaultStepMaxLines
	}
	step.requestedMaxRaw = requested
	step.effMaxLines = requested
	if requested > HardStepMaxLines {
		step.effMaxLines = HardStepMaxLines
		step.maxLinesClamped = true
	}
	return nil
}

// parseWindow parses the "start..end" window syntax (seconds, both endpoints
// required, end strictly greater than start). Fail-loud on any other form.
func parseWindow(raw string) (float64, float64, error) {
	parts := strings.SplitN(raw, "..", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("bad window %q: want \"<start_s>..<end_s>\"", raw)
	}
	start, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("bad window %q: start %q is not a number", raw, parts[0])
	}
	end, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, fmt.Errorf("bad window %q: end %q is not a number", raw, parts[1])
	}
	if end <= start {
		return 0, 0, fmt.Errorf("bad window %q: end must be > start", raw)
	}
	if start < 0 {
		return 0, 0, fmt.Errorf("bad window %q: start must be >= 0", raw)
	}
	return start, end, nil
}

func viewSupported(view string) bool {
	if view == ViewFormatCensus {
		return true
	}
	for _, v := range supportedEngineViews {
		if v == view {
			return true
		}
	}
	return false
}

func allSupportedViews() []string {
	out := append([]string(nil), supportedEngineViews...)
	out = append(out, ViewFormatCensus)
	sort.Strings(out)
	return out
}

func eventTypeSupported(token string) bool {
	for _, v := range supportedEventTypes {
		if v == token {
			return true
		}
	}
	return false
}
