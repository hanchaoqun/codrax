package tool

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/ground"
	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// These white-box fixtures exercise an already-owned declaration and read
// coverage. They do not claim native parser coverage for the named languages;
// the separate public ReadFile/EmitEvidence test supplies that Rust witness.
func b1664RegistrationAuthorityFixture() (types.EvidenceItem, *ground.Context, *repomap.FileInfo) {
	const source = "src/bridge.rs"
	item := types.EvidenceItem{
		ID: "model-registration", Kind: types.EvidenceDirect, Scope: types.ScopeLine,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "_fastlex", Subject: "PyModule", Object: "_fastlex",
		Source: source, LineStart: 10, LineEnd: 10,
		Summary: "Model-authored registration explanation is not a source-role proof.",
	}
	file := &repomap.FileInfo{RelPath: source, ParseTier: 1, Language: repomap.LangRust, Symbols: []repomap.Symbol{{
		Name: "_fastlex", Kind: "function", File: source, Line: 10, EndLine: 14,
		BodyPresence: repomap.CallableBodyPresent, BodyStartLine: 10, BodyEndLine: 14,
		ParameterBindings: []repomap.CallableParameterBinding{{Binding: "m", Type: "&Bound<'_, PyModule>"}},
	}}}
	lines := make(map[int]string)
	for n := 1; n <= 14; n++ {
		lines[n] = ""
	}
	lines[9] = "#[pymodule]"
	lines[10] = "fn _fastlex(m: &Bound<'_, PyModule>) -> PyResult<()> {"
	lines[11] = "    m.add_function(wrapped)?;"
	lines[14] = "}"
	ctx := &ground.Context{
		Graph:     &repomap.Graph{FileIndex: map[string]*repomap.FileInfo{source: file}},
		LineIndex: map[string]map[int]string{source: lines},
	}
	return item, ctx, file
}

