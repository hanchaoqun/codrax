package repl

import (
	"strings"
	"testing"
)

func TestRenderDegradedEnvHints_HealthyBox(t *testing.T) {
	if got := renderDegradedEnvHints(false, "rg", false); len(got) != 0 {
		t.Fatalf("healthy box should produce no hints, got %v", got)
	}
	if got := renderDegradedEnvHints(true, "rg", false); len(got) != 0 {
		t.Fatalf("healthy box (zh) should produce no hints, got %v", got)
	}
}

func TestRenderDegradedEnvHints_Bilingual(t *testing.T) {
	cases := []struct {
		name          string
		zh            bool
		searchBackend string
		gitMissing    bool
		wantCount     int
		wantContains  []string
	}{
		// rg missing, grep available — slow-ish nudge, not catastrophic.
		{"grep en", false, "grep", false, 1, []string{"Search backend: grep", "faster scans"}},
		{"grep zh", true, "grep", false, 1, []string{"搜索后端:grep", "进一步提速"}},

		// Both missing — native Go walker.
		{"native en", false, "native", false, 1, []string{"native Go scanner", "10×"}},
		{"native zh", true, "native", false, 1, []string{"Go 内置扫描器", "大幅提速"}},

		// Git-only degradation.
		{"git en", false, "rg", true, 1, []string{"git not detected", "filesystem walk"}},
		{"git zh", true, "rg", true, 1, []string{"未检测到 git", "repomap 走文件遍历"}},

		// Combined degradations.
		{"grep+git en", false, "grep", true, 2, []string{"Search backend: grep", "git not detected"}},
		{"native+git zh", true, "native", true, 2, []string{"Go 内置扫描器", "未检测到 git"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderDegradedEnvHints(tc.zh, tc.searchBackend, tc.gitMissing)
			if len(got) != tc.wantCount {
				t.Fatalf("want %d hints, got %d: %v", tc.wantCount, len(got), got)
			}
			joined := strings.Join(got, "\n")
			for _, frag := range tc.wantContains {
				if !strings.Contains(joined, frag) {
					t.Errorf("missing %q in %q", frag, joined)
				}
			}
		})
	}
}

func TestRenderDegradedEnvHints_OrderIsStable(t *testing.T) {
	// Both degradations present — search backend line must come first
	// so the most user-visible (every search slower) lands on the
	// top banner line.
	got := renderDegradedEnvHints(false, "native", true)
	if len(got) != 2 {
		t.Fatalf("want 2, got %d", len(got))
	}
	if !strings.Contains(got[0], "Search backend") {
		t.Fatalf("line 0 should be search backend, got %q", got[0])
	}
	if !strings.Contains(got[1], "git") {
		t.Fatalf("line 1 should be git, got %q", got[1])
	}
}

func TestRenderDegradedEnvHints_UnknownBackendNoPanic(t *testing.T) {
	// Future-proof: if SearchCommand ever grows a fourth value, the
	// renderer should just skip the search line rather than crash.
	if got := renderDegradedEnvHints(false, "wat", false); len(got) != 0 {
		t.Fatalf("unknown backend should produce no search hint, got %v", got)
	}
}

// TestDegradedEnvHints_LanguageGate pins the "only en flips to
// English, every other value — including empty — stays zh" contract.
// codrax.yaml's default is zh; the banner follows it so users who
// never set --lang see Chinese text.
func TestDegradedEnvHints_LanguageGate(t *testing.T) {
	cases := map[string]bool{ // lang input → expected zh
		"":   true, // empty → codrax.yaml default (zh)
		"en": false,
		"EN": false,
		"zh": true,
		"fr": true, // unknown → zh fallback
	}
	for lang, wantZh := range cases {
		zh := !strings.EqualFold(strings.TrimSpace(lang), "en")
		hints := renderDegradedEnvHints(zh, "grep", false)
		if len(hints) != 1 {
			t.Fatalf("lang=%q: expected 1 hint, got %d", lang, len(hints))
		}
		isZh := strings.Contains(hints[0], "搜索后端")
		if isZh != wantZh {
			t.Errorf("lang=%q: wantZh=%v, got %q", lang, wantZh, hints[0])
		}
	}
}

func TestMultiRepoBannerLine_DisabledPostureIsSingleRepoPrecise(t *testing.T) {
	got := multiRepoBannerLine("zh", false, 3, 0, 2)
	for _, want := range []string{"单仓模式", "已关闭", "3 个子仓", "/repos"} {
		if !strings.Contains(got, want) {
			t.Fatalf("disabled zh banner missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "active cap") || strings.Contains(got, "focus") {
		t.Fatalf("disabled banner must not imply active multi-repo routing: %q", got)
	}

	got = multiRepoBannerLine("en", false, 3, 0, 2)
	for _, want := range []string{"single-repo mode", "routing disabled", "3 sub-repos", "/repos"} {
		if !strings.Contains(got, want) {
			t.Fatalf("disabled en banner missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "active cap") || strings.Contains(got, "focus") {
		t.Fatalf("disabled banner must not imply active multi-repo routing: %q", got)
	}
}

func TestMultiRepoBannerLine_EnabledPostureShowsRoutingControls(t *testing.T) {
	got := multiRepoBannerLine("en", true, 3, 1, 2)
	for _, want := range []string{"multi-repo", "3 sub-repos", "active cap=2", "focus-pinned"} {
		if !strings.Contains(got, want) {
			t.Fatalf("enabled banner missing %q: %q", want, got)
		}
	}
}
