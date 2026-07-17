package skill

// defaults_c4_inversion_family_test.go — C4 两 token=一族两通道教学 pin
// (RANKDIS-SWEEP M7, §29.104.16.1, docs/design/rankdis_sweep_20260716.md 编队
// C4, 2026-07-17): the TRACE PRIORITY-INVERSION AUTHORITY clause teaches that
// the two inversion row-type tokens are ONE family on two measurement
// channels (candidate = the on-chain gated composite seat; runnable_wait =
// the runnable-overlap occurrence row), and that a tier=absorbed rank row
// holds no ranking seat. Substance pin — the jargon sweep runs separately
// (TestNoInternalTermsInPrompts).

import (
	"strings"
	"testing"
)

func TestC4InversionFamilyTwoChannelTeaching(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)
	sk, err := r.Get("explore-skill")
	if err != nil {
		t.Fatalf("Get(explore-skill) returned error: %v", err)
	}
	var body string
	for _, item := range sk.WorkflowTierB {
		if strings.HasPrefix(item.Body, "TRACE PRIORITY-INVERSION AUTHORITY") {
			body = item.Body
			if !item.AppliesTo.RequiresTrace {
				t.Fatalf("the inversion authority rule is trace-gated; AppliesTo=%+v", item.AppliesTo)
			}
		}
	}
	if body == "" {
		t.Fatalf("explore-skill WorkflowTierB missing the TRACE PRIORITY-INVERSION AUTHORITY item")
	}
	for _, want := range []string{
		"ONE priority-inversion family measured on two channels",
		"marks the on-chain gated composite seat",
		"marks the same-CPU runnable-overlap occurrence row",
		"never as two independent competing causes",
		"`tier` is `absorbed`",
		"absorbed_by_rank_family=true",
		"counts inside the family row its `absorbed_into` key names",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("two-channel family teaching missing %q:\n%s", want, body)
		}
	}
}
