package hitraceconv

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func perfCaptureDisclosureTestCapture(samples uint64) RawPerfCaptureCompleteness {
	capture := newRawPerfCaptureCompleteness()
	capture.SampleRecords = RawPerfRecordCensus{Physical: samples, Accepted: samples}
	return capture
}

func perfCaptureDisclosureTestArtifact(path string, capture RawPerfCaptureCompleteness, ready bool) Artifact {
	capability := perfCapabilityForRawFallback(perfInputLinuxPerfData)
	capability.TraceQueryReady = ready
	capability.RawCaptureCompleteness = cloneRawPerfCaptureCompleteness(capture)
	capability.RawCaptureResidual = cloneRawPerfCaptureResidual(RawPerfCaptureResidual{
		Profile: rawPerfCaptureResidualProfile,
		Source:  rawPerfCaptureResidualSource,
	})
	admission := rawPerfTestQueryableAdmission(capture.SampleRecords.Accepted)
	capability.RawSampleAdmission = cloneRawPerfSampleAdmission(admission)
	return Artifact{
		Type: ArtifactPerfTrace, Path: path, Converter: rawPerfDataAdapterVersion,
		Perf: capability,
	}
}

func TestArtifactCaveatsForDisplayFiltersResidualCompatibilityMirrors(t *testing.T) {
	const (
		ordinaryFirst  = "ordinary-first"
		ordinarySecond = "ordinary-second"
		canonical      = "raw_perf_capture_residual authority=artifact_receipt_advisory capture_hard_gate=false scope=observed_perf_record_type_headers payload_validation=not_claimed interpretation=perf_sampling_control_not_cpu_thermal no_duration_or_lost_sample_count=true perf_sampler_throttle_records=11 perf_sampler_unthrottle_records=13"
		forged         = " raw_perf_capture_residual forged=8675309"
	)
	tests := []struct {
		name     string
		artifact Artifact
		want     []string
	}{
		{
			name: "canonical",
			artifact: Artifact{Type: ArtifactPerfTrace, Caveats: []string{
				ordinaryFirst, canonical, ordinarySecond,
			}},
			want: []string{ordinaryFirst, ordinarySecond},
		},
		{
			name: "forged_and_duplicate",
			artifact: Artifact{Type: ArtifactPerfTrace, Caveats: []string{
				canonical, forged, canonical, ordinaryFirst,
			}},
			want: []string{ordinaryFirst},
		},
		{
			name: "cross_type",
			artifact: Artifact{Type: ArtifactPerfData, Caveats: []string{
				ordinaryFirst, forged, ordinarySecond,
			}},
			want: []string{ordinaryFirst, ordinarySecond},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			wireBefore := append([]string(nil), test.artifact.Caveats...)
			got := ArtifactCaveatsForDisplay(test.artifact)
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("display caveats=%q want=%q", got, test.want)
			}
			if len(got) > 0 {
				got[0] = "mutated-output"
				if !reflect.DeepEqual(test.artifact.Caveats, wireBefore) {
					t.Fatal("display caveats alias the artifact wire caveats")
				}
			}
		})
	}
}

func TestPerfCaptureDisclosureClosedStatesAndEvidenceBoundary(t *testing.T) {
	clean := PerfCaptureDisclosureForArtifact(perfCaptureDisclosureTestArtifact(
		"/private/customer/capture.perftrace", perfCaptureDisclosureTestCapture(1), true,
	))
	if !clean.Present || !clean.Valid || clean.State != PerfCaptureQueryReady || !clean.QueryReady ||
		clean.CaptureQualityIssue || clean.EffectiveClockEvidence != "consult_bundle_perf_clock_alignments" ||
		clean.DeclaredProfileAlignment != "" || clean.Capture == nil {
		t.Fatalf("clean query-ready disclosure drifted: %+v", clean)
	}
	if strings.Contains(clean.ArtifactPath, "/private/") || !strings.HasPrefix(clean.ArtifactPath, "capture.perftrace@") {
		t.Fatalf("artifact identity leaked an absolute path or lost basename: %q", clean.ArtifactPath)
	}

	qualityCapture := perfCaptureDisclosureTestCapture(2)
	qualityCapture.LostRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
	qualityCapture.LostEvents = RawPerfAggregateTotal{State: rawPerfAggregateExact, Value: 3}
	quality := PerfCaptureDisclosureForArtifact(perfCaptureDisclosureTestArtifact("quality.perftrace", qualityCapture, true))
	if !quality.Valid || quality.State != PerfCaptureQueryReadyWithQualityIssue || !quality.QueryReady ||
		!quality.CaptureQualityIssue || quality.AnalysisUse != "queryable_samples_with_capture_quality_caveat" {
		t.Fatalf("ready quality state drifted: %+v", quality)
	}

	inventoryCapture := perfCaptureDisclosureTestCapture(0)
	inventoryCapture.LostRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
	inventoryCapture.LostEvents = RawPerfAggregateTotal{State: rawPerfAggregateExact, Value: 4}
	inventory := PerfCaptureDisclosureForArtifact(perfCaptureDisclosureTestArtifact("inventory.perftrace", inventoryCapture, false))
	if !inventory.Valid || inventory.State != PerfCaptureInventoryOnly || inventory.QueryReady ||
		!inventory.CaptureQualityIssue || inventory.AnalysisUse != "capture_quality_diagnostics_only" ||
		inventory.EffectiveClockEvidence != "none" || inventory.DeclaredProfileAlignment != "assumed" ||
		inventory.SampleAggregation != "none" || inventory.ClockAlignment != "none" ||
		inventory.ThreadAttribution != "none" || inventory.RootCauseRank != "none" {
		t.Fatalf("inventory authority boundary drifted: %+v", inventory)
	}
}

