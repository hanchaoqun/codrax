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
	// constant. This pin concerns system teaching, never model answer prose.
	const beforeHash = "b1f03b587f5b72994525824b665b28c5490db3eed27c323aea0f88cd838de31e"
	if got := fmt.Sprintf("%x", sha256.Sum256([]byte(item.Body))); got != beforeHash {
		t.Fatalf("shared extraction changed the established skill wording: got %s, want %s", got, beforeHash)
	}
	if got := strings.Count(item.Body, types.TraceRepairDirectionValueTeaching); got != 1 {
		t.Fatalf("skill must consume the shared value teaching exactly once, got %d", got)
	}
	if !item.AppliesTo.RequiresTrace || len(item.OnViolation) != 0 {
		t.Fatal("sharing teaching must not introduce a hard repair/answer obligation")
	}
}
