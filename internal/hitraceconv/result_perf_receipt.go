package hitraceconv

import (
	"context"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
)

type resultOwnedPerfClaim struct {
	artifact Artifact
	profile  ownedTracePerfProfile
	coverage TraceDBCoverage
}

func ownedPerfResultPathKey(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("owned perf result path is empty")
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

func cloneTraceDBCoverage(item TraceDBCoverage) TraceDBCoverage {
	cloned := item
	cloned.FieldSources = cloneStringMap(item.FieldSources)
	cloned.ColumnsPresent = append([]string(nil), item.ColumnsPresent...)
	cloned.ColumnsMissing = append([]string(nil), item.ColumnsMissing...)
	if item.CaptureCompleteness != nil {
		completeness := *item.CaptureCompleteness
		completeness.Issues = append([]TraceCaptureCompletenessIssue(nil), item.CaptureCompleteness.Issues...)
		completeness.IntegrityIssues = append([]string(nil), item.CaptureCompleteness.IntegrityIssues...)
		cloned.CaptureCompleteness = &completeness
	}
	return cloned
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func ownedPerfProfileForCoverageTable(table string) (ownedTracePerfProfile, bool) {
	for _, profile := range []ownedTracePerfProfile{
		ownedTracePerfSimpleperfText,
		ownedTracePerfSimpleperfProto,
		ownedTracePerfHiperfProto,
		ownedTracePerfRaw,
	} {
		if table == "perftrace_"+string(profile) {
			return profile, true
		}
	}
	return "", false
}

// reconcileResultOwnedPerfReceipts is the sole Result-level projection of a
// validated perf receipt. It closes Artifact, successful Decision and coverage
// in both directions before any tracebundle observes the Result.
func reconcileResultOwnedPerfReceipts(result *Result, ledger *conversionFileLedger) error {
	if result == nil || ledger == nil {
		return newOwnedTracePublicationError("reconcile_result_receipt", "", fmt.Errorf("result receipt inputs are incomplete"))
	}
	claims := make(map[string]resultOwnedPerfClaim)
	claimOrder := make([]string, 0)
	for _, artifact := range result.Artifacts {
		if artifact.Type != ArtifactPerfTrace {
			continue
		}
		if artifact.Perf == nil {
			return newOwnedTracePublicationError("reconcile_result_receipt", artifact.Path, fmt.Errorf("perf artifact capability is absent"))
		}
		profile, ok := ownedTracePerfProfileForProvider(artifact.Perf.ProviderName)
		spec, closed := profile.claimSpec()
		if !ok || !closed || artifact.Perf.ProviderName != spec.providerName {
			return newOwnedTracePublicationError("reconcile_result_receipt", artifact.Path, fmt.Errorf("perf artifact provider profile is not closed"))
		}
		published, err := validateOwnedPerfTraceArtifactClaim(ledger, artifact, profile)
		if err != nil {
			return err
		}
		key, err := ownedPerfResultPathKey(artifact.Path)
		if err != nil {
			return newOwnedTracePublicationError("reconcile_result_receipt", artifact.Path, err)
		}
		coverage := cloneTraceDBCoverage(published.receipt.coverage)
		coverage.ArtifactPath = artifact.Path
		claim := resultOwnedPerfClaim{artifact: artifact, profile: profile, coverage: coverage}
		if existing, present := claims[key]; present {
			if existing.artifact.Path != artifact.Path || !reflect.DeepEqual(existing.artifact, artifact) ||
				!reflect.DeepEqual(existing.coverage, coverage) {
				return newOwnedTracePublicationError("reconcile_result_receipt", artifact.Path, fmt.Errorf("duplicate perf artifact disagrees with its public receipt"))
			}
			continue
		}
		claims[key] = claim
		claimOrder = append(claimOrder, key)
	}

	decisionCount := make(map[string]int, len(claims))
	for _, decision := range result.ProviderDecisions {
		profile, recognized := ownedTracePerfProfileForProvider(decision.ProviderName)
		if !recognized {
			if decision.Succeeded || decision.TraceQueryReady || strings.TrimSpace(decision.ArtifactPath) != "" {
				return newOwnedTracePublicationError("reconcile_result_receipt", decision.ArtifactPath, fmt.Errorf("successful perf decision has no closed provider profile"))
			}
			continue
		}
		spec, _ := profile.claimSpec()
		if decision.ProviderName != spec.providerName || decision.ProviderKind != spec.providerKind {
			return newOwnedTracePublicationError("reconcile_result_receipt", decision.ArtifactPath, fmt.Errorf("perf decision provider identity is not canonical"))
		}
		participates := decision.Succeeded || decision.TraceQueryReady || strings.TrimSpace(decision.ArtifactPath) != ""
		if !participates {
			if !ownedPerfNonSuccessDecisionRouteValid(decision, profile) {
				return newOwnedTracePublicationError("reconcile_result_receipt", decision.OutputPath, fmt.Errorf("perf non-success decision route fields are not canonical"))
			}
			if key, err := ownedPerfResultPathKey(decision.OutputPath); err == nil {
				if claim, present := claims[key]; present && claim.profile == profile {
					return newOwnedTracePublicationError("reconcile_result_receipt", decision.OutputPath, fmt.Errorf("validated perf artifact also has a contradictory non-success decision"))
				}
			}
			continue
		}
		if !decision.Selected || !decision.Attempted || !decision.Succeeded || !decision.TraceQueryReady ||
			strings.TrimSpace(decision.ArtifactPath) == "" || !ownedPerfSuccessDecisionRouteValid(decision, profile) {
			return newOwnedTracePublicationError("reconcile_result_receipt", decision.ArtifactPath, fmt.Errorf("perf success decision is internally inconsistent"))
		}
		key, err := ownedPerfResultPathKey(decision.ArtifactPath)
		if err != nil {
			return newOwnedTracePublicationError("reconcile_result_receipt", decision.ArtifactPath, err)
		}
		claim, present := claims[key]
		if !present || claim.profile != profile || decision.ArtifactPath != claim.artifact.Path ||
			decision.OutputPath != claim.artifact.Path || decision.InputFormat != claim.artifact.Perf.InputFormat {
			return newOwnedTracePublicationError("reconcile_result_receipt", decision.ArtifactPath, fmt.Errorf("perf success decision has no matching validated artifact"))
		}
		decisionCount[key]++
		if decisionCount[key] != 1 {
			return newOwnedTracePublicationError("reconcile_result_receipt", decision.ArtifactPath, fmt.Errorf("validated perf artifact has multiple success decisions"))
		}
	}
	for key, claim := range claims {
		if decisionCount[key] != 1 {
			return newOwnedTracePublicationError("reconcile_result_receipt", claim.artifact.Path, fmt.Errorf("validated perf artifact has no unique success decision"))
		}
	}

	coverageCount := make(map[string]int, len(claims))
	for _, coverage := range result.TraceDBCoverage {
		if _, reserved := ownedPerfProfileForCoverageTable(coverage.Table); reserved ||
			strings.HasPrefix(strings.TrimSpace(coverage.Table), "perftrace_") {
			return newOwnedTracePublicationError("reconcile_result_receipt", coverage.ArtifactPath, fmt.Errorf("perf receipt coverage is forbidden in the trace DB coverage lane"))
		}
	}
	for _, coverage := range result.TraceCoverage {
		profile, reserved := ownedPerfProfileForCoverageTable(coverage.Table)
		if !reserved {
			if strings.HasPrefix(strings.TrimSpace(coverage.Table), "perftrace_") {
				return newOwnedTracePublicationError("reconcile_result_receipt", coverage.ArtifactPath, fmt.Errorf("unknown perf receipt coverage profile"))
			}
			continue
		}
		key, err := ownedPerfResultPathKey(coverage.ArtifactPath)
		if err != nil {
			return newOwnedTracePublicationError("reconcile_result_receipt", coverage.ArtifactPath, err)
		}
		claim, present := claims[key]
		if !present || claim.profile != profile || coverage.ArtifactPath != claim.artifact.Path ||
			!reflect.DeepEqual(coverage, claim.coverage) {
			return newOwnedTracePublicationError("reconcile_result_receipt", coverage.ArtifactPath, fmt.Errorf("perf receipt coverage does not match a validated artifact"))
		}
		coverageCount[key]++
		if coverageCount[key] != 1 {
			return newOwnedTracePublicationError("reconcile_result_receipt", coverage.ArtifactPath, fmt.Errorf("validated perf artifact has duplicate receipt coverage"))
		}
	}
	for _, key := range claimOrder {
		if coverageCount[key] == 0 {
			result.TraceCoverage = append(result.TraceCoverage, claims[key].coverage)
			coverageCount[key] = 1
		}
	}
	return nil
}

// auditTraceBundleOwnedPerfReceipts applies the Result receipt contract to the
// exact, pre-dedupe bundle inputs. Unlike Result reconciliation, a bundle is
// not allowed to synthesize missing receipt coverage: every semantic claim
// must already be present before the manifest is constructed.
func auditTraceBundleOwnedPerfReceipts(
	artifacts []Artifact,
	decisions []PerfProviderDecision,
	dbCoverage []TraceDBCoverage,
	traceCoverage []TraceDBCoverage,
	ledger *conversionFileLedger,
) error {
	audit := Result{
		Artifacts:         append([]Artifact(nil), artifacts...),
		ProviderDecisions: append([]PerfProviderDecision(nil), decisions...),
		TraceDBCoverage:   cloneTraceDBCoverageList(dbCoverage),
		TraceCoverage:     cloneTraceDBCoverageList(traceCoverage),
	}
	coverageCount := len(audit.TraceCoverage)
	if err := reconcileResultOwnedPerfReceipts(&audit, ledger); err != nil {
		return err
	}
	if len(audit.TraceCoverage) != coverageCount {
		return newOwnedTracePublicationError(
			"audit_bundle_receipt", "",
			fmt.Errorf("tracebundle is missing receipt coverage for a validated perf artifact"),
		)
	}
	return nil
}

func cloneTraceDBCoverageList(items []TraceDBCoverage) []TraceDBCoverage {
	if len(items) == 0 {
		return nil
	}
	cloned := make([]TraceDBCoverage, len(items))
	for index := range items {
		cloned[index] = cloneTraceDBCoverage(items[index])
	}
	return cloned
}

// rewriteTraceBundlePerfMetadata converts public output paths to the wire
// paths of their causal children on copies only. The caller's Result remains
// bound to its public paths while the bundle stays relocatable and internally
// self-consistent.
func rewriteTraceBundlePerfMetadata(
	publicArtifacts []Artifact,
	manifestArtifacts []Artifact,
	decisions []PerfProviderDecision,
	traceCoverage []TraceDBCoverage,
) ([]PerfProviderDecision, []TraceDBCoverage, error) {
	if len(publicArtifacts) != len(manifestArtifacts) {
		return nil, nil, newOwnedTracePublicationError(
			"rewrite_bundle_receipt_paths", "",
			fmt.Errorf("tracebundle artifact projection changed cardinality"),
		)
	}
	wireByPublicKey := make(map[string]string)
	for index, artifact := range publicArtifacts {
		if artifact.Type != ArtifactPerfTrace {
			continue
		}
		wirePath := manifestArtifacts[index].Path
		if strings.TrimSpace(artifact.Path) == "" || strings.TrimSpace(wirePath) == "" ||
			manifestArtifacts[index].Type != ArtifactPerfTrace {
			return nil, nil, newOwnedTracePublicationError(
				"rewrite_bundle_receipt_paths", artifact.Path,
				fmt.Errorf("perf artifact has no bundle-relative causal child"),
			)
		}
		publicKey, err := ownedPerfResultPathKey(artifact.Path)
		if err != nil {
			return nil, nil, newOwnedTracePublicationError("rewrite_bundle_receipt_paths", artifact.Path, err)
		}
		if prior, exists := wireByPublicKey[publicKey]; exists && prior != wirePath {
			return nil, nil, newOwnedTracePublicationError(
				"rewrite_bundle_receipt_paths", artifact.Path,
				fmt.Errorf("perf artifact public path maps to multiple bundle children"),
			)
		}
		wireByPublicKey[publicKey] = wirePath
	}

	lookupWirePath := func(publicPath string) (string, bool) {
		if strings.TrimSpace(publicPath) == "" {
			return "", false
		}
		key, err := ownedPerfResultPathKey(publicPath)
		if err != nil {
			return "", false
		}
		wirePath, ok := wireByPublicKey[key]
		return wirePath, ok
	}
	manifestDecisions := append([]PerfProviderDecision(nil), decisions...)
	for index := range manifestDecisions {
		if wirePath, ok := lookupWirePath(manifestDecisions[index].ArtifactPath); ok {
			manifestDecisions[index].ArtifactPath = wirePath
		}
		if wirePath, ok := lookupWirePath(manifestDecisions[index].OutputPath); ok {
			manifestDecisions[index].OutputPath = wirePath
		}
	}
	manifestCoverage := cloneTraceDBCoverageList(traceCoverage)
	for index := range manifestCoverage {
		if _, reserved := ownedPerfProfileForCoverageTable(manifestCoverage[index].Table); !reserved {
			continue
		}
		wirePath, ok := lookupWirePath(manifestCoverage[index].ArtifactPath)
		if !ok {
			return nil, nil, newOwnedTracePublicationError(
				"rewrite_bundle_receipt_paths", manifestCoverage[index].ArtifactPath,
				fmt.Errorf("perf receipt coverage has no bundle-relative causal child"),
			)
		}
		manifestCoverage[index].ArtifactPath = wirePath
	}
	return manifestDecisions, manifestCoverage, nil
}

func finalizeResultTraceBundleWithLedger(
	ctx context.Context,
	input string,
	bundleOutputPath string,
	result *Result,
	ledger *conversionFileLedger,
) error {
	if result == nil || ledger == nil {
		return newOwnedTracePublicationError("finalize_result_bundle", "", fmt.Errorf("result bundle inputs are incomplete"))
	}
	if strings.TrimSpace(result.BundlePath) != "" {
		return newOwnedTracePublicationError("finalize_result_bundle", result.BundlePath, fmt.Errorf("result already has a tracebundle"))
	}
	for _, artifact := range result.Artifacts {
		if artifact.Type == ArtifactTraceBundle || strings.TrimSpace(artifact.Path) != "" && artifact.Path == result.BundlePath {
			return newOwnedTracePublicationError("finalize_result_bundle", artifact.Path, fmt.Errorf("result already contains a tracebundle artifact"))
		}
	}
	if err := reconcileResultOwnedPerfReceipts(result, ledger); err != nil {
		return err
	}
	normalizeResultCollections(result)
	bundleArtifact, err := writeTraceBundleWithAllCoverageAndLedger(
		ctx, input, bundleOutputPath, result.Artifacts, result.Caveats,
		result.ProviderDecisions, result.TraceDecisions, result.TraceDBCoverage, result.TraceCoverage, ledger,
	)
	if err != nil {
		return err
	}
	if bundleArtifact.Path != "" {
		result.BundlePath = bundleArtifact.Path
		result.Artifacts = append(result.Artifacts, bundleArtifact)
	}
	normalizeResultCollections(result)
	return nil
}
