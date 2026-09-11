package orchestrator

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
)

// Retiring the prose-ratio verdict must not retire the S4-3 cited comparison
// lane. Exercise its actual persist -> appendix path, not just a formatter.
func TestB193PublicPersistenceKeepsCitedFactReconciliation(t *testing.T) {
	for _, lang := range []string{"zh", "en"} {
		for _, prose := range []string{
			"状态估算 82.1 + 9.1 = 91.2ms。",
			"Measured work takes 7.500ms. In the 114.940ms window it is about 6.5%.",
		} {
			t.Run(lang+"/"+prose, func(t *testing.T) {
				o, shipped := typedReconciliationHarness(t, prose)
				o.busCtx.Language = lang
				typedRows := tool.RuntimeTraceReconciliationRows(o.busCtx)
				var accountTag string
				for _, row := range typedRows {
					if row.Kind == tool.RuntimeTraceReconciliationTargetState && row.Subject == "app-10" {
						accountTag = row.EvidenceTag
					}
				}
				if accountTag == "" {
					t.Fatalf("missing target-account evidence tag: %+v", typedRows)
				}
				visibleTag := false
				for _, block := range shipped.Blocks {
					if !tool.RuntimeTraceSystemBlock(block) {
						continue
					}
					for _, item := range block.Items {
						visibleTag = visibleTag || item.Label == accountTag
					}
				}
				if !visibleTag {
					t.Fatalf("target-account tag %s has no published evidence-roster item", accountTag)
				}
				before, _ := json.Marshal(shipped)
				lines := o.collectSystemCrossCheckFindings()
				after, _ := json.Marshal(o.busCtx.Mutable.AnswerDocumentV2())
				if string(before) != string(after) {
					t.Fatal("fact appendix mutated persisted answer")
				}
				prefix := "Reconciliation reference:"
				if lang == "zh" {
					prefix = "对账参考:"
				}
				found := false
				for _, line := range lines {
					if !strings.HasPrefix(line, prefix) {
						continue
					}
					found = true
					for _, want := range []string{"20.000ms", "30.000ms", "64.940ms", "114.940ms", "[" + accountTag + "]"} {
						if !strings.Contains(line, want) {
							t.Errorf("typed comparison missing %q: %s", want, line)
						}
					}
					for _, forbidden := range []string{"82.1", "9.1", "91.2", "7.500", "6.5%", "差值", "completeness=", "recomputes", "tolerance", "不符"} {
						if strings.Contains(line, forbidden) {
							t.Errorf("prose token or verdict copied into typed facts: %s", line)
						}
					}
				}
				if !found {
					t.Fatalf("cited fact comparison lost: %v", lines)
				}
			})
		}
	}
}
