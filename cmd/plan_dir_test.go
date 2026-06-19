package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestResolveConfiguredPlanDir(t *testing.T) {
	anchor := filepath.FromSlash("/anchor")
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"   ", ""},
		{filepath.FromSlash("/abs/plans"), filepath.FromSlash("/abs/plans")},
		{"relplans", filepath.Join(anchor, "relplans")},
		{filepath.FromSlash("sub/plans"), filepath.Join(anchor, "sub", "plans")},
	}
	for _, tc := range cases {
		if got := resolveConfiguredPlanDir(tc.in, anchor); got != tc.want {
			t.Errorf("resolveConfiguredPlanDir(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolvePlanDirRootHonorsOverride(t *testing.T) {
	saved := app.planDir
	defer func() { app.planDir = saved }()

	anchor := filepath.FromSlash("/anchor")
	app.planDir = ""
	if got, want := resolvePlanDirRoot(anchor), filepath.Join(anchor, "plans"); got != want {
		t.Errorf("default plan dir = %q, want %q", got, want)
	}

	app.planDir = filepath.FromSlash("/custom/plans")
	if got := resolvePlanDirRoot(anchor); got != filepath.FromSlash("/custom/plans") {
		t.Errorf("override plan dir = %q, want /custom/plans", got)
	}
}

// Bug #6 regression: write_plan_dir was a dead knob — writePlanFile always
// landed in <runtime-anchor>/plans. With the override resolved onto
// app.planDir, the default-dir branch must honour it.
func TestWritePlanFileHonorsConfiguredPlanDir(t *testing.T) {
	savedDir := app.planDir
	savedFlag := flagPlanOut
	defer func() { app.planDir = savedDir; flagPlanOut = savedFlag }()

	tmp := t.TempDir()
	app.planDir = tmp
	flagPlanOut = "" // exercise the default-dir branch, not --plan-out

	path, err := writePlanFile(&types.ChangePlan{ID: "p-123"})
	if err != nil {
		t.Fatalf("writePlanFile: %v", err)
	}
	want := filepath.Join(tmp, "p-123.json")
	if path != want {
		t.Errorf("plan written to %q, want %q", path, want)
	}
	if _, err := os.Stat(want); err != nil {
		t.Errorf("plan file missing at %q: %v", want, err)
	}
}
