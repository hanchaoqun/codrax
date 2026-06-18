package hitraceconv

import "strings"

const (
	perfProviderStageDirectInput      = "direct_input"
	perfProviderStageStandaloneHiperf = "standalone_hiperf"

	perfProviderKindOfficialHarmony = "official_harmony"
	perfProviderKindOfficialAndroid = "official_android"
	perfProviderKindRawFallback     = "raw_fallback"
	perfProviderKindDisabled        = "disabled"

	perfProviderNameHiperfProto       = "openharmony_hiperf_report_proto"
	perfProviderNameSimpleperfText    = "android_simpleperf_report_sample"
	perfProviderNameSimpleperfProto   = "android_simpleperf_report_proto"
	perfProviderNameRawFallback       = "codrax_raw_perfdata"
	perfProviderNamePerftraceDisabled = "perftrace_generation_disabled"
)

type perfProviderSpec struct {
	Kind            string
	Name            string
	Fallback        bool
	Implemented     bool
	SupportedInputs []perfInputFormat
}

var perfProviderRegistry = []perfProviderSpec{
	{
		Kind:            perfProviderKindOfficialHarmony,
		Name:            perfProviderNameHiperfProto,
		Implemented:     true,
		SupportedInputs: []perfInputFormat{perfInputLinuxPerfData},
	},
	{
		Kind:            perfProviderKindOfficialAndroid,
		Name:            perfProviderNameSimpleperfText,
		Implemented:     true,
		SupportedInputs: []perfInputFormat{perfInputLinuxPerfData, perfInputSimpleperfReportProto},
	},
	{
		Kind:            perfProviderKindOfficialAndroid,
		Name:            perfProviderNameSimpleperfProto,
		Implemented:     false,
		SupportedInputs: []perfInputFormat{perfInputSimpleperfReportProto},
	},
	{
		Kind:            perfProviderKindRawFallback,
		Name:            perfProviderNameRawFallback,
		Fallback:        true,
		Implemented:     true,
		SupportedInputs: []perfInputFormat{perfInputLinuxPerfData},
	},
	{
		Kind:        perfProviderKindDisabled,
		Name:        perfProviderNamePerftraceDisabled,
		Implemented: true,
	},
}

func perfProviderByName(name string) perfProviderSpec {
	for _, provider := range perfProviderRegistry {
		if provider.Name == name {
			return provider
		}
	}
	return perfProviderSpec{Name: strings.TrimSpace(name)}
}

func perfProviderSupportsInput(provider perfProviderSpec, inputFormat perfInputFormat) bool {
	if len(provider.SupportedInputs) == 0 {
		return true
	}
	for _, supported := range provider.SupportedInputs {
		if supported == inputFormat {
			return true
		}
	}
	return false
}

func newPerfProviderDecision(stage string, provider perfProviderSpec, opts Options, inputPath string, inputFormat perfInputFormat, outputPath string) PerfProviderDecision {
	mode := normalizePerfParserMode(opts.PerfParser)
	if mode == "" {
		mode = "auto"
	}
	return PerfProviderDecision{
		Stage:        stage,
		ProviderKind: provider.Kind,
		ProviderName: provider.Name,
		InputPath:    inputPath,
		InputFormat:  string(inputFormat),
		OutputPath:   outputPath,
		ParserMode:   mode,
		Fallback:     provider.Fallback,
	}
}

func perfProviderSuccess(decision PerfProviderDecision, artifact Artifact) PerfProviderDecision {
	decision.Selected = true
	decision.Attempted = true
	decision.Succeeded = true
	decision.ArtifactPath = artifact.Path
	decision.TraceQueryReady = artifact.Type == ArtifactPerfTrace
	if artifact.Perf != nil {
		decision.TraceQueryReady = artifact.Perf.TraceQueryReady
	}
	return decision
}

func perfProviderSkipped(decision PerfProviderDecision, selected bool, reason, caveat string) PerfProviderDecision {
	decision.Selected = selected
	decision.Reason = reason
	decision.Caveat = caveat
	return decision
}

func perfProviderFailure(decision PerfProviderDecision, reason, caveat string) PerfProviderDecision {
	decision.Selected = true
	decision.Attempted = true
	decision.Reason = reason
	decision.Caveat = caveat
	return decision
}
