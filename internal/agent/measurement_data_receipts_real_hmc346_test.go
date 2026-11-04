package agent

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The complete user capture is queried twice, not cropped or rewritten.
// This is an explicit model-free seam regression, not a live-eval replacement.
func TestHMC346FullDouyinDataCoveragePublicHandoff(t *testing.T) {
	path, out := os.Getenv("CODRAX_HMC346_REAL_TRACE"), os.Getenv("CODRAX_HMC346_REAL_OUT")
	if path == "" {
		t.Skip("whole user trace not supplied")
	}
	if !filepath.IsAbs(path) || !filepath.IsAbs(out) {
		t.Fatal("absolute source/output required")
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	before, size := hmc333FullSourceHash(t, path)
	if before != "93b45bcca5c9e9b415478d81e176b633d70886328c49731b5258c806695de1ef" || size != 235422121 {
		t.Fatal("whole source changed")
	}
	ctx, results := hmc346QueryContext(t, path, 57874, 236627.820748, 236628.490099)
	tables := hmc346TimelineTables(t, ctx)
	if len(tables[0].Rows) != 2783 {
		t.Fatal("complete original interval set changed")
	}
	projection := hmc218CausalProjection(t, ctx)
	_, messages := finalizerReaderFieldMessages(t, ctx)
	data := hmc302Dictionary(t, messages, "native_measurement_data")
	if len(data) != 1 || !strings.Contains(messages, "data_ref") {
		t.Fatal("whole-source actual adapter did not reuse the native timeline data")
	}
	for _, table := range tables {
		if !strings.Contains(messages, table.ObservationID) {
			t.Fatal("one original selector was lost")
		}
	}
	if !bytes.Equal(projection, hmc218CausalProjection(t, ctx)) {
		t.Fatal("display sharing changed the causal projection")
	}
	for i, r := range results {
		var payload string
		for _, record := range r.Observations {
			if record.SourceRef.PayloadRef != "" {
				payload = record.SourceRef.PayloadRef
				break
			}
		}
		for kind, source := range map[string]string{"native": payload, "raw": r.RawRef} {
			value, err := os.ReadFile(source)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(out, string(rune('A'+i))+"-"+kind+".json"), value, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := os.WriteFile(filepath.Join(out, "actual-finalizer.txt"), []byte(messages), 0600); err != nil {
		t.Fatal(err)
	}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromAgentContext(ctx, 0))
	retained, _ := json.MarshalIndent(map[string]any{"source": path, "source_sha256": before, "source_bytes": size, "model_requests": 0, "native_queries": 2, "timeline_rows_each": len(tables[0].Rows), "shared_data_bodies": len(data), "context_bytes": len(messages), "ledger_records": len(ledger.Records), "whole_source_unchanged": true, "independent_scheduler_or_causal_oracle": false}, "", "  ")
	after, afterSize := hmc333FullSourceHash(t, path)
	if after != before || afterSize != size {
		t.Fatal("whole source mutated")
	}
	if err := os.WriteFile(filepath.Join(out, "receipt.json"), retained, 0600); err != nil {
		t.Fatal(err)
	}
}
