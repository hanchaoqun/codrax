package repomap

import (
	"sync"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	ctypes "github.com/hanchaoqun/codrax/internal/types"
)

type scanNotifier func(ctypes.RepoMapScanEvent)

var scanNotifierState struct {
	mu sync.RWMutex
	fn scanNotifier
}

// SetScanNotifier installs the process-wide repomap scan progress
// sink. The callback must be non-blocking; it is called from the
// repomap build path while the user is waiting for local CPU work.
func SetScanNotifier(fn func(ctypes.RepoMapScanEvent)) {
	scanNotifierState.mu.Lock()
	defer scanNotifierState.mu.Unlock()
	scanNotifierState.fn = fn
}

func notifyRepoMapScan(ev ctypes.RepoMapScanEvent) {
	scanNotifierState.mu.RLock()
	fn := scanNotifierState.fn
	scanNotifierState.mu.RUnlock()
	if fn != nil {
		fn(ev)
	}
}

type repoMapScanProgress struct {
	repoRoot string
	mode     ctypes.RepoMapScanMode

	totalFiles     int
	parseableFiles int
	changedFiles   int
	phase          ctypes.RepoMapScanPhase

	started  bool
	start    time.Time
	lastEmit time.Time
	lastDone int

	lastCacheFile      string
	lastCacheBytes     int64
	lastCacheEmit      time.Time
	lastCacheStarted   time.Time
	cacheRecordsLoaded int
	cacheRecordsTotal  int
	cacheChunksLoaded  int
	cacheChunksTotal   int
	viewStep           string
	viewStepsDone      int
	viewStepsTotal     int
}

const (
	repoMapCacheWriteEmitBytes = 32 * 1024 * 1024
	repoMapCacheWriteEmitAfter = 5 * time.Second
)

func newRepoMapScanProgress(repoRoot string, mode ctypes.RepoMapScanMode, totalFiles, changedFiles int) *repoMapScanProgress {
	return &repoMapScanProgress{
		repoRoot:     repoRoot,
		mode:         mode,
		totalFiles:   totalFiles,
		changedFiles: changedFiles,
	}
}

func (p *repoMapScanProgress) startScan(parseableFiles int) {
	p.startPhase(ctypes.RepoMapScanPhaseParse, parseableFiles)
}

func (p *repoMapScanProgress) startPhase(phase ctypes.RepoMapScanPhase, parseableFiles int) {
	if p == nil {
		return
	}
	p.parseableFiles = parseableFiles
	p.phase = phase
	p.started = true
	p.start = time.Now()
	p.lastEmit = p.start
	notifyRepoMapScan(p.event(false, false, 0, true, ""))
}

func (p *repoMapScanProgress) parsed(done, total int) {
	if p == nil || !p.started {
		return
	}
	if total > 0 {
		p.parseableFiles = total
	}
	if done >= total {
		p.lastDone = done
		return
	}
	now := time.Now()
	minDelta := 1000
	minInterval := 2 * time.Second
	if p.phase == ctypes.RepoMapScanPhaseChangeScan {
		minDelta = 10000
		minInterval = 5 * time.Second
	}
	if done-p.lastDone < minDelta && now.Sub(p.lastEmit) < minInterval {
		return
	}
	p.lastDone = done
	p.lastEmit = now
	notifyRepoMapScan(p.event(true, false, done, true, ""))
}

func (p *repoMapScanProgress) filesScanned(done, parseable int, current string) {
	if p == nil || !p.started || p.phase != ctypes.RepoMapScanPhaseFileScan {
		return
	}
	if done > p.totalFiles {
		p.totalFiles = done
	}
	if parseable > p.parseableFiles {
		p.parseableFiles = parseable
	}
	now := time.Now()
	minDelta := 1000
	minInterval := 2 * time.Second
	if done-p.lastDone < minDelta && now.Sub(p.lastEmit) < minInterval {
		return
	}
	p.lastDone = done
	p.lastEmit = now
	ev := p.event(true, false, done, true, "")
	ev.CurrentFile = current
	notifyRepoMapScan(ev)
}

