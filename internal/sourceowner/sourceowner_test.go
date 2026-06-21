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

func TestFindEnclosingOwnerPythonMultilineTopLevelFunction(t *testing.T) {
	src := []byte(`def summarize_variable(
    name,
    var,
):
    first_col = pretty_print(name)
    return first_col
`)
	got, ok := FindEnclosingOwner("xarray/core/formatting.py", src, 5)
	if !ok {
		t.Fatal("expected owner for multiline top-level function")
	}
	if got.OwnerSymbol != "summarize_variable" || got.AnchorSymbol != "summarize_variable" {
		t.Fatalf("owner=%+v, want summarize_variable", got)
	}
}

func TestFindEnclosingOwnerPythonMultilineClassMethod(t *testing.T) {
	src := []byte(`class Documenter:
    def filter_members(
        self,
        members,
    ):
        has_doc = bool(members)
`)
	got, ok := FindEnclosingOwner("sphinx/ext/autodoc/__init__.py", src, 6)
	if !ok {
		t.Fatal("expected owner for multiline class method")
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

func TestFindEnclosingOwnerGoMultilineFunctionAndMethod(t *testing.T) {
	t.Run("function", func(t *testing.T) {
		src := []byte(`package p

func Render(
	name string,
) string {
	return name
}
`)
		got, ok := FindEnclosingOwner("pkg/render.go", src, 6)
		if !ok {
			t.Fatal("expected owner for multiline Go function")
		}
		if got.OwnerSymbol != "Render" || got.AnchorSymbol != "Render" {
			t.Fatalf("owner=%+v, want Render", got)
		}
	})

	t.Run("method", func(t *testing.T) {
		src := []byte(`package p

func (s *Server) Serve(
	ctx context.Context,
) error {
	return nil
}
`)
		got, ok := FindEnclosingOwner("pkg/server.go", src, 6)
		if !ok {
			t.Fatal("expected owner for multiline Go method")
		}
		if got.OwnerSymbol != "Server.Serve" || got.AnchorSymbol != "Serve" {
			t.Fatalf("owner=%+v, want Server.Serve", got)
		}
	})
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

func TestFindEnclosingOwnerBraceMultilineFunctionAndMethod(t *testing.T) {
	t.Run("exported function", func(t *testing.T) {
		src := []byte(`export function render(
  value: string,
) {
  return value;
}
`)
		got, ok := FindEnclosingOwner("src/render.ts", src, 4)
		if !ok {
			t.Fatal("expected owner for multiline brace-language function")
		}
		if got.OwnerSymbol != "render" || got.AnchorSymbol != "render" {
			t.Fatalf("owner=%+v, want render", got)
		}
	})

	t.Run("class method", func(t *testing.T) {
		src := []byte(`class Widget {
  render(
    value: string,
  ) {
    return value;
  }
}
`)
		got, ok := FindEnclosingOwner("src/widget.ts", src, 5)
		if !ok {
			t.Fatal("expected owner for multiline brace-language method")
		}
		if got.OwnerSymbol != "Widget.render" || got.AnchorSymbol != "render" {
			t.Fatalf("owner=%+v, want Widget.render", got)
		}
	})

	t.Run("class opening brace on next line", func(t *testing.T) {
		src := []byte(`class Widget
{
  render(
    value: string,
  ) {
    return value;
  }
}
`)
		got, ok := FindEnclosingOwner("src/widget.ts", src, 6)
		if !ok {
			t.Fatal("expected owner for brace-language class with split opening brace")
		}
		if got.OwnerSymbol != "Widget.render" || got.AnchorSymbol != "render" {
			t.Fatalf("owner=%+v, want Widget.render", got)
		}
	})
}

func TestFindEnclosingOwnerCppHeaderInlineFunction(t *testing.T) {
	src := []byte(`#include <string>

inline std::string render_year(long long calendar_year) {
    return std::to_string(calendar_year);
}

inline std::string format_tm_year(const tm_parts& parts) {
    long long calendar_year = parts.year_offset + 1900;
    return render_year(calendar_year);
}
`)
	got, ok := FindEnclosingOwner("include/tmfmt.hpp", src, 8)
	if !ok {
		t.Fatal("expected owner for C++ inline header function")
	}
	if got.OwnerSymbol != "format_tm_year" || got.AnchorSymbol != "format_tm_year" {
		t.Fatalf("owner=%+v, want format_tm_year", got)
	}

	got, ok = FindEnclosingOwner("include/tmfmt.hpp", src, 9)
	if !ok {
		t.Fatal("expected owner to remain enclosing function on return call")
	}
	if got.OwnerSymbol != "format_tm_year" || got.AnchorSymbol != "format_tm_year" {
		t.Fatalf("return call should not replace owner, got %+v", got)
	}
}

func TestFindEnclosingOwnerCppQualifiedFunction(t *testing.T) {
	src := []byte(`namespace parser {

std::string Parser::Parse(
    std::string_view input
) const {
    return std::string(input);
}
}`)
	got, ok := FindEnclosingOwner("src/parser.cpp", src, 6)
	if !ok {
		t.Fatal("expected owner for qualified C++ function")
	}
	if got.OwnerSymbol != "Parser::Parse" || got.AnchorSymbol != "Parse" {
		t.Fatalf("owner=%+v, want Parser::Parse", got)
	}
}

func TestFindEnclosingOwnerRubyMultilineMethod(t *testing.T) {
	src := []byte(`class Renderer
  def call(
    value
  )
    value
  end
end
`)
	got, ok := FindEnclosingOwner("lib/renderer.rb", src, 5)
	if !ok {
		t.Fatal("expected owner for multiline Ruby method")
	}
	if got.OwnerSymbol != "Renderer.call" || got.AnchorSymbol != "call" {
		t.Fatalf("owner=%+v, want Renderer.call", got)
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
