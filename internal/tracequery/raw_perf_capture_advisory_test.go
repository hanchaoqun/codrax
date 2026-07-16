package tracequery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

func r2aRawPerfCapture() traceBundleRawPerfCaptureCompleteness {
	return traceBundleRawPerfCaptureCompleteness{
		Profile:     traceBundleRawPerfCaptureProfile,
		Source:      traceBundleRawPerfCaptureSource,
		LostRecords: traceBundleRawPerfRecordCensus{Physical: 1, Accepted: 1},
		LostEvents:  traceBundleRawPerfAggregateTotal{State: traceBundleRawPerfAggregateExact, Value: 1},
		LostSamples: traceBundleRawPerfAggregateTotal{
			State: traceBundleRawPerfAggregateNotReported,
		},
		AuxBytes: traceBundleRawPerfAggregateTotal{
			State: traceBundleRawPerfAggregateNotReported,
		},
	}
}

func r2aRawPerfBundle(capture traceBundleRawPerfCaptureCompleteness, ready bool) traceBundleFile {
	artifactCapture := capture
	coverageCapture := capture
	return traceBundleFile{
		schemaMode: traceBundleSchemaV2,
		Artifacts: []traceBundleArtifact{{
			Type: "perftrace", Path: "capture.perftrace", Converter: "hitraceconv-v1+raw-perfdata",
			Perf: &traceBundlePerfCapability{
				ProviderKind: "raw_fallback", ProviderName: "codrax_raw_perfdata",
				InputFormat: "linux_perf_data", OutputFormat: "codrax_perftrace",
				TimeDomain: "perf_data_time_ns", TimeAlignment: "assumed",
				ThreadIdentity: "pid_tid_from_sample_or_comm", CPUIdentity: "sample_cpu_when_recorded",
				EventWeight: "period_or_1", Symbolization: "hiperf_saved_symbols_or_unsymbolized_ip",
				Callchain: "symbolized_when_hiperf_files_symbol_present_else_ip_only",
				DSOLabel:  "mmap_best_effort", BuildID: "feature_build_id_when_present",
				OffCPU: "hiperf_cpu_off_sched_switch_when_event_desc_present", Confidence: "degraded",
				Degraded: true,
				Caveats: []string{
					"raw fallback resolves function names only from saved hiperf symbol sections; without those sections it remains IP/DSO-level",
					"raw fallback can label hiperf --offcpu sched_switch samples when official EVENT_DESC and HIPERF_CPU_OFF features are present, but full off-CPU stack expansion still needs official hiperf report flow",
				},
				TraceQueryReady: ready, RawCaptureCompleteness: &artifactCapture,
			},
		}},
		ProviderDecisions: []traceBundleProviderDecision{{
			Stage: "direct_input", ProviderKind: "raw_fallback", ProviderName: "codrax_raw_perfdata",
			InputPath: "capture.perf.data", InputFormat: "linux_perf_data", OutputPath: "capture.perftrace",
			ParserMode: "raw", Selected: true, Attempted: true, Succeeded: true, Fallback: false,
			TraceQueryReady: ready, ArtifactPath: "capture.perftrace",
		}},
		TraceCoverage: []traceBundleCoverage{{
			Family: tracebundle.PerfReceiptFamily, Table: tracebundle.PerfReceiptTableRawPerf,
			Role: tracebundle.PerfReceiptRole, ArtifactPath: "capture.perftrace",
			Found: true, RowsRead: traceBundleOwnedPerftraceHeaderLines + int(capture.SampleRecords.Accepted),
			RowsEmitted: int(capture.SampleRecords.Accepted), RawCaptureCompleteness: &coverageCapture,
		}},
	}
}

func TestRawPerfCaptureAdvisoryPreservesExactZeroNotReportedAndUnknown(t *testing.T) {
	capture := r2aRawPerfCapture()
	capture.SampleRecords = traceBundleRawPerfRecordCensus{Physical: 1, Accepted: 1}
	capture.LostRecords = traceBundleRawPerfRecordCensus{Physical: 1, Accepted: 1}
	capture.LostEvents = traceBundleRawPerfAggregateTotal{State: traceBundleRawPerfAggregateExact}
	capture.LostSampleRecords = traceBundleRawPerfRecordCensus{Physical: 2, Accepted: 2}
	capture.LostSamples = traceBundleRawPerfAggregateTotal{
		State: traceBundleRawPerfAggregateUnknown, Reason: traceBundleRawPerfUnknownOverflow,
	}

	caveats := traceBundleRawPerfCaptureCompletenessCaveats(r2aRawPerfBundle(capture, true))
	if len(caveats) != 1 {
		t.Fatalf("raw census caveat cardinality=%d: %v", len(caveats), caveats)
	}
	caveat := caveats[0]
	for _, want := range []string{
		RawPerfCaptureCompletenessCaveatToken,
		"authority=manifest_advisory",
		"capture_hard_gate=false",
		"positive_evidence=preserve",
		"absence_policy=require_quality_caveat",
		"census_scope=observed_perf_record_stream",
		"device_capture_completeness=not_claimed",
		"valid=true",
		"query_ready=true",
		"lost_events=exact:0",
		"lost_samples=unknown:aggregate_overflow",
		"aux_bytes=not_reported",
	} {
		if !strings.Contains(caveat, want) {
			t.Fatalf("valid raw census caveat missing %q:\n%s", want, caveat)
		}
	}
	if strings.Contains(caveat, "sample_aggregation=none") || strings.Contains(caveat, "effective_clock_evidence=none") {
		t.Fatalf("query-ready positive samples were mislabeled inventory-only: %s", caveat)
	}
}

