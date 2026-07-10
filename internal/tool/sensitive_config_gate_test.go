package tool

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// SEC #26 pins (复核收窄后语义). The gate is a HARD gate keyed on precise
// path signals:
//   - rule 1: registered active credential paths are denied wherever they
//     live and whatever they are named;
//   - rule 2: the providers credential basename pattern (providers.yaml /
//     providers.*.yaml) is denied ONLY inside a registered anchor
//     directory (the credential store's parent dir = exe/config anchor);
//   - an EXTERNAL analyzed repository's same-named file stays fully
//     readable/searchable and carries a soft advisory (user-intent red
//     line: no hard gate on a legitimate user file);
//   - the runtime settings file (codrax.yaml) is never hard-denied;
//   - refusal wording is generic with zero path echo.

func withRegisteredSensitiveConfigPaths(t *testing.T, paths ...string) {
	t.Helper()
	SetSensitiveConfigFilePaths(paths)
	t.Cleanup(func() { SetSensitiveConfigFilePaths(nil) })
}

func writeSensitiveGateFixture(t *testing.T, dir, rel, content string) string {
	t.Helper()
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
	return path
}

// newSensitiveAnchorDomain builds a simulated exe/config anchor dir holding
// the registered credential store plus unregistered fallback-slot siblings,
// and registers the store with the shared authority.
func newSensitiveAnchorDomain(t *testing.T) (anchorDir, registered string) {
	t.Helper()
	anchorDir = t.TempDir()
	registered = writeSensitiveGateFixture(t, anchorDir, "providers.yaml", "api_key: fake-anchor-key\n")
	writeSensitiveGateFixture(t, anchorDir, "providers.deepseek.yaml", "api_key: fake-fallback-key\n")
	writeSensitiveGateFixture(t, anchorDir, "codrax.yaml", "verify_mem_limit_mb: 2048\n")
	withRegisteredSensitiveConfigPaths(t, registered)
	return anchorDir, registered
}

func TestIsSensitiveConfigFilePath_Predicate(t *testing.T) {
	anchorDir, registered := newSensitiveAnchorDomain(t)
	externalRepo := t.TempDir()
	writeSensitiveGateFixture(t, externalRepo, "providers.yaml", "grafana: provisioning\n")

	cases := []struct {
		name string
		path string
		want bool
	}{
		{"rule 1: registered credential store", registered, true},
		{"rule 2: fallback-slot sibling in anchor dir", filepath.Join(anchorDir, "providers.deepseek.yaml"), true},
		{"rule 2: uppercase spelling in anchor dir", filepath.Join(anchorDir, "PROVIDERS.YAML"), true},
		{"settings file is NOT a credential (codrax.yaml readable)", filepath.Join(anchorDir, "codrax.yaml"), false},
		{"external repo same-named file stays allowed", filepath.Join(externalRepo, "providers.yaml"), false},
		{"external repo fallback-slot spelling stays allowed", filepath.Join(externalRepo, "providers.deepseek.yaml"), false},
		{"testdata fixture under anchor stays allowed (different parent dir)", filepath.Join(anchorDir, "testdata", "providers.yaml"), false},
		{"unrelated yaml in anchor dir", filepath.Join(anchorDir, "config.yaml"), false},
		{"providers prefix without dot pattern", filepath.Join(anchorDir, "providers_test.go"), false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		if got := IsSensitiveConfigFilePath(tc.path); got != tc.want {
			t.Errorf("%s: IsSensitiveConfigFilePath(%q) = %v, want %v", tc.name, tc.path, got, tc.want)
		}
	}
}

func TestIsSensitiveConfigFilePath_SymlinkCannotBypassRegisteredPath(t *testing.T) {
	_, registered := newSensitiveAnchorDomain(t)
	link := filepath.Join(t.TempDir(), "innocent_link.txt")
	if err := os.Symlink(registered, link); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	if !IsSensitiveConfigFilePath(link) {
		t.Fatalf("symlink to registered active config must be denied")
	}
}

// 捎带④: darwin (and windows) default to case-insensitive filesystems — a
// case-variant spelling of the registered path must not bypass rule 1.
func TestIsSensitiveConfigFilePath_CaseFoldOnDarwin(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("case-insensitive-filesystem pin")
	}
	anchorDir := t.TempDir()
	registered := writeSensitiveGateFixture(t, anchorDir, "llm_credentials.custom", "api_key: fake\n")
	withRegisteredSensitiveConfigPaths(t, registered)
	variant := filepath.Join(anchorDir, "LLM_Credentials.CUSTOM")
	if !IsSensitiveConfigFilePath(variant) {
		t.Fatalf("case-variant spelling of the registered path must still be denied on %s", runtime.GOOS)
	}
}

