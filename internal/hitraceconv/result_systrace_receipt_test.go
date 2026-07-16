package hitraceconv

import (
	"context"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

func validatedResultBuiltinSystraceFixture(
	t testing.TB,
	ledger *conversionFileLedger,
	inputPath string,
	outputPath string,
	rows []renderedRow,
) (Artifact, TraceProviderDecision, TraceDBCoverage) {
	t.Helper()
	publication, err := writeValidatedOwnedBuiltinSystraceWithLedger(
		context.Background(), outputPath, rows, ledger,
	)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := traceProviderPublished(
		newTraceProviderDecision(
			traceProviderStageTraceBody,
			traceProviderByName(traceProviderNameBuiltinSys),
			Options{TraceEngine: traceEngineBuiltin},
			inputPath,
			publication.Artifact.Path,
		),
		publication.Artifact,
		ledger,
	)
	if err != nil {
		t.Fatal(err)
	}
	return publication.Artifact, decision, publication.TraceCoverage
}

func validatedSQLSystraceResultFixture(t *testing.T) (Result, *conversionFileLedger) {
	t.Helper()
	ledger, bindingPath, _ := publishOwnedSQLSystraceClaimFixture(t)
	artifact, err := newValidatedSystraceArtifact(
		ledger, bindingPath, ownedTraceValidationSQL,
		[]string{"generated from trace_streamer SQLite DB rows"},
	)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := traceProviderPublished(
		newTraceProviderDecision(
			traceProviderStageTraceBody,
			traceProviderByName(traceProviderNameTraceStreamer),
			Options{TraceEngine: traceEngineTraceStreamer},
			"capture.sys",
			artifact.Path,
		),
		artifact,
		ledger,
	)
	if err != nil {
		t.Fatal(err)
	}
	return Result{
		OutputPath:     artifact.Path,
		OutputBytes:    artifact.Bytes,
		EventsWritten:  artifact.Trace.Rows,
		Artifacts:      []Artifact{artifact},
		TraceDecisions: []TraceProviderDecision{decision},
	}, ledger
}

func TestResultOwnedSystraceReceiptReconcileIsExactAndIdempotent(t *testing.T) {
	result, ledger := validatedSQLSystraceResultFixture(t)
	if err := reconcileResultOwnedSystraceReceipts(&result, ledger); err != nil {
		t.Fatal(err)
	}
	if len(result.TraceCoverage) != 1 ||
		!tracebundle.IsSystraceReceiptCoverage(
			result.TraceCoverage[0].Family,
			result.TraceCoverage[0].Table,
			result.TraceCoverage[0].Role,
			result.TraceCoverage[0].ArtifactPath,
		) || result.TraceCoverage[0].ArtifactPath != result.OutputPath {
		t.Fatalf("closed systrace receipt coverage was not projected: %+v", result.TraceCoverage)
	}
	want := cloneTraceDBCoverageList(result.TraceCoverage)
	if err := reconcileResultOwnedSystraceReceipts(&result, ledger); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.TraceCoverage, want) {
		t.Fatalf("second reconcile duplicated or changed receipt coverage: got=%+v want=%+v", result.TraceCoverage, want)
	}
}

func TestResultOwnedSystraceReceiptAllowsInventorySuccessWithoutReadiness(t *testing.T) {
	dir := t.TempDir()
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.cleanup()
	artifact, decision, coverage := validatedResultBuiltinSystraceFixture(
		t, ledger, "capture.sys", dir+"/inventory.systrace", nil,
	)
	if artifact.Trace == nil || artifact.Trace.TraceQueryReady || !decision.Succeeded || decision.TraceQueryReady {
		t.Fatalf("inventory receipt was not a successful, non-ready publication: artifact=%+v decision=%+v", artifact, decision)
	}
	result := Result{
		OutputPath: artifact.Path, OutputBytes: artifact.Bytes, EventsWritten: 0,
		Artifacts: []Artifact{artifact}, TraceDecisions: []TraceProviderDecision{decision},
		TraceCoverage: []TraceDBCoverage{coverage},
	}
	if err := reconcileResultOwnedSystraceReceipts(&result, ledger); err != nil {
		t.Fatal(err)
	}
}

