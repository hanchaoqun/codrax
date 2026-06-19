package types

import (
	"path/filepath"
	"strings"
)

// VerificationLanguageFamily is a typed audit label for the language surface
// touched by a patch or exercised by verification. It is telemetry and proof
// routing input only; hard workflow gates must still read exact paths, parser
// outcomes, command verdicts, and policy enums.
type VerificationLanguageFamily string

const (
	VerificationLanguageUnknown        VerificationLanguageFamily = ""
	VerificationLanguageGo             VerificationLanguageFamily = "go"
	VerificationLanguagePython         VerificationLanguageFamily = "python"
	VerificationLanguageJavaScript     VerificationLanguageFamily = "javascript"
	VerificationLanguageTypeScript     VerificationLanguageFamily = "typescript"
	VerificationLanguageJava           VerificationLanguageFamily = "java"
	VerificationLanguageKotlin         VerificationLanguageFamily = "kotlin"
	VerificationLanguageRust           VerificationLanguageFamily = "rust"
	VerificationLanguageC              VerificationLanguageFamily = "c"
	VerificationLanguageCpp            VerificationLanguageFamily = "cpp"
	VerificationLanguageCuda           VerificationLanguageFamily = "cuda"
	VerificationLanguageObjectiveC     VerificationLanguageFamily = "objective_c"
	VerificationLanguageObjectiveCpp   VerificationLanguageFamily = "objective_cpp"
	VerificationLanguageRuby           VerificationLanguageFamily = "ruby"
	VerificationLanguageSwift          VerificationLanguageFamily = "swift"
	VerificationLanguageLua            VerificationLanguageFamily = "lua"
	VerificationLanguageProto          VerificationLanguageFamily = "proto"
	VerificationLanguageArkTS          VerificationLanguageFamily = "arkts"
	VerificationLanguageCangjie        VerificationLanguageFamily = "cangjie"
	VerificationLanguagePHP            VerificationLanguageFamily = "php"
	VerificationLanguageConfigWorkflow VerificationLanguageFamily = "config_workflow"
)

var supportedVerificationLanguageFamilies = []VerificationLanguageFamily{
	VerificationLanguageGo,
	VerificationLanguagePython,
	VerificationLanguageJavaScript,
	VerificationLanguageTypeScript,
	VerificationLanguageJava,
	VerificationLanguageKotlin,
	VerificationLanguageRust,
	VerificationLanguageC,
	VerificationLanguageCpp,
	VerificationLanguageCuda,
	VerificationLanguageObjectiveC,
	VerificationLanguageObjectiveCpp,
	VerificationLanguageRuby,
	VerificationLanguageSwift,
	VerificationLanguageLua,
	VerificationLanguageProto,
	VerificationLanguageArkTS,
	VerificationLanguageCangjie,
	VerificationLanguagePHP,
	VerificationLanguageConfigWorkflow,
}

// SupportedVerificationLanguageFamilies returns every typed family Codrax can
// audit in write final reports. It intentionally mirrors the read-mode
// repomap matrix plus source extensions that the write/test surface already
// treats as code or workflow/config.
func SupportedVerificationLanguageFamilies() []VerificationLanguageFamily {
	out := make([]VerificationLanguageFamily, len(supportedVerificationLanguageFamilies))
	copy(out, supportedVerificationLanguageFamilies)
	return out
}

func NormalizeVerificationLanguageFamilies(in []VerificationLanguageFamily) []VerificationLanguageFamily {
	seen := map[VerificationLanguageFamily]bool{}
	out := make([]VerificationLanguageFamily, 0, len(in))
	for _, family := range in {
		family = normalizeVerificationLanguageFamily(family)
		if family == VerificationLanguageUnknown || seen[family] {
			continue
		}
		seen[family] = true
		out = append(out, family)
	}
	return out
}