func TestRawPerfCaptureInventoryIsQualityOnlyAndNeverAnalysisEvidence(t *testing.T) {
	capture := r2aRawPerfCapture()
	capture.LostRecords = traceBundleRawPerfRecordCensus{Physical: 1, Accepted: 1}
	capture.LostEvents = traceBundleRawPerfAggregateTotal{State: traceBundleRawPerfAggregateExact, Value: 7}
	caveats := traceBundleRawPerfCaptureCompletenessCaveats(r2aRawPerfBundle(capture, false))
	if len(caveats) != 1 {
		t.Fatalf("inventory caveat cardinality=%d: %v", len(caveats), caveats)
	}
	for _, want := range []string{
		"query_ready=false", "capture_state=inventory_only", "capture_quality_issue=true",
		"effective_clock_evidence=none", "sample_aggregation=none", "clock_alignment=none",
		"thread_attribution=none", "root_cause_rank=none", "census_participation=capture_quality_only",
		"sample_records=physical:0,accepted:0,rejected:0", "lost_events=exact:7",
	} {
		if !strings.Contains(caveats[0], want) {
			t.Fatalf("inventory boundary missing %q:\n%s", want, caveats[0])
		}
	}
}

func TestRawPerfCaptureReadyAuxUnknownIsQualityIssueWithoutChangingReadiness(t *testing.T) {
	capture := r2aRawPerfCapture()
	capture.SampleRecords = traceBundleRawPerfRecordCensus{Physical: 1, Accepted: 1}
	capture.AuxRecords = traceBundleRawPerfRecordCensus{Physical: 2, Accepted: 2}
	capture.AuxBytes = traceBundleRawPerfAggregateTotal{
		State: traceBundleRawPerfAggregateUnknown, Reason: traceBundleRawPerfUnknownOverflow,
	}
	caveats := traceBundleRawPerfCaptureCompletenessCaveats(r2aRawPerfBundle(capture, true))
	if len(caveats) != 1 || !strings.Contains(caveats[0], "query_ready=true") ||
		!strings.Contains(caveats[0], "capture_quality_issue=true") ||
		!strings.Contains(caveats[0], "aux_bytes=unknown:aggregate_overflow") {
		t.Fatalf("ready AUX unknown quality disclosure drifted: %v", caveats)
	}
}

func TestRawPerfCaptureInventoryRequiresProducerZeroSampleIssueGate(t *testing.T) {
	empty := func() traceBundleRawPerfCaptureCompleteness {
		capture := r2aRawPerfCapture()
		capture.LostRecords = traceBundleRawPerfRecordCensus{}
		capture.LostEvents = traceBundleRawPerfAggregateTotal{State: traceBundleRawPerfAggregateNotReported}
		return capture
	}
	invalid := []struct {
		name   string
		mutate func(*traceBundleRawPerfCaptureCompleteness)
	}{
		{name: "clean_zero"},
		{name: "exact_zero_loss", mutate: func(capture *traceBundleRawPerfCaptureCompleteness) {
			capture.LostRecords = traceBundleRawPerfRecordCensus{Physical: 1, Accepted: 1}
			capture.LostEvents = traceBundleRawPerfAggregateTotal{State: traceBundleRawPerfAggregateExact}
		}},
		{name: "aux_only", mutate: func(capture *traceBundleRawPerfCaptureCompleteness) {
			capture.AuxRecords = traceBundleRawPerfRecordCensus{Physical: 1, Accepted: 1}
			capture.AuxBytes = traceBundleRawPerfAggregateTotal{State: traceBundleRawPerfAggregateExact, Value: 64}
		}},
		{name: "aux_overflow", mutate: func(capture *traceBundleRawPerfCaptureCompleteness) {
			capture.AuxRecords = traceBundleRawPerfRecordCensus{Physical: 2, Accepted: 2}
			capture.AuxBytes = traceBundleRawPerfAggregateTotal{State: traceBundleRawPerfAggregateUnknown, Reason: traceBundleRawPerfUnknownOverflow}
		}},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			capture := empty()
			if test.mutate != nil {
				test.mutate(&capture)
			}
			caveats := traceBundleRawPerfCaptureCompletenessCaveats(r2aRawPerfBundle(capture, false))
			if len(caveats) != 1 || !strings.Contains(caveats[0], "valid=false") || strings.Contains(caveats[0], "physical:") {
				t.Fatalf("zero-sample non-issue inventory did not fail closed: %v", caveats)
			}
		})
	}

	rejected := empty()
	rejected.SampleRecords = traceBundleRawPerfRecordCensus{Physical: 1, Rejected: 1}
	if caveats := traceBundleRawPerfCaptureCompletenessCaveats(r2aRawPerfBundle(rejected, false)); len(caveats) != 1 || !strings.Contains(caveats[0], "valid=true") ||
		!strings.Contains(caveats[0], "capture_quality_issue=true") {
		t.Fatalf("rejected sample inventory should satisfy the producer issue gate: %v", caveats)
	}
}

