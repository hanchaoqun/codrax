package preview

import (
	"bytes"
	"html"
	"strings"
	"testing"
)

// Both customer surfaces are reproduced from the frozen HMC278 B report:
// 20261021-185446.182-56024.md (SHA256 23b1ff9e95b360bcb195714f9ce1b5c5872270216a66fbd9ef0384d34bf72a70),
// lines 24 (free bold list) and 100 (system metric excerpt). The original
// HTML turned the inner namespace-qualified type into an invented link.
func TestHMC279QualifiedIdentifierPublicHTML(t *testing.T) {
	const symbol = "OHOS::HDI::Sensor::V3_0::SensorCallbackProxy::OnDataEvent_(OHOS::HDI::Sensor::V3_0::HdfSensorEvents const&, OHOS::sptr<OHOS::IRemoteObject>)"
	for _, tc := range []struct{ name, markdown, literal string }{
		{"customer_bold_list", "2. **" + symbol + "** — 用户态回调。", symbol},
		{"customer_system_excerpt", "> **系统补充：结构化指标摘录**：symbol=\"" + symbol + "\"; samples=82", symbol},
		{"nested_generics", "**std::vector<ns::Box<other::Item>>**", "std::vector<ns::Box<other::Item>>"},
		{"table", "| 类型 |\n| --- |\n| Box<app::model::Value_2> |", "Box<app::model::Value_2>"},
		{"standalone", "<Alpha::Beta>", "<Alpha::Beta>"},
		{"ambiguous_custom_uri_is_literal", "<custom::opaque>", "<custom::opaque>"},
		{"unicode_identifiers", "Box<领域::类型2>", "Box<领域::类型2>"},
		{"comparisons", "a < b && b > c; left<right; n <= limit; limit >= n", "a < b && b > c; left<right; n <= limit; limit >= n"},
		{"pointer", "**Box<ns::Type*>**", "Box<ns::Type*>"},
		{"reference", "**Box<ns::Type&>**", "Box<ns::Type&>"},
		{"array", "**Box<ns::Type[]>**", "Box<ns::Type[]>"},
		{"multi_pointer", "**Box<ns::Type**>**", "Box<ns::Type**>"},
		{"rvalue_reference", "**Box<ns::Type&&>**", "Box<ns::Type&&>"},
		{"array_extent", "**Box<ns::Type[4]>**", "Box<ns::Type[4]>"},
		{"multiple_parameters", "**Pair<ns::A,ns::B>**", "Pair<ns::A,ns::B>"},
		{"builtin_parameter", "**Pair<ns::A,int>**", "Pair<ns::A,int>"},
		{"cv_emphasis", "**Box<ns::A const*>**", "Box<ns::A const*>"},
		{"combined_decorations", "**Box<const ns::A* volatile*, other::B&&, int[2]>**", "Box<const ns::A* volatile*, other::B&&, int[2]>"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source := []byte(tc.markdown)
			before := append([]byte(nil), source...)
			for _, render := range []struct {
				name string
				call func([]byte) (string, error)
			}{
				{"live", RenderMarkdownHTML},
				{"standalone", func(s []byte) (string, error) { return RenderStandaloneMarkdownHTML("literal identity", s) }},
			} {
				out, err := render.call(source)
				if err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(out, html.EscapeString(tc.literal)) {
					t.Fatalf("%s lost literal identity %q", render.name, tc.literal)
				}
				if strings.HasPrefix(tc.markdown, "**") && strings.HasSuffix(tc.markdown, "**") && !strings.Contains(out, "<strong>"+html.EscapeString(tc.literal)+"</strong>") {
					t.Fatalf("%s lost existing emphasis for %q", render.name, tc.literal)
				}
				if strings.Contains(out, `href="OHOS::IRemoteObject"`) || strings.Contains(out, `href="Alpha::Beta"`) || strings.Contains(out, `href="other::Item"`) {
					t.Fatalf("%s invented an identifier hyperlink", render.name)
				}
			}
			if !bytes.Equal(source, before) {
				t.Fatal("renderer rewrote source markdown")
			}
		})
	}
}

func TestHMC279QualifiedIdentifierPreservesLinkAndSafetyBoundaries(t *testing.T) {
	for _, tc := range []struct{ name, markdown, want string }{
		{"angle_https", "<https://example.com/a?q=x&v=y>", `href="https://example.com/a?q=x&amp;v=y"`},
		{"bare_https", "https://example.com/a", `href="https://example.com/a"`},
		{"angle_email", "<dev@example.com>", `href="mailto:dev@example.com"`},
		{"bare_email", "dev@example.com", `href="mailto:dev@example.com"`},
		{"custom_uri", "<custom:opaque>", `href="custom:opaque"`},
		{"urn", "<urn:isbn:9780000000000>", `href="urn:isbn:9780000000000"`},
		{"ipv6", "<https://[2001:db8::1]/path>", `href="https://%5B2001:db8::1%5D/path"`},
		{"explicit_identifier_destination", "[explicit destination](Alpha::Beta)", `href="Alpha::Beta"`},
		{"explicit_ambiguous_uri_destination", "[explicit destination](custom::opaque)", `href="custom::opaque"`},
		{"explicit_decorated_destination", "[explicit destination](custom::opaque*)", `href="custom::opaque*"`},
		{"scoped_uri_path", "<custom::opaque/path>", `href="custom::opaque/path"`},
		{"scoped_uri_query", "<custom::opaque?x=y>", `href="custom::opaque?x=y"`},
		{"scoped_uri_fragment", "<custom::opaque#part>", `href="custom::opaque#part"`},
		{"inline_code", "`Box<Alpha::Beta>`", "<code>Box&lt;Alpha::Beta&gt;</code>"},
		{"code_fence", "```cpp\nBox<Alpha::Beta>\n```", "Box&lt;Alpha::Beta&gt;"},
		{"raw_html", "<script>alert(1)</script>", "&lt;script&gt;alert(1)&lt;/script&gt;"},
		{"raw_event_handler", "<img src=x onerror=alert(1)>", "&lt;img src=x onerror=alert(1)&gt;"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := RenderMarkdownHTML([]byte(tc.markdown))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out, tc.want) {
				t.Fatalf("lost existing link/code/safety behavior %q:\n%s", tc.want, out)
			}
			if strings.Contains(out, "<script>") || strings.Contains(out, "<img src=x") {
				t.Fatalf("raw HTML gained execution authority:\n%s", out)
			}
		})
	}
}

func TestHMC279QualifiedIdentifierDoesNotRewriteMermaid(t *testing.T) {
	const body = "flowchart LR\nA[\"Box<Alpha::Beta>\"] --> B[\"complete\"]"
	out, err := RenderMarkdownHTML([]byte("```mermaid\n" + body + "\n```"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `class="mermaid"`) || !strings.Contains(out, html.EscapeString(body)) {
		t.Fatalf("shared prose parsing altered mermaid source:\n%s", out)
	}
	if strings.Contains(out, `href="Alpha::Beta"`) {
		t.Fatal("mermaid source became a prose link")
	}
}
