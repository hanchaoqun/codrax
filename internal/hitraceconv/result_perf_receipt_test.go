package hitraceconv

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validatedResultPerfFixture(
	t *testing.T,
	ledger *conversionFileLedger,
	profile ownedTracePerfProfile,
	path string,
) (Artifact, PerfProviderDecision) {
	t.Helper()
	writeOneValidatedPerfTraceForClaimTest(t, profile, path, ledger)
	spec, ok := profile.claimSpec()
	if !ok {
		t.Fatalf("fixture profile %q is not closed", profile)
	}
	inputFormat := perfInputLinuxPerfData
	stage := perfProviderStageDirectInput
	opts := Options{}
	if profile == ownedTracePerfSimpleperfProto {
		inputFormat = perfInputSimpleperfReportProto
	}
	if profile == ownedTracePerfHiperfProto {
		stage = perfProviderStageStandaloneHiperf
	}
	if profile == ownedTracePerfRaw {
		opts.PerfParser = "raw"
	}
	artifact, err := newValidatedPerfTraceArtifact(ledger, path, profile, inputFormat, "fixture", []string{"artifact disclosure"})
	if err != nil {
		t.Fatal(err)
	}
	decision := newPerfProviderDecision(stage, perfProviderByName(spec.providerName), opts, "input", inputFormat, path)
	if profile == ownedTracePerfRaw {
		decision.Fallback = false
	}
	decision, err = perfProviderSuccess(decision, artifact, ledger)
	if err != nil {
		t.Fatal(err)
	}
	return artifact, decision
}

func TestResultPerfReceiptReconcileIsExactAndIdempotent(t *testing.T) {
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.cleanup()
	dir := t.TempDir()
	firstArtifact, firstDecision := validatedResultPerfFixture(t, ledger, ownedTracePerfSimpleperfText, filepath.Join(dir, "first.perftrace"))
	secondArtifact, secondDecision := validatedResultPerfFixture(t, ledger, ownedTracePerfSimpleperfText, filepath.Join(dir, "second.perftrace"))
	protoArtifact, protoDecision := validatedResultPerfFixture(t, ledger, ownedTracePerfSimpleperfProto, filepath.Join(dir, "proto.perftrace"))
	hiperfArtifact, hiperfDecision := validatedResultPerfFixture(t, ledger, ownedTracePerfHiperfProto, filepath.Join(dir, "hiperf.perftrace"))
	rawArtifact, rawDecision := validatedResultPerfFixture(t, ledger, ownedTracePerfRaw, filepath.Join(dir, "raw.perftrace"))
	// Raw is the one profile whose fallback bit describes route selection;
	// this fixture is the explicit --perf-parser=raw route.
	result := Result{
		Artifacts: []Artifact{
			firstArtifact, firstArtifact, secondArtifact, protoArtifact, hiperfArtifact, rawArtifact,
		},
		ProviderDecisions: []PerfProviderDecision{
			firstDecision, secondDecision, protoDecision, hiperfDecision, rawDecision,
		},
	}
	if err := reconcileResultOwnedPerfReceipts(&result, ledger); err != nil {
		t.Fatal(err)
	}
	if len(result.TraceCoverage) != 5 {
		t.Fatalf("same-profile public generations need distinct coverage: %+v", result.TraceCoverage)
	}
	seen := map[string]bool{}
	for _, coverage := range result.TraceCoverage {
		if coverage.ArtifactPath == "" || !strings.HasPrefix(coverage.Table, "perftrace_") ||
			!coverage.Found || coverage.Error != "" || coverage.RowsEmitted != 1 {
			t.Fatalf("receipt coverage drifted: %+v", coverage)
		}
		seen[coverage.ArtifactPath] = true
	}
	if !seen[firstArtifact.Path] || !seen[secondArtifact.Path] || !seen[protoArtifact.Path] ||
		!seen[hiperfArtifact.Path] || !seen[rawArtifact.Path] {
		t.Fatalf("coverage lost public generation identity: %+v", result.TraceCoverage)
	}
	if err := reconcileResultOwnedPerfReceipts(&result, ledger); err != nil {
		t.Fatalf("exact receipt reconciliation is not idempotent: %v", err)
	}
	if len(result.TraceCoverage) != 5 {
		t.Fatalf("idempotent reconcile duplicated coverage: %+v", result.TraceCoverage)
	}
}

