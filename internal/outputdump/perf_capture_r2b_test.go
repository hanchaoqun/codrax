package outputdump

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

const r2cRawPerfResidualCaveat = "raw_perf_capture_residual authority=artifact_receipt_advisory capture_hard_gate=false scope=observed_perf_record_type_headers payload_validation=not_claimed interpretation=perf_sampling_control_not_cpu_thermal no_duration_or_lost_sample_count=true perf_sampler_throttle_records=11 perf_sampler_unthrottle_records=13"

func r2bRawPerfArtifact(path string, samples uint64, qualityIssue bool) hitraceconv.Artifact {
	capture := hitraceconv.RawPerfCaptureCompleteness{
		Profile: "raw_perf_record_census_v1", Source: "linux_perf_data_record_stream",
		SampleRecords: hitraceconv.RawPerfRecordCensus{Physical: samples, Accepted: samples},
		LostEvents:    hitraceconv.RawPerfAggregateTotal{State: "not_reported"},
		LostSamples:   hitraceconv.RawPerfAggregateTotal{State: "not_reported"},
		AuxBytes:      hitraceconv.RawPerfAggregateTotal{State: "not_reported"},
	}
	if qualityIssue {
		capture.LostRecords = hitraceconv.RawPerfRecordCensus{Physical: 1, Accepted: 1}
		capture.LostEvents = hitraceconv.RawPerfAggregateTotal{State: "exact", Value: 3}
	}
	return hitraceconv.Artifact{
		Type: "perftrace", Path: path, Converter: "hitraceconv-v1+raw-perfdata",
		Perf: &hitraceconv.PerfArtifactCapability{
			ProviderKind: "raw_fallback", ProviderName: "codrax_raw_perfdata",
			InputFormat: "linux_perf_data", OutputFormat: "codrax_perftrace",
			TimeDomain: "perf_data_time_ns", TimeAlignment: "assumed",
			ThreadIdentity: "pid_tid_from_sample_or_comm", CPUIdentity: "sample_cpu_when_recorded",
			EventWeight: "period_or_1", Symbolization: "hiperf_saved_symbols_or_unsymbolized_ip",
			Callchain: "symbolized_when_hiperf_files_symbol_present_else_ip_only",
			DSOLabel:  "mmap_best_effort", BuildID: "feature_build_id_when_present",
			OffCPU: "hiperf_cpu_off_sched_switch_when_event_desc_present", Confidence: "degraded",
			TraceQueryReady: samples > 0, Degraded: true,
			RawCaptureCompleteness: &capture,
			RawCaptureResidual: &hitraceconv.RawPerfCaptureResidual{
				Profile: "raw_perf_record_header_residual_v1",
				Source:  "linux_perf_data_record_headers",
			},
			Caveats: []string{
				"raw fallback resolves function names only from saved hiperf symbol sections; without those sections it remains IP/DSO-level",
				"raw fallback can label hiperf --offcpu sched_switch samples when official EVENT_DESC and HIPERF_CPU_OFF features are present, but full off-CPU stack expansion still needs official hiperf report flow",
			},
		},
	}
}

func r2bRuntimeArtifacts(t *testing.T, artifacts ...hitraceconv.Artifact) []RuntimeArtifact {
	t.Helper()
	body, err := json.Marshal(struct {
		Version   string                 `json:"version"`
		Artifacts []hitraceconv.Artifact `json:"artifacts"`
	}{Version: "hitraceconv-v1", Artifacts: artifacts})
	if err != nil {
		t.Fatal(err)
	}
	return RuntimeArtifactsFromAttachment("trace", "# codrax-source: capture.tracebundle.json\n"+string(body))
}

