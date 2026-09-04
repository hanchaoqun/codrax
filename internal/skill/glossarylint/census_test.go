package glossarylint

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// moduleRoot resolves the repository root from this package directory
// and refuses to guess: go.mod must be there.
func moduleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("module root %s has no go.mod: %v", root, err)
	}
	return root
}

// TestRendererRosterTotality is the repo-wide census behind the
// "single lint entry" promise (§40.52, V8-7/V11-3): every package that
// carries a producer shape is in RendererRoster, every roster row has a
// marker test running exactly the lanes it declares, and no marker
// exists outside the roster. Any of the three drifting fails naming
// the package, so a new renderer package cannot ship unscanned.
func TestRendererRosterTotality(t *testing.T) {
	root := moduleRoot(t)
	producers, err := ProducerPackages(root)
	if err != nil {
		t.Fatalf("%v", err)
	}
	markers, err := MarkerPackages(root)
	if err != nil {
		t.Fatalf("%v", err)
	}
	roster := map[string]RosterEntry{}
	for _, e := range RendererRoster {
		if _, dup := roster[e.Dir]; dup {
			t.Errorf("roster lists %s twice", e.Dir)
		}
		if e.Lane == 0 || e.Why == "" {
			t.Errorf("roster row %s must declare a lane and a reason", e.Dir)
		}
		roster[e.Dir] = e
	}
	byDir := map[string][]Producer{}
	for _, p := range producers {
		byDir[p.Dir] = append(byDir[p.Dir], p)
	}
	if len(byDir) == 0 {
		t.Fatalf("producer census found no producer shape anywhere — walker is broken")
	}
	for dir, ps := range byDir {
		if _, ok := roster[dir]; ok {
			continue
		}
		shapes := map[string]bool{}
		for _, p := range ps {
			shapes[p.Shape+"@"+p.Where] = true
		}
		var list []string
		for s := range shapes {
			list = append(list, s)
		}
		sort.Strings(list)
		if len(list) > 6 {
			list = append(list[:6], "…")
		}
		t.Errorf("package %s carries producer shapes but is not in glossarylint.RendererRoster: %s", dir, strings.Join(list, ", "))
	}
	for dir, e := range roster {
		if _, err := os.Stat(filepath.Join(root, dir)); err != nil {
			t.Errorf("stale roster row %s: %v", dir, err)
			continue
		}
		got := markers[dir]
		if got != e.Lane {
			t.Errorf("roster row %s declares lane %s but its _test.go files call %s", dir, e.Lane, got)
		}
	}
	for dir, lane := range markers {
		if _, ok := roster[dir]; !ok {
			t.Errorf("package %s calls glossarylint marker %s but has no RendererRoster row — add the row with its reason", dir, lane)
		}
	}
}

// TestNoGlossaryForksInTests pins that no test outside this package
// re-lists glossary vocabulary in a string-slice literal (the
// orchestrator's retired 22-entry mirror is the pattern this closes).
func TestNoGlossaryForksInTests(t *testing.T) {
	forks, err := GlossaryForks(moduleRoot(t))
	if err != nil {
		t.Fatalf("%v", err)
	}
	for _, f := range forks {
		t.Errorf("  %s", f)
	}
	if len(forks) > 0 {
		t.Fatalf("%d glossary fork(s) in test files", len(forks))
	}
}

