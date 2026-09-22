package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the prepared-attachment transport, the real two-step controller,
// BaseAgent message construction and the registered production emitters. The
// scripted adapter supplies model outputs only; no PerfBundle is fabricated.
type scopedPerfExcerptLLM struct {
	segments           []tool.PerfSegment
	prompts            []string
	cancel             context.CancelFunc
	cancelInExtraction bool
	checkCanonical     func()
	beforeExtraction   func()
}

func (l *scopedPerfExcerptLLM) Chat(ctx context.Context, messages []llm.Message, _ []llm.ToolSchema, _ llm.ChatOptions) (llm.Response, error) {
	if l.checkCanonical != nil {
		l.checkCanonical()
	}
	var b strings.Builder
	for _, message := range messages {
		if message.Role == "user" {
			b.WriteString(message.Content)
			b.WriteByte('\n')
		}
	}
	l.prompts = append(l.prompts, b.String())
	call := len(l.prompts)
	if call == 1 {
		params, _ := json.Marshal(map[string]any{"segments": l.segments})
		return llm.Response{ToolCalls: []llm.ToolCall{{ID: "partition", Name: "emit_perf_segmentation", Params: params}}}, nil
	}
	if l.cancelInExtraction {
		l.cancel()
		return llm.Response{}, ctx.Err()
	}
	if call == 2 && l.beforeExtraction != nil {
		l.beforeExtraction()
	}
	params, _ := json.Marshal(map[string]any{
		"meta": map[string]any{"source": "ftrace"},
		"observations": []map[string]any{{"kind": "runtime_event", "subject": fmt.Sprintf("region-%d", call-1),
			"summary": fmt.Sprintf("observed region %d", call-1), "evidence": fmt.Sprintf("Region%d", call-1), "confidence": 0.9,
			"line_start": 1, "line_end": 2}},
	})
	return llm.Response{ToolCalls: []llm.ToolCall{{ID: fmt.Sprintf("extract-%d", call), Name: "emit_perf_trace", Params: params}}}, nil
}

func (*scopedPerfExcerptLLM) ModelID() string               { return "scoped-perf-excerpt-public" }
func (*scopedPerfExcerptLLM) MaxContextTokens() int         { return 128000 }
func (*scopedPerfExcerptLLM) MaxOutputTokens() int          { return 4096 }
func (*scopedPerfExcerptLLM) RequestTimeout() time.Duration { return 0 }
func (*scopedPerfExcerptLLM) RetryMaxAttempts() int         { return 0 }

type scopedPerfExcerptEmit struct {
	tool.EmitPerfTrace
	partials       []*types.PerfBundle
	afterFirst     context.CancelFunc
	checkCanonical func(*types.BusContext)
}

func (e *scopedPerfExcerptEmit) Execute(ctx *types.BusContext, params json.RawMessage) (types.ToolResult, error) {
	if e.checkCanonical != nil {
		e.checkCanonical(ctx)
	}
	result, err := e.EmitPerfTrace.Execute(ctx, params)
	if err == nil && result.Success {
		// Snapshot the actual emitter result before the controller merges it.
		data, _ := json.Marshal(ctx.Mutable.PerfTrace())
		var bundle types.PerfBundle
		if decodeErr := json.Unmarshal(data, &bundle); decodeErr != nil {
			return result, decodeErr
		}
		e.partials = append(e.partials, &bundle)
		if len(e.partials) == 1 && e.afterFirst != nil {
			e.afterFirst()
		}
	}
	return result, err
}

type scopedPerfExcerptFixture struct {
	ctx                   *types.AgentContext
	material              *attachment.TraceMaterial
	preview, source, path string
	starts                []int
	adapter               *scopedPerfExcerptLLM
	emitter               *scopedPerfExcerptEmit
	agent                 Agent
}