func TestRawPerfCaptureInvalidOrInconsistentPayloadPublishesNoDeclaredNumbers(t *testing.T) {
	validCapture := r2aRawPerfCapture()
	validCapture.LostRecords = traceBundleRawPerfRecordCensus{Physical: 1, Accepted: 1}
	validCapture.LostEvents = traceBundleRawPerfAggregateTotal{State: traceBundleRawPerfAggregateExact, Value: 8675309}

	tests := []struct {
		name   string
		mutate func(*traceBundleFile)
	}{
		{name: "legacy_schema", mutate: func(bundle *traceBundleFile) { bundle.schemaMode = traceBundleSchemaLegacy }},
		{name: "invalid_profile", mutate: func(bundle *traceBundleFile) { bundle.Artifacts[0].Perf.RawCaptureCompleteness.Profile = "forged" }},
		{name: "invalid_source", mutate: func(bundle *traceBundleFile) { bundle.Artifacts[0].Perf.RawCaptureCompleteness.Source = "forged" }},
		{name: "artifact_missing_census", mutate: func(bundle *traceBundleFile) {
			bundle.Artifacts[0].Perf.RawCaptureCompleteness = nil
		}},
		{name: "all_raw_metadata_census_stripped", mutate: func(bundle *traceBundleFile) {
			bundle.Artifacts[0].Perf.RawCaptureCompleteness = nil
			bundle.ProviderDecisions = nil
			bundle.TraceCoverage = nil
		}},
		{name: "closed_profile_converter_drift", mutate: func(bundle *traceBundleFile) {
			bundle.Artifacts[0].Converter = "forged+raw-perfdata"
		}},
		{name: "closed_profile_identity_drift", mutate: func(bundle *traceBundleFile) {
			bundle.Artifacts[0].Perf.ThreadIdentity = "forged"
		}},
		{name: "closed_profile_caveat_missing", mutate: func(bundle *traceBundleFile) {
			bundle.Artifacts[0].Perf.Caveats = bundle.Artifacts[0].Perf.Caveats[:1]
		}},
		{name: "closed_profile_caveat_reordered", mutate: func(bundle *traceBundleFile) {
			bundle.Artifacts[0].Perf.Caveats[0], bundle.Artifacts[0].Perf.Caveats[1] =
				bundle.Artifacts[0].Perf.Caveats[1], bundle.Artifacts[0].Perf.Caveats[0]
		}},
		{name: "closed_profile_caveat_edited", mutate: func(bundle *traceBundleFile) {
			bundle.Artifacts[0].Perf.Caveats[0] += " forged"
		}},
		{name: "closed_profile_caveat_extra", mutate: func(bundle *traceBundleFile) {
			bundle.Artifacts[0].Perf.Caveats = append(bundle.Artifacts[0].Perf.Caveats, "forged")
		}},
		{name: "unclosed_record", mutate: func(bundle *traceBundleFile) {
			bundle.Artifacts[0].Perf.RawCaptureCompleteness.LostRecords.Rejected = 1
		}},
		{name: "invalid_unknown_reason", mutate: func(bundle *traceBundleFile) {
			bundle.Artifacts[0].Perf.RawCaptureCompleteness.LostEvents = traceBundleRawPerfAggregateTotal{State: traceBundleRawPerfAggregateUnknown, Reason: "forged"}
		}},
		{name: "artifact_coverage_mismatch", mutate: func(bundle *traceBundleFile) { bundle.TraceCoverage[0].RawCaptureCompleteness.LostEvents.Value++ }},
		{name: "missing_coverage", mutate: func(bundle *traceBundleFile) { bundle.TraceCoverage = nil }},
		{name: "duplicate_coverage", mutate: func(bundle *traceBundleFile) {
			bundle.TraceCoverage = append(bundle.TraceCoverage, bundle.TraceCoverage[0])
		}},
		{name: "missing_receipt_census", mutate: func(bundle *traceBundleFile) {
			bundle.TraceCoverage[0].RawCaptureCompleteness = nil
		}},
		{name: "extra_raw_receipt_without_census", mutate: func(bundle *traceBundleFile) {
			extra := bundle.TraceCoverage[0]
			extra.RawCaptureCompleteness = nil
			bundle.TraceCoverage = append(bundle.TraceCoverage, extra)
		}},
		{name: "wrong_receipt_table", mutate: func(bundle *traceBundleFile) {
			bundle.TraceCoverage[0].Table = tracebundle.PerfReceiptTableSimpleperfText
		}},
		{name: "wrong_db_lane", mutate: func(bundle *traceBundleFile) {
			bundle.TraceDBCoverage = append(bundle.TraceDBCoverage, bundle.TraceCoverage[0])
			bundle.TraceCoverage = nil
		}},
		{name: "db_lane_raw_receipt_without_census", mutate: func(bundle *traceBundleFile) {
			coverage := bundle.TraceCoverage[0]
			coverage.RawCaptureCompleteness = nil
			bundle.TraceDBCoverage = append(bundle.TraceDBCoverage, coverage)
			bundle.TraceCoverage = nil
		}},
		{name: "db_lane_fuzzy_perf_receipt_without_census", mutate: func(bundle *traceBundleFile) {
			bundle.TraceDBCoverage = append(bundle.TraceDBCoverage, traceBundleCoverage{
				Family: "future", Table: "perftrace_future", Role: "future", ArtifactPath: "other.perftrace",
			})
		}},
		{name: "trace_lane_raw_table_wrong_family_without_census", mutate: func(bundle *traceBundleFile) {
			bundle.TraceCoverage = append(bundle.TraceCoverage, traceBundleCoverage{
				Family: "future", Table: tracebundle.PerfReceiptTableRawPerf,
				Role: tracebundle.PerfReceiptRole, ArtifactPath: "other.perftrace",
			})
		}},
		{name: "trace_lane_fuzzy_perf_receipt_without_census", mutate: func(bundle *traceBundleFile) {
			bundle.TraceCoverage = append(bundle.TraceCoverage, traceBundleCoverage{
				Family: "future", Table: "perftrace_future", Role: "future", ArtifactPath: "other.perftrace",
			})
		}},
		{name: "competing_closed_nonraw_receipt", mutate: func(bundle *traceBundleFile) {
			bundle.TraceCoverage = append(bundle.TraceCoverage, traceBundleCoverage{
				Family: tracebundle.PerfReceiptFamily, Table: tracebundle.PerfReceiptTableSimpleperfText,
				Role: tracebundle.PerfReceiptRole, ArtifactPath: "capture.perftrace", Found: true,
			})
		}},
		{name: "receipt_rows_read_drift", mutate: func(bundle *traceBundleFile) {
			bundle.TraceCoverage[0].RowsRead++
		}},
		{name: "cross_type", mutate: func(bundle *traceBundleFile) { bundle.Artifacts[0].Perf.ProviderKind = "official_android" }},
		{name: "readiness_mismatch", mutate: func(bundle *traceBundleFile) { bundle.Artifacts[0].Perf.TraceQueryReady = true }},
		{name: "missing_provider_decision", mutate: func(bundle *traceBundleFile) { bundle.ProviderDecisions = nil }},
		{name: "duplicate_provider_decision", mutate: func(bundle *traceBundleFile) {
			bundle.ProviderDecisions = append(bundle.ProviderDecisions, bundle.ProviderDecisions[0])
		}},
		{name: "provider_readiness_mismatch", mutate: func(bundle *traceBundleFile) {
			bundle.ProviderDecisions[0].TraceQueryReady = true
		}},
		{name: "failed_provider_does_not_claim", mutate: func(bundle *traceBundleFile) {
			bundle.ProviderDecisions[0].Succeeded = false
		}},
		{name: "failed_raw_decision_names_claim", mutate: func(bundle *traceBundleFile) {
			failed := bundle.ProviderDecisions[0]
			failed.Succeeded = false
			failed.TraceQueryReady = false
			failed.ArtifactPath = ""
			failed.Reason = "failed"
			bundle.ProviderDecisions = append(bundle.ProviderDecisions, failed)
		}},
		{name: "failed_raw_decision_bad_selected_attempted_state", mutate: func(bundle *traceBundleFile) {
			failed := bundle.ProviderDecisions[0]
			failed.Succeeded = false
			failed.TraceQueryReady = false
			failed.ArtifactPath = ""
			failed.OutputPath = "failed.perftrace"
			failed.Selected = false
			failed.Attempted = true
			failed.Reason = "raw_parser_error"
			bundle.ProviderDecisions = append(bundle.ProviderDecisions, failed)
		}},
		{name: "failed_raw_decision_bad_stage", mutate: func(bundle *traceBundleFile) {
			failed := bundle.ProviderDecisions[0]
			failed.Succeeded = false
			failed.TraceQueryReady = false
			failed.ArtifactPath = ""
			failed.OutputPath = "failed.perftrace"
			failed.Stage = "future_stage"
			failed.Reason = "raw_parser_error"
			bundle.ProviderDecisions = append(bundle.ProviderDecisions, failed)
		}},
		{name: "failed_raw_decision_bad_parser", mutate: func(bundle *traceBundleFile) {
			failed := bundle.ProviderDecisions[0]
			failed.Succeeded = false
			failed.TraceQueryReady = false
			failed.ArtifactPath = ""
			failed.OutputPath = "failed.perftrace"
			failed.ParserMode = " RAW "
			failed.Reason = "raw_parser_error"
			bundle.ProviderDecisions = append(bundle.ProviderDecisions, failed)
		}},
		{name: "failed_raw_decision_bad_reason", mutate: func(bundle *traceBundleFile) {
			failed := bundle.ProviderDecisions[0]
			failed.Succeeded = false
			failed.TraceQueryReady = false
			failed.ArtifactPath = ""
			failed.OutputPath = "failed.perftrace"
			failed.Reason = " raw_parser_error "
			bundle.ProviderDecisions = append(bundle.ProviderDecisions, failed)
		}},
		{name: "failed_raw_decision_provider_kind_drift", mutate: func(bundle *traceBundleFile) {
			failed := bundle.ProviderDecisions[0]
			failed.Succeeded = false
			failed.TraceQueryReady = false
			failed.ArtifactPath = ""
			failed.OutputPath = "failed.perftrace"
			failed.ProviderKind = "future_raw"
			failed.Reason = "raw_parser_error"
			bundle.ProviderDecisions = append(bundle.ProviderDecisions, failed)
		}},
		{name: "auto_without_fallback", mutate: func(bundle *traceBundleFile) {
			bundle.ProviderDecisions[0].ParserMode = "auto"
		}},
		{name: "raw_marked_fallback", mutate: func(bundle *traceBundleFile) {
			bundle.ProviderDecisions[0].Fallback = true
		}},
		{name: "provider_stage_drift", mutate: func(bundle *traceBundleFile) {
			bundle.ProviderDecisions[0].Stage = "future_stage"
		}},
		{name: "provider_output_path_drift", mutate: func(bundle *traceBundleFile) {
			bundle.ProviderDecisions[0].OutputPath = "other.perftrace"
		}},
		{name: "provider_success_reason_drift", mutate: func(bundle *traceBundleFile) {
			bundle.ProviderDecisions[0].Reason = "forged"
		}},
		{name: "inventory_clock_alignment_conflict", mutate: func(bundle *traceBundleFile) {
			bundle.PerfClockAlignments = []traceBundlePerfClockAlignment{{ArtifactPath: "capture.perftrace"}}
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle := r2aRawPerfBundle(validCapture, false)
			test.mutate(&bundle)
			caveats := traceBundleRawPerfCaptureCompletenessCaveats(bundle)
			if len(caveats) != 1 || !strings.Contains(caveats[0], "valid=false") ||
				!strings.Contains(caveats[0], "applicability=ignored") {
				t.Fatalf("invalid payload did not fail closed once: %v", caveats)
			}
			if strings.Contains(caveats[0], "8675309") || strings.Contains(caveats[0], "exact:") ||
				strings.Contains(caveats[0], "physical:") {
				t.Fatalf("invalid payload leaked declared numbers: %s", caveats[0])
			}
		})
	}
}

