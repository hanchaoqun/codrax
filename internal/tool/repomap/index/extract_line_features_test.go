package index

import (
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