func newScopedPerfExcerptFixture(t *testing.T, runCtx context.Context) scopedPerfExcerptFixture {
	t.Helper()
	root := t.TempDir()
	first := " app-100 (100) [000] .... 10.000000: tracing_mark_write: B|100|Region1\n" +
		" app-100 (100) [000] .... 10.005000: tracing_mark_write: E|100\n"
	second := " worker-200 (200) [001] .... 20.000000: tracing_mark_write: B|200|Region2\n" +
		" worker-200 (200) [001] .... 20.007000: tracing_mark_write: E|200\n"
	source := "# tracer: nop\n" + first + second
	path := filepath.Join(root, "capture.systrace")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	material, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: path, PreviewBytes: 8192})
	if err != nil {
		t.Fatal(err)
	}
	preview := material.Preview()
	starts := []int{strings.Index(preview, first), strings.Index(preview, second)}
	if starts[0] < 0 || starts[1] <= starts[0] || !strings.HasSuffix(preview, second) {
		t.Fatal("fixture preparation did not preserve the complete two-region text")
	}
	ctx := &types.AgentContext{AgentName: types.AgentPerfTriager, Stage: types.StagePerfTriage, Ctx: runCtx,
		RepoRoot: root, WorkDir: root, AttachedHitrace: preview, AttachedTraceMaterial: material,
		Mutable: types.NewMutableState("inspect both trace regions")}
	adapter := &scopedPerfExcerptLLM{segments: []tool.PerfSegment{
		{ByteStart: starts[0], ByteEnd: starts[1], Kind: "thread_run"},
		{ByteStart: starts[1], ByteEnd: len(preview), Kind: "thread_run"},
	}}
	emitter := &scopedPerfExcerptEmit{}
	adapter.checkCanonical = func() {
		if ctx.AttachedHitrace != preview || ctx.AttachedTraceMaterial != material || ctx.AttachedTraceExcerpt != nil {
			t.Error("model dispatch changed canonical parent attachment fields")
		}
	}
	emitter.checkCanonical = func(bus *types.BusContext) {
		adapter.checkCanonical()
		if bus.AttachedHitrace != preview || bus.AttachedTraceMaterial != material {
			t.Error("registered emitter received a rewritten canonical attachment instead of a scoped view")
		}
	}
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitPerfSegmentation{})
	registry.Register(emitter)
	skills := skill.NewRegistry()
	skills.Register(&skill.Config{Name: "perf-segmentation-skill", ToolSuggestions: []string{"emit_perf_segmentation"}})
	skills.Register(&skill.Config{Name: "perf-triage-skill", ToolSuggestions: []string{"emit_perf_trace"}})
	agent := NewPerfTriagerAgent(&Dependencies{LLM: adapter, Tools: registry, Skills: skills, Emit: func(render.Event) {}},
		PerfTriageSettings{Enabled: true, MinBytes: 1, MaxRetries: 1, TwoStepEnabled: true,
			TwoStepBytes: 1, TwoStepCoverage: 0.3, MaxLLMCalls: 4, LLMMaxBytes: 8192})
	return scopedPerfExcerptFixture{ctx: ctx, material: material, preview: preview, source: source, path: path,
		starts: starts, adapter: adapter, emitter: emitter, agent: agent}
}

func (f scopedPerfExcerptFixture) assertRestored(t *testing.T) {
	t.Helper()
	if f.ctx.AttachedHitrace != f.preview || f.ctx.AttachedTraceMaterial != f.material {
		t.Error("sub-dispatch did not restore the exact parent preview/material identity")
	}
	if f.ctx.AttachedTraceExcerpt != nil {
		t.Error("sub-dispatch left a child excerpt on the parent context")
	}
	if err := f.material.Validate(context.Background(), f.ctx.AttachedHitrace); err != nil {
		t.Errorf("parent preparation receipt no longer validates: %v", err)
	}
	if data, err := os.ReadFile(f.path); err != nil || string(data) != f.source {
		t.Errorf("triage modified the physical attachment: err=%v", err)
	}
	if len(f.ctx.Mutable.PerfSegments()) != 0 {
		t.Error("completed/cancelled fan-out leaked segmentation state into the parent")
	}
}