func TestDirectPerftraceUsesNeutralArtifactLabel(t *testing.T) {
	artifacts := RuntimeArtifactsFromAttachment(
		"trace",
		"# codrax-source: capture.perftrace\n# tracer: nop\n",
	)
	body := BuildBody(Args{
		Request: "inspect the attachment", Answer: "answer", RuntimeArtifacts: artifacts,
	})
	if !strings.Contains(body, "perftrace text artifact") {
		t.Fatalf("direct perftrace lost its neutral artifact label:\n%s", body)
	}
	if strings.Contains(body, "perf sample text") {
		t.Fatalf("direct perftrace was mislabeled as sample-bearing:\n%s", body)
	}
}

func TestOutputDumpRawPerfInventoryIsNeutralAndNonCausal(t *testing.T) {
	inventory := r2bRawPerfArtifact("inventory.perftrace", 0, true)
	body := BuildBody(Args{
		Request: "inspect", Answer: "answer", RuntimeArtifacts: r2bRuntimeArtifacts(t, inventory),
	})
	for _, want := range []string{
		"perftrace text artifact", "capture_state=inventory_only", "query_ready=false",
		"manifest_disclosure_only=true", "validation=trace_query_v2_three_faces_required",
		"analysis_use=capture_quality_diagnostics_only", "effective_clock_evidence=none",
		"sample_aggregation=none", "clock_alignment=none", "thread_attribution=none", "root_cause_rank=none",
		"转换成功（质量库存），不可查询", "建议重新采集",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("inventory output missing %q:\n%s", want, body)
		}
	}
	for _, banned := range []string{"perf sample text", "root_cause_rank=1", "effective_clock_evidence=assumed"} {
		if strings.Contains(body, banned) {
			t.Fatalf("inventory output acquired causal/sample authority %q:\n%s", banned, body)
		}
	}
}

func TestOutputDumpReadyQualityKeepsSamplesButQualifiesClockAndAbsence(t *testing.T) {
	ready := r2bRawPerfArtifact("ready.perftrace", 2, true)
	body := BuildBody(Args{
		Request: "inspect", Answer: "answer", RuntimeArtifacts: r2bRuntimeArtifacts(t, ready),
	})
	for _, want := range []string{
		"capture_state=query_ready_with_quality_issue", "query_ready=true", "capture_quality_issue=true",
		"analysis_use=queryable_samples_with_capture_quality_caveat",
		"absence_policy=require_capture_quality_caveat",
		"effective_clock_evidence=consult_bundle_perf_clock_alignments",
		"可查询正样本", "缺失/不存在结论须附采集质量说明",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("ready-quality output missing %q:\n%s", want, body)
		}
	}
}

func TestOutputDumpRawPerfResidualUsesTypedDisplaySeatOnce(t *testing.T) {
	ready := r2bRawPerfArtifact("ready.perftrace", 2, true)
	ready.Perf.RawCaptureResidual.ThrottleRecords = 11
	ready.Perf.RawCaptureResidual.UnthrottleRecords = 13
	ready.Caveats = []string{r2cRawPerfResidualCaveat, "ordinary-display-caveat"}
	body := BuildBody(Args{
		Language: "en", Request: "inspect", Answer: "answer", RuntimeArtifacts: r2bRuntimeArtifacts(t, ready),
	})
	for _, exact := range []string{
		"perf_sampler_throttle_scope=observed_perf_record_type_headers",
		"perf_sampler_throttle_payload_validation=not_claimed",
		"perf_sampler_throttle_records=exact:11",
		"perf_sampler_unthrottle_records=exact:13",
	} {
		if count := strings.Count(body, exact); count != 1 {
			t.Fatalf("outputdump residual seat count for %q=%d want=1:\n%s", exact, count, body)
		}
	}
	if strings.Contains(body, "raw_perf_capture_residual") {
		t.Fatalf("outputdump leaked the compatibility mirror beside typed display:\n%s", body)
	}
	if !strings.Contains(body, "ordinary-display-caveat") {
		t.Fatalf("outputdump dropped an unrelated artifact caveat:\n%s", body)
	}
}

