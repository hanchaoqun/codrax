package hitraceconv

import (
	"fmt"
	"os"
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
	Name        string
	Kind        string
	Available   bool
	Path        string
	Source      string
	Version     string
	Python      string
	InstallHint string
	Caveats     []string
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
			Name:        "openharmony_hiperf",
			Kind:        "official_harmony",
			InstallHint: "Install or build OpenHarmony developtools_hiperf host tool, then pass --hiperf-host or set CODRAX_HIPERF_HOST; add --hiperf-symbol-dir for symbols.",
		},
		Simpleperf: PerfToolProviderStatus{
			Name:        "android_simpleperf_report_sample",
			Kind:        "official_android",
			InstallHint: "Use Android simpleperf scripts/report_sample.py, then pass --simpleperf-report-sample or set CODRAX_SIMPLEPERF_REPORT_SAMPLE; add --simpleperf-python, --simpleperf-symfs, and --simpleperf-kallsyms as needed.",
		},
		RawFallback: PerfToolProviderStatus{
			Name:        "codrax_raw_perfdata",
			Kind:        "raw_fallback",
			Source:      "built-in",
			Available:   !opts.DisablePerfAdapter && rawPerfParserAllowed(opts),
			InstallHint: "Built into Codrax; emits source=raw_perfdata_fallback and symbolization_status=unsymbolized, so it is a fallback for time/thread/DSO/IP correlation rather than full symbolization.",
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
	}
	return status, nil
}

func perfSymbolizationExpectation(mode string, disabled bool) string {
	if disabled {
		return "no .perftrace will be generated"
	}
	switch mode {
	case "official":
		return "official hiperf/simpleperf adapter required; output can be symbolized when matching symbols are supplied"
	case "raw", "fallback":
		return "Codrax raw fallback only; output is IP/DSO context with symbolization_status=unsymbolized"
	default:
		return "auto prefers official hiperf/simpleperf symbolized output, then falls back to raw IP/DSO context when supported"
	}
}

func perfToolPathUsable(path, source string, status *PerfToolProviderStatus) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	if strings.Contains(source, "PATH") || strings.Contains(source, "on PATH") {
		return true
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
	return true
}
