package tracequery

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStreamFrameCatalogPairsPhysicalEmitterStacks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frames.ftrace")
	const trace = `app-7 (7) [000] .... 1.000000001: tracing_mark_write: B|99|Choreographer#doFrame 9
app-7 (7) [001] .... 1.001000002: tracing_mark_write: B|99|draw-VRI[Main]
other-8 (8) [002] .... 1.001100003: tracing_mark_write: E|99
app-7 (7) [000] .... 1.002000004: tracing_mark_write: E|99
app-7 (7) [000] .... 1.003000005: tracing_mark_write: E
app-7 (7) [000] .... 1.004000006: tracing_mark_write: B|99|Choreographer#doFrame 9
app-7 (7) [000] .... 1.005000007: tracing_mark_write: E|99
app-7 (7) [000] .... 1.006000008: tracing_mark_write: B|99|Choreographer#doFrame 10
app-7 (7) [000] .... 1.007000009: tracing_mark_write: E|bad
app-7 (7) [000] .... 1.008000010: tracing_mark_write: B|99|Choreographer#doFrame 11
`
	if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := StreamFrameCatalog(context.Background(), path, Query{Thread: "app", TimeEnd: 1.009})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Frames) != 2 || got.Unclosed != 2 || got.ScannedLines != 10 {
		t.Fatalf("catalog = %+v", got)
	}
	first, second := got.Frames[0], got.Frames[1]
	if first.ID != "f1" || second.ID != "f6" || first.Vsync != second.Vsync || first.TID != 7 ||
		first.StartNS != 1000000001 || first.EndNS != 1003000005 || first.EndLine != 5 ||
		len(first.Points) != 1 || first.Points[0] != "draw-VRI[Main]" {
		t.Fatalf("unexpected frames: %+v / %+v", first, second)
	}
	if len(got.PointSpans) != 1 {
		t.Fatalf("expected one closed in-frame point: %+v", got.PointSpans)
	}
	point := got.PointSpans[0]
	if point.Name != "draw-VRI[Main]" || point.FrameLine != first.StartLine ||
		point.StartNS != 1001000002 || point.EndNS != 1002000004 || point.StartLine != 2 || point.EndLine != 4 {
		t.Fatalf("unexpected point duration or evidence: %+v", point)
	}
	got, err = StreamFrameCatalog(context.Background(), path, Query{PID: 7, TimeStart: 1, TimeEnd: 1.002})
	if err != nil || len(got.Frames) != 1 || got.Frames[0].EndNS != first.EndNS {
		t.Fatalf("ending outside the requested start interval must retain the real boundary: %+v, %v", got, err)
	}
}

func TestStreamFrameCatalogLinksRenderByProcessAndVsync(t *testing.T) {
	path := filepath.Join(t.TempDir(), "draw.ftrace")
	const raw = `app-7 (7) [000] .... 1.000: tracing_mark_write: B|99|Choreographer#doFrame 9
foreign-8 (8) [001] .... 1.001: tracing_mark_write: B|88|DrawFrame 9
foreign-8 (8) [001] .... 1.002: tracing_mark_write: B|88|unrelated
foreign-8 (8) [001] .... 1.003: tracing_mark_write: E|88
foreign-8 (8) [001] .... 1.004: tracing_mark_write: E|88
RenderThread-17 (7) [002] .... 1.005: tracing_mark_write: B|99|DrawFrame 9
RenderThread-17 (7) [002] .... 1.006: tracing_mark_write: B|99|flushTexture
RenderThread-17 (7) [002] .... 1.007: tracing_mark_write: E|99
app-7 (7) [000] .... 1.008: tracing_mark_write: E|99
RenderThread-17 (7) [002] .... 1.009: tracing_mark_write: E|99
`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := StreamFrameCatalog(context.Background(), path, Query{PID: 7, TimeEnd: 1.01})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Frames) != 1 || len(got.PointSpans) != 1 {
		t.Fatalf("wrong local render association: %+v", got)
	}
	p := got.PointSpans[0]
	if p.Role != "render" || p.Name != "flushTexture" || p.TID != 17 || p.FrameLine != 1 || p.StartLine != 7 || p.EndLine != 8 {
		t.Fatalf("wrong physical render evidence: %+v", p)
	}
}

func TestStreamFrameCatalogLinksActiveVideoOutputToApp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "video.ftrace")
	const raw = `app-7 (7) [000] .... 1.000: tracing_mark_write: B|99|activityResume
app-7 (7) [000] .... 1.001: tracing_mark_write: E|99
VOutle2-V1-18 (8) [000] .... 1.010: tracing_mark_write: B|88|dequeueBuffer - SurfaceView[Foreign]
VOutle2-V1-18 (8) [000] .... 1.011: tracing_mark_write: B|88|queueBuffer
VOutle2-V1-17 (7) [000] .... 1.020: tracing_mark_write: B|99|dequeueBuffer - SurfaceTexture-0-99-4
VOutle2-V1-17 (7) [000] .... 1.021: tracing_mark_write: B|99|queueBuffer
VOutle2-V1-17 (7) [000] .... 1.050: tracing_mark_write: B|99|dequeueBuffer - SurfaceTexture-0-99-4
VOutle2-V1-17 (7) [000] .... 1.051: tracing_mark_write: B|99|queueBuffer
VOutle2-V1-17 (7) [000] .... 1.080: tracing_mark_write: B|99|dequeueBuffer - SurfaceTexture-0-99-4
VOutle2-V1-17 (7) [000] .... 1.081: tracing_mark_write: B|99|queueBuffer
app-7 (7) [000] .... 1.090: tracing_mark_write: B|99|Choreographer#doFrame 9
app-7 (7) [000] .... 1.091: tracing_mark_write: E|99
app-7 (7) [000] .... 1.400: tracing_mark_write: B|99|Choreographer#doFrame 10
app-7 (7) [000] .... 1.401: tracing_mark_write: E|99
`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := StreamFrameCatalog(context.Background(), path, Query{PID: 7, TimeEnd: 1.5})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Frames) != 2 || got.Frames[0].VideoOutputPath != "surface_texture" || got.Frames[1].VideoOutputPath != "" {
		t.Fatalf("incorrect active producer coverage: %+v", got.Frames)
	}
}

