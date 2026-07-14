package hitraceconv

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

var traceStreamerExecutablePath = os.Executable

type TraceToolStatus struct {
	RequestedEngine  string
	OrderedRoute     []string
	FirstLane        string
	PreflightEngine  string
	ExecutionBlocker string
	// EngineMode and SelectedEngine are compatibility aliases for older
	// consumers. They mean requested and preflight respectively; actual runtime
	// execution/fallback is reported only by Result.TraceDecisions.
	EngineMode           string
	SelectedEngine       string
	InputPath            string
	InputInspected       bool
	InputKind            string
	InputHasPerfSidecar  bool
	InputInspectionError string
	TraceStreamer        TraceToolProviderStatus
	BuiltinModern        TraceToolProviderStatus
	SysBinaryParity      TraceToolGateStatus
	Caveats              []string
}

type TraceToolGateStatus struct {
	Name                 string   `json:"name,omitempty"`
	State                string   `json:"state,omitempty"`
	Proven               bool     `json:"proven"`
	FixtureManifestCount int      `json:"fixture_manifest_count"`
	RequiredEvidence     string   `json:"required_evidence,omitempty"`
	Evidence             []string `json:"evidence,omitempty"`
	Caveats              []string `json:"caveats,omitempty"`
}

type TraceToolProviderStatus struct {
	Name            string
	Kind            string
	Available       bool
	Path            string
	Source          string
	Version         string
	CheckCommand    string
	AuxiliaryChecks []string
	InstallCommand  string
	DocsURL         string
	InstallHint     string
	Caveats         []string
}

const (
	traceToolGateNameSysBinaryParity                  = "no_perf_sys_binary_parity"
	traceToolGateStatePendingRepresentativeFixture    = "pending_representative_fixture"
	traceToolGateStateRepresentativeFixtureConfigured = "representative_fixture_manifest_present"

	traceToolGateSysParityRequiredEvidence  = "commit a redistributable real no-perf Harmony/Donghu .sys fixture manifest under internal/hitraceconv/testdata/representative_sys_traces and pass TestRepresentativeSysTraceFixtures"
	traceToolGateSysParitySyntheticEvidence = "synthetic scheduler/raw-ftrace parity guards are delivered"
	traceToolGateSysParityOpenCaveat        = "no redistributable representative no-perf .sys fixture has been committed; the built-in sys binary parser remains an explicit guarded lane"
	traceToolGateSysParityTracePerfCaveat   = "trace+perf htrace in auto mode may fall back to the built-in raw trace parser when SQL is unavailable or fails; explicit trace_streamer mode never falls back"
)

var traceSysParityManifestGlob = defaultTraceSysParityManifestGlob

func buildTraceProviderPlan(opts Options, inspectUnselected bool) (traceProviderPlan, error) {
	return buildTraceProviderPlanWithInput(opts, inspectUnselected, traceInputUsesDirectPerfRoute(opts))
}

func buildTraceProviderPlanWithInput(opts Options, inspectUnselected bool, directPerf bool) (traceProviderPlan, error) {
	if err := validateTraceEngineMode(opts.TraceEngine); err != nil {
		return traceProviderPlan{}, err
	}
	mode := requestedTraceEngineMode(opts.TraceEngine)
	plan := traceProviderPlan{
		RequestedEngine: mode,
		TraceStreamer: traceProviderLanePlan{
			Engine:               traceEngineTraceStreamer,
			Provider:             traceProviderByName(traceProviderNameTraceStreamer),
			ExternalInputProfile: externalToolInputSnapshotOnly,
		},
		Builtin: traceProviderLanePlan{
			Engine:    traceEngineBuiltin,
			Provider:  traceProviderByName(traceProviderNameBuiltinModern),
			Available: true,
			Source:    "built-in",
		},
	}
	if directPerf && mode != traceEngineTraceStreamer {
		if err := validateDirectPerfTraceOptions(opts); err != nil {
			plan.ExecutionBlocker = err.Error()
		}
		plan.DirectPerf = true
		plan.OrderedEngines = []string{traceEngineDirectPerf}
		plan.PreflightEngine = traceEngineDirectPerf
		return plan, nil
	}
	if mode != traceEngineBuiltin || inspectUnselected {
		plan.TraceStreamer = resolveTraceStreamerLanePlan(opts, plan.TraceStreamer)
	}
	switch mode {
	case traceEngineAuto:
		plan.OrderedEngines = []string{traceEngineTraceStreamer, traceEngineBuiltin}
		if plan.TraceStreamer.Available {
			plan.PreflightEngine = traceEngineTraceStreamer
			plan.TraceStreamer.Selected = true
		} else {
			plan.PreflightEngine = traceEngineBuiltin
			plan.Builtin.Selected = true
		}
	case traceEngineTraceStreamer:
		plan.OrderedEngines = []string{traceEngineTraceStreamer}
		plan.PreflightEngine = traceEngineTraceStreamer
		plan.TraceStreamer.Selected = true
	case traceEngineBuiltin:
		plan.OrderedEngines = []string{traceEngineBuiltin}
		plan.PreflightEngine = traceEngineBuiltin
		plan.Builtin.Selected = true
	}
	return plan, nil
}

