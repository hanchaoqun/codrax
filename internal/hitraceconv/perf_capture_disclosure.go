package hitraceconv

import (
	"crypto/sha256"
	"fmt"
	pathpkg "path"
	"strings"
	"unicode"
)

// PerfCaptureState is the closed user-facing capability state for one
// converter-owned raw perftrace artifact. It is deliberately separate from
// the declared provider profile: an "assumed" profile clock is not effective
// cross-artifact clock evidence.
type PerfCaptureState string

const (
	PerfCaptureQueryReady                 PerfCaptureState = "query_ready"
	PerfCaptureQueryReadyWithQualityIssue PerfCaptureState = "query_ready_with_quality_issue"
	PerfCaptureInventoryOnly              PerfCaptureState = "inventory_only"
)

// PerfCaptureInvalidReason is a closed reason token. Detailed validator text
// and census values are never copied into an invalid disclosure.
type PerfCaptureInvalidReason string

const (
	PerfCaptureInvalidArtifactType      PerfCaptureInvalidReason = "artifact_type_mismatch"
	PerfCaptureInvalidArtifactPath      PerfCaptureInvalidReason = "artifact_path_invalid"
	PerfCaptureInvalidCapabilityMissing PerfCaptureInvalidReason = "capability_missing"
	PerfCaptureInvalidRawProfile        PerfCaptureInvalidReason = "raw_profile_mismatch"
	PerfCaptureInvalidCensusMissing     PerfCaptureInvalidReason = "capture_census_missing"
	PerfCaptureInvalidCensus            PerfCaptureInvalidReason = "capture_census_invalid"
	PerfCaptureInvalidReadiness         PerfCaptureInvalidReason = "readiness_sample_account_mismatch"
	PerfCaptureInvalidZeroSampleIssue   PerfCaptureInvalidReason = "zero_sample_issue_required"
	PerfCaptureInvalidDuplicatePath     PerfCaptureInvalidReason = "duplicate_artifact_path"
)

const (
	PerfCaptureCensusScopeRecordStreamOnly  = "record_stream_only"
	PerfCaptureDeviceCompletenessNotClaimed = "not_claimed"

	perfCaptureDisclosureLimit = 8
)

// PerfCaptureDisclosure is the single typed authority consumed by CLI, REPL,
// output dumps and analysis prompts. Present=false means the artifact is not a
// raw-capture disclosure. Omitted>0 is the bounded-list summary marker. For an
// invalid raw-looking artifact only Present, Valid=false and Reason are set;
// in particular Capture remains nil so callers cannot mistake zero values for
// a declared census.
type PerfCaptureDisclosure struct {
	Present bool
	Valid   bool
	Reason  PerfCaptureInvalidReason
	Omitted int

	ArtifactPath              string
	State                     PerfCaptureState
	QueryReady                bool
	CaptureQualityIssue       bool
	CensusScope               string
	DeviceCaptureCompleteness string
	DeclaredProfileAlignment  string
	EffectiveClockEvidence    string
	PositiveEvidence          string
	AbsencePolicy             string
	AnalysisUse               string
	SampleAggregation         string
	ClockAlignment            string
	ThreadAttribution         string
	RootCauseRank             string
	Capture                   *RawPerfCaptureCompleteness
}

