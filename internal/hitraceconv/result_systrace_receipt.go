package hitraceconv

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

type resultOwnedSystraceClaim struct {
	artifact  Artifact
	kind      ownedTraceValidationKind
	coverage  TraceDBCoverage
	published publishedOwnedTraceValidation
}

type resultOwnedSystraceNonSuccessDecision struct {
	decision TraceProviderDecision
	index    int
}

func ownedSystraceResultPathKey(path string) (string, error) {
	if path == "" || path != strings.TrimSpace(path) {
		return "", fmt.Errorf("owned systrace result path is empty")
	}
	// Public paths are immutable receipt labels, not lookup authorities. In
	// particular, a relative label must not be re-resolved after CWD changes;
	// the private traceReceiptBindingPath carries the frozen absolute identity.
	return path, nil
}

func ownedSystraceKindForCoverageTable(table string) (ownedTraceValidationKind, bool) {
	switch table {
	case tracebundle.SystraceReceiptTableSQL:
		return ownedTraceValidationSQL, true
	case tracebundle.SystraceReceiptTableBuiltin:
		return ownedTraceValidationBuiltin, true
	case tracebundle.SystraceReceiptTableProfiler:
		return ownedTraceValidationProfiler, true
	default:
		return "", false
	}
}

func ownedSystracePathlessFailureReason(reason string) bool {
	switch reason {
	case traceDBPostvalidationCanceled,
		traceDBPostvalidationCountMismatch,
		traceDBPostvalidationGenerationInvalid,
		traceDBPostvalidationHeaderInvalid,
		traceDBPostvalidationParsePanic,
		traceDBPostvalidationScanFailed,
		traceDBPostvalidationClockRegression,
		traceDBPostvalidationUnknownOwnedRow,
		traceDBPostvalidationUnparsedOwnedRow,
		traceDBPostvalidationEventTypeMismatch,
		traceDBPostvalidationEventInvalid,
		traceDBPostvalidationWireMismatch,
		traceDBPostvalidationZeroRows:
		return true
	default:
		return false
	}
}

func ownedSystraceArtifactKind(artifact Artifact) (ownedTraceValidationKind, error) {
	if artifact.Trace == nil {
		return "", fmt.Errorf("systrace artifact capability is absent")
	}
	kind, ok := ownedTraceSystraceKindForProvider(artifact.Trace.ProviderName)
	spec, closed := kind.systraceClaimSpec()
	if !ok || !closed || artifact.Trace.ProviderName != spec.providerName ||
		artifact.Trace.ProviderKind != spec.providerKind ||
		artifact.Trace.ValidationProfile != string(kind) {
		return "", fmt.Errorf("systrace artifact provider profile is not closed")
	}
	return kind, nil
}

func ownedSystraceSuccessDecisionValid(
	decision TraceProviderDecision,
	claim resultOwnedSystraceClaim,
) bool {
	spec, closed := claim.kind.systraceClaimSpec()
	if !closed || decision.Stage != traceProviderStageTraceBody ||
		decision.ProviderKind != spec.providerKind || decision.ProviderName != spec.providerName ||
		decision.InputPath == "" || decision.InputPath != strings.TrimSpace(decision.InputPath) ||
		decision.OutputPath != claim.artifact.Path || decision.ArtifactPath != claim.artifact.Path ||
		!decision.Selected || !decision.Attempted || !decision.Succeeded ||
		decision.TraceQueryReady != claim.published.receipt.queryReady || decision.Reason != "" ||
		decision.EngineMode != requestedTraceEngineMode(decision.EngineMode) ||
		validateTraceEngineMode(decision.EngineMode) != nil {
		return false
	}
	switch claim.kind {
	case ownedTraceValidationSQL:
		return !decision.Fallback &&
			(decision.EngineMode == traceEngineAuto || decision.EngineMode == traceEngineTraceStreamer) &&
			(decision.DBPath == "" || decision.DBPath == strings.TrimSpace(decision.DBPath))
	case ownedTraceValidationBuiltin, ownedTraceValidationProfiler:
		return decision.DBPath == "" && decision.Caveat == "" &&
			decision.Fallback == (decision.EngineMode == traceEngineAuto) &&
			(decision.EngineMode == traceEngineAuto || decision.EngineMode == traceEngineBuiltin)
	default:
		return false
	}
}

