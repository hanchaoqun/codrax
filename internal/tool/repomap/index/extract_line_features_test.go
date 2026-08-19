package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestExtractLineFeatures_AssignmentAndMemberInitializers(t *testing.T) {
	cases := []struct {
		name string
		lang string
		src  string
		line int
		want types.LineFeature
	}{
		{
			name: "go short assignment",
			lang: types.LangGo,
			src: `package p

func f() {
	cfg := Config{}
}
`,
			line: 4,
			want: types.LineFeatureAssignment,
		},
		{
			name: "go keyed composite literal",
			lang: types.LangGo,
			src: `package p

func f() {
	cfg := Config{
		EntryConditions: critEntry("ready"),
	}
	_ = cfg
}
`,
			line: 5,
			want: types.LineFeatureMemberInitializer,
		},
		{
			name: "typescript object member",
			lang: types.LangTypeScript,
			src: `const cfg = {
  entryConditions: buildEntry("ready"),
};
`,
			line: 2,
			want: types.LineFeatureMemberInitializer,
		},
		{
			name: "c designated initializer",
			lang: types.LangC,
			src: `struct config cfg = {
  .entry_conditions = build_entry(),
};
`,
			line: 2,
			want: types.LineFeatureMemberInitializer,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := parseSourceFor(t, tc.lang, tc.src)
			got := extractLineFeatures(root, []byte(tc.src))
			if !lineFeatureSetContains(got[tc.line], tc.want) {
				t.Fatalf("line %d features=%v, want %s", tc.line, got[tc.line], tc.want)
			}
		})
	}
}

func TestExtractMemberInitializerBindingsPreserveOnlyExplicitContainerIdentity(t *testing.T) {
	tests := []struct {
		name, lang, src string
		line            int
		wantMember      string
		wantOwner       string
	}{
		{
			name: "go keyed composite", lang: types.LangGo, line: 5,
			src:        "package p\n\nfunc f(bus *BusContext) {\n ac := &AgentContext{\n  Mutable: bus.Mutable,\n }\n _ = ac\n}\n",
			wantMember: "Mutable", wantOwner: "AgentContext",
		},
		{
			name: "rust struct expression", lang: types.LangRust, line: 3,
			src:        "fn f(bus: BusContext) {\n let ac = AgentContext {\n  mutable_state: bus.mutable_state,\n };\n}\n",
			wantMember: "mutable_state", wantOwner: "AgentContext",
		},
		{
			name: "typescript explicitly typed object", lang: types.LangTypeScript, line: 2,
			src:        "const ac: AgentContext = {\n mutable: bus.mutable,\n};\n",
			wantMember: "mutable", wantOwner: "AgentContext",
		},
		{
			name: "c designated initializer", lang: types.LangC, line: 2,
			src:        "struct AgentContext ac = {\n .mutable_state = bus.mutable_state,\n};\n",
			wantMember: "mutable_state", wantOwner: "AgentContext",
		},
		{
			name: "cpp designated initializer", lang: types.LangCpp, line: 2,
			src:        "AgentContext ac = {\n .mutable_state = bus.mutable_state,\n};\n",
			wantMember: "mutable_state", wantOwner: "AgentContext",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := parseSourceFor(t, tc.lang, tc.src)
			got := extractMemberInitializerBindings(root, []byte(tc.src), tc.lang)[tc.line]
			if len(got) != 1 || got[0].Member != tc.wantMember || got[0].OwnerType != tc.wantOwner {
				t.Fatalf("line %d member initializer bindings=%+v, want %s.%s", tc.line, got, tc.wantOwner, tc.wantMember)
			}
		})
	}

	for _, tc := range []struct {
		name, lang, src string
	}{
		{name: "javascript untyped object", lang: types.LangJavaScript, src: "const ac = {\n mutable: bus.mutable,\n};\n"},
		{name: "typescript untyped object", lang: types.LangTypeScript, src: "const ac = {\n mutable: bus.mutable,\n};\n"},
		{name: "typescript nested untyped object", lang: types.LangTypeScript, src: "const ac: AgentContext = {\n options: { mutable: bus.mutable },\n};\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := parseSourceFor(t, tc.lang, tc.src)
			got := extractMemberInitializerBindings(root, []byte(tc.src), tc.lang)
			if tc.name == "typescript nested untyped object" {
				// The outer typed `options` member may be qualified, but the
				// inner `mutable` field must not borrow AgentContext.
				for _, rows := range got {
					for _, row := range rows {
						if row.Member == "mutable" {
							t.Fatalf("inner untyped object borrowed outer owner: %+v", got)
						}
					}
				}
				return
			}
			if len(got) != 0 {
				t.Fatalf("untyped object minted container identity: %+v", got)
			}
		})
	}
}

