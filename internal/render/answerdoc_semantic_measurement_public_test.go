package render_test

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/preview"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeWorkReceiptPublicSemanticQueryKeepsMeasurementIdentity(t *testing.T) {
	// The outer pair crosses the start of the query; the nested pair must
	// not add to its wall time. A separate pair makes the family envelope
	// larger than both the interval union and its longest member.
	fixture := `# tracer: nop
      reader-100 (  100) [001] ...1  1.000000: tracing_mark_write: B|100|VerifyClass Outer
      reader-100 (  100) [001] ...1  1.015000: tracing_mark_write: B|100|VerifyClass Nested
      reader-100 (  100) [001] ...1  1.020000: tracing_mark_write: E|100
      reader-100 (  100) [001] ...1  1.030000: tracing_mark_write: E|100
      reader-100 (  100) [001] ...1  1.040000: tracing_mark_write: B|100|VerifyClass Later
      reader-100 (  100) [001] ...1  1.042000: tracing_mark_write: E|100
      reader-100 (  100) [001] ...1  1.050000: tracing_mark_write: B|100|VerifyClass Last
      reader-100 (  100) [001] ...1  1.053000: tracing_mark_write: E|100
`
	for _, tc := range []struct {
		name, end, ms string
		zh, en, not   []string
	}{
		{"single-clipped", "1.014", "4.000", []string{"VerifyClass Outer", "窗内实测 4.000ms", "完整耗时 30.000ms"}, []string{"VerifyClass Outer", "measured 4.000ms in the query window", "full elapsed 30.000ms"}, []string{"合计", "member records"}},
		{"overlapping-family", "1.045", "22.000", []string{"类校验", "3 条记录", "查询窗内合计 22.000ms", "重叠区间只计一次", "最长记录 `VerifyClass Outer`（窗内 20.000ms）", "完整配对范围合计 32.000ms"}, []string{"class verification", "3 records", "total 22.000ms in the query window", "overlapping intervals counted once", "longest record `VerifyClass Outer` (20.000ms in the window)", "full paired intervals total 32.000ms"}, []string{"`VerifyClass Outer`（线程", "`VerifyClass Outer` (thread", "27.000ms", "35.000ms"}},
		{"disjoint-family", "1.054", "5.000", []string{"类校验", "2 条记录", "查询窗内合计 5.000ms", "最长记录 `VerifyClass Last`（窗内 3.000ms）"}, []string{"class verification", "2 records", "total 5.000ms in the query window", "longest record `VerifyClass Last` (3.000ms in the window)"}, []string{"重叠区间只计一次", "full paired intervals", "完整配对范围合计", "13.000ms"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "work.ftrace")
			if err := os.WriteFile(path, []byte(fixture), 0600); err != nil {
				t.Fatal(err)
			}
			start := "1.010"
			if tc.name == "disjoint-family" {
				start = "1.040"
			}
			p := fmt.Sprintf(`{"source":"path","path":%q,"view":"window_stats","pid":100,"time_start":%s,"time_end":%s,"trace_flavor":"harmony_hitrace"}`, path, start, tc.end)
			result, err := (&tool.TraceQuery{}).Execute(&types.BusContext{RepoRoot: dir, WorkDir: dir}, json.RawMessage(p))
			if err != nil || !result.Success {
				t.Fatalf("native query failed: %v; %s", err, result.Summary)
			}
			var id string
			for _, r := range result.Observations {
				if r.Predicate == "trace_semantic_span" && r.Object == "class_verification" {
					if id != "" || r.Value != tc.ms {
						t.Fatalf("native fold not as expected: %+v", r)
					}
					id = r.ID
				}
			}
			if id == "" {
				t.Fatalf("no classified native work: %s", result.Summary)
			}
			contract := types.BuildRuntimeWorkRelationContract(types.ObservationLedgerInput{ToolResults: []types.ToolResult{result}}, true)
			receipt := &types.AnswerRuntimeWorkRelationReceipt{ObservationID: id, Conclusion: types.RuntimeWorkRelationConclusionRelationUnproven}
			if !types.BindRuntimeWorkRelationReceipt(receipt, contract) {
				t.Fatalf("native work measurement not offered: %+v", contract)
			}
			if types.BindRuntimeWorkRelationReceipt(&types.AnswerRuntimeWorkRelationReceipt{ObservationID: id, Conclusion: types.RuntimeWorkRelationConclusionCausalContributionSupported}, contract) {
				t.Fatal("a measurement did not prove causality")
			}
			wire, err := json.Marshal(receipt)
			if err != nil || string(wire) != fmt.Sprintf(`{"observation_id":%q,"conclusion":"relation_unproven"}`, id) {
				t.Fatalf("unexpected model selection shape: %s %v", wire, err)
			}
			doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{ID: "work", Kind: types.BlockSummary, Text: "Retain original model prose.", RuntimeWorkRelation: receipt}}}
			for lang, want := range map[string][]string{"zh": tc.zh, "en": tc.en} {
				md := render.RenderAnswerDocument(doc, lang)
				page, err := preview.RenderMarkdownHTML([]byte(md))
				if err != nil {
					t.Fatal(err)
				}
				page = strings.NewReplacer("<code>", "`", "</code>", "`").Replace(html.UnescapeString(page))
				for surface, out := range map[string]string{"markdown": md, "html": page} {
					for _, s := range append(append([]string(nil), want...), "Retain original model prose.") {
						if !strings.Contains(out, s) {
							t.Errorf("%s %s missing %q:\n%s", lang, surface, s, out)
						}
					}
					for _, s := range tc.not {
						if strings.Contains(out, s) {
							t.Errorf("%s %s published wrong identity/caliber %q:\n%s", lang, surface, s, out)
						}
					}
				}
			}
		})
	}
}
