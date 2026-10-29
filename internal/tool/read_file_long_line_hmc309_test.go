package tool

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadFileLongLineHMC309Public(t *testing.T) {
	for _, path := range []string{"source.go", ".codrax/query-result.json"} {
		for _, offset := range []int{0, 1} {
			t.Run(fmt.Sprintf("%s/offset_%d", path, offset), func(t *testing.T) {
				prefix := ""
				if offset == 1 {
					prefix = "first row\n"
				}
				content := prefix + `{"value":"` + strings.Repeat("宽", MaxInlineBytes) + `","tail":42}` + "\n"
				ctx := physicalReadFixture(t, path, content)
				out := physicalRead(t, ctx, path, offset, 1)
				if out.Success || out.Repair == nil || out.Repair.Code != "read_file_physical_line_exceeds_byte_budget" {
					t.Fatalf("display budget failure must remain distinct from a missing or empty file: %+v", out)
				}
				if out.ReadCoverage != nil || len(out.Observations) != 0 {
					t.Fatalf("non-visible physical line acquired source coverage: %+v", out)
				}
				if out.EnumerationAuthority == nil || out.EnumerationAuthority.Status != "incomplete" || len(out.EnumerationAuthority.Boundaries) != 1 || out.EnumerationAuthority.Boundaries[0].Emitted != 0 || out.EnumerationAuthority.Boundaries[0].Total != offset+1 {
					t.Fatalf("omitted line was counted as complete or empty input: %+v", out)
				}
				if strings.Contains(out.Summary, "Call read_file again with line_offset=") || (out.Refinement != nil && out.Refinement.NextCursor != "") {
					t.Fatalf("line paging cannot recover omitted bytes in this line: %+v", out)
				}
				if !strings.Contains(out.Summary, "physical line") || !strings.Contains(out.Summary, "byte budget") || out.RawRef == "" {
					t.Fatalf("missing bounded-byte explanation or retained full selected bytes: %+v", out)
				}
				body, ok := LoadBlobText(out.RawRef, len(content)+1024)
				if !ok || !strings.Contains(body, `"tail":42`) {
					t.Fatal("full selected bytes were lost from the raw artifact")
				}
				if strings.HasPrefix(path, ".codrax/") && (out.RuntimeArtifactRead == nil || out.RuntimeArtifactRead.LineStart != 0 || out.RuntimeArtifactRead.LineEnd != 0) {
					t.Fatal("runtime identity must survive without claiming non-visible rows")
				}
			})
		}
	}
}

func TestReadFileLongLineNoPersistedArtifactHMC309Public(t *testing.T) {
	ctx := physicalReadFixture(t, "source.go", strings.Repeat("x", MaxInlineBytes*2))
	ctx.WorkDir = ""
	out := physicalRead(t, ctx, "source.go", 0, 1)
	if out.Success || out.RawRef != "" || !strings.Contains(out.Summary, "could not be persisted") || strings.Contains(out.Summary, "retains the selected bytes") {
		t.Fatalf("missing artifact was reported as retained: %+v", out)
	}
}

func TestReadFileLongLineHeldTraceIdentityHMC309Public(t *testing.T) {
	data := "# tracer: nop\n# " + strings.Repeat("x", MaxInlineBytes*2) + "\n" +
		" app-100 (100) [000] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=idle next_pid=0 next_prio=120\n" +
		" idle-0 (0) [000] .... 5.003000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=120\n"
	ctx := physicalReadFixture(t, "capture.go", data)
	path := filepath.Join(ctx.RepoRoot, "capture.go")
	params, _ := json.Marshal(map[string]any{"source": "path", "path": path, "view": "event_search", "time_start": 5, "time_end": 5.01, "limit": 10})
	query, err := (&TraceQuery{}).Execute(ctx, params)
	if err != nil || !query.Success || query.TraceQuerySourceRead.Path() == "" {
		t.Fatalf("public native source premise failed: %+v %v", query, err)
	}
	ctx.Mutable.AppendDispatchToolResult(query)
	out := physicalRead(t, ctx, path, 1, 1)
	if out.Success || out.ReadCoverage != nil || len(out.Observations) != 0 || out.RuntimeArtifactRead == nil || out.RuntimeArtifactRead.Kind != "trace" || out.RuntimeArtifactRead.LineStart != 0 || out.RuntimeArtifactRead.LineEnd != 0 || out.RuntimeArtifactRead.TotalLines != 4 {
		t.Fatalf("display failure lost original trace identity or acquired unseen rows: %+v", out)
	}
}