func assertSensitiveRefusal(t *testing.T, result types.ToolResult, mustNotContain ...string) {
	t.Helper()
	if result.Success {
		t.Fatalf("expected refusal, got success: %s", result.Summary)
	}
	if !strings.Contains(result.Summary, "configuration credentials file") {
		t.Fatalf("refusal must carry the generic caveat, got: %s", result.Summary)
	}
	if result.Repair == nil || result.Repair.Code != "sensitive_config_file_denied" {
		t.Fatalf("refusal must carry the typed repair code, got: %+v", result.Repair)
	}
	for _, needle := range mustNotContain {
		if needle == "" {
			continue
		}
		if strings.Contains(result.Summary, needle) {
			t.Fatalf("refusal must not echo %q (zero path echo), got: %s", needle, result.Summary)
		}
	}
}

func TestSensitiveConfigGate_AnchorDomainDenied(t *testing.T) {
	anchorDir, _ := newSensitiveAnchorDomain(t)
	// Self-analysis posture: the analyzed repo IS the anchor dir.
	bus := &types.BusContext{RepoRoot: anchorDir}

	for _, target := range []string{"providers.yaml", "providers.deepseek.yaml"} {
		params, _ := json.Marshal(map[string]string{"path": target})
		result, err := (&ReadFile{}).Execute(bus, params)
		if err != nil {
			t.Fatalf("read_file(%s): %v", target, err)
		}
		assertSensitiveRefusal(t, result, anchorDir, "fake-anchor-key", "fake-fallback-key")

		gparams, _ := json.Marshal(grepToolParams{Pattern: "api_key", Path: target})
		gresult, err := (&GrepTool{}).Execute(bus, gparams)
		if err != nil {
			t.Fatalf("grep(%s): %v", target, err)
		}
		assertSensitiveRefusal(t, gresult, anchorDir, "fake-anchor-key", "fake-fallback-key")
	}
}

// 复核必修① pin: the runtime settings file is never hard-denied.
func TestSensitiveConfigGate_SettingsFileStaysReadable(t *testing.T) {
	anchorDir, _ := newSensitiveAnchorDomain(t)
	bus := &types.BusContext{RepoRoot: anchorDir}
	params, _ := json.Marshal(map[string]string{"path": "codrax.yaml"})
	result, err := (&ReadFile{}).Execute(bus, params)
	if err != nil {
		t.Fatalf("read_file: %v", err)
	}
	if !result.Success {
		t.Fatalf("codrax.yaml (runtime knobs, not credentials) must stay readable, got: %s", result.Summary)
	}
	if !strings.Contains(result.Summary, "verify_mem_limit_mb") {
		t.Fatalf("settings content missing from read result: %s", result.Summary)
	}
}

