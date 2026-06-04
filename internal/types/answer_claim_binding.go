package types

import (
	"fmt"
	"strings"
)

// AnswerClaimBinding is the origin-specific handoff consumed by answer-writing
// stages. It keeps the model-authored aggregate fact, evidence origin, visible
// output shape, and grounding policy in one deterministic record so downstream
// code does not independently reinterpret history/count/runtime/source facts.
type AnswerClaimBinding struct {
	ClaimID            string
	Source             string
	AggregateIndex     int
	AggregateKind      AnswerAggregateKind
	AggregateRole      AnswerAggregateRole
	Label              string
	Value              string
	TargetRef          string
	Origin             AnswerEvidenceOrigin
	RequestedOutputs   []AnswerRequestedOutput
	SupportRefs        []string
	ExactSourceSupport bool
	AuthorityCeiling   AuthorityCeiling
	DriftReason        DriftReason
	GroundingPolicy    ClaimGroundingPolicy
}

func CompileAnswerClaimBindings(facts []AnswerAggregateFact, rm *RequestModel, answerContract *AnswerContract) []AnswerClaimBinding {
	out := CompileAnswerClaimBindingsFromAggregateFacts(facts, rm, answerContract)
	out = append(out, CompileRuntimeArtifactClaimBindings(rm, answerContract)...)
	return out
}

// CompileAnswerClaimBindingsFromAggregateFacts projects stable
// emit_investigation_complete.aggregate_facts into origin-specific claim
// bindings. It consumes typed aggregate dimensions and request model fields
// only; it does not inspect raw user prose or model free text.
func CompileAnswerClaimBindingsFromAggregateFacts(facts []AnswerAggregateFact, rm *RequestModel, answerContract *AnswerContract) []AnswerClaimBinding {
	if len(facts) == 0 {
		return nil
	}
	requestContract := AnswerIntentContract{}
	if rm != nil {
		var contract *AnswerContract
		if answerContract != nil {
			contract = answerContract
		}
		requestContract = CompileAnswerIntentContract(*rm, contract)
	}
	outputs := requestContract.RequestedOutputs
	if len(outputs) == 0 {
		outputs = []AnswerRequestedOutput{AnswerRequestedOutputSummary}
	}
	out := make([]AnswerClaimBinding, 0, len(facts))
	for idx, fact := range facts {
		role := AnswerAggregateFactRoleForRequest(fact, rm)
		origins := AnswerAggregateFactEvidenceOrigins(fact, rm)
		if len(origins) == 0 {
			origins = []AnswerEvidenceOrigin{AnswerEvidenceOriginCurrentSource}
		}
		for _, origin := range origins {
			if origin == AnswerEvidenceOriginUnknown || !origin.IsValid() {
				continue
			}
			exactCurrentSourceSupport := origin == AnswerEvidenceOriginCurrentSource && answerAggregateFactHasExactCurrentSourceSupport(fact)
			policy := AnswerClaimBindingGroundingPolicy(origin, role)
			if origin == AnswerEvidenceOriginCurrentSource &&
				policy == ClaimGroundingHard &&
				!exactCurrentSourceSupport {
				policy = ClaimGroundingRepairable
			}
			out = append(out, AnswerClaimBinding{
				ClaimID:            answerClaimBindingID(idx, origin),
				Source:             "aggregate_facts",
				AggregateIndex:     idx,
				AggregateKind:      fact.Kind,
				AggregateRole:      role,
				Label:              fact.Label,
				Value:              fact.Value,
				TargetRef:          answerClaimBindingTargetRef(fact),
				Origin:             origin,
				RequestedOutputs:   cloneAnswerRequestedOutputs(outputs),
				SupportRefs:        cloneAnswerClaimBindingStrings(fact.SupportRefs),
				ExactSourceSupport: exactCurrentSourceSupport,
				AuthorityCeiling:   AnswerClaimBindingDefaultAuthorityCeiling(origin, exactCurrentSourceSupport),
				GroundingPolicy:    policy,
			})
		}
	}
	return out
}

// CompileRuntimeArtifactClaimBindings projects already-validated log/perf
// bundles into runtime-artifact claim bindings. These bindings are intentionally
// separate from current-source citations: artifact frames and durations are
// valid observations even when no frame resolves to the active checkout.
func CompileRuntimeArtifactClaimBindings(rm *RequestModel, answerContract *AnswerContract) []AnswerClaimBinding {
	if rm == nil {
		return nil
	}
	requestContract := CompileAnswerIntentContract(*rm, answerContract)
	outputs := requestContract.RequestedOutputs
	if len(outputs) == 0 {
		outputs = []AnswerRequestedOutput{AnswerRequestedOutputSummary}
	}
	var out []AnswerClaimBinding
	if rm.LogTriage != nil {
		out = append(out, logBundleClaimBindings(rm.LogTriage, outputs)...)
	}
	if rm.PerfTrace != nil {
		out = append(out, perfBundleClaimBindings(rm.PerfTrace, outputs)...)
	}
	return out
}

