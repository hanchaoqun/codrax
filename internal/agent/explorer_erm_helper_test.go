package agent

import "testing"

func TestFirstSegmentIsBinds(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"binds at start", "`RegisterDefaultSubAgents()` binds ONLY NewSubExplorer(deps) → `SubExplorer.Name()` returns \"explorer\"", true},
		{"returns at start", "`NewProposeSubAgents()` returns &ProposeSubAgents{ → `ProposeSubAgents.Name()` returns \"propose_sub_agents\"", false},
		{"no arrow binds", "`Foo()` binds ONLY Bar", true},
		{"no arrow returns", "`Foo()` returns Bar", false},
		{"binds in second segment only", "`NewFoo()` returns &Foo{} → `Foo.init()` binds X", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := firstSegmentIsBinds(tc.in); got != tc.want {
				t.Errorf("got %v want %v for: %s", got, tc.want, tc.in)
			}
		})
	}
}

func TestEndsWithShortLiteralReturn(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"3-hop Name", "`RegisterDefaultSubAgents()` binds ONLY NewSubExplorer(deps) → `SubExplorer.Name()` returns \"explorer\"", true},
		{"Description long", "`NewProposeSubAgents()` returns &ProposeSubAgents{ → `ProposeSubAgents.Description()` returns \"Propose splitting the current task into parallel sub-tasks\"", false},
		{"2-hop Run assigns", "`RegisterDefaultSubAgents()` binds ONLY NewSubExplorer(deps) → `SubExplorer.Run()` assigns sk := &skill.Config{", false},
		{"with source locator", "`RegisterDefaultSubAgents()` binds ONLY NewSubExplorer(deps) → `SubExplorer.Name()` returns \"explorer\" (internal/agent/sub_explorer.go:32)", true},
		{"short bool", "`A.Foo()` returns &B{} → `B.IsWrite()` returns false", false}, // not quoted
		{"short single-quoted", "`a()` returns b → `c()` returns 'x'", true},
		{"no arrow", "standalone returns \"x\"", true},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := endsWithShortLiteralReturn(tc.in)
			if got != tc.want {
				t.Errorf("got %v want %v for: %s", got, tc.want, tc.in)
			}
		})
	}
}
