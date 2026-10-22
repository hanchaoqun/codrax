package markdownext

import (
	"bytes"
	"html"
	"strings"
	"testing"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

func TestHMC279QualifiedIdentifierGrammar(t *testing.T) {
	for _, literal := range []string{
		"<Alpha::Beta>", "<ns_2::Type_9>", "<_private::item3::field>",
		"<领域::类型2>", "<cafe\u0301::Value>",
		"<ns::Type*>", "<ns::Type&>", "<ns::Type[]>", "<ns::Type**>", "<ns::Type&&>", "<ns::Type[4]>",
		"<ns::Type[Count][dims::Width]>", "<ns::A,ns::B>", "<ns::A,int>", "<ns::A const*>",
		"<const ns::A* volatile*, other::B&&, int[2]>",
	} {
		t.Run(literal, func(t *testing.T) {
			if got := qualifiedIdentifierLength([]byte(literal + " suffix")); got != len(literal) {
				t.Fatalf("literal length=%d want=%d", got, len(literal))
			}
			source := []byte("**" + literal + "**")
			md := goldmark.New(goldmark.WithExtensions(QualifiedIdentifierLiteral, extension.Linkify))
			doc := md.Parser().Parse(text.NewReader(source))
			if err := ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
				if entering && (n.Kind() == ast.KindAutoLink || n.Kind() == ast.KindRawHTML) {
					t.Fatalf("qualified identity gained markup/link authority: %s", n.Kind())
				}
				return ast.WalkContinue, nil
			}); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := md.Renderer().Render(&out, source, doc); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), "<strong>"+html.EscapeString(literal)+"</strong>") {
				t.Fatalf("not literal within existing emphasis: %s", out.String())
			}
		})
	}
	for _, other := range []string{
		"", "<", "<Type>", "<ns:Type>", "<ns::>", "<::Type>", "<ns:::Type>",
		"<2ns::Type>", "<ns::2Type>", "<ns::Type attr=x>", "<ns::Type\n>",
		"<https://example.com>", "<user@example.com>", "<custom:opaque>", "<urn:isbn:9780000000000>",
		"<script>alert(1)</script>", "<img src=x onerror=alert(1)>", "<ns::Type/>",
		"<ns::Type/path>", "<ns::Type?query>", "<ns::Type#fragment>", "<ns::Type constancy>",
		"<ns::Type&&&>", "<ns::Type&&*>", "<ns::Type[1+2]>", "<ns::Type()>", "<ns::A,>",
		"<ns::Type const*\n>", "<Plain*,other::Type>", "<ns::A, https://example.com>",
	} {
		t.Run("leave_"+other, func(t *testing.T) {
			if got := qualifiedIdentifierLength([]byte(other)); got != 0 {
				t.Fatalf("claimed non-identifier syntax %q: %d", other, got)
			}
		})
	}
}
