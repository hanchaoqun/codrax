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
