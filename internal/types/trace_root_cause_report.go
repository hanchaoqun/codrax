package types

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// TraceRootCauseReportSchemaVersion is the user-facing JSON sidecar schema.
//
// Version 2 replaces the fixed root_cause_1/root_cause_2 pair with an
// evidence-sized root_causes array and adds a required impact_seconds value
// to every emitted cause.
const TraceRootCauseReportSchemaVersion = 2

// Shared by the frozen-fact binder and the public v2 validator. Packing may
// use more entries, but must not widen these limits or truncate references.
const (
	TraceRootCauseEvidenceMaxEntries = 4
	TraceRootCauseEvidenceMaxRunes   = 240
)

// TraceRootCauseCategory is the closed, stable root-cause vocabulary exposed
// in the JSON sidecar. Evidence is bound from typed candidate facts.
type TraceRootCauseCategory string

const (
	TraceRootCauseIOBlocking            TraceRootCauseCategory = "io_blocking"
	TraceRootCauseLockContention        TraceRootCauseCategory = "lock_contention"
	TraceRootCauseSynchronousBinder     TraceRootCauseCategory = "synchronous_binder"
	TraceRootCausePriorityInversion     TraceRootCauseCategory = "priority_inversion"
	TraceRootCauseGCLongPause           TraceRootCauseCategory = "gc_long_pause"
	TraceRootCauseCPUSchedulingDelay    TraceRootCauseCategory = "cpu_scheduling_delay"
	TraceRootCausePhaseHighLoad         TraceRootCauseCategory = "phase_high_load"
	TraceRootCauseJITCompilation        TraceRootCauseCategory = "jit_compilation"
	TraceRootCauseShaderCompilation     TraceRootCauseCategory = "shader_compilation"
	TraceRootCauseSleepBlocking         TraceRootCauseCategory = "sleep_blocking"
	TraceRootCauseComputeSupplyShortage TraceRootCauseCategory = "compute_supply_shortage"
)

func AllTraceRootCauseCategories() []TraceRootCauseCategory {
	return []TraceRootCauseCategory{
		TraceRootCauseIOBlocking,
		TraceRootCauseLockContention,
		TraceRootCauseSynchronousBinder,
		TraceRootCausePriorityInversion,
		TraceRootCauseGCLongPause,
		TraceRootCauseCPUSchedulingDelay,
		TraceRootCausePhaseHighLoad,
		TraceRootCauseJITCompilation,
		TraceRootCauseShaderCompilation,
		TraceRootCauseSleepBlocking,
		TraceRootCauseComputeSupplyShortage,
	}
}

// TraceRootCauseItemV2 is one structured conclusion plus its corresponding
// concise evidence. Summary is normalized by the runtime from Category and
// the relevant identity field so the wire format stays consistent. Rank is
// also runtime-owned: array order is the model's strongest-to-weakest order,
// and normalization writes contiguous 1-based ranks.
type TraceRootCauseItemV2 struct {
	// CandidateID is accepted only on the model-to-runtime selector wire. The
	// binder clears it before persistence, so the public sidecar remains the
	// stable user-facing v2 shape rather than leaking an internal receipt key.
	CandidateID   string                 `json:"candidate_id,omitempty"`
	Rank          int                    `json:"rank"`
	Category      TraceRootCauseCategory `json:"category"`
	ThreadName    string                 `json:"thread_name,omitempty"`
	ResourceName  string                 `json:"resource_name,omitempty"`
	PhaseName     string                 `json:"phase_name,omitempty"`
	ImpactSeconds *float64               `json:"impact_seconds"`
	// ImpactCaliber (SIDECAR-Q1, §40.28 ②) names the ruler behind
	// impact_seconds — "effective_attribution" (the engine-published effective
	// attribution) or "window_projection" (the raw window projection of a seat
	// whose effective was never published). Always explicit; append-only v2
	// extension (schema_version stays 2, consumers ignore unknown keys).
	ImpactCaliber string `json:"impact_caliber"`
	// CausalQualifier (SIDECAR-Q1, §40.28 ②; QUALGATE-1, §40.30): closed set
	// "proven" / "frame_unproven" / "not_applicable" (AllTraceCausalQualifiers),
	// seat-level, sourced from the same gated evidence-ID authority as the
	// Markdown headline qualifier 「（帧因果未证）」; not_applicable = the
	// request is not a frame/jank question per the analyzer's typed decision.
	// Always explicit.
	CausalQualifier string   `json:"causal_qualifier"`
	Summary         string   `json:"summary"`
	Evidence        []string `json:"evidence"`
}

// TraceImpactCaliber values carried on the public sidecar — closed set.
const (
	TraceImpactCaliberEffectiveAttribution = "effective_attribution"
	TraceImpactCaliberWindowProjection     = "window_projection"
)

