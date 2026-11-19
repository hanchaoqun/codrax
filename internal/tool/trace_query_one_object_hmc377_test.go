package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// HMC377 R-377-4 (owner ruling on §376 A audit failure 1, a must item), whole
// original at the public trace_query entry. Raw (audit A): the waker 64176 is
// ready 45.378 ms inside the dependency window [11343.250704, 11343.297396)
// and 45.878 ms over the asked window; the lower-priority running on its CPU
// while it was ready is about 8.7 ms. It is on the analysed thread's chain
// (rank 1, 36.311 ms), so it is one object: its whole-window ready account is
// the chain seat's detail, never a separate time-adjacent "not proven on
// chain" candidate, and 45.878 ms is never named a same-core overlap.
func TestHMC377WholeOriginalHoneybirdChainThreadIsOneObject(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC365_HONEYBIRD", hmc377HoneybirdDigest, hmc377HoneybirdSize)
	result, native := hmc376QueryTarget(t, path, "root_cause_rank", map[string]any{"pid": 64156, "target_scope": "thread"}, 11343.248, 11343.310)
	if native.RootCauseRank == nil {
		t.Fatal("no rank board")
	}
	var published []string
	remainders := 0
	for _, item := range native.RootCauseRank.Items {
		if item.Thread.PID != 64176 {
			continue
		}
		published = append(published, hmc376RowLabel(item))
		switch {
		case item.Type == "priority_inversion_candidate":
		case item.Type == "runnable_wait" && item.ChainAnchorRemainderSeat && item.ChainRelevance == "adjacent":
			// Different time: the 0.500 ms of ready time outside the
			// dependency window (45.878 − 45.378), plain ready time.
			remainders++
			if !hmc376Near(item.RunnableMs, 0.500) || !hmc376Near(item.ChainAnchoredMs, 45.378) ||
				!hmc376Near(item.ChainAnchorFullMs, 45.878) || item.GatedRunnableMs != 0 {
				t.Fatalf("outside remainder must be the plain 0.500 ms ready time: %s", hmc376RowLabel(item))
			}
		default:
			t.Fatalf("64176 must be one object for the same time (its chain seat) plus at most its outside ready remainder: %v", published)
		}
	}
	if remainders > 1 || len(published) != 1+remainders {
		t.Fatalf("64176 rows: %v", published)
	}
	found := false
	for _, item := range native.RootCauseRank.AbsorbedItems {
		if item.Thread.PID == 64176 && item.Type == "priority_inversion_runnable_wait" {
			found = true
			if !hmc376Near(item.RunnableMs, 45.878) || !hmc376Near(item.EffectiveImpactMs, 8.408) ||
				item.ChainCredentialLaneDemoted {
				t.Fatalf("the window account keeps its values as the seat's detail, with no no-credential demotion: %s", hmc376RowLabel(item))
			}
		}
	}
	if !found {
		t.Fatal("the waker's window account must stay lossless on the absorbed carrier")
	}
	// Protected numbers.
	for _, item := range native.RootCauseRank.Items {
		switch {
		case item.Type == "priority_inversion_candidate" && item.Thread.PID == 64176:
			if !hmc376Near(item.EffectiveImpactMs, 36.311) || !hmc376Near(item.RunnableMs, 45.378) || item.Rank != 1 {
				t.Fatalf("protected chain seat moved: %s", hmc376RowLabel(item))
			}
		case item.Type == "class_verification" && item.Thread.PID == 64156:
			if !hmc376Near(item.EffectiveImpactMs, 3.868) {
				t.Fatalf("protected verification moved: %s", hmc376RowLabel(item))
			}
		case item.Type == "running" && item.Thread.PID == 64156:
			if !hmc376Near(item.EffectiveImpactMs, 2.600) {
				t.Fatalf("protected running moved: %s", hmc376RowLabel(item))
			}
		case item.Type == "priority_inversion_runnable_wait" && item.Thread.PID == 64156:
			if !hmc376Near(item.EffectiveImpactMs, 0.013) {
				t.Fatalf("protected target overlap moved: %s", hmc376RowLabel(item))
			}
		}
	}
	projection := hmc376Projection(result)
	for _, node := range append(append([]types.TraceCausalProjectionNode(nil), projection.AdjacentCauses...), projection.BackgroundCauses...) {
		if strings.HasSuffix(node.Subject, "-64176") && (node.TypeToken != "runnable_wait" || !node.ChainAnchorRemainderSeat) {
			t.Fatalf("the chain thread must not be a separate adjacent/background candidate for the same time: %+v", node)
		}
	}
	for _, lang := range []string{"zh", "en"} {
		var text strings.Builder
		for _, block := range runtimeTraceCausalProjectionCluster(projection, lang, runtimeTraceProjUserFocus{}) {
			text.WriteString(block.Text)
			text.WriteString("\n")
		}
		rendered := text.String()
		// HMC377 D3-R2 (reviewed pin): with one display name per thread the
		// analysed thread's own rows sit in its own stanza, so the coverage
		// sentence reads against its waiting time — raw: S 46.692 + R 0.090 =
		// 46.782 ms, 46.692 ms covered by the chain (base read 46.692 of the
		// 62 ms window, because a renamed label kept a target row outside its
		// stanza).
		if lang == "zh" && !strings.Contains(rendered, "关注线程等待(sleep/D-state/runnable) 46.782ms 中链路覆盖 46.692ms(100%),未覆盖 0.090ms(0%)") {
			t.Fatalf("the coverage sentence must read against the analysed thread's raw waiting time:\n%s", rendered)
		}
		for _, line := range strings.Split(rendered, "\n") {
			// 45.878 may appear only inside the remainder's account identity
			// (whole window = anchored + remainder), never as an object's value
			// or beside an overlap/inversion word.
			if strings.Contains(line, "45.878") && (!strings.Contains(line, "45.378") ||
				strings.Contains(line, "反转") || strings.Contains(line, "重叠") ||
				strings.Contains(strings.ToLower(line), "inversion") || strings.Contains(strings.ToLower(line), "overlap")) {
				t.Fatalf("%s: the waker's whole-window account must not render as its own object or as an overlap: %q", lang, line)
			}
			if strings.Contains(line, "0.090ms") && strings.Contains(line, "自身·") &&
				!strings.Contains(line, TraceRunnableInversionAccountWord(true)) {
				t.Fatalf("the target's whole ready time must wear the runnable account word: %q", line)
			}
		}
	}
}
