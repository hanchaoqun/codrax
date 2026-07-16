package hitraceconv

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// validatedOwnedPerfTraceClaim is the sole read-side authority for a
// converter-owned perftrace capability. A path, Artifact type, or provider
// self-report can describe an output, but none of them can make it
// trace_query-ready without the receipt bound to that exact public generation.
func validatedOwnedPerfTraceClaim(
	ledger *conversionFileLedger,
	path string,
	expectedProfile ownedTracePerfProfile,
) (publishedOwnedTraceValidation, error) {
	if ledger == nil || strings.TrimSpace(path) == "" {
		return publishedOwnedTraceValidation{}, newOwnedTracePublicationError(
			"consume_public_receipt", path, fmt.Errorf("owned perftrace claim inputs are incomplete"),
		)
	}
	if _, _, ok := expectedProfile.sourceClock(); !ok {
		return publishedOwnedTraceValidation{}, newOwnedTracePublicationError(
			"consume_public_receipt", path, fmt.Errorf("owned perftrace profile %q is not closed", expectedProfile),
		)
	}
	published, ok := ledger.ownedTraceValidation(path)
	if !ok || published.receipt.kind != ownedTraceValidationPerf ||
		published.receipt.perfProfile != expectedProfile || !published.receipt.queryReady ||
		published.receipt.size <= 0 || !published.publishedIdentity.Initialized() {
		return publishedOwnedTraceValidation{}, newOwnedTracePublicationError(
			"consume_public_receipt", path, fmt.Errorf("exact validated perftrace generation is unavailable"),
		)
	}
	return published, nil
}

type ownedTracePerfClaimSpec struct {
	providerKind string
	providerName string
	converter    string
}

func (profile ownedTracePerfProfile) claimSpec() (ownedTracePerfClaimSpec, bool) {
	switch profile {
	case ownedTracePerfSimpleperfText:
		return ownedTracePerfClaimSpec{
			providerKind: perfProviderKindOfficialAndroid,
			providerName: perfProviderNameSimpleperfText,
			converter:    simpleperfAdapterVersion,
		}, true
	case ownedTracePerfSimpleperfProto:
		return ownedTracePerfClaimSpec{
			providerKind: perfProviderKindOfficialAndroid,
			providerName: perfProviderNameSimpleperfProto,
			converter:    simpleperfProtoConverter,
		}, true
	case ownedTracePerfHiperfProto:
		return ownedTracePerfClaimSpec{
			providerKind: perfProviderKindOfficialHarmony,
			providerName: perfProviderNameHiperfProto,
			converter:    hiperfAdapterVersion,
		}, true
	case ownedTracePerfRaw:
		return ownedTracePerfClaimSpec{
			providerKind: perfProviderKindRawFallback,
			providerName: perfProviderNameRawFallback,
			converter:    rawPerfDataAdapterVersion,
		}, true
	default:
		return ownedTracePerfClaimSpec{}, false
	}
}

func ownedTracePerfProfileForProvider(providerName string) (ownedTracePerfProfile, bool) {
	providerName = strings.TrimSpace(providerName)
	for _, profile := range []ownedTracePerfProfile{
		ownedTracePerfSimpleperfText,
		ownedTracePerfSimpleperfProto,
		ownedTracePerfHiperfProto,
		ownedTracePerfRaw,
	} {
		spec, _ := profile.claimSpec()
		if spec.providerName == providerName {
			return profile, true
		}
	}
	return "", false
}

func ownedPerfCapabilityForProfile(
	profile ownedTracePerfProfile,
	inputFormat perfInputFormat,
	providerSource string,
) *PerfArtifactCapability {
	switch profile {
	case ownedTracePerfSimpleperfText:
		return perfCapabilityForSimpleperfReportSample(inputFormat, providerSource)
	case ownedTracePerfSimpleperfProto:
		return perfCapabilityForSimpleperfReportProto(providerSource)
	case ownedTracePerfHiperfProto:
		return perfCapabilityForHiperfProto(providerSource)
	case ownedTracePerfRaw:
		return perfCapabilityForRawFallback(inputFormat)
	default:
		return nil
	}
}

// ownedPerfCapabilitySemanticsEqual excludes only the dynamic provider-source
// caveat. Fixed caveats and every field that changes how trace analysis
// interprets, weights, symbolizes, or trusts a sample are part of the closed
// profile and must match mechanically.
func ownedPerfCapabilitySemanticsEqual(left, right *PerfArtifactCapability) bool {
	return left != nil && right != nil &&
		left.ProviderKind == right.ProviderKind &&
		left.ProviderName == right.ProviderName &&
		left.InputFormat == right.InputFormat &&
		left.OutputFormat == right.OutputFormat &&
		left.TimeDomain == right.TimeDomain &&
		left.TimeAlignment == right.TimeAlignment &&
		left.ThreadIdentity == right.ThreadIdentity &&
		left.CPUIdentity == right.CPUIdentity &&
		left.EventWeight == right.EventWeight &&
		left.Symbolization == right.Symbolization &&
		left.Callchain == right.Callchain &&
		left.DSOLabel == right.DSOLabel &&
		left.BuildID == right.BuildID &&
		left.OffCPU == right.OffCPU &&
		left.Confidence == right.Confidence &&
		left.TraceQueryReady == right.TraceQueryReady &&
		left.Degraded == right.Degraded &&
		ownedPerfCapabilityFixedCaveatsEqual(left.Caveats, right.Caveats)
}

