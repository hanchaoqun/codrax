package tracequery

import (
	"fmt"
	"strings"
	"testing"
)

func TestResourceAndPluginIdentityTuplesDoNotAlias(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse=%t", reverse), func(t *testing.T) {
			payloads := []string{
				"file_system: syscall=read/x path=/y dev=8,0 address=0x1 duration_ms=2 bytes=10",
				"file_system: syscall=read path=x//y dev=8,0 address=0x1 duration_ms=3 bytes=20",
				"xpower_cpu: domain=a/b event_name=c metric=CPU value=7 scene=foreground",
				"xpower_cpu: domain=a event_name=b/c metric=CPU value=7 scene=foreground",
				"xpower_cpu: domain=a event_name=b/c metric=CPU value=7 scene=background",
				"xpower_cpu: domain=a event_name=b/c metric=CPU value=7 scene=background",
			}
			if reverse {
				for i, j := 0, len(payloads)-1; i < j; i, j = i+1, j-1 {
					payloads[i], payloads[j] = payloads[j], payloads[i]
				}
			}
			var source strings.Builder
			for i, payload := range payloads {
				fmt.Fprintf(&source, "worker-20 (20) [001] .... 1.%06d: %s\n", i+1, payload)
			}
			idx := buildTraceIndex(t, "identity-tuples.ftrace", source.String())
			stats := ComputeWindowStats(idx, Query{TimeStart: 1, TimeEnd: 1.01})
			if len(stats.FilesystemResources) != 2 || len(stats.XPowerEvents) != 3 {
				t.Fatalf("source tuple boundaries collapsed: resources=%+v plugins=%+v", stats.FilesystemResources, stats.XPowerEvents)
			}
			var bytes int64
			var ms float64
			for _, item := range stats.FilesystemResources {
				bytes += item.Bytes
				ms += item.TotalLatencyMs
				if item.Count != 1 || item.Dev != "8,0" || item.Address != "0x1" {
					t.Fatalf("resource fields changed: %+v", item)
				}
			}
			if bytes != 30 || ms != 5 {
				t.Fatalf("split grouping changed original totals: %d bytes %f ms", bytes, ms)
			}
			if item := stats.XPowerEvents[0]; item.Domain != "a" || item.EventName != "b/c" || item.Category != "background" || item.Count != 2 {
				t.Fatalf("same-category repeated rows no longer aggregate independently: %+v", item)
			}
		})
	}
}