func resolveTraceStreamerLanePlan(opts Options, lane traceProviderLanePlan) traceProviderLanePlan {
	resolution := resolveTraceStreamerToolResolution(opts)
	lane.Path = resolution.Path
	lane.Source = resolution.Source
	lane.ExternalInputProfile = resolution.ExternalInputProfile
	if strings.TrimSpace(lane.Source) == "" {
		lane.Source = "unresolved"
		for _, caveat := range resolution.Caveats {
			lower := strings.ToLower(caveat)
			switch {
			case strings.Contains(lower, "default embedded trace_streamer tier has no bundled payload"):
				lane.Source = "embedded_default_gap"
			case strings.Contains(lower, "embedded trace_streamer is not usable"):
				lane.Source = "embedded_integrity_failure"
			}
		}
	}
	lane.Caveats = append([]string(nil), resolution.Caveats...)
	probe := TraceToolProviderStatus{}
	lane.Available = traceToolPathUsable(lane.Path, &probe)
	lane.Caveats = append(lane.Caveats, probe.Caveats...)
	return lane
}

func defaultTraceSysParityManifestGlob() ([]string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("runtime caller unavailable")
	}
	pattern := filepath.Join(filepath.Dir(file), "testdata", "representative_sys_traces", "*.json")
	return filepath.Glob(pattern)
}

func BuildTraceToolStatus(opts Options) (TraceToolStatus, error) {
	plan, err := buildTraceProviderPlan(opts, true)
	if err != nil {
		return TraceToolStatus{}, err
	}
	status := TraceToolStatus{
		RequestedEngine:  plan.RequestedEngine,
		OrderedRoute:     append([]string(nil), plan.OrderedEngines...),
		FirstLane:        plan.OrderedEngines[0],
		PreflightEngine:  plan.PreflightEngine,
		ExecutionBlocker: plan.ExecutionBlocker,
		EngineMode:       plan.RequestedEngine,
		SelectedEngine:   plan.PreflightEngine,
		SysBinaryParity:  buildSysBinaryParityGateStatus(),
		TraceStreamer: TraceToolProviderStatus{
			Name:            traceProviderNameTraceStreamer,
			Kind:            traceProviderKindOfficialDB,
			CheckCommand:    "trace_streamer --help",
			AuxiliaryChecks: traceStreamerAuxiliaryChecks(opts),
			InstallCommand:  "Install OpenHarmony/SmartPerf trace_streamer, or place a platform-matched trace_streamer next to the Codrax binary.",
			DocsURL:         "https://gitcode.com/diting/hmtrace/tree/main",
			InstallHint:     "Pass --trace-streamer /path/to/trace_streamer, set CODRAX_TRACE_STREAMER, or place trace_streamer beside the Codrax binary, optionally under trace_streamer/<platform>/ or trace-streamer/<platform>/ for multi-platform bundles; verify it can run `trace_streamer --help` and export DBs with `trace_streamer <input> -e <output.db>`.",
			Caveats:         []string{"trace_streamer DB export is the preferred trace body path for trace+perf htrace and can also normalize trace-only captures to systrace with tracebundle coverage for trace_query; auto falls back to the built-in raw trace parser when SQL is unavailable or fails"},
		},
		BuiltinModern: TraceToolProviderStatus{
			Name:           traceProviderNameBuiltinModern,
			Kind:           traceProviderKindBuiltin,
			Available:      true,
			Source:         "built-in",
			CheckCommand:   "codrax trace convert --trace-engine=builtin",
			InstallCommand: "built-in",
			InstallHint:    "Built into Codrax; select it explicitly with --trace-engine=builtin for modern profiler/session text payloads and sys binary conversion, or let auto use it when trace_streamer is not discovered or SQL conversion fails.",
			Caveats:        []string{"built-in modern/sys parser is selected explicitly with --trace-engine=builtin or used by auto after trace_streamer is unavailable/fails; explicit trace_streamer mode does not fall back"},
		},
	}
	status.TraceStreamer.Caveats = append(status.TraceStreamer.Caveats, plan.TraceStreamer.Caveats...)
	status.TraceStreamer.Path = plan.TraceStreamer.Path
	status.TraceStreamer.Source = plan.TraceStreamer.Source
	status.TraceStreamer.Available = plan.TraceStreamer.Available
	inspectTraceToolStatusInput(&status, opts)
	switch plan.OrderedEngines[0] {
	case traceEngineTraceStreamer:
		if !status.TraceStreamer.Available {
			if plan.RequestedEngine == traceEngineAuto {
				if status.InputHasPerfSidecar {
					status.Caveats = append(status.Caveats, "auto trace engine did not discover trace_streamer; inspected input contains a standalone perf sidecar, so conversion will use built-in raw trace parsing and standalone perf fallback")
				} else {
					status.Caveats = append(status.Caveats, "auto trace engine did not discover trace_streamer; conversion will use the built-in raw trace parser")
				}
			} else {
				status.Caveats = append(status.Caveats, "trace_streamer engine was selected but trace_streamer is not available")
			}
		} else {
			if plan.RequestedEngine == traceEngineAuto {
				status.Caveats = append(status.Caveats, "auto trace engine discovered trace_streamer; conversion will use SQL first and fall back to the built-in raw trace parser if SQL execution or normalization fails")
			} else {
				status.Caveats = append(status.Caveats, "trace_streamer is explicitly selected; conversion uses SQL only and SQL execution failure will not fall back to the built-in parser")
			}
		}
	case traceEngineBuiltin:
	case traceEngineDirectPerf:
		status.Caveats = append(status.Caveats, "trace provider route is not applicable because the inspected input is a typed standalone perf capture with no trace body")
		if plan.ExecutionBlocker != "" {
			status.Caveats = append(status.Caveats, "execution_blocked: "+plan.ExecutionBlocker)
		}
	}
	return status, nil
}