func TestPerfCaptureDisclosureReadyAuxUnknownKeepsSamplesAndQualifiesCapture(t *testing.T) {
	capture := perfCaptureDisclosureTestCapture(1)
	capture.AuxRecords = RawPerfRecordCensus{Physical: 2, Accepted: 2}
	capture.AuxBytes = RawPerfAggregateTotal{State: rawPerfAggregateUnknown, Reason: rawPerfUnknownAggregateOverflow}
	disclosure := PerfCaptureDisclosureForArtifact(perfCaptureDisclosureTestArtifact("aux.perftrace", capture, true))
	if !disclosure.Valid || disclosure.State != PerfCaptureQueryReadyWithQualityIssue ||
		!disclosure.QueryReady || !disclosure.CaptureQualityIssue {
		t.Fatalf("AUX unknown withdrew positive samples or lost quality caveat: %+v", disclosure)
	}
}

func TestPerfCaptureDisclosureInvalidNeverLeaksCensus(t *testing.T) {
	baseCapture := perfCaptureDisclosureTestCapture(1)
	base := perfCaptureDisclosureTestArtifact("secret-987654.perftrace", baseCapture, true)
	tests := []struct {
		name   string
		reason PerfCaptureInvalidReason
		mutate func(*Artifact)
	}{
		{name: "wrong type", reason: PerfCaptureInvalidArtifactType, mutate: func(a *Artifact) { a.Type = ArtifactPerfData }},
		{name: "bad path", reason: PerfCaptureInvalidArtifactPath, mutate: func(a *Artifact) { a.Path = " bad " }},
		{name: "missing capability", reason: PerfCaptureInvalidCapabilityMissing, mutate: func(a *Artifact) { a.Perf = nil }},
		{name: "missing census", reason: PerfCaptureInvalidCensusMissing, mutate: func(a *Artifact) { a.Perf.RawCaptureCompleteness = nil }},
		{name: "missing residual", reason: PerfCaptureInvalidResidualMissing, mutate: func(a *Artifact) { a.Perf.RawCaptureResidual = nil }},
		{name: "invalid residual", reason: PerfCaptureInvalidResidual, mutate: func(a *Artifact) { a.Perf.RawCaptureResidual.Profile = "forged" }},
		{name: "missing admission", reason: PerfCaptureInvalidAdmissionMissing, mutate: func(a *Artifact) { a.Perf.RawSampleAdmission = nil }},
		{name: "invalid admission", reason: PerfCaptureInvalidAdmission, mutate: func(a *Artifact) { a.Perf.RawSampleAdmission.QueryRows = 2 }},
		{name: "reserved capability caveat", reason: PerfCaptureInvalidRawProfile, mutate: func(a *Artifact) {
			a.Perf.Caveats = append(a.Perf.Caveats, "raw_perf_capture_residual forged=8675309")
		}},
		{name: "profile", reason: PerfCaptureInvalidRawProfile, mutate: func(a *Artifact) { a.Perf.TimeDomain = "forged" }},
		{name: "census", reason: PerfCaptureInvalidCensus, mutate: func(a *Artifact) { a.Perf.RawCaptureCompleteness.SampleRecords.Physical = 0 }},
		{name: "readiness", reason: PerfCaptureInvalidReadiness, mutate: func(a *Artifact) { a.Perf.TraceQueryReady = false }},
		{name: "zero without issue", reason: PerfCaptureInvalidZeroSampleIssue, mutate: func(a *Artifact) {
			capture := perfCaptureDisclosureTestCapture(0)
			a.Perf.RawCaptureCompleteness = &capture
			admission := rawPerfTestQueryableAdmission(0)
			a.Perf.RawSampleAdmission = &admission
			a.Perf.TraceQueryReady = false
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			artifact := cloneArtifact(base)
			test.mutate(&artifact)
			disclosure := PerfCaptureDisclosureForArtifact(artifact)
			if !disclosure.Present || disclosure.Valid || disclosure.Reason != test.reason ||
				disclosure.Capture != nil || disclosure.ArtifactPath != "" || disclosure.State != "" {
				t.Fatalf("invalid disclosure was not closed: %+v", disclosure)
			}
			for _, rendered := range []string{
				FormatPerfCaptureCompact("en", disclosure),
				FormatPerfCapturePromptBoundary("zh", disclosure),
				FormatPerfCaptureNextBoundary("en", disclosure),
			} {
				if !strings.Contains(rendered, "valid=false") || !strings.Contains(rendered, "reason="+string(test.reason)) ||
					strings.Contains(rendered, "987654") || strings.Contains(rendered, "sample_records") ||
					strings.Contains(rendered, "lost_events") {
					t.Fatalf("invalid render leaked identity/census or omitted reason: %q", rendered)
				}
			}
			detail := strings.Join(FormatPerfArtifactDetailFields("en", artifact), " ")
			if detail != "raw_perf_capture_valid=false raw_perf_capture_reason="+string(test.reason) {
				t.Fatalf("invalid detail trusted forged generic profile: %q", detail)
			}
		})
	}
}