func TestPerfTriagePreparedScopedExcerptPublic(t *testing.T) {
	f := newScopedPerfExcerptFixture(t, context.Background())
	out, err := f.agent.Execute(f.ctx, nil)
	if err != nil || out == nil || out.Error != "" || len(f.adapter.prompts) != 3 || len(f.emitter.partials) != 2 {
		t.Fatalf("real two-step fixture failed: err=%v out=%+v calls=%d emitted=%d", err, out, len(f.adapter.prompts), len(f.emitter.partials))
	}
	f.assertRestored(t)
	t.Run("actual_model_content", func(t *testing.T) {
		if !strings.Contains(f.adapter.prompts[0], "Region1") || !strings.Contains(f.adapter.prompts[0], "Region2") {
			t.Fatal("segmentation dispatch did not receive both original regions")
		}
		for i, prompt := range f.adapter.prompts[1:] {
			own, other := fmt.Sprintf("Region%d", i+1), fmt.Sprintf("Region%d", 2-i)
			if !strings.Contains(prompt, own) || strings.Contains(prompt, other) {
				t.Errorf("segment %d must expose only its own trace content (own=%v other=%v)", i+1, strings.Contains(prompt, own), strings.Contains(prompt, other))
			}
			if strings.Contains(prompt, "changed or became unavailable") || strings.Contains(prompt, "Reattach the original source") {
				t.Errorf("segment %d falsely reports unchanged prepared material as changed", i+1)
			}
		}
	})
	t.Run("time_and_coordinate_scope", func(t *testing.T) {
		for i, partial := range f.emitter.partials {
			var found bool
			for _, obs := range partial.Observations {
				segment := f.adapter.segments[i]
				wantLine := strings.Count(f.preview[:f.starts[i]], "\n") + 1
				scope := obs.SourceScope
				if scope == nil || scope.ParentPreviewSHA256 != fmt.Sprintf("%x", sha256.Sum256([]byte(f.preview))) ||
					scope.ParentPreviewBytes != len(f.preview) || scope.ByteStart != segment.ByteStart || scope.ByteEnd != segment.ByteEnd ||
					scope.LineCoordinates != "parent_preview" || scope.LineStart != wantLine || scope.LineEnd != wantLine+1 {
					t.Errorf("segment %d observation lost its exact preview-only source scope: %+v", i+1, scope)
				}
				if obs.LineStart != wantLine || obs.LineEnd != wantLine+1 {
					t.Errorf("segment %d must preserve parent-preview coordinates %d..%d rather than local 1..2: %+v", i+1, wantLine, wantLine+1, obs)
				}
				if obs.Kind == "runtime_event" && obs.Authority != types.PerfObservationAuthorityPreTriageModelExtraction {
					t.Errorf("segment %d source scope promoted a model observation to measured authority: %s", i+1, obs.Authority)
				}
				if obs.Kind != "time_semantics" {
					continue
				}
				found = true
				wantStart, wantDuration := float64((i+1)*10000), float64(5+2*i)
				if math.Abs(obs.StartTsMs-wantStart) > 1e-6 || math.Abs(obs.DurationMs-wantDuration) > 1e-6 {
					t.Errorf("segment %d changed measured timestamp units/extent: %+v", i+1, obs)
				}
				if strings.Contains(obs.Summary, "whole attached excerpt") {
					t.Errorf("segment %d promotes its extent to the whole attachment", i+1)
				}
			}
			if !found {
				t.Errorf("segment %d lost its deterministic time/unit observation", i+1)
			}
		}
	})
	t.Run("merged_observations", func(t *testing.T) {
		bundle := f.ctx.Mutable.PerfTrace()
		if bundle == nil {
			t.Fatal("missing merged bundle")
		}
		wantCoverage := types.PerfExtractionCoverage{ParentPreviewBytes: len(f.preview), Segments: 2, Attempted: 2, Succeeded: 2,
			ExtractedPreviewBytes: len(f.preview) - f.starts[0]}
		if bundle.ExtractionCoverage == nil || *bundle.ExtractionCoverage != wantCoverage {
			t.Errorf("successful extraction accounting must exclude unselected preview headers: got=%+v want=%+v", bundle.ExtractionCoverage, wantCoverage)
		}
		seen := map[string]bool{}
		rows := map[string]int{}
		for _, obs := range bundle.Observations {
			seen[obs.Subject] = true
			data, err := json.Marshal(obs)
			if err != nil {
				t.Fatal(err)
			}
			rows[string(data)]++
		}
		for _, subject := range []string{"region-1", "region-2"} {
			if !seen[subject] {
				t.Errorf("two-segment merge lost accepted observation %q", subject)
			}
		}
		// Preserve each emitter-owned observation, including both separately
		// scoped time ranges and any later source-scope metadata. A merge must
		// not retain only model summaries while dropping the numeric provenance.
		for i, partial := range f.emitter.partials {
			for _, obs := range partial.Observations {
				data, err := json.Marshal(obs)
				if err != nil {
					t.Fatal(err)
				}
				if rows[string(data)] != 1 {
					t.Errorf("segment %d accepted %s observation must survive exactly once with its original scope/values: count=%d", i+1, obs.Kind, rows[string(data)])
				}
			}
		}
	})
}

