package hitraceconv

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

func traceBundlePerfLedgerRecord(t *testing.T, ledger *conversionFileLedger, path string) *createdConversionFile {
	t.Helper()
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		t.Fatal(err)
	}
	index, ok := ledger.byPath[abs]
	if !ok || index < 0 || index >= len(ledger.created) {
		t.Fatalf("fixture ledger has no record for %s", path)
	}
	return &ledger.created[index]
}

func TestTraceBundlePerfReceiptBoundaryRejectsOneSidedOrForgedClaims(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Result, *createdConversionFile)
	}{
		{name: "missing_receipt", mutate: func(_ *Result, record *createdConversionFile) {
			record.traceValidation = nil
		}},
		{name: "receipt_profile_drift", mutate: func(_ *Result, record *createdConversionFile) {
			record.traceValidation.receipt.perfProfile = ownedTracePerfRaw
			record.traceValidation.receipt.perfSource, record.traceValidation.receipt.perfClock, _ = ownedTracePerfRaw.sourceClock()
		}},
		{name: "published_identity_drift", mutate: func(_ *Result, record *createdConversionFile) {
			record.traceValidation.publishedIdentity = filegeneration.Identity{}
		}},
		{name: "artifact_bytes_drift", mutate: func(result *Result, _ *createdConversionFile) {
			result.Artifacts[0].Bytes++
		}},
		{name: "artifact_sha_drift", mutate: func(result *Result, _ *createdConversionFile) {
			result.Artifacts[0].SHA256 = strings.Repeat("0", sha256.Size*2)
		}},
		{name: "artifact_provider_drift", mutate: func(result *Result, _ *createdConversionFile) {
			capability := *result.Artifacts[0].Perf
			result.Artifacts[0].Perf = &capability
			result.Artifacts[0].Perf.ProviderName = perfProviderNameRawFallback
		}},
		{name: "missing_decision", mutate: func(result *Result, _ *createdConversionFile) {
			result.ProviderDecisions = nil
		}},
		{name: "duplicate_decision", mutate: func(result *Result, _ *createdConversionFile) {
			result.ProviderDecisions = append(result.ProviderDecisions, result.ProviderDecisions[0])
		}},
		{name: "orphan_success_decision", mutate: func(result *Result, _ *createdConversionFile) {
			orphan := result.ProviderDecisions[0]
			orphan.ArtifactPath += ".orphan"
			orphan.OutputPath += ".orphan"
			result.ProviderDecisions = append(result.ProviderDecisions, orphan)
		}},
		{name: "missing_coverage", mutate: func(result *Result, _ *createdConversionFile) {
			result.TraceCoverage = nil
		}},
		{name: "duplicate_coverage", mutate: func(result *Result, _ *createdConversionFile) {
			result.TraceCoverage = append(result.TraceCoverage, cloneTraceDBCoverage(result.TraceCoverage[0]))
		}},
		{name: "forged_coverage", mutate: func(result *Result, _ *createdConversionFile) {
			result.TraceCoverage[0].RowsEmitted++
		}},
		{name: "orphan_receipt_coverage", mutate: func(result *Result, _ *createdConversionFile) {
			orphan := cloneTraceDBCoverage(result.TraceCoverage[0])
			orphan.ArtifactPath += ".orphan"
			result.TraceCoverage = append(result.TraceCoverage, orphan)
		}},
		{name: "db_lane_receipt", mutate: func(result *Result, _ *createdConversionFile) {
			result.TraceDBCoverage = append(result.TraceDBCoverage, result.TraceCoverage[0])
		}},
		{name: "db_lane_unknown_reserved_namespace", mutate: func(result *Result, _ *createdConversionFile) {
			result.TraceDBCoverage = append(result.TraceDBCoverage, TraceDBCoverage{Table: "perftrace_future"})
		}},
		{name: "inconsistent_duplicate_artifact", mutate: func(result *Result, _ *createdConversionFile) {
			forged := result.Artifacts[0]
			forged.Bytes++
			result.Artifacts = append(result.Artifacts, forged)
		}},
		{name: "joint_receipt_and_artifact_sha_forgery", mutate: func(result *Result, record *createdConversionFile) {
			forged := sha256.Sum256([]byte("not the held child"))
			record.traceValidation.receipt.wireSHA256 = forged
			result.Artifacts[0].SHA256 = hex.EncodeToString(forged[:])
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "capture.perf.data")
			if err := os.WriteFile(input, []byte("input"), 0o600); err != nil {
				t.Fatal(err)
			}
			ledger, err := newConversionFileLedger(input)
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.cleanup()
			artifact, decision := validatedResultPerfFixture(
				t, ledger, ownedTracePerfSimpleperfText, filepath.Join(dir, "capture.perftrace"),
			)
			result := Result{Artifacts: []Artifact{artifact}, ProviderDecisions: []PerfProviderDecision{decision}}
			if err := reconcileResultOwnedPerfReceipts(&result, ledger); err != nil {
				t.Fatal(err)
			}
			test.mutate(&result, traceBundlePerfLedgerRecord(t, ledger, artifact.Path))

			_, err = writeTraceBundleWithAllCoverageAndLedger(
				context.Background(), input, filepath.Join(dir, "capture"), result.Artifacts, nil,
				result.ProviderDecisions, nil, result.TraceDBCoverage, result.TraceCoverage, ledger,
			)
			if err == nil || !ownedTraceOutputHardFailure(err) {
				t.Fatalf("forged bundle claim escaped typed hard gate: %T %v", err, err)
			}
			if _, statErr := os.Lstat(filepath.Join(dir, "capture.tracebundle.json")); !os.IsNotExist(statErr) {
				t.Fatalf("failed perf receipt gate left a manifest publication: %v", statErr)
			}
		})
	}
}