func validatedBuiltinAfterSQLFailureResultFixture(t testing.TB) (Result, *conversionFileLedger) {
	t.Helper()
	dir := t.TempDir()
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	output := dir + "/fallback.systrace"
	artifact, success, coverage := validatedResultBuiltinSystraceFixture(
		t, ledger, "capture.sys", output, []renderedRow{builtinWriterKnownRow(1_000_000, 0)},
	)
	failure := traceProviderFailure(
		newTraceProviderDecision(
			traceProviderStageTraceBody,
			traceProviderByName(traceProviderNameTraceStreamer),
			Options{TraceEngine: traceEngineAuto},
			"capture.sys",
			output,
		),
		"trace_db_normalize_failed",
		"trace_streamer DB did not produce a validated systrace",
	)
	return Result{
		OutputPath: artifact.Path, OutputBytes: artifact.Bytes, EventsWritten: artifact.Trace.Rows,
		Artifacts: []Artifact{artifact}, TraceDecisions: []TraceProviderDecision{failure, success},
		TraceCoverage: []TraceDBCoverage{coverage},
	}, ledger
}

func pathlessSQLFailureCoverage() TraceDBCoverage {
	return TraceDBCoverage{
		Family: tracebundle.SystraceReceiptFamily,
		Table:  tracebundle.SystraceReceiptTableSQL,
		Role:   tracebundle.SystraceReceiptRole,
		Found:  true,
		Error:  traceDBPostvalidationScanFailed,
	}
}

func TestResultOwnedSystraceReceiptAllowsSQLFailureBeforeBuiltinSuccess(t *testing.T) {
	result, ledger := validatedBuiltinAfterSQLFailureResultFixture(t)
	defer ledger.cleanup()
	result.TraceCoverage = append([]TraceDBCoverage{pathlessSQLFailureCoverage()}, result.TraceCoverage...)
	if err := reconcileResultOwnedSystraceReceipts(&result, ledger); err != nil {
		t.Fatal(err)
	}
}

func makeSQLPredecessorSkipped(result *Result) {
	if result == nil || len(result.TraceDecisions) == 0 {
		return
	}
	result.TraceDecisions[0].Selected = false
	result.TraceDecisions[0].Attempted = false
	result.TraceDecisions[0].Reason = "trace_streamer_skipped"
}

func TestResultOwnedSystraceReceiptAllowsCanonicalSkippedSQLPredecessor(t *testing.T) {
	result, ledger := validatedBuiltinAfterSQLFailureResultFixture(t)
	defer ledger.cleanup()
	makeSQLPredecessorSkipped(&result)
	if err := reconcileResultOwnedSystraceReceipts(&result, ledger); err != nil {
		t.Fatal(err)
	}
}

func TestResultOwnedSystraceReceiptRejectsNonCanonicalOrLateSQLPredecessor(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Result)
	}{
		{name: "skipped_different_output", mutate: func(result *Result) {
			makeSQLPredecessorSkipped(result)
			result.TraceDecisions[0].OutputPath += ".other"
		}},
		{name: "skipped_explicit_engine", mutate: func(result *Result) {
			makeSQLPredecessorSkipped(result)
			result.TraceDecisions[0].EngineMode = traceEngineTraceStreamer
		}},
		{name: "skipped_after_success", mutate: func(result *Result) {
			makeSQLPredecessorSkipped(result)
			result.TraceDecisions[0], result.TraceDecisions[1] = result.TraceDecisions[1], result.TraceDecisions[0]
		}},
		{name: "attempted_failure_after_success", mutate: func(result *Result) {
			result.TraceDecisions[0], result.TraceDecisions[1] = result.TraceDecisions[1], result.TraceDecisions[0]
		}},
		{name: "duplicate_skipped", mutate: func(result *Result) {
			makeSQLPredecessorSkipped(result)
			result.TraceDecisions = append(result.TraceDecisions, result.TraceDecisions[0])
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, ledger := validatedBuiltinAfterSQLFailureResultFixture(t)
			defer ledger.cleanup()
			test.mutate(&result)
			if err := reconcileResultOwnedSystraceReceipts(&result, ledger); err == nil || !ownedTraceOutputHardFailure(err) {
				t.Fatalf("noncanonical SQL predecessor was accepted: %+v", result.TraceDecisions)
			}
		})
	}
}

