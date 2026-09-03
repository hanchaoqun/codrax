package agent

import (
	"os"
	"strings"
	"testing"
)

// QUALGATE-1 (§40.30) wiring tripwire: the sidecar contract must build its
// seat authority through the ONE gated provider and hand exactly that
// authority to the compiler — bypassing the gate on this face while the
// crown face stays gated would re-open the two-face split.
func TestTraceFindingContractBuildsSeatAuthorityThroughTheGatedProvider(t *testing.T) {
	src, err := os.ReadFile("trace_finding_contract.go")
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	for _, want := range []string{
		"tracefinding.BuildSeatFrameCausalityAuthority(seatInput)",
		"tracefinding.CompileCandidateContract(ledger, set, seatAuthority)",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("trace_finding_contract.go must wire the gated provider verbatim (missing %q)", want)
		}
	}
	for _, forbidden := range []string{"BuildSeatFrameCausalityIndex(", "Applicable: true"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("trace_finding_contract.go must not build or force an ungated index (%q found)", forbidden)
		}
	}
}
