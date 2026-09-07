package tool_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/index"
	rmtypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1613EmitAnswerSymbolRealGenericReceiverRemainsUnknown(t *testing.T) {
	repo := t.TempDir()
	file := "service.go"
	source := []byte("package service\n\ntype A[T any] struct{}\nfunc (a *A[T]) Run() {}\n")
	path := filepath.Join(repo, file)
	if err := os.WriteFile(path, source, 0600); err != nil {
		t.Fatal(err)
	}
	infos := index.ParseFiles([]index.FileEntry{{RelPath: file, AbsPath: path, Language: rmtypes.LangGo, Size: int64(len(source))}}, repo)
	if len(infos) != 1 {
		t.Fatalf("parser files=%d", len(infos))
	}
	found := false
	for _, symbol := range infos[0].Symbols {
		if symbol.Name == "Run" && symbol.Kind == "method" && symbol.Receiver == "A[T]" && symbol.Line == 4 {
			found = true
		}
	}
	if !found {
		t.Fatalf("fixture did not expose the real generic receiver representation: %+v", infos[0].Symbols)
	}
	for _, line := range []int{4, 60} {
		t.Run(map[int]string{4: "original_location", 60: "no_unproved_relocation"}[line], func(t *testing.T) {
			ctx := &types.BusContext{Mutable: types.NewMutableState(""), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentEnumerate}}}
			ctx.Mutable.SetSearchGraph(&rmtypes.Graph{FileIndex: map[string]*rmtypes.FileInfo{file: infos[0]}})
			params, _ := json.Marshal(map[string]any{"items": []map[string]any{{"name": "A.Run", "file": file, "line": line, "kind": "method"}}, "completeness": "lower_bound"})
			res, err := (&tool.EmitAnswerSymbol{}).Execute(ctx, params)
			if err != nil || !res.Success {
				t.Fatalf("unsupported generic-owner comparison is unknown, not a proven different owner: err=%v result=%+v", err, res)
			}
			got, _ := ctx.Mutable.EmittedAnswerSymbols()
			if len(got) != 1 || got[0].Name != "A.Run" || got[0].File != file || got[0].Line != line {
				t.Fatalf("missing generic equivalence proof must not rewrite the model identity or location: %+v", got)
			}
		})
	}
}