// PerfCaptureDisclosureForArtifact classifies one artifact without consulting
// caveat prose. Only the complete converter-owned raw perftrace profile can
// publish a census; partial identity matches fail closed as present+invalid.
func PerfCaptureDisclosureForArtifact(artifact Artifact) PerfCaptureDisclosure {
	if !perfCaptureArtifactLooksRaw(artifact) {
		return PerfCaptureDisclosure{}
	}
	invalid := func(reason PerfCaptureInvalidReason) PerfCaptureDisclosure {
		return PerfCaptureDisclosure{Present: true, Reason: reason}
	}
	if artifact.Type != ArtifactPerfTrace {
		return invalid(PerfCaptureInvalidArtifactType)
	}
	if artifact.Path == "" || artifact.Path != strings.TrimSpace(artifact.Path) {
		return invalid(PerfCaptureInvalidArtifactPath)
	}
	if artifact.Perf == nil {
		return invalid(PerfCaptureInvalidCapabilityMissing)
	}
	capability := artifact.Perf
	if capability.RawCaptureCompleteness == nil {
		return invalid(PerfCaptureInvalidCensusMissing)
	}
	capture := *capability.RawCaptureCompleteness
	expected := perfCapabilityForRawFallback(perfInputLinuxPerfData)
	expected.TraceQueryReady = capability.TraceQueryReady
	expected.RawCaptureCompleteness = cloneRawPerfCaptureCompleteness(capture)
	if artifact.Converter != rawPerfDataAdapterVersion ||
		!ownedPerfCapabilitySemanticsEqual(capability, expected) {
		return invalid(PerfCaptureInvalidRawProfile)
	}
	if validateRawPerfCaptureCompleteness(capture) != "" {
		return invalid(PerfCaptureInvalidCensus)
	}
	hasSamples := capture.SampleRecords.Accepted > 0
	if capability.TraceQueryReady != hasSamples {
		return invalid(PerfCaptureInvalidReadiness)
	}
	publicationIssue, err := rawPerfCaptureHasPublicationIssue(capture)
	if err != nil {
		return invalid(PerfCaptureInvalidCensus)
	}
	if !hasSamples && !publicationIssue {
		return invalid(PerfCaptureInvalidZeroSampleIssue)
	}
	qualityIssue := publicationIssue || capture.AuxBytes.State == rawPerfAggregateUnknown
	state := PerfCaptureQueryReady
	analysisUse := "queryable_samples"
	if qualityIssue {
		state = PerfCaptureQueryReadyWithQualityIssue
		analysisUse = "queryable_samples_with_capture_quality_caveat"
	}
	disclosure := PerfCaptureDisclosure{
		Present:                   true,
		Valid:                     true,
		ArtifactPath:              perfCaptureArtifactToken(artifact.Path),
		State:                     state,
		QueryReady:                true,
		CaptureQualityIssue:       qualityIssue,
		CensusScope:               PerfCaptureCensusScopeRecordStreamOnly,
		DeviceCaptureCompleteness: PerfCaptureDeviceCompletenessNotClaimed,
		EffectiveClockEvidence:    "consult_bundle_perf_clock_alignments",
		PositiveEvidence:          "preserve",
		AbsencePolicy:             "require_capture_quality_caveat",
		AnalysisUse:               analysisUse,
		Capture:                   cloneRawPerfCaptureCompleteness(capture),
	}
	if !hasSamples {
		disclosure.State = PerfCaptureInventoryOnly
		disclosure.QueryReady = false
		disclosure.DeclaredProfileAlignment = capability.TimeAlignment
		disclosure.EffectiveClockEvidence = "none"
		disclosure.AnalysisUse = "capture_quality_diagnostics_only"
		disclosure.SampleAggregation = "none"
		disclosure.ClockAlignment = "none"
		disclosure.ThreadAttribution = "none"
		disclosure.RootCauseRank = "none"
	}
	return disclosure
}

