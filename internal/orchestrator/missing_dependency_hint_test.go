package orchestrator

import (
	"strings"
	"testing"
)

// missing_dependency_hint_test.go pins detectMissingDependencyHint's
// pattern table. The detector's job is to recognize "package not
// found / module not installed" failures emitted by N runners and
// prepend a structural retry-hint section directing the planner to
// update the project manifest. Detection is on machine-generated
// runner output (data side), not on user request text — this is the
// red-line distinction from the no-keyword-matching rule.

func TestDetectMissingDependencyHint_RecognizesEachLanguage(t *testing.T) {
	cases := []struct {
		name    string
		summary string
	}{
		{"python_modulenotfounderror", "FAILED test_main.py - ModuleNotFoundError: No module named 'pygame'"},
		{"python_importerror", "ImportError: No module named numpy"},
		{"python_importerror_cannot_import", "ImportError: cannot import name 'foo' from 'bar'"},
		{"node_cannot_find_module", "Error: Cannot find module 'lodash'\n  at Function.Module._resolveFilename"},
		{"node_err_module_not_found", "Error [ERR_MODULE_NOT_FOUND]: Cannot find package 'foo'"},
		{"go_no_required_module", "main.go:5:8: no required module provides package github.com/x/y"},
		{"go_cannot_find_package", "main.go:5:8: cannot find package \"foo\" in any of:"},
		{"go_missing_gosum", "missing go.sum entry for module providing package x"},
		{"rust_unresolved_import", "error[E0432]: unresolved import `tokio`"},
		{"rust_failed_to_resolve", "error[E0433]: failed to resolve: use of unresolved module or type `serde`"},
		{"rust_no_crate", "error[E0463]: can't find crate for `mycrate`"},
		{"java_package_not_exist", "error: package com.foo.bar does not exist"},
		{"java_cannot_find_symbol", "error: cannot find symbol\n  symbol: class FooBar"},
		{"ruby_load_error", "LoadError: cannot load such file -- foo"},
		{"c_fatal_error", "main.c:3:10: fatal error: stdio.h: No such file or directory"},
		{"c_file_not_found", "main.cpp:3:10: 'iostream' file not found"},
		{"swift_no_such_module", "main.swift:1:8: error: no such module 'Foundation'"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := detectMissingDependencyHint(c.summary)
			if got == "" {
				t.Fatalf("expected a hint for summary %q; got empty", c.summary)
			}
			// Hint must direct the planner to update the manifest.
			if !strings.Contains(got, "manifest") {
				t.Errorf("hint should mention 'manifest'; got %q", got)
			}
			// Hint must NOT leak internal tool names (the LLM-facing
			// hint references emit_change_plan etc. via a different
			// path; this section is structural advice only).
			for _, banned := range []string{"emit_change_plan", "PartialChangePlan", "BusContext"} {
				if strings.Contains(got, banned) {
					t.Errorf("hint must not leak %q; got %q", banned, got)
				}
			}
		})
	}
}

func TestDetectMissingDependencyHint_SkipsUnrelatedFailures(t *testing.T) {
	// Generic test failures without dep markers must not trigger.
	cases := []string{
		"",
		"AssertionError: expected 5 got 4",
		"timeout: test exceeded 30s",
		"panic: runtime error: index out of range",
		"FAIL TestAdd 0.01s",
	}
	for _, summary := range cases {
		if got := detectMissingDependencyHint(summary); got != "" {
			t.Errorf("non-dep failure %q should not trigger hint; got: %q", summary, got)
		}
	}
}