func normalizeVerificationLanguageFamily(raw VerificationLanguageFamily) VerificationLanguageFamily {
	text := strings.ToLower(strings.TrimSpace(string(raw)))
	text = strings.ReplaceAll(text, "-", "_")
	switch text {
	case "go", "golang":
		return VerificationLanguageGo
	case "py", "python", "pytest", "unittest", "django":
		return VerificationLanguagePython
	case "js", "javascript", "node", "nodejs", "jest", "vitest":
		return VerificationLanguageJavaScript
	case "ts", "tsx", "typescript":
		return VerificationLanguageTypeScript
	case "java", "jvm", "maven", "mvn", "junit":
		return VerificationLanguageJava
	case "kt", "kts", "kotlin", "gradle":
		return VerificationLanguageKotlin
	case "rs", "rust", "cargo":
		return VerificationLanguageRust
	case "c":
		return VerificationLanguageC
	case "cc", "cpp", "cxx", "c++":
		return VerificationLanguageCpp
	case "cuda":
		return VerificationLanguageCuda
	case "objc", "objective_c", "objective-c":
		return VerificationLanguageObjectiveC
	case "objcpp", "objective_cpp", "objective-cpp", "objective_c++":
		return VerificationLanguageObjectiveCpp
	case "rb", "ruby", "rspec", "minitest":
		return VerificationLanguageRuby
	case "swift":
		return VerificationLanguageSwift
	case "lua":
		return VerificationLanguageLua
	case "proto", "protobuf":
		return VerificationLanguageProto
	case "ets", "arkts", "hvigor":
		return VerificationLanguageArkTS
	case "cj", "cangjie", "cjpm":
		return VerificationLanguageCangjie
	case "php":
		return VerificationLanguagePHP
	case "config", "workflow", "config_workflow":
		return VerificationLanguageConfigWorkflow
	default:
		return VerificationLanguageUnknown
	}
}

