package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestParseExternalObservationPolicy_ChineseVerbatimQuote pins the exact
// customer trace_repl.log (2026-07-02) request shape: a Chinese negative
// boundary "不分析代码" glued into a longer sentence. A verbatim quote must
// anchor and keep the exclude; a translated/paraphrased quote must demote the
// exclude with an audit warning (never silently).
func TestParseExternalObservationPolicy_ChineseVerbatimQuote(t *testing.T) {
	raw := "你是Android手机性能问题分析专家, 完成以下丢帧原因分析, 分析东湖Trace:record_trace_20260605224432@3279-299954687.sys.ftrace里面这一帧Choreographer#doFrame 94410, 注意是Activity Resume后的首帧, 不分析代码"
	conf := 0.9

	t.Run("verbatim_quote_keeps_exclude", func(t *testing.T) {
		policy, errStr, warnings := parseExternalObservationPolicy(raw, &emitExternalObservationPolicyParam{
			CurrentSourceMode: "exclude",
			ExclusionKind:     "explicit_user_exclusion",
			SourceQuotes:      []string{"不分析代码"},
			Confidence:        &conf,
		})
		if errStr != "" {
			t.Fatalf("unexpected error: %s", errStr)
		}
		if policy == nil || !policy.ExcludesCurrentSource() {
			t.Fatalf("verbatim Chinese boundary quote must keep the exclude, got %+v (warnings=%v)", policy, warnings)
		}
		for _, w := range warnings {
			if strings.Contains(w, "exclude ignored") {
				t.Fatalf("anchored exclude must not be demoted: %v", warnings)
			}
		}
	})

	t.Run("paraphrased_quote_demotes_with_warning", func(t *testing.T) {
		policy, errStr, warnings := parseExternalObservationPolicy(raw, &emitExternalObservationPolicyParam{
			CurrentSourceMode: "exclude",
			ExclusionKind:     "explicit_user_exclusion",
			SourceQuotes:      []string{"不要分析源代码"},
			Confidence:        &conf,
		})
		if errStr != "" {
			t.Fatalf("unexpected error: %s", errStr)
		}
		if policy != nil && policy.CurrentSourceMode == types.ExternalObservationCurrentSourceExclude {
			t.Fatalf("paraphrased quote must demote the exclude, got %+v", policy)
		}
		joined := strings.Join(warnings, "\n")
		if !strings.Contains(joined, "not copied from the current request") ||
			!strings.Contains(joined, "exclude ignored because no source_quote survived") {
			t.Fatalf("demotion must leave an audit warning, got %v", warnings)
		}
	})
}
