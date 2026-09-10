package tool

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceResultRoleTeachingActualPublicationAndReads(t *testing.T) {
	ctx, original, refs := b1624PublishedResult(t)
	before, _ := json.Marshal(original)
	if _, ok := ctx.Mutable.ResolveTraceQueryBlobRef(ctx.AttachedHitrace); ok {
		t.Fatal("original capture must not acquire the published-result escape")
	}
	for _, ref := range refs {
		body, err := os.ReadFile(ref)
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasSuffix(ref, ".txt") {
			for name, text := range map[string]string{"published summary": original.Summary, "persisted result": string(body)} {
				if strings.Contains(text, "instead of reading this payload directly") {
					t.Errorf("%s contradicts actual allowed result readers", name)
				}
				if !strings.Contains(text, "grep") || !strings.Contains(text, "read_file") || !strings.Contains(text, "not result pagination") {
					t.Errorf("%s lacks the result/original-trace coordinate distinction", name)
				}
			}
		}
		params, _ := json.Marshal(map[string]any{"path": ref, "line_offset": 0, "limit": 100})
		read, err := (&ReadFile{}).Execute(ctx, params)
		if err != nil || !read.Success {
			t.Fatalf("actual result read: %v %+v", err, read)
		}
		if read.RuntimeArtifactRead == nil || !read.RuntimeArtifactRead.TraceQueryBlob || read.ReadCoverage != nil || len(read.Observations) != 0 {
			t.Fatal("published result read changed source/evidence role")
		}
		params, _ = json.Marshal(map[string]any{"path": ref, "pattern": "payload_ref", "fixed_string": true, "context_lines": 0})
		grep, err := (&GrepTool{}).Execute(ctx, params)
		if err != nil || !grep.Success {
			t.Fatalf("actual result grep: %v %+v", err, grep)
		}
		for name, text := range map[string]string{"read": read.Summary, "grep": grep.Summary} {
			if !strings.Contains(text, "query_result_return_navigation:") || !strings.Contains(text, "not result pagination") {
				t.Errorf("%s does not preserve the same result-role teaching", name)
			}
			if strings.Contains(text, "instead of reading this payload directly") {
				t.Errorf("%s repeats the contradictory payload instruction", name)
			}
		}
		after, _ := os.ReadFile(ref)
		if !reflect.DeepEqual(body, after) {
			t.Fatal("result reading changed published bytes")
		}
	}
	after, _ := json.Marshal(original)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("reader changed original typed result")
	}
}

func TestTraceResultRoleTeachingActualSchemaAndCaptureLineFilter(t *testing.T) {
	ctx, _, refs := b1624PublishedResult(t)
	var schema struct {
		Properties map[string]struct{ Type, Description string }
	}
	if err := json.Unmarshal((&TraceQuery{}).Parameters(), &schema); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"line_start", "line_end"} {
		field := schema.Properties[key]
		if field.Type != "integer" {
			t.Fatalf("%s type changed: %+v", key, field)
		}
		if strings.Contains(field.Description, "Optional result line window") || !strings.Contains(field.Description, "not result pagination") {
			t.Errorf("actual %s schema confuses capture line selection with result rows: %s", key, field.Description)
		}
	}
	// Field-level coordinate teaching is sufficient here. Keep the existing
	// dispatch-sensitive Description byte golden instead of duplicating it.
	body, err := os.ReadFile(refs[1])
	if err != nil {
		t.Fatal(err)
	}
	var full tracequery.Result
	if err := json.Unmarshal(body, &full); err != nil || len(full.Events) < 20 {
		t.Fatalf("actual full event result: %v count=%d", err, len(full.Events))
	}
	want := full.Events[10]
	params, _ := json.Marshal(map[string]any{"source": "path", "path": ctx.AttachedHitrace, "view": "event_search", "line_start": want.Line, "line_end": want.Line, "limit": 256})
	filtered, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !filtered.Success {
		t.Fatalf("actual capture line filter: %v %+v", err, filtered)
	}
	var payload string
	for _, row := range filtered.Observations {
		if row.SourceRef.PayloadRef != "" {
			payload = row.SourceRef.PayloadRef
			break
		}
	}
	if payload == "" {
		t.Fatal("bounded query did not provide original typed result payload")
	}
	body, err = os.ReadFile(payload)
	if err != nil {
		t.Fatal(err)
	}
	var bounded tracequery.Result
	if err := json.Unmarshal(body, &bounded); err != nil {
		t.Fatal(err)
	}
	if len(bounded.Events) != 1 || !reflect.DeepEqual(bounded.Events[0], want) {
		t.Fatalf("line selection must filter the original capture event, not index into returned rows: got=%+v want=%+v", bounded.Events, want)
	}
}
