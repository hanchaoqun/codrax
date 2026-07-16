package tracequery

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
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
	admission := traceBundleRawPerfSampleAdmission{
		Profile:    traceBundleRawPerfAdmissionProfile,
		Source:     traceBundleRawPerfAdmissionSource,
		Candidates: capture.SampleRecords.Accepted,
	}
	if ready {
		admission.QueryRows = admission.Candidates
	} else {
		admission.InventoryOnly = admission.Candidates
		admission.MissingTID = admission.InventoryOnly
	}
	artifactAdmission := admission
	coverageAdmission := admission
	bundle := traceBundleFile{
		schemaMode: traceBundleSchemaV2,
		Artifacts: []traceBundleArtifact{{
			Type: "perftrace", Path: "capture.perftrace", Converter: "hitraceconv-v1+raw-perfdata",
			Perf: &traceBundlePerfCapability{
				ProviderKind: "raw_fallback", ProviderName: "codrax_raw_perfdata",
				InputFormat: "linux_perf_data", OutputFormat: "codrax_perftrace",
				TimeDomain: "perf_data_time_ns", TimeAlignment: "assumed",
				ThreadIdentity: "present_valid_sample_pid_tid_only", CPUIdentity: "present_valid_sample_cpu_else_unknown",
				EventWeight: "present_valid_period_zero_as_sample_count", Symbolization: "hiperf_saved_symbols_or_unsymbolized_ip",
				Callchain: "symbolized_when_hiperf_files_symbol_present_else_ip_only",
				DSOLabel:  "mmap_best_effort", BuildID: "feature_build_id_when_present",
				OffCPU: "hiperf_cpu_off_sched_switch_when_event_desc_present", Confidence: "degraded",
				Degraded: true,
				Caveats: []string{
					"raw fallback resolves function names only from saved hiperf symbol sections; without those sections it remains IP/DSO-level",
					"raw fallback can label hiperf --offcpu sched_switch samples when official EVENT_DESC and HIPERF_CPU_OFF features are present, but full off-CPU stack expansion still needs official hiperf report flow",
					"structurally parsed samples without required time, thread identity, or period remain receipt-bound inventory and never receive synthesized coordinates or weight",
				},
				TraceQueryReady: ready, RawCaptureCompleteness: &artifactCapture,
				RawSampleAdmission: &artifactAdmission,
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
			Found: true, RowsRead: traceBundleOwnedPerftraceHeaderLines + int(admission.QueryRows),
			RowsEmitted: int(admission.QueryRows), RawCaptureCompleteness: &coverageCapture,
			RawSampleAdmission: &coverageAdmission,
		}},
	}
	if canonical, present := traceBundleRawPerfAdmissionCaveat(admission); present {
		bundle.Artifacts[0].Caveats = append([]string{canonical}, bundle.Artifacts[0].Caveats...)
	}
	return bundle
}

func r2cRawPerfResidual(throttle, unthrottle uint64) traceBundleRawPerfCaptureResidual {
	return traceBundleRawPerfCaptureResidual{
		Profile:           traceBundleRawPerfResidualProfile,
		Source:            traceBundleRawPerfResidualSource,
		ThrottleRecords:   throttle,
		UnthrottleRecords: unthrottle,
	}
}

func r2cAttachRawPerfResidual(bundle *traceBundleFile, residual traceBundleRawPerfCaptureResidual) {
	artifactResidual := residual
	coverageResidual := residual
	bundle.Artifacts[0].Perf.RawCaptureResidual = &artifactResidual
	bundle.TraceCoverage[0].RawCaptureResidual = &coverageResidual
	if canonical, present := traceBundleRawPerfResidualCaveat(residual); present {
		bundle.Artifacts[0].Caveats = append([]string{canonical}, bundle.Artifacts[0].Caveats...)
	}
}

func r2cReadyRawPerfBundle() traceBundleFile {
	capture := r2aRawPerfCapture()
	capture.SampleRecords = traceBundleRawPerfRecordCensus{Physical: 1, Accepted: 1}
	return r2aRawPerfBundle(capture, true)
}