func (p *repoMapScanProgress) cacheLoaded(recordsLoaded, recordsTotal, chunksLoaded, chunksTotal int, current string) {
	if p == nil || !p.started || p.phase != ctypes.RepoMapScanPhaseCacheLoad {
		return
	}
	if recordsTotal > 0 {
		p.totalFiles = recordsTotal
	}
	now := time.Now()
	final := recordsTotal > 0 && recordsLoaded >= recordsTotal
	minDelta := 5000
	minInterval := 2 * time.Second
	if !final && recordsLoaded-p.cacheRecordsLoaded < minDelta && now.Sub(p.lastEmit) < minInterval {
		return
	}
	p.lastDone = recordsLoaded
	p.lastEmit = now
	p.cacheRecordsLoaded = recordsLoaded
	p.cacheRecordsTotal = recordsTotal
	p.cacheChunksLoaded = chunksLoaded
	p.cacheChunksTotal = chunksTotal
	ev := p.event(true, false, recordsLoaded, true, "")
	ev.CurrentFile = current
	ev.CacheRecordsLoaded = recordsLoaded
	ev.CacheRecordsTotal = recordsTotal
	ev.CacheChunksLoaded = chunksLoaded
	ev.CacheChunksTotal = chunksTotal
	notifyRepoMapScan(ev)
}

func (p *repoMapScanProgress) viewRendered(step string, done, total int) {
	if p == nil || !p.started || p.phase != ctypes.RepoMapScanPhaseViewRender {
		return
	}
	if done < 0 {
		done = 0
	}
	if total < done {
		total = done
	}
	p.lastDone = done
	p.lastEmit = time.Now()
	p.viewStep = step
	p.viewStepsDone = done
	p.viewStepsTotal = total
	ev := p.event(true, false, p.parseableFiles, true, "")
	ev.ViewStep = step
	ev.ViewStepsDone = done
	ev.ViewStepsTotal = total
	notifyRepoMapScan(ev)
}

func (p *repoMapScanProgress) activeFile(path string) {
	if p == nil || !p.started || path == "" {
		return
	}
	now := time.Now()
	if now.Sub(p.lastEmit) < 2*time.Second {
		return
	}
	p.lastEmit = now
	ev := p.event(true, false, p.lastDone, true, "")
	ev.CurrentFile = path
	notifyRepoMapScan(ev)
}

func (p *repoMapScanProgress) setPhase(phase ctypes.RepoMapScanPhase) {
	if p == nil || !p.started || phase == "" {
		return
	}
	p.phase = phase
	p.lastDone = p.parseableFiles
	p.lastEmit = time.Now()
	logging.Info("repo_map: phase %s (%d source files, %d files total, elapsed=%dms)",
		phase, p.parseableFiles, p.totalFiles, time.Since(p.start).Milliseconds())
	notifyRepoMapScan(p.event(true, false, p.parseableFiles, true, ""))
}

func (p *repoMapScanProgress) cacheWriteFile(path string, written, total int64) {
	if p == nil || !p.started || p.phase != ctypes.RepoMapScanPhaseCacheWrite {
		return
	}
	now := time.Now()
	final := total > 0 && written >= total
	if path == p.lastCacheFile && !final &&
		written-p.lastCacheBytes < repoMapCacheWriteEmitBytes &&
		now.Sub(p.lastCacheEmit) < repoMapCacheWriteEmitAfter {
		return
	}
	if path != p.lastCacheFile {
		p.lastCacheStarted = now
	}
	p.lastEmit = now
	p.lastCacheEmit = now
	p.lastCacheFile = path
	p.lastCacheBytes = written
	ev := p.event(true, false, p.parseableFiles, true, "")
	ev.CurrentFile = path
	ev.BytesWritten = written
	ev.BytesTotal = total
	notifyRepoMapScan(ev)
}

func (p *repoMapScanProgress) finish(ok bool, err error) {
	if p == nil || !p.started {
		return
	}
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	notifyRepoMapScan(p.event(false, true, p.parseableFiles, ok, errText))
}

func (p *repoMapScanProgress) event(progress, finished bool, parsed int, ok bool, errText string) ctypes.RepoMapScanEvent {
	elapsedMs := int64(0)
	if !p.start.IsZero() {
		elapsedMs = time.Since(p.start).Milliseconds()
	}
	return ctypes.RepoMapScanEvent{
		RepoRoot:           p.repoRoot,
		Mode:               p.mode,
		Phase:              p.phase,
		Started:            !progress && !finished,
		Progress:           progress,
		Finished:           finished,
		OK:                 ok,
		TotalFiles:         p.totalFiles,
		ParseableFiles:     p.parseableFiles,
		ParsedFiles:        parsed,
		ChangedFiles:       p.changedFiles,
		ElapsedMs:          elapsedMs,
		Error:              errText,
		CacheRecordsLoaded: p.cacheRecordsLoaded,
		CacheRecordsTotal:  p.cacheRecordsTotal,
		CacheChunksLoaded:  p.cacheChunksLoaded,
		CacheChunksTotal:   p.cacheChunksTotal,
		ViewStep:           p.viewStep,
		ViewStepsDone:      p.viewStepsDone,
		ViewStepsTotal:     p.viewStepsTotal,
	}
}
