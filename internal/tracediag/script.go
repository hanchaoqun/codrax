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
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

const (
	// ScriptVersion preserves the original v1 name for callers/tests.  V2 is
	// an additive, explicitly versioned discovery/fan-out schema; v1 scripts
	// keep their exact static execution contract.
	ScriptVersion   = 1
	ScriptVersionV2 = 2
	// DefaultStepMaxLines is the per-step body line cap when a step (and the
	// script defaults) set none.
	DefaultStepMaxLines = 800
	// HardStepMaxLines is the per-step hard cap: larger requested values are
	// CLAMPED (not rejected) and the clamp is disclosed in the report
	// (§28.12: 每步行数帽默认 800 硬帽 1000, 超设夹取+披露).
	HardStepMaxLines = 1000
	// DefaultTotalMaxLines bounds the whole report so a runaway script cannot
	// produce an unbounded file; hitting it is disclosed, never silent.
	DefaultTotalMaxLines         = 5000
	DefaultV2MaxGeneratedWindows = 8
	HardV2MaxGeneratedWindows    = 16
	DefaultV2MaxExpandedSteps    = 16
	HardV2MaxExpandedSteps       = 32
	DefaultV2MaxReportLines      = DefaultTotalMaxLines
	HardV2MaxReportLines         = DefaultTotalMaxLines
	DefaultDiscoveryMaxLines     = 80
	HardDiscoveryMaxLines        = 200
	HardV2DiscoveryCount         = 8
	// ViewFormatCensus is the tracediag-only census step (§28.12 补充裁定:
	// 格式盲点普查): whole-window format census computed in THIS package from
	// the engine's exported Index face (event names / marker forms / clock
	// tracks / sched domain / FS-IO kv coverage / power events / spans /
	// line-level quality). It is not a tracequery view.
	ViewFormatCensus = "format_census"
)

// supportedEngineViews is the canonical tracequery view enum accepted by the
// step "view" field — CONSUMED from the engine's exported enumerator
// (tracequery.CanonicalViewNames, TDIAG B1 §28.13, 2026-07-09; the former
// self-maintained copy is deleted). The enumerator is derived from the
// engine's own capacity table, so an engine view rename/addition flows here
// in the same edit — the old cross-check pin evolved into
// TestSupportedEngineViewsAreTheExportedEnumerator (导出面=引擎容量表).
// Aliases the LLM tool schema tolerates (state_churn→window_stats, …) stay
// out by construction: the capacity-table keys are canonical, and collection
// scripts are deterministic artifacts — fail-loud beats silent normalization
// (R2 discipline).
var supportedEngineViews = tracequery.CanonicalViewNames()

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
	Version     int               `yaml:"version"`
	Description string            `yaml:"description"`
	Inputs      *ScriptInputs     `yaml:"inputs"`
	Defaults    Defaults          `yaml:"defaults"`
	Limits      *V2Limits         `yaml:"limits"`
	Discoveries []WindowDiscovery `yaml:"discoveries"`
	Steps       []Step            `yaml:"steps"`

	v2Limits           V2Limits
	v2WorstReportLines int
	windowOverrideSet  bool
	tidOverride        int
	tidOverrideSet     bool
}

// ScriptInputs declares the small, typed set of runtime values a reusable v2
// collection script requires. Values are closed enums rather than arbitrary
// key/value interpolation, so adding a future input cannot turn the script
// loader into a template language.
type ScriptInputs struct {
	Window string `yaml:"window"`
	TID    string `yaml:"tid"`
}

type V2Limits struct {
	MaxGeneratedWindows int `yaml:"max_generated_windows"`
	MaxExpandedSteps    int `yaml:"max_expanded_steps"`
	MaxReportLines      int `yaml:"max_report_lines"`
}

type WindowDiscovery struct {
	Label            string   `yaml:"label"`
	Strategy         string   `yaml:"strategy"`
	Families         []string `yaml:"families"`
	Window           string   `yaml:"window"`
	LineStart        int      `yaml:"line_start"`
	LineEnd          int      `yaml:"line_end"`
	MaxWindows       int      `yaml:"max_windows"`
	MaxWindowMS      float64  `yaml:"max_window_ms"`
	PaddingMS        float64  `yaml:"padding_ms"`
	EndpointLimit    int      `yaml:"endpoint_limit"`
	ActiveLaneLimit  int      `yaml:"active_lane_limit"`
	CohortEventLimit int      `yaml:"cohort_event_limit"`
	MaxLines         int      `yaml:"max_lines"`

	windowStart     float64
	windowEnd       float64
	windowSet       bool
	windowInherited bool
	effMaxLines     int
}

