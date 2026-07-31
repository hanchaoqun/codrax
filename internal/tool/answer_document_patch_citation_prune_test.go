package tool

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPersistMergedAnswerDocumentPrunesUnusedPatchCitationEntries(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "source.go"), []byte("package fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{
		RepoRoot: repo,
		WorkDir:  repo,
		Mutable:  types.NewMutableState("patch citation reachability"),
	}
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "grounded source mechanism",
			Items: []types.AnswerBlockItem{{
				ID:          "source",
				CitationRef: 0,
			}},
		}},
		Citations: []types.Citation{
			{File: "source.go", Line: 1, Quote: "package fixture"},
			{
				File:            "attached_trace.txt",
				Line:            5,
				Scope:           types.ScopeNegative,
				NegativePattern: "present-span",
			},
		},
	}

	result, err := persistMergedAnswerDocument(
		ctx,
		"emit_answer_document_patch",
		types.MutationPartial,
		"append unreferenced citation",
		doc,
		time.Now(),
	)
	if err != nil || !result.Success {
		t.Fatalf("persist failed: result=%+v err=%v", result, err)
	}
	got := ctx.Mutable.AnswerDocumentV2()
	if got == nil {
		t.Fatal("persisted document missing")
	}
	if len(got.Citations) != 1 || got.Citations[0].File != "source.go" {
		t.Fatalf("unreferenced patch citation survived shared persist pruning: %+v", got.Citations)
	}
	if got.Blocks[0].Items[0].CitationRef != 0 {
		t.Fatalf("referenced source citation remap drifted: %+v", got.Blocks[0].Items)
	}
}
