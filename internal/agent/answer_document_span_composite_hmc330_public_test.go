package agent

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tracebundle"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Exercise the public producer-to-finalizer bridge without a model answer or
// a fabricated observation receipt. The adapter stops before any model call.
func TestHMC330CompositeSpanActualFinalizerAndEmit(t *testing.T) {
	ctx, _ := hmc320BusinessFinalizer(t)
	ctx.AnalysisIR.RequestModel.RuntimeQuestionProfile.FactFamilies = []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}
	ctx.Mutable.SetRequestModel(ctx.AnalysisIR.RequestModel)
	dir, err := filepath.EvalSymlinks(ctx.RepoRoot)
	if err != nil {
		t.Fatal(err)
	}
	var body strings.Builder
	for i := 0; i < 3; i++ {
		start := 5.001 + float64(i)*0.003
		fmt.Fprintf(&body, "business-700 (600) [003] .... %.6f: tracing_mark_write: B|600|SharedWork\n", start)
		fmt.Fprintf(&body, "business-700 (600) [003] .... %.6f: tracing_mark_write: E|600\n", start+float64(i+1)/1000)
	}
	original := []byte(body.String())
	digest := sha256.Sum256(original)
	sha := hex.EncodeToString(digest[:])
	physicalByID := map[string]string{}
	queryByID := map[string]string{}
	for i := 0; i < 2; i++ {
		child := filepath.Join(dir, fmt.Sprintf("physical-%d.systrace", i))
		if err := os.WriteFile(child, original, 0o600); err != nil {
			t.Fatal(err)
		}
		member := tracebundle.CaptureMember{Type: "systrace", Path: filepath.Base(child), Bytes: int64(len(original)), SHA256: sha}
		capture, err := tracebundle.CaptureID([]tracebundle.CaptureMember{member})
		if err != nil {
			t.Fatal(err)
		}
		manifest, err := json.Marshal(map[string]any{"schema": tracebundle.SchemaV2, "capture_id": capture, "systrace": member.Path,
			"artifacts": []any{map[string]any{"type": member.Type, "path": member.Path, "bytes": member.Bytes, "sha256": member.SHA256}}})
		if err != nil {
			t.Fatal(err)
		}
		bundle := filepath.Join(dir, fmt.Sprintf("capture-%d.tracebundle.json", i))
		if err := os.WriteFile(bundle, manifest, 0o600); err != nil {
			t.Fatal(err)
		}
		result := hmc320BusinessQuery(t, ctx, bundle, map[string]any{"view": "span_window", "span_name": "SharedWork", "pid": 700})
		count := 0
		for _, record := range result.Observations {
			if record.Predicate != types.TraceBusinessSpanPredicate {
				continue
			}
			count++
			if record.SourceRef.Path != bundle || record.Subject != "business-700" || len(record.SupportRefs) != 1 || !strings.HasPrefix(record.SupportRefs[0], child+":") {
				t.Fatalf("producer confused physical member with query capture: %+v", record)
			}
			physicalByID[record.ID], queryByID[record.ID] = child, bundle
		}
		if count != 3 {
			t.Fatalf("native pair roster=%d want3 for %s", count, bundle)
		}
	}
	choices := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract.Choices()
	if len(choices) != 6 || len(physicalByID) != 6 {
		t.Fatalf("six independent native instances missing: choices=%d originals=%d", len(choices), len(physicalByID))
	}
	before, _ := json.Marshal(answerDocObservationLedger(ctx))
	projectionBefore := hmc218CausalProjection(t, ctx)
	initial := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	actual := dependencyObservationMessages(t, ctx)
	for _, table := range choices {
		selector := fmt.Sprintf("observation_id=%q view=%q", table.ObservationID, table.View)
		for stage, prompt := range map[string]string{"initial": initial, "adapter": actual} {
			if !strings.Contains(prompt, selector) || !strings.Contains(prompt, physicalByID[table.ObservationID]) || !strings.Contains(prompt, queryByID[table.ObservationID]) {
				t.Errorf("%s handoff lost exact pair selector or either source coordinate: %s", stage, selector)
			}
		}
		if len(table.Rows) != 6 || table.View != types.RuntimeMeasurementSummary {
			t.Fatalf("pair lookup gained unqueried state/self facts: %+v", table)
		}
	}
	blocks := []any{map[string]any{"id": "lead", "kind": "summary", "text": "下表分别保留各原件中三个已配对业务区间；墙钟区间不证明CPU状态或根因。"}}
	wanted := map[string]types.RuntimeMeasurementTable{}
	for i, table := range choices {
		wanted[table.ObservationID] = table
		blocks = append(blocks, map[string]any{"id": fmt.Sprintf("pair-%d", i), "kind": "table",
			"runtime_measurement": map[string]any{"observation_id": table.ObservationID, "view": table.View}})
	}
	params, _ := json.Marshal(map[string]any{"blocks": blocks})
	out, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), params)
	if err != nil || !out.Success {
		t.Fatalf("public emit failed: %v %s", err, out.Summary)
	}
	doc := ctx.Mutable.AnswerDocumentV2()
	seen := map[string]bool{}
	for _, block := range doc.Blocks {
		if block.RuntimeMeasurement == nil {
			continue
		}
		table, exists := wanted[block.RuntimeMeasurement.ObservationID]
		if !exists || seen[table.ObservationID] || !block.RuntimeMeasurement.IsBound() ||
			!reflect.DeepEqual(table.Rows, block.RuntimeMeasurement.BoundTable.Rows) || !reflect.DeepEqual(table.Notes, block.RuntimeMeasurement.BoundTable.Notes) {
			t.Fatalf("emit changed or merged a source-bound original: %+v", block)
		}
		seen[table.ObservationID] = true
	}
	if len(seen) != 6 {
		t.Fatalf("same-named physical instances lost at emit: %d want6", len(seen))
	}
	visible := html.UnescapeString(render.RenderAnswerDocument(doc, "zh"))
	for id, physical := range physicalByID {
		if !strings.Contains(visible, physical) || !strings.Contains(visible, queryByID[id]) {
			t.Errorf("render lost physical/query source separation: %s", id)
		}
	}
	after, _ := json.Marshal(answerDocObservationLedger(ctx))
	if !bytes.Equal(before, after) || !bytes.Equal(projectionBefore, hmc218CausalProjection(t, ctx)) || ctx.Mutable.TraceRootCauseReport() != nil {
		t.Fatal("native measurement presentation changed source facts or causal authority")
	}
}
