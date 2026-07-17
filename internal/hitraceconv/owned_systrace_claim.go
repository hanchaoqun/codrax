package hitraceconv

import (
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

const ownedSystraceOutputFormat = "systrace_text"

type ownedTraceSystraceClaimSpec struct {
	providerKind  string
	providerName  string
	converter     string
	coverageTable string
}

func (kind ownedTraceValidationKind) systraceClaimSpec() (ownedTraceSystraceClaimSpec, bool) {
	switch kind {
	case ownedTraceValidationSQL:
		return ownedTraceSystraceClaimSpec{
			providerKind:  traceProviderKindOfficialDB,
			providerName:  traceProviderNameTraceStreamer,
			converter:     traceStreamerConverter,
			coverageTable: tracebundle.SystraceReceiptTableSQL,
		}, true
	case ownedTraceValidationBuiltin:
		return ownedTraceSystraceClaimSpec{
			providerKind:  traceProviderKindBuiltinSys,
			providerName:  traceProviderNameBuiltinSys,
			converter:     converterVersion,
			coverageTable: tracebundle.SystraceReceiptTableBuiltin,
		}, true
	case ownedTraceValidationProfiler:
		return ownedTraceSystraceClaimSpec{
			providerKind:  traceProviderKindBuiltin,
			providerName:  traceProviderNameBuiltinModern,
			converter:     converterVersion + "+openharmony-profiler",
			coverageTable: tracebundle.SystraceReceiptTableProfiler,
		}, true
	default:
		return ownedTraceSystraceClaimSpec{}, false
	}
}

func ownedTraceSystraceKindForProvider(providerName string) (ownedTraceValidationKind, bool) {
	for _, kind := range []ownedTraceValidationKind{
		ownedTraceValidationSQL,
		ownedTraceValidationBuiltin,
		ownedTraceValidationProfiler,
	} {
		spec, _ := kind.systraceClaimSpec()
		if spec.providerName == providerName {
			return kind, true
		}
	}
	return "", false
}

// validatedOwnedSystraceClaim is the sole read-side authority for a
// converter-owned systrace capability. A type, path, or provider self-report
// can describe inventory, but only this exact public-generation receipt can
// grant trace_query readiness.
func validatedOwnedSystraceClaim(
	ledger *conversionFileLedger,
	bindingPath string,
	kind ownedTraceValidationKind,
) (publishedOwnedTraceValidation, error) {
	spec, closed := kind.systraceClaimSpec()
	if ledger == nil || strings.TrimSpace(bindingPath) == "" ||
		bindingPath != strings.TrimSpace(bindingPath) || !filepath.IsAbs(bindingPath) ||
		filepath.Clean(bindingPath) != bindingPath || !closed {
		return publishedOwnedTraceValidation{}, newOwnedTracePublicationError(
			"consume_public_receipt", bindingPath, fmt.Errorf("owned systrace claim inputs are incomplete or open"),
		)
	}
	published, ok := ledger.ownedTraceValidation(bindingPath)
	if !ok || published.receipt.kind != kind || published.receipt.size <= 0 ||
		!published.publishedIdentity.Initialized() ||
		!tracebundle.IsSystraceReceiptCoverage(
			published.receipt.coverage.Family,
			published.receipt.coverage.Table,
			published.receipt.coverage.Role,
			published.receipt.coverage.ArtifactPath,
		) || published.receipt.coverage.Table != spec.coverageTable ||
		published.receipt.coverage.ArtifactPath != bindingPath {
		return publishedOwnedTraceValidation{}, newOwnedTracePublicationError(
			"consume_public_receipt", bindingPath, fmt.Errorf("exact validated systrace generation is unavailable"),
		)
	}
	return published, nil
}

func ownedSystraceCapability(
	kind ownedTraceValidationKind,
	receipt ownedTraceValidationReceipt,
) (*TraceArtifactCapability, error) {
	spec, ok := kind.systraceClaimSpec()
	if !ok || receipt.kind != kind {
		return nil, fmt.Errorf("owned systrace validation profile is not closed")
	}
	capability := &TraceArtifactCapability{
		ProviderKind:       spec.providerKind,
		ProviderName:       spec.providerName,
		OutputFormat:       ownedSystraceOutputFormat,
		ValidationProfile:  string(kind),
		Rows:               receipt.rows,
		Known:              receipt.known,
		AuthoritativeKnown: receipt.authoritativeKnown,
		AdvisoryRows:       receipt.advisory,
		TraceQueryReady:    receipt.queryReady,
	}
	switch kind {
	case ownedTraceValidationBuiltin:
		capability.IntentionalUnknown = receipt.unknown
		capability.IntentionalHeaderOnly = receipt.unparsed
	case ownedTraceValidationProfiler:
		capability.IntentionalUnknown = receipt.unknown
	case ownedTraceValidationSQL:
		// SQL is the strict-known profile.
	}
	return capability, nil
}

// newValidatedSystraceArtifact is the only constructor which projects a
// converter-owned systrace receipt into typed analysis capability. Q2/Q3
// migrate the three writers to this constructor in adjacent commits.
func newValidatedSystraceArtifact(
	ledger *conversionFileLedger,
	bindingPath string,
	kind ownedTraceValidationKind,
	caveats []string,
) (Artifact, error) {
	published, err := validatedOwnedSystraceClaim(ledger, bindingPath, kind)
	if err != nil {
		return Artifact{}, err
	}
	artifactPath := published.artifactPath
	if artifactPath == "" || artifactPath != strings.TrimSpace(artifactPath) {
		return Artifact{}, newOwnedTracePublicationError(
			"consume_public_receipt", bindingPath, fmt.Errorf("owned systrace receipt has no frozen artifact path"),
		)
	}
	spec, _ := kind.systraceClaimSpec()
	capability, err := ownedSystraceCapability(kind, published.receipt)
	if err != nil {
		return Artifact{}, newOwnedTracePublicationError("consume_public_receipt", bindingPath, err)
	}
	return Artifact{
		Type:                     ArtifactSystrace,
		Path:                     artifactPath,
		Bytes:                    published.receipt.size,
		SHA256:                   hex.EncodeToString(published.receipt.wireSHA256[:]),
		Converter:                spec.converter,
		Trace:                    capability,
		Caveats:                  append([]string(nil), caveats...),
		traceReceiptBindingPath:  bindingPath,
		traceReceiptArtifactPath: artifactPath,
	}, nil
}

func validateOwnedSystraceArtifactClaim(
	ledger *conversionFileLedger,
	artifact Artifact,
	kind ownedTraceValidationKind,
) (publishedOwnedTraceValidation, error) {
	bindingPath := artifact.traceReceiptBindingPath
	published, err := validatedOwnedSystraceClaim(ledger, bindingPath, kind)
	if err != nil {
		return publishedOwnedTraceValidation{}, err
	}
	spec, ok := kind.systraceClaimSpec()
	expectedCapability, capabilityErr := ownedSystraceCapability(kind, published.receipt)
	wantSHA := hex.EncodeToString(published.receipt.wireSHA256[:])
	if !ok || capabilityErr != nil || artifact.Type != ArtifactSystrace ||
		strings.TrimSpace(artifact.Path) == "" || artifact.Path != strings.TrimSpace(artifact.Path) ||
		artifact.traceReceiptArtifactPath == "" || artifact.Path != artifact.traceReceiptArtifactPath ||
		artifact.Path != published.artifactPath ||
		artifact.Bytes != published.receipt.size || artifact.SHA256 != wantSHA ||
		artifact.Converter != spec.converter || artifact.Trace == nil ||
		*artifact.Trace != *expectedCapability || artifact.Perf != nil || artifact.Standalone != nil || artifact.DataType != 0 ||
		artifact.PluginName != "" || artifact.PluginVersion != "" || artifact.SourceOffset != 0 ||
		artifact.SourceBytes != 0 {
		return publishedOwnedTraceValidation{}, newOwnedTracePublicationError(
			"consume_artifact_receipt", artifact.Path,
			fmt.Errorf("systrace artifact does not match its validated public generation"),
		)
	}
	return published, nil
}

func ownedSystraceSuccessDecisionRouteValid(decision TraceProviderDecision, kind ownedTraceValidationKind) bool {
	if decision.Stage != traceProviderStageTraceBody || decision.InputPath == "" ||
		decision.InputPath != strings.TrimSpace(decision.InputPath) || decision.OutputPath == "" ||
		decision.OutputPath != strings.TrimSpace(decision.OutputPath) || decision.DBPath != "" ||
		decision.Selected != decision.Attempted || decision.Succeeded || decision.TraceQueryReady ||
		decision.ArtifactPath != "" || decision.Reason != "" || decision.Caveat != "" ||
		decision.EngineMode != requestedTraceEngineMode(decision.EngineMode) ||
		validateTraceEngineMode(decision.EngineMode) != nil {
		return false
	}
	switch kind {
	case ownedTraceValidationSQL:
		return !decision.Fallback &&
			(decision.EngineMode == traceEngineAuto || decision.EngineMode == traceEngineTraceStreamer)
	case ownedTraceValidationBuiltin, ownedTraceValidationProfiler:
		return decision.Fallback == (decision.EngineMode == traceEngineAuto) &&
			(decision.EngineMode == traceEngineAuto || decision.EngineMode == traceEngineBuiltin)
	default:
		return false
	}
}

// traceProviderPublished binds a success decision to the same receipt-backed
// Artifact. It intentionally returns an error so provider fallback cannot hide
// an internal generation, receipt, or semantic drift.
func traceProviderPublished(
	decision TraceProviderDecision,
	artifact Artifact,
	ledger *conversionFileLedger,
) (TraceProviderDecision, error) {
	kind, ok := ownedTraceSystraceKindForProvider(decision.ProviderName)
	spec, closed := kind.systraceClaimSpec()
	if !ok || !closed || decision.ProviderName != spec.providerName ||
		decision.ProviderKind != spec.providerKind || !ownedSystraceSuccessDecisionRouteValid(decision, kind) ||
		decision.OutputPath != artifact.Path {
		return decision, newOwnedTracePublicationError(
			"consume_provider_receipt", artifact.Path,
			fmt.Errorf("systrace provider route is not canonical for its closed profile"),
		)
	}
	published, err := validateOwnedSystraceArtifactClaim(ledger, artifact, kind)
	if err != nil {
		return decision, err
	}
	provider := traceProviderByName(decision.ProviderName)
	if provider.Name != spec.providerName || provider.Kind != spec.providerKind {
		return decision, newOwnedTracePublicationError(
			"consume_provider_receipt", artifact.Path,
			fmt.Errorf("systrace provider registry differs from its closed receipt profile"),
		)
	}
	decision.Selected = true
	decision.Attempted = true
	decision.Succeeded = true
	decision.ArtifactPath = artifact.Path
	decision.TraceQueryReady = published.receipt.queryReady
	return decision, nil
}
