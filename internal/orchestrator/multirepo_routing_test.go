package orchestrator

import (
	"sort"
	"testing"
)

func TestInferQueryLanguages(t *testing.T) {
	cases := []struct {
		req  string
		want []string
	}{
		{"fix foo.go panic", []string{"go"}},
		{"why does main.rs leak memory?", []string{"rust"}},
		{"src/lib.cj 编译失败", []string{"cangjie"}},
		{"sub-a/foo.go imports sub-b/bar.rs", []string{"go", "rust"}},
		{"plain English question without extensions", nil},
		{"", nil},
		{"check pages/Index.ets layout", []string{"arkts"}},
		{"build.gradle works but Main.kt doesn't", []string{"kotlin"}},
	}
	for _, c := range cases {
		got := inferQueryLanguages(c.req)
		sort.Strings(got)
		want := append([]string(nil), c.want...)
		sort.Strings(want)
		if len(got) != len(want) {
			t.Errorf("inferQueryLanguages(%q) = %v, want %v", c.req, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("inferQueryLanguages(%q) = %v, want %v", c.req, got, want)
				break
			}
		}
	}
}
