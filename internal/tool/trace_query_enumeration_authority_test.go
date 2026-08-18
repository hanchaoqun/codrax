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
		"枚举未完整",
		"事件检索的事件已展示 8 项，总数未知",
		"只能作为样本或下界",
		"全部/仅有/总计/共N/最大/最小",
	} {
		if !strings.Contains(block.Text, want) {
			t.Fatalf("coverage block missing %q:\n%s", want, block.Text)
		}
	}
	for _, raw := range []string{"enumeration_status=", "compacted_views=", "boundaries=", "emitted=", "total="} {
		if strings.Contains(block.Text, raw) {
			t.Fatalf("reader-facing enumeration boundary leaked control metadata %q:\n%s", raw, block.Text)
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
		"枚举未完整",
		"其他结果范围的行已展示 3 项，共 10 项",
		"展示上限或分页",
	} {
		if !strings.Contains(block.Text, want) {
			t.Fatalf("runtime read coverage block missing %q:\n%s", want, block.Text)
		}
	}
}

func TestTraceQueryBlobPageCoverageUsesTypedPublicScope(t *testing.T) {
	privatePath := "/work/.codrax/blob/session/trace_query-deadbeef.txt"
	block := runtimeTraceCausalProjectionCoverageBlock(types.ObservationLedgerInput{
		ToolResults: []types.ToolResult{{
			ToolName: "read_file",
			Success:  true,
			RuntimeArtifactRead: &types.ToolRuntimeArtifactRead{
				RequestedPath: privatePath, Kind: "blob", TraceQueryBlob: true,
			},
			EnumerationAuthority: readFileEnumerationAuthority(privatePath, 1, 66, 332, true),
		}},
	}, "zh")
	if block == nil || !strings.Contains(block.Text, "Trace 查询结果页的行已展示 66 项，共 332 项") {
		t.Fatalf("trace query page lost its exact public boundary: %+v", block)
	}
	if strings.Contains(block.Text, privatePath) || strings.Contains(block.Text, ".codrax/blob") {
		t.Fatalf("trace coverage leaked private blob identity: %s", block.Text)
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
