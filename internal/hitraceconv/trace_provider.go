package hitraceconv

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	codraxtypes "github.com/hanchaoqun/codrax/internal/types"
)

const (
	traceEngineAuto          = "auto"
	traceEngineTraceStreamer = "trace_streamer"
	traceEngineBuiltin       = "builtin"
	traceEngineDirectPerf    = "direct_perf"

	traceProviderStageTraceBody = "trace_body"

	traceProviderKindOfficialDB = "official_trace_db"
	traceProviderKindBuiltin    = "builtin_modern"
	traceProviderKindBuiltinSys = "builtin_sys_binary"
	traceProviderKindNotNeeded  = "not_applicable"

	traceProviderNameTraceStreamer = "trace_streamer_db"
	traceProviderNameBuiltinModern = "codrax_builtin_modern_profiler"
	traceProviderNameBuiltinSys    = "codrax_builtin_sys_binary"
	traceProviderNameDirectPerf    = "direct_perf_input"
)

type traceProviderSpec struct {
	Kind        string
	Name        string
	Fallback    bool
	Implemented bool
}

// traceProviderPlan is the single typed route consumed by both tool status and
// conversion execution. OrderedEngines is authoritative: auto is
// trace_streamer first followed by the built-in fallback, while each explicit
// mode contains exactly one engine and therefore cannot degrade implicitly.
type traceProviderPlan struct {
	RequestedEngine  string
	PreflightEngine  string
	OrderedEngines   []string
	DirectPerf       bool
	ExecutionBlocker string
	TraceStreamer    traceProviderLanePlan
	Builtin          traceProviderLanePlan
}

func traceInputUsesDirectPerfRoute(opts Options) bool {
	if requestedTraceEngineMode(opts.TraceEngine) == traceEngineTraceStreamer {
		return false
	}
	input := strings.TrimSpace(opts.InputPath)
	return input != "" && simpleperfDirectRequested(detectPerfInputFormat(input))
}

type traceProviderLanePlan struct {
	Engine    string
	Provider  traceProviderSpec
	Available bool
	Selected  bool
	Path      string
	Source    string
	Caveats   []string
}

func (p traceProviderPlan) includesEngine(engine string) bool {
	for _, candidate := range p.OrderedEngines {
		if candidate == engine {
			return true
		}
	}
	return false
}

func (p traceProviderPlan) allowsBuiltinFallback() bool {
	return p.RequestedEngine == traceEngineAuto && p.includesEngine(traceEngineBuiltin)
}

// TraceProviderFallbackError preserves both failed lanes when auto cannot
// produce a trace body. FirstDecision/Caveats retain the trace_streamer
// provider evidence; Unwrap returns the built-in failure so errors.As still
// reaches the exact *BuiltinSysDecodeError code.
type TraceProviderFallbackError struct {
	FirstDecision TraceProviderDecision
	FirstSource   string
	FirstPath     string
	FirstStage    string
	FirstCode     string
	FirstCaveats  []string
	FirstCause    error
	RolledBackDB  string
	Fallback      error
}

func (e *TraceProviderFallbackError) Error() string {
	if e == nil {
		return "trace provider fallback failed"
	}
	first := strings.TrimSpace(e.FirstDecision.ProviderName)
	if first == "" {
		first = "unknown"
	}
	reason := strings.TrimSpace(e.FirstDecision.Reason)
	if reason == "" {
		reason = "unknown"
	}
	message := fmt.Sprintf("trace provider fallback failed: first_provider=%s source=%s path=%s reason=%s",
		strconv.Quote(first), strconv.Quote(strings.TrimSpace(e.FirstSource)), strconv.Quote(strings.TrimSpace(e.FirstPath)), strconv.Quote(reason))
	if stage := strings.TrimSpace(e.FirstStage); stage != "" {
		message += " stage=" + strconv.Quote(stage)
	}
	if code := strings.TrimSpace(e.FirstCode); code != "" {
		message += " code=" + strconv.Quote(code)
	}
	if caveat := strings.TrimSpace(e.FirstDecision.Caveat); caveat != "" {
		message += " caveat=" + strconv.Quote(caveat)
	}
	if rolledBackDB := strings.TrimSpace(e.RolledBackDB); rolledBackDB != "" {
		message += " rolled_back_db=" + strconv.Quote(rolledBackDB)
	}
	firstCaveats := dedupeStrings(e.FirstCaveats)
	if decisionCaveat := strings.TrimSpace(e.FirstDecision.Caveat); decisionCaveat != "" {
		filtered := firstCaveats[:0]
		for _, caveat := range firstCaveats {
			if caveat != decisionCaveat {
				filtered = append(filtered, caveat)
			}
		}
		firstCaveats = filtered
	}
	if len(firstCaveats) > 0 {
		message += " first_caveats=" + strconv.Quote(strings.Join(firstCaveats, " | "))
	}
	if e.Fallback != nil {
		message += "; fallback: " + e.Fallback.Error()
	}
	return boundedTraceProviderErrorText(message, 8192)
}

