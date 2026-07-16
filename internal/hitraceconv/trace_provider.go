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
	Engine               string
	Provider             traceProviderSpec
	Available            bool
	Selected             bool
	EmbeddedLinuxRuntime bool
	Path                 string
	Source               string
	ExternalInputProfile externalToolInputProfile
	Caveats              []string
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

// TraceProviderFailureError is the single-lane counterpart of
// TraceProviderFallbackError. Explicit trace_streamer mode must not discard
// the same provider provenance that auto mode publishes before its built-in
// fallback: Source/Stage/Code/Caveats remain machine-readable and Cause keeps
// errors.Is/errors.As behavior.
type TraceProviderFailureError struct {
	Decision     TraceProviderDecision
	Source       string
	Path         string
	Stage        string
	Code         string
	Caveats      []string
	Cause        error
	RolledBackDB string
}

func (e *TraceProviderFailureError) Error() string {
	if e == nil {
		return "trace provider failed"
	}
	provider := firstNonEmpty(strings.TrimSpace(e.Decision.ProviderName), "unknown")
	reason := firstNonEmpty(strings.TrimSpace(e.Decision.Reason), "unknown")
	message := fmt.Sprintf("trace provider failed: provider=%s source=%s path=%s reason=%s",
		strconv.Quote(provider), strconv.Quote(strings.TrimSpace(e.Source)), strconv.Quote(strings.TrimSpace(e.Path)), strconv.Quote(reason))
	if stage := strings.TrimSpace(e.Stage); stage != "" {
		message += " stage=" + strconv.Quote(stage)
	}
	if code := strings.TrimSpace(e.Code); code != "" {
		message += " code=" + strconv.Quote(code)
	}
	if caveat := strings.TrimSpace(e.Decision.Caveat); caveat != "" {
		message += " caveat=" + strconv.Quote(caveat)
	}
	if rolledBackDB := strings.TrimSpace(e.RolledBackDB); rolledBackDB != "" {
		message += " rolled_back_db=" + strconv.Quote(rolledBackDB)
	}
	caveats := dedupeStrings(e.Caveats)
	if decisionCaveat := strings.TrimSpace(e.Decision.Caveat); decisionCaveat != "" {
		filtered := caveats[:0]
		for _, caveat := range caveats {
			if caveat != decisionCaveat {
				filtered = append(filtered, caveat)
			}
		}
		caveats = filtered
	}
	if len(caveats) > 0 {
		message += " caveats=" + strconv.Quote(strings.Join(caveats, " | "))
	}
	if e.Cause != nil {
		message += "; cause: " + e.Cause.Error()
	}
	return boundedTraceProviderErrorText(message, 8192)
}

func (e *TraceProviderFailureError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
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

// ValidateOptions validates content-independent option syntax and the static
// built-in/trace_streamer option conflict. Input classification, direct-perf
// precedence, and filesystem alias checks belong to the immutable conversion
// transaction; this helper never opens or probes the source path.
func ValidateOptions(opts Options) error {
	if err := validateOptionEnums(opts); err != nil {
		return err
	}
	return validateBuiltinTraceOptions(opts)
}

func validateOptionEnums(opts Options) error {
	if err := validatePerfParserMode(opts.PerfParser); err != nil {
		return err
	}
	if err := validateTraceEngineMode(opts.TraceEngine); err != nil {
		return err
	}
	return nil
}

// validateOptionsForInput is the dynamic route/collision authority used only
// after ConvertFile has opened one immutable input generation and classified
// its fixed probe. It returns the typed route bit consumed by the provider
// plan; callers must never repeat path-based content detection.
func validateOptionsForInput(opts Options, authority *conversionInputAuthority, inputFormat perfInputFormat) (bool, error) {
	if err := authority.Validate(conversionInputStageRoute); err != nil {
		return false, err
	}
	directPerf := requestedTraceEngineMode(opts.TraceEngine) != traceEngineTraceStreamer && simpleperfDirectRequested(inputFormat)
	if directPerf {
		if err := validateDirectPerfTraceOptions(opts); err != nil {
			return false, err
		}
	}
	inputPath, err := authority.canonicalIdentity()
	if err != nil {
		return false, err
	}
	if err := validateTraceOutputPathCollisionsForInput(opts, authority.DisplayPath(), inputPath); err != nil {
		return false, err
	}
	if !directPerf {
		if err := validateBuiltinTraceOptions(opts); err != nil {
			return false, err
		}
	}
	if err := authority.Validate(conversionInputStageRoute); err != nil {
		return false, err
	}
	return directPerf, nil
}

func validateBuiltinTraceOptions(opts Options) error {
	if requestedTraceEngineMode(opts.TraceEngine) != traceEngineBuiltin {
		return nil
	}
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
	return fmt.Errorf("--trace-engine=builtin bypasses trace_streamer and cannot be combined with %s", strings.Join(conflicts, ", "))
}

type traceCanonicalPath struct {
	path string
	info os.FileInfo
}

func validateTraceOutputPathCollisionsForInput(opts Options, input string, inputPath traceCanonicalPath) error {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	output := strings.TrimSpace(opts.OutputPath)
	if output == "" {
		output = DefaultOutputPath(input)
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