func TestRawPerfCaptureWireMirrorsHaveClosedTopLevelFields(t *testing.T) {
	tests := []struct {
		name string
		wire any
		want []string
	}{
		{
			name: "census_v1",
			wire: traceBundleRawPerfCaptureCompleteness{},
			want: []string{
				"aux_bytes", "aux_records", "lost_events", "lost_records",
				"lost_sample_records", "lost_samples", "profile", "sample_records", "source",
			},
		},
		{
			name: "header_residual_v1",
			wire: traceBundleRawPerfCaptureResidual{},
			want: []string{"profile", "source", "throttle_records", "unthrottle_records"},
		},
		{
			name: "sample_admission_v1",
			wire: traceBundleRawPerfSampleAdmission{},
			want: []string{
				"candidates", "invalid_cpu", "invalid_identity", "invalid_period",
				"inventory_only", "missing_period", "missing_tid", "missing_time",
				"profile", "query_rows", "source",
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body, err := json.Marshal(test.wire)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(body, &fields); err != nil {
				t.Fatal(err)
			}
			got := make([]string, 0, len(fields))
			for name := range fields {
				got = append(got, name)
			}
			sort.Strings(got)
			want := append([]string(nil), test.want...)
			sort.Strings(want)
			if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
				t.Fatalf("%s field set drifted: got=%v want=%v wire=%s", test.name, got, want, body)
			}
		})
	}
}

func TestRawPerfSampleAdmissionPublishesOneReceiptBoundAdvisory(t *testing.T) {
	bundle := r2aRawPerfBundle(traceBundleRawPerfCaptureCompleteness{
		Profile:       traceBundleRawPerfCaptureProfile,
		Source:        traceBundleRawPerfCaptureSource,
		SampleRecords: traceBundleRawPerfRecordCensus{Physical: 2, Accepted: 2},
		LostEvents:    traceBundleRawPerfAggregateTotal{State: traceBundleRawPerfAggregateNotReported},
		LostSamples:   traceBundleRawPerfAggregateTotal{State: traceBundleRawPerfAggregateNotReported},
		AuxBytes:      traceBundleRawPerfAggregateTotal{State: traceBundleRawPerfAggregateNotReported},
	}, false)

	joined := strings.Join(traceBundleCaveats(bundle), "\n")
	for _, want := range []string{
		"valid=true",
		"query_ready=false",
		"capture_state=inventory_only",
		"capture_quality_issue=true",
		"sample_admission_candidates=2",
		"sample_admission_query_rows=0",
		"sample_admission_inventory_only=2",
		"sample_admission_missing_tid=2",
		"thread_attribution=none",
		"root_cause_rank=none",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("admission advisory missing %q:\n%s", want, joined)
		}
	}
	if got := strings.Count(joined, RawPerfCaptureCompletenessCaveatToken+" "); got != 1 {
		t.Fatalf("typed advisory cardinality=%d:\n%s", got, joined)
	}
	if strings.Contains(joined, traceBundleRawPerfAdmissionCaveatToken+" ") {
		t.Fatalf("reserved Artifact admission caveat escaped its typed projection:\n%s", joined)
	}
}

func TestRawPerfSampleAdmissionMissingTamperedAndCrossFaceMismatchFailClosed(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*traceBundleFile)
		want   string
	}{
		{
			name: "artifact_missing",
			mutate: func(bundle *traceBundleFile) {
				bundle.Artifacts[0].Perf.RawSampleAdmission = nil
			},
			want: "reason=raw_artifact_missing_sample_admission",
		},
		{
			name: "receipt_missing",
			mutate: func(bundle *traceBundleFile) {
				bundle.TraceCoverage[0].RawSampleAdmission = nil
			},
			want: "reason=raw_receipt_missing_sample_admission",
		},
		{
			name: "artifact_reason_census_tampered",
			mutate: func(bundle *traceBundleFile) {
				bundle.Artifacts[0].Perf.RawSampleAdmission.MissingTime = 1
			},
			want: "reason=sample_admission_missing_time_exceeds_inventory",
		},
		{
			name: "artifact_coverage_mismatch",
			mutate: func(bundle *traceBundleFile) {
				bundle.TraceCoverage[0].RawSampleAdmission.QueryRows = 0
				bundle.TraceCoverage[0].RawSampleAdmission.InventoryOnly = 1
				bundle.TraceCoverage[0].RawSampleAdmission.MissingTID = 1
			},
			want: "reason=artifact_coverage_admission_mismatch",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle := r2cReadyRawPerfBundle()
			test.mutate(&bundle)
			joined := strings.Join(traceBundleCaveats(bundle), "\n")
			if !strings.Contains(joined, "valid=false") || !strings.Contains(joined, test.want) {
				t.Fatalf("tampered admission did not fail closed with %q:\n%s", test.want, joined)
			}
			if strings.Contains(joined, "valid=true") {
				t.Fatalf("tampered admission retained valid projection:\n%s", joined)
			}
		})
	}
}