func TestResultPerfReceiptReconcileRejectsOneSidedClaims(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Result)
	}{
		{"missing_decision", func(result *Result) { result.ProviderDecisions = nil }},
		{"duplicate_decision", func(result *Result) {
			result.ProviderDecisions = append(result.ProviderDecisions, result.ProviderDecisions[0])
		}},
		{"decision_path_drift", func(result *Result) { result.ProviderDecisions[0].OutputPath += ".other" }},
		{"official_fallback_drift", func(result *Result) { result.ProviderDecisions[0].Fallback = true }},
		{"success_reason_drift", func(result *Result) { result.ProviderDecisions[0].Reason = "forged" }},
		{"output_whitespace_drift", func(result *Result) {
			result.ProviderDecisions[0].OutputPath = " " + result.ProviderDecisions[0].OutputPath
		}},
		{"input_format_whitespace_drift", func(result *Result) { result.ProviderDecisions[0].InputFormat += " " }},
		{"same_route_failure_and_success", func(result *Result) {
			failed := result.ProviderDecisions[0]
			failed.Succeeded = false
			failed.TraceQueryReady = false
			failed.ArtifactPath = ""
			failed.Reason = "failed"
			failed.Caveat = "failure disclosure"
			result.ProviderDecisions = append(result.ProviderDecisions, failed)
		}},
		{"artifact_bytes_drift", func(result *Result) { result.Artifacts[0].Bytes++ }},
		{"forged_coverage", func(result *Result) {
			result.TraceCoverage = append(result.TraceCoverage, TraceDBCoverage{
				Family: "trace_cross_validation", ArtifactPath: result.Artifacts[0].Path,
				Table: "perftrace_simpleperf_text", Role: "tracequery_cross_validation", Found: true,
			})
		}},
		{"unknown_reserved_coverage", func(result *Result) {
			result.TraceCoverage = append(result.TraceCoverage, TraceDBCoverage{Table: "perftrace_future", Found: true})
		}},
		{"db_lane_receipt_injection", func(result *Result) {
			result.TraceDBCoverage = append(result.TraceDBCoverage, TraceDBCoverage{
				ArtifactPath: result.Artifacts[0].Path, Table: "perftrace_simpleperf_text", Found: true,
			})
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ledger, err := newConversionFileLedger()
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.cleanup()
			artifact, decision := validatedResultPerfFixture(t, ledger, ownedTracePerfSimpleperfText, filepath.Join(t.TempDir(), "capture.perftrace"))
			result := Result{Artifacts: []Artifact{artifact}, ProviderDecisions: []PerfProviderDecision{decision}}
			test.mutate(&result)
			if err := reconcileResultOwnedPerfReceipts(&result, ledger); !ownedTraceOutputHardFailure(err) {
				t.Fatalf("one-sided claim escaped hard receipt gate: %T %v", err, err)
			}
		})
	}
}

func TestRawPerfDecisionFallbackBitMatchesParserRoute(t *testing.T) {
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.cleanup()
	path := filepath.Join(t.TempDir(), "raw.perftrace")
	writeOneValidatedPerfTraceForClaimTest(t, ownedTracePerfRaw, path, ledger)
	artifact, err := newValidatedPerfTraceArtifact(ledger, path, ownedTracePerfRaw, perfInputLinuxPerfData, "fixture", nil)
	if err != nil {
		t.Fatal(err)
	}
	newDecision := func(mode string, fallback bool) PerfProviderDecision {
		decision := newPerfProviderDecision(
			perfProviderStageDirectInput, perfProviderByName(perfProviderNameRawFallback),
			Options{PerfParser: mode}, "input", perfInputLinuxPerfData, path,
		)
		decision.Fallback = fallback
		decision.Selected = true
		decision.Attempted = true
		decision.Succeeded = true
		decision.TraceQueryReady = true
		decision.ArtifactPath = path
		return decision
	}
	for _, valid := range []PerfProviderDecision{
		newDecision("auto", true),
		newDecision("raw", false),
		newDecision("fallback", false),
	} {
		result := Result{Artifacts: []Artifact{artifact}, ProviderDecisions: []PerfProviderDecision{valid}}
		if err := reconcileResultOwnedPerfReceipts(&result, ledger); err != nil {
			t.Fatalf("valid raw route was rejected: decision=%+v err=%v", valid, err)
		}
	}
	for _, invalid := range []PerfProviderDecision{
		newDecision("auto", false),
		newDecision("raw", true),
		newDecision("official", false),
	} {
		result := Result{Artifacts: []Artifact{artifact}, ProviderDecisions: []PerfProviderDecision{invalid}}
		if err := reconcileResultOwnedPerfReceipts(&result, ledger); !ownedTraceOutputHardFailure(err) {
			t.Fatalf("impossible raw route escaped: decision=%+v err=%v", invalid, err)
		}
	}
}