func buildSysBinaryParityGateStatus() TraceToolGateStatus {
	status := TraceToolGateStatus{
		Name:             traceToolGateNameSysBinaryParity,
		State:            traceToolGateStatePendingRepresentativeFixture,
		RequiredEvidence: traceToolGateSysParityRequiredEvidence,
		Evidence: []string{
			traceToolGateSysParitySyntheticEvidence,
		},
		Caveats: []string{
			traceToolGateSysParityOpenCaveat,
			traceToolGateSysParityTracePerfCaveat,
		},
	}
	manifests, err := traceSysParityManifestGlob()
	if err != nil {
		status.Caveats = append(status.Caveats, fmt.Sprintf("representative fixture manifest directory could not be inspected: %v", err))
		return status
	}
	status.FixtureManifestCount = len(manifests)
	status.Evidence = append(status.Evidence, fmt.Sprintf("representative_fixture_manifests=%d", status.FixtureManifestCount))
	if status.FixtureManifestCount > 0 {
		status.State = traceToolGateStateRepresentativeFixtureConfigured
		status.Caveats = append(status.Caveats, "representative fixture manifest is present; verify TestRepresentativeSysTraceFixtures before retiring the built-in sys binary parser")
	}
	return status
}

func inspectTraceToolStatusInput(status *TraceToolStatus, opts Options) {
	input := strings.TrimSpace(opts.InputPath)
	if input == "" {
		return
	}
	status.InputPath = input
	_, err := os.Stat(input)
	if err != nil {
		status.InputKind = "unreadable"
		status.InputInspectionError = err.Error()
		status.Caveats = append(status.Caveats, fmt.Sprintf("trace input could not be inspected: %v", err))
		return
	}
	if simpleperfDirectRequested(detectPerfInputFormat(input)) {
		status.InputInspected = true
		status.InputKind = "direct_perf"
		return
	}
	hasPerf, err := statusInputContainsStandalonePerfSidecar(context.Background(), input)
	status.InputInspected = true
	status.InputHasPerfSidecar = hasPerf
	if hasPerf {
		status.InputKind = "trace_perf"
	} else {
		status.InputKind = "trace_only_or_unknown"
	}
	if err != nil {
		status.InputKind = "inspection_error"
		status.InputInspectionError = err.Error()
		status.Caveats = append(status.Caveats, fmt.Sprintf("trace input inspection failed: %v", err))
	}
}

