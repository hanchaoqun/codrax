package tracequery

import (
	"math"
	"regexp"
	"sort"
	"strings"
)

func rerankByFrameHostComposition(target FrameCatalogEntry, candidates []FrameMatchCandidate) {
	t := target.HostComposition
	first := candidates[0].Frame
	b := first.HostComposition
	if t == nil || b == nil || t.SecondaryAcquireLine == 0 || b.SecondaryAcquireLine != 0 || t.Cycles != 1 || b.Cycles != 1 {
		return
	}
	best := 0
	bestScore := .7*candidates[0].Score + .15*math.Exp(-math.Abs(b.FrameToHostMS-t.FrameToHostMS)/4)
	bestSignal := 0.0
	for i, candidate := range candidates[1:] {
		f := candidate.Frame
		c := f.HostComposition
		if c == nil || c.Cycles != 1 || c.SecondaryAcquireLine == 0 ||
			math.Abs(float64(f.StartNS)-float64(first.StartNS)) > 250_000_000 {
			continue
		}
		if candidate.Evidence["local_points"] == 0 ||
			(candidate.Evidence["before_gaps"] == 0 && candidate.Evidence["anchor_position"] == 0) {
			continue
		}
		signal := .5 + .5*math.Exp(-math.Abs(c.FrameToHostMS-t.FrameToHostMS)/4)
		score := .7*candidate.Score + .3*signal
		if score > bestScore {
			best, bestScore, bestSignal = i+1, score, signal
		}
	}
	if best != 0 {
		candidate := candidates[best]
		candidate.Evidence["host_composition_similarity"] = bestSignal
		candidate.Evidence["host_composition_selection_score"] = bestScore
		copy(candidates[1:best+1], candidates[:best])
		candidates[0] = candidate
	}
}

// FrameHostComposition records a source-local window and queue association.
type FrameHostComposition struct {
	FlushLine            int     `json:"flush_line"`
	AcquireLine          int     `json:"acquire_line"`
	CompositionLine      int     `json:"composition_line"`
	Cycles               int     `json:"cycles"`
	FrameToHostMS        float64 `json:"frame_to_host_ms"`
	SecondaryAcquireLine int     `json:"secondary_acquire_line,omitempty"`
}

var (
	frameHostFlushName = regexp.MustCompile(`^H:FlushBuffer name: ShellAssistantAnco_([0-9]+)Surface queueId: ([0-9]+) sequence: [0-9]+`)
	frameHostMainName  = regexp.MustCompile(`^ShellAssistantAnco_([0-9]+)Surface$`)
	frameGuestWindow   = regexp.MustCompile(`\bmWindowId:\s*([0-9]+)`)
)

type frameHostFlush struct {
	ns    uint64
	line  int
	queue string
}

type frameHostLayer struct {
	ns          uint64
	line        int
	name, queue string
}

type frameHostCycle struct {
	depth, line int
	start       uint64
	layers      []frameHostLayer
}

type frameHostAcquire struct {
	frameHostLayer
	window string
	cycle  *frameHostCycle
}

type frameCatalogHost struct {
	starts  []uint64
	open    map[int]*frameHostCycle
	flushes map[string][]frameHostFlush
	queues  map[string][]frameHostAcquire
}

func (h *frameCatalogHost) observe(tid int, comm string, ns uint64, line int, mark traceMarkParseResult, inScope bool) {
	if mark.action == "B" && inScope && strings.HasPrefix(mark.name, "H:FlushBuffer name: ShellAssistantAnco_") {
		if m := frameHostFlushName.FindStringSubmatch(mark.name); m != nil {
			h.flushes[m[1]] = append(h.flushes[m[1]], frameHostFlush{ns, line, m[2]})
		}
	}
	if comm != "render_service" {
		return
	}
	if mark.action == "B" && strings.HasPrefix(mark.name, "H:RSMainThread::DoComposition:") {
		if inScope {
			h.open[tid] = &frameHostCycle{depth: 1, line: line, start: ns}
			h.starts = append(h.starts, ns)
		}
		return
	}
	cycle := h.open[tid]
	if cycle == nil {
		return
	}
	switch mark.action {
	case "B":
		cycle.depth++
		if tail, ok := strings.CutPrefix(mark.name, "H:AcquireBuffer with PresentTimestamp name: "); ok {
			name, tail, ok := strings.Cut(tail, " queueId: ")
			queue, _, found := strings.Cut(tail, " queueSize:")
			if ok && found && isAllDigits(queue) {
				cycle.layers = append(cycle.layers, frameHostLayer{ns, line, name, queue})
			}
		}
	case "E":
		cycle.depth--
		if cycle.depth != 0 {
			return
		}
		delete(h.open, tid)
		for _, layer := range cycle.layers {
			if m := frameHostMainName.FindStringSubmatch(layer.name); m != nil {
				h.queues[layer.queue] = append(h.queues[layer.queue], frameHostAcquire{layer, m[1], cycle})
			}
		}
	}
}

func (h *frameCatalogHost) attach(catalog *FrameCatalog, surfaceTokens map[int]string) {
	if len(h.queues) == 0 || len(surfaceTokens) == 0 {
		return
	}
	submissions := make(map[int][]FrameCatalogPoint)
	for _, p := range catalog.PointSpans {
		if p.Role == "render" && strings.HasPrefix(p.Name, "ProducerSurfaceDelegator::QueueBuffer ") {
			submissions[p.FrameLine] = append(submissions[p.FrameLine], p)
		}
	}
	for i := range catalog.Frames {
		f := &catalog.Frames[i]
		surfaceToken := surfaceTokens[f.StartLine]
		points := submissions[f.StartLine]
		if surfaceToken == "" || len(points) != 1 {
			continue
		}
		p := points[0]
		m := frameGuestWindow.FindStringSubmatch(p.Name)
		if m == nil {
			continue
		}
		window := m[1]
		flushes := h.flushes[window]
		a := sort.Search(len(flushes), func(i int) bool { return flushes[i].ns >= p.StartNS })
		end := sort.Search(len(flushes), func(i int) bool { return flushes[i].ns > p.EndNS })
		if end != a+1 {
			continue
		}
		flush := flushes[a]
		entries := h.queues[flush.queue]
		j := sort.Search(len(entries), func(i int) bool { return entries[i].ns >= flush.ns })
		if j == len(entries) {
			continue
		}
		acquire := entries[j]
		if acquire.window != window || acquire.ns-flush.ns > 50_000_000 ||
			end < len(flushes) && flushes[end].ns < acquire.ns {
			continue
		}
		secondary := 0
		for _, layer := range acquire.cycle.layers {
			if strings.HasPrefix(layer.name, "ShellAssistantAnco_") && strings.Contains(layer.name, "_"+surfaceToken+" SurfaceView[") {
				secondary = layer.line
				break
			}
		}
		f.HostComposition = &FrameHostComposition{
			FlushLine: flush.line, AcquireLine: acquire.line, CompositionLine: acquire.cycle.line,
			Cycles: sort.Search(len(h.starts), func(i int) bool { return h.starts[i] > acquire.cycle.start }) -
				sort.Search(len(h.starts), func(i int) bool { return h.starts[i] >= f.StartNS }),
			FrameToHostMS: (float64(acquire.ns) - float64(f.StartNS)) / 1e6, SecondaryAcquireLine: secondary,
		}
	}
}
