package tool

import (
	"regexp"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 D2-R5: a blocking_span node with neither a typed contention payload
// nor a value basis carries no lock evidence, so no face — the family word,
// the row form, the node word — may name a lock (lock words come only from a
// parsed payload). A typed payload row keeps its lock form.
func TestHMC377TokenOnlyBlockingNodeNamesNoLock(t *testing.T) {
	node := types.TraceCausalProjectionNode{TypeToken: "blocking_span", Object: "blocking_span", Subject: "w-1", ImpactMS: 10}
	lockWord := regexp.MustCompile(`(?i)\block\b|锁`)
	for _, zh := range []bool{true, false} {
		family := runtimeTraceProjImpactFormFamilyWord(node, zh)
		word := TraceMeasurementNodeTypeDisplayLabel("blocking_span", node, zh)
		for _, face := range []string{family, word} {
			if lockWord.MatchString(face) {
				t.Fatalf("zh=%v: a token-only blocking node names a lock: family=%q word=%q", zh, family, word)
			}
		}
	}
	if form := runtimeTraceProjImpactFormForNode(node, ""); form != runtimeTraceProjImpactFormBlockingWait {
		t.Fatalf("a token-only blocking node wears the blocking-wait form, got %v", form)
	}
	typed := node
	typed.BlockingKind = "monitor_contention"
	if form := runtimeTraceProjImpactFormForNode(typed, ""); form != runtimeTraceProjImpactFormLock {
		t.Fatalf("a typed contention row keeps the lock form, got %v", form)
	}
}