// PerfCaptureDisclosures returns at most eight per-artifact rows plus one
// honest omitted=N marker. It preserves mixed ready/inventory states and
// fails duplicate raw artifact paths closed instead of folding their census.
func PerfCaptureDisclosures(artifacts []Artifact) []PerfCaptureDisclosure {
	rows := make([]PerfCaptureDisclosure, 0, min(len(artifacts), perfCaptureDisclosureLimit)+1)
	pathRows := make(map[string]int)
	seenRows := 0
	for _, artifact := range artifacts {
		disclosure := PerfCaptureDisclosureForArtifact(artifact)
		if !disclosure.Present {
			continue
		}
		seenRows++
		path := artifact.Path
		if path != "" && path == strings.TrimSpace(path) {
			if index, duplicate := pathRows[path]; duplicate {
				if index >= 0 {
					rows[index] = PerfCaptureDisclosure{Present: true, Reason: PerfCaptureInvalidDuplicatePath}
				}
				continue
			}
			pathRows[path] = -1
		}
		if len(rows) >= perfCaptureDisclosureLimit {
			continue
		}
		if path != "" && path == strings.TrimSpace(path) {
			pathRows[path] = len(rows)
		}
		rows = append(rows, disclosure)
	}
	if omitted := seenRows - len(rows); omitted > 0 {
		rows = append(rows, PerfCaptureDisclosure{Omitted: omitted})
	}
	return rows
}

// FormatPerfCaptureCompact emits one bounded single-line disclosure. State,
// scope and census tokens remain language-independent; only the lead label is
// localized.
func FormatPerfCaptureCompact(lang string, disclosure PerfCaptureDisclosure) string {
	lead := "Raw perf capture"
	if convertUseZh(lang) {
		lead = "Raw perf采集边界"
	}
	if disclosure.Omitted > 0 {
		return fmt.Sprintf("%s: raw_perf_capture omitted=%d", lead, disclosure.Omitted)
	}
	if !disclosure.Present {
		return ""
	}
	if !disclosure.Valid {
		return fmt.Sprintf("%s: raw_perf_capture valid=false reason=%s", lead, disclosure.Reason)
	}
	parts := perfCaptureMachineFields(disclosure)
	return lead + ": raw_perf_capture " + strings.Join(parts, " ")
}

// FormatPerfArtifactDetailFields preserves the established generic perf
// capability detail for every provider and then appends the raw typed boundary
// when applicable. Nonraw artifacts therefore never lose provider or
// symbolization detail merely because no raw disclosure is present.
func FormatPerfArtifactDetailFields(lang string, artifact Artifact) []string {
	disclosure := PerfCaptureDisclosureForArtifact(artifact)
	if disclosure.Present && !disclosure.Valid {
		return []string{
			"raw_perf_capture_valid=false",
			"raw_perf_capture_reason=" + string(disclosure.Reason),
		}
	}
	if artifact.Perf == nil {
		return nil
	}
	capability := artifact.Perf
	var fields []string
	add := func(key, value string) {
		if value != "" {
			fields = append(fields, perfCaptureDetailKV(lang, key, value))
		}
	}
	add("perf_provider", capability.ProviderName)
	add("perf_provider_kind", capability.ProviderKind)
	add("perf_input", capability.InputFormat)
	add("perf_symbolization", capability.Symbolization)
	add("perf_cpu", capability.CPUIdentity)
	add("perf_callchain", capability.Callchain)
	if !disclosure.Present {
		add("perf_time_alignment", capability.TimeAlignment)
	}
	add("trace_query_ready", perfCaptureBoolValue(lang, capability.TraceQueryReady))
	add("perf_degraded", perfCaptureBoolValue(lang, capability.Degraded))
	if !disclosure.Present {
		return fields
	}
	return append(fields, perfCaptureMachineFields(disclosure)...)
}

