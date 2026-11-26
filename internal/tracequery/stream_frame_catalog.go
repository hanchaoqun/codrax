package tracequery

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const ViewFrameCatalog = "frame_catalog"

// FrameCatalogEntry identifies a closed Android Choreographer#doFrame by its
// physical begin line. Vsync numbers are local to one trace and can repeat.
type FrameCatalogEntry struct {
	ID              string                `json:"id"`
	Vsync           uint64                `json:"vsync"`
	TID             int                   `json:"tid"`
	ProcessID       int                   `json:"process_id,omitempty"`
	Thread          string                `json:"thread"`
	StartNS         uint64                `json:"start_ns"`
	EndNS           uint64                `json:"end_ns"`
	StartLine       int                   `json:"start_line"`
	EndLine         int                   `json:"end_line"`
	Points          []string              `json:"points,omitempty"`
	VideoOutputPath string                `json:"video_output_path,omitempty"`
	HostComposition *FrameHostComposition `json:"host_composition,omitempty"`
}

type FrameCatalog struct {
	Frames       []FrameCatalogEntry `json:"frames"`
	PointSpans   []FrameCatalogPoint `json:"point_spans,omitempty"`
	ScannedLines int                 `json:"scanned_lines"`
	Unclosed     int                 `json:"unclosed_frames"`
}

