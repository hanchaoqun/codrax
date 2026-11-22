package tool

import (
	"strings"
	"testing"
)

// HMC377 round 12 (R11O-1, owner ruling R-377-12 ⑦): the four-state running
// line counts semantic classes by family identity and reads the class wall
// for its maximum. A split own-work account (a8e2ee04f, R-377-11 ③) renders
// one class as two account parts — its own work outside the thread's chain
// waits and the part inside them; the line said 「共2类」 for that one class
// and its 最大类墙钟 read the own-work part's price instead of the class wall.
// Real witness: 105 元宝 后台销毁冷启动 #2, TID 58533 bindApplication
// [10834.099821,10834.620005): 「共2类,最大类墙钟 37.945ms」 against the class
// wall 40.085 (hmc377/fix11/merge/sweep/out/dio/merge/merge_w22.fence_zh.txt;
// raw class union hmc377/fix11/review-population/tools/rawstate.py).

// hmc377R12FourStateCapture is the round-11 split-family shape with the window
// fully covered by scheduler events, so the four-state partition is provable:
// main-100 verifies three classes in [10.000, 10.200) — A [10.010, 10.020)
// running, B [10.030, 10.050) with a 5 ms sleep [10.035, 10.040) that
// waker-150 ends (a chain wait inside the span), C [10.060, 10.065) running.
// The class wall (union) is 35 ms: own work 30 ms + wait part 5 ms.
func hmc377R12FourStateCapture(t *testing.T) string {
	t.Helper()
	var c hmc376Trace
	c.run(9.990000, "main", 100, 100, 1)
	c.run(9.990100, "waker", 150, 100, 2)
	c.span(10.010000, 10.020000, "main", 100, 100, 1, "VerifyClass com.example.A")
	c.span(10.030000, 10.050000, "main", 100, 100, 1, "VerifyClass com.example.B")
	c.sleep(10.035000, "main", 100, 100, 1)
	c.add(10.040000, "waker", 150, 100, 2, "sched_wakeup: comm=main pid=100 prio=120 target_cpu=001")
	c.run(10.040010, "main", 100, 100, 1)
	c.span(10.060000, 10.065000, "main", 100, 100, 1, "VerifyClass com.example.C")
	c.sleep(10.100000, "main", 100, 100, 1)
	c.add(10.150000, "waker", 150, 100, 2, "sched_wakeup: comm=main pid=100 prio=120 target_cpu=001")
	c.run(10.150010, "main", 100, 100, 1)
	c.sleep(10.190000, "main", 100, 100, 1)
	c.add(10.250000, "waker", 150, 100, 2, "sched_wakeup: comm=main pid=100 prio=120 target_cpu=001")
	c.run(10.250010, "main", 100, 100, 1)
	return c.write(t)
}

func hmc377R12FourStateRunningLine(t *testing.T, text, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), prefix) {
			return line
		}
	}
	t.Fatalf("no running line %q in:\n%s", prefix, text)
	return ""
}

func TestHMC377R12FourStateLineCountsClassesByFamily(t *testing.T) {
	faces := hmc377R11Query(t, hmc377R12FourStateCapture(t), 10.0, 10.2)
	t.Run("zh", func(t *testing.T) {
		line := hmc377R12FourStateRunningLine(t, faces.zh, "- running ")
		if !strings.Contains(line, "(运行∩语义span;共1类,最大类墙钟 35.000ms 见 ") {
			t.Fatalf("one class in two account parts is one class, its wall the class union 35.000:\n%s", line)
		}
	})
	t.Run("en", func(t *testing.T) {
		line := hmc377R12FourStateRunningLine(t, faces.en, "- running ")
		if !strings.Contains(line, "(running ∩ semantic spans; 1 classes, largest class wall 35.000ms, see ") {
			t.Fatalf("one class in two account parts is one class, its wall the class union 35.000:\n%s", line)
		}
	})
	t.Run("parts_stay_two_rows", func(t *testing.T) {
		// The count is a word fact; the tree still draws each part once.
		values := hmc377R11ClassValues(runtimeTraceProjSMR1AllRows(&faces.model), runtimeTraceProjNodeDisplayImpact)
		if len(values) != 2 || !hmc377R11Has(values, 30) || !hmc377R11Has(values, 5) {
			t.Fatalf("the two parts stay two rows (30 + 5): %v", values)
		}
	})
}

func TestHMC377R12WholeOriginalFourStateLineCountsClassesByFamily(t *testing.T) {
	path := hmc376Original(t, "CODRAX_HMC377_YUANBAO_BGKILL2", hmc377YuanbaoBgKill2Digest, hmc377YuanbaoBgKill2Size)
	faces := hmc377R11QueryPID(t, path, 58533, 10834.099821, 10834.620005)
	line := hmc377R12FourStateRunningLine(t, faces.zh, "- running ")
	// Raw: the class_verification spans of TID 58533 cover 40.085 ms in the
	// window (one class; the own-work part is priced 37.945 outside the
	// thread's chain waits).
	if !strings.Contains(line, "共1类,最大类墙钟 40.085ms 见 ") {
		t.Fatalf("the Yuanbao running line must count one class with its 40.085 ms wall:\n%s", line)
	}
}
