package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC254ManagedDeliveryContextSeparatesIntentFromTerminalState(t *testing.T) {
	ctx := pendingAnswerContextHMC253(t)
	c := ctx.Mutable.TraceCatalogs()[0]
	args, _ := json.Marshal(map[string]any{"action": "export", "catalog_id": c.ID(), "export": map[string]string{"target": "capture-review"}})
	actual, err := (&tool.TraceCatalog{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !actual.Success || !strings.Contains(actual.Summary, `"status":"pending_publication"`) {
		t.Fatalf("public acceptance receipt must remain honest: %v %+v", err, actual)
	}
	check := func() {
		t.Helper()
		prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
		start := strings.Index(prompt, "## Evidence-package delivery")
		if start < 0 {
			t.Fatal("real finalizer instruction lost accepted delivery")
		}
		section := strings.SplitN(prompt[start:], "\n## ", 2)[0]
		if !strings.Contains(section, `"target":"capture-review"`) || !strings.Contains(section, "terminal component") {
			t.Fatalf("intent/owner missing: %s", section)
		}
		for _, stale := range []string{"pending_publication", `"intent_id":`, `"catalog_id":`, `"execution_coverage":`, `"manifest_path":`, `"physical_success":`, `"status":"published"`} {
			if strings.Contains(section, stale) {
				t.Errorf("mutable delivery snapshot %q re-entered analysis context: %s", stale, section)
			}
		}
	}
	check()
	model := "Analysis remains unchanged."
	final := FinalizeTraceCatalogExports(ctx, model, "en")
	if !strings.HasPrefix(final, model+"\n\n") || !strings.Contains(final, "matches current registered execution") {
		t.Fatalf("terminal owner did not publish actual result: %s", final)
	}
	check() // A later instruction must not duplicate physical receipts either.
}

func TestHMC254DeliveryContextRetainsFailuresAndStandaloneReceipts(t *testing.T) {
	for _, mode := range []string{"cancelled", "revoked", "standalone"} {
		t.Run(mode, func(t *testing.T) {
			ctx := pendingAnswerContextHMC253(t)
			switch mode {
			case "cancelled":
				cancelled, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Ctx = cancelled
				FinalizeTraceCatalogExports(ctx, "Original analysis.", "en")
			case "revoked":
				ctx.RepoRoot, ctx.UserPinnedFiles = t.TempDir(), nil
			case "standalone":
				ctx.PipelineExports = nil
				ctx.DeliveryRoot = tracecatalog.NewManagedExportRoot(t.TempDir())
				c := ctx.Mutable.TraceCatalogs()[0]
				args, _ := json.Marshal(map[string]any{"action": "export", "catalog_id": c.ID(), "export": map[string]string{"target": "standalone-review"}})
				r, err := (&tool.TraceCatalog{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
				if err != nil || !r.Success {
					t.Fatalf("standalone public export: %v %+v", err, r)
				}
			}
			got := renderAnswerExportDeliveryContext(ctx)
			want := map[string]string{"cancelled": "publication_cancelled", "revoked": "source_authorization", "standalone": `"manifest_path":`}[mode]
			if !strings.Contains(got, want) {
				t.Fatalf("real delivery fact lost (%s): %s", mode, got)
			}
			if mode != "standalone" && strings.Contains(got, `"physical_success":true`) {
				t.Fatal("failure minted physical success")
			}
		})
	}
}