func TestRawPerfCaptureValidatorDoesNotDependOnPointerAliasing(t *testing.T) {
	capture := r2aRawPerfCapture()
	bundle := r2aRawPerfBundle(capture, false)
	if bundle.Artifacts[0].Perf.RawCaptureCompleteness == bundle.TraceCoverage[0].RawCaptureCompleteness {
		t.Fatal("fixture accidentally aliases artifact and receipt census pointers")
	}
	if caveats := traceBundleRawPerfCaptureCompletenessCaveats(bundle); len(caveats) != 1 || !strings.Contains(caveats[0], "valid=true") {
		t.Fatalf("independent equal census faces did not reconcile: %v", caveats)
	}

	aliased := r2aRawPerfBundle(capture, false)
	aliased.TraceCoverage[0].RawCaptureCompleteness = aliased.Artifacts[0].Perf.RawCaptureCompleteness
	if caveats := traceBundleRawPerfCaptureCompletenessCaveats(aliased); len(caveats) != 1 || !strings.Contains(caveats[0], "valid=true") {
		t.Fatalf("value reconciliation incorrectly depended on pointer inequality: %v", caveats)
	}
}

func TestRawPerfCaptureAcceptsAutoFallbackAndExplicitRawRoutes(t *testing.T) {
	for _, test := range []struct {
		name     string
		mode     string
		fallback bool
	}{
		{name: "explicit_raw", mode: "raw", fallback: false},
		{name: "explicit_fallback", mode: "fallback", fallback: false},
		{name: "auto_fallback", mode: "auto", fallback: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			bundle := r2aRawPerfBundle(r2aRawPerfCapture(), false)
			bundle.ProviderDecisions[0].ParserMode = test.mode
			bundle.ProviderDecisions[0].Fallback = test.fallback
			caveats := traceBundleRawPerfCaptureCompletenessCaveats(bundle)
			if len(caveats) != 1 || !strings.Contains(caveats[0], "valid=true") {
				t.Fatalf("closed raw success route rejected: %v", caveats)
			}
		})
	}
}

