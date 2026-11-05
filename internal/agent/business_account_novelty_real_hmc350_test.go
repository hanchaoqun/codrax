package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Operator input pins whole external captures, not case-specific production
// behavior. Both native queries and the actual Finalizer adapter are exercised
// without a model call; original native JSON/raw bytes survive TempDir cleanup.
func TestHMC350WholeCapturesKeepIndependentAccountsWithinOldBudget(t *testing.T) {
	input, out := os.Getenv("CODRAX_HMC350_REAL_INPUTS"), os.Getenv("CODRAX_HMC350_REAL_OUT")
	if input == "" {
		t.Skip("whole external captures not supplied")
	}
	if !filepath.IsAbs(input) || !filepath.IsAbs(out) {
		t.Fatal("absolute operator inputs and fresh output required")
	}
	var captures []struct {
		Name, Path, SHA256 string
		Bytes              int64
		PID                int
		Start, End         float64
	}
	raw, err := os.ReadFile(input)
	if err != nil || json.Unmarshal(raw, &captures) != nil || len(captures) < 2 {
		t.Fatal("multiple complete, pinned captures required", err)
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	for _, capture := range captures {
		t.Run(capture.Name, func(t *testing.T) {
			if !filepath.IsAbs(capture.Path) || capture.PID <= 0 || capture.End <= capture.Start || capture.Bytes <= 0 {
				t.Fatal("invalid whole-source operator input")
			}
			sha, size := hmc333FullSourceHash(t, capture.Path)
			if sha != capture.SHA256 || size != capture.Bytes {
				t.Fatal("whole original changed")
			}
			dir := filepath.Join(out, capture.Name)
			if filepath.Base(capture.Name) != capture.Name || os.Mkdir(dir, 0700) != nil {
				t.Fatal("fresh simple capture output required")
			}
			ctx := &types.AgentContext{RepoRoot: t.TempDir(), WorkDir: t.TempDir(), Language: "zh", Stage: types.StageFinalize, AgentName: types.AgentFinalizer,
				Mutable: types.NewMutableState("Explain the observed business accounts"), AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Language: "zh", PerfTrace: &types.PerfBundle{}}}}
			for _, limit := range []int{40, 41} {
				hmc320BusinessQuery(t, ctx, capture.Path, map[string]any{"pid": capture.PID, "time_start": capture.Start, "time_end": capture.End, "limit": limit})
			}
			ledger := answerDocObservationLedger(ctx)
			before, _ := json.Marshal(ledger)
			causalBefore, _ := json.Marshal(types.CompileTraceCausalProjectionSet(ledger))
			plan := planAnswerDocBusinessTreeFacts(ctx, ledger)
			unique, queries := map[string]bool{}, map[string]bool{}
			for _, a := range plan.accounts {
				unique[a.fact.Node.ID] = true
				queries[a.record.SourceRef.QueryScopeID] = true
			}
			write := func(name string, value any) {
				body, err := json.MarshalIndent(value, "", "  ")
				if err != nil || os.WriteFile(filepath.Join(dir, name), append(body, '\n'), 0600) != nil {
					t.Fatal("save current whole-source audit", err)
				}
			}
			results := ctx.Mutable.TurnAArtifacts().ToolResults
			write("actual-tool-results.json", results)
			write("original-ledger.json", ledger)
			files := map[string]bool{}
			for _, result := range results {
				files[result.RawRef] = true
				for _, r := range result.Observations {
					if r.SourceRef.PayloadRef != "" {
						files[r.SourceRef.PayloadRef] = true
					}
				}
			}
			paths := make([]string, 0, len(files))
			for p := range files {
				if p != "" {
					paths = append(paths, p)
				}
			}
			sort.Strings(paths)
			for i, p := range paths {
				body, err := os.ReadFile(p)
				if err != nil || os.WriteFile(filepath.Join(dir, fmt.Sprintf("source-%02d.original", i+1)), body, 0600) != nil {
					t.Fatal("retain original raw/payload before cleanup", err)
				}
			}
			if len(plan.accounts) != 16 || len(unique) < 15 || len(queries) != 2 || plan.shownGroups != 2 {
				t.Fatalf("unchanged budget lost useful accounts or independent queries: records=%d physical=%d queries=%d", len(plan.accounts), len(unique), len(queries))
			}
			_, messages := finalizerReaderFieldMessages(t, ctx)
			for _, a := range plan.accounts {
				if !strings.Contains(messages, tool.TraceBusinessTreeNodeText(a.fact.Node)) || !strings.Contains(messages, "observation_id="+quotedTreeValue(a.record.ID)) {
					t.Fatal("actual finalizer lost a complete selected source/query account")
				}
			}
			afterLedger := answerDocObservationLedger(ctx)
			after, _ := json.Marshal(afterLedger)
			causalAfter, _ := json.Marshal(types.CompileTraceCausalProjectionSet(afterLedger))
			if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(causalBefore, causalAfter) {
				t.Fatal("display preference changed native facts or causal projection")
			}
			if err := os.WriteFile(filepath.Join(dir, "actual-finalizer-context.txt"), []byte(messages), 0600); err != nil {
				t.Fatal(err)
			}
			afterSHA, afterSize := hmc333FullSourceHash(t, capture.Path)
			if afterSHA != sha || afterSize != size {
				t.Fatal("external original capture changed")
			}
			write("scope-audit.json", map[string]any{"source": capture.Path, "source_sha256": sha, "source_bytes": size, "source_unchanged": true,
				"selected_records": len(plan.accounts), "selected_physical_accounts": len(unique), "independent_queries": len(queries), "accepted_records": plan.accepted,
				"causal_projection_unchanged": true, "model_calls": 0, "scope": "Every selected account remains complete and individually source/query-bound; novelty is soft ordering only, not a shared authority or a larger budget."})
		})
	}
}
