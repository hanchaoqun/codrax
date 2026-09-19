package orchestrator

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRunNamedBinaryTracePreparesFullMaterialAndIsolatesRuns(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	path := filepath.Join(repo, "raw capture.sys")
	runtimeAnchor := filepath.Join(t.TempDir(), "runtime")
	analyzerCalls, otherCalls := 0, 0
	var events []render.Event
	o := newTypedNamedTraceAdmissionTestOrchestrator([]string{path}, &analyzerCalls, &otherCalls, &events)
	o.SetTraceRuntimeAnchor(runtimeAnchor)
	var previous types.TraceInputPreparer
	var previousMaterial *attachment.TraceMaterial
	for run := 0; run < 2; run++ {
		original := namedBinaryTraceFixture(uint32(100 + run*100))
		if err := os.WriteFile(path, original, 0o600); err != nil {
			t.Fatal(err)
		}
		events = nil
		bus, _ := o.Run("只分析 `"+path+"` 的调度，不分析代码", repo, "main")
		if bus == nil || bus.TraceInputPreparer == nil || !hasTraceAdmissionEventKind(events, render.EventAnalysisReady) {
			t.Fatalf("supported binary did not pass typed admission on Run %d: bus=%v events=%v", run, bus != nil, events)
		}
		if bus.TraceInputPreparer == previous {
			t.Fatal("separate Runs shared a preparation coordinator")
		}
		material, err := bus.TraceInputPreparer.Prepare(context.Background(), path)
		if err != nil || material == nil || material == previousMaterial {
			t.Fatalf("Run %d reused stale material or lost successful preparation: %v %v", run, material, err)
		}
		index, err := tracequery.BuildIndex(context.Background(), material.QueryPath())
		if err != nil || len(index.Events) != 12 || index.Events[11].WakeePID != 111+run*100 {
			t.Fatalf("full binary material lost tail or mixed Run generations: events=%+v err=%v", index, err)
		}
		anchor, err := filepath.EvalSymlinks(runtimeAnchor)
		if err != nil || !strings.HasPrefix(material.QueryPath(), anchor+string(filepath.Separator)) {
			t.Fatalf("prepared output escaped stable anchor: query=%q anchor=%q err=%v", material.QueryPath(), anchor, err)
		}
		if material.QueryPath() == path || strings.HasPrefix(material.QueryPath(), bus.WorkDir+string(filepath.Separator)) || bus.AttachedHitrace != "" || bus.AttachedTraceMaterial != nil {
			t.Fatalf("named input became sticky attachment or temporary material: path=%q work=%q", material.QueryPath(), bus.WorkDir)
		}
		if got, err := os.ReadFile(path); err != nil || !bytes.Equal(got, original) {
			t.Fatalf("preparation changed original bytes: %v", err)
		}
		if previousMaterial != nil && previousMaterial.Validate(context.Background(), previousMaterial.Preview()) == nil {
			t.Fatal("old receipt remained valid after source generation changed")
		}
		previous, previousMaterial = bus.TraceInputPreparer, material
	}
}

func TestRunNamedBinarySetFailurePreventsPartialInvestigation(t *testing.T) {
	repo := t.TempDir()
	writeTraceAdmissionRepoSource(t, repo)
	good, bad := filepath.Join(repo, "good.sys"), filepath.Join(repo, "bad.sys")
	if err := os.WriteFile(good, namedBinaryTraceFixture(100), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte{0xdf, 0x49, 0, 1}, 0o600); err != nil {
		t.Fatal(err)
	}
	analyzerCalls, otherCalls := 0, 0
	var events []render.Event
	o := newTypedNamedTraceAdmissionTestOrchestrator([]string{good, bad}, &analyzerCalls, &otherCalls, &events)
	o.SetTraceRuntimeAnchor(filepath.Join(t.TempDir(), "runtime"))
	bus, err := o.Run("对比 `"+good+"` 与 `"+bad+"` 两份 trace，不分析代码", repo, "main")
	if err == nil || bus == nil || analyzerCalls != 1 || otherCalls != 0 || hasTraceAdmissionEventKind(events, render.EventAnalysisReady) {
		t.Fatalf("partially admitted binary set entered investigation: analyzer=%d other=%d err=%v", analyzerCalls, otherCalls, err)
	}
	materials := bus.TraceInputPreparer.PreparedMaterials()
	if len(materials) != 1 || !materials[0].MatchesPath(good) {
		t.Fatalf("successful preparation was lost or failed material published: %+v", materials)
	}
	if err := materials[0].Validate(context.Background(), materials[0].Preview()); err != nil {
		t.Fatalf("one failed sibling destroyed committed query material: %v", err)
	}
}

type admissionPreparationProbe struct {
	delegate    types.TraceInputPreparer
	paths       []string
	contexts    []context.Context
	secondError error
}