func ownedSystraceNonSuccessDecisionValid(decision TraceProviderDecision, kind ownedTraceValidationKind) bool {
	spec, closed := kind.systraceClaimSpec()
	if !closed || decision.Stage != traceProviderStageTraceBody ||
		decision.ProviderKind != spec.providerKind || decision.ProviderName != spec.providerName ||
		decision.InputPath == "" || decision.InputPath != strings.TrimSpace(decision.InputPath) ||
		decision.OutputPath == "" || decision.OutputPath != strings.TrimSpace(decision.OutputPath) ||
		decision.ArtifactPath != "" || decision.Succeeded || decision.TraceQueryReady ||
		decision.Reason == "" || decision.Reason != strings.TrimSpace(decision.Reason) ||
		decision.Selected != decision.Attempted ||
		decision.EngineMode != requestedTraceEngineMode(decision.EngineMode) ||
		validateTraceEngineMode(decision.EngineMode) != nil {
		return false
	}
	switch kind {
	case ownedTraceValidationSQL:
		return !decision.Fallback &&
			(decision.EngineMode == traceEngineAuto || decision.EngineMode == traceEngineTraceStreamer) &&
			(decision.DBPath == "" || decision.DBPath == strings.TrimSpace(decision.DBPath))
	case ownedTraceValidationBuiltin, ownedTraceValidationProfiler:
		return decision.DBPath == "" &&
			decision.Fallback == (decision.EngineMode == traceEngineAuto) &&
			(decision.EngineMode == traceEngineAuto || decision.EngineMode == traceEngineBuiltin)
	default:
		return false
	}
}

