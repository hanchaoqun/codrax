package orchestrator

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracefence"
	"github.com/hanchaoqun/codrax/internal/types"
)

// A truthful copy of the complete native residual-D label must not acquire
// the IO comparator from a word embedded in the label. The old soft checker
// still discloses a genuine excess over the disjoint D lane, not its D+IO fold.
func TestHMC323SharedDLabelKeepsDisjointProseComparator(t *testing.T) {
	for _, lang := range []struct {
		name string
		zh   bool
	}{{"zh", true}, {"en", false}} {
		for _, tc := range []struct {
			name, value string
			excess      bool
		}{{"faithful", "4.039", false}, {"excess", "5.000", true}} {
			t.Run(lang.name+"/"+tc.name, func(t *testing.T) {
				rec := p6AccountRecord("app-100", 100, 20, 30, 4.039, 0, 155.379, 155.379)
				rec.RichNotes = append(rec.RichNotes, types.TraceNoteKeyIOWait+"=1.340")
				mut := psgTraceMutable(rec)
				text := fmt.Sprintf("app-100 %s %sms.", tracefence.StateNonIODStateWord(lang.zh), tc.value)
				doc := psgProseDoc(text)
				claims := proseWallClockScanClaims(doc, []proseWallClockAccount{{subject: "app-100", tid: "100"}})
				if len(claims) != 1 || claims[0].dim != proseWallClockDimDStateNonIO {
					t.Errorf("complete native label must retain the residual-D comparator, not its embedded IO word or an unscanned claim: %+v", claims)
				}
				findings := proseWallClockConservationFindings(doc, psgBus(mut), mut)
				if !tc.excess {
					if len(findings) != 0 {
						t.Fatalf("faithful residual 4.039ms must not be compared to IO 1.340ms: %+v", findings)
					}
				} else if len(findings) != 1 || !strings.Contains(findings[0].userReadable("zh"), "4.039") || strings.Contains(findings[0].userReadable("zh"), "1.340") {
					t.Fatalf("genuine 5.000ms excess must disclose the 4.039ms residual comparator, not IO or the 5.379ms fold: %+v", findings)
				}
				if len(doc.Blocks) != 1 || doc.Blocks[0].Text != text {
					t.Fatal("soft disclosure must not rewrite the model-owned answer")
				}
			})
		}
	}
}

func TestHMC323SharedDLabelDoesNotOwnLaterStateClaim(t *testing.T) {
	for _, zh := range []bool{true, false} {
		text := fmt.Sprintf("app-100 %s 4.039ms and io_wait 2.200ms.", tracefence.StateNonIODStateWord(zh))
		claims := proseWallClockScanClaims(psgProseDoc(text), []proseWallClockAccount{{subject: "app-100", tid: "100"}})
		if len(claims) != 2 || claims[0].dim != proseWallClockDimDStateNonIO || claims[1].dim != proseWallClockDimIOWait {
			t.Fatalf("full-label binding must stop before a later explicit state claim (zh=%t): %+v", zh, claims)
		}
	}
}
