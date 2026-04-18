package ground

import "testing"

func TestCanonicalRepoRelative(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		repoRoot string
		want     string
	}{
		{
			name: "empty path returns empty",
			path: "", repoRoot: "/repo", want: "",
		},
		{
			name: "absolute inside repoRoot is stripped",
			path: "/mnt/d/repo/internal/x.go", repoRoot: "/mnt/d/repo", want: "internal/x.go",
		},
		{
			name: "absolute path equals repoRoot",
			path: "/mnt/d/repo", repoRoot: "/mnt/d/repo", want: ".",
		},
		{
			name: "absolute outside repoRoot returns cleaned original",
			path: "/etc/passwd", repoRoot: "/mnt/d/repo", want: "/etc/passwd",
		},
		{
			name: "empty repoRoot leaves path unchanged (cleaned)",
			path: "internal/./x.go", repoRoot: "", want: "internal/x.go",
		},
		{
			name: "relative path is cleaned",
			path: "./internal/../internal/x.go", repoRoot: "/repo", want: "internal/x.go",
		},
		{
			name: "repoRoot with trailing slash still matches",
			path: "/mnt/d/repo/README.md", repoRoot: "/mnt/d/repo/", want: "README.md",
		},
		{
			name: "absolute path that would escape via `..` returns cleaned absolute",
			path: "/mnt/e/other/x.go", repoRoot: "/mnt/d/repo", want: "/mnt/e/other/x.go",
		},
		{
			name: "README at repo top-level",
			path: "/mnt/d/temp/donghu/iSulad-master/README.md",
			repoRoot: "/mnt/d/temp/donghu/iSulad-master",
			want: "README.md",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CanonicalRepoRelative(tt.path, tt.repoRoot)
			if got != tt.want {
				t.Errorf("CanonicalRepoRelative(%q, %q) = %q, want %q",
					tt.path, tt.repoRoot, got, tt.want)
			}
		})
	}
}
