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
}

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
	if done-p.lastDone < 1000 && now.Sub(p.lastEmit) < 2*time.Second {
		return
	}
	p.lastDone = done
	p.lastEmit = now
	notifyRepoMapScan(p.event(true, false, done, true, ""))
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
		RepoRoot:       p.repoRoot,
		Mode:           p.mode,
		Phase:          p.phase,
		Started:        !progress && !finished,
		Progress:       progress,
		Finished:       finished,
		OK:             ok,
		TotalFiles:     p.totalFiles,
		ParseableFiles: p.parseableFiles,
		ParsedFiles:    parsed,
		ChangedFiles:   p.changedFiles,
		ElapsedMs:      elapsedMs,
		Error:          errText,
	}
}