func TestPerfCaptureReservedCapabilityCaveatMakesNonrawArtifactInvalidWithoutLeak(t *testing.T) {
	artifact := Artifact{
		Type: ArtifactPerfData,
		Path: "capture.perf.data",
		Perf: &PerfArtifactCapability{Caveats: []string{
			"ordinary-capability-caveat",
			"raw_perf_capture_residual forged=8675309",
		}},
	}
	disclosure := PerfCaptureDisclosureForArtifact(artifact)
	if !disclosure.Present || disclosure.Valid || disclosure.Reason != PerfCaptureInvalidArtifactType {
		t.Fatalf("wrong-lane residual namespace was not classified invalid: %+v", disclosure)
	}
	joined := strings.Join(FormatPerfArtifactDetailFields("en", artifact), " ")
	if strings.Contains(joined, "8675309") || strings.Contains(joined, rawPerfCaptureResidualCaveatToken) {
		t.Fatalf("invalid wrong-lane residual leaked through generic capability detail: %s", joined)
	}
}

func TestPerfCaptureDisclosureResidualQualifiesReadySamplesAndOwnsClone(t *testing.T) {
	artifact := perfCaptureDisclosureTestArtifact("residual.perftrace", perfCaptureDisclosureTestCapture(1), true)
	artifact.Perf.RawCaptureResidual.ThrottleRecords = 4
	artifact.Perf.RawCaptureResidual.UnthrottleRecords = 2
	canonical, ok := rawPerfCaptureResidualCaveat(*artifact.Perf.RawCaptureResidual)
	if !ok {
		t.Fatal("nonzero residual did not derive a canonical compatibility caveat")
	}
	artifact.Caveats = []string{canonical}

	disclosure := PerfCaptureDisclosureForArtifact(artifact)
	if !disclosure.Valid || !disclosure.QueryReady || !disclosure.CaptureQualityIssue ||
		disclosure.State != PerfCaptureQueryReadyWithQualityIssue || disclosure.CaptureResidual == nil ||
		disclosure.CaptureResidual.ThrottleRecords != 4 || disclosure.CaptureResidual.UnthrottleRecords != 2 {
		t.Fatalf("nonzero residual disclosure drifted: %+v", disclosure)
	}
	joined := strings.Join(perfCaptureMachineFields(disclosure), " ")
	for _, want := range []string{
		"perf_sampler_throttle_scope=observed_perf_record_type_headers",
		"perf_sampler_throttle_payload_validation=not_claimed",
		"perf_sampler_throttle_records=exact:4",
		"perf_sampler_unthrottle_records=exact:2",
		"perf_sampler_throttle_semantics=capture_quality_not_cpu_thermal",
		"perf_sampler_throttle_duration=not_reported",
		"perf_sampler_throttle_lost_samples=not_reported",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("residual machine disclosure missing %q: %s", want, joined)
		}
	}
	disclosure.CaptureResidual.ThrottleRecords = 99
	if artifact.Perf.RawCaptureResidual.ThrottleRecords != 4 {
		t.Fatal("disclosure residual aliases the artifact capability")
	}

	missingCanonical := artifact
	missingCanonical.Caveats = nil
	invalid := PerfCaptureDisclosureForArtifact(missingCanonical)
	if invalid.Valid || invalid.Reason != PerfCaptureInvalidResidual || invalid.CaptureResidual != nil {
		t.Fatalf("missing residual canonical did not fail closed: %+v", invalid)
	}
}

