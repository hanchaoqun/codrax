package cmd

import (
	"bytes"
	"os"
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
		"--cache-dir",
		"codrax trace convert --input capture.sys",
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
	if resolved.cacheRoot != wantYAMLCache || resolved.lang != "en" || flagLang != "en" {
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

func TestStaticBuildsDoNotClaimGlibcTraceStreamerPayload(t *testing.T) {
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
		"STATIC_BUILD_TAGS := slim_streamer$(if $(strip $(STATIC_EXTRA_TAGS)),$(comma)$(strip $(STATIC_EXTRA_TAGS)))",
		"$(error STATIC_TAGS is unsafe and no longer supported",
	} {
		if !strings.Contains(contents, hardening) {
			t.Fatalf("Makefile lacks immutable slim_streamer hardening %q", hardening)
		}
	}
	if strings.Contains(contents, "STATIC_TAGS   ?=") {
		t.Fatal("legacy STATIC_TAGS remains user-overridable and can remove slim_streamer")
	}
	staticRecipes := 0
	for _, line := range strings.Split(contents, "\n") {
		if !strings.Contains(line, "extldflags") || !strings.Contains(line, "-static") {
			continue
		}
		staticRecipes++
		if !strings.Contains(line, "-tags '$(STATIC_BUILD_TAGS)'") {
			t.Fatalf("musl-static recipe can embed the glibc trace_streamer payload:\n%s", line)
		}
	}
	if staticRecipes != 4 {
		t.Fatalf("static recipe census=%d, want 4; update the packaging pin when adding a target", staticRecipes)
	}
	for _, disclosure := range []string{"embed trace_streamer by default", "Linux/musl", "requires glibc", "external trace_streamer", "STATIC_EXTRA_TAGS", "slim_streamer cannot be removed"} {
		if !strings.Contains(contents, disclosure) {
			t.Fatalf("Makefile help lacks packaging disclosure %q", disclosure)
		}
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
