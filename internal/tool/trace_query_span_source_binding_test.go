package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDiscoveredSpanPhysicalSourceRequiresCompleteNativeMapping(t *testing.T) {
	for _, name := range []string{"complete", "partial_head", "partial_tail", "ambiguous", "isolated", "missing_map", "foreign_physical", "foreign_query", "foreign_support", "extra_support"} {
		t.Run(name, func(t *testing.T) {
			result := tracequery.Result{SourcePath: "/capture.json", TraceArtifacts: []tracequery.TraceArtifactSource{{
				SourcePath: "/member.ftrace", VirtualLineBase: 100, LocalLineCount: 10, CausalCompatible: true,
			}}}
			row := types.ObservationRecord{SourceRef: types.ObservationSourceRef{Path: result.SourcePath},
				Span: types.ObservationSpan{LineStart: 102, LineEnd: 108}, SupportRefs: []string{"/member.ftrace:2-8"}}
			span := tracequery.TraceSpanSummary{SourcePath: "/member.ftrace", StartLine: 102, EndLine: 108}
			switch name {
			case "partial_head":
				result.TraceArtifacts[0].VirtualLineBase = 104
			case "partial_tail":
				result.TraceArtifacts[0].LocalLineCount = 4
			case "ambiguous":
				result.TraceArtifacts = append(result.TraceArtifacts, result.TraceArtifacts[0])
			case "isolated":
				result.TraceArtifacts[0].CausalCompatible = false
			case "missing_map":
				result.TraceArtifacts = nil
			case "foreign_physical":
				span.SourcePath = "/other.ftrace"
			case "foreign_query":
				row.SourceRef.Path = "/other-capture.json"
			case "foreign_support":
				row.SupportRefs[0] = "/other.ftrace:2-8"
			case "extra_support":
				row.SupportRefs = append(row.SupportRefs, "/other.ftrace:2-8")
			}
			physical, ok := traceQueryDiscoveredSpanPhysicalSource(result, row, span)
			if ok != (name == "complete") {
				t.Fatalf("mapping eligibility=%v, source=%+v", ok, physical)
			}
			if ok && (physical.SourcePath != "/member.ftrace" || physical.LocalLineStart != 2 || physical.LocalLineEnd != 8) {
				t.Fatalf("virtual coordinates were published as physical-local coordinates: %+v", physical)
			}
		})
	}
}