// reconcileResultOwnedSystraceReceipts is the sole Result-level projection of
// converter-owned systrace receipts. Inventory existence cannot mint query
// readiness: Artifact, success Decision, receipt coverage, primary Result
// counters and the transaction ledger must all name the same public generation.
func reconcileResultOwnedSystraceReceipts(result *Result, ledger *conversionFileLedger) error {
	if result == nil || ledger == nil {
		return newOwnedTracePublicationError("reconcile_systrace_result_receipt", "", fmt.Errorf("result receipt inputs are incomplete"))
	}
	claims := make(map[string]resultOwnedSystraceClaim)
	bindingClaims := make(map[string]string)
	claimOrder := make([]string, 0)
	for _, artifact := range result.Artifacts {
		if artifact.Type != ArtifactSystrace {
			continue
		}
		kind, err := ownedSystraceArtifactKind(artifact)
		if err != nil {
			return newOwnedTracePublicationError("reconcile_systrace_result_receipt", artifact.Path, err)
		}
		published, err := validateOwnedSystraceArtifactClaim(ledger, artifact, kind)
		if err != nil {
			return err
		}
		key, err := ownedSystraceResultPathKey(artifact.Path)
		if err != nil {
			return newOwnedTracePublicationError("reconcile_systrace_result_receipt", artifact.Path, err)
		}
		coverage := cloneTraceDBCoverage(published.receipt.coverage)
		coverage.ArtifactPath = artifact.Path
		claim := resultOwnedSystraceClaim{
			artifact: artifact, kind: kind, coverage: coverage, published: published,
		}
		bindingKey := artifact.traceReceiptBindingPath
		if bindingKey == "" || bindingKey != strings.TrimSpace(bindingKey) {
			return newOwnedTracePublicationError(
				"reconcile_systrace_result_receipt", artifact.Path,
				fmt.Errorf("systrace artifact has no frozen receipt binding"),
			)
		}
		if priorPath, present := bindingClaims[bindingKey]; present {
			return newOwnedTracePublicationError(
				"reconcile_systrace_result_receipt", artifact.Path,
				fmt.Errorf("duplicate systrace generation is already claimed as %s", priorPath),
			)
		}
		if existing, present := claims[key]; present {
			reason := "duplicate systrace artifact is forbidden before Result normalization"
			if existing.kind != kind || existing.artifact.Path != artifact.Path ||
				!reflect.DeepEqual(existing.artifact, artifact) ||
				!reflect.DeepEqual(existing.coverage, coverage) ||
				!existing.published.publishedIdentity.SameVersion(published.publishedIdentity) {
				reason = "duplicate systrace artifact disagrees with its public receipt"
			}
			return newOwnedTracePublicationError(
				"reconcile_systrace_result_receipt", artifact.Path, fmt.Errorf("%s", reason),
			)
		}
		claims[key] = claim
		bindingClaims[bindingKey] = artifact.Path
		claimOrder = append(claimOrder, key)
	}

	decisionCount := make(map[string]int, len(claims))
	successDecisionIndex := make(map[string]int, len(claims))
	nonSuccessDecisionCount := make(map[ownedTraceValidationKind]int)
	nonSuccessDecisions := make(map[ownedTraceValidationKind]resultOwnedSystraceNonSuccessDecision)
	attemptedFailureDecisionCount := make(map[ownedTraceValidationKind]int)
	for decisionIndex, decision := range result.TraceDecisions {
		kind, recognized := ownedTraceSystraceKindForProvider(decision.ProviderName)
		if !recognized {
			if decision.Succeeded || decision.TraceQueryReady || strings.TrimSpace(decision.ArtifactPath) != "" {
				return newOwnedTracePublicationError(
					"reconcile_systrace_result_receipt", decision.ArtifactPath,
					fmt.Errorf("successful systrace decision has no closed provider profile"),
				)
			}
			continue
		}
		participates := decision.Succeeded || decision.TraceQueryReady || strings.TrimSpace(decision.ArtifactPath) != ""
		if !participates {
			if !ownedSystraceNonSuccessDecisionValid(decision, kind) {
				return newOwnedTracePublicationError(
					"reconcile_systrace_result_receipt", decision.OutputPath,
					fmt.Errorf("systrace non-success decision route fields are not canonical"),
				)
			}
			if key, err := ownedSystraceResultPathKey(decision.OutputPath); err == nil {
				if claim, present := claims[key]; present && claim.kind == kind {
					return newOwnedTracePublicationError(
						"reconcile_systrace_result_receipt", decision.OutputPath,
						fmt.Errorf("validated systrace artifact also has a contradictory non-success decision"),
					)
				}
			}
			nonSuccessDecisionCount[kind]++
			if nonSuccessDecisionCount[kind] != 1 {
				return newOwnedTracePublicationError(
					"reconcile_systrace_result_receipt", decision.OutputPath,
					fmt.Errorf("systrace provider profile has duplicate non-success decisions"),
				)
			}
			nonSuccessDecisions[kind] = resultOwnedSystraceNonSuccessDecision{
				decision: decision,
				index:    decisionIndex,
			}
			if decision.Selected && decision.Attempted {
				attemptedFailureDecisionCount[kind]++
			}
			continue
		}
		key, err := ownedSystraceResultPathKey(decision.ArtifactPath)
		if err != nil {
			return newOwnedTracePublicationError("reconcile_systrace_result_receipt", decision.ArtifactPath, err)
		}
		claim, present := claims[key]
		if !present || claim.kind != kind || !ownedSystraceSuccessDecisionValid(decision, claim) {
			return newOwnedTracePublicationError(
				"reconcile_systrace_result_receipt", decision.ArtifactPath,
				fmt.Errorf("systrace success decision has no matching validated artifact"),
			)
		}
		decisionCount[key]++
		if decisionCount[key] != 1 {
			return newOwnedTracePublicationError(
				"reconcile_systrace_result_receipt", decision.ArtifactPath,
				fmt.Errorf("validated systrace artifact has multiple success decisions"),
			)
		}
		successDecisionIndex[key] = decisionIndex
	}
	for key, claim := range claims {
		if decisionCount[key] != 1 {
			return newOwnedTracePublicationError(
				"reconcile_systrace_result_receipt", claim.artifact.Path,
				fmt.Errorf("validated systrace artifact has no unique success decision"),
			)
		}
	}
	if len(claims) > 0 && len(nonSuccessDecisions) > 0 {
		primary, primaryPresent := claims[result.OutputPath]
		primarySuccessIndex, primarySuccessPresent := successDecisionIndex[result.OutputPath]
		for kind, predecessor := range nonSuccessDecisions {
			failure := predecessor.decision
			if !primaryPresent || kind != ownedTraceValidationSQL ||
				(primary.kind != ownedTraceValidationBuiltin && primary.kind != ownedTraceValidationProfiler) ||
				failure.EngineMode != traceEngineAuto || failure.OutputPath != result.OutputPath ||
				!primarySuccessPresent || predecessor.index >= primarySuccessIndex {
				return newOwnedTracePublicationError(
					"reconcile_systrace_result_receipt", failure.OutputPath,
					fmt.Errorf("failed systrace provider is not the exact auto fallback predecessor of the primary artifact"),
				)
			}
		}
	}

	for _, coverage := range result.TraceDBCoverage {
		_, reserved := ownedSystraceKindForCoverageTable(coverage.Table)
		if reserved {
			return newOwnedTracePublicationError(
				"reconcile_systrace_result_receipt", coverage.ArtifactPath,
				fmt.Errorf("systrace receipt coverage is forbidden in the trace DB coverage lane"),
			)
		}
	}
	coverageCount := make(map[string]int, len(claims))
	pathlessFailureCoverageCount := make(map[ownedTraceValidationKind]int)
	for _, coverage := range result.TraceCoverage {
		kind, reserved := ownedSystraceKindForCoverageTable(coverage.Table)
		if !reserved {
			if coverage.Family == tracebundle.SystraceReceiptFamily &&
				coverage.Role == tracebundle.SystraceReceiptRole &&
				!tracebundle.IsClosedPerfReceiptTable(coverage.Table) {
				return newOwnedTracePublicationError(
					"reconcile_systrace_result_receipt", coverage.ArtifactPath,
					fmt.Errorf("unknown systrace receipt coverage profile"),
				)
			}
			continue
		}
		if coverage.ArtifactPath == "" && strings.TrimSpace(coverage.Error) != "" {
			// A writer/postvalidation failure may disclose its exact closed
			// profile and reason without acquiring a public receipt path. It is
			// diagnostics only and cannot satisfy any claim count below.
			if kind != ownedTraceValidationSQL ||
				coverage.Family != tracebundle.SystraceReceiptFamily ||
				coverage.Role != tracebundle.SystraceReceiptRole ||
				coverage.Error != strings.TrimSpace(coverage.Error) ||
				!ownedSystracePathlessFailureReason(coverage.Error) {
				return newOwnedTracePublicationError(
					"reconcile_systrace_result_receipt", "",
					fmt.Errorf("pathless systrace failure coverage is not an exact SQL diagnostic"),
				)
			}
			pathlessFailureCoverageCount[kind]++
			if pathlessFailureCoverageCount[kind] != 1 {
				return newOwnedTracePublicationError(
					"reconcile_systrace_result_receipt", "",
					fmt.Errorf("systrace provider profile has duplicate pathless failure coverage"),
				)
			}
			continue
		}
		if !tracebundle.IsSystraceReceiptCoverage(
			coverage.Family, coverage.Table, coverage.Role, coverage.ArtifactPath,
		) {
			return newOwnedTracePublicationError(
				"reconcile_systrace_result_receipt", coverage.ArtifactPath,
				fmt.Errorf("systrace receipt coverage tuple is not canonical"),
			)
		}
		key, err := ownedSystraceResultPathKey(coverage.ArtifactPath)
		if err != nil {
			return newOwnedTracePublicationError("reconcile_systrace_result_receipt", coverage.ArtifactPath, err)
		}
		claim, present := claims[key]
		if !present || claim.kind != kind || coverage.ArtifactPath != claim.artifact.Path ||
			!reflect.DeepEqual(coverage, claim.coverage) {
			return newOwnedTracePublicationError(
				"reconcile_systrace_result_receipt", coverage.ArtifactPath,
				fmt.Errorf("systrace receipt coverage does not match a validated artifact"),
			)
		}
		coverageCount[key]++
		if coverageCount[key] != 1 {
			return newOwnedTracePublicationError(
				"reconcile_systrace_result_receipt", coverage.ArtifactPath,
				fmt.Errorf("validated systrace artifact has duplicate receipt coverage"),
			)
		}
	}
	for kind, count := range pathlessFailureCoverageCount {
		predecessor, present := nonSuccessDecisions[kind]
		if count != 1 || attemptedFailureDecisionCount[kind] != 1 || !present ||
			!predecessor.decision.Selected || !predecessor.decision.Attempted ||
			predecessor.decision.Reason != "trace_db_normalize_failed" {
			return newOwnedTracePublicationError(
				"reconcile_systrace_result_receipt", "",
				fmt.Errorf("pathless systrace failure coverage has no matching failed provider decision"),
			)
		}
	}
	for _, key := range claimOrder {
		if coverageCount[key] == 0 {
			result.TraceCoverage = append(result.TraceCoverage, claims[key].coverage)
			coverageCount[key] = 1
		}
	}

	if len(claims) == 0 {
		if strings.TrimSpace(result.OutputPath) != "" || result.OutputBytes != 0 || result.EventsWritten != 0 {
			return newOwnedTracePublicationError(
				"reconcile_systrace_result_receipt", result.OutputPath,
				fmt.Errorf("primary systrace Result fields have no validated artifact"),
			)
		}
		return nil
	}
	key, err := ownedSystraceResultPathKey(result.OutputPath)
	if err != nil {
		return newOwnedTracePublicationError("reconcile_systrace_result_receipt", result.OutputPath, err)
	}
	claim, present := claims[key]
	if !present || result.OutputPath != claim.artifact.Path ||
		result.OutputBytes != claim.artifact.Bytes || result.EventsWritten != claim.artifact.Trace.Rows {
		return newOwnedTracePublicationError(
			"reconcile_systrace_result_receipt", result.OutputPath,
			fmt.Errorf("primary systrace Result fields do not match a validated artifact"),
		)
	}
	return nil
}