// 复核必修① pin: an EXTERNAL repository's same-named file is fully readable
// and greppable, with the soft advisory present (never a hard gate).
func TestSensitiveConfigGate_ExternalRepoSameNameAllowedWithAdvisory(t *testing.T) {
	newSensitiveAnchorDomain(t) // registered anchor exists elsewhere
	repo := t.TempDir()
	writeSensitiveGateFixture(t, repo, "providers.yaml", "grafana_dashboards: providers-fixture-value\n")
	bus := &types.BusContext{RepoRoot: repo}

	params, _ := json.Marshal(map[string]string{"path": "providers.yaml"})
	result, err := (&ReadFile{}).Execute(bus, params)
	if err != nil {
		t.Fatalf("read_file: %v", err)
	}
	if !result.Success {
		t.Fatalf("external repo providers.yaml must stay readable, got: %s", result.Summary)
	}
	if !strings.Contains(result.Summary, "providers-fixture-value") {
		t.Fatalf("external file content missing: %s", result.Summary)
	}
	if !strings.Contains(result.Summary, "[advisory]") {
		t.Fatalf("soft advisory missing from read result: %s", result.Summary)
	}

	gparams, _ := json.Marshal(grepToolParams{Pattern: "grafana_dashboards", Path: "providers.yaml"})
	gresult, err := (&GrepTool{}).Execute(bus, gparams)
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if !gresult.Success {
		t.Fatalf("external repo providers.yaml must stay greppable, got: %s", gresult.Summary)
	}
	if !strings.Contains(gresult.Summary, "providers-fixture-value") {
		t.Fatalf("grep must return the matching line, got: %s", gresult.Summary)
	}
	if !strings.Contains(gresult.Summary, "[advisory]") {
		t.Fatalf("soft advisory missing from grep result: %s", gresult.Summary)
	}
}

// 复核必修① pin: broad scans of an EXTERNAL repo include its same-named
// file (no silent removal).
func TestSensitiveConfigGate_BroadGrepIncludesExternalRepoSameName(t *testing.T) {
	newSensitiveAnchorDomain(t)
	repo := t.TempDir()
	sentinel := "EXTERNAL_SENTINEL_XYZZY"
	writeSensitiveGateFixture(t, repo, "providers.yaml", "value: "+sentinel+"-providers\n")
	writeSensitiveGateFixture(t, repo, "source.go", "// "+sentinel+"-source\n")
	bus := &types.BusContext{RepoRoot: repo}

	params, _ := json.Marshal(grepToolParams{Pattern: sentinel, Path: "."})
	result, err := (&GrepTool{}).Execute(bus, params)
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if !result.Success {
		t.Fatalf("broad grep must succeed, got: %s", result.Summary)
	}
	for _, want := range []string{sentinel + "-providers", sentinel + "-source"} {
		if !strings.Contains(result.Summary, want) {
			t.Fatalf("broad grep in an external repo lost %q (silent removal regression):\n%s", want, result.Summary)
		}
	}
}

// Self-analysis posture: broad scans must NOT surface anchor-domain
// credential content (registered store + fallback-slot siblings).
func TestSensitiveConfigGate_BroadGrepExcludesAnchorDomainCredentials(t *testing.T) {
	anchorDir, _ := newSensitiveAnchorDomain(t)
	sentinel := "ANCHOR_SENTINEL_XYZZY"
	writeSensitiveGateFixture(t, anchorDir, "providers.yaml", "api_key: "+sentinel+"-providers\n")
	writeSensitiveGateFixture(t, anchorDir, "providers.deepseek.yaml", "api_key: "+sentinel+"-fallback\n")
	writeSensitiveGateFixture(t, anchorDir, "source.go", "// "+sentinel+"-source\n")
	bus := &types.BusContext{RepoRoot: anchorDir}

	params, _ := json.Marshal(grepToolParams{Pattern: sentinel, Path: "."})
	result, err := (&GrepTool{}).Execute(bus, params)
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if !result.Success {
		t.Fatalf("broad grep must succeed, got: %s", result.Summary)
	}
	if !strings.Contains(result.Summary, sentinel+"-source") {
		t.Fatalf("broad grep lost the legitimate source match: %s", result.Summary)
	}
	for _, leaked := range []string{sentinel + "-providers", sentinel + "-fallback"} {
		if strings.Contains(result.Summary, leaked) {
			t.Fatalf("broad grep leaked anchor-domain credential content %q:\n%s", leaked, result.Summary)
		}
	}
}

