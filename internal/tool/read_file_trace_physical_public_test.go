package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/ground"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestReadFilePhysicalTraceCoordinatesPublic(t *testing.T) {
	first := " app-100 (100) [000] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120"
	last := " idle-0 (0) [000] .... 5.003000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120"
	for _, tc := range []struct {
		name, content   string
		total, lastLine int
	}{
		{"lf", "# tracer: nop\n" + first + "\n" + last + "\n", 3, 3},
		{"genuine_blank", "# tracer: nop\n" + first + "\n\n" + last + "\n\n", 5, 4},
		{"no_terminal_lf", "# tracer: nop\n" + first + "\n" + last, 3, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := physicalReadFixture(t, "capture.systrace", tc.content)
			ctx.WorkDir = filepath.Join(ctx.RepoRoot, ".codrax", "blob", "query")
			path := filepath.Join(ctx.RepoRoot, "capture.systrace")
			p, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "limit": 20})
			query, err := (&TraceQuery{}).Execute(ctx, p)
			if err != nil || !query.Success {
				t.Fatalf("trace query: %v %+v", err, query)
			}
			ctx.Mutable.AppendDispatchToolResult(query)
			payloads, err := filepath.Glob(filepath.Join(ctx.WorkDir, "trace-query-result-*.json"))
			if err != nil || len(payloads) != 1 {
				t.Fatalf("missing query payload: %v %v", payloads, err)
			}
			data, err := os.ReadFile(payloads[0])
			if err != nil {
				t.Fatal(err)
			}
			var parsed tracequery.Result
			if err := json.Unmarshal(data, &parsed); err != nil {
				t.Fatal(err)
			}
			if parsed.LineCount != tc.total || len(parsed.Events) != 2 || parsed.Events[1].Line != tc.lastLine {
				t.Fatalf("trace physical fixture drift: %+v", parsed)
			}
			read := physicalRead(t, ctx, path, 0, 0)
			if !read.Success || read.ReadCoverage != nil || read.RuntimeArtifactRead == nil || read.RuntimeArtifactRead.Kind != "trace" || read.RuntimeArtifactRead.TotalLines != parsed.LineCount || !strings.Contains(read.Summary, fmt.Sprintf("%6d│ %s", tc.lastLine, last)) {
				t.Fatalf("ReadFile and TraceQuery disagree on physical lines or authority: %+v", read)
			}
			if _, ok := ctx.Mutable.ResolveTraceQueryBlobRef(payloads[0]); !ok {
				t.Fatal("actual query publication did not register its payload")
			}
			blob := physicalRead(t, ctx, payloads[0], 0, 0)
			if !blob.Success || blob.ReadCoverage != nil || blob.RuntimeArtifactRead == nil || !blob.RuntimeArtifactRead.TraceQueryBlob || len(blob.Observations) != 0 {
				t.Fatalf("registered result role/source boundary changed: %+v", blob)
			}
		})
	}
}

func TestReadFilePhysicalInlinePagingPublic(t *testing.T) {
	var content strings.Builder
	for n := 1; n <= 400; n++ {
		fmt.Fprintf(&content, "row%03d %s\n", n, strings.Repeat("x", 94))
	}
	ctx := physicalReadFixture(t, "source.go", content.String())
	first := physicalRead(t, ctx, "source.go", 0, 400)
	if !first.Success || first.ReadCoverage == nil || first.ReadCoverage.TotalLines != 400 || first.ReadCoverage.LineEnd >= 400 || first.Refinement == nil || first.Refinement.NextCursor != strconv.Itoa(first.ReadCoverage.LineEnd) {
		t.Fatalf("clamp/continuation contract changed: %+v", first)
	}
	end := first.ReadCoverage.LineEnd
	next := physicalRead(t, ctx, "source.go", end, 400)
	if !next.Success || next.ReadCoverage.LineStart != end+1 || next.ReadCoverage.LineEnd != 400 || next.ReadCoverage.TotalLines != 400 || next.Refinement != nil || !strings.Contains(next.Summary, "row400 ") {
		t.Fatalf("continuation skipped or added a physical row: %+v", next)
	}
}

func TestReadFilePhysicalEmptyRefreshPublic(t *testing.T) {
	ctx := physicalReadFixture(t, "source.go", "")
	read := physicalRead(t, ctx, "source.go", 0, 0)
	ctx.Mutable.AppendDispatchToolResult(read)
	closure := types.NewEvidenceClosure(ctx.RepoRoot)
	ground.RefreshClosureCoverage(ctx, closure)
	assertPhysicalEmptyClosure(t, closure)
}