// auditTraceBundleOwnedSystraceReceipts applies the Result contract to the
// exact pre-dedupe bundle inputs. Missing receipt coverage is an error here:
// the manifest must never synthesize semantic claims after its caller freezes
// the Result collections.
func auditTraceBundleOwnedSystraceReceipts(
	primarySystracePath string,
	artifacts []Artifact,
	traceDecisions []TraceProviderDecision,
	dbCoverage []TraceDBCoverage,
	traceCoverage []TraceDBCoverage,
	ledger *conversionFileLedger,
) error {
	seenArtifacts := make(map[string]struct{})
	primaryMatches := 0
	for _, artifact := range artifacts {
		if artifact.Type != ArtifactSystrace {
			continue
		}
		key, err := ownedSystraceResultPathKey(artifact.Path)
		if err != nil {
			return newOwnedTracePublicationError("audit_bundle_systrace_receipt", artifact.Path, err)
		}
		if _, duplicate := seenArtifacts[key]; duplicate {
			return newOwnedTracePublicationError(
				"audit_bundle_systrace_receipt", artifact.Path,
				fmt.Errorf("tracebundle declares a duplicate systrace artifact before deduplication"),
			)
		}
		seenArtifacts[key] = struct{}{}
		if artifact.Path == primarySystracePath {
			primaryMatches++
		}
	}
	if len(seenArtifacts) > 0 && primaryMatches != 1 {
		return newOwnedTracePublicationError(
			"audit_bundle_systrace_receipt", primarySystracePath,
			fmt.Errorf("tracebundle primary systrace path does not select exactly one raw artifact"),
		)
	}
	audit := Result{
		Artifacts:       append([]Artifact(nil), artifacts...),
		TraceDecisions:  append([]TraceProviderDecision(nil), traceDecisions...),
		TraceDBCoverage: cloneTraceDBCoverageList(dbCoverage),
		TraceCoverage:   cloneTraceDBCoverageList(traceCoverage),
	}
	for _, artifact := range artifacts {
		if artifact.Type != ArtifactSystrace {
			continue
		}
		if artifact.Path != primarySystracePath {
			continue
		}
		audit.OutputPath = artifact.Path
		audit.OutputBytes = artifact.Bytes
		if artifact.Trace != nil {
			audit.EventsWritten = artifact.Trace.Rows
		}
	}
	coverageCount := len(audit.TraceCoverage)
	if err := reconcileResultOwnedSystraceReceipts(&audit, ledger); err != nil {
		return err
	}
	if len(audit.TraceCoverage) != coverageCount {
		return newOwnedTracePublicationError(
			"audit_bundle_systrace_receipt", "",
			fmt.Errorf("tracebundle is missing receipt coverage for a validated systrace artifact"),
		)
	}
	return nil
}

