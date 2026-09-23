package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the full display safety-net and public Render entry over an already
// supplied projection. Ordinary TraceQuery -> Emit compiles the types-layer
// duplicate fold first; this is not a native-query end-to-end counterexample.
func TestRuntimeIOCaliberDisplayUnknownSeedNumericDonor(t *testing.T) {
	for _, token := range []string{"io_wait", "io_burst_episode"} {
		for _, lang := range []string{"en", "zh"} {
			for _, tc := range []struct {
				name                              string
				first, second                     float64
				firstCaliber, secondCaliber, want string
			}{
				{"unknown_seed_known_larger", 35, 35.1, "", types.TraceIOValueCaliberMixed, types.TraceIOValueCaliberMixed},
				{"known_seed_unknown_larger", 35, 35.1, types.TraceIOValueCaliberMixed, "", ""},
				{"unknown_seed_exact_tie", 35, 35, "", types.TraceIOValueCaliberMixed, ""},
				{"known_seed_exact_tie", 35, 35, types.TraceIOValueCaliberMixed, "", types.TraceIOValueCaliberMixed},
			} {
				t.Run(token+"/"+lang+"/"+tc.name, func(t *testing.T) {
					a := types.TraceCausalProjectionNode{EvidenceID: "first", Subject: "issuer-42", Object: "worker-99", TypeToken: token, StateKind: "io_wait", Role: types.TraceCausalRoleRootCauseContext, ChainRelevance: "adjacent", LineStart: 10, LineEnd: 20, ImpactMS: tc.first, CumulativeImpactMS: tc.first, IOValueCaliber: tc.firstCaliber}
					b := a
					b.EvidenceID, b.ImpactMS, b.CumulativeImpactMS, b.IOValueCaliber = "second", tc.second, tc.second, tc.secondCaliber
					projection := types.TraceCausalProjection{AdjacentCauses: []types.TraceCausalProjectionNode{a, b}}
					before, _ := json.Marshal(projection)
					model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), lang == "zh")
					if len(model.Adjacent) != 1 {
						t.Fatalf("existing display fold stopped admitting the pair: %+v", model.Adjacent)
					}
					node := model.Adjacent[0].Node
					if node.ImpactMS != max(tc.first, tc.second) || node.CumulativeImpactMS != max(tc.first, tc.second) || node.DuplicatePublications != 2 {
						t.Fatalf("value or publication count changed: %+v", node)
					}
					if node.IOValueCaliber != tc.want {
						t.Errorf("display numeric donor lost its own ruler: got %q want %q", node.IOValueCaliber, tc.want)
					}
					blocks := runtimeTraceCausalProjectionCluster(projection, lang, runtimeTraceProjUserFocus{})
					doc := &types.AnswerDocumentV2{Blocks: blocks}
					text := render.RenderAnswerDocument(doc, lang)
					if !strings.Contains(text, "issuer-42") || !strings.Contains(text, "35.") || len(blocks) == 0 {
						t.Fatalf("public renderer did not publish the measurement: %s", text)
					}
					if tc.want != "" {
						label := types.TraceIOValueCaliberLabel(tc.want, lang == "zh")
						row := runtimeTraceProjTreeRowLine(model.Adjacent[0], 100, 50, true, lang == "zh")
						if !strings.Contains(row, label) || !strings.Contains(text, label) {
							t.Errorf("published display lost donor label %q: row=%s", label, row)
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
