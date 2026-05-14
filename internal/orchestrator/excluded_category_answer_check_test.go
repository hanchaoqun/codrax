package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestUserExcludedCategoryAnswerCheck_VariableNamesLeak(t *testing.T) {
	mut := types.NewMutableState("列出公开类型和函数；不要列变量")
	doc := docWithExcludedCategorySummary("变量（ErrUnknownKind、defaultExternalArtifactFloor、fileExtensionTokenSuffixes、registered）按要求不列出。")

	got := runUserExcludedCategoryAnswerCheck(doc, mut)
	if len(got) != 1 {
		t.Fatalf("violations = %d, want 1: %+v", len(got), got)
	}
	if got[0].Kind != types.ViolMustExclude {
		t.Fatalf("kind = %q, want %q", got[0].Kind, types.ViolMustExclude)
	}
	for _, want := range []string{"ErrUnknownKind", "defaultExternalArtifactFloor", "fileExtensionTokenSuffixes", "registered"} {
		if !strings.Contains(got[0].Detail, want) {
			t.Fatalf("detail missing %q: %s", want, got[0].Detail)
		}
	}
}

func TestUserExcludedCategoryAnswerCheck_CategoryOnlyDisclosureAllowed(t *testing.T) {
	mut := types.NewMutableState("列出公开类型和函数；不要列变量")
	doc := docWithExcludedCategorySummary("变量类别按要求排除，未列入主答案。")

	if got := runUserExcludedCategoryAnswerCheck(doc, mut); len(got) != 0 {
		t.Fatalf("category-only exclusion disclosure should pass, got %+v", got)
	}
}

func TestUserExcludedCategoryAnswerCheck_IgnoresObjectiveWithoutExclusion(t *testing.T) {
	mut := types.NewMutableState("列出公开变量和函数")
	doc := docWithExcludedCategorySummary("变量（defaultExternalArtifactFloor）也列出。")

	if got := runUserExcludedCategoryAnswerCheck(doc, mut); len(got) != 0 {
		t.Fatalf("non-exclusion objective should pass, got %+v", got)
	}
}

func TestUserExcludedCategoryAnswerCheck_EnglishVariableExclusion(t *testing.T) {
	mut := types.NewMutableState("List public functions, but do not list variables.")
	doc := docWithExcludedCategorySummary("Variables (defaultExternalArtifactFloor, fileExtensionTokenSuffixes) are excluded.")

	got := runUserExcludedCategoryAnswerCheck(doc, mut)
	if len(got) != 1 {
		t.Fatalf("violations = %d, want 1: %+v", len(got), got)
	}
}

func TestUserExcludedCategoryAnswerCheck_FunctionDescriptionNotVariableList(t *testing.T) {
	mut := types.NewMutableState("List public functions; do not list variables.")
	doc := docWithExcludedCategorySummary("SetExternalArtifactFloor adjusts the variable threshold used by the evaluator.")

	if got := runUserExcludedCategoryAnswerCheck(doc, mut); len(got) != 0 {
		t.Fatalf("ordinary function description should not be treated as excluded variable list, got %+v", got)
	}
}

func TestUserExcludedCategoryAnswerCheck_GeneralExcludedCodeCategories(t *testing.T) {
	tests := []struct {
		name      string
		objective string
		answer    string
		want      []string
	}{
		{
			name:      "test helpers",
			objective: "List production symbols, excluding test helpers.",
			answer:    "Test helpers (`buildFakeRepo`, `fixtureRoot`) were excluded.",
			want:      []string{"buildFakeRepo", "fixtureRoot"},
		},
		{
			name:      "generated paths",
			objective: "列出手写生产文件，不要包含生成文件",
			answer:    "生成文件（`internal/gen/schema.pb.go`、`src/generated/client.ts`）按要求排除。",
			want:      []string{"internal/gen/schema.pb.go", "src/generated/client.ts"},
		},
		{
			name:      "private helpers",
			objective: "List the public API and omit private helpers.",
			answer:    "Private helpers (loadCandidateCache, normalize_path, valid?) are not listed.",
			want:      []string{"loadCandidateCache", "normalize_path", "valid?"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := runUserExcludedCategoryAnswerCheck(docWithExcludedCategorySummary(tt.answer), types.NewMutableState(tt.objective))
			if len(got) != 1 {
				t.Fatalf("violations = %d, want 1: %+v", len(got), got)
			}
			for _, want := range tt.want {
				if !strings.Contains(got[0].Detail, want) {
					t.Fatalf("detail missing %q: %s", want, got[0].Detail)
				}
			}
		})
	}
}

func TestUserExcludedCategoryAnswerCheck_CategoryDisclosureWithValidSymbolNotList(t *testing.T) {
	mut := types.NewMutableState("List production symbols, excluding test helpers.")
	doc := docWithExcludedCategorySummary("Test helpers are excluded. TestRunner is a production type and remains in scope.")

	if got := runUserExcludedCategoryAnswerCheck(doc, mut); len(got) != 0 {
		t.Fatalf("separate in-scope symbol should pass, got %+v", got)
	}
}

func TestUserExcludedCategoryAnswerCheck_ASCIICategoryMarkerBoundaries(t *testing.T) {
	mut := types.NewMutableState("List the latest public API without unrelated prose.")
	doc := docWithExcludedCategorySummary("LatestSurface is in scope.")

	if got := runUserExcludedCategoryAnswerCheck(doc, mut); len(got) != 0 {
		t.Fatalf("substring inside latest should not activate test category, got %+v", got)
	}
}

func docWithExcludedCategorySummary(text string) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: text,
		}},
	}
}