func (e *TraceProviderFallbackError) Unwrap() []error {
	if e == nil {
		return nil
	}
	causes := make([]error, 0, 2)
	if e.FirstCause != nil {
		causes = append(causes, e.FirstCause)
	}
	if e.Fallback != nil {
		causes = append(causes, e.Fallback)
	}
	return causes
}

func boundedTraceProviderErrorText(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}
	const marker = "..."
	if maxBytes <= len(marker) {
		return codraxtypes.CutPrefixRuneSafe(marker, maxBytes)
	}
	return codraxtypes.CutPrefixRuneSafe(value, maxBytes-len(marker)) + marker
}

var traceProviderRegistry = []traceProviderSpec{
	{
		Kind:        traceProviderKindOfficialDB,
		Name:        traceProviderNameTraceStreamer,
		Implemented: true,
	},
	{
		Kind:        traceProviderKindBuiltin,
		Name:        traceProviderNameBuiltinModern,
		Fallback:    true,
		Implemented: true,
	},
	{
		Kind:        traceProviderKindBuiltinSys,
		Name:        traceProviderNameBuiltinSys,
		Fallback:    true,
		Implemented: true,
	},
	{
		Kind:        traceProviderKindNotNeeded,
		Name:        traceProviderNameDirectPerf,
		Implemented: true,
	},
}

func traceProviderByName(name string) traceProviderSpec {
	for _, provider := range traceProviderRegistry {
		if provider.Name == name {
			return provider
		}
	}
	return traceProviderSpec{Name: strings.TrimSpace(name)}
}

func normalizeTraceEngineMode(mode string) string {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	return normalized
}

func selectedTraceEngineMode(mode string) string {
	normalized := normalizeTraceEngineMode(mode)
	switch normalized {
	case "", traceEngineAuto:
		return traceEngineTraceStreamer
	default:
		return normalized
	}
}

func requestedTraceEngineMode(mode string) string {
	normalized := normalizeTraceEngineMode(mode)
	if normalized == "" {
		return traceEngineAuto
	}
	return normalized
}

func isAutoTraceEngineMode(mode string) bool {
	return requestedTraceEngineMode(mode) == traceEngineAuto
}

func validateTraceEngineMode(mode string) error {
	switch normalizeTraceEngineMode(mode) {
	case "", traceEngineAuto, traceEngineTraceStreamer, traceEngineBuiltin:
		return nil
	default:
		return errUnsupportedTraceEngine(mode)
	}
}

// ValidateOptions is the single conversion-time option authority shared by
// library callers, CLI, and REPL entry points. Status-only inspection may
// intentionally carry trace_streamer options while showing an explicitly
// selected built-in route, so BuildTraceToolStatus validates only the engine
// enum and does not call this conversion conflict gate.
func ValidateOptions(opts Options) error {
	if err := validatePerfParserMode(opts.PerfParser); err != nil {
		return err
	}
	if err := validateTraceEngineMode(opts.TraceEngine); err != nil {
		return err
	}
	// Direct-perf classification is a precise input-format decision and is
	// authoritative before trace-engine conflict checks. Status and execution
	// must report the same trace-only-option blocker for auto and builtin; an
	// explicit trace_streamer request intentionally bypasses this route.
	if traceInputUsesDirectPerfRoute(opts) {
		if err := validateDirectPerfTraceOptions(opts); err != nil {
			return err
		}
	}
	if err := validateTraceOutputPathCollisions(opts); err != nil {
		return err
	}
	if requestedTraceEngineMode(opts.TraceEngine) == traceEngineBuiltin {
		var conflicts []string
		if strings.TrimSpace(opts.TraceStreamerPath) != "" {
			conflicts = append(conflicts, "--trace-streamer")
		}
		if strings.TrimSpace(opts.TraceDBOutputPath) != "" {
			conflicts = append(conflicts, "--trace-db-output")
		}
		if opts.KeepTraceDB {
			conflicts = append(conflicts, "--keep-trace-db")
		}
		for _, dir := range opts.TraceStreamerSoDirs {
			if strings.TrimSpace(dir) != "" {
				conflicts = append(conflicts, "--trace-streamer-so-dir")
				break
			}
		}
		if len(conflicts) != 0 {
			sort.Strings(conflicts)
			return fmt.Errorf("--trace-engine=builtin bypasses trace_streamer and cannot be combined with %s", strings.Join(conflicts, ", "))
		}
	}
	return nil
}