func TestOutputDumpInvalidAndCrossTypeResidualMirrorsLeakNoNumbers(t *testing.T) {
	invalid := r2bRawPerfArtifact("invalid.perftrace", 1, false)
	invalid.Perf.RawCaptureResidual.ThrottleRecords = 41
	invalid.Perf.RawCaptureResidual.UnthrottleRecords = 43
	invalid.Caveats = []string{
		"raw_perf_capture_residual authority=forged perf_sampler_throttle_records=8675309 perf_sampler_unthrottle_records=8675311",
	}
	invalidBody := BuildBody(Args{
		Language: "en", Request: "inspect", Answer: "answer", RuntimeArtifacts: r2bRuntimeArtifacts(t, invalid),
	})
	for _, want := range []string{"raw_perf_capture valid=false", "reason=capture_residual_invalid"} {
		if !strings.Contains(invalidBody, want) {
			t.Fatalf("invalid residual output missing %q:\n%s", want, invalidBody)
		}
	}
	for _, leaked := range []string{
		"raw_perf_capture_residual", "8675309", "8675311",
		"perf_sampler_throttle_records=exact:41", "perf_sampler_unthrottle_records=exact:43",
	} {
		if strings.Contains(invalidBody, leaked) {
			t.Fatalf("invalid residual output leaked %q:\n%s", leaked, invalidBody)
		}
	}

	crossType := hitraceconv.Artifact{
		Type: "perfdata", Path: "cross.perf.data",
		Caveats: []string{
			"ordinary-cross-type-caveat",
			"raw_perf_capture_residual forged=99887766",
		},
	}
	crossTypeBody := BuildBody(Args{
		Language: "en", Request: "inspect", Answer: "answer", RuntimeArtifacts: r2bRuntimeArtifacts(t, crossType),
	})
	if strings.Contains(crossTypeBody, "raw_perf_capture_residual") || strings.Contains(crossTypeBody, "99887766") {
		t.Fatalf("cross-type residual mirror leaked through generic artifact detail:\n%s", crossTypeBody)
	}
	if !strings.Contains(crossTypeBody, "ordinary-cross-type-caveat") {
		t.Fatalf("cross-type filtering dropped an unrelated caveat:\n%s", crossTypeBody)
	}
}

func TestOutputDumpReservedResidualNamespaceCannotLeakFromBundleMetadataLanes(t *testing.T) {
	const forged = "raw_perf_capture_residual forged=8675309"
	bundle := traceBundleReportMetadata{
		Version: "hitraceconv-v1",
		Artifacts: []traceBundleReportArtifact{{
			Type: "systrace", Path: "capture.systrace", Caveats: []string{"ordinary-artifact", forged},
		}},
		TraceDecisions: []traceBundleReportTraceDecision{{
			ProviderName: "trace_streamer", Selected: true, Attempted: true, Succeeded: true,
			Reason: "ordinary-reason", Caveat: forged,
		}},
		TraceCoverage: []traceBundleReportTraceCoverage{{
			Family: "trace", Table: "sched", Found: false, Error: forged,
		}},
		Caveats: []string{"ordinary-bundle", forged},
	}
	body, err := json.Marshal(bundle)
	if err != nil {
		t.Fatal(err)
	}
	report := BuildBody(Args{
		Language: "en", Request: "inspect", Answer: "answer",
		RuntimeArtifacts: RuntimeArtifactsFromAttachment(
			"trace", "# codrax-source: capture.tracebundle.json\n"+string(body),
		),
	})
	if strings.Contains(report, "raw_perf_capture_residual") || strings.Contains(report, "8675309") {
		t.Fatalf("reserved residual namespace leaked from a generic bundle metadata lane:\n%s", report)
	}
	for _, ordinary := range []string{"ordinary-artifact", "ordinary-reason", "ordinary-bundle"} {
		if !strings.Contains(report, ordinary) {
			t.Fatalf("bundle metadata redaction dropped %q:\n%s", ordinary, report)
		}
	}
}