// rewriteTraceBundleSystraceMetadata converts only receipt-bound public
// systrace paths on manifest copies. Exact public spelling is authoritative:
// relative artifacts retain a separate frozen absolute binding and must not be
// re-resolved against a caller's later working directory.
func rewriteTraceBundleSystraceMetadata(
	publicArtifacts []Artifact,
	manifestArtifacts []Artifact,
	traceDecisions []TraceProviderDecision,
	traceCoverage []TraceDBCoverage,
) ([]TraceProviderDecision, []TraceDBCoverage, error) {
	if len(publicArtifacts) != len(manifestArtifacts) {
		return nil, nil, newOwnedTracePublicationError(
			"rewrite_bundle_systrace_receipt_paths", "",
			fmt.Errorf("tracebundle artifact projection changed cardinality"),
		)
	}
	wireByPublicPath := make(map[string]string)
	for index, artifact := range publicArtifacts {
		if artifact.Type != ArtifactSystrace {
			continue
		}
		wirePath := manifestArtifacts[index].Path
		if artifact.Trace == nil || artifact.Path == "" || artifact.Path != strings.TrimSpace(artifact.Path) ||
			artifact.traceReceiptArtifactPath != artifact.Path ||
			wirePath == "" || wirePath != strings.TrimSpace(wirePath) ||
			manifestArtifacts[index].Type != ArtifactSystrace {
			return nil, nil, newOwnedTracePublicationError(
				"rewrite_bundle_systrace_receipt_paths", artifact.Path,
				fmt.Errorf("systrace artifact has no bundle-relative causal child"),
			)
		}
		if prior, exists := wireByPublicPath[artifact.Path]; exists && prior != wirePath {
			return nil, nil, newOwnedTracePublicationError(
				"rewrite_bundle_systrace_receipt_paths", artifact.Path,
				fmt.Errorf("systrace artifact public path maps to multiple bundle children"),
			)
		}
		wireByPublicPath[artifact.Path] = wirePath
	}

	manifestDecisions := append([]TraceProviderDecision(nil), traceDecisions...)
	for index := range manifestDecisions {
		if wirePath, ok := wireByPublicPath[manifestDecisions[index].ArtifactPath]; ok {
			manifestDecisions[index].ArtifactPath = wirePath
		}
		if wirePath, ok := wireByPublicPath[manifestDecisions[index].OutputPath]; ok {
			manifestDecisions[index].OutputPath = wirePath
		}
	}
	manifestCoverage := cloneTraceDBCoverageList(traceCoverage)
	for index := range manifestCoverage {
		if _, reserved := ownedSystraceKindForCoverageTable(manifestCoverage[index].Table); !reserved {
			continue
		}
		if manifestCoverage[index].ArtifactPath == "" && strings.TrimSpace(manifestCoverage[index].Error) != "" {
			continue
		}
		wirePath, ok := wireByPublicPath[manifestCoverage[index].ArtifactPath]
		if !ok {
			return nil, nil, newOwnedTracePublicationError(
				"rewrite_bundle_systrace_receipt_paths", manifestCoverage[index].ArtifactPath,
				fmt.Errorf("systrace receipt coverage has no bundle-relative causal child"),
			)
		}
		manifestCoverage[index].ArtifactPath = wirePath
	}
	return manifestDecisions, manifestCoverage, nil
}
