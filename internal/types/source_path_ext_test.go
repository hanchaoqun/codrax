package types

import "testing"

func TestIsCodeOrConfigPathExtension(t *testing.T) {
	for _, ext := range []string{
		".go", ".c", ".cc", ".cpp", ".cxx", ".h", ".hh", ".hpp", ".hxx",
		".cj", ".cjo", ".ets", ".ts", ".tsx", ".js", ".jsx", ".mjs",
		".java", ".kt", ".kts", ".py", ".pyi", ".rs", ".rb", ".php", ".swift",
		".lua", ".proto", ".m", ".mm", ".cu", ".cuh",
		".yaml", ".yml", ".json", ".toml", ".ini", ".xml", ".md",
	} {
		if !IsCodeOrConfigPathExtension(ext) {
			t.Errorf("IsCodeOrConfigPathExtension(%q) = false; want true", ext)
		}
	}
	for _, ext := range []string{
		"", ".unknown", ".txt", ".log", "go", "no-dot",
	} {
		if IsCodeOrConfigPathExtension(ext) {
			t.Errorf("IsCodeOrConfigPathExtension(%q) = true; want false", ext)
		}
	}
	if !IsCodeOrConfigPathExtension(".GO") || !IsCodeOrConfigPathExtension(".YAML") {
		t.Error("IsCodeOrConfigPathExtension must be case-insensitive")
	}
}

func TestHasCodeOrConfigPathSuffix(t *testing.T) {
	hits := map[string]bool{
		"foo.go":             true,
		"a/b/foo.go":         true,
		"bar.YAML":           true,
		"weird_FILE.PYI":     true,
		"some-name.proto":    true,
		"package_clause.cjo": true,
	}
	for s, want := range hits {
		if got := HasCodeOrConfigPathSuffix(s); got != want {
			t.Errorf("HasCodeOrConfigPathSuffix(%q) = %v; want %v", s, got, want)
		}
	}
	misses := []string{
		"", "bare", "foo.unknown", "foo.txt", "no_extension_dot_here",
	}
	for _, s := range misses {
		if HasCodeOrConfigPathSuffix(s) {
			t.Errorf("HasCodeOrConfigPathSuffix(%q) = true; want false", s)
		}
	}
}

func TestLooksLikeRuntimeArtifactPath(t *testing.T) {
	for _, s := range []string{
		"record_trace.systrace",
		"/tmp/app.log",
		"trace.HTRACE",
		"capture.atrace",
		"capture.ftrace",
		"sample.perfetto",
		"sample.perftrace",
		"sample.tracebundle.json",
		"perf.data",
		"sample.perf.data",
		"attached_trace.txt",
		"/tmp/.codrax/blob/session/attached_trace.txt",
		"/tmp/.codrax/blob/session/attached_log.txt",
	} {
		if !LooksLikeRuntimeArtifactPath(s) {
			t.Errorf("LooksLikeRuntimeArtifactPath(%q) = false; want true", s)
		}
	}
	for _, s := range []string{"", ".log", ".trace", ".systrace", "main.go", "config.yaml", "README.md", "trace.txt"} {
		if LooksLikeRuntimeArtifactPath(s) {
			t.Errorf("LooksLikeRuntimeArtifactPath(%q) = true; want false", s)
		}
	}
}
