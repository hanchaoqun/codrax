package types

import "testing"

func TestVerificationLanguageFamiliesFromPath_CoversCodraxLanguageMatrix(t *testing.T) {
	cases := []struct {
		path string
		want []VerificationLanguageFamily
	}{
		{"cmd/main.go", []VerificationLanguageFamily{VerificationLanguageGo}},
		{"pkg/__init__.py", []VerificationLanguageFamily{VerificationLanguagePython}},
		{"web/app.js", []VerificationLanguageFamily{VerificationLanguageJavaScript}},
		{"web/app.ts", []VerificationLanguageFamily{VerificationLanguageTypeScript}},
		{"src/main.java", []VerificationLanguageFamily{VerificationLanguageJava}},
		{"src/Main.kt", []VerificationLanguageFamily{VerificationLanguageKotlin}},
		{"src/lib.rs", []VerificationLanguageFamily{VerificationLanguageRust}},
		{"src/native.c", []VerificationLanguageFamily{VerificationLanguageC}},
		{"include/native.h", []VerificationLanguageFamily{VerificationLanguageC}},
		{"src/native.cpp", []VerificationLanguageFamily{VerificationLanguageCpp}},
		{"include/native.hpp", []VerificationLanguageFamily{VerificationLanguageCpp}},
		{"src/kernel.cu", []VerificationLanguageFamily{VerificationLanguageCuda, VerificationLanguageCpp}},
		{"src/AppDelegate.m", []VerificationLanguageFamily{VerificationLanguageObjectiveC, VerificationLanguageC}},
		{"src/AppDelegate.mm", []VerificationLanguageFamily{VerificationLanguageObjectiveCpp, VerificationLanguageCpp}},
		{"lib/task.rb", []VerificationLanguageFamily{VerificationLanguageRuby}},
		{"Sources/App/main.swift", []VerificationLanguageFamily{VerificationLanguageSwift}},
		{"lua/plugin.lua", []VerificationLanguageFamily{VerificationLanguageLua}},
		{"proto/service.proto", []VerificationLanguageFamily{VerificationLanguageProto}},
		{"entry/src/main/ets/pages/Index.ets", []VerificationLanguageFamily{VerificationLanguageArkTS}},
		{"src/main.cj", []VerificationLanguageFamily{VerificationLanguageCangjie}},
		{"public/index.php", []VerificationLanguageFamily{VerificationLanguagePHP}},
		{".github/workflows/ci.yml", []VerificationLanguageFamily{VerificationLanguageConfigWorkflow}},
		{"oh-package.json5", []VerificationLanguageFamily{VerificationLanguageConfigWorkflow}},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			got := VerificationLanguageFamiliesFromPath(tc.path)
			for _, want := range tc.want {
				if !verificationLanguageFamilyContains(got, want) {
					t.Fatalf("VerificationLanguageFamiliesFromPath(%q) = %v, missing %q", tc.path, got, want)
				}
			}
		})
	}
}

func TestVerificationLanguageFamiliesFromReport_UsesTypedRunnerEvidence(t *testing.T) {
	report := &ChangeReport{
		NoTestsRunners: []string{"cjpm"},
		TestResults: []TestResult{{
			Suite:  "verification_probe/arkts",
			Passed: true,
		}, {
			Kind:   TestResultKindBuildError,
			Suite:  "build",
			Passed: false,
			BuildErrors: []BuildError{{
				File:    "src/kernel.cu",
				Message: "compile failed",
			}},
		}},
		ExecutedCommands: []ExecutedCommand{{
			Runner:     "node",
			ExitCode:   0,
			Outcome:    "executed",
			ReasonCode: "tests_passed",
		}, {
			Runner:   "java",
			ExitCode: 0,
			Outcome:  "executed",
		}, {
			Runner:   "cmake",
			ExitCode: 0,
			Outcome:  "executed",
		}},
		TestSurface: &TestSurface{Candidates: []TestSurfaceCandidate{{
			Runner: "hvigor",
			Source: "oh-package.json5",
		}}},
	}

	got := VerificationLanguageFamiliesFromReport(report)
	for _, want := range []VerificationLanguageFamily{
		VerificationLanguageJavaScript,
		VerificationLanguageTypeScript,
		VerificationLanguageJava,
		VerificationLanguageKotlin,
		VerificationLanguageC,
		VerificationLanguageCpp,
		VerificationLanguageCuda,
		VerificationLanguageArkTS,
		VerificationLanguageCangjie,
		VerificationLanguageConfigWorkflow,
	} {
		if !verificationLanguageFamilyContains(got, want) {
			t.Fatalf("VerificationLanguageFamiliesFromReport() = %v, missing %q", got, want)
		}
	}
}

func TestSupportedVerificationLanguageFamilies_ReturnsDefensiveCopy(t *testing.T) {
	first := SupportedVerificationLanguageFamilies()
	if len(first) == 0 {
		t.Fatal("SupportedVerificationLanguageFamilies returned no families")
	}
	original := first[0]
	first[0] = "mutated"
	second := SupportedVerificationLanguageFamilies()
	if second[0] != original {
		t.Fatalf("SupportedVerificationLanguageFamilies leaked mutable backing slice: got %q want %q", second[0], original)
	}
}

func TestVerificationProbeDirectSourceFamilies_UsesOneProviderRelation(t *testing.T) {
	js := VerificationProbeDirectSourceFamilies("javascript")
	for _, want := range []VerificationLanguageFamily{VerificationLanguageJavaScript, VerificationLanguageTypeScript} {
		if !verificationLanguageFamilyContains(js, want) {
			t.Fatalf("javascript direct families = %v, missing %q", js, want)
		}
	}
	for _, tc := range []struct {
		language string
		want     VerificationLanguageFamily
	}{
		{"go", VerificationLanguageGo},
		{"python", VerificationLanguagePython},
		{"java", VerificationLanguageJava},
		{"ruby", VerificationLanguageRuby},
		{"arkts", VerificationLanguageArkTS},
		{"cangjie", VerificationLanguageCangjie},
	} {
		got := VerificationProbeDirectSourceFamilies(tc.language)
		if len(got) != 1 || got[0] != tc.want {
			t.Fatalf("%s direct families = %v, want [%s]", tc.language, got, tc.want)
		}
	}
}

func verificationLanguageFamilyContains(got []VerificationLanguageFamily, want VerificationLanguageFamily) bool {
	for _, item := range got {
		if item == want {
			return true
		}
	}
	return false
}
