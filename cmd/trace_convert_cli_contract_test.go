package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/hanchaoqun/codrax/internal/hitraceconv"
)

func TestTraceHelpSurfacesCanonicalConversionWithoutGlobalFlagNoise(t *testing.T) {
	rootHelp := commandHelpText(t, rootCmd)
	if !strings.Contains(rootHelp, "codrax trace convert --input <binary-trace> [--output <text.systrace>]") {
		t.Fatalf("root help does not surface the canonical trace conversion command:\n%s", rootHelp)
	}
	for _, flagName := range []string{"log", "htrace", "mermaid-render", "auto-init-repo"} {
		flag := rootCmd.Flag(flagName)
		if flag == nil {
			t.Fatalf("root flag --%s is not registered", flagName)
		}
		if strings.Contains(flag.Usage, "`") {
			t.Fatalf("root flag --%s contains a pflag metavar backtick: %q", flagName, flag.Usage)
		}
	}

	traceHelp := commandHelpText(t, traceCmd)
	if !strings.Contains(traceHelp, "Usage:\n  codrax trace [command]") ||
		strings.Contains(traceHelp, "Usage:\n  codrax trace [flags]") {
		t.Fatalf("trace parent help lost command-shaped usage:\n%s", traceHelp)
	}
	if !strings.Contains(traceHelp, "convert") || !strings.Contains(traceHelp, "codrax trace convert --input capture.sys") {
		t.Fatalf("trace parent help does not make conversion discoverable:\n%s", traceHelp)
	}

	convertHelp := commandHelpText(t, traceConvertCmd)
	for _, want := range []string{
		"Input and output:",
		"Trace engine:",
		"Perf sidecars:",
		"Diagnostics:",
		"Common:",
		"--trace-engine",
		"--trace-streamer",
		"--diagnostic-report",
		"hard limit: 900 lines",
		"--archive-member",
		"--cache-dir",
		"codrax trace convert --input capture.sys",
		"codrax trace convert --input capture.sys.zip",
		"detected by content magic",
		"explicit retained trace_streamer SQLite DB path",
		"without this flag or --keep-trace-db the DB and",
		".ohos.ts companion are temporary",
		"retain the trace_streamer SQLite DB at its derived",
		"sidecar path together with any .ohos.ts timestamp companion",
	} {
		if !strings.Contains(convertHelp, want) {
			t.Fatalf("conversion help missing %q:\n%s", want, convertHelp)
		}
	}
	if strings.Contains(convertHelp, "default is derived from --output or input when DB export is enabled") ||
		strings.Contains(convertHelp, "tool sidecars instead of cleaning temporary files") {
		t.Fatalf("conversion help retained the pre-staging DB retention contract:\n%s", convertHelp)
	}
	for _, unrelated := range []string{"--repo", "--write-phase", "--pipeline-max-steps", "--providers"} {
		if strings.Contains(convertHelp, unrelated) {
			t.Fatalf("conversion-only help leaked unrelated global flag %q:\n%s", unrelated, convertHelp)
		}
	}
}

func TestTraceConvertRejectsPositionalArguments(t *testing.T) {
	if err := traceConvertCmd.Args(traceConvertCmd, []string{"capture.sys"}); err == nil {
		t.Fatal("trace convert accepted a positional argument; --input is the sole input surface")
	}
}

