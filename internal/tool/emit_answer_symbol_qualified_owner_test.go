package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A qualified name and another owner's same leaf are not an alias pair.
// Exercise the actual emitter, including read grounding and accepted-buffer
// preservation; a helper-only check would miss the emitter's second leaf retry.
func TestEmitAnswerSymbolQualifiedOwnerDoesNotMoveToSibling(t *testing.T) {
	ctx := newAnswerSymbolCtx()
	ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentEnumerate}}
	file := "src/service.go"
	seedReadFileHistory(ctx, file, 10, "func (a *A) Run() {}")
	seedReadFileHistory(ctx, file, 20, "func (b *B) Run() {}")
	seedReadFileHistory(ctx, file, 58, "", "", "return nil", "", "")
	ctx.Mutable.SetSearchGraph(&repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
		file: {RelPath: file, Language: "go", Package: "service", Symbols: []repomap.Symbol{
			{Name: "Run", Receiver: "A", Kind: "method", Line: 10, EndLine: 10},
			{Name: "Run", Receiver: "B", Kind: "method", Line: 20, EndLine: 20},
		}},
	}})
	ctx.Mutable.AppendEvidence([]types.EvidenceItem{{
		ID: "b-run-definition", Kind: types.EvidenceDirect, Scope: types.ScopeLine,
		Source: file, LineStart: 20, LineEnd: 20,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "B.Run", OwnerSymbol: "B.Run",
		GroundingStatus: types.GroundingGrounded,
	}})
	tool := &EmitAnswerSymbol{}
	correct, _ := json.Marshal(map[string]any{"items": []map[string]any{{"name": "A.Run", "file": file, "line": 10, "kind": "method"}}, "completeness": "lower_bound"})
	res, err := tool.Execute(ctx, correct)
	if err != nil || !res.Success {
		t.Fatalf("precise model-selected owner must be accepted: err=%v result=%+v", err, res)
	}
	before, beforeClaim := ctx.Mutable.EmittedAnswerSymbols()
	// Do not let the accepted slate itself provide a correct A.Run relocation:
	// retain an unrelated accepted answer as a mutation sentinel through the
	// legacy unknown-origin setter, which also invalidates the surface cache.
	ctx.Mutable.SetEmittedAnswerSymbols([]types.AnswerSymbol{{Name: "Untouched", File: "src/other.go", Line: 7, Kind: "type"}}, beforeClaim)
	before, beforeClaim = ctx.Mutable.EmittedAnswerSymbols()
	wrongLine, _ := json.Marshal(map[string]any{"items": []map[string]any{{"name": "A.Run", "file": file, "line": 60, "kind": "method"}}, "completeness": "lower_bound"})
	res, err = tool.Execute(ctx, wrongLine)
	if err != nil {
		t.Fatal(err)
	}
	if res.Success {
		got, _ := ctx.Mutable.EmittedAnswerSymbols()
		t.Fatalf("A.Run must not be silently replaced or relocated to sibling B.Run: got=%+v result=%s", got, res.Summary)
	}
	got, claim := ctx.Mutable.EmittedAnswerSymbols()
	if !reflect.DeepEqual(got, before) || claim != beforeClaim {
		t.Fatalf("rejected identity mismatch changed accepted answer: got=%+v claim=%s before=%+v claim=%s", got, claim, before, beforeClaim)
	}
}

