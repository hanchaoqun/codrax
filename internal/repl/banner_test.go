package repl

import (
	"strings"
	"testing"
)

func TestRenderDegradedEnvHints_HealthyBox(t *testing.T) {
	if got := renderDegradedEnvHints(false, false, false); len(got) != 0 {
		t.Fatalf("healthy box should produce no hints, got %v", got)
	}
	if got := renderDegradedEnvHints(true, false, false); len(got) != 0 {
		t.Fatalf("healthy box (zh) should produce no hints, got %v", got)
	}
}

func TestRenderDegradedEnvHints_Bilingual(t *testing.T) {
	cases := []struct {
		name         string
		zh           bool
		nativeGrep   bool
		gitMissing   bool
		wantCount    int
		wantContains []string
	}{
		{"native en", false, true, false, 1, []string{"native Go scanner", "install ripgrep"}},
		{"native zh", true, true, false, 1, []string{"Go 内置扫描器", "装 ripgrep"}},
		{"git en", false, false, true, 1, []string{"git not detected", "filesystem walk"}},
		{"git zh", true, false, true, 1, []string{"未检测到 git", "repomap 走文件遍历"}},
		{"both en", false, true, true, 2, []string{"native Go scanner", "git not detected"}},
		{"both zh", true, true, true, 2, []string{"Go 内置扫描器", "未检测到 git"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := renderDegradedEnvHints(tc.zh, tc.nativeGrep, tc.gitMissing)
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
	got := renderDegradedEnvHints(false, true, true)
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

func TestPreferZh_TolerantMatching(t *testing.T) {
	cases := map[string]bool{
		"":          false,
		"en":        false,
		"english":   false,
		"zh":        true,
		"ZH":        true,
		"zh-CN":     true,
		" zh-cn ":   true,
		"cn":        true,
		"chinese":   true,
		"简体中文":    true,
		"fr":        false,
		"Japanese":  false,
	}
	for lang, want := range cases {
		r := &REPL{language: lang}
		if got := r.preferZh(); got != want {
			t.Errorf("preferZh(%q): want %v, got %v", lang, want, got)
		}
	}
}
