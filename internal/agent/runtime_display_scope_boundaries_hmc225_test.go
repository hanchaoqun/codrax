package agent

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC225DisplayScopeReceiptLifecycleAndOptionalWindow(t *testing.T) {
	ctx, _, _ := hmc225DisplayContext(t)
	ledger := answerDocObservationLedger(ctx)
	profile := ctx.AnalysisIR.RequestModel.RuntimeArtifactScopeProfile
	var first, second types.ObservationRecord
	for _, r := range ledger.Records {
		if r.Predicate == "cpu_state_frequency_observation" {
			if strings.HasSuffix(r.SourceRef.Path, "old.systrace") {
				first = r
			} else {
				second = r
			}
		}
	}
	if first.ID == "" || second.ID == "" {
		t.Fatal("real native records missing")
	}
	for _, r := range []types.ObservationRecord{first, second} {
		if p, ok := ledger.RuntimeDisplayScope(r, profile); !ok || !p.HasExplicitTimeWindows() {
			t.Fatal("healthy source lost scope")
		}
	}
	t.Run("copy_crop", func(t *testing.T) {
		copy := ledger
		copy.Records = []types.ObservationRecord{first}
		if _, ok := copy.RuntimeDisplayScope(first, profile); !ok {
			t.Fatal("read-only ledger crop lost receipt")
		}
	})
	t.Run("json_has_no_authority", func(t *testing.T) {
		bytes, _ := json.Marshal(ledger)
		var restored types.ObservationLedger
		if err := json.Unmarshal(bytes, &restored); err != nil {
			t.Fatal(err)
		}
		if _, ok := restored.RuntimeDisplayScope(first, profile); ok {
			t.Fatal("JSON restored private original authority")
		}
		if len(restored.Records) != len(ledger.Records) {
			t.Fatal("unknown ownership deleted evidence")
		}
	})
	for _, field := range []string{"path", "query", "payload", "window", "line_selector", "clock", "target"} {
		t.Run("forged_"+field, func(t *testing.T) {
			forged := first
			switch field {
			case "path":
				forged.SourceRef.Path = second.SourceRef.Path
			case "query":
				forged.SourceRef.QueryScopeID += "foreign"
			case "payload":
				forged.SourceRef.PayloadRef += "foreign"
			case "window":
				forged.SourceRef.QueryWindowStartTs += .001
			case "line_selector":
				forged.SourceRef.QueryLineStart = 1
			case "clock":
				forged.SourceRef.TimeDomain += "foreign"
			case "target":
				forged.SourceRef.QueryTargetPID++
			}
			if _, ok := ledger.RuntimeDisplayScope(forged, profile); ok {
				t.Fatal("mutated source tuple borrowed a receipt")
			}
		})
	}
	t.Run("independent_unspecified_side", func(t *testing.T) {
		data, _ := json.Marshal(profile)
		var mixed types.RuntimeArtifactScopeProfile
		_ = json.Unmarshal(data, &mixed)
		mixed.Comparison.Current.TimeStart = nil
		mixed.Comparison.Current.TimeEnd = nil
		mixed.TimeWindows = mixed.TimeWindows[:1]
		p, ok := ledger.RuntimeDisplayScope(second, &mixed)
		if !ok || p.HasExplicitTimeWindows() {
			t.Fatal("unspecified current side inherited baseline window")
		}
		p, ok = ledger.RuntimeDisplayScope(first, &mixed)
		if !ok || !p.HasExplicitTimeWindows() {
			t.Fatal("explicit baseline ruler lost")
		}
	})
	t.Run("legacy_unbound_unchanged", func(t *testing.T) {
		legacy := *profile
		legacy.Comparison = nil
		if p, ok := (types.ObservationLedger{}).RuntimeDisplayScope(first, &legacy); !ok || p != &legacy {
			t.Fatal("noncomparison behavior changed")
		}
	})
	t.Run("wide_query_preserves_valid_local_projection", func(t *testing.T) {
		var local types.ObservationRecord
		for _, row := range ledger.Records {
			if strings.HasSuffix(row.SourceRef.Path, "old.systrace") && row.SourceRef.QueryWindowStartTs == 1 {
				local = row
				break
			}
		}
		if local.ID == "" {
			t.Fatal("missing native wide query receipt")
		}
		// Model no new facts: exercise the existing native selected_window
		// carrier while retaining its actual wider query and closing witness.
		local.RichNotes = append(append([]string(nil), local.RichNotes...), "selected_window=1.002..1.007")
		selected, omitted := answerDocSelectedWindowObservationRecords(ctx, []types.ObservationRecord{local})
		if len(selected) != 1 || omitted != 0 || selected[0].SourceRef != local.SourceRef {
			t.Fatal("valid source-local selected window lost the wider query receipt")
		}
	})
	t.Run("source_generation", func(t *testing.T) {
		data, err := os.ReadFile(profile.Comparison.Baseline.Path)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(profile.Comparison.Baseline.Path, append(data, []byte("\n# replaced\n")...), 0600); err != nil {
			t.Fatal(err)
		}
		if _, ok := ledger.RuntimeDisplayScope(first, profile); ok {
			t.Fatal("old source generation retained display scope")
		}
		if _, ok := ledger.RuntimeDisplayScope(second, profile); !ok {
			t.Fatal("healthy sibling invalidated")
		}
	})
	t.Run("consumer_epoch", func(t *testing.T) {
		ctx.Mutable.ResetTurnAArtifacts()
		if _, ok := ledger.RuntimeDisplayScope(second, profile); ok {
			t.Fatal("old ledger revived after consumer reset")
		}
		if len(ledger.Records) == 0 {
			t.Fatal("epoch revoked audit observations")
		}
	})
}
