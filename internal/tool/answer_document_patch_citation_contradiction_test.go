package tool

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestNormalizeUnusedContradictedRuntimeArtifactNegativeCitations(t *testing.T) {
	work := t.TempDir()
	artifactName := "attached_trace-44d2a269.txt"
	if err := os.WriteFile(
		filepath.Join(work, artifactName),
		[]byte("alpha\nH:RenderService:DoFrame\nomega\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{WorkDir: work}
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Items: []types.AnswerBlockItem{{
				ID:          "source",
				CitationRef: 0,
			}},
		}},
		Citations: []types.Citation{
			{File: "source.go", Line: 1, Quote: "package fixture"},
			{
				File:            artifactName,
				Scope:           types.ScopeNegative,
				NegativePattern: "H:RenderService:DoFrame",
			},
			{
				File:            artifactName,
				Scope:           types.ScopeNegative,
				NegativePattern: "missing_vsync",
			},
			{
				File:            "source.go",
				Scope:           types.ScopeNegative,
				NegativePattern: "H:RenderService:DoFrame",
			},
		},
	}

	if fixed := normalizeUnusedContradictedRuntimeArtifactNegativeCitations(doc, ctx); fixed != 1 {
		t.Fatalf("fixed=%d, want exactly one contradicted unused citation", fixed)
	}
	if len(doc.Citations) != 3 {
		t.Fatalf("citations=%+v, want source + absent trace proof + source proof", doc.Citations)
	}
	if doc.Blocks[0].Items[0].CitationRef != 0 {
		t.Fatalf("referenced source citation remap drifted: %+v", doc.Blocks[0].Items)
	}
	for _, cit := range doc.Citations {
		if cit.File == artifactName && cit.NegativePattern == "H:RenderService:DoFrame" {
			t.Fatalf("contradicted runtime-artifact absence citation survived: %+v", doc.Citations)
		}
	}
}

func TestNormalizeUnusedContradictedRuntimeArtifactNegativeCitationsPreservesReferencedEntry(t *testing.T) {
	work := t.TempDir()
	artifactName := "attached_trace-a1b2c3d4.txt"
	if err := os.WriteFile(filepath.Join(work, artifactName), []byte("present-span\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	doc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Items: []types.AnswerBlockItem{{
				ID:          "negative-proof",
				CitationRef: 0,
			}},
		}},
		Citations: []types.Citation{{
			File:            artifactName,
			Scope:           types.ScopeNegative,
			NegativePattern: "present-span",
		}},
	}

	if fixed := normalizeUnusedContradictedRuntimeArtifactNegativeCitations(doc, &types.BusContext{WorkDir: work}); fixed != 0 {
		t.Fatalf("referenced inherited citation must preserve patch index semantics, fixed=%d", fixed)
	}
	if len(doc.Citations) != 1 || doc.Blocks[0].Items[0].CitationRef != 0 {
		t.Fatalf("referenced citation drifted: doc=%+v", doc)
	}
}

func TestPersistMergedAnswerDocumentDropsContradictedUnusedPatchCitation(t *testing.T) {
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, "source.go"), []byte("package fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	artifactName := "attached_trace-44d2a269.txt"
	if err := os.WriteFile(filepath.Join(repo, artifactName), []byte("H:RenderService:DoFrame\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{
		RepoRoot: repo,
		WorkDir:  repo,
		Mutable:  types.NewMutableState("patch citation contradiction"),
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
				File:            artifactName,
				Scope:           types.ScopeNegative,
				NegativePattern: "H:RenderService:DoFrame",
			},
		},
	}

	result, err := persistMergedAnswerDocument(
		ctx,
		"emit_answer_document_patch",
		types.MutationPartial,
		"append contradicted citation",
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
		t.Fatalf("contradicted patch citation survived shared persist: %+v", got.Citations)
	}
	if got.Blocks[0].Items[0].CitationRef != 0 {
		t.Fatalf("referenced source citation remap drifted: %+v", got.Blocks[0].Items)
	}
}
