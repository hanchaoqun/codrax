package hitraceconv

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

type TraceToolStatus struct {
	EngineMode     string
	SelectedEngine string
	TraceStreamer  TraceToolProviderStatus
	BuiltinModern  TraceToolProviderStatus
	Caveats        []string
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

func BuildTraceToolStatus(opts Options) (TraceToolStatus, error) {
	if err := validateTraceEngineMode(opts.TraceEngine); err != nil {
		return TraceToolStatus{}, err
	}
	mode := requestedTraceEngineMode(opts.TraceEngine)
	selected := selectedTraceEngineMode(opts.TraceEngine)
	status := TraceToolStatus{
		EngineMode:     mode,
		SelectedEngine: selected,
		TraceStreamer: TraceToolProviderStatus{
			Name:            traceProviderNameTraceStreamer,
			Kind:            traceProviderKindOfficialDB,
			CheckCommand:    "trace_streamer --help",
			AuxiliaryChecks: traceStreamerAuxiliaryChecks(opts),
			InstallCommand:  "Install OpenHarmony/SmartPerf trace_streamer, or use an hmtrace-style embedded trace_streamer binary once Codrax embedding is enabled.",
			DocsURL:         "https://gitcode.com/diting/hmtrace/tree/main",
			InstallHint:     "Pass --trace-streamer /path/to/trace_streamer or set CODRAX_TRACE_STREAMER; verify it can run `trace_streamer --help` and export DBs with `trace_streamer <input> -e <output.db>`.",
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
	if tool, source := resolveTraceStreamerTool(opts); tool != "" {
		status.TraceStreamer.Path = tool
		status.TraceStreamer.Source = source
		status.TraceStreamer.Available = traceToolPathUsable(tool, &status.TraceStreamer)
	}
	switch selected {
	case traceEngineTraceStreamer:
		status.SelectedEngine = traceEngineTraceStreamer
		if !status.TraceStreamer.Available {
			if mode == traceEngineAuto {
				status.SelectedEngine = traceEngineBuiltin
				status.Caveats = append(status.Caveats, "auto trace engine did not discover trace_streamer; trace-only conversion will use the built-in parser, while trace+perf htrace still requires trace_streamer/SQLite")
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

func resolveTraceStreamerTool(opts Options) (string, string) {
	if path := strings.TrimSpace(opts.TraceStreamerPath); path != "" {
		return path, "configured trace_streamer"
	}
	if path := strings.TrimSpace(os.Getenv("CODRAX_TRACE_STREAMER")); path != "" {
		return path, "CODRAX_TRACE_STREAMER"
	}
	if path, err := exec.LookPath(traceStreamerBinaryName()); err == nil && strings.TrimSpace(path) != "" {
		return path, traceStreamerBinaryName() + " on PATH"
	}
	for _, candidate := range traceStreamerKnownLocationCandidates() {
		if path := firstUsableTraceStreamerCandidate(candidate); path != "" {
			return path, "known OpenHarmony/SmartPerf/hmtrace location"
		}
	}
	return "", ""
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
	if runtime.GOOS == "windows" {
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