func b1664CheckRegistrationAuthority(t *testing.T, item types.EvidenceItem, ctx *ground.Context, file *repomap.FileInfo, want string) {
	t.Helper()
	// Marshal before the read-only call rather than taking a shallow expected
	// copy: parameter slices and read maps must also remain byte-identical.
	snapshot := func() []byte {
		var lines, observed map[string]map[int]string
		var graph *repomap.Graph
		if ctx != nil {
			lines, observed, graph = ctx.LineIndex, ctx.ObservedLineIndex, ctx.Graph
		}
		b, err := json.Marshal(struct {
			Item     types.EvidenceItem
			File     *repomap.FileInfo
			Graph    *repomap.Graph
			Lines    map[string]map[int]string
			Observed map[string]map[int]string
		}{item, file, graph, lines, observed})
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	before := snapshot()
	if got := registrationDefinitionParameterReference(item, ctx); got != want {
		t.Errorf("parameter-only withdrawal binding = %q, want %q (empty means unknown, not valid registration)", got, want)
	}
	if !reflect.DeepEqual(before, snapshot()) {
		t.Fatal("source-role inspection changed the model evidence, parser identities, or observed source")
	}
}

func TestB1664RegistrationAuthorityRequiresCompleteOwnedDeclaration(t *testing.T) {
	tests := []struct {
		name string
		edit func(*types.EvidenceItem, **ground.Context, *repomap.FileInfo)
		want string
	}{
		{"complete owned declaration", nil, "m"},
		{"legacy tier zero is primary", func(_ *types.EvidenceItem, _ **ground.Context, f *repomap.FileInfo) { f.ParseTier = 0 }, "m"},
		{"qualified original identities", func(i *types.EvidenceItem, _ **ground.Context, _ *repomap.FileInfo) {
			i.Subject, i.Object, i.AnchorSymbol = "py::PyModule", "py::_fastlex", "py::_fastlex"
		}, "m"},
		{"nil context", func(_ *types.EvidenceItem, c **ground.Context, _ *repomap.FileInfo) { *c = nil }, ""},
		{"missing graph", func(_ *types.EvidenceItem, c **ground.Context, _ *repomap.FileInfo) { (*c).Graph = nil }, ""},
		{"missing source file", func(i *types.EvidenceItem, c **ground.Context, _ *repomap.FileInfo) {
			delete((*c).Graph.FileIndex, i.Source)
		}, ""},
		{"fallback parser", func(_ *types.EvidenceItem, _ **ground.Context, f *repomap.FileInfo) { f.ParseTier = 3 }, ""},
		{"unread prefix", func(i *types.EvidenceItem, c **ground.Context, _ *repomap.FileInfo) {
			delete((*c).LineIndex[i.Source], 6)
		}, ""},
		{"grep-only prefix is not read coverage", func(i *types.EvidenceItem, c **ground.Context, _ *repomap.FileInfo) {
			(*c).ObservedLineIndex = map[string]map[int]string{i.Source: {6: ""}}
			delete((*c).LineIndex[i.Source], 6)
		}, ""},
		{"unread declaration", func(i *types.EvidenceItem, c **ground.Context, _ *repomap.FileInfo) {
			delete((*c).LineIndex[i.Source], 10)
		}, ""},
		{"ambiguous same-name declaration", func(_ *types.EvidenceItem, _ **ground.Context, f *repomap.FileInfo) {
			other := f.Symbols[0]
			other.Parent = "OtherOwner"
			f.Symbols = append(f.Symbols, other)
		}, ""},
		{"non-callable declaration", func(_ *types.EvidenceItem, _ **ground.Context, f *repomap.FileInfo) { f.Symbols[0].Kind = "struct" }, ""},
		{"unknown body presence", func(_ *types.EvidenceItem, _ **ground.Context, f *repomap.FileInfo) { f.Symbols[0].BodyPresence = "" }, ""},
		{"multiline header", func(i *types.EvidenceItem, c **ground.Context, f *repomap.FileInfo) {
			f.Symbols[0].BodyStartLine = 11
			(*c).LineIndex[i.Source][10] = "fn _fastlex(m: &Bound<'_, PyModule>)"
			(*c).LineIndex[i.Source][11] = "    -> PyResult<()> {"
		}, ""},
		{"inline complete body", func(i *types.EvidenceItem, c **ground.Context, f *repomap.FileInfo) {
			f.Symbols[0].BodyEndLine = 10
			(*c).LineIndex[i.Source][10] = "fn _fastlex(m: &PyModule) { m.register(); }"
		}, ""},
		{"inline body with later close", func(i *types.EvidenceItem, c **ground.Context, _ *repomap.FileInfo) {
			(*c).LineIndex[i.Source][10] = "fn _fastlex(m: &Bound<'_, PyModule>) { m.register();"
		}, ""},
		{"attribute anchor is not declaration line", func(i *types.EvidenceItem, _ **ground.Context, _ *repomap.FileInfo) { i.LineStart = 9 }, ""},
		{"scope file", func(i *types.EvidenceItem, _ **ground.Context, _ *repomap.FileInfo) { i.Scope = types.ScopeFile }, ""},
		{"call role", func(i *types.EvidenceItem, _ **ground.Context, _ *repomap.FileInfo) { i.AnchorKind = types.AnchorCall }, ""},
		{"missing declaration anchor", func(i *types.EvidenceItem, _ **ground.Context, _ *repomap.FileInfo) { i.AnchorSymbol = "" }, ""},
		{"different object", func(i *types.EvidenceItem, _ **ground.Context, _ *repomap.FileInfo) { i.Object = "AnotherCallable" }, ""},
		{"prose subject cannot supply identity", func(i *types.EvidenceItem, _ **ground.Context, _ *repomap.FileInfo) {
			i.Subject = "the PyModule registry"
		}, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			item, ctx, file := b1664RegistrationAuthorityFixture()
			if tc.edit != nil {
				tc.edit(&item, &ctx, file)
			}
			b1664CheckRegistrationAuthority(t, item, ctx, file, tc.want)
		})
	}
}

