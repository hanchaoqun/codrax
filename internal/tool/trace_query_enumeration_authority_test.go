package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryEnumerationAuthorityCarriesUnknownTotal(t *testing.T) {
	result := tracequery.Result{
		View: "event_search",
		Compactions: []tracequery.ViewCompaction{{
			View: "event_search", Dimension: tracequery.CompactionDimensionEvents, Emitted: 8,
		}},
	}
	enumeration := traceQueryEnumerationAuthority(result)
	if enumeration == nil || enumeration.Status != "incomplete" ||
		len(enumeration.Boundaries) != 1 ||
		enumeration.Boundaries[0].Scope != "event_search" ||
		enumeration.Boundaries[0].Emitted != 8 ||
		enumeration.Boundaries[0].TotalKnown {
		t.Fatalf("enumeration authority drifted: %+v", enumeration)
	}
}

func TestTraceCausalCoverageBlockPublishesEnumerationCeiling(t *testing.T) {
	input := types.ObservationLedgerInput{ToolResults: []types.ToolResult{{
		ToolName: "trace_query",
		Success:  true,
		EnumerationAuthority: &types.ToolEnumerationAuthority{
			Status: "incomplete",
			Boundaries: []types.ToolEnumerationBoundary{{
				Scope: "event_search", Dimension: "events", Emitted: 8,
			}},
		},
	}}}
	block := runtimeTraceCausalProjectionCoverageBlock(input, "zh")
	if block == nil {
		t.Fatal("typed enumeration boundary must create a deterministic coverage block")
	}
	for _, want := range []string{
		"enumeration_status=incomplete",
		"event_search/events:emitted=8,total=unknown",
		"只能作为样本或下界",
		"全部/仅有/总计/共N/最大/最小",
	} {
		if !strings.Contains(block.Text, want) {
			t.Fatalf("coverage block missing %q:\n%s", want, block.Text)
		}
	}
}

func TestRuntimeArtifactReadEnumerationAuthorityIsNotACompleteCensus(t *testing.T) {
	enumeration := readFileEnumerationAuthority("customer@trace.sys", 2, 4, 10, false)
	if enumeration == nil || enumeration.Status != "incomplete" ||
		len(enumeration.Boundaries) != 1 ||
		enumeration.Boundaries[0].Emitted != 3 ||
		enumeration.Boundaries[0].Total != 10 ||
		!enumeration.Boundaries[0].TotalKnown {
		t.Fatalf("paged read enumeration drifted: %+v", enumeration)
	}
	block := runtimeTraceCausalProjectionCoverageBlock(types.ObservationLedgerInput{
		ToolResults: []types.ToolResult{{
			ToolName:             "read_file",
			Success:              true,
			RuntimeArtifactRead:  &types.ToolRuntimeArtifactRead{RequestedPath: "customer@trace.sys", Kind: "trace"},
			EnumerationAuthority: enumeration,
		}},
	}, "zh")
	if block == nil {
		t.Fatal("partial runtime-artifact read must publish an enumeration boundary")
	}
	for _, want := range []string{
		"enumeration_status=incomplete",
		"customer@trace.sys/lines:emitted=3,total=10",
		"分页只返回",
	} {
		if !strings.Contains(block.Text, want) {
			t.Fatalf("runtime read coverage block missing %q:\n%s", want, block.Text)
		}
	}
}

func TestSourceReadEnumerationDoesNotPolluteTraceCoverageBoundary(t *testing.T) {
	block := runtimeTraceCausalProjectionCoverageBlock(types.ObservationLedgerInput{
		ToolResults: []types.ToolResult{{
			ToolName: "read_file",
			Success:  true,
			EnumerationAuthority: &types.ToolEnumerationAuthority{
				Status: "incomplete",
				Boundaries: []types.ToolEnumerationBoundary{{
					Scope: "internal/tool/example.go", Dimension: "lines", Emitted: 20, Total: 100, TotalKnown: true,
				}},
			},
		}},
	}, "zh")
	if block != nil {
		t.Fatalf("ordinary source pagination must not create a trace coverage boundary: %+v", block)
	}
}
