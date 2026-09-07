package index

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	coretypes "github.com/hanchaoqun/codrax/internal/types"
)

// Keep the actual parse -> graph -> completion path pinned. These are source
// shapes, not desired prose or an interface-to-implementation dispatch rule.
func TestCallableBodyPresenceActualParserCompletionEntry(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		wantBodyDebt bool
	}{
		{"interface_signature", "interface Transport {\n  send(value: string): string;\n}\nfunction deliver(t: Transport) {\n  return t.send('value');\n}\n", false},
		{"concrete_method", "class Transport {\n  send(value: string): string { return value; }\n}\nfunction deliver(t: Transport) {\n  return t.send('value');\n}\n", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, selected := callableBodyCompletionContext(t, types.LangTypeScript, tc.source, "Transport.send", 5)
			if selected == nil || selected.HasParserOwnedBody() != tc.wantBodyDebt {
				t.Fatalf("parser witness=%+v", selected)
			}
			result := completeCallableBodyFixture(t, ctx)
			if tc.wantBodyDebt {
				if ctx.Mutable.IsInvestigationComplete() || !strings.Contains(result.Summary, "implementation-body evidence") {
					t.Fatalf("real body obligation removed: %s", result.Summary)
				}
			} else {
				if !ctx.Mutable.IsInvestigationComplete() || !strings.Contains(result.Summary, "declaration without a local body") {
					t.Fatalf("declaration completion failed or lost boundary: %s", result.Summary)
				}
			}
		})
	}
}

func TestCallableBodyPresenceActualCompletionExcludesParametersKeepsFirstStatement(t *testing.T) {
	for _, tc := range []struct {
		language, source string
		callLine         int
	}{
		{types.LangPython, "def work(\n    value\n):\n    return value\n\ndef deliver():\n    return work('value')\n", 7},
		{types.LangRuby, "def work(\n  value\n)\n  value\nend\ndef deliver\n  work('value')\nend\n", 7},
		{types.LangLua, "function work(\n  value\n)\n  return value\nend\nfunction deliver()\n  return work('value')\nend\n", 7},
	} {
		for _, line := range []int{2, 4} {
			t.Run(fmt.Sprintf("%s/line%d", tc.language, line), func(t *testing.T) {
				ctx, sym := callableBodyCompletionContext(t, tc.language, tc.source, "work", tc.callLine)
				if sym == nil || !sym.HasParserOwnedBody() || sym.BodyStartLine != 4 {
					t.Fatalf("body extent=%+v", sym)
				}
				ctx.Mutable.AppendEvidence([]coretypes.EvidenceItem{{ID: "selected-source", Kind: coretypes.EvidenceMechanism, AnchorKind: coretypes.AnchorDefinition,
					AnchorSymbol: "work", Subject: "work", Source: sym.File, LineStart: line, Scope: coretypes.ScopeLine,
					Producer: coretypes.EvidenceProducerExplorerEmitEvidence, GroundingStatus: coretypes.GroundingGrounded}})
				result := completeCallableBodyFixture(t, ctx)
				if line == 2 {
					if ctx.Mutable.IsInvestigationComplete() || !strings.Contains(result.Summary, "implementation-body evidence") {
						t.Fatalf("parameter miscounted as body: %s", result.Summary)
					}
				} else if !ctx.Mutable.IsInvestigationComplete() {
					t.Fatalf("first actual statement was refused: %s", result.Summary)
				}
			})
		}
	}
}

func callableBodyCompletionContext(t *testing.T, language, source, identity string, callLine int) (*coretypes.BusContext, *types.Symbol) {
	t.Helper()
	repo, file := t.TempDir(), "fixture.source"
	path := filepath.Join(repo, file)
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	files := ParseFiles([]FileEntry{{RelPath: file, AbsPath: path, Language: language, Size: int64(len(source))}}, repo)
	if len(files) != 1 {
		t.Fatalf("parser files=%d", len(files))
	}
	graph := BuildGraph(repo, files)
	var selected *types.Symbol
	for i := range files[0].Symbols {
		sym := &files[0].Symbols[i]
		name := sym.Name
		if sym.Parent != "" {
			name = sym.Parent + "." + name
		}
		if name == identity {
			selected = sym
		}
	}
	mut := coretypes.NewMutableState("explain delivery and configuration")
	mut.SetSearchGraph(graph)
	ctx := &coretypes.BusContext{RepoRoot: repo, Mutable: mut, AnalysisIR: &coretypes.AnalysisIR{RequestModel: coretypes.RequestModel{
		Intent: coretypes.IntentExplain, AnalyzerHints: coretypes.AnalyzerHints{Kind: string(coretypes.ReqMechanism)},
		SubTopics: []coretypes.SubTopic{
			{Summary: "delivery", Entities: []string{identity}, EntityProvenance: []coretypes.EntityProvenance{{
				Surface: identity, Origin: coretypes.EntityOriginSubTopicEntity, Resolution: coretypes.EntityResolutionSymbol, Resolved: true, UseForSearch: true, UseForShape: true,
			}}}, {Summary: "configuration", Entities: []string{"configuration"}},
		},
	}}}
	lines := strings.Split(strings.TrimSuffix(source, "\n"), "\n")
	var read strings.Builder
	fmt.Fprintf(&read, "[%s: showing lines 1-%d of %d]\n", file, len(lines), len(lines))
	for i, line := range lines {
		fmt.Fprintf(&read, "%4d│ %s\n", i+1, line)
	}
	mut.SetTurnAArtifacts(coretypes.TurnAArtifacts{ToolResults: []coretypes.ToolResult{{ToolName: "read_file", Success: true, Summary: read.String()}}})
	mut.EvidenceClosure().SetReadSet(map[string]bool{file: true})
	mut.EvidenceClosure().SetReadRanges(map[string][]coretypes.LineRange{file: {{Start: 1, End: len(lines)}}})
	mut.AppendEvidence([]coretypes.EvidenceItem{{
		ID: "call", Kind: coretypes.EvidenceRelationship, AnchorKind: coretypes.AnchorCall, AnchorSymbol: identity, Subject: "deliver", Object: identity,
		Source: file, LineStart: callLine, Scope: coretypes.ScopeLine, Producer: coretypes.EvidenceProducerExplorerEmitEvidence, GroundingStatus: coretypes.GroundingGrounded,
	}})
	return ctx, selected
}

func completeCallableBodyFixture(t *testing.T, ctx *coretypes.BusContext) coretypes.ToolResult {
	t.Helper()
	result, err := (&tool.EmitInvestigationComplete{}).Execute(ctx, json.RawMessage(`{"reason":"The declaration and selected call have been inspected; implementation dispatch is not inferred.","confidence":"high","result_kind":"resolved"}`))
	if err != nil || !result.Success {
		t.Fatalf("completion error=%v result=%+v", err, result)
	}
	return result
}
