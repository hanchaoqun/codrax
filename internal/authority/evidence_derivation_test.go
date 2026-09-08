package authority

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDerivationCandidateCeilingSurvivesBackfill(t *testing.T) {
	m := types.NewMutableState("inspect a candidate")
	for _, preset := range []types.AuthorityCeiling{types.AuthorityUnknown, types.AuthorityFactual, types.AuthorityConditional, types.AuthorityHistorical, types.AuthorityIllustrative} {
		ev := types.EvidenceItem{Kind: types.EvidenceConcrete, Scope: types.ScopeLine, Source: "src/a.go", LineStart: 10,
			GroundingStatus: types.GroundingGrounded, DerivationCandidate: true, Origin: types.ClaimOriginCurrentRepo, Authority: preset}
		proj := ComputeForEvidence(ev, &types.BusContext{Mutable: m})
		if proj.Authority == types.AuthorityFactual || proj.Authority == types.AuthorityUnknown {
			t.Errorf("candidate projected factual: %+v", proj)
		}
		rows := BackfillEvidenceProjector()([]types.EvidenceItem{ev}, m)
		rows = BackfillEvidenceProjector()(rows, m)
		want := types.WeakerOf(preset, types.AuthorityConditional)
		if rows[0].Authority != want || !rows[0].DerivationCandidate || rows[0].Source != ev.Source {
			t.Errorf("idempotent backfill upgraded/lost candidate: %+v want %s", rows[0], want)
		}
		if strings.Count(rows[0].AuthorityReason, types.EvidenceDerivationBoundary(ev)) != 1 {
			t.Errorf("backfill omitted or repeated candidate boundary: %+v", rows[0])
		}
	}
	plain := types.EvidenceItem{Kind: types.EvidenceConcrete, Scope: types.ScopeLine, Source: "src/a.go", LineStart: 10, GroundingStatus: types.GroundingGrounded}
	if ComputeForEvidence(plain, &types.BusContext{Mutable: m}).Authority != types.AuthorityFactual {
		t.Fatal("ordinary precise lane changed")
	}
}
