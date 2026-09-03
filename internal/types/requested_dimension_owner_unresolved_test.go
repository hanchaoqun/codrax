package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

// V4-4 (colleague_merge_audit §40.22): an owner the analyzer could not name
// is a typed SOFT marker, compiled from schema-validated fields only.

func ownerUnresolvedProfile() *RequestedAnswerDimensionProfile {
	return &RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []RequestedAnswerDimension{
		{Index: 1, Role: RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		{Index: 2, Role: RequestedAnswerDimensionBranchBehavior, Required: true},
	}}
}

func TestCompileDimensionOwnerUnresolved_MarksMissingOwnerAndUnclassifiedFile(t *testing.T) {
	got := CompileDimensionOwnerUnresolved(ownerUnresolvedProfile(), []RequiredFileHint{
		{Path: "./config/load.go", Confidence: 0.95, RequestedDimensionIndices: []int{1}},
		{Path: "cmd/root.go", Confidence: 0.95},
	})
	want := &DimensionOwnerUnresolved{DimensionIndices: []int{2}, UnclassifiedFiles: []string{"cmd/root.go"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("marker=%+v want %+v", got, want)
	}
}

func TestCompileDimensionOwnerUnresolved_NilWhenSettledOrNotAsserted(t *testing.T) {
	profile := ownerUnresolvedProfile()
	cases := map[string]struct {
		profile *RequestedAnswerDimensionProfile
		hints   []RequiredFileHint
	}{
		"every dimension owned and every file classified": {profile, []RequiredFileHint{
			{Path: "config/load.go", Confidence: 0.95, RequestedDimensionIndices: []int{1, 2}},
			{Path: "docs/nav.go", Confidence: 0.9, RequestedDimensionNavigationOnly: true},
		}},
		"no high-confidence hint (legacy any-source seats)": {profile, []RequiredFileHint{
			{Path: "config/load.go", Confidence: 0.7},
		}},
		"single ownership dimension": {&RequestedAnswerDimensionProfile{IsDimensionedAnswer: true, Dimensions: []RequestedAnswerDimension{
			{Index: 1, Role: RequestedAnswerDimensionFunctionOrPurpose, Required: true},
		}}, []RequiredFileHint{{Path: "config/load.go", Confidence: 0.95}}},
		"nil profile": {nil, []RequiredFileHint{{Path: "config/load.go", Confidence: 0.95}}},
	}
	for name, tc := range cases {
		if got := CompileDimensionOwnerUnresolved(tc.profile, tc.hints); got != nil {
			t.Fatalf("%s: marker=%+v want nil", name, got)
		}
	}
}

func TestCompileDimensionOwnerUnresolved_NavigationOnlyIsAClassificationNotAnOwner(t *testing.T) {
	got := CompileDimensionOwnerUnresolved(ownerUnresolvedProfile(), []RequiredFileHint{
		{Path: "docs/nav.go", Confidence: 0.9, RequestedDimensionNavigationOnly: true},
	})
	want := &DimensionOwnerUnresolved{DimensionIndices: []int{1, 2}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("marker=%+v want %+v (navigation-only file is classified, dimensions still unowned)", got, want)
	}
}

func TestRequiredFileHint_NavigationOnlyRoundTrips(t *testing.T) {
	in := RequiredFileHint{Path: "docs/nav.go", Confidence: 0.9, RequestedDimensionNavigationOnly: true}
	body, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out RequiredFileHint
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatal(err)
	}
	if !out.RequestedDimensionNavigationOnly {
		t.Fatalf("navigation-only classification lost across persistence: %s", body)
	}
	plain, _ := json.Marshal(RequiredFileHint{Path: "a.go", Confidence: 0.9})
	if reflect.DeepEqual(plain, body) || string(plain) != `{"path":"a.go","confidence":0.9}` {
		t.Fatalf("false flag must be omitted: %s", plain)
	}
}

func TestDimensionOwnerUnresolved_CloneIsNilSafeAndDetached(t *testing.T) {
	var nilMarker *DimensionOwnerUnresolved
	if nilMarker.Clone() != nil {
		t.Fatal("nil clone must stay nil")
	}
	src := &DimensionOwnerUnresolved{DimensionIndices: []int{2}, UnclassifiedFiles: []string{"cmd/root.go"}}
	dst := src.Clone()
	dst.DimensionIndices[0] = 9
	if src.DimensionIndices[0] != 2 || !reflect.DeepEqual(src.UnclassifiedFiles, []string{"cmd/root.go"}) {
		t.Fatalf("clone aliased the source: %+v", src)
	}
}

// §40.47 fold-in (A0): a system-projected hint is not a model declaration.
// A fully declared model roster plus a system hint (runtime-artifact path,
// prescan candidate, scope promotion, user pin) compiles to nil, and a
// system-only high-confidence roster is "no model assertion" (nil), while the
// model's own unclassified file still marks.
func TestCompileDimensionOwnerUnresolved_IgnoresSystemProjectedHints(t *testing.T) {
	profile := ownerUnresolvedProfile()
	declared := RequiredFileHint{Path: "config/load.go", Confidence: 0.95, RequestedDimensionIndices: []int{1, 2}}
	for _, origin := range []RequiredFileHintOrigin{
		RequiredFileHintOriginRuntimeArtifactPath,
		RequiredFileHintOriginAnalyzerPrescan,
		RequiredFileHintOriginPrincipalScopePromotion,
		RequiredFileHintOriginUserPinnedPath,
	} {
		system := RequiredFileHint{Path: "/tmp/app.systrace", Confidence: 0.8, Origin: origin}
		if got := CompileDimensionOwnerUnresolved(profile, []RequiredFileHint{declared, system}); got != nil {
			t.Fatalf("origin %q: system hint entered the marker: %+v", origin, got)
		}
		if got := CompileDimensionOwnerUnresolved(profile, []RequiredFileHint{system}); got != nil {
			t.Fatalf("origin %q: system-only roster is not a model assertion, got %+v", origin, got)
		}
		if system.ModelDeclared() {
			t.Fatalf("origin %q reports ModelDeclared", origin)
		}
	}
	got := CompileDimensionOwnerUnresolved(profile, []RequiredFileHint{
		{Path: "cmd/root.go", Confidence: 0.95},
		{Path: "/tmp/app.systrace", Confidence: 0.8, Origin: RequiredFileHintOriginRuntimeArtifactPath},
	})
	want := &DimensionOwnerUnresolved{DimensionIndices: []int{1, 2}, UnclassifiedFiles: []string{"cmd/root.go"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("model-declared gap must survive next to a system hint: got %+v want %+v", got, want)
	}
	var roundTrip RequiredFileHint
	if err := json.Unmarshal([]byte(`{"path":"cmd/root.go","confidence":0.9}`), &roundTrip); err != nil {
		t.Fatal(err)
	}
	if !roundTrip.ModelDeclared() {
		t.Fatalf("a persisted hint without origin must decode as model-declared: %+v", roundTrip)
	}
}