func TestPerfCaptureDisclosureFormatsPreserveThreeValueAggregatesAndMachineTokens(t *testing.T) {
	capture := perfCaptureDisclosureTestCapture(1)
	capture.LostRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
	capture.LostEvents = RawPerfAggregateTotal{State: rawPerfAggregateExact}
	capture.LostSampleRecords = RawPerfRecordCensus{Physical: 2, Accepted: 2}
	capture.LostSamples = RawPerfAggregateTotal{State: rawPerfAggregateUnknown, Reason: rawPerfUnknownAggregateOverflow}
	disclosure := PerfCaptureDisclosureForArtifact(perfCaptureDisclosureTestArtifact("states.perftrace", capture, true))
	for _, lang := range []string{"en", "zh"} {
		compact := FormatPerfCaptureCompact(lang, disclosure)
		prompt := FormatPerfCapturePromptBoundary(lang, disclosure)
		for _, want := range []string{
			"capture_state=query_ready_with_quality_issue",
			"census_scope=record_stream_only",
			"device_capture_completeness=not_claimed",
			"lost_events=exact:0",
			"lost_samples=unknown:aggregate_overflow",
			"aux_bytes=not_reported",
			"effective_clock_evidence=consult_bundle_perf_clock_alignments",
		} {
			if !strings.Contains(compact, want) || !strings.Contains(prompt, want) {
				t.Fatalf("%s formatter lost machine token %q\ncompact=%s\nprompt=%s", lang, want, compact, prompt)
			}
		}
		if !strings.Contains(prompt, "manifest disclosure numbers are not validated evidence; trace_query V2 three-face validation first") {
			t.Fatalf("%s prompt lost validation order: %s", lang, prompt)
		}
	}
	if got := FormatPerfCaptureNextBoundary("zh", disclosure); !strings.Contains(got, "可查询正样本") ||
		!strings.Contains(got, "缺失/不存在结论须附采集质量说明") ||
		!strings.Contains(got, "capture_state=query_ready_with_quality_issue") {
		t.Fatalf("zh ready-quality next boundary drifted: %s", got)
	}
}

func TestPerfCaptureDisclosureInventoryFormatDeniesAnalysisAuthority(t *testing.T) {
	capture := perfCaptureDisclosureTestCapture(0)
	capture.LostRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
	capture.LostEvents = RawPerfAggregateTotal{State: rawPerfAggregateExact, Value: 1}
	disclosure := PerfCaptureDisclosureForArtifact(perfCaptureDisclosureTestArtifact("inventory.perftrace", capture, false))
	for _, rendered := range []string{
		FormatPerfCaptureCompact("en", disclosure),
		FormatPerfCapturePromptBoundary("zh", disclosure),
	} {
		for _, want := range []string{
			"capture_state=inventory_only", "analysis_use=capture_quality_diagnostics_only",
			"effective_clock_evidence=none", "declared_profile_time_alignment=assumed",
			"sample_aggregation=none", "clock_alignment=none", "thread_attribution=none", "root_cause_rank=none",
		} {
			if !strings.Contains(rendered, want) {
				t.Fatalf("inventory format missing %q: %s", want, rendered)
			}
		}
	}
	if got := FormatPerfCaptureNextBoundary("zh", disclosure); !strings.Contains(got, "转换成功（质量库存），不可查询") ||
		!strings.Contains(got, "建议重新采集") || !strings.Contains(got, "capture_state=inventory_only") {
		t.Fatalf("zh inventory next boundary drifted: %s", got)
	}
}