// ValidTraceImpactCaliber reports closed-set membership.
func ValidTraceImpactCaliber(v string) bool {
	switch v {
	case TraceImpactCaliberEffectiveAttribution, TraceImpactCaliberWindowProjection:
		return true
	}
	return false
}

// TraceRootCauseReportV2 is written next to the full Markdown/HTML answer.
// RootCauses is evidence-sized: the finalizer emits every independently
// supported, quantitatively grounded cause in strongest-to-weakest order.
// An empty array is the honest representation when no such cause exists.
type TraceRootCauseReportV2 struct {
	SchemaVersion int                     `json:"schema_version"`
	RootCauses    []*TraceRootCauseItemV2 `json:"root_causes"`
}

// NormalizeAndValidateTraceRootCauseReport validates an already bound report,
// compacts evidence whitespace, and owns the fixed Chinese summary spelling.
// It never derives a cause from the long answer prose.
func NormalizeAndValidateTraceRootCauseReport(in *TraceRootCauseReportV2) (*TraceRootCauseReportV2, error) {
	if in == nil {
		return nil, fmt.Errorf("trace_root_causes is required")
	}
	if in.SchemaVersion != TraceRootCauseReportSchemaVersion {
		return nil, fmt.Errorf("trace_root_causes schema_version=%d, want %d", in.SchemaVersion, TraceRootCauseReportSchemaVersion)
	}
	out := &TraceRootCauseReportV2{
		SchemaVersion: TraceRootCauseReportSchemaVersion,
		RootCauses:    make([]*TraceRootCauseItemV2, 0, len(in.RootCauses)),
	}
	seenCauses := make(map[traceRootCauseIdentity]int, len(in.RootCauses))
	for index, cause := range in.RootCauses {
		field := fmt.Sprintf("root_causes[%d]", index)
		normalized, err := normalizeTraceRootCauseItem(cause, field)
		if err != nil {
			return nil, err
		}
		normalized.Rank = index + 1
		identity := traceRootCauseIdentityKey(normalized)
		if previous, duplicate := seenCauses[identity]; duplicate {
			return nil, fmt.Errorf("trace_root_causes.%s duplicates root_causes[%d]", field, previous)
		}
		seenCauses[identity] = index
		out.RootCauses = append(out.RootCauses, normalized)
	}
	return out, nil
}

// Display summaries deliberately collapse details (e.g. "供给不足"). They
// cannot identify an independently selected cause. Bound candidate receipts
// preserve that identity even across equal categories/subjects; the binder
// strips the private id only after validation. Legacy already-bound reports
// without receipts retain conservative typed identity checks, not prose keys.
type traceRootCauseIdentity struct {
	candidateID             string
	category                TraceRootCauseCategory
	thread, resource, phase string
}

func traceRootCauseIdentityKey(item *TraceRootCauseItemV2) traceRootCauseIdentity {
	if item.CandidateID != "" {
		return traceRootCauseIdentity{candidateID: item.CandidateID}
	}
	return traceRootCauseIdentity{category: item.Category, thread: item.ThreadName, resource: item.ResourceName, phase: item.PhaseName}
}