func resolveTraceStreamerTool(opts Options) (string, string) {
	resolution := resolveTraceStreamerToolResolution(opts)
	return resolution.Path, resolution.Source
}

func resolveTraceStreamerToolWithCaveats(opts Options) (string, string, []string) {
	resolution := resolveTraceStreamerToolResolution(opts)
	return resolution.Path, resolution.Source, append([]string(nil), resolution.Caveats...)
}

type traceStreamerToolResolution struct {
	Path                 string
	Source               string
	Caveats              []string
	ExternalInputProfile externalToolInputProfile
}

func traceStreamerSnapshotToolResolution(path, source string, caveats []string) traceStreamerToolResolution {
	return traceStreamerToolResolution{
		Path:                 path,
		Source:               source,
		Caveats:              append([]string(nil), caveats...),
		ExternalInputProfile: externalToolInputSnapshotOnly,
	}
}

func resolveTraceStreamerToolResolution(opts Options) traceStreamerToolResolution {
	if path := strings.TrimSpace(opts.TraceStreamerPath); path != "" {
		return traceStreamerSnapshotToolResolution(path, "configured trace_streamer", nil)
	}
	if path := strings.TrimSpace(os.Getenv("CODRAX_TRACE_STREAMER")); path != "" {
		return traceStreamerSnapshotToolResolution(path, "CODRAX_TRACE_STREAMER", nil)
	}
	for _, candidate := range traceStreamerCodraxBinaryDirCandidates() {
		if traceToolPathUsable(candidate, nil) {
			return traceStreamerSnapshotToolResolution(candidate, "codrax executable directory", nil)
		}
	}
	embeddedPath, embeddedSource, embeddedCaveats := resolveEmbeddedTraceStreamerTool()
	if strings.TrimSpace(embeddedPath) != "" {
		return traceStreamerSnapshotToolResolution(embeddedPath, embeddedSource, embeddedCaveats)
	}
	persistentEmbeddedCaveats := embeddedIntegrityCaveats(embeddedCaveats)
	if path, err := exec.LookPath(traceStreamerBinaryName()); err == nil && strings.TrimSpace(path) != "" {
		return traceStreamerSnapshotToolResolution(path, traceStreamerBinaryName()+" on PATH", persistentEmbeddedCaveats)
	}
	for _, candidate := range traceStreamerKnownLocationCandidates() {
		if path := firstUsableTraceStreamerCandidate(candidate); path != "" {
			return traceStreamerSnapshotToolResolution(path, "known OpenHarmony/SmartPerf/hmtrace location", persistentEmbeddedCaveats)
		}
	}
	if len(embeddedCaveats) > 0 {
		source := "embedded_integrity_failure"
		for _, caveat := range embeddedCaveats {
			if strings.Contains(strings.ToLower(caveat), "default embedded trace_streamer tier has no bundled payload") {
				source = "embedded_default_gap"
				break
			}
		}
		return traceStreamerSnapshotToolResolution("", source, embeddedCaveats)
	}
	return traceStreamerSnapshotToolResolution("", "unresolved", nil)
}

func embeddedIntegrityCaveats(caveats []string) []string {
	var out []string
	for _, caveat := range caveats {
		if strings.Contains(strings.ToLower(caveat), "embedded trace_streamer is not usable") {
			out = append(out, caveat)
		}
	}
	return dedupeStrings(out)
}

func traceStreamerCodraxBinaryDirCandidates() []string {
	exe, err := traceStreamerExecutablePath()
	if err != nil {
		return nil
	}
	exe = strings.TrimSpace(exe)
	if exe == "" {
		return nil
	}
	return traceStreamerCodraxBinaryDirCandidatesFor(filepath.Dir(exe), runtime.GOOS, runtime.GOARCH)
}

func traceStreamerCodraxBinaryDirCandidatesFor(dir, goos, goarch string) []string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil
	}
	name := traceStreamerBinaryNameFor(goos)
	out := []string{filepath.Join(dir, name)}
	for _, platformDir := range traceStreamerHostPlatformDirsFor(goos, goarch) {
		out = append(out,
			filepath.Join(dir, platformDir, name),
			filepath.Join(dir, "trace_streamer", platformDir, name),
			filepath.Join(dir, "trace-streamer", platformDir, name),
		)
	}
	return uniqueNonEmptyStrings(out)
}