func TestOutputDumpInvalidRawCapabilityLeaksNoCensusNumbers(t *testing.T) {
	invalid := r2bRawPerfArtifact("invalid.perftrace", 8675309, true)
	invalid.Perf.TimeDomain = "forged"
	body := BuildBody(Args{
		Request: "inspect", Answer: "answer", RuntimeArtifacts: r2bRuntimeArtifacts(t, invalid),
	})
	for _, want := range []string{"raw_perf_capture valid=false", "reason=raw_profile_mismatch"} {
		if !strings.Contains(body, want) {
			t.Fatalf("invalid output missing %q:\n%s", want, body)
		}
	}
	for _, leaked := range []string{"8675309", "sample_records=", "lost_events=exact:"} {
		if strings.Contains(body, leaked) {
			t.Fatalf("invalid output leaked census token %q:\n%s", leaked, body)
		}
	}
}

func TestOutputDumpRawConverterMissingCapabilityFailsClosed(t *testing.T) {
	missing := hitraceconv.Artifact{
		Type: "perftrace", Path: "missing.perftrace", Converter: "hitraceconv-v1+raw-perfdata",
	}
	body := BuildBody(Args{
		Request: "inspect", Answer: "answer", RuntimeArtifacts: r2bRuntimeArtifacts(t, missing),
	})
	if !strings.Contains(body, "raw_perf_capture valid=false") ||
		!strings.Contains(body, "reason=capability_missing") {
		t.Fatalf("raw converter with missing Perf capability escaped typed classifier:\n%s", body)
	}
}

func TestOutputDumpMultiArtifactAndHTMLShareTypedFactSource(t *testing.T) {
	ready := r2bRawPerfArtifact("a/capture.perftrace", 1, false)
	inventory := r2bRawPerfArtifact("b/capture.perftrace", 0, true)
	markdown := BuildBody(Args{
		Language: "en", Request: "inspect", Answer: "answer", RuntimeArtifacts: r2bRuntimeArtifacts(t, ready, inventory),
	})
	html, err := BuildHTML("report", markdown)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"a/capture.perftrace", "b/capture.perftrace",
		"capture_state=query_ready", "capture_state=inventory_only", "Input quality (raw perf)",
	} {
		if !strings.Contains(markdown, want) || !strings.Contains(html, want) {
			t.Fatalf("Markdown/HTML typed fact parity lost %q (markdown=%t html=%t)",
				want, strings.Contains(markdown, want), strings.Contains(html, want))
		}
	}
	if !strings.Contains(markdown, "### Input quality (raw perf)") ||
		!strings.Contains(html, "Input quality (raw perf)</h3>") {
		t.Fatalf("input-quality heading lost Markdown/HTML semantic parity")
	}
}

func TestOutputDumpRawPerfDisclosureIsGloballyBoundedAndPrecedesAnswer(t *testing.T) {
	artifacts := make([]hitraceconv.Artifact, 0, 12)
	for index := 0; index < 12; index++ {
		artifacts = append(artifacts, r2bRawPerfArtifact(fmt.Sprintf("capture-%d.perftrace", index), 1, false))
	}
	markdown := BuildBody(Args{
		Language: "en", Request: "inspect", Answer: "causal detail", RuntimeArtifacts: r2bRuntimeArtifacts(t, artifacts...),
	})
	if got := strings.Count(markdown, " sample_records=physical:"); got != 8 {
		t.Fatalf("raw census rows=%d want=8:\n%s", got, markdown)
	}
	if strings.Count(markdown, "raw_perf_capture omitted=4") != 1 ||
		strings.Count(markdown, "next_boundary=inspect_omitted_raw_perf_artifacts omitted=4") != 1 {
		t.Fatalf("bounded output lost exact omitted marker:\n%s", markdown)
	}
	artifactsAt := strings.Index(markdown, "## Runtime Artifacts")
	qualityAt := strings.Index(markdown, "### Input quality (raw perf)")
	answerAt := strings.Index(markdown, "# Answer")
	if artifactsAt < 0 || qualityAt <= artifactsAt || answerAt <= qualityAt {
		t.Fatalf("input quality block is not between artifact summary and answer details:\n%s", markdown)
	}
	html, err := BuildHTML("report", markdown)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"raw_perf_capture omitted=4", "capture-0.perftrace", "capture-7.perftrace"} {
		if !strings.Contains(markdown, want) || !strings.Contains(html, want) {
			t.Fatalf("bounded Markdown/HTML parity lost %q", want)
		}
	}
}