func TestRawPerfSampleAdmissionArtifactCaveatIsCanonicalAndExact(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(*traceBundleFile)
		want   string
	}{
		{
			name: "missing",
			mutate: func(bundle *traceBundleFile) {
				bundle.Artifacts[0].Caveats = bundle.Artifacts[0].Caveats[1:]
			},
			want: "reason=raw_sample_admission_artifact_caveat_count_mismatch",
		},
		{
			name: "tampered",
			mutate: func(bundle *traceBundleFile) {
				bundle.Artifacts[0].Caveats[0] += " forged=true"
			},
			want: "reason=raw_sample_admission_artifact_caveat_mismatch",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			capture := r2aRawPerfCapture()
			capture.SampleRecords = traceBundleRawPerfRecordCensus{Physical: 1, Accepted: 1}
			bundle := r2aRawPerfBundle(capture, false)
			test.mutate(&bundle)
			joined := strings.Join(traceBundleCaveats(bundle), "\n")
			if !strings.Contains(joined, "valid=false") || !strings.Contains(joined, test.want) {
				t.Fatalf("non-canonical admission caveat did not fail closed with %q:\n%s", test.want, joined)
			}
		})
	}
}

func TestRawPerfSampleAdmissionMalformedFacesFailClosedWithoutNumericLeak(t *testing.T) {
	const sentinel = uint64(771234567)
	forgedAdmission := traceBundleRawPerfSampleAdmission{
		Profile:       traceBundleRawPerfAdmissionProfile,
		Source:        traceBundleRawPerfAdmissionSource,
		Candidates:    sentinel,
		InventoryOnly: sentinel,
		MissingTime:   sentinel,
	}
	canonical, present := traceBundleRawPerfAdmissionCaveat(forgedAdmission)
	if !present {
		t.Fatal("valid nonquery admission did not produce a canonical compatibility caveat")
	}

	tests := []struct {
		name   string
		mutate func(*traceBundleFile)
	}{
		{name: "artifact_typed_counter", mutate: func(bundle *traceBundleFile) {
			bundle.Artifacts[0].Perf.RawSampleAdmission.InvalidCPU = sentinel
		}},
		{name: "coverage_typed_counter", mutate: func(bundle *traceBundleFile) {
			bundle.TraceCoverage[0].RawSampleAdmission.InvalidCPU = sentinel
		}},
		{name: "bundle_caveat", mutate: func(bundle *traceBundleFile) {
			bundle.Caveats = append(bundle.Caveats, canonical)
		}},
		{name: "artifact_perf_capability_caveat", mutate: func(bundle *traceBundleFile) {
			bundle.Artifacts[0].Perf.Caveats = append(bundle.Artifacts[0].Perf.Caveats, canonical)
		}},
		{name: "provider_decision_caveat", mutate: func(bundle *traceBundleFile) {
			bundle.ProviderDecisions[0].Caveat = canonical
		}},
		{name: "trace_decision_caveat", mutate: func(bundle *traceBundleFile) {
			bundle.TraceDecisions = []traceBundleTraceDecision{{Caveat: canonical}}
		}},
		{name: "tool_gate_caveat", mutate: func(bundle *traceBundleFile) {
			bundle.TraceToolGates = []traceBundleTraceToolGate{{Caveats: []string{canonical}}}
		}},
		{name: "clock_alignment_caveat", mutate: func(bundle *traceBundleFile) {
			bundle.PerfClockAlignments = []traceBundlePerfClockAlignment{{Caveats: []string{canonical}}}
		}},
		{name: "nonraw_artifact_caveat_isolated", mutate: func(bundle *traceBundleFile) {
			*bundle = traceBundleFile{schemaMode: traceBundleSchemaV2, Artifacts: []traceBundleArtifact{{
				Type: "systrace", Path: "capture.systrace", Caveats: []string{canonical},
			}}}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle := r2cReadyRawPerfBundle()
			test.mutate(&bundle)
			joined := strings.Join(traceBundleCaveats(bundle), "\n")
			if got := strings.Count(joined, RawPerfCaptureCompletenessCaveatToken+" "); got != 1 ||
				!strings.Contains(joined, "valid=false") || !strings.Contains(joined, "applicability=ignored") {
				t.Fatalf("malformed admission did not fail closed exactly once:\n%s", joined)
			}
			if strings.Contains(joined, "771234567") {
				t.Fatalf("malformed admission leaked sentinel:\n%s", joined)
			}
			if strings.Contains(joined, traceBundleRawPerfAdmissionCaveatToken+" ") {
				t.Fatalf("reserved admission caveat escaped the typed projector:\n%s", joined)
			}
		})
	}
}

