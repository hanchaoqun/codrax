package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

type focusSubRepo struct {
	rootRel string
	slug    string
}

type fakeFocusResolver struct {
	cap   int
	repos []focusSubRepo
}

func (f fakeFocusResolver) IsSingle() bool { return len(f.repos) == 1 }
func (f fakeFocusResolver) Cap() int       { return f.cap }
func (f fakeFocusResolver) MultiRepoSubRepoCount() int {
	return len(f.repos)
}
func (f fakeFocusResolver) ResolveMultiRepoSubRepo(token string) (string, string, bool) {
	trimmed := strings.Trim(strings.ReplaceAll(token, "\\", "/"), "/")
	for _, repo := range f.repos {
		if trimmed == repo.rootRel || trimmed == repo.slug {
			return repo.rootRel, repo.slug, true
		}
	}
	return "", "", false
}

func testMultiRepoFocusBus(t *testing.T, objective string, capN int) *types.BusContext {
	t.Helper()
	return &types.BusContext{
		Mutable: types.NewMutableState(objective),
		MultiGraph: fakeFocusResolver{cap: capN, repos: []focusSubRepo{
			{slug: "api-aa", rootRel: "api"},
			{slug: "web-bb", rootRel: "web"},
			{slug: "docs-cc", rootRel: "docs"},
		}},
	}
}

func TestEmitMultiRepoFocus_ToleratesStringArray(t *testing.T) {
	ctx := testMultiRepoFocusBus(t, "explain web package", 2)
	payload := `{"recommended_focus_subrepos":["web"],"confidence":"0.73"}`
	res, err := (&EmitMultiRepoFocus{}).Execute(ctx, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got %s", res.Summary)
	}
	decision := ctx.Mutable.MultiRepoFocusDecision()
	if decision == nil || len(decision.Candidates) != 1 || decision.Candidates[0].RootRel != "web" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if decision.Confidence != 0.73 {
		t.Fatalf("confidence string should parse to 0.73, got %.2f", decision.Confidence)
	}
}

func TestEmitMultiRepoFocus_ToleratesCommaString(t *testing.T) {
	ctx := testMultiRepoFocusBus(t, "compare api and web", 2)
	payload := `{"recommended_focus_subrepos":"api, web","confidence":0.8}`
	res, err := (&EmitMultiRepoFocus{}).Execute(ctx, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected success, got %s", res.Summary)
	}
	decision := ctx.Mutable.MultiRepoFocusDecision()
	if decision == nil || len(decision.Candidates) != 2 {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if decision.Candidates[0].RootRel != "api" || decision.Candidates[1].RootRel != "web" {
		t.Fatalf("comma string should preserve resolved order, got %#v", decision.Candidates)
	}
}

func TestEmitMultiRepoFocus_ExplicitRequestRequiresExactPathMention(t *testing.T) {
	ctx := testMultiRepoFocusBus(t, "only inspect api for this question", 2)
	payload := `{
		"recommended_focus_subrepos":[{"root_rel":"api","source":"user_explicit_in_request"}],
		"confidence":0.9
	}`
	res, err := (&EmitMultiRepoFocus{}).Execute(ctx, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !res.Success {
		t.Fatalf("expected explicit path success, got %s", res.Summary)
	}
	if got := ctx.Mutable.MultiRepoFocusDecision().Source; got != types.MultiRepoFocusSourceUserExplicitInRequest {
		t.Fatalf("candidate-level explicit source should lift to decision source, got %q", got)
	}

	ctx = testMultiRepoFocusBus(t, "only inspect backend for this question", 2)
	res, err = (&EmitMultiRepoFocus{}).Execute(ctx, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success {
		t.Fatalf("explicit source without exact root_rel mention should fail")
	}
	if !strings.Contains(res.Summary, "current request does not exactly mention") {
		t.Fatalf("failure should guide model to use model_recommended or exact paths, got %q", res.Summary)
	}
}

func TestEmitMultiRepoFocus_RejectsOverCap(t *testing.T) {
	ctx := testMultiRepoFocusBus(t, "compare api web docs", 2)
	payload := `{"recommended_focus_subrepos":["api","web","docs"],"confidence":0.8}`
	res, err := (&EmitMultiRepoFocus{}).Execute(ctx, json.RawMessage(payload))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.Success {
		t.Fatalf("over-cap recommendation should fail")
	}
	if !strings.Contains(res.Summary, "exceed active cap 2") {
		t.Fatalf("expected cap guidance, got %q", res.Summary)
	}
}