func AnswerClaimBindingGroundingPolicy(origin AnswerEvidenceOrigin, role AnswerAggregateRole) ClaimGroundingPolicy {
	principal := NormalizeAnswerAggregateRole(role).IsPrincipal()
	switch origin {
	case AnswerEvidenceOriginCurrentSource:
		if principal {
			return ClaimGroundingHard
		}
		return ClaimGroundingRepairable
	case AnswerEvidenceOriginRepoNegativeSearch,
		AnswerEvidenceOriginCommandMeasurement:
		if principal {
			return ClaimGroundingHard
		}
		return ClaimGroundingSoft
	case AnswerEvidenceOriginVCSMetadata,
		AnswerEvidenceOriginVCSDiff,
		AnswerEvidenceOriginRuntimeArtifact,
		AnswerEvidenceOriginCrossRepoIndex,
		AnswerEvidenceOriginExternalDocument,
		AnswerEvidenceOriginWebPage,
		AnswerEvidenceOriginMCPResource,
		AnswerEvidenceOriginConnectorResource:
		if principal {
			return ClaimGroundingRepairable
		}
		return ClaimGroundingSoft
	case AnswerEvidenceOriginSystemInference:
		if principal {
			return ClaimGroundingSoft
		}
		return ClaimGroundingDisplayOnly
	default:
		return ClaimGroundingSoft
	}
}

// AnswerClaimBindingDefaultAuthorityCeiling returns the default authority cap
// for a claim binding when no per-evidence authority axis is available. The
// value is intentionally origin-based and typed-only: runtime and VCS facts
// remain valid observations/history, but they cannot by themselves prove the
// current checkout's causal path. Current-source bindings only get factual
// authority when they carry exact current-source support.
func AnswerClaimBindingDefaultAuthorityCeiling(origin AnswerEvidenceOrigin, exactCurrentSourceSupport bool) AuthorityCeiling {
	switch origin {
	case AnswerEvidenceOriginCurrentSource:
		if exactCurrentSourceSupport {
			return AuthorityFactual
		}
		return AuthorityUnknown
	case AnswerEvidenceOriginRuntimeArtifact,
		AnswerEvidenceOriginVCSMetadata,
		AnswerEvidenceOriginVCSDiff:
		return AuthorityHistorical
	case AnswerEvidenceOriginSystemInference:
		return AuthorityIllustrative
	case AnswerEvidenceOriginRepoNegativeSearch,
		AnswerEvidenceOriginCommandMeasurement,
		AnswerEvidenceOriginCrossRepoIndex,
		AnswerEvidenceOriginExternalDocument,
		AnswerEvidenceOriginWebPage,
		AnswerEvidenceOriginMCPResource,
		AnswerEvidenceOriginConnectorResource:
		return AuthorityFactual
	default:
		return AuthorityUnknown
	}
}

func AnswerClaimBindingAuthorityCeiling(binding AnswerClaimBinding) AuthorityCeiling {
	if binding.AuthorityCeiling != "" {
		return binding.AuthorityCeiling
	}
	return AnswerClaimBindingDefaultAuthorityCeiling(
		binding.Origin,
		AnswerClaimBindingHasExactCurrentSourceSupport(binding),
	)
}

// AnswerClaimBindingHasExactCurrentSourceSupport reports whether a compiled
// claim binding carries an explicit current-checkout source location. It is a
// citation-eligibility helper, not an evidence classifier.
func AnswerClaimBindingHasExactCurrentSourceSupport(binding AnswerClaimBinding) bool {
	if binding.Origin != AnswerEvidenceOriginCurrentSource {
		return false
	}
	if binding.ExactSourceSupport {
		return true
	}
	for _, ref := range binding.SupportRefs {
		if answerSupportRefHasSourceLine(ref) {
			return true
		}
	}
	return false
}

func AnswerClaimBindingCarriesOriginSpecificSupport(binding AnswerClaimBinding) bool {
	return AnswerEvidenceOriginCarriesOriginSpecificSupport(binding.Origin)
}

func AnswerClaimBindingsHaveOnlyOriginSpecificSupport(bindings []AnswerClaimBinding) bool {
	hasOriginSpecific := false
	for _, binding := range bindings {
		if binding.Origin == AnswerEvidenceOriginUnknown {
			continue
		}
		if !AnswerClaimBindingCarriesOriginSpecificSupport(binding) {
			return false
		}
		hasOriginSpecific = true
	}
	return hasOriginSpecific
}