// FormatPerfCapturePromptBoundary returns an LLM-facing semantic boundary,
// never a command. The validation sentence is intentionally invariant across
// languages so prompt consumers can pin it mechanically.
func FormatPerfCapturePromptBoundary(lang string, disclosure PerfCaptureDisclosure) string {
	if disclosure.Omitted > 0 {
		return fmt.Sprintf("raw_perf_capture omitted=%d", disclosure.Omitted)
	}
	if !disclosure.Present {
		return ""
	}
	policy := "manifest disclosure numbers are not validated evidence; trace_query V2 three-face validation first"
	if !disclosure.Valid {
		return fmt.Sprintf("raw_perf_capture valid=false reason=%s; %s", disclosure.Reason, policy)
	}
	human := "Use positive query-ready samples, qualify absence conclusions, and obtain effective clock evidence only from bundle perf_clock_alignments."
	if disclosure.State == PerfCaptureInventoryOnly {
		human = "Use this inventory only for capture-quality diagnostics; it has no sample, clock, thread-attribution, or root-cause-ranking authority."
	}
	if convertUseZh(lang) {
		human = "可保留已观察到的可查询样本；缺失性结论必须携带采集质量限定，有效时钟证据只服从 bundle perf_clock_alignments。"
		if disclosure.State == PerfCaptureInventoryOnly {
			human = "该清单仅用于采集质量诊断，不具有样本聚合、时钟对齐、线程归因或根因排序权限。"
		}
	}
	return human + " " + policy + "; raw_perf_capture " + strings.Join(perfCaptureMachineFields(disclosure), " ")
}

// FormatPerfCaptureNextBoundary returns a semantic next-step fragment. Shell,
// CLI and REPL command syntax belongs to the consuming surface.
func FormatPerfCaptureNextBoundary(lang string, disclosure PerfCaptureDisclosure) string {
	if disclosure.Omitted > 0 {
		return fmt.Sprintf("next_boundary=inspect_omitted_raw_perf_artifacts omitted=%d", disclosure.Omitted)
	}
	if !disclosure.Present {
		return ""
	}
	if !disclosure.Valid {
		return "next_boundary=validate_tracequery_v2_three_faces raw_perf_capture_valid=false reason=" + string(disclosure.Reason)
	}
	fragment := "query-ready samples available; next_boundary=analyze_query_ready_perf_samples effective_clock_evidence=consult_bundle_perf_clock_alignments"
	if disclosure.State == PerfCaptureQueryReadyWithQualityIssue {
		fragment = "positive samples are queryable; absence/non-existence conclusions require a capture-quality note; next_boundary=analyze_positive_samples_with_capture_quality_caveat absence_conclusions=qualified effective_clock_evidence=consult_bundle_perf_clock_alignments"
	}
	if disclosure.State == PerfCaptureInventoryOnly {
		fragment = "conversion succeeded (quality inventory), not queryable; re-collect with query-ready perf samples; next_boundary=collect_or_convert_query_ready_perf_samples analysis_use=capture_quality_diagnostics_only"
	}
	if convertUseZh(lang) {
		switch disclosure.State {
		case PerfCaptureQueryReadyWithQualityIssue:
			fragment = "可查询正样本；缺失/不存在结论须附采集质量说明；next_boundary=analyze_positive_samples_with_capture_quality_caveat absence_conclusions=qualified effective_clock_evidence=consult_bundle_perf_clock_alignments"
		case PerfCaptureInventoryOnly:
			fragment = "转换成功（质量库存），不可查询；建议重新采集；next_boundary=collect_or_convert_query_ready_perf_samples analysis_use=capture_quality_diagnostics_only"
		default:
			fragment = "已有可查询样本；next_boundary=analyze_query_ready_perf_samples effective_clock_evidence=consult_bundle_perf_clock_alignments"
		}
		return "后续边界：" + fragment + " capture_state=" + string(disclosure.State)
	}
	return "Next boundary: " + fragment + " capture_state=" + string(disclosure.State)
}

func perfCaptureArtifactLooksRaw(artifact Artifact) bool {
	if artifact.Converter == rawPerfDataAdapterVersion {
		return true
	}
	if artifact.Perf == nil {
		return false
	}
	return artifact.Perf.RawCaptureCompleteness != nil ||
		artifact.Perf.ProviderKind == perfProviderKindRawFallback ||
		artifact.Perf.ProviderName == perfProviderNameRawFallback
}