func TestArkTSMemberInitializerBindingsUseTierOneTypeScriptAST(t *testing.T) {
	src := []byte("const ac: AgentContext = {\n mutable: bus.mutable,\n};\n")
	_, _, _, _, _, bindings, _, tier := extractArkTSWithStructuralFeatures(src, "entry/src/main/ets/state.ets")
	if tier != 1 || len(bindings[2]) != 1 || bindings[2][0].OwnerType != "AgentContext" || bindings[2][0].Member != "mutable" {
		t.Fatalf("ArkTS tier=%d bindings=%+v", tier, bindings)
	}
}

func TestParseOneFilePublishesMemberInitializerBindings(t *testing.T) {
	for _, tc := range []struct {
		name, lang, rel, src, member, owner string
		line                                int
	}{
		{
			name: "go production parser", lang: types.LangGo, rel: "builder.go", line: 5,
			src:    "package p\n\nfunc f(bus *BusContext) {\n ac := &AgentContext{\n  Mutable: bus.Mutable,\n }\n _ = ac\n}\n",
			member: "Mutable", owner: "AgentContext",
		},
		{
			name: "arkts early return", lang: types.LangArkTS, rel: "state.ets", line: 2,
			src:    "const ac: AgentContext = {\n mutable: bus.mutable,\n};\n",
			member: "mutable", owner: "AgentContext",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			abs := filepath.Join(dir, tc.rel)
			if err := os.WriteFile(abs, []byte(tc.src), 0o600); err != nil {
				t.Fatal(err)
			}
			fi := parseOneFile(FileEntry{AbsPath: abs, RelPath: tc.rel, Language: tc.lang, Size: int64(len(tc.src))})
			rows := fi.MemberInitializerBindings[tc.line]
			if len(rows) != 1 || rows[0].Member != tc.member || rows[0].OwnerType != tc.owner {
				t.Fatalf("FileInfo member initializer bindings=%+v", fi.MemberInitializerBindings)
			}
		})
	}
}

func lineFeatureSetContains(features []types.LineFeature, want types.LineFeature) bool {
	for _, got := range features {
		if got == want {
			return true
		}
	}
	return false
}

func TestHarmonySpecializedExtractorsPublishReturnCallLineFeatures(t *testing.T) {
	if extractorVersions[types.LangArkTS] < 10 || extractorVersions[types.LangCangjie] < 7 {
		t.Fatalf("Harmony LineFeatures output changed without cache generation bumps: ArkTS=%d Cangjie=%d",
			extractorVersions[types.LangArkTS], extractorVersions[types.LangCangjie])
	}
	t.Run("arkts tier1 typescript grammar", func(t *testing.T) {
		src := []byte(`function render(input: string): string {
  return transform(input, rewrite)
}

function rewrite(input: string): string {
  return input
}
`)
		_, _, _, _, features, tier := extractArkTSWithLineFeatures(src, "pages/Index.ets")
		if tier != 1 {
			t.Fatalf("ArkTS tier=%d, want 1", tier)
		}
		if !lineFeatureSetContains(features[2], types.LineFeatureReturnStmt) ||
			!lineFeatureSetContains(features[2], types.LineFeatureCallExpression) {
			t.Fatalf("ArkTS line 2 features=%v, want return+call", features[2])
		}
	})

	t.Run("cangjie lexer and call parser", func(t *testing.T) {
		src := []byte(`package demo
func render(input: String): String {
  // return ignored(input)
  let note = "return hidden(input)"
  return transform(input, rewrite)
}
func rewrite(input: String): String {
  return input
}
`)
		_, _, _, _, features, tier := extractCangjieWithLineFeatures(src, "src/render.cj")
		if tier != 1 {
			t.Fatalf("Cangjie tier=%d, want 1", tier)
		}
		if !lineFeatureSetContains(features[5], types.LineFeatureReturnStmt) ||
			!lineFeatureSetContains(features[5], types.LineFeatureCallExpression) {
			t.Fatalf("Cangjie line 5 features=%v, want return+call", features[5])
		}
		if lineFeatureSetContains(features[3], types.LineFeatureReturnStmt) ||
			lineFeatureSetContains(features[4], types.LineFeatureReturnStmt) {
			t.Fatalf("Cangjie comments/strings must not mint return features: line3=%v line4=%v", features[3], features[4])
		}
	})
}