func TestPerfNonSuccessDecisionRoutesRemainDisclosed(t *testing.T) {
	tests := []struct {
		name     string
		profile  ownedTracePerfProfile
		decision PerfProviderDecision
	}{
		{
			name:    "hiperf_skipped_for_explicit_raw",
			profile: ownedTracePerfHiperfProto,
			decision: perfProviderSkipped(
				newPerfProviderDecision(perfProviderStageStandaloneHiperf, perfProviderByName(perfProviderNameHiperfProto), Options{PerfParser: "raw"}, "input", perfInputLinuxPerfData, "out.perftrace"),
				false, "skipped_by_raw_parser_mode", "official provider skipped",
			),
		},
		{
			name:    "raw_disabled_by_official_mode",
			profile: ownedTracePerfRaw,
			decision: perfProviderSkipped(
				newPerfProviderDecision(perfProviderStageDirectInput, perfProviderByName(perfProviderNameRawFallback), Options{PerfParser: "official"}, "input", perfInputLinuxPerfData, "out.perftrace"),
				false, "disabled_by_parser_mode", "raw provider disabled",
			),
		},
		{
			name:    "raw_explicit_mode_unsupported_proto",
			profile: ownedTracePerfRaw,
			decision: perfProviderSkipped(
				newPerfProviderDecision(perfProviderStageDirectInput, perfProviderByName(perfProviderNameRawFallback), Options{PerfParser: "fallback"}, "input", perfInputSimpleperfReportProto, "out.perftrace"),
				true, "unsupported_input_format", "raw provider cannot parse proto",
			),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !ownedPerfNonSuccessDecisionRouteValid(test.decision, test.profile) {
				t.Fatalf("honest non-success route was rejected: %+v", test.decision)
			}
			ledger, err := newConversionFileLedger()
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.cleanup()
			result := Result{ProviderDecisions: []PerfProviderDecision{test.decision}}
			if err := reconcileResultOwnedPerfReceipts(&result, ledger); err != nil {
				t.Fatalf("honest non-success disclosure failed reconcile: %v", err)
			}
		})
	}
}

func TestAnalyzableStandaloneSidecarRequiresValidatedArtifactClaim(t *testing.T) {
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.cleanup()
	claimlessPath := filepath.Join(t.TempDir(), "claimless.perftrace")
	if err := os.WriteFile(claimlessPath, []byte("claimless"), 0o600); err != nil {
		t.Fatal(err)
	}
	claimless := Artifact{
		Type: ArtifactPerfTrace, Path: claimlessPath, Bytes: 9,
		Perf: perfCapabilityForSimpleperfReportSample(perfInputLinuxPerfData, "fixture"),
	}
	claimless.Perf.TraceQueryReady = true
	if ready, err := hasAnalyzableStandaloneSidecar(context.Background(), []Artifact{claimless}, ledger); err == nil || ready || !ownedTraceOutputHardFailure(err) {
		t.Fatal("path/type/capability-only sidecar became analyzable")
	}
	artifact, _ := validatedResultPerfFixture(t, ledger, ownedTracePerfSimpleperfText, filepath.Join(t.TempDir(), "validated.perftrace"))
	if ready, err := hasAnalyzableStandaloneSidecar(context.Background(), []Artifact{artifact}, ledger); err != nil || !ready {
		t.Fatal("exact validated perf sidecar was not analyzable")
	}
	forged := artifact
	capability := *artifact.Perf
	forged.Perf = &capability
	forged.Perf.CPUIdentity = "forged"
	if ready, err := hasAnalyzableStandaloneSidecar(context.Background(), []Artifact{forged}, ledger); err == nil || ready || !ownedTraceOutputHardFailure(err) {
		t.Fatal("semantic drift survived standalone sidecar gate")
	}
	replacement := bytes.Repeat([]byte{'x'}, int(artifact.Bytes))
	if err := os.WriteFile(artifact.Path, replacement, 0o644); err == nil {
		if ready, gateErr := hasAnalyzableStandaloneSidecar(context.Background(), []Artifact{artifact}, ledger); gateErr == nil || ready || !ownedTraceOutputHardFailure(gateErr) {
			t.Fatalf("same-path generation drift survived standalone sidecar gate: ready=%t err=%v", ready, gateErr)
		}
	}
}

