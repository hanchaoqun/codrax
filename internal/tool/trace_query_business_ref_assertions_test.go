package tool

import (
	"bytes"
	"compress/gzip"
	"context"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	promptctx "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/traceinput"
	"github.com/hanchaoqun/codrax/internal/types"
)

func businessRefAssertionFixture(t *testing.T) (*types.BusContext, types.TraceBusinessSpanRef) {
	t.Helper()
	path, err := filepath.Abs("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	material, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: path, PreviewBytes: 512})
	if err != nil {
		t.Fatal(err)
	}
	ctx := &types.BusContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Mutable: types.NewMutableState("exact reference assertions"), AttachedTraceMaterial: material, AttachedHitrace: material.Preview()}
	r := businessRefTestQuery(t, ctx, map[string]any{"source": "attached_trace", "view": "span_window", "span_name": "OpenDocument"})
	for _, ref := range r.TraceBusinessSpanRefs {
		if ref.Data().Name == "OpenDocument" {
			return ctx, ref
		}
	}
	t.Fatalf("missing public reference: %s", r.Summary)
	return nil, types.TraceBusinessSpanRef{}
}

func TestTraceQueryBusinessRefMatchingAssertionsKeepExactNativeAccount(t *testing.T) {
	ctx, ref := businessRefAssertionFixture(t)
	d := ref.Data()
	for _, view := range []string{"window_stats", "wakeup_chain", "root_cause_rank"} {
		t.Run(view, func(t *testing.T) {
			base := businessRefTestQuery(t, ctx, map[string]any{"view": view, "business_span_ref": ref.Token()})
			want := carrierNativePayload(t, base, ctx.WorkDir)
			variants := map[string]map[string]any{
				"live-shape":       {"source": "attached_trace", "thread": d.Thread, "pid": d.TID},
				"all-tuple-fields": {"source": "attached_trace", "path": d.Path, "thread": d.Thread, "pid": d.TID, "target_scope": "thread", "span_name": d.Name, "time_start": d.StartTs, "time_end": d.EndTs, "line_start": d.StartLine, "line_end": d.EndLine},
				"path-mode":        {"source": "path"},
				"unit-string":      {"time_start": "1000ms", "time_end": "1050ms"},
			}
			for name, params := range variants {
				t.Run(name, func(t *testing.T) {
					params["view"], params["business_span_ref"] = view, ref.Token()
					got := businessRefTestQuery(t, ctx, params)
					if !got.Success {
						t.Fatalf("equal assertions rejected: %s", got.Summary)
					}
					payload := carrierNativePayload(t, got, ctx.WorkDir)
					if !reflect.DeepEqual(payload.TargetWindowStates, want.TargetWindowStates) || !reflect.DeepEqual(payload.WindowStats, want.WindowStats) || !reflect.DeepEqual(payload.WakeupChain, want.WakeupChain) || !reflect.DeepEqual(payload.RootCauseRank, want.RootCauseRank) {
						t.Fatalf("redundant assertions changed the native result for %s", view)
					}
					if payload.WindowStats != nil && math.Abs((payload.WindowStats.Window.EndTs-payload.WindowStats.Window.StartTs)*1000-50) > 1e-6 {
						t.Fatalf("lost exact 50ms window: %+v", payload.WindowStats.Window)
					}
				})
			}
		})
	}
	if status, selected := ctx.Mutable.AcceptedTraceBusinessFocus(); status != types.TraceBusinessFocusNone || selected.Token() != "" {
		t.Fatal("query assertions accepted a completion focus")
	}
}

func TestTraceQueryBusinessRefConflictingAssertionsDoNotSeedQueries(t *testing.T) {
	ctx, ref := businessRefAssertionFixture(t)
	d := ref.Data()
	other := filepath.Join(ctx.WorkDir, "same-name-other-capture.systrace")
	bytes, err := os.ReadFile(d.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, bytes, 0600); err != nil {
		t.Fatal(err)
	}
	for key, value := range map[string]any{
		"pid": 200, "thread": "app", "target_scope": "process", "span_name": "Document", "time_start": 1.000000001, "time_end": 1.051, "line_start": d.StartLine + 1, "line_end": d.EndLine + 1, "path": other, "source": "unknown",
	} {
		t.Run(key, func(t *testing.T) {
			before := ctx.Mutable.TraceQueryCallWindows()
			r := businessRefTestQuery(t, ctx, map[string]any{"view": "window_stats", "business_span_ref": ref.Token(), key: value})
			if r.Success || !strings.Contains(r.Summary, "business_span_ref") || len(r.Observations) != 0 || len(r.TraceBusinessSpanRefs) != 0 || !reflect.DeepEqual(before, ctx.Mutable.TraceQueryCallWindows()) {
				t.Fatalf("conflict mutated query authority: %+v", r)
			}
		})
	}
	// A matching attached selector must not hide a conflicting explicit path.
	r := businessRefTestQuery(t, ctx, map[string]any{"view": "window_stats", "business_span_ref": ref.Token(), "source": "attached_trace", "path": other})
	if r.Success {
		t.Fatal("attached source hid a different explicitly named capture")
	}
}