func TestReadFileBannerBudgetHMC309Public(t *testing.T) {
	oldInline, oldHead, oldTail := MaxInlineBytes, PreviewHeadBytesValue(), PreviewTailBytesValue()
	SetBlobLimits(512, 512, oldTail)
	t.Cleanup(func() { SetBlobLimits(oldInline, oldHead, oldTail) })
	var content strings.Builder
	for n := 1; n <= 10; n++ {
		fmt.Fprintf(&content, "row%d %s\n", n, strings.Repeat("x", 70))
	}
	ctx := physicalReadFixture(t, "source.go", content.String())
	for offset := 0; offset < 10; {
		out := physicalRead(t, ctx, "source.go", offset, 10)
		if !out.Success || out.ReadCoverage == nil {
			t.Fatalf("complete prefix must remain readable: %+v", out)
		}
		c := out.ReadCoverage
		if c.LineStart != offset+1 || c.LineEnd < c.LineStart || c.TotalLines != 10 {
			t.Fatalf("lost absolute source range: %+v", c)
		}
		for row := c.LineStart; row <= c.LineEnd; row++ {
			if !strings.Contains(out.Summary, fmt.Sprintf("%6d│ row%d %s", row, row, strings.Repeat("x", 70))) {
				t.Fatalf("covered row %d is not fully visible: %q", row, out.Summary)
			}
		}
		if strings.Contains(out.Summary, fmt.Sprintf("%6d│", c.LineEnd+1)) || strings.Contains(out.Summary, "Call read_file again") {
			t.Fatalf("preview exposed a partial row or blob-relative continuation: %q", out.Summary)
		}
		if c.LineEnd < 10 && (out.Refinement == nil || out.Refinement.NextCursor != fmt.Sprint(c.LineEnd)) {
			t.Fatalf("wrong source continuation: %+v", out.Refinement)
		}
		offset = c.LineEnd
	}
}

func TestReadFileLongLineDoesNotBlockOtherLinesHMC309Public(t *testing.T) {
	ctx := physicalReadFixture(t, "source.go", "before\n"+strings.Repeat("x", MaxInlineBytes*2)+"\nafter\n")
	first := physicalRead(t, ctx, "source.go", 0, 3)
	if !first.Success || first.ReadCoverage == nil || first.ReadCoverage.LineEnd != 1 || first.Refinement == nil || first.Refinement.NextCursor != "1" {
		t.Fatalf("complete leading row or exact continuation lost: %+v", first)
	}
	oversized := physicalRead(t, ctx, "source.go", 1, 1)
	if oversized.Success || oversized.ReadCoverage != nil || oversized.Refinement != nil {
		t.Fatalf("oversized row was skipped or certified: %+v", oversized)
	}
	last := physicalRead(t, ctx, "source.go", 2, 1)
	if !last.Success || last.ReadCoverage == nil || last.ReadCoverage.LineStart != 3 || !strings.Contains(last.Summary, "     3│ after") {
		t.Fatalf("display failure became a whole-source terminal gate: %+v", last)
	}
}

func TestReadFileExpandedSmallLimitRespectsDisplayBudgetHMC309Public(t *testing.T) {
	ctx := physicalReadFixture(t, "source.go", strings.Repeat("x\n", 4000))
	out := physicalRead(t, ctx, "source.go", 0, 1)
	if !out.Success || out.ReadCoverage == nil || out.ReadCoverage.LineEnd >= 4000 || out.Refinement == nil {
		t.Fatalf("small input expansion bypassed rendered-byte budget: %+v", out)
	}
	end := out.ReadCoverage.LineEnd
	if strings.Contains(out.Summary, "showing all") || !strings.Contains(out.Summary, fmt.Sprintf("%6d│ x", end)) || strings.Contains(out.Summary, fmt.Sprintf("%6d│", end+1)) {
		t.Fatalf("expanded line count differs from actual visible rows: %q", out.Summary)
	}
	if out.EnumerationAuthority.Status != "incomplete" || out.EnumerationAuthority.Boundaries[0].Emitted != end || out.Refinement.NextCursor != fmt.Sprint(end) {
		t.Fatalf("expanded preview granted a full-file receipt or wrong cursor: %+v", out)
	}
}