func TestRawPerfCaptureIgnoresCanonicalFailedOfficialProbe(t *testing.T) {
	bundle := r2aRawPerfBundle(r2aRawPerfCapture(), false)
	failedOfficial := traceBundleProviderDecision{
		Stage: "direct_input", ProviderKind: "official_android", ProviderName: "android_simpleperf_report_sample",
		InputPath: "capture.perf.data", InputFormat: "linux_perf_data", OutputPath: "capture.perftrace",
		ParserMode: "auto", Selected: true, Attempted: true, Reason: "official_adapter_failed",
		Caveat: "official provider failed before raw fallback",
	}
	bundle.ProviderDecisions = append([]traceBundleProviderDecision{failedOfficial}, bundle.ProviderDecisions...)
	if caveats := traceBundleRawPerfCaptureCompletenessCaveats(bundle); len(caveats) != 1 || !strings.Contains(caveats[0], "valid=true") {
		t.Fatalf("legitimate failed official probe was over-tightened: %v", caveats)
	}
}

func TestRawPerfCaptureAcceptsCanonicalUnrelatedRawFailure(t *testing.T) {
	bundle := r2aRawPerfBundle(r2aRawPerfCapture(), false)
	failedRaw := traceBundleProviderDecision{
		Stage: "standalone_hiperf", ProviderKind: "raw_fallback", ProviderName: "codrax_raw_perfdata",
		InputPath: "other.perf.data", InputFormat: "simpleperf_report_sample_proto", OutputPath: "other.perftrace",
		ParserMode: "fallback", Selected: true, Attempted: false, Reason: "unsupported_input_format",
		Caveat: "raw provider cannot parse proto",
	}
	bundle.ProviderDecisions = append(bundle.ProviderDecisions, failedRaw)
	if caveats := traceBundleRawPerfCaptureCompletenessCaveats(bundle); len(caveats) != 1 || !strings.Contains(caveats[0], "valid=true") {
		t.Fatalf("canonical unrelated raw failure was over-tightened: %v", caveats)
	}
}

