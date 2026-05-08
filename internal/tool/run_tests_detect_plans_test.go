package tool

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectRunnerPlans_NestedProjects(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(rel, body string) {
		t.Helper()
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("services/api/go.mod", "module example.com/api\n")
	mustWrite("web/package.json", `{"name":"web","scripts":{"test":"vitest"}}`)

	plans := detectRunnerPlans(dir, "")
	if len(plans) != 2 {
		t.Fatalf("detectRunnerPlans() len = %d, want 2 (%+v)", len(plans), plans)
	}
	if got := runnerPlanRel(dir, plans[0]); got != "services/api" {
		t.Fatalf("plans[0] rel = %q, want services/api", got)
	}
	if plans[0].Runner != "go" {
		t.Fatalf("plans[0].Runner = %q, want go", plans[0].Runner)
	}
	if got := runnerPlanRel(dir, plans[1]); got != "web" {
		t.Fatalf("plans[1] rel = %q, want web", got)
	}
	if plans[1].Runner != "node" {
		t.Fatalf("plans[1].Runner = %q, want node", plans[1].Runner)
	}
}

func TestDetectRunnerPlans_PrefersSpecificManifestPerDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "CMakeLists.txt"), []byte("project(foo)\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Makefile"), []byte("check:\n\ttrue\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plans := detectRunnerPlans(dir, "")
	if len(plans) != 1 {
		t.Fatalf("detectRunnerPlans() len = %d, want 1", len(plans))
	}
	if plans[0].Runner != "cmake" {
		t.Fatalf("Runner = %q, want cmake", plans[0].Runner)
	}
}

func TestDetectRunnerPlans_SkipsBuildAndVendorDirs(t *testing.T) {
	dir := t.TempDir()
	mustWrite := func(rel, body string) {
		t.Helper()
		abs := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("package.json", `{"name":"root","scripts":{"test":"jest"}}`)
	mustWrite("vendor/nested/go.mod", "module example.com/vendor\n")
	mustWrite("build/generated/package.json", `{"name":"generated"}`)

	plans := detectRunnerPlans(dir, "")
	if len(plans) != 1 {
		t.Fatalf("detectRunnerPlans() len = %d, want 1", len(plans))
	}
	if plans[0].Runner != "node" || runnerPlanRel(dir, plans[0]) != "." {
		t.Fatalf("unexpected root plan: %+v", plans[0])
	}
}

// TestDetectRunnerPlans_WalkRootScoping verifies the multi-repo
// fix (design §4.6 Phase 5): when walkRoot is non-empty, manifest
// discovery is confined to that sub-tree even though the labels
// stay relative to repoRoot. Layout under tmp:
//
//	parent/
//	  sub-go/go.mod
//	  sub-node/package.json
//
// Walking from sub-go alone must NOT find sub-node's package.json.
func TestDetectRunnerPlans_WalkRootScoping(t *testing.T) {
	parent := t.TempDir()
	mustWrite := func(rel, body string) {
		t.Helper()
		abs := filepath.Join(parent, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	mustWrite("sub-go/go.mod", "module example.com/api\n")
	mustWrite("sub-node/package.json", `{"name":"web","scripts":{"test":"vitest"}}`)

	subGo := filepath.Join(parent, "sub-go")
	plans := detectRunnerPlans(parent, subGo)
	if len(plans) != 1 {
		t.Fatalf("walkRoot=sub-go: expected 1 plan, got %d (%+v)", len(plans), plans)
	}
	if plans[0].Runner != "go" {
		t.Fatalf("expected go runner only (sub-node should be invisible), got %s", plans[0].Runner)
	}
	if got := runnerPlanRel(parent, plans[0]); got != "sub-go" {
		t.Fatalf("plan label should be relative to repoRoot=parent, got %q", got)
	}

	// Sanity: walkRoot empty falls back to parent → both plans surface.
	all := detectRunnerPlans(parent, "")
	if len(all) != 2 {
		t.Fatalf("walkRoot empty: expected 2 plans (sub-go + sub-node), got %d", len(all))
	}
}
