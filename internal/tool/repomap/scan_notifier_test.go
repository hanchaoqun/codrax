package repomap

import (
	"testing"

	ctypes "github.com/hanchaoqun/codrax/internal/types"
)

func TestRepoMapScanProgressThrottlesChangeScanEvents(t *testing.T) {
	var events []ctypes.RepoMapScanEvent
	SetScanNotifier(func(ev ctypes.RepoMapScanEvent) {
		if ev.Progress {
			events = append(events, ev)
		}
	})
	defer SetScanNotifier(nil)

	progress := newRepoMapScanProgress("/repo", "", 100000, 0)
	progress.startPhase(ctypes.RepoMapScanPhaseChangeScan, 0)
	for i := 1; i < 10000; i++ {
		progress.parsed(i, 100000)
	}
	if len(events) != 0 {
		t.Fatalf("change scan emitted before 10000-file boundary: %d event(s)", len(events))
	}

	progress.parsed(10000, 100000)
	if len(events) != 1 || events[0].ParsedFiles != 10000 {
		t.Fatalf("change scan first event = (%d,%d), want one event at 10000", len(events), firstParsed(events))
	}

	for i := 10001; i < 20000; i++ {
		progress.parsed(i, 100000)
	}
	if len(events) != 1 {
		t.Fatalf("change scan emitted too frequently before next boundary: %d event(s)", len(events))
	}
}

func TestRepoMapScanProgressThrottlesCacheWriteEvents(t *testing.T) {
	var events []ctypes.RepoMapScanEvent
	SetScanNotifier(func(ev ctypes.RepoMapScanEvent) {
		if ev.Progress && ev.BytesWritten > 0 {
			events = append(events, ev)
		}
	})
	defer SetScanNotifier(nil)

	progress := newRepoMapScanProgress("/repo", ctypes.RepoMapScanFull, 10, 10)
	progress.startPhase(ctypes.RepoMapScanPhaseCacheWrite, 10)
	progress.cacheWriteFile("relations.md", 4*1024, 128*1024*1024)
	progress.cacheWriteFile("relations.md", 5*1024*1024, 128*1024*1024)
	if len(events) != 1 {
		t.Fatalf("cache write emitted before byte threshold: %d event(s)", len(events))
	}

	progress.cacheWriteFile("relations.md", repoMapCacheWriteEmitBytes+4*1024, 128*1024*1024)
	if len(events) != 2 {
		t.Fatalf("cache write did not emit at byte threshold: %d event(s)", len(events))
	}
}

func firstParsed(events []ctypes.RepoMapScanEvent) int {
	if len(events) == 0 {
		return 0
	}
	return events[0].ParsedFiles
}