// 捎带③ pin: a registered credential store with a GENERIC basename must not
// cause broad-scan exclusion of an external repo's same-named file.
func TestSensitiveConfigGate_GenericNameRegistrationDoesNotOverExclude(t *testing.T) {
	otherDir := t.TempDir()
	registered := writeSensitiveGateFixture(t, otherDir, "llm.yaml", "api_key: fake\n")
	withRegisteredSensitiveConfigPaths(t, registered)

	repo := t.TempDir()
	sentinel := "GENERIC_NAME_SENTINEL"
	writeSensitiveGateFixture(t, repo, "llm.yaml", "note: "+sentinel+"-repo-llm\n")
	bus := &types.BusContext{RepoRoot: repo}

	params, _ := json.Marshal(grepToolParams{Pattern: sentinel, Path: "."})
	result, err := (&GrepTool{}).Execute(bus, params)
	if err != nil {
		t.Fatalf("grep: %v", err)
	}
	if !result.Success || !strings.Contains(result.Summary, sentinel+"-repo-llm") {
		t.Fatalf("external repo llm.yaml must stay searchable despite a generic-named registered store elsewhere:\n%s", result.Summary)
	}
}

// Backend-independent pin of the scan-exclude computation itself (the GNU
// grep fallback consumes the basename list, so the under-scan-root gate must
// hold in the pure function, not just in the rg lane):
//   - scanning an EXTERNAL tree with the credential store elsewhere yields
//     ZERO excludes (捎带③: no over-exclusion of same-named user files);
//   - scanning the anchor tree yields root-anchored rg globs + basenames
//     for the store and its credential-named siblings.
func TestSensitiveConfigScanExcludes_UnderRootGate(t *testing.T) {
	anchorDir, _ := newSensitiveAnchorDomain(t)
	externalRepo := t.TempDir()
	writeSensitiveGateFixture(t, externalRepo, "providers.yaml", "grafana: x\n")

	rgGlobs, grepExcludes := sensitiveConfigScanExcludes(externalRepo)
	if len(rgGlobs) != 0 || len(grepExcludes) != 0 {
		t.Fatalf("external scan root must yield zero excludes, got rg=%v grep=%v", rgGlobs, grepExcludes)
	}

	rgGlobs, grepExcludes = sensitiveConfigScanExcludes(anchorDir)
	joined := strings.Join(rgGlobs, " ")
	for _, want := range []string{"!/providers.yaml", "!/providers.deepseek.yaml"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("anchor scan root missing rg glob %q, got %v", want, rgGlobs)
		}
	}
	joinedExcl := strings.Join(grepExcludes, " ")
	for _, want := range []string{"providers.yaml", "providers.deepseek.yaml"} {
		if !strings.Contains(joinedExcl, want) {
			t.Fatalf("anchor scan root missing grep exclude %q, got %v", want, grepExcludes)
		}
	}
	for _, glob := range rgGlobs {
		if strings.Contains(glob, "..") {
			t.Fatalf("rg glob escaped the scan root: %q", glob)
		}
	}
	// codrax.yaml is runtime knobs, never excluded.
	if strings.Contains(joined, "codrax.yaml") || strings.Contains(joinedExcl, "codrax.yaml") {
		t.Fatalf("settings file must not be scan-excluded: rg=%v grep=%v", rgGlobs, grepExcludes)
	}
}

func TestSensitiveConfigGate_TestdataFixtureExplicitTargetsAllowed(t *testing.T) {
	anchorDir, _ := newSensitiveAnchorDomain(t)
	writeSensitiveGateFixture(t, anchorDir, "testdata/providers.yaml", "api_key: fixture-value\n")
	bus := &types.BusContext{RepoRoot: anchorDir}

	params, _ := json.Marshal(map[string]string{"path": "testdata/providers.yaml"})
	result, err := (&ReadFile{}).Execute(bus, params)
	if err != nil {
		t.Fatalf("read_file: %v", err)
	}
	if !result.Success || !strings.Contains(result.Summary, "fixture-value") {
		t.Fatalf("testdata fixture must stay readable, got: %s", result.Summary)
	}

	gparams, _ := json.Marshal(grepToolParams{Pattern: "api_key", Path: "testdata/providers.yaml"})
	gresult, err := (&GrepTool{}).Execute(bus, gparams)
	if err != nil {
		t.Fatalf("grep fixture: %v", err)
	}
	if !gresult.Success || !strings.Contains(gresult.Summary, "fixture-value") {
		t.Fatalf("explicit testdata fixture grep must succeed, got: %s", gresult.Summary)
	}
}