func TestB1664RegistrationAuthorityLeavesOtherIdentityUsesUnknown(t *testing.T) {
	tests := []struct {
		name string
		edit func(*types.EvidenceItem, *ground.Context, *repomap.Symbol)
	}{
		{"own callable identity", func(i *types.EvidenceItem, _ *ground.Context, s *repomap.Symbol) { i.Subject = s.Name }},
		{"parent type", func(_ *types.EvidenceItem, _ *ground.Context, s *repomap.Symbol) { s.Parent = "py::PyModule" }},
		{"receiver type", func(_ *types.EvidenceItem, _ *ground.Context, s *repomap.Symbol) { s.Receiver = "PyModule" }},
		{"parameter binding same as subject", func(i *types.EvidenceItem, c *ground.Context, s *repomap.Symbol) {
			s.ParameterBindings[0].Binding = "PyModule"
			c.LineIndex[i.Source][10] = "fn _fastlex(PyModule: &Bound<'_, PyModule>) {"
		}},
		{"callback parameter type", func(i *types.EvidenceItem, c *ground.Context, s *repomap.Symbol) {
			s.ParameterBindings[0].Type = "fn(PyModule)"
			c.LineIndex[i.Source][10] = "fn _fastlex(m: fn(PyModule)) {"
		}},
		{"base type in declaration", func(i *types.EvidenceItem, c *ground.Context, _ *repomap.Symbol) {
			c.LineIndex[i.Source][10] = "fn _fastlex(m: &Bound<'_, PyModule>) extends PyModule {"
		}},
		{"return only", func(i *types.EvidenceItem, c *ground.Context, s *repomap.Symbol) {
			s.ParameterBindings[0].Type, s.ReturnTypeNames = "u8", []string{"PyModule"}
			c.LineIndex[i.Source][10] = "fn _fastlex(m: u8) -> PyModule {"
		}},
		{"const expression", func(i *types.EvidenceItem, c *ground.Context, s *repomap.Symbol) {
			i.Subject = "Registry"
			s.ParameterBindings[0].Type = "[u8; Registry]"
			c.LineIndex[i.Source][10] = "fn _fastlex(m: [u8; Registry]) {"
		}},
		{"go array length is a value not a type", func(i *types.EvidenceItem, c *ground.Context, s *repomap.Symbol) {
			i.Subject = "Registry"
			s.ParameterBindings[0].Type = "[Registry]Item"
			c.LineIndex[i.Source][10] = "func _fastlex(m [Registry]Item) {"
		}},
		{"block const expression", func(i *types.EvidenceItem, c *ground.Context, s *repomap.Symbol) {
			s.ParameterBindings[0].Type = "Buffer<{PyModule::LEN}>"
			c.LineIndex[i.Source][10] = "fn _fastlex(m: Buffer<{PyModule::LEN}>) {"
		}},
		{"lifetime identity is not type identity", func(i *types.EvidenceItem, c *ground.Context, s *repomap.Symbol) {
			s.ParameterBindings[0].Type = "&'PyModule u8"
			c.LineIndex[i.Source][10] = "fn _fastlex(m: &'PyModule u8) {"
		}},
		{"subject repeated outside type", func(i *types.EvidenceItem, c *ground.Context, _ *repomap.Symbol) {
			c.LineIndex[i.Source][10] = "fn _fastlex(m: &Bound<'_, PyModule>) -> PyModule {"
		}},
		{"same full type occurs twice", func(i *types.EvidenceItem, c *ground.Context, s *repomap.Symbol) {
			s.ParameterBindings = []repomap.CallableParameterBinding{{Binding: "m", Type: "&PyModule"}, {Binding: "other", Type: "&PyModule"}}
			c.LineIndex[i.Source][10] = "fn _fastlex(m: &PyModule, other: &PyModule) {"
		}},
		{"attached annotation uses subject", func(i *types.EvidenceItem, c *ground.Context, _ *repomap.Symbol) {
			c.LineIndex[i.Source][9] = "#[register(PyModule)]"
		}},
		{"stacked attributes can extend beyond four lines", func(i *types.EvidenceItem, c *ground.Context, _ *repomap.Symbol) {
			c.LineIndex[i.Source][4] = "#[register(PyModule)]"
			for n := 5; n <= 9; n++ {
				c.LineIndex[i.Source][n] = "#[other]"
			}
		}},
		{"blank line does not detach an attribute", func(i *types.EvidenceItem, c *ground.Context, _ *repomap.Symbol) {
			c.LineIndex[i.Source][4] = "#[register(PyModule)]"
			c.LineIndex[i.Source][5] = ""
		}},
		{"multiline annotation starts beyond four-line prefix", func(i *types.EvidenceItem, c *ground.Context, _ *repomap.Symbol) {
			lines := c.LineIndex[i.Source]
			lines[3], lines[4], lines[5] = "#[register(", "PyModule,", "other,"
			lines[6], lines[7], lines[8], lines[9] = "another,", "third,", "fourth", ")]"
		}},
		{"multiline annotation unread distant opener", func(i *types.EvidenceItem, c *ground.Context, _ *repomap.Symbol) {
			delete(c.LineIndex[i.Source], 5)
			c.LineIndex[i.Source][6], c.LineIndex[i.Source][7], c.LineIndex[i.Source][8], c.LineIndex[i.Source][9] = "arg1,", "arg2,", "arg3", ")]"
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			item, ctx, file := b1664RegistrationAuthorityFixture()
			tc.edit(&item, ctx, &file.Symbols[0])
			b1664CheckRegistrationAuthority(t, item, ctx, file, "")
		})
	}
}

