package mermaidcompat

import (
	"strings"
	"testing"
)

func TestExplicitNodeLabelNormalizationIsSafeAndIdempotent(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"第一行\r\n第二行\r第三行\n第四行", "第一行<br/>第二行<br/>第三行<br/>第四行"},
		{`a<br/>b<br>c<br />d &amp; &quot;x&quot;`, `a<br/>b<br>c<br />d &amp; &quot;x&quot;`},
		{`a < b > c & "quoted"`, `a < b > c & &quot;quoted&quot;`},
		{`C:\work\new\report & escaped \"quoted\"`, `C:&#92;work&#92;new&#92;report & escaped &#92;&quot;quoted&#92;&quot;`},
		{`C:\temp\new\`, `C:&#92;temp&#92;new&#92;`},
		{`literal \\ pair`, `literal &#92;&#92; pair`},
		{`literal \n not newline`, `literal &#92;n not newline`},
		{"\n[\"edge\"]\nparticipant Other\nA --> B\n", "[&quot;edge&quot;]<br/>participant Other<br/>A --> B"},
	} {
		got, err := NormalizeExplicitNodeLabel(tc.input)
		if err != nil || got != tc.want {
			t.Fatalf("label %q: got=%q err=%v want=%q", tc.input, got, err, tc.want)
		}
		if again, err := NormalizeExplicitNodeLabel(got); err != nil || again != got {
			t.Fatalf("already encoded labels must not be double-escaped: %q -> %q (%v)", got, again, err)
		}
	}
}

func TestAddExplicitNodeDeclarationCheckedMultilineCannotInject(t *testing.T) {
	const id = "rt_e215c998b0d2aab43132b4211c35b9d6897dfac6f4871e97b720427aff6ccf51"
	for _, header := range []string{"flowchart LR", "sequenceDiagram", "classDiagram"} {
		t.Run(header, func(t *testing.T) {
			original := "%% retained comment\n" + header + "\n"
			body, err := AddExplicitNodeDeclarationChecked(original, id, "visible\r\n\"]\nparticipant Other\nA --> B; click A \"evil\"\n%% comment")
			if err != nil {
				t.Fatal(err)
			}
			if strings.Count(body, "\n") != strings.Count(original, "\n")+1 || len(ParseEdges(body)) != 0 {
				t.Fatalf("label escaped its one declaration:\n%s", body)
			}
			var declarations []NodeDecl
			for _, line := range strings.Split(body, "\n") {
				switch header {
				case "sequenceDiagram":
					declarations = append(declarations, SequenceParticipantDeclarations(strings.TrimSpace(line))...)
				case "classDiagram":
					declarations = append(declarations, classNodeDeclarations(line)...)
				default:
					declarations = append(declarations, NodeDeclarationsAll(line)...)
				}
			}
			if len(declarations) != 1 || declarations[0].Ident != id {
				t.Fatalf("label invented node identity: %+v\n%s", declarations, body)
			}
			if changed, err := AddExplicitNodeDeclarationChecked(body, id, "replacement"); err == nil || !strings.Contains(err.Error(), "already has an explicit declaration") || changed != body {
				t.Fatalf("existing declaration must remain immutable: err=%v body=%q", err, changed)
			}
		})
	}
}

func TestAddExplicitNodeDeclarationCheckedPreciseFailure(t *testing.T) {
	for _, tc := range []struct{ name, body, id, label, reason string }{
		{"unsafe id", "flowchart LR\n", "A;B", "label", "invalid endpoint identifier"},
		{"sequence qualified boundary", "flowchart LR\n", "Logger.log", "label", "invalid endpoint identifier"},
		{"nul", "sequenceDiagram\n", "New", "label\x00", "unsupported control character U+0000"},
		{"trimmed control", "flowchart LR\n", "New", "\vlabel", "unsupported control character U+000B"},
		{"del", "classDiagram\n", "New", "label\x7f", "unsupported control character U+007F"},
		{"utf8", "flowchart LR\n", "New", "label\xff", "invalid UTF-8"},
		{"empty", "flowchart LR\n", "New", " \n ", "empty display text"},
		{"existing", "flowchart LR\n Existing[\"immutable\"]", "Existing", "different", "already has an explicit declaration"},
		{"unsupported", "stateDiagram-v2\n", "New", "label", "unsupported Mermaid family"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := AddExplicitNodeDeclarationChecked(tc.body, tc.id, tc.label)
			if got != tc.body || err == nil || !strings.Contains(err.Error(), tc.reason) {
				t.Fatalf("expected exact failure %q without mutation: got=%q err=%v", tc.reason, got, err)
			}
			if tc.name != "unsupported" && strings.Contains(err.Error(), "unsupported Mermaid family") {
				t.Fatalf("failure misreported as diagram family: %v", err)
			}
			if compat, ok := AddExplicitNodeDeclaration(tc.body, tc.id, tc.label); ok || compat != tc.body {
				t.Fatalf("legacy bool adapter did not preserve failure: ok=%v body=%q", ok, compat)
			}
		})
	}
}