func perfCaptureMachineFields(disclosure PerfCaptureDisclosure) []string {
	if !disclosure.Valid || disclosure.Capture == nil {
		return nil
	}
	capture := disclosure.Capture
	fields := []string{
		"valid=true",
		"artifact=" + disclosure.ArtifactPath,
		"capture_state=" + string(disclosure.State),
		fmt.Sprintf("query_ready=%t", disclosure.QueryReady),
		fmt.Sprintf("capture_quality_issue=%t", disclosure.CaptureQualityIssue),
		"census_scope=" + disclosure.CensusScope,
		"device_capture_completeness=" + disclosure.DeviceCaptureCompleteness,
		"positive_evidence=" + disclosure.PositiveEvidence,
		"absence_policy=" + disclosure.AbsencePolicy,
		"analysis_use=" + disclosure.AnalysisUse,
		"effective_clock_evidence=" + disclosure.EffectiveClockEvidence,
	}
	if disclosure.DeclaredProfileAlignment != "" {
		fields = append(fields, "declared_profile_time_alignment="+disclosure.DeclaredProfileAlignment)
	}
	for _, field := range [][2]string{
		{"sample_aggregation", disclosure.SampleAggregation},
		{"clock_alignment", disclosure.ClockAlignment},
		{"thread_attribution", disclosure.ThreadAttribution},
		{"root_cause_rank", disclosure.RootCauseRank},
	} {
		if field[1] != "" {
			fields = append(fields, field[0]+"="+field[1])
		}
	}
	return append(fields,
		perfCaptureRecordField("sample_records", capture.SampleRecords),
		perfCaptureRecordField("lost_records", capture.LostRecords),
		perfCaptureRecordField("lost_sample_records", capture.LostSampleRecords),
		perfCaptureRecordField("aux_records", capture.AuxRecords),
		perfCaptureAggregateField("lost_events", capture.LostEvents),
		perfCaptureAggregateField("lost_samples", capture.LostSamples),
		perfCaptureAggregateField("aux_bytes", capture.AuxBytes),
	)
}

func perfCaptureRecordField(name string, census RawPerfRecordCensus) string {
	return fmt.Sprintf("%s=physical:%d,accepted:%d,rejected:%d", name, census.Physical, census.Accepted, census.Rejected)
}

func perfCaptureAggregateField(name string, total RawPerfAggregateTotal) string {
	switch total.State {
	case rawPerfAggregateExact:
		return fmt.Sprintf("%s=exact:%d", name, total.Value)
	case rawPerfAggregateUnknown:
		return name + "=unknown:" + total.Reason
	default:
		return name + "=not_reported"
	}
}

func perfCaptureArtifactToken(path string) string {
	original := path
	leaf := pathpkg.Base(strings.ReplaceAll(path, "\\", "/"))
	var builder strings.Builder
	runeCount := 0
	for _, r := range leaf {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune("._-+@", r) {
			builder.WriteRune(r)
		} else {
			builder.WriteByte('_')
		}
		runeCount++
		if runeCount >= 64 {
			break
		}
	}
	if builder.Len() == 0 {
		builder.WriteString("artifact")
	}
	digest := sha256.Sum256([]byte(original))
	return fmt.Sprintf("%s@%x", builder.String(), digest[:8])
}

func perfCaptureDetailKV(lang, key, value string) string {
	if convertUseZh(lang) {
		switch key {
		case "perf_provider":
			key = "perf提供方"
		case "perf_provider_kind":
			key = "perf提供方类型"
		case "perf_input":
			key = "perf输入"
		case "perf_symbolization":
			key = "perf符号化"
		case "perf_cpu":
			key = "perfCPU信息"
		case "perf_callchain":
			key = "perf调用栈"
		case "perf_time_alignment":
			key = "perf时间对齐"
		case "trace_query_ready":
			key = "可供trace_query消费"
		case "perf_degraded":
			key = "perf降级"
		}
	}
	return key + "=" + value
}

func perfCaptureBoolValue(lang string, value bool) string {
	if !convertUseZh(lang) {
		return fmt.Sprintf("%t", value)
	}
	if value {
		return "是"
	}
	return "否"
}
