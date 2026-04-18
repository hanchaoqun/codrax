package subject

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// fixtureGraph builds a minimal in-memory repomap.Graph the judges
// can consult. Adds: skill registration in internal/skill/foo.go;
// agent name "explorer" in internal/agent/explorer.go; a YAML-defined
// config key in codrax.yaml.
func fixtureGraph() *repomap.Graph {
	g := &repomap.Graph{
		SymbolDefs: map[string][]*repomap.Symbol{
			// Skill name (lives in internal/skill/)
			"explore-skill": {
				&repomap.Symbol{Name: "explore-skill", Kind: "var", File: "internal/skill/defaults.go", Line: 14},
			},
			// Agent name (lives in internal/agent/)
			"explorer": {
				&repomap.Symbol{Name: "explorer", Kind: "var", File: "internal/agent/explorer.go", Line: 10},
			},
			// Function symbol
			"BuildPromptContext": {
				&repomap.Symbol{Name: "BuildPromptContext", Kind: "function", File: "internal/context/builder.go", Line: 246},
			},
			// Type symbol
			"AnswerSubject": {
				&repomap.Symbol{Name: "AnswerSubject", Kind: "struct", File: "internal/types/analysis_ir.go", Line: 110},
			},
			// Interface symbol
			"Evaluator": {
				&repomap.Symbol{Name: "Evaluator", Kind: "interface", File: "internal/agent/agent.go", Line: 200},
			},
			// Config key in YAML file
			"log_max_files": {
				&repomap.Symbol{Name: "log_max_files", Kind: "field", File: "codrax.yaml", Line: 5},
			},
			// Const enum value
			"IntentEnumerate": {
				&repomap.Symbol{Name: "IntentEnumerate", Kind: "const", File: "internal/types/analysis_ir.go", Line: 87},
			},
		},
	}
	return g
}

// (TestScore_SkillName and TestScore_AgentName deleted — the
// judges they exercised were removed in the Session 11 over-
// fitting audit. Skill / Agent naming conventions are project-
// specific, not industry-general; the taxonomy now relies on
// universal identifier kinds (FunctionName, TypeName,
// ConfigKey, ...) so a token like "explore-skill" is scored as
// a generic quoted string literal, not a project-level role.)

func TestScore_FunctionAndType(t *testing.T) {
	g := fixtureGraph()
	if got := Score("BuildPromptContext", types.SubjectFunctionName, g); got < 0.8 {
		t.Errorf("Score(BuildPromptContext, FunctionName) = %.2f, want >= 0.8", got)
	}
	if got := Score("AnswerSubject", types.SubjectTypeName, g); got < 0.8 {
		t.Errorf("Score(AnswerSubject, TypeName) = %.2f, want >= 0.8", got)
	}
	if got := Score("nonexistent", types.SubjectFunctionName, g); got > 0.1 {
		t.Errorf("Score(nonexistent, FunctionName) = %.2f, want ~0", got)
	}
}

func TestScore_ConfigKey(t *testing.T) {
	g := fixtureGraph()
	if got := Score("log_max_files", types.SubjectConfigKey, g); got < 0.7 {
		t.Errorf("Score(log_max_files, ConfigKey) = %.2f, want >= 0.7", got)
	}
}

func TestScore_FilePath(t *testing.T) {
	if got := Score("internal/agent/explorer.go", types.SubjectFilePath, nil); got < 0.8 {
		t.Errorf("Score(internal/agent/explorer.go, FilePath) = %.2f, want >= 0.8", got)
	}
	if got := Score("just-a-name", types.SubjectFilePath, nil); got > 0.1 {
		t.Errorf("Score(just-a-name, FilePath) = %.2f, want ~0", got)
	}
}

func TestScore_StringLiteralAndNumeric(t *testing.T) {
	if got := Score(`"hello"`, types.SubjectStringLiteral, nil); got < 0.8 {
		t.Errorf("Score(\"hello\", StringLiteral) = %.2f", got)
	}
	if got := Score("42", types.SubjectNumeric, nil); got < 0.8 {
		t.Errorf("Score(42, Numeric) = %.2f", got)
	}
	if got := Score("forty-two", types.SubjectNumeric, nil); got > 0.1 {
		t.Errorf("Score(forty-two, Numeric) = %.2f, want ~0", got)
	}
}

func TestScore_HandlerRoute(t *testing.T) {
	if got := Score("/api/v1/users", types.SubjectHandlerRoute, nil); got < 0.4 {
		t.Errorf("Score(/api/v1/users, HandlerRoute) = %.2f", got)
	}
	if got := Score("GET /users", types.SubjectHandlerRoute, nil); got < 0.7 {
		t.Errorf("Score(GET /users, HandlerRoute) = %.2f", got)
	}
}

func TestScore_EnumValue(t *testing.T) {
	g := fixtureGraph()
	if got := Score("IntentEnumerate", types.SubjectEnumValue, g); got < 0.5 {
		t.Errorf("Score(IntentEnumerate, EnumValue) = %.2f", got)
	}
	if got := Score("ALL_CAPS_NAME", types.SubjectEnumValue, nil); got < 0.5 {
		t.Errorf("Score(ALL_CAPS_NAME, EnumValue) = %.2f", got)
	}
}

func TestScore_UnknownIsZero(t *testing.T) {
	if got := Score("anything", types.SubjectUnknown, nil); got != 0 {
		t.Errorf("Score(anything, Unknown) = %.2f, want 0", got)
	}
}

func TestChainTerminalToken(t *testing.T) {
	cases := []struct {
		summary string
		want    string
	}{
		{
			summary: "`A.foo()` binds NewB → `B.bar()` returns \"explorer\"",
			want:    "explorer",
		},
		{
			summary: "`RegisterDefaults()` registers `explore-skill`",
			want:    "explore-skill",
		},
		{
			summary: "single hop `Foo.Name()`",
			want:    "Foo.Name",
		},
	}
	for _, c := range cases {
		got := ChainTerminalToken(c.summary)
		if got != c.want {
			t.Errorf("ChainTerminalToken(%q) = %q, want %q", c.summary, got, c.want)
		}
	}
}

// (TestScore_BugRegression_SubExplorerNameNotASkill removed in
// the Session 11 over-fitting audit — the SubjectSkillName kind
// it exercised was itself a project-specific concept and got
// deleted with the cleanup. The regression scenario — "don't let
// an agent's Name() value outrank the real answer in a chain
// ranker" — is now covered at the generic-kind level by
// TestScore_FunctionAndType and TestScore_EnumValue.)

// IsAnswerOfKind threshold check — uses a universal kind
// (FunctionName) so the test survives the Session 11 audit.
func TestIsAnswerOfKind_Threshold(t *testing.T) {
	g := fixtureGraph()
	if !IsAnswerOfKind("BuildPromptContext", types.SubjectFunctionName, g) {
		t.Errorf("IsAnswerOfKind(BuildPromptContext, FunctionName) = false, want true")
	}
	if IsAnswerOfKind("nonsense", types.SubjectFunctionName, g) {
		t.Errorf("IsAnswerOfKind(nonsense, FunctionName) = true, want false")
	}
}