// VerificationLanguageFamiliesFromPath derives language families from a
// repo-relative path or typed patch-effect language. Path shape is precise
// enough for audit grouping but must not be used as a semantic proof by itself.
func VerificationLanguageFamiliesFromPath(path string) []VerificationLanguageFamily {
	path = strings.ReplaceAll(strings.TrimSpace(path), `\`, `/`)
	if path == "" {
		return nil
	}
	lower := strings.ToLower(path)
	base := strings.ToLower(filepath.Base(lower))
	var out []VerificationLanguageFamily
	switch {
	case strings.HasPrefix(lower, ".github/workflows/") &&
		(strings.HasSuffix(lower, ".yml") || strings.HasSuffix(lower, ".yaml")):
		out = append(out, VerificationLanguageConfigWorkflow)
	}
	switch base {
	case "go.mod", "go.sum", "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock",
		"pyproject.toml", "setup.cfg", "requirements.txt", "tox.ini", "noxfile.py",
		"cargo.toml", "cargo.lock", "pom.xml", "build.gradle", "build.gradle.kts",
		"settings.gradle", "settings.gradle.kts", "cmakelists.txt", "makefile", "meson.build",
		"oh-package.json5", "build-profile.json5", "hvigorfile.ts", "cjpm.toml":
		out = append(out, VerificationLanguageConfigWorkflow)
	}
	switch ext := strings.ToLower(filepath.Ext(base)); ext {
	case ".go":
		out = append(out, VerificationLanguageGo)
	case ".py", ".pyi":
		out = append(out, VerificationLanguagePython)
	case ".js", ".jsx", ".mjs", ".cjs":
		out = append(out, VerificationLanguageJavaScript)
	case ".ts", ".tsx":
		out = append(out, VerificationLanguageTypeScript)
	case ".java":
		out = append(out, VerificationLanguageJava)
	case ".kt", ".kts":
		out = append(out, VerificationLanguageKotlin)
	case ".rs":
		out = append(out, VerificationLanguageRust)
	case ".c":
		out = append(out, VerificationLanguageC)
	case ".h":
		out = append(out, VerificationLanguageC)
	case ".cc", ".cpp", ".cxx", ".hh", ".hpp", ".hxx":
		out = append(out, VerificationLanguageCpp)
	case ".cu", ".cuh":
		out = append(out, VerificationLanguageCuda, VerificationLanguageCpp)
	case ".m":
		out = append(out, VerificationLanguageObjectiveC, VerificationLanguageC)
	case ".mm":
		out = append(out, VerificationLanguageObjectiveCpp, VerificationLanguageCpp)
	case ".rb":
		out = append(out, VerificationLanguageRuby)
	case ".swift":
		out = append(out, VerificationLanguageSwift)
	case ".lua":
		out = append(out, VerificationLanguageLua)
	case ".proto":
		out = append(out, VerificationLanguageProto)
	case ".ets":
		out = append(out, VerificationLanguageArkTS)
	case ".cj":
		out = append(out, VerificationLanguageCangjie)
	case ".php":
		out = append(out, VerificationLanguagePHP)
	case ".yaml", ".yml", ".json", ".json5", ".toml", ".ini", ".xml":
		out = append(out, VerificationLanguageConfigWorkflow)
	}
	if strings.HasSuffix(base, ".d.ts") {
		out = append(out, VerificationLanguageTypeScript)
	}
	return NormalizeVerificationLanguageFamilies(out)
}

func VerificationLanguageFamiliesFromRunner(runner, framework string) []VerificationLanguageFamily {
	runner = strings.ToLower(strings.TrimSpace(runner))
	framework = strings.ToLower(strings.TrimSpace(framework))
	var out []VerificationLanguageFamily
	out = append(out, normalizeVerificationLanguageFamily(VerificationLanguageFamily(runner)))
	out = append(out, normalizeVerificationLanguageFamily(VerificationLanguageFamily(framework)))
	switch runner {
	case "node":
		out = append(out, VerificationLanguageJavaScript, VerificationLanguageTypeScript)
	case "java":
		out = append(out, VerificationLanguageJava, VerificationLanguageKotlin)
	case "cmake", "meson", "make":
		out = append(out, VerificationLanguageC, VerificationLanguageCpp)
	case "hvigor":
		out = append(out, VerificationLanguageArkTS)
	case "cjpm":
		out = append(out, VerificationLanguageCangjie)
	}
	return NormalizeVerificationLanguageFamilies(out)
}

func VerificationLanguageFamiliesFromVerificationProbeSuite(suite string) []VerificationLanguageFamily {
	suite = strings.TrimSpace(suite)
	const prefix = "verification_probe/"
	if !strings.HasPrefix(suite, prefix) {
		return nil
	}
	rest := strings.TrimPrefix(suite, prefix)
	if rest == "" || strings.Contains(rest, "/") {
		return nil
	}
	return NormalizeVerificationLanguageFamilies([]VerificationLanguageFamily{
		normalizeVerificationLanguageFamily(VerificationLanguageFamily(rest)),
	})
}

func VerificationLanguageFamiliesFromReport(report *ChangeReport) []VerificationLanguageFamily {
	if report == nil {
		return nil
	}
	var out []VerificationLanguageFamily
	for _, cmd := range report.ExecutedCommands {
		out = append(out, VerificationLanguageFamiliesFromRunner(cmd.Runner, cmd.Framework)...)
		out = append(out, VerificationLanguageFamiliesFromVerificationProbeSuite(cmd.Suite)...)
	}
	for _, runner := range report.NoTestsRunners {
		out = append(out, VerificationLanguageFamiliesFromRunner(runner, "")...)
	}
	for _, result := range report.TestResults {
		out = append(out, VerificationLanguageFamiliesFromVerificationProbeSuite(result.Suite)...)
		for _, err := range result.BuildErrors {
			out = append(out, VerificationLanguageFamiliesFromPath(err.File)...)
		}
	}
	if report.TestSurface != nil {
		for _, cand := range report.TestSurface.Candidates {
			out = append(out, VerificationLanguageFamiliesFromRunner(cand.Runner, cand.Framework)...)
			out = append(out, VerificationLanguageFamiliesFromPath(cand.Source)...)
		}
	}
	return NormalizeVerificationLanguageFamilies(out)
}