func TestResultOwnedSystraceReceiptRejectsOneSidedAndForgedClaims(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Result)
	}{
		{name: "type_only_artifact", mutate: func(result *Result) {
			result.Artifacts[0].Trace = nil
		}},
		{name: "artifact_bytes", mutate: func(result *Result) {
			result.Artifacts[0].Bytes++
		}},
		{name: "artifact_sha", mutate: func(result *Result) {
			result.Artifacts[0].SHA256 = "00"
		}},
		{name: "artifact_profile", mutate: func(result *Result) {
			result.Artifacts[0].Trace.ValidationProfile = string(ownedTraceValidationBuiltin)
		}},
		{name: "relative_private_binding", mutate: func(result *Result) {
			result.Artifacts[0].traceReceiptBindingPath = "capture.systrace"
		}},
		{name: "duplicate_artifact", mutate: func(result *Result) {
			result.Artifacts = append(result.Artifacts, result.Artifacts[0])
		}},
		{name: "missing_success_decision", mutate: func(result *Result) {
			result.TraceDecisions = nil
		}},
		{name: "duplicate_success_decision", mutate: func(result *Result) {
			result.TraceDecisions = append(result.TraceDecisions, result.TraceDecisions[0])
		}},
		{name: "decision_artifact_path", mutate: func(result *Result) {
			result.TraceDecisions[0].ArtifactPath += ".other"
		}},
		{name: "decision_readiness", mutate: func(result *Result) {
			result.TraceDecisions[0].TraceQueryReady = false
		}},
		{name: "decision_provider_kind", mutate: func(result *Result) {
			result.TraceDecisions[0].ProviderKind = traceProviderKindBuiltinSys
		}},
		{name: "output_path", mutate: func(result *Result) {
			result.OutputPath += ".other"
		}},
		{name: "output_bytes", mutate: func(result *Result) {
			result.OutputBytes++
		}},
		{name: "events_written", mutate: func(result *Result) {
			result.EventsWritten++
		}},
		{name: "orphan_coverage", mutate: func(result *Result) {
			coverage := cloneTraceDBCoverage(result.TraceCoverage[0])
			coverage.ArtifactPath += ".orphan"
			result.TraceCoverage = []TraceDBCoverage{coverage}
		}},
		{name: "duplicate_coverage", mutate: func(result *Result) {
			result.TraceCoverage = append(result.TraceCoverage, cloneTraceDBCoverage(result.TraceCoverage[0]))
		}},
		{name: "coverage_in_db_lane", mutate: func(result *Result) {
			result.TraceDBCoverage = append(result.TraceDBCoverage, result.TraceCoverage[0])
			result.TraceCoverage = nil
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, ledger := validatedSQLSystraceResultFixture(t)
			if err := reconcileResultOwnedSystraceReceipts(&result, ledger); err != nil {
				t.Fatal(err)
			}
			test.mutate(&result)
			if err := reconcileResultOwnedSystraceReceipts(&result, ledger); err == nil || !ownedTraceOutputHardFailure(err) {
				t.Fatalf("forged systrace Result was accepted: %+v", result)
			}
		})
	}
}

func TestResultOwnedSystraceReceiptRejectsForgedPathlessFailureCoverage(t *testing.T) {
	tests := []struct {
		name           string
		withoutFailure bool
		mutate         func(*TraceDBCoverage)
	}{
		{name: "orphan_without_failed_decision", withoutFailure: true},
		{name: "wrong_family", mutate: func(item *TraceDBCoverage) { item.Family = "forged_family" }},
		{name: "wrong_role", mutate: func(item *TraceDBCoverage) { item.Role = "advisory" }},
		{name: "builtin_profile", mutate: func(item *TraceDBCoverage) { item.Table = tracebundle.SystraceReceiptTableBuiltin }},
		{name: "profiler_profile", mutate: func(item *TraceDBCoverage) { item.Table = tracebundle.SystraceReceiptTableProfiler }},
		{name: "noncanonical_error", mutate: func(item *TraceDBCoverage) { item.Error += " " }},
		{name: "unknown_error", mutate: func(item *TraceDBCoverage) { item.Error = "forged_postvalidation" }},
		{name: "whitespace_artifact_path", mutate: func(item *TraceDBCoverage) { item.ArtifactPath = " " }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var result Result
			var ledger *conversionFileLedger
			if test.withoutFailure {
				result, ledger = validatedSQLSystraceResultFixture(t)
			} else {
				result, ledger = validatedBuiltinAfterSQLFailureResultFixture(t)
			}
			defer ledger.cleanup()
			coverage := pathlessSQLFailureCoverage()
			if test.mutate != nil {
				test.mutate(&coverage)
			}
			result.TraceCoverage = append(result.TraceCoverage, coverage)
			if err := reconcileResultOwnedSystraceReceipts(&result, ledger); err == nil || !ownedTraceOutputHardFailure(err) {
				t.Fatalf("forged pathless failure coverage was accepted: %+v", coverage)
			}
		})
	}
}