func AnswerClaimBindingsRequestExactOutput(bindings []AnswerClaimBinding) bool {
	for _, binding := range bindings {
		for _, output := range binding.RequestedOutputs {
			if AnswerRequestedOutputIsExactValueShape(output) {
				return true
			}
		}
	}
	return false
}

func AnswerRequestedOutputIsExactValueShape(output AnswerRequestedOutput) bool {
	switch output {
	case AnswerRequestedOutputScalar,
		AnswerRequestedOutputKeyValue,
		AnswerRequestedOutputCount,
		AnswerRequestedOutputAbsence:
		return true
	default:
		return false
	}
}

func answerAggregateFactHasExactCurrentSourceSupport(fact AnswerAggregateFact) bool {
	for _, ref := range fact.SupportRefs {
		if answerSupportRefHasSourceLine(ref) {
			return true
		}
	}
	for _, member := range fact.Members {
		if answerSupportRefHasSourceLine(member) {
			return true
		}
	}
	return false
}

func answerSupportRefHasSourceLine(raw string) bool {
	_, loc, ok := ParseAnswerSupportRefMemberLocation(raw)
	return ok && strings.TrimSpace(loc.File) != "" && loc.LineStart > 0
}

func answerClaimBindingID(index int, origin AnswerEvidenceOrigin) string {
	return fmt.Sprintf("aggregate_facts[%d]#%s", index, origin)
}

func runtimeArtifactClaimBindingID(source string, index int) string {
	return fmt.Sprintf("%s[%d]#%s", source, index, AnswerEvidenceOriginRuntimeArtifact)
}

func answerClaimBindingTargetRef(fact AnswerAggregateFact) string {
	if fact.Kind == AnswerAggregateNegativeSearch || fact.Kind == AnswerAggregateNegativeObservation {
		if target := answerClaimBindingNegativeTargetRef(fact); target != "" {
			return target
		}
	}
	label := strings.TrimSpace(fact.Label)
	if label != "" {
		return label
	}
	if len(fact.Members) > 0 {
		return strings.TrimSpace(fact.Members[0])
	}
	return strings.TrimSpace(fact.Value)
}

func answerClaimBindingNegativeTargetRef(fact AnswerAggregateFact) string {
	dims := aggregateDimensionMap(fact.Dimensions)
	target := firstNonEmptyAggregateDim(dims, "target", "query", "pattern", "predicate")
	scope := strings.TrimSpace(dims["scope"])
	if target == "" {
		return ""
	}
	if scope != "" {
		return target + " @ " + scope
	}
	return target
}

func cloneAnswerRequestedOutputs(in []AnswerRequestedOutput) []AnswerRequestedOutput {
	if len(in) == 0 {
		return nil
	}
	out := make([]AnswerRequestedOutput, len(in))
	copy(out, in)
	return out
}

func cloneAnswerClaimBindingStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func logBundleClaimBindings(bundle *LogBundle, outputs []AnswerRequestedOutput) []AnswerClaimBinding {
	if bundle == nil {
		return nil
	}
	var out []AnswerClaimBinding
	add := func(target string, support []string) {
		target = strings.TrimSpace(target)
		if target == "" {
			return
		}
		out = append(out, AnswerClaimBinding{
			ClaimID:          runtimeArtifactClaimBindingID("log_triage", len(out)),
			Source:           "log_triage",
			AggregateIndex:   -1,
			Label:            target,
			TargetRef:        target,
			Origin:           AnswerEvidenceOriginRuntimeArtifact,
			RequestedOutputs: cloneAnswerRequestedOutputs(outputs),
			SupportRefs:      cloneAnswerClaimBindingStrings(support),
			AuthorityCeiling: AnswerClaimBindingDefaultAuthorityCeiling(AnswerEvidenceOriginRuntimeArtifact, false),
			GroundingPolicy:  AnswerClaimBindingGroundingPolicy(AnswerEvidenceOriginRuntimeArtifact, AnswerAggregateRolePrincipalAnswer),
		})
	}
	var walk func(err LogError)
	walk = func(err LogError) {
		target := strings.TrimSpace(err.Type)
		if target == "" {
			target = strings.TrimSpace(err.Message)
		}
		add(target, logFrameSupportRefs(err.Frames))
		if err.Cause != nil {
			walk(*err.Cause)
		}
	}
	for _, err := range bundle.Errors {
		walk(err)
	}
	for _, obs := range bundle.Observations {
		target := strings.TrimSpace(obs.Subject)
		if target == "" {
			target = strings.TrimSpace(string(obs.Kind))
		}
		var support []string
		var parts []string
		if obs.LineStart > 0 {
			if obs.LineEnd > obs.LineStart {
				parts = append(parts, fmt.Sprintf("log_lines=%d-%d", obs.LineStart, obs.LineEnd))
			} else {
				parts = append(parts, fmt.Sprintf("log_line=%d", obs.LineStart))
			}
		}
		if obs.Summary != "" {
			parts = append(parts, "summary="+obs.Summary)
		}
		if len(parts) > 0 {
			support = append(support, strings.Join(parts, " "))
		}
		if obs.Evidence != "" {
			support = append(support, strings.TrimSpace(obs.Evidence))
		}
		add(target, support)
	}
	return out
}