func TestB1664RegistrationAuthoritySimpleTypeExpressions(t *testing.T) {
	for _, tc := range []struct{ name, language, subject, typeText, line string }{
		{"rust borrowed generic", "rust", "PyModule", "&Bound<'_, PyModule>", "fn _fastlex(m: &Bound<'_, PyModule>) {"},
		{"rust named lifetime and true type", "rust", "PyModule", "&'PyModule PyModule", "fn _fastlex(m: &'PyModule PyModule) {"},
		{"go pointer", "go", "BusContext", "*types.BusContext", "func _fastlex(m *types.BusContext) {"},
		{"go slice", "go", "Registry", "[]Registry", "func _fastlex(m []Registry) {"},
		{"java generic", "java", "Module", "List<Module>", "void _fastlex(List<Module> m) {"},
		{"cangjie array", "cangjie", "Module", "Array<Module>", "func _fastlex(m: Array<Module>) {"},
		{"arkts array", "arkts", "Registry", "Registry[]", "function _fastlex(m: Registry[]) {"},
		{"cpp qualified reference", "cpp", "Module", "const io::Module&", "void _fastlex(const io::Module& m) {"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item, ctx, file := b1664RegistrationAuthorityFixture()
			item.Subject, file.Language = tc.subject, tc.language
			file.Symbols[0].ParameterBindings[0].Type = tc.typeText
			ctx.LineIndex[item.Source][10] = tc.line
			b1664CheckRegistrationAuthority(t, item, ctx, file, "m")
		})
	}
}

func TestB1664RegistrationAuthorityPrefixNeedsStructuralBoundary(t *testing.T) {
	for _, tc := range []struct{ name, want string }{
		{"same-owner previous body", "m"},
		{"no previous body beyond budget", ""},
		{"different owner", ""},
		{"different receiver", ""},
		{"unread boundary", ""},
		{"unread prefix", ""},
		{"inline suffix at previous end", ""},
		{"body end not declaration end", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			item, ctx, file := b1664RegistrationAuthorityFixture()
			line := ctx.LineIndex[item.Source][10]
			for n := 1; n <= 44; n++ {
				ctx.LineIndex[item.Source][n] = ""
			}
			item.LineStart, item.LineEnd = 40, 40
			decl := &file.Symbols[0]
			decl.Line, decl.EndLine, decl.BodyStartLine, decl.BodyEndLine = 40, 44, 40, 44
			ctx.LineIndex[item.Source][40] = line
			previous := repomap.Symbol{Name: "previous", Kind: "function", Parent: decl.Parent, Line: 33, EndLine: 35,
				BodyPresence: repomap.CallableBodyPresent, BodyStartLine: 33, BodyEndLine: 35}
			ctx.LineIndex[item.Source][35] = "}"
			switch tc.name {
			case "different owner":
				previous.Parent = "other"
			case "different receiver":
				previous.Receiver = "other"
			case "unread boundary":
				delete(ctx.LineIndex[item.Source], 35)
			case "unread prefix":
				delete(ctx.LineIndex[item.Source], 36)
			case "inline suffix at previous end":
				ctx.LineIndex[item.Source][35] = "} #[register(PyModule)]"
			case "body end not declaration end":
				previous.BodyEndLine = 34
			}
			if tc.name != "no previous body beyond budget" {
				file.Symbols = append(file.Symbols, previous)
			}
			b1664CheckRegistrationAuthority(t, item, ctx, file, tc.want)
		})
	}
}