func TestPerfArtifactDetailFieldsKeepGenericNonrawCapability(t *testing.T) {
	artifact := Artifact{Type: ArtifactPerfTrace, Path: "official.perftrace", Perf: &PerfArtifactCapability{
		ProviderName: "official", ProviderKind: "official_android", InputFormat: "linux_perf_data",
		Symbolization: "symbols", CPUIdentity: "sample_cpu", Callchain: "frames", TimeAlignment: "calibrated",
		TraceQueryReady: true,
	}}
	for _, want := range []string{
		"perf_provider=official", "perf_provider_kind=official_android", "perf_input=linux_perf_data",
		"perf_symbolization=symbols", "perf_cpu=sample_cpu", "perf_callchain=frames",
		"perf_time_alignment=calibrated", "trace_query_ready=true", "perf_degraded=false",
	} {
		if got := strings.Join(FormatPerfArtifactDetailFields("en", artifact), " "); !strings.Contains(got, want) {
			t.Fatalf("nonraw generic detail missing %q: %s", want, got)
		}
	}
}

func TestPerfCaptureDisclosuresBoundedMixedAndDuplicatePaths(t *testing.T) {
	ready := perfCaptureDisclosureTestCapture(1)
	inventory := perfCaptureDisclosureTestCapture(0)
	inventory.LostRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
	inventory.LostEvents = RawPerfAggregateTotal{State: rawPerfAggregateExact, Value: 1}
	artifacts := []Artifact{
		perfCaptureDisclosureTestArtifact("ready.perftrace", ready, true),
		perfCaptureDisclosureTestArtifact("inventory.perftrace", inventory, false),
	}
	for index := 0; index < 8; index++ {
		artifacts = append(artifacts, perfCaptureDisclosureTestArtifact(fmt.Sprintf("extra-%d.perftrace", index), ready, true))
	}
	rows := PerfCaptureDisclosures(artifacts)
	if len(rows) != perfCaptureDisclosureLimit+1 || rows[0].State != PerfCaptureQueryReady ||
		rows[1].State != PerfCaptureInventoryOnly || rows[len(rows)-1].Omitted != 2 {
		t.Fatalf("bounded mixed disclosures drifted: %+v", rows)
	}

	duplicateRows := PerfCaptureDisclosures([]Artifact{artifacts[0], artifacts[0]})
	if len(duplicateRows) != 2 || duplicateRows[0].Reason != PerfCaptureInvalidDuplicatePath ||
		duplicateRows[0].Capture != nil || duplicateRows[1].Omitted != 1 {
		t.Fatalf("duplicate path was folded or leaked census: %+v", duplicateRows)
	}
}

func TestPerfCaptureDisclosuresForGroupsScopesPathsAndOwnsGlobalBudget(t *testing.T) {
	ready := perfCaptureDisclosureTestCapture(1)
	inventory := perfCaptureDisclosureTestCapture(0)
	inventory.LostRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
	inventory.LostEvents = RawPerfAggregateTotal{State: rawPerfAggregateExact, Value: 1}
	groups := []PerfCaptureArtifactGroup{
		{Scope: "/private/a/first.tracebundle.json", Artifacts: []Artifact{
			perfCaptureDisclosureTestArtifact("capture.perftrace", ready, true),
		}},
		{Scope: "/private/b/second.tracebundle.json", Artifacts: []Artifact{
			perfCaptureDisclosureTestArtifact("capture.perftrace", inventory, false),
		}},
	}
	for index := 0; index < 10; index++ {
		groups[1].Artifacts = append(groups[1].Artifacts,
			perfCaptureDisclosureTestArtifact(fmt.Sprintf("extra-%d.perftrace", index), ready, true))
	}
	rows := PerfCaptureDisclosuresForGroups(groups)
	if len(rows) != perfCaptureDisclosureLimit+1 || rows[0].State != PerfCaptureQueryReady ||
		rows[1].State != PerfCaptureInventoryOnly || rows[len(rows)-1].Omitted != 4 {
		t.Fatalf("grouped disclosure budget/state drifted: %+v", rows)
	}
	if rows[0].ArtifactPath == rows[1].ArtifactPath ||
		!strings.HasPrefix(rows[0].ArtifactPath, "first.tracebundle.json@") ||
		!strings.HasPrefix(rows[1].ArtifactPath, "second.tracebundle.json@") ||
		strings.Contains(rows[0].ArtifactPath, "/private/") || strings.Contains(rows[1].ArtifactPath, "/private/") {
		t.Fatalf("group scope was ambiguous or leaked a private path: %+v", rows[:2])
	}

	duplicate := PerfCaptureDisclosuresForGroups([]PerfCaptureArtifactGroup{{
		Scope: "duplicate.tracebundle.json",
		Artifacts: []Artifact{
			perfCaptureDisclosureTestArtifact("same.perftrace", ready, true),
			perfCaptureDisclosureTestArtifact("same.perftrace", ready, true),
		},
	}})
	if len(duplicate) != 2 || duplicate[0].Reason != PerfCaptureInvalidDuplicatePath || duplicate[1].Omitted != 1 {
		t.Fatalf("within-group duplicate escaped fail-closure: %+v", duplicate)
	}
}

