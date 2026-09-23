package tool

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func physicalReadFixture(t *testing.T, path, content string) *types.BusContext {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	return &types.BusContext{RepoRoot: root, WorkDir: root, Mutable: types.NewMutableState("read exact file lines")}
}

func physicalRead(t *testing.T, ctx *types.BusContext, path string, offset, limit int) types.ToolResult {
	t.Helper()
	p, err := json.Marshal(map[string]any{"path": path, "line_offset": offset, "limit": limit})
	if err != nil {
		t.Fatal(err)
	}
	r, err := (&ReadFile{}).Execute(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestReadFilePhysicalLinesPublic(t *testing.T) {
	for _, tc := range []struct {
		name, content string
		lines         []string
	}{
		{"empty", "", nil},
		{"unterminated", "alpha", []string{"alpha"}},
		{"terminal_lf", "alpha\n", []string{"alpha"}},
		{"one_blank", "\n", []string{""}},
		{"two_blank", "\n\n", []string{"", ""}},
		{"real_trailing_blank", "alpha\n\n", []string{"alpha", ""}},
		{"crlf", "alpha\r\nbeta\r\n", []string{"alpha\r", "beta\r"}},
		{"crlf_blank", "alpha\r\n\r\n", []string{"alpha\r", "\r"}},
		{"mixed_unterminated", "alpha\r\nβ", []string{"alpha\r", "β"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := physicalReadFixture(t, "source.go", tc.content)
			r := physicalRead(t, ctx, "source.go", 0, 0)
			if !r.Success || r.RawRef == "" || r.ReadCoverage == nil || r.RuntimeArtifactRead != nil {
				t.Fatalf("real source read failed: %+v", r)
			}
			if r.ReadCoverage.TotalLines != len(tc.lines) {
				t.Fatalf("physical total = %d, want %d", r.ReadCoverage.TotalLines, len(tc.lines))
			}
			if r.EnumerationAuthority == nil || len(r.EnumerationAuthority.Boundaries) != 1 {
				t.Fatalf("missing precise enumeration: %+v", r.EnumerationAuthority)
			}
			b := r.EnumerationAuthority.Boundaries[0]
			if !b.TotalKnown || b.Total != len(tc.lines) || b.Emitted != len(tc.lines) || r.EnumerationAuthority.Status != "complete" {
				t.Fatalf("wrong enumeration: %+v", r.EnumerationAuthority)
			}
			body, ok := LoadBlobText(r.RawRef, 1<<20)
			if !ok {
				t.Fatal("missing visible read artifact")
			}
			if len(tc.lines) == 0 {
				if r.ReadCoverage.LineStart != 0 || r.ReadCoverage.LineEnd != 0 || strings.Contains(body, "│") || strings.Contains(body, "1-0") || r.Refinement != nil {
					t.Fatalf("empty read fabricated range or continuation: %+v %q", r, body)
				}
				for _, obs := range r.Observations {
					if obs.EvidenceScope != types.ScopeFile || obs.Span.LineStart != 0 || obs.Span.LineEnd != 0 {
						t.Fatalf("empty file minted a line observation: %+v", obs)
					}
				}
			} else if !strings.HasSuffix(body, renderWithLineGutter(tc.lines, 1)) || strings.Contains(body, fmt.Sprintf("%6d│", len(tc.lines)+1)) {
				t.Fatalf("gutter lost bytes or added EOF row: %q", body)
			}
			actual, err := os.ReadFile(filepath.Join(ctx.RepoRoot, "source.go"))
			if err != nil || string(actual) != tc.content {
				t.Fatal("read rewrote original bytes")
			}
		})
	}
}

func TestReadFilePhysicalLinesPagingPublic(t *testing.T) {
	ctx := physicalReadFixture(t, "source.go", "alpha\nbeta\n")
	for _, offset := range []int{2, 3, int(^uint(0) >> 1)} {
		t.Run(fmt.Sprintf("offset_%d", offset), func(t *testing.T) {
			r := physicalRead(t, ctx, "source.go", offset, 10)
			if r.Success || r.ReadCoverage != nil || r.RuntimeArtifactRead != nil || !strings.Contains(r.Summary, "empty range") {
				t.Fatalf("out-of-file offset became evidence: %+v", r)
			}
		})
	}
	t.Run("last_line_huge_limit", func(t *testing.T) {
		r := physicalRead(t, ctx, "source.go", 1, int(^uint(0)>>1))
		if !r.Success || r.ReadCoverage == nil || r.ReadCoverage.LineStart != 2 || r.ReadCoverage.LineEnd != 2 || r.ReadCoverage.TotalLines != 2 {
			t.Fatalf("last physical line lost: %+v", r)
		}
	})
}

func TestReadFilePhysicalEmptyEvidencePublic(t *testing.T) {
	ctx := physicalReadFixture(t, "fixtures/expected.output", "")
	ctx.Mode = types.ModePlan
	closure := ctx.Mutable.EvidenceClosure()
	appendAdvisoryReadCoverageCaveat(ctx, []types.PendingRead{{File: "fixtures/expected.output"}})
	r := physicalRead(t, ctx, "fixtures/expected.output", 0, 10)
	if !r.Success {
		t.Fatalf("empty file read failed: %+v", r)
	}
	ctx.Mutable.AppendDispatchToolResult(r)
	readSet, ranges, totals := types.ExtractReadCoverage([]types.ToolResult{r}, ctx.RepoRoot)
	closure.IngestEvidenceReducerInput(types.EvidenceReducerInput{
		Class: types.EvidenceReducerInputReadCoverageDelta, ReadSet: readSet, ReadRanges: ranges, FileTotalLines: totals,
	}, ctx.RepoRoot)
	if !closure.HasRead("fixtures/expected.output") || !closure.HasFullyRead("fixtures/expected.output") || closure.HasReadLine("fixtures/expected.output", 1) || closure.HasReadLine("fixtures/expected.output", 999) || closure.MergedReadLines("fixtures/expected.output") != 0 {
		t.Fatal("known-empty read must prove only the file, never any positive line")
	}
	if len(closure.CurrentCompletionCaveats()) != 0 || len(closure.CompletionCaveats()) == 0 {
		t.Fatal("whole-file empty read must settle display debt without rewriting history")
	}
	write, err := (&EmitWriteAnalysis{}).Execute(ctx, protectedReadAnalysisParams(t, "fixtures/expected.output"))
	if err != nil || !write.Success {
		t.Fatalf("real empty baseline read lost its physical receipt: %+v %v", write, err)
	}
	if out := physicalRead(t, ctx, "fixtures/expected.output", 1, 1); out.Success || out.ReadCoverage != nil {
		t.Fatalf("empty file has no positive offset: %+v", out)
	}
}

func TestReadFilePhysicalEmptyRuntimePublic(t *testing.T) {
	ctx := physicalReadFixture(t, ".codrax/result.txt", "")
	r := physicalRead(t, ctx, ".codrax/result.txt", 0, 0)
	if !r.Success || r.RuntimeArtifactRead == nil || r.RuntimeArtifactRead.TotalLines != 0 || r.RuntimeArtifactRead.LineStart != 0 || r.RuntimeArtifactRead.LineEnd != 0 || r.ReadCoverage != nil || len(r.Observations) != 0 {
		t.Fatalf("empty runtime/source boundary drift: %+v", r)
	}
	readSet, ranges, totals := types.ExtractReadCoverage([]types.ToolResult{r}, ctx.RepoRoot)
	if len(readSet)+len(ranges)+len(totals) != 0 {
		t.Fatal("empty runtime bytes granted source authority")
	}
}
