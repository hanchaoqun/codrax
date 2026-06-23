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
	traceToolGateSysParityTracePerfCaveat   = "trace+perf htrace never falls back to the built-in sys binary parser"
)

var traceSysParityManifestGlob = defaultTraceSysParityManifestGlob

func defaultTraceSysParityManifestGlob() ([]string, error) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return nil, fmt.Errorf("runtime caller unavailable")
	}
	pattern := filepath.Join(filepath.Dir(file), "testdata", "representative_sys_traces", "*.json")
	return filepath.Glob(pattern)
}

func BuildTraceToolStatus(opts Options) (TraceToolStatus, error) {
	if err := validateTraceEngineMode(opts.TraceEngine); err != nil {
		return TraceToolStatus{}, err
	}
	mode := requestedTraceEngineMode(opts.TraceEngine)
	selected := selectedTraceEngineMode(opts.TraceEngine)
	status := TraceToolStatus{
		EngineMode:      mode,
		SelectedEngine:  selected,
		SysBinaryParity: buildSysBinaryParityGateStatus(),
		TraceStreamer: TraceToolProviderStatus{
			Name:            traceProviderNameTraceStreamer,
			Kind:            traceProviderKindOfficialDB,
			CheckCommand:    "trace_streamer --help",
			AuxiliaryChecks: traceStreamerAuxiliaryChecks(opts),
			InstallCommand:  "Install OpenHarmony/SmartPerf trace_streamer, or place a platform-matched trace_streamer next to the Codrax binary.",
			DocsURL:         "https://gitcode.com/diting/hmtrace/tree/main",
			InstallHint:     "Pass --trace-streamer /path/to/trace_streamer, set CODRAX_TRACE_STREAMER, or place trace_streamer beside the Codrax binary, optionally under trace_streamer/<platform>/ or trace-streamer/<platform>/ for multi-platform bundles; verify it can run `trace_streamer --help` and export DBs with `trace_streamer <input> -e <output.db>`.",
			Caveats:         []string{"trace_streamer DB export is the required trace body path for trace+perf htrace and can also normalize trace-only captures to systrace with tracebundle coverage for trace_query"},
		},
		BuiltinModern: TraceToolProviderStatus{
			Name:           traceProviderNameBuiltinModern,
			Kind:           traceProviderKindBuiltin,
			Available:      true,
			Source:         "built-in",
			CheckCommand:   "codrax trace convert --trace-engine=builtin",
			InstallCommand: "built-in",
			InstallHint:    "Built into Codrax; select it explicitly with --trace-engine=builtin for trace-only modern profiler/session text payloads and sys binary conversion, or let auto use it for trace-only captures when trace_streamer is not discovered. It is not used for trace+perf htrace trace body conversion.",
			Caveats:        []string{"built-in modern/sys parser is an explicit trace-only engine selected with --trace-engine=builtin; auto may use it only for trace-only captures when trace_streamer is not discovered; it is not used for trace+perf htrace"},
		},
	}
	if tool, source, caveats := resolveTraceStreamerToolWithCaveats(opts); tool != "" || len(caveats) > 0 {
		status.TraceStreamer.Caveats = append(status.TraceStreamer.Caveats, caveats...)
		status.TraceStreamer.Path = tool
		status.TraceStreamer.Source = source
		status.TraceStreamer.Available = traceToolPathUsable(tool, &status.TraceStreamer)
	}
	inspectTraceToolStatusInput(&status, opts)
	switch selected {
	case traceEngineTraceStreamer:
		status.SelectedEngine = traceEngineTraceStreamer
		if !status.TraceStreamer.Available {
			if mode == traceEngineAuto {
				if status.InputHasPerfSidecar {
					status.Caveats = append(status.Caveats, "auto trace engine did not discover trace_streamer; inspected input contains a standalone perf sidecar, so trace+perf conversion remains trace_streamer/SQLite-only and will not use the built-in parser")
				} else {
					status.SelectedEngine = traceEngineBuiltin
					status.Caveats = append(status.Caveats, "auto trace engine did not discover trace_streamer; trace-only conversion will use the built-in parser, while trace+perf htrace still requires trace_streamer/SQLite")
				}
			} else {
				status.Caveats = append(status.Caveats, "trace_streamer engine was selected but trace_streamer is not available")
			}
		} else {
			if mode == traceEngineAuto {
				status.Caveats = append(status.Caveats, "auto trace engine discovered trace_streamer; trace-only conversion will use SQL, and SQL execution failure will not fall back to the built-in parser")
			} else {
				status.Caveats = append(status.Caveats, "trace_streamer is selected; trace-only conversion uses SQL, and SQL execution failure will not fall back to the built-in parser")
			}
		}
	case traceEngineBuiltin:
		status.SelectedEngine = traceEngineBuiltin
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
	info, err := os.Stat(input)
	if err != nil {
		status.InputKind = "unreadable"
		status.InputInspectionError = err.Error()
		status.Caveats = append(status.Caveats, fmt.Sprintf("trace input could not be inspected: %v", err))
		return
	}
	hasPerf, err := inputContainsStandalonePerfSidecar(context.Background(), input, info.Size())
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
	path, source, _ := resolveTraceStreamerToolWithCaveats(opts)
	return path, source
}

func resolveTraceStreamerToolWithCaveats(opts Options) (string, string, []string) {
	if path := strings.TrimSpace(opts.TraceStreamerPath); path != "" {
		return path, "configured trace_streamer", nil
	}
	if path := strings.TrimSpace(os.Getenv("CODRAX_TRACE_STREAMER")); path != "" {
		return path, "CODRAX_TRACE_STREAMER", nil
	}
	for _, candidate := range traceStreamerCodraxBinaryDirCandidates() {
		if traceToolPathUsable(candidate, nil) {
			return candidate, "codrax executable directory", nil
		}
	}
	if path, err := exec.LookPath(traceStreamerBinaryName()); err == nil && strings.TrimSpace(path) != "" {
		return path, traceStreamerBinaryName() + " on PATH", nil
	}
	for _, candidate := range traceStreamerKnownLocationCandidates() {
		if path := firstUsableTraceStreamerCandidate(candidate); path != "" {
			return path, "known OpenHarmony/SmartPerf/hmtrace location", nil
		}
	}
	return "", "", nil
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