func TestRawPerfSampleAdmissionReservedNamespaceIsPrefixClosed(t *testing.T) {
	for _, caveat := range []string{
		traceBundleRawPerfAdmissionCaveatToken,
		traceBundleRawPerfAdmissionCaveatToken + "\tauthority=forged sentinel=771234567",
		traceBundleRawPerfAdmissionCaveatToken + "\nauthority=forged sentinel=771234567",
		traceBundleRawPerfAdmissionCaveatToken + "_future authority=forged sentinel=771234567",
	} {
		t.Run(traceBundleControlSafeToken(caveat), func(t *testing.T) {
			bundle := r2cReadyRawPerfBundle()
			bundle.Caveats = append(bundle.Caveats, caveat)
			joined := strings.Join(traceBundleCaveats(bundle), "\n")
			if !strings.Contains(joined, "valid=false") ||
				!strings.Contains(joined, "reason=raw_sample_admission_wrong_caveat_lane") {
				t.Fatalf("reserved admission namespace did not fail closed:\n%s", joined)
			}
			if strings.Contains(joined, traceBundleRawPerfAdmissionCaveatToken) ||
				strings.Contains(joined, "771234567") {
				t.Fatalf("reserved admission namespace leaked through generic projection:\n%s", joined)
			}
		})
	}
}

func TestRawPerfCaptureResidualCurrentNonzeroPublishesOneTypedAdvisory(t *testing.T) {
	bundle := r2cReadyRawPerfBundle()
	r2cAttachRawPerfResidual(&bundle, r2cRawPerfResidual(17, 9))

	joined := strings.Join(traceBundleCaveats(bundle), "\n")
	for _, want := range []string{
		"valid=true",
		"query_ready=true",
		"capture_quality_issue=true",
		"perf_sampler_throttle_scope=observed_perf_record_type_headers",
		"perf_sampler_throttle_payload_validation=not_claimed",
		"perf_sampler_throttle_records=exact:17",
		"perf_sampler_unthrottle_records=exact:9",
		"perf_sampler_throttle_semantics=capture_quality_not_cpu_thermal",
		"perf_sampler_throttle_duration=not_reported",
		"perf_sampler_throttle_lost_samples=not_reported",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("current residual advisory missing %q:\n%s", want, joined)
		}
	}
	if got := strings.Count(joined, RawPerfCaptureCompletenessCaveatToken+" "); got != 1 {
		t.Fatalf("current residual advisory cardinality=%d:\n%s", got, joined)
	}
	if got := strings.Count(joined, "perf_sampler_throttle_records=exact:17"); got != 1 {
		t.Fatalf("typed throttle count published %d times:\n%s", got, joined)
	}
	if strings.Contains(joined, traceBundleRawPerfResidualCaveatToken+" ") {
		t.Fatalf("reserved Artifact caveat escaped its typed projection:\n%s", joined)
	}
}

func TestRawPerfCaptureResidualCurrentExactZeroAndOldV2RemainDistinct(t *testing.T) {
	t.Run("current_exact_zero", func(t *testing.T) {
		bundle := r2cReadyRawPerfBundle()
		r2cAttachRawPerfResidual(&bundle, r2cRawPerfResidual(0, 0))
		joined := strings.Join(traceBundleCaveats(bundle), "\n")
		for _, want := range []string{
			"valid=true",
			"perf_sampler_throttle_records=exact:0",
			"perf_sampler_unthrottle_records=exact:0",
		} {
			if !strings.Contains(joined, want) {
				t.Fatalf("current exact-zero residual missing %q:\n%s", want, joined)
			}
		}
		if strings.Contains(joined, traceBundleRawPerfResidualCaveatToken) ||
			strings.Contains(joined, "perf_sampler_throttle_records=not_reported") {
			t.Fatalf("current exact zero collapsed into an absent/legacy face:\n%s", joined)
		}
	})

	t.Run("old_v2_not_reported", func(t *testing.T) {
		joined := strings.Join(traceBundleCaveats(r2cReadyRawPerfBundle()), "\n")
		for _, want := range []string{
			"valid=true",
			"perf_sampler_throttle_scope=observed_perf_record_type_headers",
			"perf_sampler_throttle_payload_validation=not_claimed",
			"perf_sampler_throttle_records=not_reported",
			"perf_sampler_unthrottle_records=not_reported",
		} {
			if !strings.Contains(joined, want) {
				t.Fatalf("old V2 compatibility missing %q:\n%s", want, joined)
			}
		}
		if strings.Contains(joined, "perf_sampler_throttle_records=exact:") {
			t.Fatalf("old V2 absence was forged into an exact zero:\n%s", joined)
		}
	})
}

