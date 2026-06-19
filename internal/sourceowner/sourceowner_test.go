package sourceowner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestFindEnclosingOwnerPythonClassMethod(t *testing.T) {
	src := []byte(`class Documenter:
    def filter_members(self):
        for member in members:
            has_doc = bool(doc)
`)
	got, ok := FindEnclosingOwner("sphinx/ext/autodoc/__init__.py", src, 4)
	if !ok {
		t.Fatal("expected owner")
	}
	if got.OwnerSymbol != "Documenter.filter_members" || got.AnchorSymbol != "filter_members" {
		t.Fatalf("owner=%+v, want Documenter.filter_members", got)
	}
}

func TestFindEnclosingOwnerGoMethod(t *testing.T) {
	src := []byte(`package p

func (s *Server) Serve() error {
	return nil
}
`)
	got, ok := FindEnclosingOwner("pkg/server.go", src, 4)
	if !ok {
		t.Fatal("expected owner")
	}
	if got.OwnerSymbol != "Server.Serve" || got.AnchorSymbol != "Serve" {
		t.Fatalf("owner=%+v, want Server.Serve", got)
	}
}

func TestFindEnclosingOwnerBraceClassMethod(t *testing.T) {
	src := []byte(`class Widget {
  render() {
    return true;
  }
}`)
	got, ok := FindEnclosingOwner("src/widget.ts", src, 3)
	if !ok {
		t.Fatal("expected owner")
	}
	if got.OwnerSymbol != "Widget.render" || got.AnchorSymbol != "render" {
		t.Fatalf("owner=%+v, want Widget.render", got)
	}
}

func TestEnrichSourceLocalizationReviewAddsLineStructuralOwner(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "pkg"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	src := []byte(`class Documenter:
    def filter_members(self):
        has_doc = bool(doc)
`)
	if err := os.WriteFile(filepath.Join(repo, "pkg", "owner.py"), src, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	review := types.SourceLocalizationReview{
		Source:      "read_turn_a",
		SourcePaths: []string{"pkg/owner.py"},
		EvidenceRefs: []types.WriteExplorationEvidenceRef{{
			ID:        "ev-line",
			Kind:      "relationship",
			Source:    "pkg/owner.py",
			LineStart: 3,
		}},
	}

	got := EnrichSourceLocalizationReview(repo, review, "read_turn_a_structural_owner")
	view := types.OwnerAnchorViewFromSourceLocalizationReview(got, 0)
	if !view.HasOwner {
		t.Fatalf("expected structural owner anchor: %+v", got)
	}
	if len(view.Items) == 0 || view.Items[0].OwnerSymbol != "Documenter.filter_members" {
		t.Fatalf("wrong owner view: %+v", view.Items)
	}
}