func TestTraceQueryBusinessRefAssertionSourceSelectionIsIndependent(t *testing.T) {
	ctx, ref := businessRefAssertionFixture(t)
	d := ref.Data()
	other := filepath.Join(ctx.WorkDir, "other.systrace")
	body, err := os.ReadFile(d.Path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(other, body, 0600); err != nil {
		t.Fatal(err)
	}
	material, err := traceinput.Prepare(context.Background(), traceinput.Options{InputPath: other, PreviewBytes: 512})
	if err != nil {
		t.Fatal(err)
	}
	ctx.AttachedTraceMaterial, ctx.AttachedHitrace = material, material.Preview()
	params := map[string]any{"view": "window_stats", "business_span_ref": ref.Token(), "source": "attached_trace", "path": d.Path}
	if r := businessRefTestQuery(t, ctx, params); r.Success {
		t.Fatal("matching path hid a conflicting prepared attachment")
	}
	params["source"] = "path"
	if r := businessRefTestQuery(t, ctx, params); !r.Success {
		t.Fatalf("unrelated attachment blocked original path: %s", r.Summary)
	}
	// If a concrete attachment exists, an unrelated typed referenced file must
	// not override it simply because that file happens to match the reference.
	ctx.AttachedTraceMaterial, ctx.AttachedHitrace = nil, ""
	ctx.AnalysisIR = &types.AnalysisIR{EvidencePlan: types.EvidencePlan{RequiredFiles: []string{d.Path}}}
	blob := filepath.Join(ctx.WorkDir, promptctx.AttachedTraceBlobName)
	if err := os.WriteFile(blob, body, 0600); err != nil {
		t.Fatal(err)
	}
	delete(params, "path")
	params["source"] = "attached_trace"
	if r := businessRefTestQuery(t, ctx, params); r.Success {
		t.Fatal("referenced artifact masked a different concrete attachment")
	}
	if err := os.Remove(blob); err != nil {
		t.Fatal(err)
	}
	if r := businessRefTestQuery(t, ctx, params); !r.Success {
		t.Fatalf("unique typed referenced artifact failed: %s", r.Summary)
	}

	alias := filepath.Join(ctx.WorkDir, "capture-alias.systrace")
	if err := os.Symlink(d.Path, alias); err != nil {
		t.Fatal(err)
	}
	params["source"], params["path"] = "path", alias
	if r := businessRefTestQuery(t, ctx, params); !r.Success {
		t.Fatalf("exact physical alias failed: %s", r.Summary)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, alias); err != nil {
		t.Fatal(err)
	}
	if r := businessRefTestQuery(t, ctx, params); r.Success {
		t.Fatal("redirected alias accepted old receipt")
	}
}

func TestTraceQueryBusinessRefAssertionsRetainExplicitScopeAndLifetime(t *testing.T) {
	ctx, ref := businessRefAssertionFixture(t)
	d := ref.Data()
	params := map[string]any{"view": "window_stats", "business_span_ref": ref.Token(), "source": "attached_trace", "pid": d.TID, "thread": d.Thread, "time_start": d.StartTs, "time_end": d.EndTs}
	start, end := 1.01, 1.04
	ctx.Mutable.SetRequestModel(types.RequestModel{RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, SourceQuote: "narrow window", TimeStart: &start, TimeEnd: &end}})
	if r := businessRefTestQuery(t, ctx, params); r.Success || !strings.Contains(r.Summary, "explicitly requested") {
		t.Fatalf("matching fields bypassed user window: %+v", r)
	}
	ctx.Mutable.SetRequestModel(types.RequestModel{})
	if r := businessRefTestQuery(t, ctx, params); !r.Success {
		t.Fatalf("control query failed: %s", r.Summary)
	}
	ctx.Mutable.ResetTurnAArtifacts()
	if r := businessRefTestQuery(t, ctx, params); r.Success || len(r.Observations) != 0 {
		t.Fatal("matching coordinates rebuilt a revoked reference")
	}

	local, path := businessRefTestContext(t, "# tracer: nop\nworker-200 (100) [001] .... 1.000000: tracing_mark_write: B|100|Work\nworker-200 (100) [001] .... 1.050000: tracing_mark_write: E|100\n")
	r := businessRefTestQuery(t, local, map[string]any{"view": "span_window", "path": path, "span_name": "Work"})
	if len(r.TraceBusinessSpanRefs) != 1 {
		t.Fatal("missing scheduler TID reference")
	}
	q := map[string]any{"view": "window_stats", "business_span_ref": r.TraceBusinessSpanRefs[0].Token(), "pid": 100}
	if got := businessRefTestQuery(t, local, q); got.Success {
		t.Fatal("marker TGID substituted for executing scheduler TID")
	}
	q["pid"] = 200
	if got := businessRefTestQuery(t, local, q); !got.Success {
		t.Fatalf("true TID assertion failed: %s", got.Summary)
	}
	if err := os.WriteFile(path, []byte("# replaced capture\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := businessRefTestQuery(t, local, q); got.Success {
		t.Fatal("same path and matching TID revived changed capture")
	}
}

func TestTraceQueryBusinessRefAssertionsPreparedInputAliases(t *testing.T) {
	body, err := os.ReadFile("../../eval/fixtures/hmosperf_business_io_chain/events.systrace")
	if err != nil {
		t.Fatal(err)
	}
	for _, attached := range []bool{false, true} {
		name := "coordinator"
		if attached {
			name = "attachment"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "capture.gz")
			var compressed bytes.Buffer
			writer := gzip.NewWriter(&compressed)
			if _, err := writer.Write(body); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(input, compressed.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			coordinator := traceinput.NewCoordinator(traceinput.Options{RuntimeAnchor: dir, PreviewBytes: 512})
			material, err := coordinator.Prepare(context.Background(), input)
			if err != nil {
				t.Fatal(err)
			}
			if material.QueryPath() == input {
				t.Fatal("fixture did not exercise prepared coordinates")
			}
			ctx := &types.BusContext{RepoRoot: dir, WorkDir: dir, TraceInputPreparer: coordinator, Mutable: types.NewMutableState("prepared alias assertions")}
			if attached {
				ctx.AttachedTraceMaterial, ctx.AttachedHitrace = material, material.Preview()
			}
			r := businessRefTestQuery(t, ctx, map[string]any{"view": "span_window", "source": "path", "path": input, "span_name": "OpenDocument"})
			if !r.Success || len(r.TraceBusinessSpanRefs) != 1 {
				t.Fatalf("prepared native pair missing: %s", r.Summary)
			}
			ref := r.TraceBusinessSpanRefs[0]
			q := map[string]any{"view": "window_stats", "business_span_ref": ref.Token(), "source": "path", "path": input, "pid": 100, "thread": "app-main"}
			if attached {
				q["source"] = "attached_trace"
			}
			got := businessRefTestQuery(t, ctx, q)
			if !got.Success {
				t.Fatalf("existing prepared alias rejected: %s", got.Summary)
			}
			payload := carrierNativePayload(t, got, dir)
			if payload.WindowStats == nil || payload.WindowStats.Window.EndTs != 1.05 {
				t.Fatal("prepared alias changed native response window")
			}
			copyPath := filepath.Join(dir, "unprepared-copy.gz")
			if err := os.WriteFile(copyPath, compressed.Bytes(), 0600); err != nil {
				t.Fatal(err)
			}
			q["path"] = copyPath
			before := len(coordinator.PreparedMaterials())
			if got := businessRefTestQuery(t, ctx, q); got.Success {
				t.Fatal("unprepared byte-identical copy became same capture")
			}
			if len(coordinator.PreparedMaterials()) != before {
				t.Fatal("assertion started a new preparation")
			}
			q["path"] = input
			if err := os.WriteFile(input, append(compressed.Bytes(), 'x'), 0600); err != nil {
				t.Fatal(err)
			}
			if got := businessRefTestQuery(t, ctx, q); got.Success {
				t.Fatal("stale prepared input borrowed still-existing query file")
			}
		})
	}
}

func TestTraceQueryBusinessRefAssertionsPreserveCancellationKind(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		name := "canceled"
		if deadline {
			name = "deadline_exceeded"
		}
		t.Run(name, func(t *testing.T) {
			ctx, ref := businessRefAssertionFixture(t)
			runCtx, cancel := context.WithCancel(context.Background())
			cancel()
			if deadline {
				runCtx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				cancel()
			}
			ctx.Ctx = runCtx
			for _, fields := range []map[string]any{{}, {"source": "attached_trace"}, {"path": ref.Data().Path}} {
				fields["view"], fields["business_span_ref"] = "window_stats", ref.Token()
				r := businessRefTestQuery(t, ctx, fields)
				if r.Success || r.TraceViewCancellation == nil || r.TraceViewCancellation.Reason != name || len(r.Observations) != 0 {
					t.Errorf("matching assertion changed cancellation into parameter failure: %+v", r)
				}
			}
		})
	}
}