func normalizeTraceRootCauseItem(in *TraceRootCauseItemV2, field string) (*TraceRootCauseItemV2, error) {
	if in == nil {
		return nil, fmt.Errorf("trace_root_causes.%s is null", field)
	}
	out := &TraceRootCauseItemV2{
		CandidateID:     compactTraceRootCauseField(in.CandidateID),
		Category:        in.Category,
		ThreadName:      compactTraceRootCauseField(in.ThreadName),
		ResourceName:    compactTraceRootCauseField(in.ResourceName),
		PhaseName:       compactTraceRootCauseField(in.PhaseName),
		ImpactCaliber:   strings.TrimSpace(in.ImpactCaliber),
		CausalQualifier: strings.TrimSpace(in.CausalQualifier),
	}
	// SIDECAR-Q1 (§40.28 ②): both qualifiers are closed-set and REQUIRED on
	// every bound item — a consumer never infers them from absence.
	if !ValidTraceImpactCaliber(out.ImpactCaliber) {
		return nil, fmt.Errorf("trace_root_causes.%s.impact_caliber=%q is unsupported", field, out.ImpactCaliber)
	}
	if !ValidTraceCausalQualifier(out.CausalQualifier) {
		return nil, fmt.Errorf("trace_root_causes.%s.causal_qualifier=%q is unsupported", field, out.CausalQualifier)
	}
	if !validTraceRootCauseCategory(out.Category) {
		return nil, fmt.Errorf("trace_root_causes.%s.category=%q is unsupported", field, out.Category)
	}
	if traceRootCauseNeedsThread(out.Category) && out.ThreadName == "" {
		return nil, fmt.Errorf("trace_root_causes.%s.thread_name is required for category %q", field, out.Category)
	}
	if out.Category == TraceRootCauseLockContention && out.ResourceName == "" {
		return nil, fmt.Errorf("trace_root_causes.%s.resource_name is required for lock_contention", field)
	}
	if out.Category == TraceRootCausePhaseHighLoad && out.PhaseName == "" {
		return nil, fmt.Errorf("trace_root_causes.%s.phase_name is required for phase_high_load", field)
	}
	if in.ImpactSeconds == nil {
		return nil, fmt.Errorf("trace_root_causes.%s.impact_seconds is required", field)
	}
	impactSeconds := *in.ImpactSeconds
	if math.IsNaN(impactSeconds) || math.IsInf(impactSeconds, 0) || impactSeconds <= 0 {
		return nil, fmt.Errorf("trace_root_causes.%s.impact_seconds must be a finite positive number", field)
	}
	out.ImpactSeconds = &impactSeconds
	if len(in.Evidence) == 0 || len(in.Evidence) > TraceRootCauseEvidenceMaxEntries {
		return nil, fmt.Errorf("trace_root_causes.%s.evidence must contain 1 to %d concise entries", field, TraceRootCauseEvidenceMaxEntries)
	}
	for index, evidence := range in.Evidence {
		evidence = compactTraceRootCauseField(evidence)
		if evidence == "" {
			return nil, fmt.Errorf("trace_root_causes.%s.evidence[%d] is empty", field, index)
		}
		if utf8.RuneCountInString(evidence) > TraceRootCauseEvidenceMaxRunes {
			return nil, fmt.Errorf("trace_root_causes.%s.evidence[%d] exceeds %d characters", field, index, TraceRootCauseEvidenceMaxRunes)
		}
		out.Evidence = append(out.Evidence, evidence)
	}
	out.Summary = traceRootCauseSummary(*out)
	if out.CausalQualifier == TraceCausalQualifierFrameUnproven && out.Summary != "" {
		// Same words as the Markdown headline qualifier (§7.3 T3-1 ruling).
		out.Summary += TraceCausalQualifierFrameUnprovenSuffixZH
	}
	return out, nil
}

func validTraceRootCauseCategory(category TraceRootCauseCategory) bool {
	for _, candidate := range AllTraceRootCauseCategories() {
		if category == candidate {
			return true
		}
	}
	return false
}

func traceRootCauseNeedsThread(category TraceRootCauseCategory) bool {
	switch category {
	case TraceRootCauseIOBlocking,
		TraceRootCauseSynchronousBinder,
		TraceRootCausePriorityInversion,
		TraceRootCauseCPUSchedulingDelay,
		TraceRootCauseJITCompilation,
		TraceRootCauseShaderCompilation,
		TraceRootCauseSleepBlocking:
		return true
	default:
		return false
	}
}

func traceRootCauseSummary(item TraceRootCauseItemV2) string {
	switch item.Category {
	case TraceRootCauseIOBlocking:
		return item.ThreadName + "线程IO阻塞"
	case TraceRootCauseLockContention:
		return item.ResourceName + "锁竞争"
	case TraceRootCauseSynchronousBinder:
		return item.ThreadName + "线程同步binder"
	case TraceRootCausePriorityInversion:
		return item.ThreadName + "线程优先级反转"
	case TraceRootCauseGCLongPause:
		return "GC耗时长"
	case TraceRootCauseCPUSchedulingDelay:
		return item.ThreadName + "线程CPU调度延迟"
	case TraceRootCausePhaseHighLoad:
		return item.PhaseName + "阶段高负载"
	case TraceRootCauseJITCompilation:
		return item.ThreadName + "线程JIT编译耗时"
	case TraceRootCauseShaderCompilation:
		return item.ThreadName + "线程Shader编译"
	case TraceRootCauseSleepBlocking:
		return item.ThreadName + "线程阻塞"
	case TraceRootCauseComputeSupplyShortage:
		return "供给不足"
	default:
		return ""
	}
}

func compactTraceRootCauseField(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}

func cloneTraceRootCauseReportV2(in *TraceRootCauseReportV2) *TraceRootCauseReportV2 {
	if in == nil {
		return nil
	}
	out := *in
	out.RootCauses = make([]*TraceRootCauseItemV2, len(in.RootCauses))
	for index, item := range in.RootCauses {
		if item == nil {
			continue
		}
		cause := *item
		cause.Evidence = append([]string(nil), item.Evidence...)
		if item.ImpactSeconds != nil {
			impactSeconds := *item.ImpactSeconds
			cause.ImpactSeconds = &impactSeconds
		}
		out.RootCauses[index] = &cause
	}
	return &out
}