func TestMalformedPerfReceiptNamespaceAloneFailsClosed(t *testing.T) {
	for _, bundle := range []traceBundleFile{
		{
			schemaMode:      traceBundleSchemaV2,
			TraceDBCoverage: []traceBundleCoverage{{Table: tracebundle.PerfReceiptTableSimpleperfText}},
		},
		{
			schemaMode:    traceBundleSchemaV2,
			TraceCoverage: []traceBundleCoverage{{Table: "perftrace_future"}},
		},
		{
			schemaMode: traceBundleSchemaV2,
			TraceCoverage: []traceBundleCoverage{{
				Family: "future", Table: tracebundle.PerfReceiptTableRawPerf,
				Role: tracebundle.PerfReceiptRole, ArtifactPath: "capture.perftrace",
			}},
		},
	} {
		caveats := traceBundleRawPerfCaptureCompletenessCaveats(bundle)
		if len(caveats) != 1 || !strings.Contains(caveats[0], "valid=false") {
			t.Fatalf("reserved/malformed perf receipt namespace escaped: %+v => %v", bundle, caveats)
		}
	}
	legitimateOfficial := traceBundleFile{
		schemaMode: traceBundleSchemaV2,
		TraceCoverage: []traceBundleCoverage{{
			Family: tracebundle.PerfReceiptFamily, Table: tracebundle.PerfReceiptTableSimpleperfText,
			Role: tracebundle.PerfReceiptRole, ArtifactPath: "capture.perftrace",
		}},
	}
	if caveats := traceBundleRawPerfCaptureCompletenessCaveats(legitimateOfficial); len(caveats) != 0 {
		t.Fatalf("legitimate closed official receipt incorrectly entered the raw advisory lane: %v", caveats)
	}
}

func TestRawPerfCaptureDoesNotInventProducerSuffixRequirement(t *testing.T) {
	for _, artifactPath := range []string{"capture.PERFTRACE", "capture.raw-output"} {
		bundle := r2aRawPerfBundle(r2aRawPerfCapture(), false)
		bundle.Artifacts[0].Path = artifactPath
		bundle.ProviderDecisions[0].ArtifactPath = artifactPath
		bundle.ProviderDecisions[0].OutputPath = artifactPath
		bundle.TraceCoverage[0].ArtifactPath = artifactPath
		if caveats := traceBundleRawPerfCaptureCompletenessCaveats(bundle); len(caveats) != 1 || !strings.Contains(caveats[0], "valid=true") {
			t.Fatalf("producer-compatible path %q rejected: %v", artifactPath, caveats)
		}
	}
}

