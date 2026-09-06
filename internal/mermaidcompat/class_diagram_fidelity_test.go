package mermaidcompat

import (
	"strings"
	"testing"
)

// The bundled Mermaid class database adds each later class body to the same
// class object (addMembers -> addMember -> addClass); an earlier declaration
// must not make later model-authored members disappear in the portable view.
func TestNormalizeClassDiagramToFlowchart_PreservesSplitClassMembers(t *testing.T) {
	tests := []struct {
		name, body, label string
	}{
		{
			name:  "declaration before body",
			body:  "class Order\nclass Order {\n+submit()\n}",
			label: "Order<br/>+submit()",
		},
		{
			name:  "declaration after body",
			body:  "class Order {\n+submit()\n}\nclass Order",
			label: "Order<br/>+submit()",
		},
		{
			name:  "members across bodies",
			body:  "class Order {\n+String id\n}\nclass Order {\n+submit()\n+cancel()\n}",
			label: "Order<br/>+String id<br/>+submit()<br/>+cancel()",
		},
		{
			name:  "duplicate members remain authored",
			body:  "class Order {\n+submit()\n}\nclass Order {\n+submit()\n}",
			label: "Order<br/>+submit()<br/>+submit()",
		},
		{
			name:  "annotations and escaped members",
			body:  "class Order\nclass Order {\n<<service>>\n+List<Item> pending\n}\nclass Order {\n+accept(Item& item)\n}",
			label: "Order<br/>&lt;&lt;service&gt;&gt;<br/>+List&lt;Item&gt; pending<br/>+accept(Item&amp; item)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := "classDiagram\n" + tt.body + "\nclass Store\nOrder --> Store : persists"
			got, ok := NormalizeClassDiagramToFlowchart(in)
			if !ok {
				t.Fatalf("split class declarations are supported by Mermaid: %q", in)
			}
			want := "flowchart TD\n    Order[\"" + tt.label + "\"]\n    Store[\"Store\"]\n    Order -->|\"persists\"| Store"
			if got != want {
				t.Fatalf("split declaration lost or reordered content:\nwant:\n%s\ngot:\n%s", want, got)
			}
			if again, converted := NormalizeClassDiagramToFlowchart(got); converted || again != got {
				t.Fatalf("converted source must be stable: converted=%t source=%q", converted, again)
			}
		})
	}
}

func TestNormalizeClassDiagramToFlowchart_SplitClassesPreserveNodeAndEdgeOrder(t *testing.T) {
	in := "classDiagram\nclass First\nclass Second {\n+ready()\n}\nFirst --> Second : sends\nclass First {\n+start()\n}\nSecond --> Third : delivers\nclass Second {\n+finish()\n}"
	got, ok := NormalizeClassDiagramToFlowchart(in)
	if !ok {
		t.Fatal("split declarations should convert")
	}
	want := "flowchart TD\n    First[\"First<br/>+start()\"]\n    Second[\"Second<br/>+ready()<br/>+finish()\"]\n    Third[\"Third\"]\n    First -->|\"sends\"| Second\n    Second -->|\"delivers\"| Third"
	if got != want {
		t.Fatalf("declarations or edges changed order:\nwant:\n%s\ngot:\n%s", want, got)
	}
}

// A UML operator and an authored relation label are separate facts. A plain
// flowchart arrow has only one label slot: replacing the operator with that
// label would lose extends/implements/ownership/dependency semantics. Keep
// the original class source for the native viewer when both must survive.
func TestNormalizeClassDiagramToFlowchart_PreservesLabeledUMLOperators(t *testing.T) {
	for _, operator := range []string{"<|--", "--|>", "<|..", "..|>", "*--", "--*", "o--", "--o", "<..", "..>"} {
		t.Run(operator, func(t *testing.T) {
			in := "classDiagram\n  class A\n  class B\n  A " + operator + " B : provides business behavior"
			got, ok := NormalizeClassDiagramToFlowchart(in)
			if ok || got != in {
				t.Fatalf("cannot discard UML %q or its authored label: converted=%t source=%q", operator, ok, got)
			}
			if normalized := NormalizeSourceForMarkdown(got); normalized != in {
				t.Fatalf("shared Markdown repair must also preserve the native class source: %q", normalized)
			}
		})
	}
}

func TestNormalizeClassDiagramToFlowchart_IncompleteLaterBodyKeepsOriginal(t *testing.T) {
	in := "classDiagram\nclass Order {\n+submit()\n}\nclass Order {\n+cancel()"
	if got, ok := NormalizeClassDiagramToFlowchart(in); ok || got != in {
		t.Fatalf("an incomplete later declaration must not become a partial successful diagram: %t %q", ok, got)
	}
}

func TestNormalizeClassDiagramToFlowchart_AssociationLabelsStillConvert(t *testing.T) {
	for _, operator := range []string{"-->", "<--"} {
		t.Run(operator, func(t *testing.T) {
			in := "classDiagram\n  A " + operator + " B : fulfills the order"
			got, ok := NormalizeClassDiagramToFlowchart(in)
			if !ok || !strings.Contains(got, `|"fulfills the order"|`) {
				t.Fatalf("directed associations already have a lossless flowchart form: %t %q", ok, got)
			}
			before, after := ParseEdges(in), ParseEdges(got)
			if len(before) != 1 || len(after) != 1 || before[0].From != after[0].From || before[0].To != after[0].To ||
				before[0].Label != strings.Trim(after[0].Label, `"`) {
				t.Fatalf("association semantics changed: before=%+v after=%+v", before, after)
			}
		})
	}
}
