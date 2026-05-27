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

func TestRepoMapScanProgressReportsFileDiscovery(t *testing.T) {
	var events []ctypes.RepoMapScanEvent
	SetScanNotifier(func(ev ctypes.RepoMapScanEvent) {
		if ev.Progress {
			events = append(events, ev)
		}
	})
	defer SetScanNotifier(nil)

	progress := newRepoMapScanProgress("/repo", "", 0, 0)
	progress.startPhase(ctypes.RepoMapScanPhaseFileScan, 0)
	progress.filesScanned(1000, 600, "src/a.go")
	if len(events) != 1 {
		t.Fatalf("file discovery should emit at 1000-file boundary, got %d event(s)", len(events))
	}
	if events[0].TotalFiles != 1000 || events[0].ParseableFiles != 600 || events[0].ParsedFiles != 1000 || events[0].CurrentFile != "src/a.go" {
		t.Fatalf("unexpected file discovery event: %+v", events[0])
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

func TestRepoMapScanProgressReportsCacheLoadRecords(t *testing.T) {
	var events []ctypes.RepoMapScanEvent
	SetScanNotifier(func(ev ctypes.RepoMapScanEvent) {
		if ev.Progress && ev.Phase == ctypes.RepoMapScanPhaseCacheLoad {
			events = append(events, ev)
		}
	})
	defer SetScanNotifier(nil)

	progress := newRepoMapScanProgress("/repo", ctypes.RepoMapScanCacheHit, 93468, 0)
	progress.startPhase(ctypes.RepoMapScanPhaseCacheLoad, 63891)
	progress.cacheLoaded(5120, 93468, 5, 92, "chunk-00004.json")
	if len(events) != 1 {
		t.Fatalf("cache load progress should emit visible record counts, got %d event(s)", len(events))
	}
	got := events[0]
	if got.CacheRecordsLoaded != 5120 || got.CacheRecordsTotal != 93468 ||
		got.CacheChunksLoaded != 5 || got.CacheChunksTotal != 92 ||
		got.CurrentFile != "chunk-00004.json" {
		t.Fatalf("unexpected cache load event: %+v", got)
	}
}

func TestRepoMapScanProgressReportsViewRenderStep(t *testing.T) {
	var events []ctypes.RepoMapScanEvent
	SetScanNotifier(func(ev ctypes.RepoMapScanEvent) {
		if ev.Progress && ev.Phase == ctypes.RepoMapScanPhaseViewRender {
			events = append(events, ev)
		}
	})
	defer SetScanNotifier(nil)

	progress := newRepoMapScanProgress("/repo", ctypes.RepoMapScanCacheHit, 90000, 0)
	progress.startPhase(ctypes.RepoMapScanPhaseViewRender, 64000)
	progress.viewRendered("bridges", 3, 3)
	if len(events) != 1 {
		t.Fatalf("view render progress should emit a step event, got %d event(s)", len(events))
	}
	got := events[0]
	if got.ViewStep != "bridges" || got.ViewStepsDone != 3 || got.ViewStepsTotal != 3 {
		t.Fatalf("unexpected view render event: %+v", got)
	}
}

func firstParsed(events []ctypes.RepoMapScanEvent) int {
	if len(events) == 0 {
		return 0
	}
	return events[0].ParsedFiles
}
