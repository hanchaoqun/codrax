package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestConcreteValueKindToAnchorKind_AllKnownKinds pins the 11→6
// projection table. Adding a new kind in concrete_values_lang.go
// (or explorer.go's inline scanners) MUST add an arm here to keep
// the projection deterministic.
func TestConcreteValueKindToAnchorKind_AllKnownKinds(t *testing.T) {
	cases := []struct {
		kind string
		want types.AnchorKind
	}{
		{"returns", types.AnchorReturn},
		{"errors", types.AnchorReturn},
		{"calls", types.AnchorCall},
		{"binds", types.AnchorCall},
		{"conditional", types.AnchorCondition},
		{"assigns", types.AnchorAssignment},
		{"maps", types.AnchorAssignment},
		{"config", types.AnchorAssignment},
		{"decorates", types.AnchorDefinition},
		{"embeds", types.AnchorDefinition},
		{"implements", types.AnchorDefinition},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			if got := concreteValueKindToAnchorKind(c.kind); got != c.want {
				t.Errorf("kind=%q → got %q want %q", c.kind, got, c.want)
			}
		})
	}
}

// TestConcreteValueKindToAnchorKind_UnknownReturnsEmpty pins the
// fallback contract: an unknown kind MUST return AnchorKind="" so
// ClaimFormOf falls through to ClaimUnknown rather than
// fabricating a wrong projection. This keeps the 11→6 mapping a
// closed set; new kinds get explicit handling, never silent
// misclassification.
func TestConcreteValueKindToAnchorKind_UnknownReturnsEmpty(t *testing.T) {
	for _, k := range []string{"", "unknown_kind", "FOO", "NEW_KIND_NOT_IN_TABLE"} {
		if got := concreteValueKindToAnchorKind(k); got != "" {
			t.Errorf("kind=%q must fall through to empty AnchorKind; got %q", k, got)
		}
	}
}

// TestConcreteValueKindToAnchorKind_ProjectsToValidAnchorKind
// guards: every known mapping must land on one of the 6 declared
// AnchorKind constants. Catches a future typo that would silently
// produce a non-canonical AnchorKind value.
func TestConcreteValueKindToAnchorKind_ProjectsToValidAnchorKind(t *testing.T) {
	valid := map[types.AnchorKind]bool{
		types.AnchorDefinition: true,
		types.AnchorCall:       true,
		types.AnchorCondition:  true,
		types.AnchorReturn:     true,
		types.AnchorAssignment: true,
		types.AnchorImport:     true,
	}
	for _, k := range []string{
		"returns", "errors", "calls", "binds", "conditional",
		"assigns", "maps", "config", "decorates", "embeds", "implements",
	} {
		got := concreteValueKindToAnchorKind(k)
		if !valid[got] {
			t.Errorf("kind=%q projected to non-canonical AnchorKind %q", k, got)
		}
	}
}

// TestConcreteValueProjection_ClaimFormOfNoLongerUnknown is the
// integration check: an EvidenceItem with the projected
// AnchorKind + Origin from concrete_values must NOT fall through
// to ClaimUnknown via ClaimFormOf. This is the load-bearing
// behaviour change Phase-0-trace's 34% ClaimUnknown rate
// motivated.
func TestConcreteValueProjection_ClaimFormOfNoLongerUnknown(t *testing.T) {
	for _, kind := range []string{
		"returns", "errors", "calls", "binds", "conditional",
		"assigns", "maps", "config", "decorates", "embeds", "implements",
	} {
		t.Run(kind, func(t *testing.T) {
			ev := types.EvidenceItem{
				Source:     "src/x.go",
				LineStart:  10,
				Producer:   "concrete_values",
				AnchorKind: concreteValueKindToAnchorKind(kind),
				Origin:     types.ClaimOriginCurrentRepo,
			}
			form := types.ClaimFormOf(ev)
			if form == types.ClaimUnknown {
				t.Errorf("kind=%q with projected AnchorKind=%q + Origin=current_repo still projects to ClaimUnknown — projection broken", kind, ev.AnchorKind)
			}
		})
	}
}
