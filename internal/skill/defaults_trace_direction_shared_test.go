package skill

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestB1590aTraceDirectionSharedExtractionPreservesSkillBytes(t *testing.T) {
	item := finBindTierBItem(t, "TYPED WORD-FACE CONSUMPTION")
	// Frozen before extracting the existing B1573 words into the shared
	// constant. 2026-09-20: the shared locator-envelope qualification is the
	// only deliberate addition. Pin both the new complete body and the exact
	// old body after removing that one clause; all other teaching must remain
	// byte-identical. This concerns system teaching, never model answer prose.
	const beforeHash = "b1f03b587f5b72994525824b665b28c5490db3eed27c323aea0f88cd838de31e"
	const afterHash = "68d5e164ebfe68509662de558c37168d22492e093b48b984e087c61b54762865"
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(item.Body))); got != afterHash {
		t.Fatalf("shared teaching changed beyond the reviewed locator qualification: got %s, want %s", got, afterHash)
	}
	clause := types.TraceDirectionEnvelopeOverlapTeaching + " "
	if strings.Count(item.Body, clause) != 1 {
		t.Fatal("locator qualification must be present exactly once")
	}
	before := strings.Replace(item.Body, clause, "", 1)
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
