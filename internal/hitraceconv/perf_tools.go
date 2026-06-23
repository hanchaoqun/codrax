package hitraceconv

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

type PerfToolStatus struct {
	ParserMode               string
	SelectedParser           string
	SymbolizationExpectation string
	Hiperf                   PerfToolProviderStatus
	Simpleperf               PerfToolProviderStatus
	RawFallback              PerfToolProviderStatus
	Caveats                  []string
}

type PerfToolProviderStatus struct {
	Name            string
	Kind            string
	Available       bool
	Path            string
	Source          string
	Version         string
	Python          string
	CheckCommand    string
	AuxiliaryChecks []string
	InstallCommand  string
	DocsURL         string
	InstallHint     string
	Caveats         []string
}

func BuildPerfToolStatus(opts Options) (PerfToolStatus, error) {
	if err := validatePerfParserMode(opts.PerfParser); err != nil {
		return PerfToolStatus{}, err
	}
	mode := normalizePerfParserMode(opts.PerfParser)
	if mode == "" {
		mode = "auto"
	}
	status := PerfToolStatus{
		ParserMode:               mode,
		SelectedParser:           mode,
		SymbolizationExpectation: perfSymbolizationExpectation(mode, opts.DisablePerfAdapter),
		Hiperf: PerfToolProviderStatus{
			Name:            "openharmony_hiperf",
			Kind:            "official_harmony",
			CheckCommand:    "hiperf_host --help || hiperf --help",
			AuxiliaryChecks: perfHiperfAuxiliaryChecks(opts),
			InstallCommand:  "git clone https://gitee.com/openharmony/developtools_hiperf",
			DocsURL:         "https://gitee.com/openharmony/developtools_hiperf",
			InstallHint:     "Install or build OpenHarmony developtools_hiperf host tool, then pass --hiperf-host, set CODRAX_HIPERF_HOST, or place hiperf_host/hiperf beside the Codrax binary, optionally under hiperf/<platform>/ or developtools_hiperf/<platform>/ for multi-platform bundles; add --hiperf-symbol-dir for symbols.",
		},
		Simpleperf: PerfToolProviderStatus{
			Name:            "android_simpleperf_report_sample",
			Kind:            "official_android",
			CheckCommand:    "python3 /path/to/simpleperf/scripts/report_sample.py --help",
			AuxiliaryChecks: perfSimpleperfAuxiliaryChecks(opts),
			InstallCommand:  "git clone https://android.googlesource.com/platform/system/extras",
			DocsURL:         "https://android.googlesource.com/platform/system/extras/+/refs/heads/main/simpleperf/",
			InstallHint:     "Use Android simpleperf scripts/report_sample.py, then pass --simpleperf-report-sample or set CODRAX_SIMPLEPERF_REPORT_SAMPLE; add --simpleperf-python, --simpleperf-symfs, and --simpleperf-kallsyms as needed.",
		},
		RawFallback: PerfToolProviderStatus{
			Name:           "codrax_raw_perfdata",
			Kind:           "raw_fallback",
			Source:         "built-in",
			Available:      !opts.DisablePerfAdapter && rawPerfParserAllowed(opts),
			CheckCommand:   "codrax trace convert --perf-tools-status --perf-parser=raw",
			InstallCommand: "built-in",
			DocsURL:        "docs/user_guide.md#perfdata--perf-sample",
			InstallHint:    "Built into Codrax; emits source=raw_perfdata_fallback. It can use saved HIPERF_FILES_SYMBOL names when present, otherwise it remains a time/thread/DSO/IP fallback.",
		},
	}
	if opts.DisablePerfAdapter {
		status.SelectedParser = "disabled"
		status.Caveats = append(status.Caveats, "perftrace generation is disabled; official adapters and raw fallback will not run")
		status.RawFallback.Available = false
		status.RawFallback.Caveats = append(status.RawFallback.Caveats, "disabled by --no-perftrace")
	}
	if mode == "official" {
		status.RawFallback.Available = false
		status.RawFallback.Caveats = append(status.RawFallback.Caveats, "disabled by --perf-parser=official")
	}

	if tool, source := resolveHiperfTool(opts); tool != "" {
		status.Hiperf.Path = tool
		status.Hiperf.Source = source
		status.Hiperf.Available = perfToolPathUsable(tool, source, &status.Hiperf)
	}
	if tool, python, source := resolveSimpleperfReportTool(opts); tool != "" {
		status.Simpleperf.Path = tool
		status.Simpleperf.Python = python
		status.Simpleperf.Source = source
		status.Simpleperf.Available = perfToolPathUsable(tool, source, &status.Simpleperf)
		if python == "" && strings.HasSuffix(strings.ToLower(strings.TrimSpace(tool)), ".py") {
			status.Simpleperf.Caveats = append(status.Simpleperf.Caveats, "python executable was not discovered for report_sample.py")
			status.Simpleperf.Available = false
		}
	} else if path := strings.TrimSpace(opts.SimpleperfReportPath); simpleperfPathLooksReportLibrary(path) {
		status.Simpleperf.Path = path
		status.Simpleperf.Source = "configured simpleperf_report_lib.py"
		status.Simpleperf.Caveats = append(status.Simpleperf.Caveats, "simpleperf_report_lib.py is the official library, but Codrax executes the report_sample.py wrapper; place report_sample.py next to simpleperf_report_lib.py or pass --simpleperf-report-sample /path/to/report_sample.py")
	}
	return status, nil
}

func perfHiperfAuxiliaryChecks(opts Options) []string {
	var checks []string
	for _, dir := range opts.HiperfSymbolDirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		checks = append(checks, fmt.Sprintf("symbol_root=%s check=test -d %s", dir, dir))
	}
	if len(checks) == 0 {
		checks = append(checks, "symbol_roots=not_configured; pass --hiperf-symbol-dir /path/to/symbols and verify with test -d /path/to/symbols")
	}
	return checks
}

func perfSimpleperfAuxiliaryChecks(opts Options) []string {
	var checks []string
	if dir := strings.TrimSpace(opts.SimpleperfSymfsDir); dir != "" {
		checks = append(checks, fmt.Sprintf("symfs=%s check=test -d %s", dir, dir))
	} else {
		checks = append(checks, "symfs=not_configured; pass --simpleperf-symfs /path/to/symfs and verify with test -d /path/to/symfs")
	}
	if path := strings.TrimSpace(opts.SimpleperfKallsymsPath); path != "" {
		checks = append(checks, fmt.Sprintf("kallsyms=%s check=test -r %s", path, path))
	} else {
		checks = append(checks, "kallsyms=not_configured; pass --simpleperf-kallsyms /path/to/kallsyms and verify with test -r /path/to/kallsyms")
	}
	return checks
}

func perfSymbolizationExpectation(mode string, disabled bool) string {
	if disabled {
		return "no .perftrace will be generated"
	}
	switch mode {
	case "official":
		return "official hiperf/simpleperf adapter required; output can be symbolized when matching symbols are supplied"
	case "raw", "fallback":
		return "Codrax raw fallback only; saved HIPERF_FILES_SYMBOL names are used when present, otherwise output is IP/DSO context"
	default:
		return "auto prefers official hiperf/simpleperf symbolized output, then falls back to raw perf.data; saved HIPERF_FILES_SYMBOL names are used when present"
	}
}

func perfToolPathUsable(path, source string, status *PerfToolProviderStatus) bool {
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
