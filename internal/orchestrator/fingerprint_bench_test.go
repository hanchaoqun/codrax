package orchestrator

import (
	"fmt"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Baseline for the "incrementalize closure fingerprint" backlog item:
// the fingerprint is built once per scheduler round from bounded
// inputs. If this measures in the microseconds the item is dropped.
func BenchmarkClosureFingerprintBuild(b *testing.B) {
	items := make([]types.EvidenceItem, 120)
	for i := range items {
		items[i] = types.EvidenceItem{ID: fmt.Sprintf("ev-%03d", i), Summary: "evidence summary text of plausible length for a real run"}
	}
	reads := make(map[string]bool, 60)
	for i := 0; i < 60; i++ {
		reads[fmt.Sprintf("internal/pkg%02d/file%02d.go", i%10, i)] = true
	}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = types.ClosureFingerprint{
			ReadSetHash:  types.HashFileSet(reads),
			EvidenceHash: hashEvidenceIDs(items),
			ChainTermSet: hashChainTerminals(items),
		}
	}
}
