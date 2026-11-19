package tool

import (
	"strings"
	"testing"
)

// HMC377 E2-R4: a host-edge remainder (the thread's time outside every wait of
// the analysed thread that its own wakeup ended) is not a cause. The reader is
// told the analysed thread was not waiting for it there, and no face gives it
// a fix direction (Douyin reboot5: logd.writer-7067 729.859 ms ◇ "修向 调度供给"
// while the target waited for it only 0.072 ms).
func TestHMC377HostEdgeRemainderSaysTheTargetWasNotWaiting(t *testing.T) {
	projection := o3cStateEdgeProjection()
	projection.AdjacentCauses[0].FixDirection = "scheduling_supply"
	projection.OnChainCauses[1].FixDirection = "scheduling_supply"
	for _, zh := range []bool{true, false} {
		lang := "en"
		if zh {
			lang = "zh"
		}
		var text strings.Builder
		for _, block := range runtimeTraceCausalProjectionCluster(projection, lang, runtimeTraceProjUserFocus{}) {
			if len(block.Items) > 0 {
				continue // table blocks: their caption prose is not a row face
			}
			text.WriteString(block.Text)
			text.WriteString("\n")
		}
		joined := rspaFenceJoined(text.String())
		notWaiting := "目标此时并未在等待本线程,无链上凭证"
		if !zh {
			notWaiting = "the analysed thread was not waiting for this thread then; no chain credential"
		}
		if !strings.Contains(joined, notWaiting) {
			t.Fatalf("%s: the remainder must say the analysed thread was not waiting for it:\n%s", lang, joined)
		}
		inRemainder := false
		for _, line := range strings.Split(text.String(), "\n") {
			if trimmed := strings.TrimLeft(line, " │├└─"); trimmed != "" && !strings.HasPrefix(trimmed, "·") {
				// A row head: the remainder row is the 0.075 ms row of 61839.
				inRemainder = strings.Contains(line, "61839") && strings.Contains(line, "0.075ms")
			}
			if !inRemainder {
				continue
			}
			if strings.Contains(line, "修向") || strings.Contains(line, "fix-direction") ||
				strings.Contains(line, "·方向=") || strings.Contains(line, "· direction=") {
				t.Fatalf("%s: a host-edge remainder names no fix direction: %q", lang, line)
			}
		}
	}
}
