package tool

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func incompletePageHMC257() types.ToolResult {
	return types.ToolResult{ToolName: "read_file", Success: true, RuntimeArtifactRead: &types.ToolRuntimeArtifactRead{RequestedPath: "/private/.codrax/blob/query.json", TraceQueryBlob: true, LineStart: 2, LineEnd: 6, TotalLines: 100}, EnumerationAuthority: &types.ToolEnumerationAuthority{Status: "incomplete", Boundaries: []types.ToolEnumerationBoundary{{Scope: "/private/.codrax/blob/query.json", Dimension: "lines", Emitted: 5, Total: 100, TotalKnown: true}}}}
}

func TestHMC257EnumerationFooterPreservesLocalAndCausalAuthorities(t *testing.T) {
	page := incompletePageHMC257()
	for _, lang := range []string{"zh", "en"} {
		block := runtimeTraceCausalProjectionCoverageBlock(types.ObservationLedgerInput{ToolResults: []types.ToolResult{page}}, lang)
		if block == nil {
			t.Fatal("page boundary disappeared")
		}
		for _, bad := range []string{"当前返回项只能作为样本或下界", "no causal", "没有获得可支撑根因", ".codrax/blob"} {
			if strings.Contains(block.Text, bad) {
				t.Fatalf("page acquired foreign authority: %s", block.Text)
			}
		}
		if lang == "zh" && (!strings.Contains(block.Text, "仅限制该结果页的逐字读取覆盖") || !strings.Contains(block.Text, "不改变独立原生集合") || block.Title != "结果读取与查询范围") {
			t.Fatalf("page scope missing: %+v", block)
		}
	}
	failed := types.ToolResult{ToolName: "trace_query", Success: false, Summary: "query-result.json is not a trace"}
	block := runtimeTraceCausalProjectionCoverageBlock(types.ObservationLedgerInput{ToolResults: []types.ToolResult{page, failed}}, "zh")
	if block == nil || !strings.Contains(block.Text, "Trace 查询执行失败") || !strings.Contains(block.Text, "没有获得可支撑根因") {
		t.Fatalf("page coverage waived failed query/cause absence: %+v", block)
	}
	// A complete query does not erase a different, genuinely truncated set.
	truncated := types.ToolResult{ToolName: "trace_query", Success: true, RawRef: "exact-q", Observations: []types.ObservationRecord{{SourceRef: types.ObservationSourceRef{Path: "capture.data", QueryScopeID: "query", QueryWindowKnown: true, QueryWindowStartTs: 10.000000001, QueryWindowEndTs: 10.000000002}}}, EnumerationAuthority: &types.ToolEnumerationAuthority{Status: "incomplete", Boundaries: []types.ToolEnumerationBoundary{{Scope: "resource_stack", Dimension: "events", Emitted: 2, TotalKnown: false}}}}
	complete := types.ToolResult{ToolName: "trace_query", Success: true, EnumerationAuthority: &types.ToolEnumerationAuthority{Status: "complete"}}
	block = runtimeTraceCausalProjectionCoverageBlock(types.ObservationLedgerInput{ToolResults: []types.ToolResult{page, truncated, complete}}, "zh")
	for _, want := range []string{"10.000000001", "10.000000002", "总数未知", "该查询当前返回项只能作为样本或下界", "不改变独立原生集合"} {
		if block == nil || !strings.Contains(block.Text, want) {
			t.Fatalf("lost independent query boundary %q: %+v", want, block)
		}
	}
}

func TestHMC257EnumerationFooterBudgetAndUnknownCounts(t *testing.T) {
	var results []types.ToolResult
	for n := 0; n < 18; n++ {
		r := incompletePageHMC257()
		p := *r.RuntimeArtifactRead
		p.RequestedPath = fmt.Sprintf("page-%d", n)
		r.RuntimeArtifactRead = &p
		results = append(results, r)
	}
	block := runtimeTraceCausalProjectionCoverageBlock(types.ObservationLedgerInput{ToolResults: results}, "zh")
	if block == nil || !strings.Contains(block.Text, "另有 2 个独立边界未在此展开，仍未完整") {
		t.Fatalf("budget implied completeness: %+v", block)
	}
	unknown := types.ToolResult{ToolName: "trace_query", Success: true, EnumerationAuthority: &types.ToolEnumerationAuthority{Status: "incomplete"}}
	block = runtimeTraceCausalProjectionCoverageBlock(types.ObservationLedgerInput{ToolResults: []types.ToolResult{unknown}}, "zh")
	if block == nil || !strings.Contains(block.Text, "数量未知而非零") || strings.Contains(block.Text, "已展示 0") {
		t.Fatalf("missing boundary became zero: %+v", block)
	}
}