func TestPerfTriagePreparedScopedExcerptCancellationPublic(t *testing.T) {
	for _, during := range []bool{false, true} {
		t.Run(fmt.Sprintf("during_extraction_%v", during), func(t *testing.T) {
			runCtx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := newScopedPerfExcerptFixture(t, runCtx)
			if during {
				f.adapter.cancel, f.adapter.cancelInExtraction = cancel, true
			} else {
				f.emitter.afterFirst = cancel
			}
			_, err := f.agent.Execute(f.ctx, nil)
			if !errors.Is(err, context.Canceled) {
				t.Errorf("cancellation must propagate rather than become successful/degraded triage: %v", err)
			}
			if len(f.adapter.prompts) != 2 {
				t.Errorf("cancellation dispatched another segment: calls=%d want=2", len(f.adapter.prompts))
			}
			if f.ctx.Mutable.PerfTrace() != nil {
				t.Error("cancellation published a temporary/partial performance bundle")
			}
			f.assertRestored(t)
		})
	}
}

func TestPerfTriagePreparedScopedExcerptSourceRewritePublic(t *testing.T) {
	for _, during := range []bool{false, true} {
		t.Run(fmt.Sprintf("during_extraction_%v", during), func(t *testing.T) {
			f := newScopedPerfExcerptFixture(t, context.Background())
			rewritten := strings.Replace(f.source, "Region2", "Altered", 1)
			if rewritten == f.source || len(rewritten) != len(f.source) {
				t.Fatal("fixture must replace source bytes without changing the file size")
			}
			wrote := false
			rewrite := func() {
				if err := os.WriteFile(f.path, []byte(rewritten), 0o600); err != nil {
					t.Errorf("fixture source rewrite failed: %v", err)
					return
				}
				wrote = true
				if err := f.material.Validate(context.Background(), f.preview); err == nil {
					t.Error("fixture source rewrite did not invalidate its prepared generation")
				}
			}
			if during {
				f.adapter.beforeExtraction = rewrite
			} else {
				f.emitter.afterFirst = rewrite
			}
			_, err := f.agent.Execute(f.ctx, nil)
			if !wrote {
				t.Fatal("did not reach the actual extraction boundary for source replacement")
			}
			if err == nil {
				t.Error("changed prepared source became successful/degraded triage instead of an invalid generation")
			}
			if len(f.adapter.prompts) != 2 {
				t.Errorf("invalid prepared source caused additional model calls: got=%d want=2", len(f.adapter.prompts))
			}
			if f.ctx.Mutable.PerfTrace() != nil || len(f.ctx.Mutable.PerfSegments()) != 0 {
				t.Error("invalid prepared source left a published partial bundle or temporary segmentation")
			}
			f.adapter.checkCanonical()
			if data, readErr := os.ReadFile(f.path); readErr != nil || string(data) != rewritten {
				t.Errorf("triage rewrote or restored externally replaced source bytes: %v", readErr)
			}
		})
	}
}