func (p *admissionPreparationProbe) Prepare(ctx context.Context, path string) (*attachment.TraceMaterial, error) {
	p.paths = append(p.paths, path)
	p.contexts = append(p.contexts, ctx)
	if len(p.paths) == 2 && p.secondError != nil {
		return nil, p.secondError
	}
	return p.delegate.Prepare(ctx, path)
}

func (p *admissionPreparationProbe) PreparedMaterials() []*attachment.TraceMaterial {
	return p.delegate.PreparedMaterials()
}

func TestTypedNamedAdmissionRevalidatesCoordinatorUniverseWithoutRewritingInputs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.trace")
	row := "app-20 (20) [001] .... 10.000000: sched_wakeup: comm=app pid=20 prio=20 target_cpu=001\n"
	if err := os.WriteFile(path, []byte(row), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changed := errors.New("prepared source universe changed")
	probe := &admissionPreparationProbe{delegate: traceinput.NewCoordinator(traceinput.Options{}), secondError: changed}
	bus := &types.BusContext{
		TraceInputPreparer: probe,
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{
			{Kind: "trace", Source: path, Carrier: "request_path"},
		}},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Scenario:      types.ScenarioPerformanceBottleneck,
			AnalyzerHints: types.AnalyzerHints{RequiredFileHints: []types.RequiredFileHint{{Path: path, Confidence: 1}}},
		}},
	}
	// The same path cannot arm preparation without the settled typed policy.
	if err := validateTypedNamedTraceInputsBeforeExploration(ctx, bus, "trace "+path); err != nil || len(probe.paths) != 0 {
		t.Fatalf("raw path armed preparation: calls=%v err=%v", probe.paths, err)
	}
	bus.AnalysisIR.RequestModel.ExternalObservationPolicy = &types.ExternalObservationPolicy{
		CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
		ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
		SourceQuotes:      []string{"only the trace, not current source"},
	}
	err := validateTypedNamedTraceInputsBeforeExploration(ctx, bus, "trace "+path)
	if !errors.Is(err, changed) || len(probe.paths) != 2 || probe.contexts[0] != ctx || probe.contexts[1] != ctx {
		t.Fatalf("query admission missed final universe/cancellation check: calls=%v err=%v", probe.paths, err)
	}
	if probe.paths[0] != probe.paths[1] || bus.RuntimeArtifactPreflight.Artifacts[0].Source != path || bus.AnalysisIR.RequestModel.AnalyzerHints.RequiredFileHints[0].Path != path {
		t.Fatal("preparation rewrote original capture identity or typed input paths")
	}
}

func namedBinaryTraceFixture(firstPID uint32) []byte {
	var out bytes.Buffer
	header := make([]byte, 12)
	binary.LittleEndian.PutUint16(header, 0x0ace)
	header[2] = 1
	binary.LittleEndian.PutUint16(header[4:], 1)
	binary.LittleEndian.PutUint32(header[8:], 2)
	out.Write(header)
	segment := func(kind uint32, body []byte) {
		var head [8]byte
		binary.LittleEndian.PutUint32(head[:4], kind)
		binary.LittleEndian.PutUint32(head[4:], uint32(len(body)))
		out.Write(head[:])
		out.Write(body)
	}
	format := "name: sched_wakeup\nID: 10\nformat:\n"
	for _, field := range []struct {
		name         string
		offset, size int
	}{
		{"unsigned short common_type", 0, 2}, {"unsigned char common_flags", 2, 1},
		{"unsigned char common_preempt_count", 3, 1}, {"int common_pid", 4, 4},
		{"char comm[16]", 8, 16}, {"int pid", 24, 4}, {"int prio", 28, 4}, {"int target_cpu", 32, 4},
	} {
		signed := 0
		if strings.HasPrefix(field.name, "int ") {
			signed = 1
		}
		format += fmt.Sprintf("\tfield:%s;\toffset:%d;\tsize:%d;\tsigned:%d;\n", field.name, field.offset, field.size, signed)
	}
	format += "print fmt: \"comm=%s pid=%d prio=%d target_cpu=%03d\"\n"
	segment(1, []byte(format))
	segment(2, []byte("42 worker\n"))
	segment(3, []byte("42 42\n"))
	var raw bytes.Buffer
	for i := 0; i < 12; i++ {
		page := make([]byte, 4096)
		binary.LittleEndian.PutUint64(page, uint64(i+1)*1_000_000_000)
		binary.LittleEndian.PutUint64(page[8:], 42)
		binary.LittleEndian.PutUint16(page[21:], 36)
		payload := page[23:59]
		binary.LittleEndian.PutUint16(payload, 10)
		binary.LittleEndian.PutUint32(payload[4:], 42)
		copy(payload[8:24], "worker")
		binary.LittleEndian.PutUint32(payload[24:], firstPID+uint32(i))
		binary.LittleEndian.PutUint32(payload[28:], 120)
		raw.Write(page)
	}
	segment(4, raw.Bytes())
	return out.Bytes()
}