func TestRawPerfCaptureResidualMalformedFacesFailClosedWithoutNumericLeak(t *testing.T) {
	const (
		throttleSentinel   = uint64(771234567)
		unthrottleSentinel = uint64(882345678)
	)
	residual := r2cRawPerfResidual(throttleSentinel, unthrottleSentinel)
	canonical, ok := traceBundleRawPerfResidualCaveat(residual)
	if !ok {
		t.Fatal("nonzero residual did not produce its canonical compatibility caveat")
	}

	tests := []struct {
		name   string
		mutate func(*traceBundleFile)
	}{
		{name: "artifact_typed_only", mutate: func(bundle *traceBundleFile) {
			bundle.TraceCoverage[0].RawCaptureResidual = nil
		}},
		{name: "coverage_typed_only", mutate: func(bundle *traceBundleFile) {
			bundle.Artifacts[0].Perf.RawCaptureResidual = nil
			bundle.Artifacts[0].Caveats = nil
		}},
		{name: "typed_value_mismatch", mutate: func(bundle *traceBundleFile) {
			bundle.TraceCoverage[0].RawCaptureResidual.UnthrottleRecords++
		}},
		{name: "canonical_missing", mutate: func(bundle *traceBundleFile) {
			bundle.Artifacts[0].Caveats = nil
		}},
		{name: "canonical_edited", mutate: func(bundle *traceBundleFile) {
			bundle.Artifacts[0].Caveats[0] += " forged"
		}},
		{name: "canonical_duplicate", mutate: func(bundle *traceBundleFile) {
			bundle.Artifacts[0].Caveats = append(bundle.Artifacts[0].Caveats, bundle.Artifacts[0].Caveats[0])
		}},
		{name: "canonical_in_perf_capability_caveats", mutate: func(bundle *traceBundleFile) {
			bundle.Artifacts[0].Perf.Caveats = append(bundle.Artifacts[0].Perf.Caveats, canonical)
		}},
		{name: "canonical_in_bundle_caveats", mutate: func(bundle *traceBundleFile) {
			bundle.Caveats = append(bundle.Caveats, canonical)
		}},
		{name: "canonical_in_provider_decision_caveat", mutate: func(bundle *traceBundleFile) {
			bundle.ProviderDecisions[0].Caveat = canonical
		}},
		{name: "canonical_in_trace_decision_caveat", mutate: func(bundle *traceBundleFile) {
			bundle.TraceDecisions = []traceBundleTraceDecision{{Caveat: canonical}}
		}},
		{name: "canonical_in_tool_gate_caveats", mutate: func(bundle *traceBundleFile) {
			bundle.TraceToolGates = []traceBundleTraceToolGate{{Caveats: []string{canonical}}}
		}},
		{name: "canonical_in_clock_alignment_caveats", mutate: func(bundle *traceBundleFile) {
			bundle.PerfClockAlignments = []traceBundlePerfClockAlignment{{Caveats: []string{canonical}}}
		}},
		{name: "canonical_only_raw", mutate: func(bundle *traceBundleFile) {
			bundle.Artifacts[0].Perf.RawCaptureResidual = nil
			bundle.TraceCoverage[0].RawCaptureResidual = nil
		}},
		{name: "invalid_artifact_profile", mutate: func(bundle *traceBundleFile) {
			bundle.Artifacts[0].Perf.RawCaptureResidual.Profile = "forged"
		}},
		{name: "invalid_coverage_source", mutate: func(bundle *traceBundleFile) {
			bundle.TraceCoverage[0].RawCaptureResidual.Source = "forged"
		}},
		{name: "zero_with_nonzero_canonical", mutate: func(bundle *traceBundleFile) {
			zeroArtifact := r2cRawPerfResidual(0, 0)
			zeroCoverage := zeroArtifact
			bundle.Artifacts[0].Perf.RawCaptureResidual = &zeroArtifact
			bundle.TraceCoverage[0].RawCaptureResidual = &zeroCoverage
		}},
		{name: "canonical_only_nonraw_isolated", mutate: func(bundle *traceBundleFile) {
			*bundle = traceBundleFile{schemaMode: traceBundleSchemaV2, Artifacts: []traceBundleArtifact{{
				Type: "systrace", Path: "capture.systrace", Caveats: []string{canonical},
			}}}
		}},
		{name: "typed_nonraw_artifact_isolated", mutate: func(bundle *traceBundleFile) {
			artifactResidual := residual
			*bundle = traceBundleFile{schemaMode: traceBundleSchemaV2, Artifacts: []traceBundleArtifact{{
				Type: "systrace", Path: "capture.systrace",
				Perf: &traceBundlePerfCapability{RawCaptureResidual: &artifactResidual},
			}}}
		}},
		{name: "canonical_in_nonraw_perf_capability_isolated", mutate: func(bundle *traceBundleFile) {
			*bundle = traceBundleFile{schemaMode: traceBundleSchemaV2, Artifacts: []traceBundleArtifact{{
				Type: "perfdata", Path: "capture.perf.data",
				Perf: &traceBundlePerfCapability{Caveats: []string{"ordinary-capability-caveat", canonical}},
			}}}
		}},
		{name: "typed_db_coverage_isolated", mutate: func(bundle *traceBundleFile) {
			coverageResidual := residual
			*bundle = traceBundleFile{schemaMode: traceBundleSchemaV2, TraceDBCoverage: []traceBundleCoverage{{
				Family: "future", Table: "future", RawCaptureResidual: &coverageResidual,
			}}}
		}},
		{name: "typed_trace_coverage_isolated", mutate: func(bundle *traceBundleFile) {
			coverageResidual := residual
			*bundle = traceBundleFile{schemaMode: traceBundleSchemaV2, TraceCoverage: []traceBundleCoverage{{
				Family: "future", Table: "future", RawCaptureResidual: &coverageResidual,
			}}}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			bundle := r2cReadyRawPerfBundle()
			r2cAttachRawPerfResidual(&bundle, residual)
			test.mutate(&bundle)

			joined := strings.Join(traceBundleCaveats(bundle), "\n")
			if got := strings.Count(joined, RawPerfCaptureCompletenessCaveatToken+" "); got != 1 ||
				!strings.Contains(joined, "valid=false") || !strings.Contains(joined, "applicability=ignored") {
				t.Fatalf("malformed residual did not fail closed exactly once:\n%s", joined)
			}
			for _, forbidden := range []string{"771234567", "882345678", "882345679"} {
				if strings.Contains(joined, forbidden) {
					t.Fatalf("malformed residual leaked sentinel %s:\n%s", forbidden, joined)
				}
			}
			if strings.Contains(joined, traceBundleRawPerfResidualCaveatToken+" ") {
				t.Fatalf("malformed reserved caveat escaped the typed projector:\n%s", joined)
			}
		})
	}
}

