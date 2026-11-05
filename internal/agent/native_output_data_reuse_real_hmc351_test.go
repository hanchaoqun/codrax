package agent

import (
	"bytes"
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC351WholeCapturesNativeOutputDataReuse(t *testing.T) {
	input, out := os.Getenv("CODRAX_HMC351_REAL_INPUTS"), os.Getenv("CODRAX_HMC351_REAL_OUT")
	if input == "" {
		t.Skip("whole external captures not supplied")
	}
	if !filepath.IsAbs(input) || !filepath.IsAbs(out) {
		t.Fatal("absolute input and fresh output required")
	}
	var captures []struct {
		Name, Path, SHA256 string
		Bytes              int64
		PID, TimelineRows  int
		Start, End         float64
	}
	raw, err := os.ReadFile(input)
	if err != nil || json.Unmarshal(raw, &captures) != nil || len(captures) < 2 || os.Mkdir(out, 0700) != nil {
		t.Fatal("multiple pinned whole sources and fresh output required", err)
	}
	for _, capture := range captures {
		t.Run(capture.Name, func(t *testing.T) {
			if !filepath.IsAbs(capture.Path) || capture.PID <= 0 || capture.End <= capture.Start || capture.TimelineRows <= 0 {
				t.Fatal("invalid operator source scope")
			}
			sha, size := hmc333FullSourceHash(t, capture.Path)
			if sha != capture.SHA256 || size != capture.Bytes {
				t.Fatal("not the complete pinned original")
			}
			dir := filepath.Join(out, capture.Name)
			if filepath.Base(capture.Name) != capture.Name || os.Mkdir(dir, 0700) != nil {
				t.Fatal("fresh capture output required")
			}
			write := func(name string, value any) {
				data, err := json.MarshalIndent(value, "", "  ")
				if err != nil || os.WriteFile(filepath.Join(dir, name), append(data, '\n'), 0600) != nil {
					t.Fatal("save raw original audit", err)
				}
			}
			ctx, results := hmc346QueryContext(t, capture.Path, capture.PID, capture.Start, capture.End)
			write("actual-tool-results.json", results)
			files := map[string]bool{}
			for _, r := range results {
				files[r.RawRef] = true
				for _, record := range r.Observations {
					if record.SourceRef.PayloadRef != "" {
						files[record.SourceRef.PayloadRef] = true
					}
				}
			}
			var paths []string
			for p := range files {
				if p != "" {
					paths = append(paths, p)
				}
			}
			sort.Strings(paths)
			for i, p := range paths {
				data, err := os.ReadFile(p)
				if err != nil || os.WriteFile(filepath.Join(dir, "original-"+strconv.Itoa(i)+".json"), data, 0600) != nil {
					t.Fatal("retain raw/payload before assertions", err)
				}
			}
			tables := hmc346TimelineTables(t, ctx)
			write("native-tables.json", tables)
			if len(tables[0].Rows) != capture.TimelineRows || tables[0].NativeDisplayDataKey() == "" || tables[0].NativeDisplayDataKey() != tables[1].NativeDisplayDataKey() {
				t.Fatal("complete retained data or private equivalence changed")
			}
			ledger := answerDocObservationLedger(ctx)
			before, _ := json.Marshal(ledger)
			causal := hmc218CausalProjection(t, ctx)
			write("original-ledger.json", ledger)
			_, context := finalizerReaderFieldMessages(t, ctx)
			if err := os.WriteFile(filepath.Join(dir, "actual-finalizer-context.txt"), []byte(context), 0600); err != nil {
				t.Fatal(err)
			}
			for _, table := range tables {
				if !strings.Contains(context, table.ObservationID) {
					t.Fatal("independent selector lost before actual finalizer")
				}
			}
			submission, _ := json.Marshal(map[string]any{"blocks": []any{map[string]any{"id": "lead", "kind": "summary", "text": "以下保留各次查询的原生事实与独立范围说明。"}}})
			result, err := (&tool.EmitAnswerDocument{}).Execute(types.ToolBusContext(ctx, types.AgentFinalizer), submission)
			if err != nil || !result.Success {
				t.Fatalf("actual public emit failed: %v %s", err, result.Summary)
			}
			doc := ctx.Mutable.AnswerDocumentV2()
			docBefore, _ := json.Marshal(doc)
			text := render.RenderAnswerDocument(doc, "zh")
			if err := os.WriteFile(filepath.Join(dir, "actual-rendered-answer.md"), []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			if strings.Count(text, "| 线程 | 状态 | 开始") != 1 {
				t.Fatal("full native timeline data still duplicated in final render")
			}
			seen := map[string]bool{}
			for _, b := range doc.Blocks {
				if r := b.RuntimeMeasurement; r != nil && r.IsBound() {
					for _, table := range tables {
						if r.ObservationID == table.ObservationID {
							if !reflect.DeepEqual(r.BoundTable.Rows, table.Rows) || !reflect.DeepEqual(r.BoundTable.Notes, table.Notes) {
								t.Fatal("original full bound table changed")
							}
							seen[r.ObservationID] = true
						}
					}
				}
			}
			for _, table := range tables {
				for _, note := range table.Notes {
					if !strings.Contains(html.UnescapeString(text), note) {
						t.Fatal("complete query note omitted from rendered answer")
					}
				}
			}
			docAfter, _ := json.Marshal(doc)
			after, _ := json.Marshal(answerDocObservationLedger(ctx))
			afterSHA, afterSize := hmc333FullSourceHash(t, capture.Path)
			if len(seen) != 2 || !bytes.Equal(docBefore, docAfter) || !bytes.Equal(before, after) || !bytes.Equal(causal, hmc218CausalProjection(t, ctx)) || afterSHA != sha || afterSize != size {
				t.Fatal("display reuse changed receipt, document, facts, causal projection or original")
			}
			write("scope-audit.json", map[string]any{"source": capture.Path, "source_sha256": sha, "source_bytes": size, "source_unchanged": true, "query_groups": len(seen), "rows_each": len(tables[0].Rows), "rendered_data_tables": 1, "all_query_notes_retained": true, "document_unchanged": true, "ledger_unchanged": true, "causal_projection_unchanged": true, "model_calls": 0, "scope": "Two public full-source queries, actual Finalizer and emit/render; native data shown once with both original query receipts and notes. Not a new independent scheduler oracle or live model eval."})
		})
	}
}
