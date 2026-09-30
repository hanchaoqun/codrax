package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func TestTraceAttachmentExtentSharesValidatedRowGrammar(t *testing.T) {
	row := tracewire.HiSysEvent{TimestampNS: 2070000008, Domain: tracewire.HiSysEventName{Status: "null_reference"}, Event: tracewire.HiSysEventName{Status: "null_reference"}, Contents: tracewire.HiSysEventContents{StorageClass: "null"}}
	line, err := tracewire.FormatHiSysEventObservation(row)
	if err != nil {
		t.Fatal(err)
	}
	first, last, fl, ll, ok := traceTimestampWindowFromTrace(line + "\nworker-7 (7) [001] .... 1.000000: sched_wakeup: comm=target pid=8 prio=120 target_cpu=001\n")
	if !ok || first != 1 || last != 2.070000008 || fl != 1 || ll != 2 {
		t.Fatalf("out-of-order extent: %v %v %d %d %t", first, last, fl, ll, ok)
	}
	for _, input := range []string{"untrusted prose 999.000: sched_wakeup: x", strings.Replace(line, "ts_ns=2070000008", "ts_ns=9990000000", 1), line + "\n" + strings.Repeat("x", 1024*1024+1)} {
		if _, _, _, _, ok := traceTimestampWindowFromTrace(input); ok {
			t.Fatal("invalid/truncated physical scan acquired complete extent")
		}
	}
}