func TestTraceConvertStatusOnlyInspectsOtherwiseConflictingOptions(t *testing.T) {
	settingsPath := filepath.Join(t.TempDir(), "settings.yaml")
	if err := os.WriteFile(settingsPath, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODRAX_SETTINGS", settingsPath)

	oldInput, oldOutput := traceConvertInput, traceConvertOutput
	oldEngine, oldStreamer := traceConvertTraceEngine, traceConvertTraceStreamer
	oldDB, oldKeep := traceConvertTraceDBOutput, traceConvertKeepTraceDB
	oldSoDirs := append([]string(nil), traceConvertTraceStreamerSoDirs...)
	oldTraceStatus, oldPerfStatus := traceConvertTraceToolsStatus, traceConvertToolsStatus
	oldPerfParser := traceConvertPerfParser
	t.Cleanup(func() {
		traceConvertInput, traceConvertOutput = oldInput, oldOutput
		traceConvertTraceEngine, traceConvertTraceStreamer = oldEngine, oldStreamer
		traceConvertTraceDBOutput, traceConvertKeepTraceDB = oldDB, oldKeep
		traceConvertTraceStreamerSoDirs = oldSoDirs
		traceConvertTraceToolsStatus, traceConvertToolsStatus = oldTraceStatus, oldPerfStatus
		traceConvertPerfParser = oldPerfParser
		traceConvertCmd.SetOut(nil)
		hitraceconv.SetEmbeddedTraceStreamerCacheRoot("")
	})

	path := filepath.Join(t.TempDir(), "same.sys")
	traceConvertInput = path
	traceConvertOutput = path
	traceConvertTraceEngine = "builtin"
	traceConvertTraceStreamer = "/unused/trace_streamer"
	traceConvertTraceDBOutput = path
	traceConvertKeepTraceDB = true
	traceConvertTraceStreamerSoDirs = []string{"/unused/so"}
	traceConvertPerfParser = "auto"
	traceConvertTraceToolsStatus = true
	traceConvertToolsStatus = false
	var output bytes.Buffer
	traceConvertCmd.SetOut(&output)
	if err := traceConvertCmd.RunE(traceConvertCmd, nil); err != nil {
		t.Fatalf("status-only inspection must not apply conversion collision/conflict gates: %v", err)
	}
	if !strings.Contains(output.String(), "trace_provider[") {
		t.Fatalf("status-only invocation produced no provider evidence:\n%s", output.String())
	}

	traceConvertTraceToolsStatus = false
	if err := traceConvertCmd.RunE(traceConvertCmd, nil); err == nil {
		t.Fatal("actual conversion bypassed the shared option/path-collision authority")
	}
}

func TestTraceConvertUtilityRuntimePrecedenceAndNoRepositoryBootstrap(t *testing.T) {
	oldCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	workDir := t.TempDir()
	if err := os.Chdir(workDir); err != nil {
		t.Fatal(err)
	}
	resolvedWorkDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldCWD)
		hitraceconv.SetEmbeddedTraceStreamerCacheRoot("")
	})

	cacheFlag := traceConvertCmd.Flag("cache-dir")
	langFlag := traceConvertCmd.Flag("lang")
	oldCacheValue, oldCacheChanged := flagCacheDir, cacheFlag.Changed
	oldLangValue, oldLangChanged := flagLang, langFlag.Changed
	t.Cleanup(func() {
		flagCacheDir = oldCacheValue
		cacheFlag.Changed = oldCacheChanged
		flagLang = oldLangValue
		langFlag.Changed = oldLangChanged
	})
	flagCacheDir = ""
	cacheFlag.Changed = false
	flagLang = defaultLang
	langFlag.Changed = false

	settingsPath := filepath.Join(workDir, "settings.yaml")
	if err := os.WriteFile(settingsPath, []byte("cache_dir: yaml-cache\nlang: en\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODRAX_SETTINGS", settingsPath)

	resolved, err := configureTraceConvertUtilityRuntime(traceConvertCmd)
	if err != nil {
		t.Fatalf("resolve YAML utility runtime: %v", err)
	}
	wantYAMLCache := filepath.Join(resolvedWorkDir, runtimeAnchorDir, "yaml-cache")
	wantRuntimeAnchor := filepath.Join(resolvedWorkDir, runtimeAnchorDir)
	if resolved.cacheRoot != wantYAMLCache || resolved.runtimeAnchor != wantRuntimeAnchor ||
		resolved.lang != "en" || flagLang != "en" {
		t.Fatalf("YAML utility runtime = %+v flagLang=%q, want cache=%q lang=en", resolved, flagLang, wantYAMLCache)
	}
	if _, err := os.Stat(filepath.Join(resolvedWorkDir, runtimeAnchorDir)); !os.IsNotExist(err) {
		t.Fatalf("provider-free utility bootstrap created repository runtime state: err=%v", err)
	}

	flagCacheDir = "cli-cache"
	cacheFlag.Changed = true
	flagLang = "zh"
	langFlag.Changed = true
	resolved, err = configureTraceConvertUtilityRuntime(traceConvertCmd)
	if err != nil {
		t.Fatalf("resolve CLI utility runtime: %v", err)
	}
	if resolved.cacheRoot != "cli-cache" || resolved.lang != "zh" {
		t.Fatalf("explicit CLI did not override YAML: %+v", resolved)
	}

	cacheFlag.Changed = false
	langFlag.Changed = false
	absoluteCache := filepath.Join(workDir, "absolute-cache")
	if err := os.WriteFile(settingsPath, []byte("cache_dir: "+absoluteCache+"\nlang: zh\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err = configureTraceConvertUtilityRuntime(traceConvertCmd)
	if err != nil {
		t.Fatalf("resolve absolute YAML cache: %v", err)
	}
	if resolved.cacheRoot != absoluteCache {
		t.Fatalf("absolute YAML cache was re-anchored: got %q want %q", resolved.cacheRoot, absoluteCache)
	}

	if err := os.WriteFile(settingsPath, []byte("cache_dir: \"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err = configureTraceConvertUtilityRuntime(traceConvertCmd)
	if err != nil {
		t.Fatalf("resolve empty YAML cache: %v", err)
	}
	if resolved.cacheRoot != "" {
		t.Fatalf("empty YAML cache must retain default resolver root, got %q", resolved.cacheRoot)
	}

	if err := os.WriteFile(settingsPath, []byte("lang: en\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resolved, err = configureTraceConvertUtilityRuntime(traceConvertCmd)
	if err != nil {
		t.Fatalf("resolve nil YAML cache: %v", err)
	}
	if resolved.cacheRoot != "" || resolved.lang != "en" {
		t.Fatalf("nil cache/default + YAML lang resolution mismatch: %+v", resolved)
	}

	t.Setenv("CODRAX_SETTINGS", "")
	resolved, err = configureTraceConvertUtilityRuntime(traceConvertCmd)
	if err != nil {
		t.Fatalf("resolve code-default utility runtime: %v", err)
	}
	if resolved.cacheRoot != "" || resolved.lang != defaultLang {
		t.Fatalf("code-default utility runtime mismatch: %+v", resolved)
	}
}

func TestTraceConvertUtilitySettingsMissingFileSemantics(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yaml")
	settings, err := loadTraceConvertUtilitySettings(runtimeSettingsLocation{path: missing})
	if err != nil || settings != nil {
		t.Fatalf("non-explicit disappearing settings file must degrade to defaults: settings=%v err=%v", settings, err)
	}
	if _, err := loadTraceConvertUtilitySettings(runtimeSettingsLocation{path: missing, explicit: true}); err == nil {
		t.Fatal("explicit missing CODRAX_SETTINGS must fail loud")
	}
}

func TestTraceConvertUtilitySettingsProjectionIgnoresUnrelatedTypeErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "codrax.yaml")
	body := `cache_dir: trace-cache
lang: en
pipeline_max_steps: [not, an, integer]
write_enabled:
  nested: unrelated
mcp_servers: definitely-not-the-main-schema
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	settings, err := loadTraceConvertUtilitySettings(runtimeSettingsLocation{path: path, explicit: true})
	if err != nil {
		t.Fatalf("unrelated runtime setting types blocked trace conversion projection: %v", err)
	}
	if settings == nil || settings.CacheDir == nil || *settings.CacheDir != "trace-cache" || settings.Lang == nil || *settings.Lang != "en" {
		t.Fatalf("trace conversion projection lost its own fields: %+v", settings)
	}
}

func TestTraceConvertUtilitySettingsProjectionRejectsOwnedFieldTypeErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{name: "cache-dir-map", body: "cache_dir:\n  nested: invalid\n"},
		{name: "lang-sequence", body: "lang: [zh, en]\n"},
		{name: "malformed-yaml", body: "cache_dir: [unterminated\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "codrax.yaml")
			if err := os.WriteFile(path, []byte(tc.body), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := loadTraceConvertUtilitySettings(runtimeSettingsLocation{path: path, explicit: true}); err == nil {
				t.Fatalf("owned trace conversion setting type error was ignored: %s", tc.body)
			}
		})
	}
}

func TestStaticBuildsPinEmbeddedDefaultAndExplicitSlimIdentities(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	makefilePath := filepath.Join(filepath.Dir(currentFile), "..", "Makefile")
	data, err := os.ReadFile(makefilePath)
	if err != nil {
		t.Fatal(err)
	}
	contents := string(data)
	for _, hardening := range []string{
		"STATIC_EXTRA_TAGS ?=",
		"override STATIC_EMBEDDED_BUILD_TAGS := $(STANDARD_EMBEDDED_BUILD_TAGS)$(if $(strip $(STATIC_EXTRA_TAGS)),$(comma)$(strip $(STATIC_EXTRA_TAGS)))",
		"override STATIC_SLIM_BUILD_TAGS := slim_streamer$(if $(strip $(STATIC_EXTRA_TAGS)),$(comma)$(strip $(STATIC_EXTRA_TAGS)))",
		"override STATIC_SLIM_OUT              := $(BINARY)-static-slim",
		"$(error STATIC_TAGS is unsafe and no longer supported",
		"$(error STATIC_EXTRA_TAGS must not contain reserved build identity tags $(STANDARD_EMBEDDED_BUILD_TAGS) or slim_streamer)",
	} {
		if !strings.Contains(contents, hardening) {
			t.Fatalf("Makefile lacks immutable static identity hardening %q", hardening)
		}
	}
	if strings.Contains(contents, "STATIC_TAGS   ?=") || strings.Contains(contents, "STATIC_BUILD_TAGS") {
		t.Fatal("legacy static tag authority remains present")
	}
	embeddedRecipes := 0
	slimRecipes := 0
	for _, line := range strings.Split(contents, "\n") {
		if !strings.Contains(line, "extldflags") || !strings.Contains(line, "-static") {
			continue
		}
		switch {
		case strings.Contains(line, "-o $(BINARY) ."):
			embeddedRecipes++
			if !strings.Contains(line, "-tags '$(STATIC_EMBEDDED_BUILD_TAGS)'") || strings.Contains(line, "STATIC_SLIM_BUILD_TAGS") {
				t.Fatalf("default static recipe lost embedded identity:\n%s", line)
			}
		case strings.Contains(line, "-o $(STATIC_SLIM_OUT) ."), strings.Contains(line, "-o $(DIST_LINUX_AMD64_STATIC_SLIM) ."):
			slimRecipes++
			if !strings.Contains(line, "-tags '$(STATIC_SLIM_BUILD_TAGS)'") || strings.Contains(line, "STATIC_EMBEDDED_BUILD_TAGS") {
				t.Fatalf("explicit static-slim recipe lost slim identity:\n%s", line)
			}
		default:
			t.Fatalf("unclassified fully-static recipe can bypass identity authority:\n%s", line)
		}
	}
	if embeddedRecipes != 1 || slimRecipes != 4 {
		t.Fatalf("static recipe census embedded=%d slim=%d want=1/4", embeddedRecipes, slimRecipes)
	}
	embeddedVerifier := "--artifact $(BINARY) --repo . --goos linux --goarch amd64 --cgo 1 --payload linux-amd64 --linux-runtime static --require-tags '$(STATIC_EMBEDDED_BUILD_TAGS)' --forbid-tags slim_streamer"
	slimVerifier := "--artifact $(STATIC_SLIM_OUT) --repo . --goos linux --goarch amd64 --cgo 1 --payload none --linux-runtime static --require-tags '$(STATIC_SLIM_BUILD_TAGS)' --forbid-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)'"
	for _, contract := range []string{embeddedVerifier, slimVerifier} {
		if strings.Count(contents, contract) != 1 {
			t.Fatalf("static artifact contract count for %q=%d want=1", contract, strings.Count(contents, contract))
		}
	}
	if strings.Contains(contents, "--artifact $(BINARY) --repo . --goos linux --goarch amd64 --cgo 1 --payload none") {
		t.Fatal("default static output still carries the old zero-payload contract")
	}
	for _, disclosure := range []string{"fully static Linux Codrax parent", "embedded Linux trace_streamer child", "static-slim", "external-only", "child still requires glibc", "reserved embedded/slim identity tags are rejected"} {
		if !strings.Contains(contents, disclosure) {
			t.Fatalf("Makefile help lacks packaging disclosure %q", disclosure)
		}
	}
}

func TestReleaseArtifactsPinPlatformAndStreamerContract(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(currentFile), "..", "Makefile"))
	if err != nil {
		t.Fatal(err)
	}
	contents := string(data)

	for _, declaration := range []string{
		"override BINARY := codrax",
		"override GO  := go",
		"override MAKE := make",
		"override SHELL := powershell.exe",
		"override .SHELLFLAGS := -NoProfile -Command",
		"override SHELL := /bin/sh",
		"override .SHELLFLAGS := -c",
		"override DIST_LINUX_AMD64             := dist/$(BINARY)-linux-amd64",
		"override DIST_LINUX_AMD64_STATIC_SLIM := dist/$(BINARY)-linux-amd64-static-slim",
		"override DIST_WINDOWS_AMD64           := dist/$(BINARY)-windows-amd64.exe",
		"override STATIC_SLIM_OUT              := $(BINARY)-static-slim",
		"override STATIC_EMBEDDED_BUILD_TAGS := $(STANDARD_EMBEDDED_BUILD_TAGS)",
		"override STATIC_SLIM_BUILD_TAGS := slim_streamer",
		"override STANDARD_EMBEDDED_BUILD_TAGS := codrax_embedded_streamer_release",
		"override STREAMER_ARTIFACT_VERIFIER := $(GO) run ./internal/releaseartifact/cmd/verify",
		"override STREAMER_COMMERCIAL_RELEASE_VERIFIER := $(GO) run ./internal/releaseartifact/cmd/verifycommercial",
		"override POSIX_GO_BUILD_ENV := GOENV=off GOFLAGS=",
		"override POSIX_GO_VERIFY_ENV := GOENV=off GOOS= GOARCH= CGO_ENABLED=0 GOFLAGS=",
		"$(error STATIC_EXTRA_TAGS must not contain reserved build identity tags $(STANDARD_EMBEDDED_BUILD_TAGS) or slim_streamer)",
		"$(error formal release targets do not allow a command-line MAKEFLAGS override)",
		"$(error formal release targets reject make -i/--ignore-errors)",
		"$(error formal release targets reject execution-suppressing make flags (-n/-t/-q))",
		"$(error MAKECMDGOALS is an automatic authority and must not be overridden)",
		"$(error OS is a host-detection authority and must not be overridden; use an explicit cross-* target)",
		"override FORMAL_RELEASE_GOALS := release release-strict release-clean-dist verify-trace-streamer-commercial-release",
		"override RELEASE_MAKE_OPTION_WORDS :=",
		"override RELEASE_MAKE_SHORT_FLAGS :=",
		"override FORMAL_RELEASE_PREFLIGHT_RESULT :=",
		"formal commercial trace_streamer preflight rejected before target scheduling",
		"override WINDOWS_GO_ENV := if (-not $$env:GOROOT)",
		"override WSL_REPO :=",
		"override HOST_OS := windows",
		"override HOST_KIND := windows",
		"override HOST_OS := unix",
		"override HOST_KIND := darwin",
		"override HOST_KIND := linux",
		"override WINDOWS_AMD64_GO_ENV := $(WINDOWS_GO_ENV) $$env:GOENV='off'; $$env:GOFLAGS=''; $$env:GOOS='windows'; $$env:GOARCH='amd64';",
		"override WINDOWS_VERIFY_GO_ENV := $(WINDOWS_GO_ENV) $$env:GOENV='off'; $$env:GOFLAGS=''; $$env:GOOS=''; $$env:GOARCH=''; $$env:CGO_ENABLED='0';",
		"GOOS   ?= $(shell GOENV=off GOOS= GOARCH= $(GO) env GOOS)",
		"GOARCH ?= $(shell GOENV=off GOOS= GOARCH= $(GO) env GOARCH)",
	} {
		if !strings.Contains(contents, declaration) {
			t.Fatalf("release artifact authority missing %q", declaration)
		}
	}
	if got := strings.Count(contents, "$(WINDOWS_AMD64_GO_ENV)"); got != 4 {
		t.Fatalf("Windows-host amd64 authority consumer count=%d want=4 (build + lowmem + cross + release)", got)
	}
	if got := strings.Count(contents, "$(WINDOWS_VERIFY_GO_ENV)"); got != 10 {
		t.Fatalf("Windows-host verifier authority consumer count=%d want=10 (artifact verifiers plus recipe/parse commercial gates)", got)
	}

	verifierRecipes := 0
	for _, line := range strings.Split(contents, "\n") {
		if !strings.Contains(line, "$(STREAMER_ARTIFACT_VERIFIER) --artifact") {
			continue
		}
		verifierRecipes++
		if !strings.Contains(line, "$(POSIX_GO_VERIFY_ENV)") && !strings.Contains(line, "$(WINDOWS_VERIFY_GO_ENV)") {
			t.Fatalf("artifact verifier can inherit target tuple or persistent GOENV state:\n%s", line)
		}
	}
	if verifierRecipes != 27 {
		t.Fatalf("artifact verifier recipe census=%d want=27; update the authority pin when adding a target", verifierRecipes)
	}

	standardLinuxRecipes := 0
	staticSlimRecipes := 0
	windowsAMD64Recipes := 0
	for _, line := range strings.Split(contents, "\n") {
		switch {
		case strings.Contains(line, "-o $(DIST_LINUX_AMD64) ."):
			standardLinuxRecipes++
			if !strings.Contains(line, "GOOS=linux GOARCH=amd64") || !strings.Contains(line, "-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)'") || strings.Contains(line, "STATIC_SLIM_BUILD_TAGS") || strings.Contains(line, "-static") {
				t.Fatalf("standard Linux release recipe is not an embedded default-tag target:\n%s", line)
			}
		case strings.Contains(line, "-o $(DIST_LINUX_AMD64_STATIC_SLIM) ."):
			staticSlimRecipes++
			if !strings.Contains(line, "GOOS=linux GOARCH=amd64") || !strings.Contains(line, "-tags '$(STATIC_SLIM_BUILD_TAGS)'") || !strings.Contains(line, "-static") {
				t.Fatalf("Linux static-slim recipe lost its explicit target/opt-out contract:\n%s", line)
			}
		}
		if strings.Contains(line, "--artifact $(DIST_LINUX_AMD64) ") && !strings.Contains(line, "--linux-runtime glibc") {
			t.Fatalf("standard Linux artifact verifier lacks typed glibc ABI contract:\n%s", line)
		}
		if strings.Contains(line, "$(STREAMER_ARTIFACT_VERIFIER) --artifact") && strings.Contains(line, "--require-tags '$(STATIC_SLIM_BUILD_TAGS)'") {
			if !strings.Contains(line, "--linux-runtime static") || !strings.Contains(line, "--forbid-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)'") {
				t.Fatalf("static-slim verifier lacks static ABI or dual-identity rejection:\n%s", line)
			}
		}
		if strings.Contains(line, "$(STREAMER_ARTIFACT_VERIFIER) --artifact $(BINARY)") {
			if !strings.Contains(line, "--payload linux-amd64") || !strings.Contains(line, "--linux-runtime static") || !strings.Contains(line, "--require-tags '$(STATIC_EMBEDDED_BUILD_TAGS)'") || !strings.Contains(line, "--forbid-tags slim_streamer") {
				t.Fatalf("default static verifier lacks independent parent/payload identity:\n%s", line)
			}
		}
		if strings.Contains(line, "-o $(DIST_WINDOWS_AMD64) .") {
			windowsAMD64Recipes++
			if (!strings.Contains(line, "GOOS=windows GOARCH=amd64") && !strings.Contains(line, "$(WINDOWS_AMD64_GO_ENV)")) || !strings.Contains(line, "-tags '$(STANDARD_EMBEDDED_BUILD_TAGS)'") {
				t.Fatalf("windows-amd64 filename is not bound to a matching target tuple:\n%s", line)
			}
		}
	}
	if standardLinuxRecipes != 5 || staticSlimRecipes != 3 {
		t.Fatalf("Linux artifact recipe census standard=%d static-slim=%d want=5/3", standardLinuxRecipes, staticSlimRecipes)
	}
	if windowsAMD64Recipes != 5 {
		t.Fatalf("windows-amd64 recipe census=%d want=5", windowsAMD64Recipes)
	}
	if got := strings.Count(contents, "release-strict:"); got != 3 {
		t.Fatalf("release-strict host implementation count=%d want=3", got)
	}
	if strings.Contains(contents, "release: clean-dist") {
		t.Fatal("formal release can clean/build before the commercial trace_streamer evidence gate")
	}
	if got := strings.Count(contents, "release: release-clean-dist"); got != 3 {
		t.Fatalf("release commercial-gate chain count=%d want=3", got)
	}
	if got := strings.Count(contents, "release-clean-dist: verify-trace-streamer-commercial-release"); got != 2 {
		t.Fatalf("host release-clean commercial-gate implementation count=%d want=2", got)
	}
	if got := strings.Count(contents, "release-strict: verify-trace-streamer-commercial-release"); got != 3 {
		t.Fatalf("release-strict commercial preflight count=%d want=3", got)
	}
	releaseParts := strings.SplitN(contents, "# Release\n", 2)
	if len(releaseParts) != 2 {
		t.Fatal("Makefile release section marker is missing")
	}
	if strings.Contains(releaseParts[0], "--commercial-release") {
		t.Fatal("development build/cross target was accidentally coupled to the commercial evidence gate")
	}
	formalArtifactVerifiers := strings.Count(releaseParts[1], "$(STREAMER_ARTIFACT_VERIFIER) --artifact")
	if formalArtifactVerifiers != 13 || strings.Count(releaseParts[1], "--commercial-release") != formalArtifactVerifiers {
		t.Fatalf("formal artifact commercial binding census verifiers=%d flags=%d want=13/13", formalArtifactVerifiers, strings.Count(releaseParts[1], "--commercial-release"))
	}
	for _, gate := range []string{
		"$(WINDOWS_VERIFY_GO_ENV) & $(STREAMER_COMMERCIAL_RELEASE_VERIFIER) --repo .",
		"$(POSIX_GO_VERIFY_ENV) $(STREAMER_COMMERCIAL_RELEASE_VERIFIER) --repo .",
		"fails loud while provenance says NOASSERTION/blocked",
		"if (Test-Path dist) { Remove-Item -Recurse -Force dist }",
	} {
		if !strings.Contains(contents, gate) {
			t.Fatalf("commercial release gate lacks %q", gate)
		}
	}
	for marker, want := range map[string]int{
		"--artifact $(DIST_LINUX_AMD64) --repo . --goos linux --goarch amd64 --cgo 1 --payload linux-amd64":       5,
		"--artifact $(DIST_LINUX_AMD64_STATIC_SLIM) --repo . --goos linux --goarch amd64 --cgo 1 --payload none":  3,
		"--artifact $(DIST_WINDOWS_AMD64) --repo . --goos windows --goarch amd64 --cgo 1 --payload windows-amd64": 5,
	} {
		if got := strings.Count(contents, marker); got != want {
			t.Fatalf("artifact verifier census for %q=%d want=%d", marker, got, want)
		}
	}
	for _, strictPin := range []string{
		"if ($$LASTEXITCODE -ne 0) { Write-Error 'release-strict requires",
		"& $(MAKE) release; if (-not $$?) { Write-Error 'release-strict recursive release failed.'",
	} {
		if !strings.Contains(contents, strictPin) {
			t.Fatalf("Windows release-strict lacks same-shell fail-loud pin %q", strictPin)
		}
	}
	for _, disclosure := range []string{
		"dist/$(BINARY)-linux-amd64 is the standard glibc/default-tag artifact",
		"make static keeps the Codrax parent fully static and embeds the independent Linux trace_streamer child",
		"make static-slim and dist/$(BINARY)-linux-amd64-static-slim are explicit fully-static external-only artifacts",
		"embedded child still requires glibc",
		"release-strict     Build and require every supported/core artifact for this host",
	} {
		if !strings.Contains(contents, disclosure) {
			t.Fatalf("Makefile help lacks exact release disclosure %q", disclosure)
		}
	}
}

func TestFormalReleaseMakeSemanticsCannotIgnoreCommercialGate(t *testing.T) {
	makePath, err := exec.LookPath("make")
	if err != nil {
		t.Skip("make is not installed")
	}
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	repo := filepath.Clean(filepath.Join(filepath.Dir(currentFile), ".."))
	tests := []struct {
		name string
		args []string
		env  []string
		want string
	}{
		{name: "ignore errors flag", args: []string{"-i", "-n", "release"}, want: "reject make -i/--ignore-errors"},
		{name: "hidden ignore errors flag", args: []string{"-i", "-n", "release", "MAKEFLAGS="}, want: "do not allow a command-line MAKEFLAGS override"},
		{name: "environment ignore errors", args: []string{"-n", "release"}, env: []string{"MAKEFLAGS=--ignore-errors"}, want: "reject make -i/--ignore-errors"},
		{name: "dry run is not a release", args: []string{"-n", "release"}, want: "reject execution-suppressing make flags"},
		{name: "touch mode is not a release", args: []string{"-t", "release"}, want: "reject execution-suppressing make flags"},
		{name: "old-file cannot skip preflight", args: []string{"-o", "verify-trace-streamer-commercial-release", "verify-trace-streamer-commercial-release"}, want: "formal commercial trace_streamer preflight rejected before target scheduling"},
		{name: "formal goal authority", args: []string{"-i", "-n", "release", "FORMAL_RELEASE_GOALS="}, want: "reject make -i/--ignore-errors"},
		{name: "formal helper authority", args: []string{"-i", "-n", "release", "RELEASE_MAKE_OPTION_WORDS=", "RELEASE_MAKE_SHORT_FLAGS="}, want: "reject make -i/--ignore-errors"},
		{name: "automatic goal authority", args: []string{"-i", "-n", "release", "MAKECMDGOALS="}, want: "MAKECMDGOALS is an automatic authority"},
		{name: "host selector authority", args: []string{"help", "OS=Windows_NT"}, want: "OS is a host-detection authority"},
		{name: "reserved embedded static identity", args: []string{"-n", "static", "STATIC_EXTRA_TAGS=codrax_embedded_streamer_release"}, want: "must not contain reserved build identity tags"},
		{name: "reserved slim static identity", args: []string{"-n", "static", "STATIC_EXTRA_TAGS=slim_streamer"}, want: "must not contain reserved build identity tags"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command(makePath, test.args...)
			command.Dir = repo
			command.Env = append(environmentWithoutKey(os.Environ(), "MAKEFLAGS"), test.env...)
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(output), test.want) {
				t.Fatalf("make %v error=%v output=%s, want fail-loud containing %q", test.args, err, output, test.want)
			}
		})
	}
}

func environmentWithoutKey(environment []string, blocked string) []string {
	prefix := strings.ToUpper(blocked) + "="
	out := make([]string, 0, len(environment))
	for _, item := range environment {
		if !strings.HasPrefix(strings.ToUpper(item), prefix) {
			out = append(out, item)
		}
	}
	return out
}

func TestTraceConvertWSLRuntimeAnchorFallbackIsPlatformAndSignalScoped(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	primary := filepath.Join(t.TempDir(), ".codrax")
	want := filepath.Join(home, ".codrax")
	for _, test := range []struct {
		name        string
		goos        string
		release     string
		environment bool
		want        string
	}{
		{name: "WSL kernel", goos: "linux", release: "5.15.167.4-microsoft-standard-WSL2", want: want},
		{name: "WSL environment", goos: "linux", release: "6.8.0-generic", environment: true, want: want},
		{name: "ordinary Linux", goos: "linux", release: "6.8.0-generic"},
		{name: "Windows host", goos: "windows", release: "microsoft-standard-WSL2"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := traceConvertWSLRuntimeAnchorFallbackFor(test.goos, test.release, test.environment, primary); got != test.want {
				t.Fatalf("fallback=%q want=%q", got, test.want)
			}
		})
	}
	if got := traceConvertWSLRuntimeAnchorFallbackFor("linux", "microsoft-standard-WSL2", true, want); got != "" {
		t.Fatalf("native home runtime anchor should not name itself as fallback: %q", got)
	}
}

func commandHelpText(t *testing.T, command *cobra.Command) string {
	t.Helper()
	var output bytes.Buffer
	command.SetOut(&output)
	t.Cleanup(func() { command.SetOut(nil) })
	if err := command.Help(); err != nil {
		t.Fatalf("render %s help: %v", command.CommandPath(), err)
	}
	return output.String()
}
