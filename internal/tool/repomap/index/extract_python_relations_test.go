package index

import (
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
