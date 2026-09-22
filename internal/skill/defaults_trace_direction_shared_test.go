package skill

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1590aTraceDirectionSharedExtractionPreservesSkillBytes(t *testing.T) {
	item := finBindTierBItem(t, "TYPED WORD-FACE CONSUMPTION")
	// Preserve the original byte pins after undoing only the reviewed
	// 2026-09-22 estimate/realized-benefit clarification and the earlier
	// locator qualification. This tests teaching, never model answer prose.
	const beforeHash = "b1f03b587f5b72994525824b665b28c5490db3eed27c323aea0f88cd838de31e"
	const afterHash = "68d5e164ebfe68509662de558c37168d22492e093b48b984e087c61b54762865"
	if strings.Count(item.Body, tracefence.OptimizationMeaningEN) != 1 {
		t.Fatal("shared optimization meaning must be present exactly once")
	}
	legacy := strings.Replace(item.Body, " "+tracefence.OptimizationMeaningEN, "", 1)
	legacy = strings.NewReplacer(
		"adjacent-channel rows stay conditional within-model potential bounds", "adjacent-channel rows stay conditional upper bounds",
		"a demoted or adjacent row is a within-model potential bound conditional on a causal relation, never a proven cause or a guaranteed repair saving", "a demoted or adjacent row is a conditional upper bound ('at most', 'if causally linked'), never a proven cause",
		"rule-priced modeled-potential board AND raw time occupancy", "rule-priced eliminable board AND raw time occupancy",
	).Replace(legacy)
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(legacy))); got != afterHash {
		t.Fatalf("estimate clarification changed unrelated established teaching: got %s, want %s", got, afterHash)
	}
	clause := types.TraceDirectionEnvelopeOverlapTeaching + " "
	if strings.Count(item.Body, clause) != 1 {
		t.Fatal("locator qualification must be present exactly once")
	}
	before := strings.Replace(legacy, clause, "", 1)
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(before))); got != beforeHash {
		t.Fatalf("locator qualification changed unrelated established teaching: got %s, want %s", got, beforeHash)
	}
	if got := strings.Count(item.Body, types.TraceRepairDirectionValueTeaching); got != 1 {
		t.Fatalf("skill must consume the shared value teaching exactly once, got %d", got)
	}
	if !item.AppliesTo.RequiresTrace || len(item.OnViolation) != 0 {
		t.Fatal("sharing teaching must not introduce a hard repair/answer obligation")
	}
}
