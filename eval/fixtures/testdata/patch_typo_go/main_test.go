package main

import "testing"

// TestGreet gives the verify stage something to run. Pre-apply it
// will fail to compile (the main.go typo prevents the package from
// building at all); post-apply it must compile + pass. This is the
// functional corroborant of "did the write actually fix the bug" —
// separate from the string-match verdict in eval/run.sh.
func TestGreet(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", "Hello, world!"},
		{"  ", "Hello, world!"},
		{"codrax", "Hello, codrax!"},
	}
	for _, c := range cases {
		if got := greet(c.in); got != c.want {
			t.Errorf("greet(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