// FrameCatalogPoint retains one observed marker interval on the physical emitter.
// FrameLine is zero for work observed outside any selected frame.
type FrameCatalogPoint struct {
	Name      string `json:"name"`
	TID       int    `json:"tid"`
	StartNS   uint64 `json:"start_ns"`
	EndNS     uint64 `json:"end_ns"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	FrameLine int    `json:"frame_line,omitempty"`
	Role      string `json:"role,omitempty"`
}

var frameCatalogDrawID = regexp.MustCompile(`(?i)(?:^|[#.: ])(?:syncAndDrawFrame|drawFrames?)\s+([0-9]+)\b`)

type frameCatalogRender struct {
	process, tid int
	vsync        uint64
	start, end   uint64
	points       []FrameCatalogPoint
}

// StreamFrameCatalog retains closed frames and scoped marker spans, without
// parsing scheduler events. The selection bounds apply to frame starts; a closing
// marker after the requested end still supplies the actual frame boundary.
func StreamFrameCatalog(ctx context.Context, path string, q Query) (FrameCatalog, error) {
	composite, err := tracePathRequiresCompositeIndexContext(ctx, path)
	if err != nil {
		return FrameCatalog{}, err
	}
	if composite {
		return FrameCatalog{}, fmt.Errorf("frame_catalog requires one physical trace")
	}
	if q.TargetScope == TargetScopeProcess {
		return FrameCatalog{}, fmt.Errorf("frame_catalog uses a physical tid or a literal emitter thread name")
	}
	thread := strings.TrimSpace(q.Thread)
	type openSpan struct {
		frame  *FrameCatalogEntry
		render *frameCatalogRender
		point  FrameCatalogPoint
	}
	stacks := make(map[int][]openSpan)
	active := make(map[int][]*FrameCatalogEntry)
	rendering := make(map[int]*frameCatalogRender)
	processes := make(map[int]bool)
	video := frameCatalogVideo{tracks: make(map[frameVideoProducer]*frameVideoTrack), pending: make(map[int]*frameVideoTrack)}
	host := frameCatalogHost{open: make(map[int]*frameHostCycle), flushes: make(map[string][]frameHostFlush), queues: make(map[string][]frameHostAcquire)}
	hostQuery := q
	if queryBoundedTimeEnd(q) {
		hostQuery.TimeEnd += .1
	}
	var renders []*frameCatalogRender
	var result FrameCatalog
	scan, err := StreamAdmittedTraceTextLines(ctx, path, func(line AdmittedTraceTextLine) bool {
		tid, tgid, comm, ns, mark, ok := frameCatalogMarker(line.Text)
		if !ok {
			return true
		}
		if comm == "render_service" || strings.HasPrefix(mark.name, "H:FlushBuffer name: ShellAssistantAnco_") {
			host.observe(tid, comm, ns, line.Number, mark, frameCatalogInScope(hostQuery, line.Number, ns))
		}
		selected := (q.PID <= 0 || tid == q.PID) && (thread == "" || strings.EqualFold(comm, thread))
		scoped := q.PID > 0 || thread != ""
		if selected && scoped && tgid > 0 {
			processes[tgid] = true
		}
		if mark.action == "B" && processes[tgid] {
			decoder := strings.HasPrefix(comm, "VOutle")
			if (decoder || strings.HasPrefix(mark.name, "dequeueBuffer - ") || video.pending[tid] != nil) &&
				frameCatalogInScope(q, line.Number, ns) {
				video.observe(tid, tgid, ns, mark.name, decoder)
			}
		}
		var draw []string
		if !selected && rendering[tid] == nil && processes[tgid] && mark.action == "B" &&
			strings.Contains(strings.ToLower(mark.name), "drawframe") {
			draw = frameCatalogDrawID.FindStringSubmatch(mark.name)
		}
		if !selected && rendering[tid] == nil && len(draw) == 0 && len(stacks[tid]) == 0 {
			return true
		}
		if mark.invalidAction == traceMarkActionB || mark.invalidAction == traceMarkActionE {
			result.Unclosed += len(active[tid])
			delete(active, tid)
			delete(stacks, tid)
			delete(rendering, tid)
			return true
		}
		stack, frames := stacks[tid], active[tid]
		switch mark.action {
		case "B":
			span := openSpan{}
			inScope := selected && frameCatalogInScope(q, line.Number, ns)
			value, hasFrame := strings.CutPrefix(mark.name, "Choreographer#doFrame ")
			if len(draw) != 0 {
				if vsync, err := strconv.ParseUint(draw[1], 10, 64); err == nil {
					span.render = &frameCatalogRender{process: tgid, tid: tid, vsync: vsync, start: ns}
					rendering[tid] = span.render
				}
			} else if render := rendering[tid]; render != nil {
				span.point = FrameCatalogPoint{Name: clampString(mark.name, 160), TID: tid, StartNS: ns, StartLine: line.Number, Role: "render"}
			} else if hasFrame && isAllDigits(value) {
				if vsync, err := strconv.ParseUint(value, 10, 64); err == nil && inScope {
					span.frame = &FrameCatalogEntry{ID: "f" + strconv.Itoa(line.Number), Vsync: vsync, TID: tid, ProcessID: tgid, Thread: comm, StartNS: ns, StartLine: line.Number}
					active[tid] = append(frames, span.frame)
				}
			} else if len(frames) != 0 || inScope && scoped {
				span.point = FrameCatalogPoint{Name: clampString(mark.name, 160), TID: tid, StartNS: ns, StartLine: line.Number}
				if n := len(frames); n > 0 {
					span.point.FrameLine = frames[n-1].StartLine
					if label := frameCatalogPoint(mark.name); label != "" {
						for _, frame := range frames {
							if len(frame.Points) < 24 {
								frame.Points = append(frame.Points, label)
							}
						}
					}
				}
			}
			stacks[tid] = append(stack, span)
		case "E":
			n := len(stack)
			if n == 0 {
				return true
			}
			span := stack[n-1]
			stacks[tid] = stack[:n-1]
			if span.frame != nil {
				if ns >= span.frame.StartNS {
					span.frame.EndNS, span.frame.EndLine = ns, line.Number
					result.Frames = append(result.Frames, *span.frame)
				} else {
					result.Unclosed++
				}
				active[tid] = frames[:len(frames)-1]
			} else if span.render != nil {
				if ns > span.render.start {
					span.render.end = ns
					renders = append(renders, span.render)
				}
				delete(rendering, tid)
			} else if span.point.StartLine != 0 && ns >= span.point.StartNS {
				span.point.EndNS, span.point.EndLine = ns, line.Number
				if span.point.Role == "render" {
					owner := rendering[tid]
					owner.points = append(owner.points, span.point)
				} else {
					result.PointSpans = append(result.PointSpans, span.point)
				}
			}
		}
		return true
	})
	if err != nil {
		return FrameCatalog{}, err
	}
	for _, frames := range active {
		result.Unclosed += len(frames)
	}
	result.ScannedLines = scan.ScannedLines
	linkFrameCatalogRender(&result, renders)
	host.attach(&result, video.attach(result.Frames))
	sort.Slice(result.Frames, func(i, j int) bool { return result.Frames[i].StartLine < result.Frames[j].StartLine })
	sort.Slice(result.PointSpans, func(i, j int) bool { return result.PointSpans[i].StartLine < result.PointSpans[j].StartLine })
	return result, nil
}

func frameCatalogInScope(q Query, line int, ns uint64) bool {
	if q.LineStart > 0 || q.LineEnd > 0 {
		return line >= q.LineStart && (q.LineEnd == 0 || line <= q.LineEnd)
	}
	ts := float64(ns) / 1e9
	return (!queryBoundedTimeStart(q) || ts >= q.TimeStart) && (!queryBoundedTimeEnd(q) || ts < q.TimeEnd)
}

func linkFrameCatalogRender(result *FrameCatalog, renders []*frameCatalogRender) {
	type identity struct {
		process int
		vsync   uint64
	}
	draws := make(map[identity][]*frameCatalogRender)
	for _, render := range renders {
		key := identity{render.process, render.vsync}
		draws[key] = append(draws[key], render)
	}
	claims := make(map[*frameCatalogRender]int)
	for _, frame := range result.Frames {
		var candidates []*frameCatalogRender
		for _, draw := range draws[identity{frame.ProcessID, frame.Vsync}] {
			if draw.tid != frame.TID && draw.end > frame.StartNS && draw.start < frame.EndNS+50_000_000 {
				candidates = append(candidates, draw)
			}
		}
		if len(candidates) != 1 {
			for _, draw := range candidates {
				claims[draw] = -1
			}
			continue
		}
		draw := candidates[0]
		if claims[draw] != 0 {
			claims[draw] = -1
		} else {
			claims[draw] = frame.StartLine
		}
	}
	for _, draw := range renders {
		if line := claims[draw]; line > 0 {
			for _, point := range draw.points {
				point.FrameLine = line
				result.PointSpans = append(result.PointSpans, point)
			}
		}
	}
}

func frameCatalogMarker(line string) (tid int, tgid int, comm string, ns uint64, mark traceMarkParseResult, ok bool) {
	if exact, found := parseExactTraceMark(line); found {
		return exact.TID, exact.TGID, exact.Comm, exact.TimestampNS, traceMarkParseResult{action: exact.Action, name: exact.Name}, true
	}
	if marker, found := parseCPUUnavailableTraceMark(line); found {
		return marker.TID, marker.TGID, marker.Comm, marker.TimestampNS, traceMarkParseResult{action: marker.Action, name: marker.Name}, true
	}
	if !strings.Contains(line, "tracing_mark_write:") && !strings.Contains(line, "tracing_mark_write_xacct:") && !strings.Contains(line, "print:") {
		return
	}
	m := matchFtraceLine(line)
	if len(m) == 0 || !isPrintFamilyRaw(strings.TrimSuffix(strings.TrimSpace(m[6]), ":")) {
		return
	}
	fields := normalizeTraceMarkPayload(m[7])
	if !strings.HasPrefix(fields, "B|") && !strings.HasPrefix(fields, "E|") && fields != "E" && fields != "B" {
		return
	}
	tid, ok = parseFtraceHeaderTID(m[2])
	if !ok {
		return
	}
	tgid, _ = parseFtraceHeaderTID(m[3])
	ns, ok = parseTraceTimestampNanoseconds(m[5])
	return tid, tgid, strings.TrimSpace(m[1]), ns, parseTraceMarkValidated(fields), ok
}

func frameCatalogPoint(name string) string {
	switch {
	case name == "traversal", name == "postAndWait", name == "TextureView#draw()", name == "Record View#draw()":
		return name
	case strings.HasPrefix(name, "animation#action:"):
		return "animation"
	case strings.HasPrefix(name, "Choreographer#doFrame - resynced"):
		return "resync"
	case strings.HasPrefix(name, "ViewRootImpl#setFrameRateCategory"):
		return "frame-rate-category"
	case strings.HasPrefix(name, "DrawFrames setSurfaceName "):
		return "submit-surface"
	case strings.Contains(name, "relayoutWindow"):
		return "relayoutWindow"
	}
	for _, prefix := range []string{"draw-VRI[", "measure-VRI[", "layout-VRI["} {
		if tail, ok := strings.CutPrefix(name, prefix); ok {
			if window, _, closed := strings.Cut(tail, "]"); closed {
				return prefix + clampString(window, 96) + "]"
			}
		}
	}
	return ""
}
