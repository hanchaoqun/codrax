package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
)

func TestExpandKeywordsAbbreviations(t *testing.T) {
	tests := []struct {
		input []string
		want  string // at least this keyword should be in output
	}{
		{[]string{"auth"}, "authentication"},
		{[]string{"authentication"}, "auth"},
		{[]string{"config"}, "configuration"},
		{[]string{"configuration"}, "config"},
		{[]string{"ctx"}, "context"},
		{[]string{"exec"}, "execute"},
		{[]string{"eval"}, "evaluate"},
	}
	for _, tt := range tests {
		expanded := expandKeywords(tt.input)
		found := false
		for _, kw := range expanded {
			if strings.ToLower(kw) == tt.want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expandKeywords(%v): expected %q in result %v", tt.input, tt.want, expanded)
		}
	}
}

func TestExpandKeywordsNamingConventions(t *testing.T) {
	expanded := expandKeywords([]string{"sub_agent"})
	// Note: "subagent" (concatenated) is deduplicated with "SubAgent"
	// (CamelCase) because they share the same lowercase form.
	wants := []string{"sub_agent", "SubAgent", "sub-agent"}
	for _, w := range wants {
		found := false
		for _, kw := range expanded {
			if kw == w {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expandKeywords([sub_agent]): expected %q in result %v", w, expanded)
		}
	}
}

func TestExpandKeywordsNoAbbrevForUnknown(t *testing.T) {
	expanded := expandKeywords([]string{"foobar"})
	// "foobar" has no abbreviation pair, should only get itself
	if len(expanded) != 1 {
		t.Errorf("expandKeywords([foobar]): expected 1 keyword, got %d: %v", len(expanded), expanded)
	}
}

func TestDomainBoostFactor_MatchingPackageReturnsBoost(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/types/context.go":     {Package: "types"},
			"internal/tool/blob.go":         {Package: "tool"},
			"internal/orchestrator/main.go": {Package: "orchestrator"},
		},
	}
	// Term's Domain is "types"; only the types file gets the boost.
	got := domainBoostFactor("internal/types/context.go", graph, []string{"types"})
	if got <= 1.0 {
		t.Fatalf("expected boost > 1.0 for matching package, got %v", got)
	}
	got = domainBoostFactor("internal/tool/blob.go", graph, []string{"types"})
	if got != 1.0 {
		t.Fatalf("expected no boost for non-matching package, got %v", got)
	}
}

func TestDomainBoostFactor_EmptyInputsReturnsOne(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/types/context.go": {Package: "types"},
		},
	}
	// No domains, no boost.
	if got := domainBoostFactor("internal/types/context.go", graph, nil); got != 1.0 {
		t.Fatalf("nil domains should return 1.0, got %v", got)
	}
	// Nil graph, no boost.
	if got := domainBoostFactor("internal/types/context.go", nil, []string{"types"}); got != 1.0 {
		t.Fatalf("nil graph should return 1.0, got %v", got)
	}
	// File not in index, no boost.
	if got := domainBoostFactor("foo/unknown.go", graph, []string{"types"}); got != 1.0 {
		t.Fatalf("missing file should return 1.0, got %v", got)
	}
}

func TestDomainBoostFactor_EmptyPackageNotBoosted(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"scripts/build.sh": {Package: ""},
		},
	}
	// Empty hint must never match empty Package (otherwise every
	// package-less file would get boosted).
	if got := domainBoostFactor("scripts/build.sh", graph, []string{""}); got != 1.0 {
		t.Fatalf("empty-package file should not boost on empty hint, got %v", got)
	}
	if got := domainBoostFactor("scripts/build.sh", graph, []string{"scripts"}); got != 1.0 {
		t.Fatalf("empty Package cannot match any hint, got %v", got)
	}
}

func TestDomainBoostFactor_BoostSmallerThanEntityBoost(t *testing.T) {
	// Contract: domain boost is strictly smaller than the weakest
	// entity-boost branch (1.3). Domain is a coarser sibling signal
	// and must never overpower an exact-name entity match.
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"foo.go": {Package: "foo"},
		},
	}
	got := domainBoostFactor("foo.go", graph, []string{"foo"})
	if got >= 1.3 {
		t.Fatalf("domain boost %v must be strictly less than entity boost floor 1.3", got)
	}
}