func traceStreamerHostPlatformDirs() []string {
	return traceStreamerHostPlatformDirsFor(runtime.GOOS, runtime.GOARCH)
}

func traceStreamerHostPlatformDirsFor(goos, goarch string) []string {
	goos = strings.ToLower(strings.TrimSpace(goos))
	goarch = strings.ToLower(strings.TrimSpace(goarch))
	var dirs []string
	add := func(dir string) {
		dir = strings.TrimSpace(dir)
		if dir != "" {
			dirs = append(dirs, dir)
		}
	}
	switch goarch {
	case "amd64":
		add(goos + "-x86_64")
		add(goos + "-amd64")
	case "arm64":
		add(goos + "-aarch64")
		add(goos + "-arm64")
	default:
		add(goos + "-" + goarch)
	}
	return uniqueNonEmptyStrings(dirs)
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func traceStreamerAuxiliaryChecks(opts Options) []string {
	var checks []string
	for _, dir := range opts.TraceStreamerSoDirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		checks = append(checks, fmt.Sprintf("so_dir=%s check=test -d %s", dir, dir))
	}
	if db := strings.TrimSpace(opts.TraceDBOutputPath); db != "" {
		checks = append(checks, fmt.Sprintf("db_output=%s check=parent_writable", db))
	}
	if len(checks) == 0 {
		checks = append(checks, "so_dirs=not_configured; pass --trace-streamer-so-dir /path/to/so when native symbol reload is needed")
	}
	return checks
}

func traceToolPathUsable(path string, status *TraceToolProviderStatus) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil {
		if status != nil {
			status.Caveats = append(status.Caveats, fmt.Sprintf("configured path is not readable: %v", err))
		}
		return false
	}
	if info.IsDir() {
		if status != nil {
			status.Caveats = append(status.Caveats, "configured path is a directory")
		}
		return false
	}
	if runtime.GOOS != "windows" && info.Mode()&0o111 == 0 {
		if status != nil {
			status.Caveats = append(status.Caveats, "configured path is not executable")
		}
		return false
	}
	return true
}

func traceStreamerBinaryName() string {
	return traceStreamerBinaryNameFor(runtime.GOOS)
}

func traceStreamerBinaryNameFor(goos string) string {
	if strings.EqualFold(strings.TrimSpace(goos), "windows") {
		return "trace_streamer.exe"
	}
	return "trace_streamer"
}

func traceStreamerKnownLocationCandidates() []string {
	name := traceStreamerBinaryName()
	var out []string
	for _, env := range []string{"OHOS_SDK_HOME", "HARMONYOS_SDK_HOME", "DEVECO_SDK_HOME", "TRACE_STREAMER_HOME"} {
		root := strings.TrimSpace(os.Getenv(env))
		if root == "" {
			continue
		}
		out = append(out,
			filepath.Join(root, name),
			filepath.Join(root, "bin", name),
			filepath.Join(root, "toolchains", name),
			filepath.Join(root, "trace_streamer", name),
		)
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		switch runtime.GOOS {
		case "darwin":
			out = append(out, filepath.Join(home, "Library", "Caches", "hmtrace", "embedded-trace-streamer", "*", "*", name))
		case "linux":
			out = append(out, filepath.Join(home, ".cache", "hmtrace", "embedded-trace-streamer", "*", "*", name))
		}
	}
	switch runtime.GOOS {
	case "darwin":
		out = append(out,
			filepath.Join("/Applications", "DevEco-Studio.app", "Contents", "sdk", "toolchains", name),
			filepath.Join("/usr/local/bin", name),
			filepath.Join("/opt/homebrew/bin", name),
		)
	case "linux":
		out = append(out,
			filepath.Join("/usr/local/bin", name),
			filepath.Join("/usr/bin", name),
			filepath.Join("/opt/openharmony", "toolchains", name),
		)
	}
	return out
}

func firstUsableTraceStreamerCandidate(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return ""
	}
	candidates := []string{pattern}
	if strings.ContainsAny(pattern, "*?[") {
		if matches, err := filepath.Glob(pattern); err == nil && len(matches) > 0 {
			candidates = matches
		} else {
			candidates = nil
		}
	}
	for _, candidate := range candidates {
		if traceToolPathUsable(candidate, nil) {
			return candidate
		}
	}
	return ""
}