func TestPerfCaptureDisclosureReturnsIndependentCensusClone(t *testing.T) {
	capture := perfCaptureDisclosureTestCapture(1)
	artifact := perfCaptureDisclosureTestArtifact("clone.perftrace", capture, true)
	disclosure := PerfCaptureDisclosureForArtifact(artifact)
	disclosure.Capture.SampleRecords.Accepted = 99
	disclosure.SampleAdmission.QueryRows = 99
	if artifact.Perf.RawCaptureCompleteness.SampleRecords.Accepted != 1 {
		t.Fatal("disclosure exposed the artifact census pointer")
	}
	if artifact.Perf.RawSampleAdmission.QueryRows != 1 {
		t.Fatal("disclosure exposed the artifact sample-admission pointer")
	}
}

func TestQueryReadyPerfTracePathDelegatesRawClaimsToSharedClassifier(t *testing.T) {
	valid := perfCaptureDisclosureTestArtifact("valid.perftrace", perfCaptureDisclosureTestCapture(1), true)
	missingCensus := cloneArtifact(valid)
	missingCensus.Path = "missing-census.perftrace"
	missingCensus.Perf.RawCaptureCompleteness = nil
	wrongProfile := cloneArtifact(valid)
	wrongProfile.Path = "wrong-profile.perftrace"
	wrongProfile.Perf.TimeDomain = "forged"
	readinessMismatch := cloneArtifact(valid)
	readinessMismatch.Path = "readiness-mismatch.perftrace"
	readinessMismatch.Perf.TraceQueryReady = false
	nonraw := Artifact{
		Type: ArtifactPerfTrace,
		Path: "official.perftrace",
		Perf: &PerfArtifactCapability{ProviderKind: "official_android", TraceQueryReady: true},
	}

	for _, test := range []struct {
		name     string
		artifact Artifact
		want     string
	}{
		{name: "valid raw", artifact: valid, want: valid.Path},
		{name: "raw missing census", artifact: missingCensus},
		{name: "raw wrong profile", artifact: wrongProfile},
		{name: "raw readiness mismatch", artifact: readinessMismatch},
		{name: "nonraw keeps established readiness", artifact: nonraw, want: nonraw.Path},
	} {
		t.Run(test.name, func(t *testing.T) {
			artifacts := []Artifact{test.artifact}
			if got := QueryReadyPerfTracePath(artifacts); got != test.want {
				t.Fatalf("query-ready path=%q want=%q", got, test.want)
			}
			if got := HasQueryReadyPerfTrace(artifacts); got != (test.want != "") {
				t.Fatalf("query-ready bool=%t want=%t", got, test.want != "")
			}
		})
	}

	duplicate := []Artifact{cloneArtifact(valid), cloneArtifact(valid)}
	if got := QueryReadyPerfTracePath(duplicate); got != "" || HasQueryReadyPerfTrace(duplicate) {
		t.Fatalf("duplicate raw path minted query readiness: path=%q", got)
	}
	crossTypeDuplicate := cloneArtifact(valid)
	crossTypeDuplicate.Type = ArtifactPerfData
	if got := QueryReadyPerfTracePath([]Artifact{valid, crossTypeDuplicate}); got != "" {
		t.Fatalf("cross-type duplicate raw path minted query readiness: path=%q", got)
	}
	if rows := PerfCaptureDisclosures([]Artifact{valid, crossTypeDuplicate}); len(rows) == 0 || rows[0].Reason != PerfCaptureInvalidDuplicatePath {
		t.Fatalf("cross-type duplicate did not share disclosure fail-closure: %+v", rows)
	}
}