func (d *WindowDiscovery) WindowBounds() (start, end float64, ok bool) {
	return d.windowStart, d.windowEnd, d.windowSet
}

func (d *WindowDiscovery) EffectiveMaxLines() int { return d.effMaxLines }

type WindowsFrom struct {
	Discovery string `yaml:"discovery"`
}

// Defaults are optional script-wide step defaults; a step field overrides.
type Defaults struct {
	PID      int    `yaml:"pid"`
	Thread   string `yaml:"thread"`
	Window   string `yaml:"window"`
	MaxLines int    `yaml:"max_lines"`
}

// ScriptOverrides is the typed CLI-to-script override boundary. Keep this a
// closed struct rather than a string map: future collection inputs can be
// added without teaching the loader to interpret arbitrary keys or weakening
// KnownFields validation for the script itself.
type ScriptOverrides struct {
	Window string
	TID    string
}

// Step is one collection step. V2 adds only the typed pid_from and
// windows_from bindings to the original §28.12 static field set.
type Step struct {
	Label            string       `yaml:"label"`
	View             string       `yaml:"view"`
	PID              int          `yaml:"pid"`
	PIDFrom          string       `yaml:"pid_from"`
	Thread           string       `yaml:"thread"`
	Window           string       `yaml:"window"`
	LineStart        int          `yaml:"line_start"`
	LineEnd          int          `yaml:"line_end"`
	Pattern          string       `yaml:"pattern"`
	EventTypes       []string     `yaml:"event_types"`
	TraceMarkActions []string     `yaml:"trace_mark_actions"`
	MaxLines         int          `yaml:"max_lines"`
	WindowsFrom      *WindowsFrom `yaml:"windows_from"`

	// Resolved fields (populated by Validate; not part of the YAML schema).
	windowStart     float64
	windowEnd       float64
	windowSet       bool
	windowInherited bool
	effMaxLines     int
	maxLinesClamped bool
	requestedMaxRaw int
	windowOrigin    *WindowProvenance
	pidFromResolved bool
}

type WindowProvenance struct {
	DiscoveryLabel      string
	WindowOrdinal       int
	CandidateRank       int
	CandidateWindow     int
	Family              string
	Kind                string
	CoreStartTs         float64
	CoreEndTs           float64
	CoreLineStart       int
	CoreLineEnd         int
	RankBasis           string
	IdentityFingerprint string
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
	return LoadScriptWithOverrides(path, ScriptOverrides{})
}

// LoadScriptWithOverrides applies typed field-operator inputs before strict
// validation. Window replaces defaults.window only; explicit per-step and
// per-discovery windows remain authoritative. Shipped generic templates
// deliberately inherit the default so one CLI parent window can drive both
// today's pairing strategy and future typed discovery strategies.
func LoadScriptWithOverrides(path string, overrides ScriptOverrides) (*Script, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("tracediag: read script: %w", err)
	}
	return parseScript(data, overrides)
}

// ParseScript strict-decodes a collection script from bytes and validates it.
func ParseScript(data []byte) (*Script, error) {
	return parseScript(data, ScriptOverrides{})
}

func parseScript(data []byte, overrides ScriptOverrides) (*Script, error) {
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	// R2 family discipline: the schema IS the decoder — unknown keys fail
	// loud instead of being silently dropped.
	dec.KnownFields(true)
	var script Script
	if err := dec.Decode(&script); err != nil {
		return nil, fmt.Errorf("tracediag: script decode failed (unknown keys are rejected; step fields include event_types/trace_mark_actions; v2 adds inputs/limits/discoveries/windows_from/pid_from): %w", err)
	}
	if override := strings.TrimSpace(overrides.Window); override != "" {
		script.Defaults.Window = override
		script.windowOverrideSet = true
	}
	if override := strings.TrimSpace(overrides.TID); override != "" {
		tid, err := parseTIDOverride(override)
		if err != nil {
			return nil, fmt.Errorf("tracediag: --trace-tid: %w", err)
		}
		script.tidOverride = tid
		script.tidOverrideSet = true
	}
	if err := script.Validate(); err != nil {
		return nil, err
	}
	return &script, nil
}