func ownedPerfCapabilityFixedCaveatsEqual(left, right []string) bool {
	filter := func(items []string) []string {
		fixed := make([]string, 0, len(items))
		for _, item := range items {
			if strings.HasPrefix(strings.TrimSpace(item), "provider source:") {
				continue
			}
			fixed = append(fixed, item)
		}
		return fixed
	}
	leftFixed := filter(left)
	rightFixed := filter(right)
	if len(leftFixed) != len(rightFixed) {
		return false
	}
	for index := range leftFixed {
		if leftFixed[index] != rightFixed[index] {
			return false
		}
	}
	return true
}

// newValidatedPerfTraceArtifact is the only constructor for a query-ready
// owned perftrace Artifact. Capability detail remains provider-specific, while
// bytes, digest, profile and readiness all come from one ledger receipt.
func newValidatedPerfTraceArtifact(
	ledger *conversionFileLedger,
	path string,
	profile ownedTracePerfProfile,
	inputFormat perfInputFormat,
	providerSource string,
	caveats []string,
) (Artifact, error) {
	published, err := validatedOwnedPerfTraceClaim(ledger, path, profile)
	if err != nil {
		return Artifact{}, err
	}
	spec, ok := profile.claimSpec()
	if !ok {
		return Artifact{}, newOwnedTracePublicationError(
			"consume_public_receipt", path, fmt.Errorf("owned perftrace converter profile %q is unavailable", profile),
		)
	}
	provider := perfProviderByName(spec.providerName)
	if !inputFormat.valid() || inputFormat == perfInputUnknown ||
		provider.Name != spec.providerName || provider.Kind != spec.providerKind ||
		!perfProviderSupportsInput(provider, inputFormat) {
		return Artifact{}, newOwnedTracePublicationError(
			"consume_public_receipt", path, fmt.Errorf("owned perftrace input does not match its closed provider profile"),
		)
	}
	capability := ownedPerfCapabilityForProfile(profile, inputFormat, providerSource)
	if capability == nil || capability.TraceQueryReady {
		return Artifact{}, newOwnedTracePublicationError(
			"consume_public_receipt", path, fmt.Errorf("owned perftrace base capability is not fail-closed"),
		)
	}
	if capability.ProviderKind != spec.providerKind || capability.ProviderName != spec.providerName {
		return Artifact{}, newOwnedTracePublicationError(
			"consume_public_receipt", path, fmt.Errorf("owned perftrace capability does not match its closed profile"),
		)
	}
	capability.TraceQueryReady = published.receipt.queryReady
	return Artifact{
		Type:      ArtifactPerfTrace,
		Path:      path,
		Bytes:     published.receipt.size,
		SHA256:    hex.EncodeToString(published.receipt.wireSHA256[:]),
		Converter: spec.converter,
		Perf:      capability,
		Caveats:   append([]string(nil), caveats...),
	}, nil
}

func validateOwnedPerfTraceArtifactClaim(
	ledger *conversionFileLedger,
	artifact Artifact,
	profile ownedTracePerfProfile,
) (publishedOwnedTraceValidation, error) {
	published, err := validatedOwnedPerfTraceClaim(ledger, artifact.Path, profile)
	if err != nil {
		return publishedOwnedTraceValidation{}, err
	}
	spec, ok := profile.claimSpec()
	if !ok || artifact.Perf == nil {
		return publishedOwnedTraceValidation{}, newOwnedTracePublicationError(
			"consume_artifact_receipt", artifact.Path, fmt.Errorf("perf artifact has no closed capability profile"),
		)
	}
	inputFormat := perfInputFormat(artifact.Perf.InputFormat)
	provider := perfProviderByName(spec.providerName)
	expectedCapability := ownedPerfCapabilityForProfile(profile, inputFormat, "receipt-validated provider")
	if expectedCapability != nil {
		expectedCapability.TraceQueryReady = published.receipt.queryReady
	}
	wantSHA := hex.EncodeToString(published.receipt.wireSHA256[:])
	if artifact.Type != ArtifactPerfTrace || strings.TrimSpace(artifact.Path) == "" ||
		artifact.Bytes != published.receipt.size || artifact.SHA256 != wantSHA ||
		artifact.Converter != spec.converter || !artifact.Perf.TraceQueryReady ||
		!inputFormat.valid() || inputFormat == perfInputUnknown ||
		provider.Name != spec.providerName || provider.Kind != spec.providerKind ||
		!perfProviderSupportsInput(provider, inputFormat) ||
		!ownedPerfCapabilitySemanticsEqual(artifact.Perf, expectedCapability) {
		return publishedOwnedTraceValidation{}, newOwnedTracePublicationError(
			"consume_artifact_receipt", artifact.Path, fmt.Errorf("perf artifact does not match its validated public generation"),
		)
	}
	return published, nil
}