func logFrameSupportRefs(frames []LogFrame) []string {
	out := make([]string, 0, len(frames))
	for _, frame := range frames {
		ref := strings.TrimSpace(frame.Raw)
		if ref == "" && frame.File != "" {
			if frame.Line > 0 {
				ref = fmt.Sprintf("%s:%d", frame.File, frame.Line)
			} else {
				ref = frame.File
			}
		}
		if ref != "" {
			out = append(out, ref)
		}
	}
	return out
}

func firstNonEmptyAnswerString(values ...string) string {
	for _, value := range values {
		if s := strings.TrimSpace(value); s != "" {
			return s
		}
	}
	return ""
}

func perfBundleClaimBindings(bundle *PerfBundle, outputs []AnswerRequestedOutput) []AnswerClaimBinding {
	if bundle == nil {
		return nil
	}
	var out []AnswerClaimBinding
	add := func(target string, support []string) {
		target = strings.TrimSpace(target)
		if target == "" {
			return
		}
		out = append(out, AnswerClaimBinding{
			ClaimID:          runtimeArtifactClaimBindingID("perf_trace", len(out)),
			Source:           "perf_trace",
			AggregateIndex:   -1,
			Label:            target,
			TargetRef:        target,
			Origin:           AnswerEvidenceOriginRuntimeArtifact,
			RequestedOutputs: cloneAnswerRequestedOutputs(outputs),
			SupportRefs:      cloneAnswerClaimBindingStrings(support),
			AuthorityCeiling: AnswerClaimBindingDefaultAuthorityCeiling(AnswerEvidenceOriginRuntimeArtifact, false),
			GroundingPolicy:  AnswerClaimBindingGroundingPolicy(AnswerEvidenceOriginRuntimeArtifact, AnswerAggregateRolePrincipalAnswer),
		})
	}
	for _, frame := range bundle.Frames {
		target := fmt.Sprintf("frame #%d", frame.FrameNo)
		if frame.FrameNo == 0 {
			target = "frame"
		}
		add(target, []string{fmt.Sprintf("duration_ms=%.3f phase=%s janky=%t", frame.DurationMs, frame.Phase, frame.Janky)})
	}
	for _, jank := range bundle.Janks {
		target := jank.TriggerSpan
		if target == "" {
			target = "jank span"
		}
		add(target, []string{fmt.Sprintf("start_ts_ms=%.3f duration_ms=%.3f reason=%s", jank.StartTsMs, jank.DurationMs, jank.Reason)})
	}
	for _, stall := range bundle.Stalls {
		target := stall.Symbol
		if target == "" {
			target = stall.Kind
		}
		add(target, []string{fmt.Sprintf("start_ts_ms=%.3f duration_ms=%.3f file=%s:%d", stall.StartTsMs, stall.DurationMs, stall.File, stall.Line)})
	}
	for _, obs := range bundle.Observations {
		target := firstNonEmptyAnswerString(obs.Subject, obs.Kind, "trace observation")
		support := []string{}
		var parts []string
		if obs.LineStart > 0 {
			if obs.LineEnd > obs.LineStart {
				parts = append(parts, fmt.Sprintf("trace_lines=%d-%d", obs.LineStart, obs.LineEnd))
			} else {
				parts = append(parts, fmt.Sprintf("trace_line=%d", obs.LineStart))
			}
		}
		if obs.DurationMs > 0 {
			parts = append(parts, fmt.Sprintf("duration_ms=%.3f", obs.DurationMs))
		}
		if obs.Summary != "" {
			parts = append(parts, "summary="+obs.Summary)
		}
		if len(parts) > 0 {
			support = append(support, strings.Join(parts, " "))
		}
		add(target, support)
	}
	if bundle.Startup != nil {
		add("startup "+bundle.Startup.Mode, []string{fmt.Sprintf("app_launch_ms=%.3f ability_init_ms=%.3f first_frame_ms=%.3f", bundle.Startup.AppLaunchMs, bundle.Startup.AbilityInitMs, bundle.Startup.FirstFrameMs)})
	}
	return out
}
