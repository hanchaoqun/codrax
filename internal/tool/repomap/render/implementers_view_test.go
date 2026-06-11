package render

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/index"
	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// The implementers view lists every concrete type implementing the
// named interface from the structural Implements relation — the typed
// replacement for the model reading candidate files one by one.
func TestImplementersView_ExhaustiveAndGrounded(t *testing.T) {
	iface := &types.FileInfo{RelPath: "loop/ctl.go", Language: "go", Package: "loop",
		Symbols: []types.Symbol{
			{Name: "LoopController", Kind: "interface"},
			{Name: "Step", Kind: "method", Parent: "LoopController"},
			{Name: "Done", Kind: "method", Parent: "LoopController"},
		}}
	mkImpl := func(rel, name string) *types.FileInfo {
		return &types.FileInfo{RelPath: rel, Language: "go", Package: "loop",
			Symbols: []types.Symbol{
				{Name: name, Kind: "struct"},
				{Name: "Step", Receiver: name, Kind: "method"},
				{Name: "Done", Receiver: name, Kind: "method"},
			}}
	}
	g := index.BuildGraph(".", []*types.FileInfo{iface, mkImpl("loop/a.go", "AlphaLoop"), mkImpl("loop/b.go", "BetaLoop")})

	d := GenerateViewData(g, "implementers", types.ViewParams{Query: "LoopController"})
	if d == nil || d.Type != "implementers" {
		t.Fatalf("expected implementers view, got %+v", d)
	}
	md := RenderMarkdown(d)
	for _, want := range []string{"AlphaLoop", "BetaLoop", "loop/a.go", "loop/b.go", "(2)"} {
		if !strings.Contains(md, want) {
			t.Fatalf("implementers view missing %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "LoopController (interface)") {
		t.Fatal("the interface itself must not be listed as its own implementer")
	}
}

func TestImplementersView_NoMatchGuidesFallback(t *testing.T) {
	g := index.BuildGraph(".", []*types.FileInfo{
		{RelPath: "x.go", Language: "go", Package: "x", Symbols: []types.Symbol{{Name: "Thing", Kind: "struct"}}}})
	d := GenerateViewData(g, "implementers", types.ViewParams{Query: "NoSuchIface"})
	md := RenderMarkdown(d)
	if !strings.Contains(md, "No implementers") || !strings.Contains(md, "grep") {
		t.Fatalf("unknown interface must guide a grep fallback, got:\n%s", md)
	}
}
