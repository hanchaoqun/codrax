package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestCurrentReadCoverage_AnswerAndReplayFollowCurrentTypedReads(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			mu := types.NewMutableState("q")
			closure := mu.EvidenceClosure()
			root := t.TempDir()
			closure.AppendScopedReadCoverageCaveat(types.CompletionCaveat{Lane: types.DowngradeLaneForcedReadCoverage}, []types.ScopedReadCoverage{{RepositoryRoot: root, Path: "a.go"}})
			o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, Language: lang}}
			const authored = "MODEL-OWNED: root A; bounded alternatives B."
			before := o.appendSystemCaveatsToAnswer(authored)
			if before == authored || !strings.HasPrefix(before, authored) {
				t.Fatalf("unread disclosure missing or model changed: %s", before)
			}
			closure.RecordScopedReadCoverage(root, types.ToolReadCoverage{Path: "a.go", LineStart: 1, LineEnd: 4, TotalLines: 4, RawRef: "real-read"})
			if got := o.appendSystemCaveatsToAnswer(authored); got != authored {
				t.Fatalf("resolved current note remains: %s", got)
			}
			if got := o.replayRegisteredAnswerCaveats(authored); got != authored {
				t.Fatalf("historical register resurrected stale note: %s", got)
			}
			if !closure.HasCompletionCaveat(types.DowngradeLaneForcedReadCoverage) {
				t.Fatal("must not reopen historical accepted scheduler boundary")
			}
			// A later source debt makes the typed replay entry current again.
			closure.AppendScopedReadCoverageCaveat(types.CompletionCaveat{Lane: types.DowngradeLaneForcedReadCoverage}, []types.ScopedReadCoverage{{RepositoryRoot: root, Path: "b.go"}})
			if got := o.replayRegisteredAnswerCaveats(authored); got != before {
				t.Fatalf("new source debt was lost or model rewritten: %s", got)
			}
		})
	}
}