type traceCanonicalPath struct {
	path string
	info os.FileInfo
}

func validateTraceOutputPathCollisions(opts Options) error {
	input := strings.TrimSpace(opts.InputPath)
	if input == "" {
		return nil
	}
	output := strings.TrimSpace(opts.OutputPath)
	if output == "" {
		output = DefaultOutputPath(input)
	}
	inputPath, err := canonicalTracePath(input)
	if err != nil {
		return fmt.Errorf("resolve trace input path %s: %w", input, err)
	}
	outputPath, err := canonicalTracePath(output)
	if err != nil {
		return fmt.Errorf("resolve trace output path %s: %w", output, err)
	}
	if traceCanonicalPathsEqual(inputPath, outputPath) {
		return fmt.Errorf("trace input and systrace output must be different files: input=%s output=%s", input, output)
	}
	bundle := traceSidecarBase(input, output) + ".tracebundle.json"
	bundlePath, err := canonicalTracePath(bundle)
	if err != nil {
		return fmt.Errorf("resolve tracebundle output path %s: %w", bundle, err)
	}
	if traceCanonicalPathsEqual(inputPath, bundlePath) {
		return fmt.Errorf("trace input and tracebundle output must be different files: input=%s tracebundle=%s", input, bundle)
	}
	if traceCanonicalPathsEqual(outputPath, bundlePath) {
		return fmt.Errorf("systrace output and tracebundle output must be different files: output=%s tracebundle=%s", output, bundle)
	}
	db := retainedTraceDBOutputPath(opts, input, output)
	if db == "" {
		return nil
	}
	dbPath, err := canonicalTracePath(db)
	if err != nil {
		return fmt.Errorf("resolve trace DB output path %s: %w", db, err)
	}
	if traceCanonicalPathsEqual(inputPath, dbPath) {
		return fmt.Errorf("trace input and trace DB output must be different files: input=%s trace_db_output=%s", input, db)
	}
	if traceCanonicalPathsEqual(outputPath, dbPath) {
		return fmt.Errorf("systrace output and trace DB output must be different files: output=%s trace_db_output=%s", output, db)
	}
	if traceCanonicalPathsEqual(bundlePath, dbPath) {
		return fmt.Errorf("tracebundle output and trace DB output must be different files: tracebundle=%s trace_db_output=%s", bundle, db)
	}
	companion := db + ".ohos.ts"
	companionPath, err := canonicalTracePath(companion)
	if err != nil {
		return fmt.Errorf("resolve trace DB companion output path %s: %w", companion, err)
	}
	for _, candidate := range []struct {
		name string
		path traceCanonicalPath
	}{{"trace input", inputPath}, {"systrace output", outputPath}, {"tracebundle output", bundlePath}, {"trace DB output", dbPath}} {
		if traceCanonicalPathsEqual(candidate.path, companionPath) {
			return fmt.Errorf("%s and trace DB companion output must be different files: trace_db_companion=%s", candidate.name, companion)
		}
	}
	return nil
}

func retainedTraceDBOutputPath(opts Options, input, output string) string {
	if path := strings.TrimSpace(opts.TraceDBOutputPath); path != "" {
		return path
	}
	if opts.KeepTraceDB {
		return traceSidecarBase(input, output) + ".trace.db"
	}
	return ""
}

func validateDirectPerfTraceOptions(opts Options) error {
	var conflicts []string
	if strings.TrimSpace(opts.TraceStreamerPath) != "" {
		conflicts = append(conflicts, "--trace-streamer")
	}
	if strings.TrimSpace(opts.TraceDBOutputPath) != "" {
		conflicts = append(conflicts, "--trace-db-output")
	}
	if opts.KeepTraceDB {
		conflicts = append(conflicts, "--keep-trace-db")
	}
	for _, dir := range opts.TraceStreamerSoDirs {
		if strings.TrimSpace(dir) != "" {
			conflicts = append(conflicts, "--trace-streamer-so-dir")
			break
		}
	}
	if len(conflicts) == 0 {
		return nil
	}
	sort.Strings(conflicts)
	return fmt.Errorf("direct perf input has no trace body and cannot be combined with trace-only option(s) %s", strings.Join(conflicts, ", "))
}