func TestRawPerfCaptureResidualReservedFilterPreservesAdjacentArtifactCaveat(t *testing.T) {
	bundle := r2cReadyRawPerfBundle()
	r2cAttachRawPerfResidual(&bundle, r2cRawPerfResidual(3, 2))
	bundle.Artifacts[0].Caveats = append(bundle.Artifacts[0].Caveats, "ordinary-neighbor-caveat")

	joined := strings.Join(traceBundleCaveats(bundle), "\n")
	if strings.Contains(joined, traceBundleRawPerfResidualCaveatToken+" ") {
		t.Fatalf("reserved compatibility twin escaped generic filtering:\n%s", joined)
	}
	if got := strings.Count(joined, "ordinary-neighbor-caveat"); got != 1 {
		t.Fatalf("adjacent ordinary Artifact caveat count=%d:\n%s", got, joined)
	}
	if got := strings.Count(joined, "perf_sampler_throttle_records=exact:3"); got != 1 {
		t.Fatalf("typed residual projection count=%d:\n%s", got, joined)
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
		"RawCaptureResidual",
		"raw_perf_capture_residual",
		"traceBundleRawPerfResidual",
		traceBundleRawPerfResidualProfile,
		"RawSampleAdmission",
		"raw_perf_sample_admission",
		"traceBundleRawPerfAdmission",
		traceBundleRawPerfAdmissionProfile,
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
