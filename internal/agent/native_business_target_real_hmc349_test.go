package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC349FullNetEaseTargetDefaultsRetainAllNativeFacts(t *testing.T) {
	path, out := os.Getenv("CODRAX_HMC349_REAL_TRACE"), os.Getenv("CODRAX_HMC349_REAL_OUT")
	if path == "" {
		t.Skip("whole user capture not supplied")
	}
	if !filepath.IsAbs(path) || !filepath.IsAbs(out) {
		t.Fatal("absolute source and fresh audit output required")
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	sha, size := hmc333FullSourceHash(t, path)
	if size != 82838676 || sha != "12349db5bbdc5a4343360d9633c7d6c36f18c8be5f39fb64e78b53c9e345bfe4" {
		t.Fatal("not the pinned complete original")
	}
	ctx, results := hmc346QueryContext(t, path, 36434, 5969.994941, 5970.622586)
	projection := hmc218CausalProjection(t, ctx)
	contract := types.BuildAnswerSemanticViewForAgentContext(ctx).RuntimeMeasurementContract
	defaults := map[string]bool{}
	for _, r := range types.RuntimeNativeFactDisplaySelections(&ctx.AnalysisIR.RequestModel, contract) {
		defaults[r.ObservationID] = true
	}
	_, messages := finalizerReaderFieldMessages(t, ctx)
	all, automatic := 0, 0
	for _, r := range results[0].Observations {
		if r.Predicate != types.TraceBusinessSpanPredicate {
			continue
		}
		all++
		var table *types.RuntimeMeasurementTable
		for _, candidate := range contract.Choices() {
			if candidate.ObservationID == r.ID {
				copy := candidate
				table = &copy
				break
			}
		}
		if table == nil || !table.HasNativeMeasurementFacts() || !bytes.Contains([]byte(messages), []byte(r.ID)) {
			t.Fatal("complete original lost a measured business table or its current navigation")
		}
		if defaults[r.ID] {
			automatic++
			if r.Subject != "ease.cloudmusic-36434" {
				t.Fatal("another scheduler TID became default target business")
			}
		}
		if table.MemberSet != nil || table.DisplayPopulation != nil {
			t.Fatal("layout intent acquired completeness authority")
		}
	}
	if all != 8 || automatic != 1 || len(hmc347MemberTables(ctx)) != 11 || !bytes.Equal(projection, hmc218CausalProjection(t, ctx)) {
		t.Fatalf("retained=%d default=%d or causal/member facts changed", all, automatic)
	}
	write := func(name string, value any) {
		raw, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(out, name), append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("actual-tool-results.json", results)
	write("actual-native-tables.json", contract.Choices())
	for i, result := range results {
		for _, r := range result.Observations {
			if r.SourceRef.PayloadRef != "" {
				body, err := os.ReadFile(r.SourceRef.PayloadRef)
				if err != nil {
					t.Fatal(err)
				}
				name := []string{"window-stats-native.json", "thread-timeline-native.json"}[i]
				if err := os.WriteFile(filepath.Join(out, name), body, 0600); err != nil {
					t.Fatal(err)
				}
				break
			}
		}
	}
	write("scope-audit.json", map[string]any{"source": path, "source_bytes": size, "source_sha256": sha, "retained_business_tables": all, "automatic_target_business_tables": automatic, "member_tables_retained": 11, "causal_projection_unchanged": true, "model_calls": 0, "scope": "Complete original retained; only numeric query-target display intent changed. Other business facts and explicit selectors remain available."})
	if err := os.WriteFile(filepath.Join(out, "actual-finalizer-context.txt"), []byte(messages), 0600); err != nil {
		t.Fatal(err)
	}
	after, afterSize := hmc333FullSourceHash(t, path)
	if after != sha || afterSize != size {
		t.Fatal("original capture changed")
	}
}
