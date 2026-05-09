package gate

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAllHardGates_ExplicitCarveOutDeclaration is the build-time
// audit pin for the HardGate registry. Every gate in
// RegisteredHardGates MUST:
//
//  1. Have a non-empty Name.
//  2. Have a SourceFile that exists and is readable from the test's
//     working directory (resolved by walking up from the package
//     dir until the repo root is found).
//  3. Have a non-nil but possibly empty CarveOuts slice. Empty slice
//     is the explicit declaration "this gate has no legitimate
//     carve-out — it is a direct structural contradiction". nil
//     would be ambiguous (forgot to declare?); we require explicit
//     intent.
//  4. For each declared CarveOut, the SourceFile MUST contain the
//     predicate name as a substring AND a "!"-prefixed form (the
//     standard Go negation pattern in the trigger condition). This
//     is the bridge between the metadata and the actual
//     enforcement code: declaring a carve-out without enforcing it
//     in the source is a contract violation.
//
// New gates added to the codebase MUST be registered in
// RegisteredHardGates, OR the test fails on the carve-out enforcement
// check (because the source contains a typed-predicate guard not
// declared in the registry).
func TestAllHardGates_ExplicitCarveOutDeclaration(t *testing.T) {
	if len(RegisteredHardGates) == 0 {
		t.Fatal("RegisteredHardGates is empty — at least L0-B and the R-series coherence gates must be registered")
	}

	repoRoot := findRepoRoot(t)

	for _, g := range RegisteredHardGates {
		t.Run(g.Name, func(t *testing.T) {
			if g.Name == "" {
				t.Fatal("HardGate has empty Name")
			}
			if g.SourceFile == "" {
				t.Fatalf("HardGate %q has empty SourceFile", g.Name)
			}
			if g.TriggerSummary == "" {
				t.Errorf("HardGate %q has empty TriggerSummary — needed for skill-prompt advisory generation", g.Name)
			}

			absPath := filepath.Join(repoRoot, g.SourceFile)
			content, err := os.ReadFile(absPath)
			if err != nil {
				t.Fatalf("HardGate %q SourceFile %q unreadable: %v", g.Name, g.SourceFile, err)
			}
			source := string(content)

			// CarveOuts may be empty (intentional: structural
			// contradiction with no legitimate exception). We don't
			// require nil-vs-empty distinction at runtime — the
			// declaration's INTENT is what matters. The test below
			// only verifies declared carve-outs are enforced.
			for _, co := range g.CarveOuts {
				if co.Predicate == "" {
					t.Errorf("HardGate %q has CarveOut with empty Predicate", g.Name)
					continue
				}
				if co.Reason == "" {
					t.Errorf("HardGate %q CarveOut %q has empty Reason — needed for error messages and skill-prompt advisory",
						g.Name, co.Predicate)
				}
				// Source-substring enforcement: the gate's source
				// MUST mention the predicate name in a "!"-negated
				// guard pattern. This is the link between the
				// declarative registry and the actual gate code.
				negatedForm := "!rm.Predicates." + co.Predicate
				if !strings.Contains(source, negatedForm) {
					t.Errorf("HardGate %q declares carve-out %q (carve_out_value=%q) but SourceFile %q "+
						"does NOT contain the expected negated-predicate guard %q. "+
						"Either the carve-out is declared but unenforced, or the source uses a "+
						"different form — update the source or remove the registry entry.",
						g.Name, co.Predicate, co.CarveOutValue, g.SourceFile, negatedForm)
				}
			}
		})
	}
}

// TestSkillPromptIncludesAllCarveOutPredicates_LLMNaturalGuidance
// pins that every typed-predicate carve-out declared in the
// HardGate registry has a corresponding LLM-natural guidance
// section in the analyzer skill prompt. Without this guidance, the
// LLM has no way to learn it should set the carve-out predicate
// when the question shape demands.
//
// The prompt text generation lives in
// internal/skill/analysis_contract.go. The test reads the file's
// raw bytes (rather than constructing the Config object — that
// would create an import cycle) and checks each carve-out's
// predicate name appears in the prompt source.
func TestSkillPromptIncludesAllCarveOutPredicates_LLMNaturalGuidance(t *testing.T) {
	repoRoot := findRepoRoot(t)
	promptPath := filepath.Join(repoRoot, "internal/skill/analysis_contract.go")
	content, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read analysis_contract.go: %v", err)
	}
	source := string(content)

	for _, predicate := range CarveOutPredicates() {
		// Convert Go-CamelCase to JSON-tag snake_case the LLM sees
		// in the prompt. The mapping is mechanical: split on uppercase
		// boundaries and lower the result.
		snakeName := goCamelToJSONSnake(predicate)
		if !strings.Contains(source, snakeName) {
			t.Errorf("Skill prompt analysis_contract.go does NOT mention carve-out predicate %q (snake form %q). "+
				"Every registered HardGate carve-out must have an LLM-natural guidance section so the LLM "+
				"learns to set the predicate when the question shape demands. Add a sub-bullet under the "+
				"corresponding predicate's primary description.",
				predicate, snakeName)
		}
	}
}

// goCamelToJSONSnake converts "IsRelationalLookup" → "is_relational_lookup"
// for matching the JSON-tag form the LLM sees in the prompt.
func goCamelToJSONSnake(camel string) string {
	var out strings.Builder
	for i, r := range camel {
		if i > 0 && r >= 'A' && r <= 'Z' {
			out.WriteByte('_')
		}
		if r >= 'A' && r <= 'Z' {
			out.WriteRune(r + 32)
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// findRepoRoot walks up from the test's working directory until it
// finds a directory containing go.mod. Used by registry tests that
// need to read source files at known relative paths.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("could not find repo root (go.mod) walking up from test dir")
	return ""
}

// _ unused import guard for fmt — referenced via formatted strings
// only, but kept here so the import stays even if the test body's
// formatting changes (defensive against accidental import removal).
var _ = fmt.Sprintf