func TestTraceBundlePerfReceiptRejectsSamePathSameSizeRewrite(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.perf.data")
	if err := os.WriteFile(input, []byte("input"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger, err := newConversionFileLedger(input)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.cleanup()
	artifact, decision := validatedResultPerfFixture(
		t, ledger, ownedTracePerfSimpleperfText, filepath.Join(dir, "capture.perftrace"),
	)
	result := Result{Artifacts: []Artifact{artifact}, ProviderDecisions: []PerfProviderDecision{decision}}
	if err := reconcileResultOwnedPerfReceipts(&result, ledger); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifact.Path, bytes.Repeat([]byte{'x'}, int(artifact.Bytes)), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = writeTraceBundleWithAllCoverageAndLedger(
		context.Background(), input, filepath.Join(dir, "capture"), result.Artifacts, nil,
		result.ProviderDecisions, nil, result.TraceDBCoverage, result.TraceCoverage, ledger,
	)
	if err == nil || !ownedTraceOutputHardFailure(err) {
		t.Fatalf("same-path same-size public rewrite escaped bundle generation gate: %T %v", err, err)
	}
	if _, statErr := os.Lstat(filepath.Join(dir, "capture.tracebundle.json")); !os.IsNotExist(statErr) {
		t.Fatalf("generation drift left a manifest publication: %v", statErr)
	}
}

func TestTraceBundleBuilderComparesReceiptSHAWithHeldChild(t *testing.T) {
	dir := t.TempDir()
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.cleanup()
	artifact, _ := validatedResultPerfFixture(
		t, ledger, ownedTracePerfSimpleperfText, filepath.Join(dir, "capture.perftrace"),
	)
	forged := sha256.Sum256([]byte("not the held child"))
	record := traceBundlePerfLedgerRecord(t, ledger, artifact.Path)
	record.traceValidation.receipt.wireSHA256 = forged
	artifact.SHA256 = hex.EncodeToString(forged[:])
	_, _, held, err := buildTraceBundleV2Artifacts(
		context.Background(), filepath.Join(dir, "capture.tracebundle.json"), []Artifact{artifact}, ledger,
	)
	_ = closeHeldSealedOwnedFiles(held)
	if err == nil || !ownedTraceOutputHardFailure(err) || !strings.Contains(err.Error(), "validated owned trace publication failed") {
		t.Fatalf("held child did not independently reject forged receipt digest: %T %v", err, err)
	}
}

func TestTraceBundlePerfReceiptGateOrderingPinned(t *testing.T) {
	writer := sourceGenerationFunctionBody(t, "standalone.go", "writeTraceBundleWithAllCoverageAndGatesAndLedgerOps")
	rawCopyAt := strings.Index(writer, "rawArtifacts := append([]Artifact(nil), artifacts...)")
	dedupeAt := strings.Index(writer, "artifacts = dedupeArtifacts(artifacts)")
	auditAt := strings.Index(writer, "auditTraceBundleOwnedPerfReceipts(rawArtifacts,")
	buildAt := strings.Index(writer, "buildTraceBundleV2Artifacts(")
	closeGuardAt := strings.Index(writer, "heldChildrenClosed := false")
	rewriteAt := strings.Index(writer, "rewriteTraceBundlePerfMetadata(")
	if rawCopyAt < 0 || dedupeAt <= rawCopyAt || auditAt <= dedupeAt || buildAt <= auditAt ||
		closeGuardAt <= buildAt || rewriteAt <= closeGuardAt {
		t.Fatalf("bundle receipt gate lost raw-input or held-resource ordering:\n%s", writer)
	}

	builder := sourceGenerationFunctionBody(t, "standalone.go", "buildTraceBundleV2Artifacts")
	claimAt := strings.Index(builder, "validateOwnedPerfTraceArtifactClaim(ledger, publicArtifact, profile)")
	holdAt := strings.Index(builder, "ledger.holdAndMeasureSealedOwnedPath(ctx, originalPath)")
	parityAt := strings.Index(builder, "perfClaim.receipt.size != measuredBytes")
	assignAt := strings.Index(builder, "out[i].Bytes = measuredBytes")
	if claimAt < 0 || holdAt <= claimAt || parityAt <= holdAt || assignAt <= parityAt {
		t.Fatalf("bundle builder no longer validates claim and held parity before projecting measurements:\n%s", builder)
	}
}

func TestTraceBundlePerfReceiptSupportsSameProfileMultipleChildren(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.perf.data")
	if err := os.WriteFile(input, []byte("input"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger, err := newConversionFileLedger(input)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.cleanup()
	first, firstDecision := validatedResultPerfFixture(
		t, ledger, ownedTracePerfSimpleperfText, filepath.Join(dir, "first.perftrace"),
	)
	second, secondDecision := validatedResultPerfFixture(
		t, ledger, ownedTracePerfSimpleperfText, filepath.Join(dir, "second.perftrace"),
	)
	result := Result{
		Artifacts:         []Artifact{first, second},
		ProviderDecisions: []PerfProviderDecision{firstDecision, secondDecision},
	}
	if err := reconcileResultOwnedPerfReceipts(&result, ledger); err != nil {
		t.Fatal(err)
	}
	bundle, err := writeTraceBundleWithAllCoverageAndLedger(
		context.Background(), input, filepath.Join(dir, "capture"), result.Artifacts, nil,
		result.ProviderDecisions, nil, nil, result.TraceCoverage, ledger,
	)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(bundle.Path)
	if err != nil {
		t.Fatal(err)
	}
	var metadata traceBundleMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		t.Fatal(err)
	}
	if len(metadata.ProviderDecisions) != 2 || len(metadata.TraceCoverage) != 2 ||
		metadata.ProviderDecisions[0].ArtifactPath != "first.perftrace" ||
		metadata.ProviderDecisions[0].OutputPath != "first.perftrace" ||
		metadata.ProviderDecisions[1].ArtifactPath != "second.perftrace" ||
		metadata.ProviderDecisions[1].OutputPath != "second.perftrace" ||
		metadata.TraceCoverage[0].ArtifactPath != "first.perftrace" ||
		metadata.TraceCoverage[1].ArtifactPath != "second.perftrace" ||
		len(metadata.PerfClockAlignments) != 2 ||
		metadata.PerfClockAlignments[0].ArtifactPath != "first.perftrace" ||
		metadata.PerfClockAlignments[1].ArtifactPath != "second.perftrace" {
		t.Fatalf("same-profile public generations were not kept distinct: decisions=%+v coverage=%+v",
			metadata.ProviderDecisions, metadata.TraceCoverage)
	}
	if result.ProviderDecisions[0].ArtifactPath != first.Path || result.ProviderDecisions[1].ArtifactPath != second.Path ||
		result.TraceCoverage[0].ArtifactPath != first.Path || result.TraceCoverage[1].ArtifactPath != second.Path {
		t.Fatalf("same-profile bundle projection mutated public Result paths: decisions=%+v coverage=%+v",
			result.ProviderDecisions, result.TraceCoverage)
	}
}

func TestTraceBundlePerfReceiptAllowsOfficialFailureBeforeRawSuccess(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.perf.data")
	if err := os.WriteFile(input, []byte("input"), 0o600); err != nil {
		t.Fatal(err)
	}
	ledger, err := newConversionFileLedger(input)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.cleanup()
	artifact, rawSuccess := validatedResultPerfFixture(
		t, ledger, ownedTracePerfRaw, filepath.Join(dir, "capture.perftrace"),
	)
	rawSuccess.ParserMode = "auto"
	rawSuccess.Fallback = true
	officialFailure := perfProviderFailure(
		newPerfProviderDecision(
			perfProviderStageDirectInput, perfProviderByName(perfProviderNameSimpleperfText), Options{},
			input, perfInputLinuxPerfData, artifact.Path,
		),
		"official_output_unavailable", "official provider failed before raw fallback",
	)
	result := Result{
		Artifacts:         []Artifact{artifact},
		ProviderDecisions: []PerfProviderDecision{officialFailure, rawSuccess},
	}
	if err := reconcileResultOwnedPerfReceipts(&result, ledger); err != nil {
		t.Fatal(err)
	}
	bundle, err := writeTraceBundleWithAllCoverageAndLedger(
		context.Background(), input, filepath.Join(dir, "capture"), result.Artifacts, nil,
		result.ProviderDecisions, nil, nil, result.TraceCoverage, ledger,
	)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(bundle.Path)
	if err != nil {
		t.Fatal(err)
	}
	var metadata traceBundleMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		t.Fatal(err)
	}
	if len(metadata.ProviderDecisions) != 2 || metadata.ProviderDecisions[0].Succeeded ||
		metadata.ProviderDecisions[0].ArtifactPath != "" || metadata.ProviderDecisions[0].OutputPath != "capture.perftrace" ||
		!metadata.ProviderDecisions[1].Succeeded || !metadata.ProviderDecisions[1].Fallback ||
		metadata.ProviderDecisions[1].ArtifactPath != "capture.perftrace" ||
		metadata.ProviderDecisions[1].OutputPath != "capture.perftrace" {
		t.Fatalf("honest official-to-raw route was lost during bundle projection: %+v", metadata.ProviderDecisions)
	}
	if result.ProviderDecisions[0].OutputPath != artifact.Path || result.ProviderDecisions[1].ArtifactPath != artifact.Path ||
		result.TraceCoverage[0].ArtifactPath != artifact.Path {
		t.Fatalf("bundle projection mutated Result paths: decisions=%+v coverage=%+v",
			result.ProviderDecisions, result.TraceCoverage)
	}
}