func TestRawPerfCaptureSameBasenameArtifactsKeepDistinctGlobalSeats(t *testing.T) {
	retarget := func(bundle *traceBundleFile, wirePath string) {
		bundle.Artifacts[0].Path = wirePath
		bundle.ProviderDecisions[0].ArtifactPath = wirePath
		bundle.ProviderDecisions[0].OutputPath = wirePath
		bundle.TraceCoverage[0].ArtifactPath = wirePath
	}
	first := r2aRawPerfBundle(r2aRawPerfCapture(), false)
	second := r2aRawPerfBundle(r2aRawPerfCapture(), false)
	retarget(&first, "a/capture.perftrace")
	retarget(&second, "b/capture.perftrace")
	bundle := traceBundleFile{
		schemaMode:        traceBundleSchemaV2,
		Artifacts:         append(first.Artifacts, second.Artifacts...),
		ProviderDecisions: append(first.ProviderDecisions, second.ProviderDecisions...),
		TraceCoverage:     append(first.TraceCoverage, second.TraceCoverage...),
	}
	caveats := traceBundleCaveats(bundle)
	joined := strings.Join(caveats, "\n")
	if got := strings.Count(joined, RawPerfCaptureCompletenessCaveatToken+" "); got != 2 ||
		!strings.Contains(joined, "artifact=a%2Fcapture.perftrace") ||
		!strings.Contains(joined, "artifact=b%2Fcapture.perftrace") {
		t.Fatalf("same-basename raw artifacts collapsed their global advisory seats: count=%d\n%s", got, joined)
	}
}

func TestOwnedPerftraceHeaderLineParityWithProducer(t *testing.T) {
	body, err := os.ReadFile("../hitraceconv/render.go")
	if err != nil {
		t.Fatal(err)
	}
	const marker = "const systraceHeader = `"
	start := strings.Index(string(body), marker)
	if start < 0 {
		t.Fatal("producer systraceHeader literal not found")
	}
	rest := string(body)[start+len(marker):]
	end := strings.Index(rest, "`\n")
	if end < 0 {
		t.Fatal("producer systraceHeader literal is unterminated")
	}
	if got := strings.Count(rest[:end], "\n"); got != traceBundleOwnedPerftraceHeaderLines {
		t.Fatalf("raw receipt header-line closure drifted: consumer=%d producer=%d", traceBundleOwnedPerftraceHeaderLines, got)
	}
}

func TestInventoryPerfCapabilityPublishesDeclaredNotEffectiveAlignment(t *testing.T) {
	inventory := traceBundlePerfCapabilityCaveat("type=perftrace", traceBundlePerfCapability{
		TimeAlignment: "assumed", TraceQueryReady: false,
	})
	if !strings.Contains(inventory, "declared_time_alignment=assumed") || strings.Contains(inventory, " time_alignment=assumed") {
		t.Fatalf("inventory capability implied effective time alignment: %s", inventory)
	}
	ready := traceBundlePerfCapabilityCaveat("type=perftrace", traceBundlePerfCapability{
		TimeAlignment: "assumed", TraceQueryReady: true,
	})
	if !strings.Contains(ready, " time_alignment=assumed") || strings.Contains(ready, "declared_time_alignment=") {
		t.Fatalf("query-ready capability lost its effective alignment label: %s", ready)
	}
}

