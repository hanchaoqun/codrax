package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestExtractPython_DecoratedClassRetainsEveryInheritanceRelation(t *testing.T) {
	src := []byte(`from .registry import register

@register("json")
class JsonPlugin(TimestampMixin, ValidationMixin, BasePlugin):
    pass
`)
	root, ok := parseTreeSitterIfPossible(types.LangPython, src)
	if !ok {
		t.Fatal("python tree-sitter parser unavailable")
	}
	_, _, _, rels := extractPython(root, src, "pipeline/plugins.py")
	want := map[string]bool{
		"TimestampMixin":  false,
		"ValidationMixin": false,
		"BasePlugin":      false,
	}
	for _, rel := range rels {
		if rel.Kind != "inheritance" || rel.FromEP.Name != "JsonPlugin" {
			continue
		}
		if _, expected := want[rel.ToEP.Name]; !expected {
			continue
		}
		if rel.Provenance != types.ProvenanceTreeSitter || rel.ResolvedBy != "python_base_class" || rel.Line != 4 {
			t.Fatalf("decorated-class relation lost AST authority: %+v", rel)
		}
		want[rel.ToEP.Name] = true
	}
	for base, found := range want {
		if !found {
			t.Fatalf("decorated JsonPlugin missing inheritance edge to %s: %+v", base, rels)
		}
	}
	var decoration *types.Relation
	for i := range rels {
		if rels[i].Kind == "decoration" {
			decoration = &rels[i]
			break
		}
	}
	if decoration == nil || decoration.FromEP.Name != "register" || decoration.ToEP.Name != "JsonPlugin" ||
		decoration.Line != 3 || decoration.Provenance != types.ProvenanceTreeSitter ||
		decoration.ResolvedBy != "python_literal_decorator_application" ||
		decoration.Metadata["selector_literal"] != "json" || decoration.Metadata["application_surface"] != `@register("json")` {
		t.Fatalf("literal decorator application lost its exact selector role: %+v", decoration)
	}
}

func TestExtractPython_LiteralDecoratorApplicationDoesNotInventRegistrationSemantics(t *testing.T) {
	src := []byte(`class Handler:
    @deprecated("legacy")
    def handle(self):
        pass

@dynamic_selector(NAME)
class Dynamic:
    pass
`)
	root, ok := parseTreeSitterIfPossible(types.LangPython, src)
	if !ok {
		t.Fatal("python tree-sitter parser unavailable")
	}
	_, _, _, rels := extractPython(root, src, "handlers.py")
	var applications []types.Relation
	for _, rel := range rels {
		if rel.Kind == "decoration" {
			applications = append(applications, rel)
		}
	}
	if len(applications) != 1 {
		t.Fatalf("only the static literal decorator should be retained: %+v", applications)
	}
	got := applications[0]
	if got.FromEP.Name != "deprecated" || got.ToEP.Name != "handle" || got.ToEP.Receiver != "Handler" ||
		got.Metadata["selector_literal"] != "legacy" || got.Metadata["application_surface"] != `@deprecated("legacy")` {
		t.Fatalf("unexpected decorated-method relation: %+v", got)
	}
	if got.Kind == "registration" || got.ResolvedBy == "python_registry_binding" {
		t.Fatalf("decorator syntax must not invent registry semantics: %+v", got)
	}
}

func TestParseFiles_DecoratedClassesRetainEveryInheritanceRelation(t *testing.T) {
	repo := t.TempDir()
	rel := "pipeline/plugins.py"
	abs := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	src := []byte(`"""Concrete plugins. Base order in the class statement IS the MRO
order for the cooperative handle() chain."""

from .base import BasePlugin, TimestampMixin, ValidationMixin
from .registry import register

@register("csv")
class CsvPlugin(ValidationMixin, BasePlugin):
    """CSV rows: validation only — bulk imports carry their own
    timestamps, stamping here would overwrite them."""

    def content_type(self) -> str:
        return "text/csv"

@register("json")
class JsonPlugin(TimestampMixin, ValidationMixin, BasePlugin):
    """JSON events: timestamp first, then validation, then base.
    MRO: JsonPlugin -> TimestampMixin -> ValidationMixin -> BasePlugin."""

    def content_type(self) -> str:
        return "application/json"
`)
	if err := os.WriteFile(abs, src, 0o644); err != nil {
		t.Fatal(err)
	}
	infos := ParseFiles([]FileEntry{{RelPath: rel, AbsPath: abs, Language: types.LangPython, Size: int64(len(src))}}, repo)
	if len(infos) != 1 {
		t.Fatalf("ParseFiles returned %d records", len(infos))
	}
	want := map[string]bool{
		"TimestampMixin":  false,
		"ValidationMixin": false,
		"BasePlugin":      false,
	}
	for _, relation := range infos[0].Relations {
		if relation.Kind == "inheritance" && relation.FromEP.Name == "JsonPlugin" {
			if _, ok := want[relation.ToEP.Name]; ok {
				want[relation.ToEP.Name] = true
			}
		}
	}
	for base, found := range want {
		if !found {
			t.Fatalf("production ParseFiles path dropped JsonPlugin -> %s: %+v", base, infos[0].Relations)
		}
	}
}
