package repl

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/topology"
)

// reposCmdTestREPL builds a minimal REPL just plump enough to exercise
// reposFocus / reposUnfocus end-to-end. Real codepaths only need:
//   - r.topology pointer (set directly, no Discover round-trip)
//   - r.multiRepoFocus map
//   - r.multiRepoMu lock
//   - r.out non-nil writer (info/warn route here in non-interactive mode)
func reposCmdTestREPL(topo *topology.RepoTopology) (*REPL, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	r := &REPL{
		// info / warn route to r.out only when r.interactive() is
		// false; interactive() returns r.in == nil. Set r.in to a
		// real (empty) reader so the line-oriented branch fires
		// and the test buffer captures the output.
		in:               bytes.NewReader(nil),
		out:              buf,
		multiRepoFocus:   map[string]bool{},
		multiRepoMu:      sync.Mutex{},
		topology:         topo,
		multiRepoEnabled: true,
	}
	return r, buf
}

func TestReposList_DisabledPostureDoesNotAdvertiseRoutingFold(t *testing.T) {
	topo := &topology.RepoTopology{
		ParentRoot: "/workspace",
		ParentSlug: "workspace-slug",
		Repos: []topology.SubRepo{
			{Slug: "slug-a", RootRel: "alpha", FileCount: 10},
			{Slug: "slug-b", RootRel: "beta", FileCount: 12},
		},
	}
	r, buf := reposCmdTestREPL(topo)
	r.multiRepoEnabled = false

	r.handleReposCmd("/repos")

	got := buf.String()
	for _, want := range []string{"single-repo routing", "multi-repo routing disabled", "discovered-sub-repos=2"} {
		if !strings.Contains(got, want) {
			t.Fatalf("disabled /repos output missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"auto-active", "routing fold"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("disabled /repos output must not advertise %q:\n%s", forbidden, got)
		}
	}
}

func TestReposFocus_AcceptsSlugAndRootRel(t *testing.T) {
	topo := &topology.RepoTopology{
		Repos: []topology.SubRepo{
			{Slug: "slug-a", RootRel: "alpha"},
			{Slug: "slug-n", RootRel: "outer/nested"},
		},
	}

	cases := []struct {
		name     string
		token    string
		wantSlug string // pinned key after the call
	}{
		{"by slug", "slug-a", "slug-a"},
		{"by RootRel", "alpha", "slug-a"},
		{"by nested RootRel slash", "outer/nested", "slug-n"},
		{"by nested RootRel backslash", `outer\nested`, "slug-n"},
		{"by RootRel with leading ./", "./alpha", "slug-a"},
		{"by RootRel with trailing slash", "alpha/", "slug-a"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, buf := reposCmdTestREPL(topo)
			r.reposFocus(tc.token)
			r.multiRepoMu.Lock()
			pinned := r.multiRepoFocus[tc.wantSlug]
			r.multiRepoMu.Unlock()
			if !pinned {
				t.Errorf("reposFocus(%q): slug %q not pinned; out:\n%s",
					tc.token, tc.wantSlug, buf.String())
			}
			if !strings.Contains(buf.String(), "/repos focus: pinned") {
				t.Errorf("reposFocus(%q): missing success line; out:\n%s",
					tc.token, buf.String())
			}
		})
	}
}

func TestReposFocus_RejectsUnknownToken(t *testing.T) {
	topo := &topology.RepoTopology{
		Repos: []topology.SubRepo{
			{Slug: "slug-a", RootRel: "alpha"},
		},
	}
	r, buf := reposCmdTestREPL(topo)
	r.reposFocus("nonexistent")
	r.multiRepoMu.Lock()
	pinCount := len(r.multiRepoFocus)
	r.multiRepoMu.Unlock()
	if pinCount != 0 {
		t.Errorf("unknown token should not pin anything; got %d pins", pinCount)
	}
	if !strings.Contains(buf.String(), "no sub-repo with slug or path") {
		t.Errorf("expected 'no sub-repo' warn message; out:\n%s", buf.String())
	}
}

func TestReposFocus_RejectsPinsAboveActiveCap(t *testing.T) {
	topo := &topology.RepoTopology{
		Repos: []topology.SubRepo{
			{Slug: "slug-a", RootRel: "alpha"},
			{Slug: "slug-b", RootRel: "beta"},
			{Slug: "slug-c", RootRel: "gamma"},
		},
	}
	r, buf := reposCmdTestREPL(topo)
	r.multiRepoMaxActiveOverride = 2
	r.reposFocus("alpha")
	r.reposFocus("beta")
	r.reposFocus("gamma")
	r.multiRepoMu.Lock()
	pinCount := len(r.multiRepoFocus)
	r.multiRepoMu.Unlock()
	if pinCount != 2 {
		t.Fatalf("third focus above cap should be rejected; got %d pins", pinCount)
	}
	if !strings.Contains(buf.String(), "above current active cap 2") {
		t.Fatalf("expected cap warning, got:\n%s", buf.String())
	}
}

func TestReposCap_RejectsBelowFocusedCount(t *testing.T) {
	topo := &topology.RepoTopology{
		Repos: []topology.SubRepo{
			{Slug: "slug-a", RootRel: "alpha"},
			{Slug: "slug-b", RootRel: "beta"},
		},
	}
	r, buf := reposCmdTestREPL(topo)
	r.multiRepoMaxActiveOverride = 2
	r.reposFocus("alpha")
	r.reposFocus("beta")
	r.reposCap(1)
	r.multiRepoMu.Lock()
	override := r.multiRepoMaxActiveOverride
	r.multiRepoMu.Unlock()
	if override != 2 {
		t.Fatalf("cap should remain 2 when lowering below focused count, got %d", override)
	}
	if !strings.Contains(buf.String(), "below the current focused sub-repo count 2") {
		t.Fatalf("expected focused-count warning, got:\n%s", buf.String())
	}
}

func TestReposUnfocus_AcceptsSlugAndRootRel(t *testing.T) {
	topo := &topology.RepoTopology{
		Repos: []topology.SubRepo{
			{Slug: "slug-a", RootRel: "alpha"},
			{Slug: "slug-n", RootRel: "outer/nested"},
		},
	}
	// Pin both first.
	r, _ := reposCmdTestREPL(topo)
	r.reposFocus("slug-a")
	r.reposFocus("slug-n")
	r.multiRepoMu.Lock()
	if len(r.multiRepoFocus) != 2 {
		r.multiRepoMu.Unlock()
		t.Fatalf("setup: expected 2 pins, got %d", len(r.multiRepoFocus))
	}
	r.multiRepoMu.Unlock()

	// Unfocus by RootRel — internal state still keys by Slug.
	r.reposUnfocus("alpha")
	r.multiRepoMu.Lock()
	if r.multiRepoFocus["slug-a"] {
		t.Errorf("unfocus by RootRel 'alpha' did not remove slug-a")
	}
	if !r.multiRepoFocus["slug-n"] {
		t.Errorf("unfocus by RootRel 'alpha' should not affect slug-n")
	}
	r.multiRepoMu.Unlock()

	// Unfocus by Slug — symmetric.
	r.reposUnfocus("slug-n")
	r.multiRepoMu.Lock()
	if r.multiRepoFocus["slug-n"] {
		t.Errorf("unfocus by slug-n did not remove the pin")
	}
	r.multiRepoMu.Unlock()
}