func TestOutputDumpRawPerfDuplicatePathFailsClosedBeforeVisibleRowDedupe(t *testing.T) {
	ready := r2bRawPerfArtifact("same.perftrace", 1, false)
	body := BuildBody(Args{
		Language: "en", Request: "inspect", Answer: "answer",
		RuntimeArtifacts: r2bRuntimeArtifacts(t, ready, ready),
	})
	if !strings.Contains(body, "reason=duplicate_artifact_path") ||
		!strings.Contains(body, "raw_perf_capture omitted=1") ||
		strings.Contains(body, "capture_state=query_ready") {
		t.Fatalf("duplicate child was hidden by visible-row dedupe or retained readiness:\n%s", body)
	}
}

func TestOutputDumpSameRelativePathAcrossBundlesKeepsScopedDisclosures(t *testing.T) {
	first := r2bRuntimeArtifacts(t, r2bRawPerfArtifact("capture.perftrace", 1, false))
	secondBody, err := json.Marshal(struct {
		Version   string                 `json:"version"`
		Artifacts []hitraceconv.Artifact `json:"artifacts"`
	}{Version: "hitraceconv-v1", Artifacts: []hitraceconv.Artifact{r2bRawPerfArtifact("capture.perftrace", 0, true)}})
	if err != nil {
		t.Fatal(err)
	}
	second := RuntimeArtifactsFromAttachment("trace", "# codrax-source: second.tracebundle.json\n"+string(secondBody))
	// Give the first bundle a distinct scope while preserving its original
	// raw child list.
	first[0].traceBundleScope = "first.tracebundle.json"
	body := BuildBody(Args{
		Language: "en", Request: "inspect", Answer: "answer",
		RuntimeArtifacts: append(first, second...),
	})
	if strings.Count(body, "raw_perf_capture valid=true artifact=") != 2 ||
		!strings.Contains(body, "artifact=first.tracebundle.json@") ||
		!strings.Contains(body, "artifact=second.tracebundle.json@") ||
		strings.Contains(body, "duplicate_artifact_path") {
		t.Fatalf("cross-bundle relative paths collided:\n%s", body)
	}
}

func TestOutputDumpKeepsNonrawPerfCapabilityDetail(t *testing.T) {
	official := hitraceconv.Artifact{
		Type: "perftrace", Path: "official.perftrace",
		Perf: &hitraceconv.PerfArtifactCapability{
			ProviderName: "official", ProviderKind: "official_android", InputFormat: "linux_perf_data",
			Symbolization: "symbols", CPUIdentity: "sample_cpu", Callchain: "frames",
			TimeAlignment: "calibrated", TraceQueryReady: true,
		},
	}
	body := BuildBody(Args{Language: "en", Request: "inspect", Answer: "answer", RuntimeArtifacts: r2bRuntimeArtifacts(t, official)})
	for _, want := range []string{"perf_provider=official", "perf_symbolization=symbols", "perf_time_alignment=calibrated"} {
		if !strings.Contains(body, want) {
			t.Fatalf("nonraw perf detail lost %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "raw_perf_capture") {
		t.Fatalf("nonraw perf artifact entered raw disclosure lane:\n%s", body)
	}
}
