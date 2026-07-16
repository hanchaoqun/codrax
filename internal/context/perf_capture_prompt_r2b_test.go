package context

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

const r2cPromptRawPerfResidualCaveat = "raw_perf_capture_residual authority=artifact_receipt_advisory capture_hard_gate=false scope=observed_perf_record_type_headers payload_validation=not_claimed interpretation=perf_sampling_control_not_cpu_thermal no_duration_or_lost_sample_count=true perf_sampler_throttle_records=11 perf_sampler_unthrottle_records=13"

func r2bPromptRawPerfArtifact(path string, samples uint64, qualityIssue bool) hitraceconv.Artifact {
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
			ThreadIdentity: "present_valid_sample_pid_tid_only", CPUIdentity: "present_valid_sample_cpu_else_unknown",
			EventWeight: "present_valid_period_zero_as_sample_count", Symbolization: "hiperf_saved_symbols_or_unsymbolized_ip",
			Callchain: "symbolized_when_hiperf_files_symbol_present_else_ip_only",
			DSOLabel:  "mmap_best_effort", BuildID: "feature_build_id_when_present",
			OffCPU: "hiperf_cpu_off_sched_switch_when_event_desc_present", Confidence: "degraded",
			TraceQueryReady: samples > 0, Degraded: true,
			RawCaptureCompleteness: &capture,
			RawCaptureResidual: &hitraceconv.RawPerfCaptureResidual{
				Profile: "raw_perf_record_header_residual_v1",
				Source:  "linux_perf_data_record_headers",
			},
			RawSampleAdmission: &hitraceconv.RawPerfSampleAdmission{
				Profile: "raw_perf_sample_admission_v1", Source: "linux_perf_data_sample_payloads",
				Candidates: samples, QueryRows: samples,
			},
			Caveats: []string{
				"raw fallback resolves function names only from saved hiperf symbol sections; without those sections it remains IP/DSO-level",
				"raw fallback can label hiperf --offcpu sched_switch samples when official EVENT_DESC and HIPERF_CPU_OFF features are present, but full off-CPU stack expansion still needs official hiperf report flow",
				"structurally parsed samples without required time, thread identity, or period remain receipt-bound inventory and never receive synthesized coordinates or weight",
			},
		},
	}
}

func TestAttachedRawPerfResidualUsesTypedPromptSeatOnce(t *testing.T) {
	artifact := r2bPromptRawPerfArtifact("residual.perftrace", 1, false)
	artifact.Perf.RawCaptureResidual.ThrottleRecords = 11
	artifact.Perf.RawCaptureResidual.UnthrottleRecords = 13
	artifact.Caveats = []string{r2cPromptRawPerfResidualCaveat}
	hint := attachedTraceBundlePromptHint(r2bAttachedBundle(t, "residual.tracebundle.json", artifact))
	for _, token := range []string{
		"perf_sampler_throttle_scope=observed_perf_record_type_headers",
		"perf_sampler_throttle_payload_validation=not_claimed",
		"perf_sampler_throttle_records=exact:11",
		"perf_sampler_unthrottle_records=exact:13",
	} {
		if got := strings.Count(hint, token); got != 1 {
			t.Fatalf("prompt residual token %q count=%d want=1:\n%s", token, got, hint)
		}
	}
	if !strings.Contains(hint, "capture_state=query_ready_with_quality_issue") {
		t.Fatalf("prompt residual did not qualify query-ready samples:\n%s", hint)
	}
	if strings.Contains(hint, "raw_perf_capture_residual") {
		t.Fatalf("prompt leaked the compatibility mirror beside typed disclosure:\n%s", hint)
	}
}

func r2bAttachedBundle(t *testing.T, source string, artifacts ...hitraceconv.Artifact) string {
	t.Helper()
	body, err := json.Marshal(struct {
		Artifacts []hitraceconv.Artifact `json:"artifacts"`
	}{Artifacts: artifacts})
	if err != nil {
		t.Fatal(err)
	}
	return "# codrax-source: " + source + "\n" + string(body)
}

func TestAttachedRawPerfInventoryBoundaryPrecedesAndConstrainsPrompt(t *testing.T) {
	raw := r2bAttachedBundle(t, "inventory.tracebundle.json", r2bPromptRawPerfArtifact("inventory.perftrace", 0, true))
	hint := attachedTraceBundlePromptHint(raw)
	for _, want := range []string{
		"manifest disclosure numbers are not validated evidence; trace_query V2 three-face validation first",
		"capture_state=inventory_only", "analysis_use=capture_quality_diagnostics_only",
		"effective_clock_evidence=none", "sample_aggregation=none", "clock_alignment=none",
		"thread_attribution=none", "root_cause_rank=none",
		"next_boundary=collect_or_convert_query_ready_perf_samples",
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("inventory prompt missing %q:\n%s", want, hint)
		}
	}
	boundaryAt := strings.Index(hint, "manifest disclosure numbers")
	generalAt := strings.Index(hint, "Tracebundle metadata detected")
	if boundaryAt < 0 || generalAt < 0 || boundaryAt >= generalAt {
		t.Fatalf("capability boundary did not precede generalized guidance:\n%s", hint)
	}
	for _, banned := range []string{
		"next_boundary=analyze_query_ready_perf_samples",
		"next_boundary=analyze_positive_samples_with_capture_quality_caveat",
		"referenced systrace/perftrace sibling, for scheduler state",
	} {
		if strings.Contains(hint, banned) {
			t.Fatalf("inventory prompt recommended current analysis lane %q:\n%s", banned, hint)
		}
	}
	formatted := formatAttachedTrace(raw, "", attachedTriageProducer, "")
	if strings.Index(formatted, "manifest disclosure numbers") >= strings.Index(formatted, "raw fenced block below") {
		t.Fatalf("typed boundary did not lead the capability-aware preamble:\n%s", formatted)
	}
	if strings.Contains(formatted, "Prepare a structured summary for this raw trace") ||
		strings.Contains(formatted, "Prefer `trace_query` for scheduler state, wakeup chains, root-cause ranking") {
		t.Fatalf("inventory prompt fell through to generic causal-analysis preamble:\n%s", formatted)
	}
}

