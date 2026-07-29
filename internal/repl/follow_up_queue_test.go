package repl

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// PIB-2 v1 (ledger docs/design/pi_borrow_analysis_20260729.md §7.6):
// non-TTY input typed during a Run queues as follow-up turns instead
// of being warn-dropped; /cancel keeps its cancellation semantics.

type queueTestCanceller struct{ cancelled bool }

func (c *queueTestCanceller) Cancel(reason string) { c.cancelled = true }
func (c *queueTestCanceller) IsCanceled() bool     { return c.cancelled }

func TestCancelListener_QueuesFollowUpsAndKeepsCancel(t *testing.T) {
	in := strings.NewReader("再看下 renderer 的水位段\n/cancel\nafter-cancel is not read\n")
	var warned []string
	cl := startCancelListener(in, &queueTestCanceller{}, func(f string, a ...any) {
		warned = append(warned, fmt.Sprintf(f, a...))
	})
	if cl == nil {
		t.Fatal("injected reader must start the listener")
	}
	deadline := time.Now().Add(2 * time.Second)
	for len(cl.queuedLines()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	queued := cl.queuedLines()
	if len(queued) != 1 || queued[0] != "再看下 renderer 的水位段" {
		t.Fatalf("queued = %v, want the single pre-cancel line", queued)
	}
	if len(warned) == 0 || !strings.Contains(warned[0], "queued as follow-up") {
		t.Errorf("queueing must be disclosed via the one-shot warn; got %v", warned)
	}
	cl.stop()
}

func TestCancelListener_QueueCapDisclosed(t *testing.T) {
	var b strings.Builder
	for i := 0; i < cancelListenerQueueCap+5; i++ {
		fmt.Fprintf(&b, "line-%d\n", i)
	}
	var warned []string
	cl := startCancelListener(strings.NewReader(b.String()), &queueTestCanceller{}, func(f string, a ...any) {
		warned = append(warned, fmt.Sprintf(f, a...))
	})
	deadline := time.Now().Add(2 * time.Second)
	for len(cl.queuedLines()) < cancelListenerQueueCap && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := len(cl.queuedLines()); got != cancelListenerQueueCap {
		t.Fatalf("queue len = %d, want the cap %d", got, cancelListenerQueueCap)
	}
	cl.stop()
}