func TestResultPerfReceiptCoveragePrecedesTraceBundle(t *testing.T) {
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.cleanup()
	dir := t.TempDir()
	artifact, decision := validatedResultPerfFixture(t, ledger, ownedTracePerfRaw, filepath.Join(dir, "capture.perftrace"))
	result := Result{Artifacts: []Artifact{artifact}, ProviderDecisions: []PerfProviderDecision{decision}}
	if err := finalizeResultTraceBundleWithLedger(context.Background(), filepath.Join(dir, "input.perf.data"), filepath.Join(dir, "capture"), &result, ledger); err != nil {
		t.Fatal(err)
	}
	if result.BundlePath == "" || len(result.TraceCoverage) != 1 || result.TraceCoverage[0].ArtifactPath != artifact.Path {
		t.Fatalf("result receipt was not closed before bundle: %+v", result)
	}
	body, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	var metadata traceBundleMetadata
	if err := json.Unmarshal(body, &metadata); err != nil {
		t.Fatal(err)
	}
	if len(metadata.TraceCoverage) != 1 || metadata.TraceCoverage[0].ArtifactPath != filepath.Base(artifact.Path) ||
		metadata.TraceCoverage[0].Table != "perftrace_raw_perf" {
		t.Fatalf("tracebundle missed receipt-derived coverage: %+v", metadata.TraceCoverage)
	}
}

func TestResultBundleProductionPathsUseSingleFinalizer(t *testing.T) {
	tests := []struct {
		file, function string
		wantCalls      int
	}{
		{"simpleperf_text.go", "maybeConvertDirectSimpleperfPerfData", 1},
		{"trace_streamer_provider.go", "convertTraceStreamerOnly", 1},
		{"profiler_container.go", "tryConvertProfilerContainerWithLedger", 1},
		{"convert.go", "ConvertFile", 4},
	}
	for _, test := range tests {
		body := sourceGenerationFunctionBody(t, test.file, test.function)
		if got := strings.Count(body, "finalizeResultTraceBundleWithLedger("); got != test.wantCalls {
			t.Fatalf("%s finalizer calls=%d want=%d:\n%s", test.function, got, test.wantCalls, body)
		}
		if strings.Contains(body, "writeTraceBundleWithLedger(") || strings.Contains(body, "writeTraceBundleWithAllCoverageAndLedger(") {
			t.Fatalf("%s bypassed the Result receipt finalizer:\n%s", test.function, body)
		}
	}
	finalizer := sourceGenerationFunctionBody(t, "result_perf_receipt.go", "finalizeResultTraceBundleWithLedger")
	reconcileAt := strings.Index(finalizer, "reconcileResultOwnedPerfReceipts(result, ledger)")
	normalizeAt := strings.Index(finalizer, "normalizeResultCollections(result)")
	bundleAt := strings.Index(finalizer, "writeTraceBundleWithAllCoverageAndLedger(")
	if reconcileAt < 0 || normalizeAt <= reconcileAt || bundleAt <= normalizeAt {
		t.Fatalf("Result receipt coverage is not closed before bundle construction:\n%s", finalizer)
	}
}
