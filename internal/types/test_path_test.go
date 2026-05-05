package types

import "testing"

func TestLooksLikeTestFilePath(t *testing.T) {
	positive := []string{
		"internal/agent/finalizer_test.go",
		"tests/foo.rs",
		"foo/test_something.py",
		"foo/module_test.py",
		"com/acme/MyClassTest.java",
		"com/acme/ITSearch.java",
		"src/test/kotlin/com/acme/MyServiceTest.kt",
		"spec/models/user_spec.rb",
		"test/models/user_test.rb",
		"Tests/AppTests/FeatureTests.swift",
		"lua/spec/router_spec.lua",
		"src/components/Button.test.tsx",
		"src/app.spec.ts",
		"native/foo_unittest.cpp",
	}
	for _, path := range positive {
		if !LooksLikeTestFilePath(path) {
			t.Fatalf("LooksLikeTestFilePath(%q) = false, want true", path)
		}
	}

	negative := []string{
		"internal/agent/tester.go",
		"src/testimonial/page.tsx",
		"com/acme/Tester.java",
		"foo/testing.py",
		"app/bin/setup.rb",
		"lua/inspect.lua",
		"native/test_support.cpp",
	}
	for _, path := range negative {
		if LooksLikeTestFilePath(path) {
			t.Fatalf("LooksLikeTestFilePath(%q) = true, want false", path)
		}
	}
}

func TestLooksLikeAuxiliaryEvidencePath(t *testing.T) {
	positive := []string{
		"internal/agent/finalizer_test.go",
		"docs/architecture.md",
		"README.md",
		"fixtures/config.yaml",
		"examples/demo/main.go",
		"testdata/input.json",
		"internal/skill/analysis_contract.go",
	}
	for _, path := range positive {
		if !LooksLikeAuxiliaryEvidencePath(path) {
			t.Fatalf("LooksLikeAuxiliaryEvidencePath(%q) = false, want true", path)
		}
	}

	negative := []string{
		"internal/config/runtime.go",
		"cmd/root.go",
		"internal/types/config.go",
		"codrax.yaml.example",
	}
	for _, path := range negative {
		if LooksLikeAuxiliaryEvidencePath(path) {
			t.Fatalf("LooksLikeAuxiliaryEvidencePath(%q) = true, want false", path)
		}
	}
}

func TestLooksLikePromptSupportPath(t *testing.T) {
	positive := []string{
		"internal/skill/glossary.go",
		"internal/skill/defaults.go",
		"internal/analysis/hint/composer.go",
		"prompts/finalizer.md",
		"skills/config/SKILL.md",
	}
	for _, path := range positive {
		if !LooksLikePromptSupportPath(path) {
			t.Fatalf("LooksLikePromptSupportPath(%q) = false, want true", path)
		}
	}

	negative := []string{
		"internal/config/runtime.go",
		"internal/types/config.go",
		"cmd/root.go",
		"docs/architecture.md",
		"codrax.yaml.example",
	}
	for _, path := range negative {
		if LooksLikePromptSupportPath(path) {
			t.Fatalf("LooksLikePromptSupportPath(%q) = true, want false", path)
		}
	}
}