func TestB1613EmitAnswerSymbolQualifiedOwnerCompatibility(t *testing.T) {
	type definition struct {
		name, file string
		line       int
	}
	tests := []struct {
		label, requested, language, pkg, receiver, parent string
		definitions                                       []definition
		graphLine                                         int
		graphKind                                         string
		blankSymbolFile                                   bool
		extraOwner                                        string
		wantSuccess                                       bool
		wantLine                                          int
	}{
		{label: "full identity", requested: "A.Run", definitions: []definition{{name: "A.Run", line: 20}}, wantSuccess: true, wantLine: 20},
		{label: "native scope separator", requested: "A::Run", definitions: []definition{{name: "A.Run", line: 20}}, wantSuccess: true, wantLine: 20},
		{label: "receiver method expression", requested: "(*A).Run", definitions: []definition{{name: "A.Run", line: 20}}, wantSuccess: true, wantLine: 20},
		{label: "bare unique method", requested: "Run", definitions: []definition{{name: "B.Run", line: 20}}, wantSuccess: true, wantLine: 20},
		{label: "bare exact function", requested: "Run", definitions: []definition{{name: "Run", line: 20}}, wantSuccess: true, wantLine: 20},
		{label: "qualified selects among sibling leaves", requested: "A.Run", definitions: []definition{{name: "A.Run", line: 20}, {name: "B.Run", line: 30}}, wantSuccess: true, wantLine: 20},
		{label: "different owners", requested: "A.Run", definitions: []definition{{name: "B.Run", line: 20}}},
		{label: "case is identity", requested: "A.Run", definitions: []definition{{name: "a.Run", line: 20}}},
		{label: "unknown owner not alias", requested: "A.Run", definitions: []definition{{name: "Run", line: 20}}},
		{label: "unqualified remains ambiguous", requested: "Run", definitions: []definition{{name: "A.Run", line: 20}, {name: "B.Run", line: 30}}},
		{label: "other source not alias", requested: "A.Run", definitions: []definition{{name: "A.Run", file: "src/other.go", line: 20}}},
		{label: "parser wrong owner", requested: "A.Run", language: "go", receiver: "B", graphLine: 20, definitions: []definition{{name: "Run", line: 20}}},
		{label: "parser other occurrence", requested: "A.Run", language: "go", receiver: "A", graphLine: 21, definitions: []definition{{name: "Run", line: 20}}},
		{label: "Go receiver witness", requested: "(*A).Run", language: "go", receiver: "A", graphLine: 20, definitions: []definition{{name: "Run", line: 20}}, wantSuccess: true, wantLine: 20},
		{label: "Go package and receiver witness", requested: "service.A.Run", language: "go", pkg: "service", receiver: "A", graphLine: 20, definitions: []definition{{name: "Run", line: 20}}, wantSuccess: true, wantLine: 20},
		{label: "Java package and parent witness", requested: "org.demo.A.Run", language: "java", pkg: "org.demo", parent: "A", graphLine: 20, definitions: []definition{{name: "Run", line: 20}}, wantSuccess: true, wantLine: 20},
		{label: "ArkTS parent witness", requested: "A.Run", language: "arkts", parent: "A", graphLine: 20, definitions: []definition{{name: "Run", line: 20}}, wantSuccess: true, wantLine: 20},
		{label: "Cangjie top-level package witness", requested: "clinic::Run", language: "cangjie", pkg: "clinic", graphKind: "function", graphLine: 20, definitions: []definition{{name: "Run", line: 20}}, wantSuccess: true, wantLine: 20},
		{label: "method package without owner is not alias", requested: "clinic::Run", language: "cangjie", pkg: "clinic", graphLine: 20, definitions: []definition{{name: "Run", line: 20}}},
		{label: "Cangjie parent witness", requested: "A::Run", language: "cangjie", parent: "A", graphLine: 20, definitions: []definition{{name: "Run", line: 20}}, wantSuccess: true, wantLine: 20},
		{label: "qualified carriers of same definition", requested: "service.A.Run", language: "go", pkg: "service", receiver: "A", graphLine: 20, definitions: []definition{{name: "Run", line: 20}, {name: "A.Run", line: 20}}, wantSuccess: true, wantLine: 20},
		{label: "parser file-index owns blank symbol file", requested: "A.Run", language: "go", receiver: "A", graphLine: 20, blankSymbolFile: true, definitions: []definition{{name: "Run", line: 20}}, wantSuccess: true, wantLine: 20},
		{label: "same line different parser owners are ambiguous", requested: "A.Run", language: "go", receiver: "A", extraOwner: "B", graphLine: 20, definitions: []definition{{name: "Run", line: 20}}},
	}
	for _, tt := range tests {
		t.Run(tt.label, func(t *testing.T) {
			ctx := newAnswerSymbolCtx()
			ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentEnumerate}}
			file := "src/owner.code"
			seedReadFileHistory(ctx, file, 58, "", "", "return nil", "", "")
			if tt.graphLine > 0 {
				symbolFile := file
				if tt.blankSymbolFile {
					symbolFile = ""
				}
				graphKind := tt.graphKind
				if graphKind == "" {
					graphKind = "method"
				}
				syms := []repomap.Symbol{{Name: "Run", Kind: graphKind, File: symbolFile, Receiver: tt.receiver, Parent: tt.parent, Line: tt.graphLine, EndLine: tt.graphLine}}
				if tt.extraOwner != "" {
					syms = append(syms, repomap.Symbol{Name: "Run", Kind: "method", File: file, Receiver: tt.extraOwner, Line: tt.graphLine, EndLine: tt.graphLine})
				}
				ctx.Mutable.SetSearchGraph(&repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
					file: {RelPath: file, Language: tt.language, Package: tt.pkg, Symbols: syms},
				}})
			}
			for i, def := range tt.definitions {
				source := def.file
				if source == "" {
					source = file
				}
				ctx.Mutable.AppendEvidence([]types.EvidenceItem{{
					ID: "definition-" + strings.Repeat("x", i+1), Kind: types.EvidenceDirect, Scope: types.ScopeLine,
					Source: source, LineStart: def.line, LineEnd: def.line,
					AnchorKind: types.AnchorDefinition, AnchorSymbol: def.name, GroundingStatus: types.GroundingGrounded,
				}})
			}
			params, _ := json.Marshal(map[string]any{"items": []map[string]any{{"name": tt.requested, "file": file, "line": 60, "kind": "method"}}, "completeness": "lower_bound"})
			res, err := (&EmitAnswerSymbol{}).Execute(ctx, params)
			if err != nil {
				t.Fatal(err)
			}
			if res.Success != tt.wantSuccess {
				t.Fatalf("success=%v want=%v: %s", res.Success, tt.wantSuccess, res.Summary)
			}
			got, _ := ctx.Mutable.EmittedAnswerSymbols()
			if tt.wantSuccess {
				if len(got) != 1 || got[0].Name != tt.requested || got[0].File != file || got[0].Line != tt.wantLine {
					t.Fatalf("same-entity location recovery must preserve the model spelling: got=%+v want=%s:%d %s", got, file, tt.wantLine, tt.requested)
				}
			} else if len(got) != 0 {
				t.Fatalf("rejected owner mismatch mutated answer: %+v", got)
			}
		})
	}
}

