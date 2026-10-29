package tool

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The published windows are locators, not an independent arithmetic verdict.
// Start with the native public query and preserve its exact direction roster,
// including the two lawful scheduling members and the closed IO wait.
func TestHMC305PublicRepresentativeWindowsPreserveSubtotalScope(t *testing.T) {
	data, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			ctx, path := optimizationCaliberPublicQuery(t, string(data), 100, 1, 1.05)
			optimizationCaliberPublicRequest(ctx, path, lang, "app-main-100", 100, 1, 1.05, false)
			before, set := optimizationCaliberPublicSnapshot(t, ctx)
			if len(set.Projections) != 1 {
				t.Fatalf("one physical capture expected: %d", len(set.Projections))
			}
			sections := TraceAnswerDecisionDirectionSections(set.Projections[0])
			found := false
			for _, section := range sections {
				if section.Direction == "scheduling_supply" && section.Arithmetic == types.TraceAnswerDirectionArithmeticSubtotal {
					if len(section.Members) != 2 || len(section.MemberRefs) != 2 || section.SubtotalMS != 2 {
						t.Fatalf("native two-member 2ms permission changed: %+v", section)
					}
					found = true
				}
			}
			if !found {
				t.Fatalf("public fixture lost its legal scheduling subtotal: %+v", sections)
			}
			optimizationCaliberPublicPublish(t, ctx, path, lang, false)
			seen := false
			for _, block := range ctx.Mutable.AnswerDocumentV2().Blocks {
				if !strings.HasSuffix(block.ID, runtimeTraceCausalProjectionRepresentativeSuffix) {
					continue
				}
				seen = true
				for _, item := range block.Items {
					if len(item.Cells) != 4 {
						t.Fatalf("window row shape changed: %+v", item)
					}
					caliber := item.Cells[3]
					want := "本表不决定项目数值能否相加"
					if lang == "en" {
						want = "This table does not decide whether item values are additive"
					}
					if !strings.Contains(caliber, want) {
						t.Errorf("locator disclaimer overrides independent subtotal permission: %s", caliber)
					}
				}
			}
			if !seen {
				t.Fatal("public emission lost representative-window rows")
			}
			after, afterSet := optimizationCaliberPublicSnapshot(t, ctx)
			if before != after || !reflect.DeepEqual(set, afterSet) {
				t.Fatal("presentation changed native facts, query scopes or permissions")
			}
		})
	}
}

func TestHMC305WindowCaliberNeverMintsArithmetic(t *testing.T) {
	for _, kind := range []string{"disjoint", "overlap", "unknown_board", "other_query"} {
		t.Run(kind, func(t *testing.T) {
			p := elimv2DirectionBoardProjection()
			switch kind {
			case "overlap":
				for i := range p.RankedSeats {
					p.RankedSeats[i].StartTs, p.RankedSeats[i].EndTs = 1000.01, 1000.1
				}
			case "unknown_board":
				for i := range p.RankedSeats {
					p.RankedSeats[i].RankBoardParamsFingerprint = ""
				}
			case "other_query":
				for i := range p.RankedSeats {
					p.RankedSeats[i].RankQueryWindowEndTs += float64(i)
				}
			}
			before, _ := json.Marshal(p)
			authority := TraceAnswerDecisionDirectionSections(p)
			for _, zh := range []bool{true, false} {
				block := runtimeTraceCausalProjectionRepresentativeWindowsBlock(p, zh, "projection", "", nil, nil)
				if block == nil {
					t.Fatal("valid locator bounds must remain visible")
				}
				for _, row := range block.Items {
					for _, bad := range []string{"小计 9.000", "subtotal 9.000", "mutually_exclusive", "authorized_to_published_subtotal"} {
						if strings.Contains(row.Cells[3], bad) {
							t.Fatalf("window table invented its own arithmetic: %s", row.Cells[3])
						}
					}
				}
			}
			after, _ := json.Marshal(p)
			if string(before) != string(after) || !reflect.DeepEqual(authority, TraceAnswerDecisionDirectionSections(p)) {
				t.Fatal("locator wording changed direction authority")
			}
		})
	}
}