func preflightTracePublicationPaths(opts Options, input, output string, systrace bool) error {
	paths := make([]string, 0, 4)
	if systrace {
		paths = append(paths, output)
	}
	paths = append(paths, traceSidecarBase(input, output)+".tracebundle.json")
	if !systrace && !opts.DisablePerfAdapter {
		paths = append(paths, traceSidecarBase(input, output)+".perftrace")
	}
	if db := retainedTraceDBOutputPath(opts, input, output); db != "" {
		paths = append(paths, db, db+".ohos.ts")
	}
	for _, path := range uniqueNonEmptyStrings(paths) {
		parent := filepath.Dir(path)
		info, err := os.Stat(parent)
		if err != nil {
			return fmt.Errorf("inspect publication directory %s for %s: %w", parent, path, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("publication parent is not a directory: path=%s parent=%s", path, parent)
		}
		if err := ensureOutputDoesNotExist(path); err != nil {
			return err
		}
	}
	return nil
}

func canonicalTracePath(raw string) (traceCanonicalPath, error) {
	abs, err := filepath.Abs(filepath.Clean(raw))
	if err != nil {
		return traceCanonicalPath{}, err
	}
	abs = filepath.Clean(abs)
	info, statErr := os.Stat(abs)
	if statErr == nil {
		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			return traceCanonicalPath{}, err
		}
		return traceCanonicalPath{path: filepath.Clean(resolved), info: info}, nil
	}
	if !os.IsNotExist(statErr) {
		return traceCanonicalPath{}, statErr
	}

	// Resolve the nearest existing ancestor so a prospective output below a
	// symlinked directory cannot alias the input while its leaf does not yet
	// exist.
	ancestor := abs
	var suffix []string
	for {
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			break
		}
		suffix = append(suffix, filepath.Base(ancestor))
		ancestor = parent
		if _, err := os.Stat(ancestor); err == nil {
			resolved, err := filepath.EvalSymlinks(ancestor)
			if err != nil {
				return traceCanonicalPath{}, err
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			return traceCanonicalPath{path: filepath.Clean(resolved)}, nil
		} else if !os.IsNotExist(err) {
			return traceCanonicalPath{}, err
		}
	}
	return traceCanonicalPath{path: abs}, nil
}

func traceCanonicalPathsEqual(left, right traceCanonicalPath) bool {
	if left.info != nil && right.info != nil && os.SameFile(left.info, right.info) {
		return true
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left.path, right.path)
	}
	return left.path == right.path
}

func errUnsupportedTraceEngine(mode string) error {
	return &traceEngineError{mode: mode}
}

type traceEngineError struct {
	mode string
}

func (e *traceEngineError) Error() string {
	return "unsupported trace engine mode " + strconv.Quote(e.mode) + "; use auto, trace_streamer, or builtin"
}

func newTraceProviderDecision(stage string, provider traceProviderSpec, opts Options, inputPath, outputPath string) TraceProviderDecision {
	mode := requestedTraceEngineMode(opts.TraceEngine)
	return TraceProviderDecision{
		Stage:        stage,
		ProviderKind: provider.Kind,
		ProviderName: provider.Name,
		InputPath:    inputPath,
		OutputPath:   outputPath,
		EngineMode:   mode,
		Fallback:     provider.Fallback && mode == traceEngineAuto,
	}
}

func traceProviderSuccess(decision TraceProviderDecision, artifact Artifact) TraceProviderDecision {
	decision.Selected = true
	decision.Attempted = true
	decision.Succeeded = true
	decision.ArtifactPath = artifact.Path
	decision.TraceQueryReady = artifact.Type == ArtifactSystrace || artifact.Type == ArtifactTraceBundle
	if artifact.Type == ArtifactTraceDB {
		decision.DBPath = artifact.Path
	}
	return decision
}

func traceProviderSkipped(decision TraceProviderDecision, selected bool, reason, caveat string) TraceProviderDecision {
	decision.Selected = selected
	decision.Reason = reason
	decision.Caveat = caveat
	return decision
}

func traceProviderFailure(decision TraceProviderDecision, reason, caveat string) TraceProviderDecision {
	decision.Selected = true
	decision.Attempted = true
	decision.Reason = reason
	decision.Caveat = caveat
	return decision
}