func TestSensitiveConfigGate_ExecCommandArm(t *testing.T) {
	anchorDir, _ := newSensitiveAnchorDomain(t)
	writeSensitiveGateFixture(t, anchorDir, "notes.txt", "plain notes\n")
	writeSensitiveGateFixture(t, anchorDir, "testdata/providers.yaml", "api_key: exec-fixture-value\n")
	bus := &types.BusContext{RepoRoot: anchorDir}

	denied := []string{
		"cat providers.yaml",
		"grep api_key providers.yaml",
		"cat provi*.yaml",
		"sed -n 1p ./providers.deepseek.yaml",
	}
	for _, cmd := range denied {
		params, _ := json.Marshal(map[string]string{"command": cmd})
		result, err := (&ExecCommand{}).Execute(bus, params)
		if err != nil {
			t.Fatalf("exec_command(%q): %v", cmd, err)
		}
		assertSensitiveRefusal(t, result, anchorDir, "fake-anchor-key", "fake-fallback-key")
	}

	allowed := []string{
		"cat notes.txt",
		"cat testdata/providers.yaml", // fixture outside the anchor parent dir
	}
	for _, cmd := range allowed {
		params, _ := json.Marshal(map[string]string{"command": cmd})
		result, err := (&ExecCommand{}).Execute(bus, params)
		if err != nil {
			t.Fatalf("exec_command(%q): %v", cmd, err)
		}
		if !result.Success {
			t.Fatalf("exec_command(%q) must stay allowed, got: %s", cmd, result.Summary)
		}
	}
}

func TestSensitiveConfigGate_NativeGrepShouldSkipLane(t *testing.T) {
	anchorDir, _ := newSensitiveAnchorDomain(t)
	sentinel := "NATIVE_SENTINEL_PLUGH"
	writeSensitiveGateFixture(t, anchorDir, "providers.yaml", "api_key: "+sentinel+"-providers\n")
	writeSensitiveGateFixture(t, anchorDir, "source.go", "// "+sentinel+"-source marker\n")
	bus := &types.BusContext{RepoRoot: anchorDir}
	filter := NewSearchDirFilter(anchorDir, anchorDir)

	res, err := NativeGrep(context.Background(), NativeGrepOpts{
		Pattern:    sentinel,
		Root:       anchorDir,
		ShouldSkip: grepNativeShouldSkip(bus, filter),
	})
	if err != nil {
		t.Fatalf("NativeGrep: %v", err)
	}
	if !strings.Contains(res.Output, sentinel+"-source") {
		t.Fatalf("native lane lost the legitimate source match: %s", res.Output)
	}
	if strings.Contains(res.Output, sentinel+"-providers") {
		t.Fatalf("native lane leaked anchor-domain providers.yaml content: %s", res.Output)
	}
}

// SEC #26 必修② pin (citation lane): a forged citation naming the live
// credential store must NOT be materialized by the deterministic quote
// backfill; a normal citation still repairs.
func TestSensitiveConfigGate_CitationBackfillRefusesCredentialFile(t *testing.T) {
	anchorDir, _ := newSensitiveAnchorDomain(t)
	writeSensitiveGateFixture(t, anchorDir, "small.go", "package x\nfunc Real() {}\n")
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{ID: "s1", Kind: types.BlockSummary, Text: "x"}},
		Citations: []types.Citation{
			{File: "providers.yaml", Line: 1, Quote: "model-invented quote"},
			{File: "small.go", Line: 2, Quote: "stale quote"},
		},
	}
	ctx := &types.BusContext{RepoRoot: anchorDir, Mutable: types.NewMutableState("q")}
	if fixed := normalizeCurrentSourceCitationQuotes(doc, ctx); fixed != 1 {
		t.Fatalf("fixed=%d, want 1 (only small.go)", fixed)
	}
	if doc.Citations[0].Quote != "model-invented quote" {
		t.Fatalf("credential-file citation must keep the model quote (never materialize real content), got: %q", doc.Citations[0].Quote)
	}
	if strings.Contains(doc.Citations[0].Quote, "fake-anchor-key") {
		t.Fatalf("credential content materialized into citation quote")
	}
	if doc.Citations[1].Quote != "func Real() {}" {
		t.Fatalf("normal citation must still repair, got: %q", doc.Citations[1].Quote)
	}
}