func TestResultOwnedSystraceReceiptRejectsDuplicateFailureDecisionAndCoverage(t *testing.T) {
	t.Run("decision", func(t *testing.T) {
		result, ledger := validatedBuiltinAfterSQLFailureResultFixture(t)
		defer ledger.cleanup()
		result.TraceDecisions = append(result.TraceDecisions, result.TraceDecisions[0])
		if err := reconcileResultOwnedSystraceReceipts(&result, ledger); err == nil || !ownedTraceOutputHardFailure(err) {
			t.Fatalf("duplicate systrace failure decision was accepted: %+v", result.TraceDecisions)
		}
	})
	t.Run("pathless_coverage", func(t *testing.T) {
		result, ledger := validatedBuiltinAfterSQLFailureResultFixture(t)
		defer ledger.cleanup()
		coverage := pathlessSQLFailureCoverage()
		result.TraceCoverage = append(result.TraceCoverage, coverage, coverage)
		if err := reconcileResultOwnedSystraceReceipts(&result, ledger); err == nil || !ownedTraceOutputHardFailure(err) {
			t.Fatalf("duplicate pathless systrace failure coverage was accepted: %+v", result.TraceCoverage)
		}
	})
}

func TestResultOwnedSystraceReceiptRejectsNonCanonicalFailureDecision(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TraceProviderDecision)
	}{
		{name: "empty_output", mutate: func(item *TraceProviderDecision) { item.OutputPath = "" }},
		{name: "padded_output", mutate: func(item *TraceProviderDecision) { item.OutputPath += " " }},
		{name: "empty_reason", mutate: func(item *TraceProviderDecision) { item.Reason = "" }},
		{name: "padded_reason", mutate: func(item *TraceProviderDecision) { item.Reason += " " }},
		{name: "selected_without_attempt", mutate: func(item *TraceProviderDecision) { item.Attempted = false }},
		{name: "different_fallback_output", mutate: func(item *TraceProviderDecision) { item.OutputPath += ".other" }},
		{name: "explicit_engine_with_success_sibling", mutate: func(item *TraceProviderDecision) {
			item.EngineMode = traceEngineTraceStreamer
			item.Fallback = false
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, ledger := validatedBuiltinAfterSQLFailureResultFixture(t)
			defer ledger.cleanup()
			test.mutate(&result.TraceDecisions[0])
			if err := reconcileResultOwnedSystraceReceipts(&result, ledger); err == nil || !ownedTraceOutputHardFailure(err) {
				t.Fatalf("noncanonical systrace failure decision was accepted: %+v", result.TraceDecisions[0])
			}
		})
	}
}

func TestResultOwnedSystraceReceiptPathlessFailureRequiresNormalizeFailureDecision(t *testing.T) {
	result, ledger := validatedBuiltinAfterSQLFailureResultFixture(t)
	defer ledger.cleanup()
	result.TraceDecisions[0].Reason = "trace_streamer_unavailable"
	result.TraceCoverage = append(result.TraceCoverage, pathlessSQLFailureCoverage())
	if err := reconcileResultOwnedSystraceReceipts(&result, ledger); err == nil || !ownedTraceOutputHardFailure(err) {
		t.Fatalf("pathless postvalidation coverage accepted an unrelated decision: %+v", result.TraceDecisions[0])
	}
}