// The first grounding lane is independent of the grounded-candidate fallback.
// A parser-proven sibling owner must not satisfy a fully qualified model name,
// whether the submitted location was read, unread, or already that declaration.
func TestB1613EmitAnswerSymbolGroundLaneDoesNotBorrowSiblingOwner(t *testing.T) {
	for _, lane := range []string{"unread_relocation", "exact_parser_declaration", "read_declaration"} {
		t.Run(lane, func(t *testing.T) {
			ctx := newAnswerSymbolCtx()
			ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentEnumerate}}
			file := "src/service.go"
			ctx.Mutable.SetSearchGraph(&repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
				file: {RelPath: file, Language: "go", Package: "service", Symbols: []repomap.Symbol{
					{Name: "Run", Receiver: "B", Kind: "method", File: file, Line: 20, EndLine: 20},
				}},
			}})
			line := 60
			if lane != "unread_relocation" {
				line = 20
			}
			if lane == "read_declaration" {
				seedReadFileHistory(ctx, file, 20, "func (b *B) Run() {}")
			}
			ctx.Mutable.SetEmittedAnswerSymbols([]types.AnswerSymbol{{Name: "Untouched", File: "src/other.go", Line: 7, Kind: "type"}}, "lower_bound")
			before, beforeClaim := ctx.Mutable.EmittedAnswerSymbols()
			params, _ := json.Marshal(map[string]any{"items": []map[string]any{{"name": "A.Run", "file": file, "line": line, "kind": "method"}}, "completeness": "lower_bound"})
			res, err := (&EmitAnswerSymbol{}).Execute(ctx, params)
			if err != nil {
				t.Fatal(err)
			}
			got, claim := ctx.Mutable.EmittedAnswerSymbols()
			if res.Success {
				t.Fatalf("parser proves only B.Run at line 20; A.Run must not borrow that declaration: got=%+v summary=%s", got, res.Summary)
			}
			if !reflect.DeepEqual(got, before) || claim != beforeClaim {
				t.Fatalf("rejected sibling owner changed accepted slate: got=%+v claim=%s", got, claim)
			}
		})
	}
}

