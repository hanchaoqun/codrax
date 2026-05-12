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