func TestStreamFrameCatalogLinksHostLayersWithinOneComposition(t *testing.T) {
	path := filepath.Join(t.TempDir(), "host.ftrace")
	const raw = `app-7 (7) [000] .... 0.880: tracing_mark_write: B|99|activityResume
app-7 (7) [000] .... 0.881: tracing_mark_write: E|99
VOutle2-V1-18 (7) [000] .... 0.900: tracing_mark_write: B|99|dequeueBuffer - aa1 SurfaceView[example/.Main]
VOutle2-V1-18 (7) [000] .... 0.901: tracing_mark_write: B|99|queueBuffer
VOutle2-V1-18 (7) [000] .... 0.930: tracing_mark_write: B|99|dequeueBuffer - aa1 SurfaceView[example/.Main]
VOutle2-V1-18 (7) [000] .... 0.931: tracing_mark_write: B|99|queueBuffer
VOutle2-V1-18 (7) [000] .... 0.960: tracing_mark_write: B|99|dequeueBuffer - aa1 SurfaceView[example/.Main]
VOutle2-V1-18 (7) [000] .... 0.961: tracing_mark_write: B|99|queueBuffer
app-7 (7) [000] .... 1.000: tracing_mark_write: B|99|Choreographer#doFrame 9
app-7 (7) [000] .... 1.001: tracing_mark_write: B|99|draw-VRI[Main]
app-7 (7) [000] .... 1.002: tracing_mark_write: E|99
RenderThread-17 (7) [000] .... 1.003: tracing_mark_write: B|99|DrawFrame 9
app-7 (7) [000] .... 1.004: tracing_mark_write: E|99
RenderThread-17 (7) [000] .... 1.005: tracing_mark_write: B|99|ProducerSurfaceDelegator::QueueBuffer slot: 1 mWindowId: 200123
OS_IPC-30 (29) [000] .... 1.006: tracing_mark_write: B|29|H:FlushBuffer name: ShellAssistantAnco_200123Surface queueId: 314 sequence: 42|M0538
OS_IPC-30 (29) [000] .... 1.007: tracing_mark_write: E|29|M0538
RenderThread-17 (7) [000] .... 1.008: tracing_mark_write: E|99
RenderThread-17 (7) [000] .... 1.009: tracing_mark_write: E|99
render_service-29 (29) [000] .... 1.010: tracing_mark_write: B|29|H:RSMainThread::DoComposition: 1009500000|I38
render_service-29 (29) [000] .... 1.011: tracing_mark_write: B|29|H:AcquireBuffer with PresentTimestamp name: ShellAssistantAnco_200123Surface queueId: 314 queueSize: 4,expectPresentTimestamp: 1010000000|M0538
render_service-29 (29) [000] .... 1.012: tracing_mark_write: E|29|M0538
render_service-29 (29) [000] .... 1.013: tracing_mark_write: B|29|H:AcquireBuffer with PresentTimestamp name: ShellAssistantAnco_400_bb2 SurfaceView[another/.Main]#11_1Surface queueId: 315 queueSize: 4,expectPresentTimestamp: 1010000000|M0538
render_service-29 (29) [000] .... 1.014: tracing_mark_write: E|29|M0538
render_service-29 (29) [000] .... 1.015: tracing_mark_write: B|29|H:AcquireBuffer with PresentTimestamp name: ShellAssistantAnco_401_aa1 SurfaceView[example/.Main]#11_1Surface queueId: 316 queueSize: 4,expectPresentTimestamp: 1010000000|M0538
render_service-29 (29) [000] .... 1.016: tracing_mark_write: E|29|M0538
render_service-29 (29) [000] .... 1.017: tracing_mark_write: E|29|I38
`
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := StreamFrameCatalog(context.Background(), path, Query{PID: 7, TimeEnd: 1.02})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Frames) != 1 || got.Frames[0].HostComposition == nil {
		t.Fatalf("expected one linked host composition: %+v", got.Frames)
	}
	host := *got.Frames[0].HostComposition
	if host != (FrameHostComposition{FlushLine: 15, AcquireLine: 20, CompositionLine: 19, Cycles: 1, FrameToHostMS: 11, SecondaryAcquireLine: 24}) {
		t.Fatalf("incorrect host composition evidence: %+v", host)
	}
	structural := strings.ReplaceAll(raw, "VOutle2-V1-18", "Reade2-V17-18")
	structural = strings.ReplaceAll(structural, "B|99|queueBuffer", "B|99|ProducerSurfaceDelegator::QueueBuffer slot: 1 mWindowId: 200123")
	if err := os.WriteFile(path, []byte(structural), 0600); err != nil {
		t.Fatal(err)
	}
	got, err = StreamFrameCatalog(context.Background(), path, Query{PID: 7, TimeEnd: 1.02})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Frames) != 1 || got.Frames[0].VideoOutputPath != "" ||
		got.Frames[0].HostComposition == nil || *got.Frames[0].HostComposition != host {
		t.Fatalf("explicit layer submission must retain host evidence and leave decoder route unknown: %+v", got.Frames)
	}
}
