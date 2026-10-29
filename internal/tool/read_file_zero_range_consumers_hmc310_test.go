package tool

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC310PublicReadFileObservationLedgerVisibleRange(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		offset     int
		visible    bool
		empty      bool
		absolute   bool
	}{
		{name: "hidden_first", body: strings.Repeat("x", MaxInlineBytes*2) + "\n"},
		{name: "hidden_later", body: "earlier\n" + strings.Repeat("x", MaxInlineBytes*2) + "\n", offset: 1},
		{name: "visible", body: "package fixture\n", visible: true},
		{name: "empty", empty: true},
		{name: "absolute_empty", empty: true, absolute: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			const path = "source.go"
			ctx := physicalReadFixture(t, path, tc.body)
			readPath := path
			if tc.absolute {
				readPath = filepath.Join(ctx.RepoRoot, path)
			}
			read := physicalRead(t, ctx, readPath, tc.offset, 1)
			if !read.Success || read.ReadCoverage == nil || read.RawRef == "" {
				t.Fatalf("public readable source premise failed: %+v", read)
			}
			ledger := types.CompileObservationLedger(types.ObservationLedgerInput{RepoRoot: ctx.RepoRoot, ToolResults: []types.ToolResult{read}})
			var records []types.ObservationRecord
			for _, record := range ledger.Records {
				if record.Origin == types.AnswerEvidenceOriginCurrentSource {
					records = append(records, record)
				}
			}
			if !tc.visible && !tc.empty {
				if len(records) != 0 {
					t.Fatalf("hidden source bytes became current-source observations: %+v", records)
				}
				return
			}
			if len(records) != 1 {
				t.Fatalf("legitimate visible/empty source record lost: %+v", records)
			}
			r := records[0]
			if tc.visible {
				if r.Span.LineStart != 1 || r.Span.LineEnd != 1 || r.ResultCount == nil || *r.ResultCount != 1 {
					t.Fatalf("visible source coverage changed: %+v", r)
				}
				return
			}
			reads, ranges, totals := types.ExtractReadCoverage([]types.ToolResult{read}, ctx.RepoRoot)
			if !types.ReadFileHasKnownEmptyLines(read, ctx.RepoRoot) || !reads[path] || len(ranges[path]) != 0 || totals[path] != 0 ||
				r.Span.LineStart != 0 || r.Span.LineEnd != 0 || r.EvidenceScope != types.ScopeFile || r.ResultCount == nil || *r.ResultCount != 0 || types.ObservationRecordHasCurrentSourceLineSpan(r) {
				t.Fatalf("known-empty file became a fictional line or lost completion proof: %+v %+v", read, r)
			}
		})
	}
}

func TestHMC310PublicReadFileCompletionDoesNotRecoverHiddenMember(t *testing.T) {
	for _, hidden := range []bool{true, false} {
		name := "visible"
		body := "func Validate() {}\n"
		if hidden {
			name = "hidden"
			body = "func Validate() {} // " + strings.Repeat("x", MaxInlineBytes*2) + "\n"
		}
		t.Run(name, func(t *testing.T) {
			const path = "internal/analysis/findings_validator/validator.go"
			ctx := physicalReadFixture(t, path, body)
			ctx.Mutable.SetObjective("列出这个包的全部入口函数")
			ctx.AnalysisIR = &types.AnalysisIR{
				RequestModel: types.RequestModel{
					Intent:        types.IntentEnumerate,
					Predicates:    types.SemanticPredicates{IsCategoryEnumeration: true},
					AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqEnumeration), Entities: []string{"findings_validator"}},
					CompletenessObligation: &types.CompletenessObligation{
						Required: true, SourceQuote: "全部入口函数",
					},
				},
				AnswerContract: types.AnswerContract{CitationReq: types.CitationReq{Required: false}},
			}
			if !types.RequiresExhaustiveEnumerationMemberSetHandoff(ctx.AnalysisIR.RequestModel) {
				t.Fatal("test must exercise the typed complete-member handoff contract")
			}
			read := physicalRead(t, ctx, path, 0, 1)
			if !read.Success || read.RawRef == "" || read.ReadCoverage == nil {
				t.Fatalf("public read premise failed: %+v", read)
			}
			ctx.Mutable.AppendDispatchToolResult(read)
			lines := aggregateReadFileToolLinesFromResult(read)
			if hidden && len(lines) != 0 {
				t.Errorf("hidden raw-artifact row acquired aggregate member support: %d rows", len(lines))
			}
			params, err := json.Marshal(map[string]any{
				"reason": "package entry identified", "confidence": "high", "result_kind": "resolved",
				"aggregate_facts": []map[string]any{{"kind": "member_set", "label": "package entries", "value": "1", "members": []string{"findings_validator → Validate"}}},
			})
			if err != nil {
				t.Fatal(err)
			}
			out, err := (&EmitInvestigationComplete{}).Execute(ctx, params)
			if err != nil {
				t.Fatal(err)
			}
			facts := ctx.Mutable.StableInvestigationAggregateFacts()
			if hidden {
				if ctx.Mutable.IsInvestigationComplete() {
					t.Errorf("invisible source member closed investigation: %s", out.Summary)
				}
				for _, fact := range facts {
					if len(fact.SupportRefs) != 0 {
						t.Errorf("hidden bytes became accepted support refs: %+v", fact)
					}
				}
				return
			}
			if !ctx.Mutable.IsInvestigationComplete() || len(facts) != 1 || len(facts[0].SupportRefs) != 1 || facts[0].SupportRefs[0] != path+":1" {
				t.Fatalf("legitimate visible member stopped completing: %s %+v", out.Summary, facts)
			}
		})
	}
}