func TestB1613EmitAnswerSymbolGroundLaneKeepsIdentityAndUnknownDistinct(t *testing.T) {
	for _, tt := range []struct {
		label, name, receiver, evidenceName, pkg, graphKind string
		citedLine, wantLine                                 int
		wantSuccess                                         bool
	}{
		{label: "same owner unread location repairs", name: "A.Run", receiver: "A", citedLine: 60, wantLine: 20, wantSuccess: true},
		{label: "receiver syntax repairs without renaming", name: "(*A).Run", receiver: "A", citedLine: 60, wantLine: 20, wantSuccess: true},
		{label: "unqualified unread location repairs", name: "Run", receiver: "B", citedLine: 60, wantLine: 20, wantSuccess: true},
		{label: "unknown owner does not move location", name: "A.Run", citedLine: 60, wantLine: 60, wantSuccess: true},
		{label: "unknown owner not new exact line rejection", name: "A.Run", citedLine: 20, wantLine: 20, wantSuccess: true},
		{label: "missing namespace not new exact line rejection", name: "ns.A.Run", receiver: "A", citedLine: 20, wantLine: 20, wantSuccess: true},
		{label: "missing namespace cannot move location", name: "ns.A.Run", receiver: "A", citedLine: 60, wantLine: 60, wantSuccess: true},
		{label: "known namespace conflict remains contrary", name: "ns.A.Run", receiver: "A", pkg: "other", citedLine: 20},
		{label: "method package cannot replace unknown owner", name: "A.Run", pkg: "service", citedLine: 20, wantLine: 20, wantSuccess: true},
		{label: "method package cannot authorize relocation", name: "service.Run", pkg: "service", citedLine: 60, wantLine: 60, wantSuccess: true},
		{label: "top-level function package authorizes relocation", name: "service.Run", pkg: "service", graphKind: "function", citedLine: 60, wantLine: 20, wantSuccess: true},
		{label: "exact evidence cannot override contrary parser owner", name: "A.Run", receiver: "B", evidenceName: "A.Run", citedLine: 60},
	} {
		t.Run(tt.label, func(t *testing.T) {
			ctx := newAnswerSymbolCtx()
			ctx.AnalysisIR = &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentEnumerate}}
			file := "src/service.code"
			graphKind := tt.graphKind
			if graphKind == "" {
				graphKind = "method"
			}
			ctx.Mutable.SetSearchGraph(&repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
				file: {RelPath: file, Package: tt.pkg, Symbols: []repomap.Symbol{
					{Name: "Run", Receiver: tt.receiver, Kind: graphKind, File: file, Line: 20, EndLine: 20},
				}},
			}})
			if tt.evidenceName != "" {
				ctx.Mutable.AppendEvidence([]types.EvidenceItem{{
					ID: "definition", Kind: types.EvidenceDirect, Scope: types.ScopeLine, Source: file,
					LineStart: 20, LineEnd: 20, AnchorKind: types.AnchorDefinition,
					AnchorSymbol: tt.evidenceName, GroundingStatus: types.GroundingGrounded,
				}})
			}
			params, _ := json.Marshal(map[string]any{"items": []map[string]any{{"name": tt.name, "file": file, "line": tt.citedLine, "kind": "method"}}, "completeness": "lower_bound"})
			res, err := (&EmitAnswerSymbol{}).Execute(ctx, params)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := ctx.Mutable.EmittedAnswerSymbols()
			if res.Success != tt.wantSuccess {
				t.Fatalf("success=%v want=%v: %s", res.Success, tt.wantSuccess, res.Summary)
			}
			if tt.wantSuccess && (len(got) != 1 || got[0].Name != tt.name || got[0].Line != tt.wantLine || got[0].File != file) {
				t.Fatalf("identity or unknown-state changed: got=%+v want=%s at %s:%d", got, tt.name, file, tt.wantLine)
			}
			if !tt.wantSuccess && len(got) != 0 {
				t.Fatalf("contrary owner evidence was accepted: %+v", got)
			}
		})
	}
}
