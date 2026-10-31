package tool

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
)

// Independent literals for the HMC328 capability correction. Do not derive
// either side from current production teaching: only this existing-slot delta
// may be reversed before comparing a historical whole-description digest.
const traceBusinessTreeOwnerPriorityApproved = "Without an explicit physical thread, it inventories all emitters in begin-line order, not target-filtered or duration-ranked Top work. An explicit physical thread gives its measured longer inclusive accounts first display priority; other accounts retain begin-line order. Both paths keep the all-emitter inventory counts and omissions; display priority is not causal contribution, completeness or root identity."

const traceBusinessTreeOwnerPriorityPrior = "It inventories all emitters in begin-line order, not target-filtered or duration-ranked Top work."

const traceDescriptionBeforeBusinessTreeOwnerPrioritySHA = "9575f64e48d3f7de1022e4059b181028b1eb28d8c497835d94c6fb790e41be8e"

func reverseTraceBusinessTreeOwnerPriorityTeaching(description string) (string, bool) {
	if strings.Count(description, traceBusinessTreeOwnerPriorityApproved) != 1 || strings.Contains(description, traceBusinessTreeOwnerPriorityPrior) {
		return "", false
	}
	return strings.Replace(description, traceBusinessTreeOwnerPriorityApproved, traceBusinessTreeOwnerPriorityPrior, 1), true
}

func traceQueryDescriptionBeforeBusinessTreeOwnerPriorityEvolution(t *testing.T, description string) string {
	t.Helper()
	prior, ok := reverseTraceBusinessTreeOwnerPriorityTeaching(description)
	if !ok {
		t.Fatal("business-tree owner priority teaching must be the unique approved existing-slot correction")
	}
	return prior
}

func TestHMC329BusinessTreeOwnerPriorityOnlyEvolution(t *testing.T) {
	prior := traceQueryDescriptionBeforeBusinessTreeOwnerPriorityEvolution(t, (&TraceQuery{}).Description())
	// This is the complete canonical golden before HMC328, not a regenerated
	// current golden or an edited expected digest from a failed test.
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(prior))); got != traceDescriptionBeforeBusinessTreeOwnerPrioritySHA {
		t.Fatalf("Description changed outside the exact business-tree owner-priority correction: %s", got)
	}
}

func TestHMC329BusinessTreeOwnerPriorityEvolutionRejectsUnapprovedDeltas(t *testing.T) {
	description := (&TraceQuery{}).Description()
	for _, tc := range []struct {
		name        string
		description string
		projection  bool
	}{
		{"missing", strings.Replace(description, traceBusinessTreeOwnerPriorityApproved, "", 1), false},
		{"duplicate", description + " " + traceBusinessTreeOwnerPriorityApproved, false},
		{"mixed_prior", description + " " + traceBusinessTreeOwnerPriorityPrior, false},
		{"prior_only", strings.Replace(description, traceBusinessTreeOwnerPriorityApproved, traceBusinessTreeOwnerPriorityPrior, 1), false},
		{"changed_authority", strings.Replace(description, "display priority is not causal contribution", "display priority is causal contribution", 1), false},
		{"unrelated_change", "Unapproved extra dispatch prose. " + description, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prior, ok := reverseTraceBusinessTreeOwnerPriorityTeaching(tc.description)
			if ok != tc.projection {
				t.Fatalf("exact delta eligibility: got %v, want %v", ok, tc.projection)
			}
			if !ok {
				if prior != "" {
					t.Fatal("rejected delta returned a usable historical description")
				}
				return
			}
			// A valid local delta must not erase an unrelated change. Only the
			// full historical digest establishes equivalence outside that delta.
			if got := fmt.Sprintf("%x", sha256.Sum256([]byte(prior))); got == traceDescriptionBeforeBusinessTreeOwnerPrioritySHA {
				t.Fatal("reverse projection concealed an unapproved description change")
			}
		})
	}
}

func TestHMC329BusinessTreeOwnerPriorityTeachingSharedSurfaces(t *testing.T) {
	var schema struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal((&TraceQuery{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	for name, surface := range map[string]string{
		"description": (&TraceQuery{}).Description(),
		"view":        schema.Properties["view"].Description,
		"matrix":      skill.RenderTraceQueryViewMatrix(),
	} {
		t.Run(name, func(t *testing.T) {
			if strings.Count(surface, skill.TraceBusinessTreeTeaching) != 1 || strings.Count(surface, traceBusinessTreeOwnerPriorityApproved) != 1 || strings.Contains(surface, traceBusinessTreeOwnerPriorityPrior) {
				t.Fatal("shared surface lost the unique approved conditional display contract")
			}
		})
	}
}
