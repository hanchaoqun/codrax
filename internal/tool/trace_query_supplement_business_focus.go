package tool

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// These bytes are the actual core-view call, not a second reconstructed
// source/target/window tuple. The optional hook is a test-only wire witness.
var traceSupplementBusinessFocusParamsHook func([]byte)

func traceSupplementBusinessFocusParams(view string, ref types.TraceBusinessSpanRef) ([]byte, error) {
	return json.Marshal(struct {
		View string `json:"view"`
		Ref  string `json:"business_span_ref"`
	}{view, ref.Token()})
}

func traceSupplementHasExplicitUserTarget(ctx *types.BusContext) bool {
	if traceSupplementRequestedTargetProfile(ctx).NamedTarget() {
		return true
	}
	has := func(rm *types.RequestModel) bool {
		if rm == nil {
			return false
		}
		for _, target := range rm.RuntimeTargets {
			if strings.TrimSpace(target.Source) == "user_explicit" {
				return true
			}
		}
		return false
	}
	return ctx.AnalysisIR != nil && has(&ctx.AnalysisIR.RequestModel) || has(ctx.Mutable.RequestModel())
}

// User-authored coordinates keep the old explicit lane. An exact matching
// thread PID may use the reference without mixing fields; names and process
// IDs cannot grant this equivalence. A bounded business selector is not an
// explicit time window and can be resolved by an accepted native instance.
func traceSupplementAcceptedBusinessFocus(ctx *types.BusContext) (*types.TraceBusinessSpanRef, string) {
	scope := traceSupplementRequestedArtifactScope(ctx)
	if scope.HasExplicitTimeWindows() || scope.FullArtifact() || scope != nil && scope.TimeWindows != nil {
		return nil, ""
	}
	status, ref := ctx.Mutable.AcceptedTraceBusinessFocus()
	// An explicit target alone supplies no time window and cannot revive a
	// rejected business selection's earlier exploratory window.
	if status == types.TraceBusinessFocusConflict {
		return nil, types.TraceSupplementReasonWindowInconsistent
	}
	if status == types.TraceBusinessFocusInvalid {
		return nil, types.TraceSupplementReasonExecutionFailed
	}
	if traceSupplementHasExplicitUserTarget(ctx) {
		target, source, ok := traceSupplementDeriveTarget(ctx)
		if status != types.TraceBusinessFocusSelected || !ok || source != "user" || target.PID <= 0 || target.PID != ref.Data().TID ||
			(target.TargetScope != "" && target.TargetScope != "thread") {
			return nil, ""
		}
	}
	switch status {
	case types.TraceBusinessFocusSelected:
		if !ctx.Mutable.TraceBusinessSpanRefCurrent(ref) {
			return nil, types.TraceSupplementReasonExecutionFailed
		}
		return &ref, ""
	case types.TraceBusinessFocusConflict:
		return nil, types.TraceSupplementReasonWindowInconsistent
	case types.TraceBusinessFocusInvalid:
		return nil, types.TraceSupplementReasonExecutionFailed
	default:
		return nil, ""
	}
}

func traceSupplementBusinessFocusCurrent(ctx *types.BusContext, ref *types.TraceBusinessSpanRef) bool {
	if ref == nil {
		return true
	}
	status, active := ctx.Mutable.AcceptedTraceBusinessFocus()
	return status == types.TraceBusinessFocusSelected && active.Token() == ref.Token() && ctx.Mutable.TraceBusinessSpanRefCurrent(*ref)
}

// Recompilation starts only from native results carrying the same private
// physical-source generation. Public rows and serialized mirrors cannot
// recover that eligibility. Same-source source-wide census is a separate
// inventory; only the exact full instance window/TID can waive core views.
func traceSupplementBusinessFocusInput(input types.ObservationLedgerInput, ref types.TraceBusinessSpanRef, instance bool) types.ObservationLedgerInput {
	out := types.ObservationLedgerInput{RepoRoot: input.RepoRoot, RuntimeArtifactPreflight: input.RuntimeArtifactPreflight, RequestModel: input.RequestModel}
	d := ref.Data()
	for _, result := range append(append([]types.ToolResult(nil), input.ToolResults...), input.SystemTraceSupplementResults...) {
		if !types.TraceBusinessSpanResultSourceMatches(ref, result) {
			continue
		}
		copy := result
		copy.Observations = nil
		for _, record := range result.Observations {
			s := record.SourceRef
			if record.Origin != types.AnswerEvidenceOriginRuntimeArtifact || !types.RuntimeObservationProducerIsDeterministicQuery(record.Producer) ||
				s.Kind != types.ObservationSourceRuntimeArtifact || filepath.Clean(s.Path) != filepath.Clean(d.Path) {
				continue
			}
			if instance && (s.QueryScopeID == "" || s.PayloadRef == "" && s.RawRef == "" || !s.QueryWindowKnown || !s.QueryLineRangeKnown || s.QueryLineStart != 0 || s.QueryLineEnd != 0 ||
				s.QueryTargetPID != d.TID || s.QueryTargetScope != "thread" ||
				s.QueryWindowStartTs != d.StartTs || s.QueryWindowEndTs != d.EndTs) {
				continue
			}
			copy.Observations = append(copy.Observations, record)
		}
		if len(copy.Observations) != len(result.Observations) {
			copy.TraceEvidenceAuthority = nil
		}
		// An empty filtered result must not remint summary-derived facts.
		if len(copy.Observations) > 0 {
			out.ToolResults = append(out.ToolResults, copy)
		}
	}
	return out
}
