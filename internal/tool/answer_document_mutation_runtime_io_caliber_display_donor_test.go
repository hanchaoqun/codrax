package tool

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Without source-backed identity, near values and matching coordinates do not
// prove duplicate publications. Keep both measurements and their own rulers.
// This is a display/Render test, not a native-query end-to-end counterexample.
func TestRuntimeIOCaliberDisplayUnknownIdentityKeepsMeasurements(t *testing.T) {
	for _, token := range []string{"io_wait", "io_burst_episode"} {
		for _, lang := range []string{"en", "zh"} {
			for _, tc := range []struct {
				name                        string
				first, second               float64
				firstCaliber, secondCaliber string
			}{
				{"unknown_seed_known_larger", 35, 35.1, "", types.TraceIOValueCaliberMixed},
				{"known_seed_unknown_larger", 35, 35.1, types.TraceIOValueCaliberMixed, ""},
				{"unknown_seed_exact_tie", 35, 35, "", types.TraceIOValueCaliberMixed},
				{"known_seed_exact_tie", 35, 35, types.TraceIOValueCaliberMixed, ""},
			} {
				t.Run(token+"/"+lang+"/"+tc.name, func(t *testing.T) {
					a := types.TraceCausalProjectionNode{EvidenceID: "first", Subject: "issuer-42", Object: "worker-99", TypeToken: token, StateKind: "io_wait", Role: types.TraceCausalRoleRootCauseContext, ChainRelevance: "adjacent", LineStart: 10, LineEnd: 20, ImpactMS: tc.first, CumulativeImpactMS: tc.first, IOValueCaliber: tc.firstCaliber}
					b := a
					b.EvidenceID, b.ImpactMS, b.CumulativeImpactMS, b.IOValueCaliber = "second", tc.second, tc.second, tc.secondCaliber
					projection := types.TraceCausalProjection{AdjacentCauses: []types.TraceCausalProjectionNode{a, b}}
					before, _ := json.Marshal(projection)
					model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), lang == "zh")
					if len(model.Adjacent) != 2 {
						t.Fatalf("unidentified measurements were merged: %+v", model.Adjacent)
					}
					blocks := runtimeTraceCausalProjectionCluster(projection, lang, runtimeTraceProjUserFocus{})
					doc := &types.AnswerDocumentV2{Blocks: blocks}
					text := render.RenderAnswerDocument(doc, lang)
					if !strings.Contains(text, "issuer-42") || !strings.Contains(text, "35.") || len(blocks) == 0 {
						t.Fatalf("public renderer did not publish the measurement: %s", text)
					}
					want := map[string]types.TraceCausalProjectionNode{a.EvidenceID: a, b.EvidenceID: b}
					for _, item := range model.Adjacent {
						node := item.Node
						original, ok := want[node.EvidenceID]
						if !ok || node.ImpactMS != original.ImpactMS || node.CumulativeImpactMS != original.CumulativeImpactMS || node.IOValueCaliber != original.IOValueCaliber || node.DuplicatePublications > 1 {
							t.Fatalf("independent value, ruler or identity changed: %+v", node)
						}
						delete(want, node.EvidenceID)
						row := runtimeTraceProjTreeRowLine(item, 100, 50, true, lang == "zh")
						if value := fmt.Sprintf("%.3f", original.ImpactMS); !strings.Contains(row, value) || !strings.Contains(text, value) {
							t.Errorf("published display lost member value %q: row=%s", value, row)
						}
						if original.IOValueCaliber != "" {
							label := types.TraceIOValueCaliberLabel(original.IOValueCaliber, lang == "zh")
							if !strings.Contains(row, label) || !strings.Contains(text, label) {
								t.Errorf("published display lost member label %q: row=%s", label, row)
							}
						}
					}
					after, _ := json.Marshal(projection)
					if string(before) != string(after) {
						t.Fatal("display mutated input projection")
					}
				})
			}
		}
	}
}