// Validate applies the fail-loud schema rules and resolves defaults, window
// bounds and line caps onto each step.
func (s *Script) Validate() error {
	if s.Version != ScriptVersion && s.Version != ScriptVersionV2 {
		return fmt.Errorf("tracediag: unsupported script version %d (supported: %d, %d)", s.Version, ScriptVersion, ScriptVersionV2)
	}
	if len(s.Steps) == 0 {
		return fmt.Errorf("tracediag: script has no steps")
	}
	if s.Version == ScriptVersion && s.tidOverrideSet {
		return fmt.Errorf("tracediag: --trace-tid requires a version: 2 script with inputs.tid and pid_from: tid")
	}
	if s.Version == ScriptVersion && (s.Inputs != nil || s.Limits != nil || len(s.Discoveries) > 0 || scriptHasWindowsFrom(s.Steps) || scriptHasPIDFrom(s.Steps)) {
		return fmt.Errorf("tracediag: inputs/limits/discoveries/windows_from/pid_from are v2-only fields; set version: 2")
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
	discoveries := map[string]*WindowDiscovery{}
	if s.Version == ScriptVersionV2 {
		if err := s.validateV2Inputs(); err != nil {
			return err
		}
		if err := s.validateV2Limits(); err != nil {
			return err
		}
		if len(s.Discoveries) > HardV2DiscoveryCount {
			return fmt.Errorf("tracediag: discoveries=%d exceeds hard cap %d", len(s.Discoveries), HardV2DiscoveryCount)
		}
		for i := range s.Discoveries {
			if err := s.validateDiscovery(i, &s.Discoveries[i], seen); err != nil {
				return err
			}
			discoveries[s.Discoveries[i].Label] = &s.Discoveries[i]
		}
	}
	for i := range s.Steps {
		step := &s.Steps[i]
		if err := s.validateStep(i, step, seen, discoveries); err != nil {
			return err
		}
	}
	if s.Version == ScriptVersionV2 {
		if err := s.validateV2InputConsumers(); err != nil {
			return err
		}
		if err := s.validateV2Budgets(discoveries); err != nil {
			return err
		}
	}
	return nil
}

func (s *Script) validateV2Inputs() error {
	if s.Inputs == nil {
		if s.tidOverrideSet {
			return fmt.Errorf("tracediag: --trace-tid was provided but the script does not declare inputs.tid")
		}
		return nil
	}
	s.Inputs.Window = strings.TrimSpace(s.Inputs.Window)
	s.Inputs.TID = strings.TrimSpace(s.Inputs.TID)
	if s.Inputs.Window == "" && s.Inputs.TID == "" {
		return fmt.Errorf("tracediag: inputs must declare at least one typed input (supported: window, tid)")
	}
	if s.Inputs.Window != "" && s.Inputs.Window != "required" {
		return fmt.Errorf("tracediag: inputs.window=%q is unsupported; supported: required", s.Inputs.Window)
	}
	if s.Inputs.TID != "" && s.Inputs.TID != "required" {
		return fmt.Errorf("tracediag: inputs.tid=%q is unsupported; supported: required", s.Inputs.TID)
	}
	if s.Inputs.Window == "required" && !s.windowOverrideSet {
		return fmt.Errorf("tracediag: this script requires --trace-window <start_s>..<end_s>")
	}
	if s.Inputs.TID == "required" && !s.tidOverrideSet {
		return fmt.Errorf("tracediag: this script requires --trace-tid <positive_tid>")
	}
	if s.tidOverrideSet && s.Inputs.TID == "" {
		return fmt.Errorf("tracediag: --trace-tid was provided but the script does not declare inputs.tid")
	}
	return nil
}

func (s *Script) validateV2InputConsumers() error {
	if s.Inputs != nil && s.Inputs.Window == "required" {
		consumed := false
		for i := range s.Discoveries {
			if s.Discoveries[i].windowInherited {
				consumed = true
				break
			}
		}
		if !consumed {
			for i := range s.Steps {
				if s.Steps[i].windowInherited {
					consumed = true
					break
				}
			}
		}
		if !consumed {
			return fmt.Errorf("tracediag: inputs.window=required is unused; at least one discovery or static step must inherit defaults.window")
		}
	}
	if s.Inputs == nil || s.Inputs.TID != "required" {
		return nil
	}
	for i := range s.Steps {
		if s.Steps[i].PIDFrom == "tid" {
			return nil
		}
	}
	return fmt.Errorf("tracediag: inputs.tid=required is unused; at least one step must set pid_from: tid")
}

func (s *Script) validateStep(i int, step *Step, seen map[string]bool, discoveries map[string]*WindowDiscovery) error {
	at := fmt.Sprintf("tracediag: steps[%d]", i)
	step.Label = strings.TrimSpace(step.Label)
	if step.Label == "" {
		return fmt.Errorf("%s: label is required", at)
	}
	if seen[step.Label] {
		return fmt.Errorf("%s: duplicate label %q", at, step.Label)
	}
	seen[step.Label] = true
	if s.Version == ScriptVersionV2 && !v2LabelRE.MatchString(step.Label) {
		return fmt.Errorf("%s: label %q must match %s", at, step.Label, v2LabelRE.String())
	}
	step.View = strings.TrimSpace(step.View)
	if step.View == "" {
		return fmt.Errorf("%s (%s): view is required; supported: %s", at, step.Label, strings.Join(allSupportedViews(), ", "))
	}
	if !viewSupported(step.View) {
		return fmt.Errorf("%s (%s): unknown view %q; supported: %s", at, step.Label, step.View, strings.Join(allSupportedViews(), ", "))
	}
	step.PIDFrom = strings.TrimSpace(step.PIDFrom)
	if step.PIDFrom != "" {
		if step.PIDFrom != "tid" {
			return fmt.Errorf("%s (%s): pid_from=%q is unsupported; supported: tid", at, step.Label, step.PIDFrom)
		}
		if s.Inputs == nil || s.Inputs.TID != "required" {
			return fmt.Errorf("%s (%s): pid_from: tid requires inputs.tid: required", at, step.Label)
		}
		if !step.pidFromResolved && (step.PID != 0 || strings.TrimSpace(step.Thread) != "") {
			return fmt.Errorf("%s (%s): pid_from cannot be combined with pid or thread", at, step.Label)
		}
		if step.pidFromResolved && (step.PID != s.tidOverride || strings.TrimSpace(step.Thread) != "") {
			return fmt.Errorf("%s (%s): resolved pid_from binding was modified", at, step.Label)
		}
		step.PID = s.tidOverride
		step.Thread = ""
		step.pidFromResolved = true
	} else {
		// Defaults inheritance: a step-zero field inherits the script default.
		if step.PID == 0 {
			step.PID = s.Defaults.PID
		}
		if strings.TrimSpace(step.Thread) == "" {
			step.Thread = s.Defaults.Thread
		}
	}
	if step.PID < 0 {
		return fmt.Errorf("%s (%s): pid must be >= 0, got %d", at, step.Label, step.PID)
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
	if step.WindowsFrom == nil && strings.TrimSpace(step.Window) == "" {
		step.Window = s.Defaults.Window
		step.windowInherited = true
	}
	step.Window = strings.TrimSpace(step.Window)
	if step.WindowsFrom != nil && step.Window != "" {
		return fmt.Errorf("%s (%s): window and windows_from cannot both be set", at, step.Label)
	}
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
	if step.WindowsFrom != nil {
		step.WindowsFrom.Discovery = strings.TrimSpace(step.WindowsFrom.Discovery)
		if step.WindowsFrom.Discovery == "" {
			return fmt.Errorf("%s (%s): windows_from.discovery is required", at, step.Label)
		}
		if discoveries[step.WindowsFrom.Discovery] == nil {
			return fmt.Errorf("%s (%s): windows_from references unknown discovery %q", at, step.Label, step.WindowsFrom.Discovery)
		}
		if step.LineStart > 0 || step.LineEnd > 0 {
			return fmt.Errorf("%s (%s): generated time windows cannot be combined with line_start/line_end (engine line bounds would disable the time window)", at, step.Label)
		}
	}
	for _, et := range step.EventTypes {
		if !eventTypeSupported(et) {
			return fmt.Errorf("%s (%s): unknown event type %q; supported: %s", at, step.Label, et, strings.Join(supportedEventTypes, ", "))
		}
	}
	eventTypes := make([]tracequery.EventType, 0, len(step.EventTypes))
	for _, eventType := range step.EventTypes {
		eventTypes = append(eventTypes, tracequery.EventType(eventType))
	}
	actions := make([]tracequery.TraceMarkAction, 0, len(step.TraceMarkActions))
	for _, action := range step.TraceMarkActions {
		actions = append(actions, tracequery.TraceMarkAction(action))
	}
	if err := tracequery.ValidateTraceMarkActionFilter(step.View, eventTypes, actions); err != nil {
		return fmt.Errorf("%s (%s): %w", at, step.Label, err)
	}
	if step.View == tracequery.FallbackViewEventSearch && (step.PID > 0 || step.Thread != "") {
		if global := tracequery.CPUGlobalEventSearchTypes(eventTypes); len(global) > 0 {
			parts := make([]string, 0, len(global))
			for _, eventType := range global {
				parts = append(parts, string(eventType))
			}
			return fmt.Errorf("%s (%s): pid/thread cannot be combined with CPU-global event_search types [%s]; the selector filters incidental emitter identity, not CPU-state ownership", at, step.Label, strings.Join(parts, ","))
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
	if step.View == "event_search" && step.effMaxLines < eventSearchReportBaseLines {
		return fmt.Errorf("%s (%s): event_search max_lines=%d is too small; need >=%d for result/window metadata plus visible match accounting", at, step.Label, step.effMaxLines, eventSearchReportBaseLines)
	}
	if step.WindowsFrom != nil && step.View == "event_search" && step.effMaxLines < 5 {
		return fmt.Errorf("%s (%s): generated event_search max_lines=%d is too small; need >=5 for result/window metadata plus at least one complete start/done pair", at, step.Label, step.effMaxLines)
	}
	return nil
}

var v2LabelRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

func scriptHasWindowsFrom(steps []Step) bool {
	for i := range steps {
		if steps[i].WindowsFrom != nil {
			return true
		}
	}
	return false
}

func scriptHasPIDFrom(steps []Step) bool {
	for i := range steps {
		if strings.TrimSpace(steps[i].PIDFrom) != "" {
			return true
		}
	}
	return false
}

func (s *Script) validateV2Limits() error {
	limits := V2Limits{}
	if s.Limits != nil {
		limits = *s.Limits
	}
	if limits.MaxGeneratedWindows == 0 {
		limits.MaxGeneratedWindows = DefaultV2MaxGeneratedWindows
	}
	if limits.MaxExpandedSteps == 0 {
		limits.MaxExpandedSteps = DefaultV2MaxExpandedSteps
	}
	if limits.MaxReportLines == 0 {
		limits.MaxReportLines = DefaultV2MaxReportLines
	}
	if limits.MaxGeneratedWindows < 1 || limits.MaxGeneratedWindows > HardV2MaxGeneratedWindows {
		return fmt.Errorf("tracediag: limits.max_generated_windows=%d outside 1..%d", limits.MaxGeneratedWindows, HardV2MaxGeneratedWindows)
	}
	if limits.MaxExpandedSteps < 1 || limits.MaxExpandedSteps > HardV2MaxExpandedSteps {
		return fmt.Errorf("tracediag: limits.max_expanded_steps=%d outside 1..%d", limits.MaxExpandedSteps, HardV2MaxExpandedSteps)
	}
	if limits.MaxReportLines < 100 || limits.MaxReportLines > HardV2MaxReportLines {
		return fmt.Errorf("tracediag: limits.max_report_lines=%d outside 100..%d", limits.MaxReportLines, HardV2MaxReportLines)
	}
	s.v2Limits = limits
	return nil
}

func (s *Script) validateDiscovery(i int, discovery *WindowDiscovery, seen map[string]bool) error {
	at := fmt.Sprintf("tracediag: discoveries[%d]", i)
	discovery.Label = strings.TrimSpace(discovery.Label)
	if discovery.Label == "" {
		return fmt.Errorf("%s: label is required", at)
	}
	if !v2LabelRE.MatchString(discovery.Label) {
		return fmt.Errorf("%s: label %q must match %s", at, discovery.Label, v2LabelRE.String())
	}
	if seen[discovery.Label] {
		return fmt.Errorf("%s: duplicate label %q across discoveries/steps", at, discovery.Label)
	}
	seen[discovery.Label] = true
	discovery.Strategy = strings.TrimSpace(discovery.Strategy)
	supportedStrategy := false
	for _, strategy := range tracequery.WindowDiscoveryStrategyNames() {
		if discovery.Strategy == strategy {
			supportedStrategy = true
			break
		}
	}
	if !supportedStrategy {
		return fmt.Errorf("%s (%s): unknown strategy %q; supported: %s", at, discovery.Label, discovery.Strategy, strings.Join(tracequery.WindowDiscoveryStrategyNames(), ", "))
	}
	knownFamilies := map[string]bool{}
	for _, family := range tracequery.WindowDiscoveryFamilyNames(tracequery.WindowDiscoveryStrategy(discovery.Strategy)) {
		knownFamilies[family] = true
	}
	seenFamily := map[string]bool{}
	for _, family := range discovery.Families {
		if !knownFamilies[family] {
			return fmt.Errorf("%s (%s): unknown family %q", at, discovery.Label, family)
		}
		if seenFamily[family] {
			return fmt.Errorf("%s (%s): duplicate family %q", at, discovery.Label, family)
		}
		seenFamily[family] = true
	}
	if strings.TrimSpace(discovery.Window) == "" {
		discovery.Window = s.Defaults.Window
		discovery.windowInherited = true
	}
	discovery.Window = strings.TrimSpace(discovery.Window)
	if discovery.Window != "" {
		start, end, err := parseWindow(discovery.Window)
		if err != nil {
			return fmt.Errorf("%s (%s): %w", at, discovery.Label, err)
		}
		discovery.windowStart, discovery.windowEnd, discovery.windowSet = start, end, true
	}
	if discovery.LineStart < 0 || discovery.LineEnd < 0 || (discovery.LineStart > 0 && discovery.LineEnd > 0 && discovery.LineEnd < discovery.LineStart) {
		return fmt.Errorf("%s (%s): invalid line range %d..%d", at, discovery.Label, discovery.LineStart, discovery.LineEnd)
	}
	if discovery.windowSet && (discovery.LineStart > 0 || discovery.LineEnd > 0) {
		return fmt.Errorf("%s (%s): window and line range cannot be combined", at, discovery.Label)
	}
	if discovery.MaxWindows == 0 {
		discovery.MaxWindows = tracequery.DefaultWindowDiscoveryMaxWindows
	}
	if discovery.MaxWindows < 1 || discovery.MaxWindows > tracequery.HardWindowDiscoveryMaxWindows {
		return fmt.Errorf("%s (%s): max_windows=%d outside 1..%d", at, discovery.Label, discovery.MaxWindows, tracequery.HardWindowDiscoveryMaxWindows)
	}
	if discovery.MaxWindowMS == 0 {
		discovery.MaxWindowMS = tracequery.DefaultWindowDiscoveryMaxWindowMs
	}
	if math.IsNaN(discovery.MaxWindowMS) || math.IsInf(discovery.MaxWindowMS, 0) || discovery.MaxWindowMS <= 0 || discovery.MaxWindowMS > tracequery.HardPairingDiscoveryMaxWindowMs {
		return fmt.Errorf("%s (%s): max_window_ms=%v outside (0,%.0f]", at, discovery.Label, discovery.MaxWindowMS, tracequery.HardPairingDiscoveryMaxWindowMs)
	}
	if discovery.PaddingMS == 0 {
		discovery.PaddingMS = tracequery.DefaultWindowDiscoveryPaddingMs
	}
	if math.IsNaN(discovery.PaddingMS) || math.IsInf(discovery.PaddingMS, 0) || discovery.PaddingMS < 0 || discovery.PaddingMS*2 > discovery.MaxWindowMS {
		return fmt.Errorf("%s (%s): padding_ms=%v must be finite and <= max_window_ms/2", at, discovery.Label, discovery.PaddingMS)
	}
	if discovery.EndpointLimit < 0 || discovery.EndpointLimit > tracequery.HardWindowDiscoveryEndpointLimit {
		return fmt.Errorf("%s (%s): endpoint_limit=%d must be 0(default) or 1..%d", at, discovery.Label, discovery.EndpointLimit, tracequery.HardWindowDiscoveryEndpointLimit)
	}
	if discovery.ActiveLaneLimit < 0 || discovery.ActiveLaneLimit > tracequery.HardWindowDiscoveryActiveLaneLimit {
		return fmt.Errorf("%s (%s): active_lane_limit=%d must be 0(default) or 1..%d", at, discovery.Label, discovery.ActiveLaneLimit, tracequery.HardWindowDiscoveryActiveLaneLimit)
	}
	if discovery.CohortEventLimit < 0 || discovery.CohortEventLimit == 1 || discovery.CohortEventLimit > tracequery.HardWindowDiscoveryCohortEventLimit {
		return fmt.Errorf("%s (%s): cohort_event_limit=%d must be 0(default) or 2..%d", at, discovery.Label, discovery.CohortEventLimit, tracequery.HardWindowDiscoveryCohortEventLimit)
	}
	if discovery.MaxLines == 0 {
		discovery.MaxLines = DefaultDiscoveryMaxLines
	}
	if discovery.MaxLines < 1 || discovery.MaxLines > HardDiscoveryMaxLines {
		return fmt.Errorf("%s (%s): max_lines=%d outside 1..%d", at, discovery.Label, discovery.MaxLines, HardDiscoveryMaxLines)
	}
	discovery.effMaxLines = discovery.MaxLines
	return nil
}

func (s *Script) validateV2Budgets(discoveries map[string]*WindowDiscovery) error {
	generated := 0
	for i := range s.Discoveries {
		generated += s.Discoveries[i].MaxWindows
	}
	if generated > s.v2Limits.MaxGeneratedWindows {
		return fmt.Errorf("tracediag: worst-case generated windows=%d exceeds limits.max_generated_windows=%d", generated, s.v2Limits.MaxGeneratedWindows)
	}
	expanded := 0
	// Reserve two lines for optional CLI override provenance (window + TID).
	// They are runtime options, so the script-only planner budgets both.
	worstLines := 66
	for i := range s.Discoveries {
		worstLines += s.Discoveries[i].EffectiveMaxLines() + 12
	}
	for i := range s.Steps {
		step := &s.Steps[i]
		multiplier := 1
		if step.WindowsFrom != nil {
			multiplier = discoveries[step.WindowsFrom.Discovery].MaxWindows
		}
		expanded += multiplier
		worstLines += multiplier * (step.EffectiveMaxLines() + 12)
	}
	if expanded > s.v2Limits.MaxExpandedSteps {
		return fmt.Errorf("tracediag: worst-case expanded steps=%d exceeds limits.max_expanded_steps=%d", expanded, s.v2Limits.MaxExpandedSteps)
	}
	if worstLines > s.v2Limits.MaxReportLines {
		return fmt.Errorf("tracediag: worst-case report lines=%d exceeds limits.max_report_lines=%d; reduce discovery max_windows or step/discovery max_lines", worstLines, s.v2Limits.MaxReportLines)
	}
	s.v2WorstReportLines = worstLines
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
	if math.IsNaN(start) || math.IsInf(start, 0) || math.IsNaN(end) || math.IsInf(end, 0) {
		return 0, 0, fmt.Errorf("bad window %q: endpoints must be finite", raw)
	}
	if end <= start {
		return 0, 0, fmt.Errorf("bad window %q: end must be > start", raw)
	}
	if start < 0 {
		return 0, 0, fmt.Errorf("bad window %q: start must be >= 0", raw)
	}
	return start, end, nil
}

func parseTIDOverride(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, fmt.Errorf("value is empty")
	}
	for _, r := range raw {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("%q is not a positive base-10 integer", raw)
		}
	}
	value, err := strconv.ParseUint(raw, 10, 31)
	if err != nil || value == 0 {
		return 0, fmt.Errorf("%q must be in 1..2147483647", raw)
	}
	return int(value), nil
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