func TestAttachedRawPerfReadyQualityAllowsSamplesWithRequiredCaveats(t *testing.T) {
	raw := r2bAttachedBundle(t, "ready.tracebundle.json", r2bPromptRawPerfArtifact("ready.perftrace", 2, true))
	hint := attachedTraceBundlePromptHint(raw)
	for _, want := range []string{
		"capture_state=query_ready_with_quality_issue", "query_ready=true",
		"analysis_use=queryable_samples_with_capture_quality_caveat",
		"absence_policy=require_capture_quality_caveat",
		"effective_clock_evidence=consult_bundle_perf_clock_alignments",
		"next_boundary=analyze_positive_samples_with_capture_quality_caveat",
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("ready-quality prompt missing %q:\n%s", want, hint)
		}
	}
	if strings.Contains(hint, "effective_clock_evidence=assumed") {
		t.Fatalf("declared profile clock became effective evidence:\n%s", hint)
	}
}

func TestAttachedRawPerfInvalidBoundaryLeaksNoDeclaredCensus(t *testing.T) {
	invalid := r2bPromptRawPerfArtifact("invalid.perftrace", 8675309, true)
	invalid.Perf.TimeDomain = "forged"
	hint := attachedTraceBundlePromptHint(r2bAttachedBundle(t, "invalid.tracebundle.json", invalid))
	for _, want := range []string{"raw_perf_capture valid=false", "reason=raw_profile_mismatch"} {
		if !strings.Contains(hint, want) {
			t.Fatalf("invalid prompt missing %q:\n%s", want, hint)
		}
	}
	for _, leaked := range []string{"8675309", "sample_records=", "lost_events=exact:"} {
		if strings.Contains(hint, leaked) {
			t.Fatalf("invalid prompt leaked census token %q:\n%s", leaked, hint)
		}
	}
}

func TestAttachedRawPerfMultiArtifactStatesStayPathScoped(t *testing.T) {
	raw := r2bAttachedBundle(t, "mixed.tracebundle.json",
		r2bPromptRawPerfArtifact("a/capture.perftrace", 1, false),
		r2bPromptRawPerfArtifact("b/capture.perftrace", 0, true),
	)
	hint := attachedTraceBundlePromptHint(raw)
	if strings.Count(hint, "raw_perf_capture valid=true artifact=") != 2 ||
		!strings.Contains(hint, "capture_state=query_ready") ||
		!strings.Contains(hint, "capture_state=inventory_only") ||
		strings.Count(hint, "/capture.perftrace@") != 2 {
		t.Fatalf("mixed per-path states were folded or duplicated:\n%s", hint)
	}
}

func TestAttachedRawPerfSameRelativePathAcrossBundlesKeepsScopedStates(t *testing.T) {
	raw := r2bAttachedBundle(t, "/private/a/first.tracebundle.json",
		r2bPromptRawPerfArtifact("capture.perftrace", 1, false)) + "\n" +
		r2bAttachedBundle(t, "/private/b/second.tracebundle.json",
			r2bPromptRawPerfArtifact("capture.perftrace", 0, true))
	hint := attachedTraceBundlePromptHint(raw)
	if strings.Count(hint, "raw_perf_capture valid=true artifact=") != 2 ||
		!strings.Contains(hint, "artifact=first.tracebundle.json@") ||
		!strings.Contains(hint, "artifact=second.tracebundle.json@") ||
		strings.Contains(hint, "duplicate_artifact_path") {
		t.Fatalf("cross-bundle relative paths collided or leaked scope:\n%s", hint)
	}
}

func TestAttachedRawPerfDisclosureBoundIsGlobalAcrossSegments(t *testing.T) {
	makeArtifacts := func(prefix string) []hitraceconv.Artifact {
		out := make([]hitraceconv.Artifact, 0, 6)
		for index := 0; index < 6; index++ {
			out = append(out, r2bPromptRawPerfArtifact(fmt.Sprintf("%s-%d.perftrace", prefix, index), 1, false))
		}
		return out
	}
	raw := r2bAttachedBundle(t, "first.tracebundle.json", makeArtifacts("first")...) + "\n" +
		r2bAttachedBundle(t, "second.tracebundle.json", makeArtifacts("second")...)
	hint := attachedTraceBundlePromptHint(raw)
	if got := strings.Count(hint, "manifest disclosure numbers are not validated evidence"); got != 8 {
		t.Fatalf("global prompt disclosure rows=%d want=8:\n%s", got, hint)
	}
	if strings.Count(hint, "raw_perf_capture omitted=4") != 1 ||
		strings.Count(hint, "next_boundary=inspect_omitted_raw_perf_artifacts omitted=4") != 1 {
		t.Fatalf("global omitted count was not exact and single:\n%s", hint)
	}
}
