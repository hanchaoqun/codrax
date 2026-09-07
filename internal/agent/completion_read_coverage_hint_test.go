package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestCurrentReadCoverage_AcceptedDispositionSharesCurrentView(t *testing.T) {
	mu := types.NewMutableState("q")
	ctx := &types.AgentContext{Mutable: mu}
	root := t.TempDir()
	closure := mu.EvidenceClosure()
	closure.AppendScopedReadCoverageCaveat(types.CompletionCaveat{Lane: types.DowngradeLaneForcedReadCoverage}, []types.ScopedReadCoverage{{RepositoryRoot: root, Path: "a.go"}})
	render := func() string {
		var b strings.Builder
		renderAcceptedInvestigationDisposition(&b, ctx, "resolved")
		return b.String()
	}
	if !strings.Contains(render(), "boundaries remain unproven") {
		t.Fatal("unread source must remain bounded")
	}
	closure.RecordScopedReadCoverage(root, types.ToolReadCoverage{Path: "a.go", LineStart: 1, LineEnd: 3, TotalLines: 3, RawRef: "read"})
	if got := render(); !strings.Contains(got, "result_kind: `resolved`") {
		t.Fatalf("settled read scope still degrades model hint: %s", got)
	}
	closure.AppendCompletionCaveat(types.CompletionCaveat{Lane: types.DowngradeLaneWakeupChainDrilldown})
	if !strings.Contains(render(), "boundaries remain unproven") {
		t.Fatal("must not retire independent trace debt")
	}
}
