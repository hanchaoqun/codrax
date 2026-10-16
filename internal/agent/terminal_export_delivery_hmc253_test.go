package agent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/preview"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracecatalog"
	"github.com/hanchaoqun/codrax/internal/types"
	"golang.org/x/net/html"
)

func pendingAnswerContextHMC253(t *testing.T) *types.AgentContext {
	t.Helper()
	ctx, _ := resourceSelectionPublicContextHMC248(t)
	ctx.DeliveryRoot = tracecatalog.NewManagedExportRoot(t.TempDir())
	ctx.PipelineExports = tracecatalog.NewPipelineExports(ctx.DeliveryRoot)
	c := ctx.Mutable.TraceCatalogs()[0]
	args, _ := json.Marshal(map[string]any{"action": "export", "catalog_id": c.ID(), "export": map[string]string{"target": "capture-review"}})
	r, err := (&tool.TraceCatalog{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
	if err != nil || !r.Success {
		t.Fatalf("accept request: %v %s", err, r.Summary)
	}
	return ctx
}

func TestHMC253FailedDeliveryTargetRendersOnlyLiteralMetadata(t *testing.T) {
	for _, lang := range []string{"en", "zh"} {
		for _, target := range []string{"[audit](report.md)", "![audit](report.md)", "<audit@example.test>", "`[audit](report.md)`", "&#91;audit](report.md)"} {
			t.Run(lang+"/"+target, func(t *testing.T) {
				ctx := pendingAnswerContextHMC253(t)
				c := ctx.Mutable.TraceCatalogs()[0]
				// These are legal package names. Only their presentation is
				// untrusted; validation must not silently narrow accepted names.
				args, _ := json.Marshal(map[string]any{"action": "export", "catalog_id": c.ID(), "export": map[string]string{"target": target}})
				r, err := (&tool.TraceCatalog{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), args)
				if err != nil || !r.Success {
					t.Fatalf("legal target rejected: %v %s", err, r.Summary)
				}
				cancelled, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Ctx = cancelled
				answer := FinalizeTraceCatalogExports(ctx, "Original model answer.", lang)
				rendered, err := preview.RenderMarkdownHTML([]byte(answer))
				if err != nil {
					t.Fatal(err)
				}
				z := html.NewTokenizer(strings.NewReader(rendered))
				var visible strings.Builder
				for tt := z.Next(); tt != html.ErrorToken; tt = z.Next() {
					switch tt {
					case html.TextToken:
						visible.WriteString(string(z.Text()))
					case html.StartTagToken, html.SelfClosingTagToken:
						for _, attr := range z.Token().Attr {
							if attr.Key == "href" || attr.Key == "src" {
								t.Errorf("failed target minted a rendered link: %s=%q", attr.Key, attr.Val)
							}
						}
					}
				}
				if !strings.Contains(visible.String(), target) {
					t.Errorf("target metadata did not remain literal: %s", visible.String())
				}
				requests, err := tool.AuthorizedPendingTraceCatalogExports(types.ToolBusContext(ctx, types.AgentFinalizer))
				if err != nil || len(requests) != 2 {
					t.Fatalf("private intent missing: %+v %v", requests, err)
				}
				found := false
				for _, request := range requests {
					if request.Target == target {
						found = request.Status == "cancelled" && request.Delivery == nil
					}
				}
				if !found {
					t.Fatal("display escaping changed target identity or minted delivery")
				}
			})
		}
	}
}

func TestHMC253DraftDeliveryAndTerminalOwnership(t *testing.T) {
	ctx := pendingAnswerContextHMC253(t)
	if got := renderAnswerExportDeliveryContext(ctx); !strings.Contains(got, `"target":"capture-review"`) || strings.Contains(got, `"physical_success":true`) {
		t.Fatalf("pending context: %s", got)
	}
	if got := renderAnswerExportDeliveries(ctx, "en"); got != "" {
		t.Fatalf("draft must leave delivery component to terminal owner: %s", got)
	}
	model := "Keep this interpretation exactly."
	final := FinalizeTraceCatalogExports(ctx, model, "en")
	if !strings.HasPrefix(final, model+"\n\n") || strings.Count(final, "### System delivery:") != 1 {
		t.Fatalf("terminal component: %s", final)
	}
	rows, err := tool.AuthorizedTraceCatalogDeliveries(types.ToolBusContext(ctx, types.AgentFinalizer))
	if err != nil || len(rows) != 1 || !rows[0].PackageValid || !rows[0].CurrentMatch || !strings.Contains(final, rows[0].Receipt.ManifestPath) {
		t.Fatalf("published rows: %+v %v", rows, err)
	}
	if after := renderAnswerExportDeliveries(ctx, "en"); after != "" {
		t.Fatal("a later draft renderer duplicated the terminal-owned component")
	}
}

func TestHMC253TerminalExportDoesNotMintMissingAuthority(t *testing.T) {
	for _, mode := range []string{"no_request", "cancelled", "revoked", "empty_answer"} {
		t.Run(mode, func(t *testing.T) {
			ctx := pendingAnswerContextHMC253(t)
			answer := "Original model answer."
			switch mode {
			case "no_request":
				ctx.PipelineExports = tracecatalog.NewPipelineExports(ctx.DeliveryRoot)
			case "cancelled":
				c, cancel := context.WithCancel(context.Background())
				cancel()
				ctx.Ctx = c
			case "revoked":
				ctx.RepoRoot = t.TempDir()
				ctx.UserPinnedFiles = nil
			case "empty_answer":
				answer = ""
			}
			got := FinalizeTraceCatalogExports(ctx, answer, "en")
			if !strings.HasPrefix(got, answer) || strings.Contains(got, "](<") || strings.Contains(got, "matches current registered execution") {
				t.Fatalf("authority failure minted delivery: %s", got)
			}
			if (mode == "no_request" || mode == "empty_answer") && got != answer {
				t.Fatal("no request/answer must be an exact no-op")
			}
		})
	}
}
