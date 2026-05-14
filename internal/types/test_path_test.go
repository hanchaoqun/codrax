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
		"entry/src/ohosTest/ets/pages/Index.test.ets",
		"src/__tests__/widget.spec.ets",
		"tests/service.proto",
		"tests/cangjie/feature_test.cj",
		"test/cangjie/feature.cj",
		"cuda/kernel_test.cu",
		"objc/foo_test.mm",
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
		"tests/input.yaml",
		"test/input.json",
		"src/__tests__/snapshot.json",
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

func TestClassifySourcePathRole(t *testing.T) {
	cases := []struct {
		path string
		want SourcePathRole
	}{
		{"internal/agent/finalizer_test.go", SourcePathRoleTest},
		{"entry/src/ohosTest/ets/pages/Index.test.ets", SourcePathRoleTest},
		{"tests/cangjie/feature_test.cj", SourcePathRoleTest},
		{"native/foo_unittest.cpp", SourcePathRoleTest},
		{"fixtures/config.yaml", SourcePathRoleFixture},
		{"testdata/input.json", SourcePathRoleFixture},
		{"examples/demo/main.go", SourcePathRoleExample},
		{"docs/architecture.md", SourcePathRoleDocumentation},
		{"README.md", SourcePathRoleDocumentation},
		{"internal/skill/analysis_contract.go", SourcePathRolePromptSupport},
		{"internal/config/runtime.go", SourcePathRoleProduction},
		{"codrax.yaml.example", SourcePathRoleProduction},
	}
	for _, tc := range cases {
		if got := ClassifySourcePathRole(tc.path); got != tc.want {
			t.Fatalf("ClassifySourcePathRole(%q) = %q, want %q", tc.path, got, tc.want)
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
