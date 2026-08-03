package orchestrator

import "testing"

func TestNormalizeControllerPathCanonicalizesLexicalAliases(t *testing.T) {
	tests := map[string]string{
		"src/main//java/App.java": "src/main/java/App.java",
		"./src/./main.go":         "src/main.go",
		`src\main\App.java`:       "src/main/App.java",
	}
	for raw, want := range tests {
		if got := normalizeControllerPath(raw); got != want {
			t.Fatalf("normalizeControllerPath(%q)=%q, want %q", raw, got, want)
		}
	}
}
