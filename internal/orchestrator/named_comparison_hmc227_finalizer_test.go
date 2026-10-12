package orchestrator

import (
	"fmt"
	"testing"
)

func TestHMC227ComparisonSharedWindowNaturalPublicRun(t *testing.T) {
	for _, policy := range []string{"nil", "default", "allow"} {
		for _, singles := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/singles_%t", policy, singles), func(t *testing.T) {
				hmc224PartialComparisonFinalizer(t, policy, singles, "shared")
			})
		}
	}
}

func TestHMC227ComparisonSharedWindowBothHealthyPublicRun(t *testing.T) {
	for _, singles := range []bool{false, true} {
		t.Run(fmt.Sprintf("singles_%t", singles), func(t *testing.T) {
			hmc224PartialComparisonFinalizer(t, "nil", singles, "shared_healthy")
		})
	}
}
