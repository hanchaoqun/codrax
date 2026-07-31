package types

import "testing"

func TestRuntimeArtifactScopeProfileAuthorityRequiresAnchoredValidShape(t *testing.T) {
	start, end := 10.25, 10.75
	full := &RuntimeArtifactScopeProfile{
		RequestedScope: RuntimeArtifactScopeFullArtifact,
		SourceQuote:    "this trace",
	}
	if !full.FullArtifact() {
		t.Fatal("quote-anchored full_artifact should be authoritative")
	}
	full.SourceQuote = ""
	if full.FullArtifact() {
		t.Fatal("unanchored full_artifact must not be authoritative")
	}
	if full.Active() {
		t.Fatal("unanchored full_artifact must not be active")
	}

	explicit := &RuntimeArtifactScopeProfile{
		RequestedScope: RuntimeArtifactScopeExplicitWindow,
		TimeStart:      &start,
		TimeEnd:        &end,
		SourceQuote:    "10.25..10.75",
	}
	gotStart, gotEnd, ok := explicit.ExplicitTimeWindow()
	if !ok || gotStart != start || gotEnd != end {
		t.Fatalf("explicit window authority drifted: %.3f..%.3f ok=%t", gotStart, gotEnd, ok)
	}
	if !explicit.Active() {
		t.Fatal("anchored valid explicit window should be active")
	}
	badEnd := start
	explicit.TimeEnd = &badEnd
	if _, _, ok := explicit.ExplicitTimeWindow(); ok {
		t.Fatal("non-positive explicit window must fail closed")
	}
	if explicit.Active() {
		t.Fatal("invalid explicit window must not be active")
	}

	bounded := &RuntimeArtifactScopeProfile{
		RequestedScope: RuntimeArtifactScopeBoundedSelector,
		SourceQuote:    "selected process",
	}
	if !bounded.Active() {
		t.Fatal("anchored bounded selector should be active")
	}
	bounded.RequestedScope = RuntimeArtifactScopeUnspecified
	if bounded.Active() {
		t.Fatal("unspecified scope must not be active")
	}
}
