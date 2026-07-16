package hitraceconv

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

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
		Implemented:     true,
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

func perfProviderSuccess(decision PerfProviderDecision, artifact Artifact, ledger *conversionFileLedger) (PerfProviderDecision, error) {
	profile, ok := ownedTracePerfProfileForProvider(decision.ProviderName)
	if !ok {
		return decision, newOwnedTracePublicationError(
			"consume_provider_receipt", artifact.Path, fmt.Errorf("provider %q has no closed perf profile", decision.ProviderName),
		)
	}
	published, err := validatedOwnedPerfTraceClaim(ledger, artifact.Path, profile)
	if err != nil {
		return decision, err
	}
	spec, _ := profile.claimSpec()
	inputFormat := perfInputFormat(strings.TrimSpace(decision.InputFormat))
	provider := perfProviderByName(decision.ProviderName)
	expectedCapability := ownedPerfCapabilityForProfile(profile, inputFormat, "receipt-validated provider")
	if expectedCapability != nil {
		expectedCapability.TraceQueryReady = published.receipt.queryReady
	}
	wantSHA := fmt.Sprintf("%x", published.receipt.wireSHA256)
	if artifact.Type != ArtifactPerfTrace || artifact.Bytes != published.receipt.size || artifact.SHA256 != wantSHA ||
		artifact.Converter != spec.converter || artifact.Perf == nil || !artifact.Perf.TraceQueryReady ||
		artifact.Perf.ProviderKind != spec.providerKind || artifact.Perf.ProviderName != spec.providerName ||
		decision.ProviderName != spec.providerName || decision.ProviderKind != spec.providerKind ||
		provider.Name != spec.providerName || provider.Kind != spec.providerKind ||
		strings.TrimSpace(decision.OutputPath) != artifact.Path ||
		!inputFormat.valid() || inputFormat == perfInputUnknown || !perfProviderSupportsInput(provider, inputFormat) ||
		!ownedPerfCapabilitySemanticsEqual(artifact.Perf, expectedCapability) {
		return decision, newOwnedTracePublicationError(
			"consume_provider_receipt", artifact.Path, fmt.Errorf("perf provider artifact does not match its validated public generation"),
		)
	}
	decision.Selected = true
	decision.Attempted = true
	decision.Succeeded = true
	decision.ArtifactPath = artifact.Path
	decision.TraceQueryReady = published.receipt.queryReady
	return decision, nil
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

type perfPrivatePathError struct {
	err     error
	message string
}

type privatePathIdentity struct {
	prefixes []string
}

func (err *perfPrivatePathError) Error() string {
	if err == nil {
		return "perf provider failed"
	}
	return err.message
}

func (err *perfPrivatePathError) Unwrap() error {
	if err == nil {
		return nil
	}
	return err.err
}

// redactPerfProviderPrivateOutputs prevents provider-owned random staging
// paths from reaching errors, decisions, bundles, or artifact caveats while
// preserving the wrapped typed error for errors.Is/errors.As callers.
func redactPerfProviderPrivateOutputs(artifact *Artifact, caveat *string, decisions *[]PerfProviderDecision, resultErr *error, privateIdentity privatePathIdentity) {
	if len(privateIdentity.prefixes) == 0 {
		return
	}
	redact := func(value string) string {
		for _, prefix := range privateIdentity.prefixes {
			value = replaceAllASCIIPathFold(value, prefix, "<private_perf_staging>")
		}
		return value
	}
	if resultErr != nil && *resultErr != nil {
		message := redact((*resultErr).Error())
		if message != (*resultErr).Error() {
			*resultErr = &perfPrivatePathError{err: *resultErr, message: message}
		}
	}
	if caveat != nil {
		*caveat = redact(*caveat)
	}
	publicInput := ""
	structuredLeak := false
	containsPrivatePath := func(value string) bool {
		return redact(value) != value
	}
	if decisions != nil {
		for index := range *decisions {
			decision := &(*decisions)[index]
			if publicInput == "" && !containsPrivatePath(decision.InputPath) {
				publicInput = decision.InputPath
			}
			structuredLeak = structuredLeak || containsPrivatePath(decision.InputPath) ||
				containsPrivatePath(decision.OutputPath) || containsPrivatePath(decision.ArtifactPath)
			decision.Caveat = redact(decision.Caveat)
		}
	}
	if artifact != nil {
		structuredLeak = structuredLeak || containsPrivatePath(artifact.Path)
		for index := range artifact.Caveats {
			artifact.Caveats[index] = redact(artifact.Caveats[index])
		}
		if artifact.Perf != nil {
			for index := range artifact.Perf.Caveats {
				artifact.Perf.Caveats[index] = redact(artifact.Perf.Caveats[index])
			}
		}
	}
	if structuredLeak {
		contractErr := conversionInputFailure(
			ConversionInputCodeInternalContract,
			conversionInputStageExternalTool,
			publicInput,
			fmt.Errorf("private perf staging path escaped a structured provider field"),
		)
		if resultErr != nil {
			*resultErr = traceDBJoinPreservingSingle(contractErr, *resultErr)
		}
		if artifact != nil {
			*artifact = Artifact{}
		}
		if caveat != nil {
			*caveat = ""
		}
		if decisions != nil {
			*decisions = nil
		}
		return
	}
}

// replaceAllASCIIPathFold is deliberately byte-oriented. Unix permits
// non-UTF-8 path bytes, so customer-derived paths must never enter regexp or
// Unicode normalization code that can panic or lose byte offsets. ASCII fold
// covers Windows drive/separator case and the random ASCII staging leaf; the
// captured realpath aliases cover platform path-prefix rewrites.
func replaceAllASCIIPathFold(value, old, replacement string) string {
	if old == "" || len(value) < len(old) {
		return value
	}
	equalFold := func(left, right byte) bool {
		if left >= 'A' && left <= 'Z' {
			left += 'a' - 'A'
		}
		if right >= 'A' && right <= 'Z' {
			right += 'a' - 'A'
		}
		return left == right
	}
	var out strings.Builder
	last := 0
	for index := 0; index+len(old) <= len(value); {
		match := true
		for offset := 0; offset < len(old); offset++ {
			if !equalFold(value[index+offset], old[offset]) {
				match = false
				break
			}
		}
		if !match {
			index++
			continue
		}
		out.WriteString(value[last:index])
		out.WriteString(replacement)
		index += len(old)
		last = index
	}
	if last == 0 {
		return value
	}
	out.WriteString(value[last:])
	return out.String()
}

func privatePathRedactionPrefixes(path string) []string {
	return capturePrivatePathIdentity(path).prefixes
}

func capturePrivatePathIdentity(path string) privatePathIdentity {
	path = strings.TrimSpace(path)
	if path == "" {
		return privatePathIdentity{}
	}
	cleaned := filepath.Clean(path)
	candidates := []string{path, cleaned, filepath.Base(cleaned)}
	if absolute, err := filepath.Abs(path); err == nil {
		candidates = append(candidates, absolute)
	}
	for _, candidate := range append([]string(nil), candidates...) {
		if resolved, err := filepath.EvalSymlinks(candidate); err == nil {
			candidates = append(candidates, resolved)
		}
	}
	prefixes := make([]string, 0, len(candidates)*4)
	for _, candidate := range uniqueNonEmptyStrings(candidates) {
		slashed := filepath.ToSlash(candidate)
		quoted := strconv.Quote(candidate)
		if len(quoted) >= 2 {
			quoted = quoted[1 : len(quoted)-1]
		}
		prefixes = append(prefixes, candidate, slashed, strings.ReplaceAll(slashed, "/", `\`), quoted)
	}
	return privatePathIdentity{prefixes: uniqueNonEmptyStrings(prefixes)}
}

func boundedPerfAdapterCommandOutput(output []byte, provider string) string {
	if strings.TrimSpace(string(output)) == "" {
		return ""
	}
	// External adapters may echo the private argv. The shared command buffer can
	// truncate that path before redaction sees it, so no arbitrary child bytes
	// are copied to customer-visible decisions or bundles.
	return ": [" + strings.TrimSpace(provider) + " child output suppressed]"
}
