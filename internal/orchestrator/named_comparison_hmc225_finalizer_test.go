package orchestrator

import (
	"fmt"
	"testing"
)

func TestHMC225UnboundedComparisonPublicRun(t *testing.T) {
	for _, scope := range []string{"unbounded", "mixed", "full_unanchored"} {
		for _, policy := range []string{"nil", "default", "allow"} {
			for _, singles := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/singles_%t", scope, policy, singles), func(t *testing.T) {
					hmc224PartialComparisonFinalizer(t, policy, singles, scope)
				})
			}
		}
	}
}