func TestRawPerfCaptureTrustedV2IsGlobalOutsideSampleWindow(t *testing.T) {
	dir := t.TempDir()
	perfPath := filepath.Join(dir, "capture.perftrace")
	bundlePath := filepath.Join(dir, "capture.tracebundle.json")
	if err := os.WriteFile(perfPath, []byte("# tracer: nop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	capture := r2aRawPerfCapture()
	capture.LostRecords = traceBundleRawPerfRecordCensus{Physical: 1, Accepted: 1}
	capture.LostEvents = traceBundleRawPerfAggregateTotal{State: traceBundleRawPerfAggregateExact, Value: 7}
	bundle := r2aRawPerfBundle(capture, false)
	legacy, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	writeTraceBundleV2ForTest(t, bundlePath, legacy)

	idx, err := BuildIndexWithOptions(context.Background(), bundlePath, BuildOptions{
		TimeStart: 100, TimeEnd: 101, TimeStartSet: true, TimeEndSet: true,
	})
	if err != nil {
		t.Fatalf("build trusted inventory bundle: %v", err)
	}
	result := Run(idx, Query{View: "event_search", TimeStart: 100, TimeEnd: 101, TimeStartSet: true, TimeEndSet: true})
	joined := strings.Join(result.Caveats, "\n")
	if strings.Count(joined, RawPerfCaptureCompletenessCaveatToken) != 1 ||
		!strings.Contains(joined, "lost_events=exact:7") || !strings.Contains(joined, "query_ready=false") ||
		len(result.Events) != 0 {
		t.Fatalf("global inventory disclosure/sample exclusion drifted: events=%d caveats=%v", len(result.Events), result.Caveats)
	}
	if result.PerfStats != nil || result.RootCauseRank != nil || result.WindowStats != nil && result.WindowStats.PerfSamples != nil {
		t.Fatalf("inventory-only census entered an analysis face: %+v", result)
	}
}

func TestRawPerfCaptureForgedV2MismatchIsIgnoredAndLegacyMetadataIsStripped(t *testing.T) {
	dir := t.TempDir()
	perfPath := filepath.Join(dir, "capture.perftrace")
	bundlePath := filepath.Join(dir, "capture.tracebundle.json")
	if err := os.WriteFile(perfPath, []byte("# tracer: nop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	capture := r2aRawPerfCapture()
	capture.LostRecords = traceBundleRawPerfRecordCensus{Physical: 1, Accepted: 1}
	capture.LostEvents = traceBundleRawPerfAggregateTotal{State: traceBundleRawPerfAggregateExact, Value: 424242}
	bundle := r2aRawPerfBundle(capture, false)
	legacy, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	v2 := traceBundleV2JSONForTest(t, bundlePath, legacy)
	var forged traceBundleFile
	if err := json.Unmarshal(v2, &forged); err != nil {
		t.Fatal(err)
	}
	forged.TraceCoverage[0].RawCaptureCompleteness.LostEvents.Value++
	forgedBody, err := json.Marshal(forged)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bundlePath, forgedBody, 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := BuildIndex(context.Background(), bundlePath)
	if err != nil {
		t.Fatalf("metadata-only forged census should remain an ignored advisory: %v", err)
	}
	joined := strings.Join(idx.Caveats, "\n")
	if strings.Count(joined, RawPerfCaptureCompletenessCaveatToken) != 1 ||
		!strings.Contains(joined, "valid=false") || strings.Contains(joined, "424242") {
		t.Fatalf("forged V2 census was not ignored without numbers: %s", joined)
	}

	// Legacy metadata is never bound to a perf child at all. A single systrace
	// legacy bundle strips the injected capability before traceBundleCaveats.
	systracePath := filepath.Join(dir, "legacy.systrace")
	legacyBundlePath := filepath.Join(dir, "legacy.tracebundle.json")
	if err := os.WriteFile(systracePath, []byte("# tracer: nop\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	legacyBody := `{
  "version":"legacy",
  "systrace":"legacy.systrace",
  "artifacts":[{
    "type":"systrace",
    "path":"legacy.systrace",
    "perf_capability":{"raw_perf_capture_completeness":{
      "profile":"raw_perf_record_census_v1",
      "source":"linux_perf_data_record_stream",
      "sample_records":{},
      "lost_records":{"physical":1,"accepted":1},
      "lost_sample_records":{},
      "aux_records":{},
      "lost_events":{"state":"exact","value":999999},
      "lost_samples":{"state":"not_reported","value":0},
      "aux_bytes":{"state":"not_reported","value":0}
    }}
  }]
}`
	if err := os.WriteFile(legacyBundlePath, []byte(legacyBody), 0o644); err != nil {
		t.Fatal(err)
	}
	legacyIdx, err := BuildIndex(context.Background(), legacyBundlePath)
	if err != nil {
		t.Fatalf("legacy single-child compatibility: %v", err)
	}
	legacyJoined := strings.Join(legacyIdx.Caveats, "\n")
	if strings.Contains(legacyJoined, RawPerfCaptureCompletenessCaveatToken) || strings.Contains(legacyJoined, "999999") ||
		!strings.Contains(legacyJoined, "tracebundle_legacy_unbound=true") {
		t.Fatalf("legacy metadata escaped its unbound boundary: %s", legacyJoined)
	}
}

func TestPerfBundleAdmissionCannotReadRawCaptureCompleteness(t *testing.T) {
	body, err := os.ReadFile("perf_bundle_admission.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"RawCaptureCompleteness",
		"raw_perf_capture_completeness",
		"traceBundleRawPerfCapture",
		traceBundleRawPerfCaptureProfile,
	} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("perf admission acquired raw census authority through %q", forbidden)
		}
	}
}

func TestRawPerfCaptureCaveatTokenHasSingleProductionAuthority(t *testing.T) {
	for _, path := range []string{
		"../tool/trace_query.go",
		"../tracediag/render.go",
		"../tracediag/render_key_first.go",
	} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(body), RawPerfCaptureCompletenessCaveatToken) {
			t.Fatalf("consumer %s copied the raw caveat token instead of using tracequery authority", path)
		}
	}
}
