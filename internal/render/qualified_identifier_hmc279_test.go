package render

import (
	"strings"
	"testing"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

func TestHMC279QualifiedIdentifierTerminalParity(t *testing.T) {
	const symbol = "OHOS::sptr<OHOS::IRemoteObject>"
	source := []byte("**" + symbol + "** and std::vector<ns::Box<other::Item>>; **Pair<ns::A const*,other::B&&>**; <https://example.com> <dev@example.com> [explicit](Alpha::Beta)")
	r := New(nil, true)
	doc := r.markdown.md.Parser().Parse(text.NewReader(source))
	var url, email, explicit bool
	if err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := n.(type) {
		case *ast.AutoLink:
			label := string(node.Label(source))
			if label == "OHOS::IRemoteObject" || label == "other::Item" {
				t.Fatalf("terminal parsed qualified identity as an autolink: %s", label)
			}
			url = url || label == "https://example.com"
			email = email || label == "dev@example.com"
		case *ast.Link:
			explicit = explicit || string(node.Destination) == "Alpha::Beta"
		}
		return ast.WalkContinue, nil
	}); err != nil {
		t.Fatal(err)
	}
	if !url || !email || !explicit {
		t.Fatalf("existing link intent lost: URL=%t email=%t explicit=%t", url, email, explicit)
	}
	plain := stripAnsiEscapes(r.RenderMarkdown(string(source)))
	for _, literal := range []string{symbol, "std::vector<ns::Box<other::Item>>", "Pair<ns::A const*,other::B&&>"} {
		if !strings.Contains(plain, literal) {
			t.Fatalf("terminal lost original identity %q: %s", literal, plain)
		}
	}
}