// TestCensus_SelfRedOnScratchTree proves each producer shape and the
// marker detector on a synthetic module tree, including the fail-loud
// unrecognized-carrier path.
func TestCensus_SelfRedOnScratchTree(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	write("go.mod", "module scratch\n")
	write("main.go", "package main\n\nfunc main() {}\n")
	write("internal/ev/ev.go", `package ev

import "github.com/hanchaoqun/codrax/internal/types"

type e struct{}

func (e) BuildInitialInstruction() string { return "" }

var v = types.Violation{Detail: "x"}
`)
	write("internal/ev/ev_test.go", `package ev

import gl "github.com/hanchaoqun/codrax/internal/skill/glossarylint"

func TestX(t *testing.T) { gl.RunPackageScan(t, ".", gl.Policy{}) }
`)
	write("internal/tl/tl.go", `package tl

import "encoding/json"

type tool struct{}

func (tool) Parameters() json.RawMessage { return nil }

const helperSystemPrompt = "p"
`)
	write("internal/al/al.go", `package al

import "github.com/hanchaoqun/codrax/internal/analysis/hint"

type Hint = hint.Hint

var h = Hint{WhatFailed: "x"}
`)
	write("internal/def/def.go", `package def

type Violation struct{ Detail string }

var v = Violation{Detail: "constructor in the defining package"}
`)
	write("cmd/cli.go", `package cmd

import "github.com/hanchaoqun/codrax/internal/llm"

var summarizerSystemPrompt = "p"
var schema = llm.ToolSchema{Name: "x"}
`)
	write("internal/sm/sm.go", `package sm

import "github.com/hanchaoqun/codrax/internal/llm"

var wireKeyPrompt = "not_a_prompt_key" // the suffix convention is SystemPrompt

func messages() []llm.Message {
	return []llm.Message{{Role: "system", Content: "x"}, {Role: "user", Content: "y"}}
}
`)
	write("internal/skip/testdata/fixture.go", `package fixture

import "github.com/hanchaoqun/codrax/internal/types"

var v = types.Violation{Detail: "testdata trees are not walked"}
`)
	write("docs/x.go", `package docs

import "github.com/hanchaoqun/codrax/internal/types"

var v = types.Violation{Detail: "outside the census roots"}
`)

	producers, err := ProducerPackages(root)
	if err != nil {
		t.Fatalf("%v", err)
	}
	got := map[string][]string{}
	for _, p := range producers {
		got[p.Dir] = append(got[p.Dir], p.Shape)
	}
	for dir := range got {
		sort.Strings(got[dir])
	}
	want := map[string]string{
		"internal/ev": "BuildInitialInstruction Carrier:Violation",
		"internal/tl": "PromptConst ToolParameters",
		"internal/al": "Carrier:Hint",
		"cmd":         "Carrier:ToolSchema PromptConst",
		"internal/sm": "SystemMessage",
	}
	for dir, shapes := range want {
		if strings.Join(got[dir], " ") != shapes {
			t.Errorf("%s: shapes %v, want %s", dir, got[dir], shapes)
		}
	}
	for _, dir := range []string{"internal/def", "internal/skip/testdata", "docs", "."} {
		if len(got[dir]) != 0 {
			t.Errorf("%s must not be a producer: %v", dir, got[dir])
		}
	}
	markers, err := MarkerPackages(root)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if markers["internal/ev"] != LanePackage || len(markers) != 1 {
		t.Errorf("markers = %v, want only internal/ev → RunPackageScan", markers)
	}

	write("internal/bad/bad.go", `package bad

var v = SuspectedRoot{Reason: "neither alias nor local type"}
`)
	if _, err := ProducerPackages(root); err == nil || !strings.Contains(err.Error(), "unrecognized carrier shape") {
		t.Fatalf("unaliased carrier literal must fail loud, got %v", err)
	}
}

func TestGlossaryForks_SelfRed(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "x"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module scratch\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	body := `package x

import "strings"

var fixture = []string{"TaskGraph", "EvidencePlan"} // data, never an oracle

func oracle(text string) bool {
	banned := []string{"TaskGraph", "EvidencePlan", "local-extra"}
	for _, term := range banned {
		if strings.Contains(text, term) {
			return true
		}
	}
	for _, term := range []string{"TaskGraph", "local-only"} {
		if strings.Contains(text, term) {
			return true
		}
	}
	for _, term := range []string{"AnalysisIR", "BusContext"} {
		_ = term // ranged, but never matched against text
	}
	return false
}
`
	if err := os.WriteFile(filepath.Join(root, "internal", "x", "x_test.go"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// The vocabulary owner and its dependency: the ForkExemptions row for
	// internal/types is verified against skill's imports, and an oracle
	// inside the exempt package is skipped.
	for rel, content := range map[string]string{
		"internal/skill/skill.go":  "package skill\n\nimport _ \"github.com/hanchaoqun/codrax/internal/types\"\n",
		"internal/types/types.go":  "package types\n",
		"internal/types/t_test.go": strings.Replace(body, "package x", "package types", 1),
	} {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	forks, err := GlossaryForks(root)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(forks) != 1 || !strings.Contains(forks[0], "x_test.go:8") {
		t.Fatalf("expected exactly the two-entry oracle bound at line 8 to be a fork (types oracle exempt), got %v", forks)
	}
	// A stale exemption row — skill no longer imports the package — fails loud.
	if err := os.WriteFile(filepath.Join(root, "internal", "skill", "skill.go"), []byte("package skill\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := GlossaryForks(root); err == nil || !strings.Contains(err.Error(), "stale fork exemption") {
		t.Fatalf("stale vocabulary-dependency row must fail loud, got %v", err)
	}
}
