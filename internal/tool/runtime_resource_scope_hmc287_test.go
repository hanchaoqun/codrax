package tool

import (
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// A display projection must not contradict valid original-query statistics,
// even when all retained rows happen to match. Neither case proves that the
// query covered every event in the user's requested population.
func TestHMC287ProjectedResourceStatisticsRetainQueryScope(t *testing.T) {
	for _, end := range []float64{10.05, 10.2} {
		t.Run(strconv.FormatFloat(end, 'f', -1, 64), func(t *testing.T) {
			bus, result := resourceProjectionFixtureHMC286(t)
			bus.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile.TimeEnd = &end
			projection, ok := types.RuntimeResourceCurrentDisplayProjection(types.ObservationLedgerInputFromBusContext(bus, 0), result, resourceRecordHMC279(t, result))
			if !ok {
				t.Fatal("expected display projection")
			}
			want := 4
			if end == 10.2 {
				want = 5
			}
			if projection.ProjectedEvents != want {
				t.Fatalf("projected events=%d want=%d", projection.ProjectedEvents, want)
			}
			d := resourceDistributionHMC279(t, types.BuildAnswerSemanticViewForBusContext(bus))
			notes := strings.Join(d.Notes, "\n")
			if !strings.Contains(notes, "适用范围以原查询筛选条件为准") || strings.Contains(notes, "不能用作请求主体或窗口的合计") {
				t.Fatalf("statistics scope teaching contradicts original query: %s", notes)
			}
			if d.DefaultPresentation || d.MemberSet != nil || d.DisplayPopulation != nil || d.HasCompleteNativeDisplayPopulation() {
				t.Fatalf("scope wording minted population authority: %+v", d)
			}
		})
	}
}
