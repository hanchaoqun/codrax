package agent

import (
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Whole source, exact public query/adapter, and an independently streamed
// B/E LIFO integer-microsecond oracle. Neither this oracle nor expected values
// is supplied to either live model evaluation.
func TestHMC347FullDouyinMemberIntervalsPublicHandoff(t *testing.T) {
	path, out, oraclePath := os.Getenv("CODRAX_HMC347_REAL_TRACE"), os.Getenv("CODRAX_HMC347_REAL_OUT"), os.Getenv("CODRAX_HMC347_MARKER_ORACLE")
	if path == "" {
		t.Skip("whole user trace not supplied")
	}
	if !filepath.IsAbs(path) || !filepath.IsAbs(out) || !filepath.IsAbs(oraclePath) {
		t.Fatal("absolute source/output/oracle required")
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	before, size := hmc333FullSourceHash(t, path)
	if before != "93b45bcca5c9e9b415478d81e176b633d70886328c49731b5258c806695de1ef" || size != 235422121 {
		t.Fatal("whole source changed")
	}
	var oracle struct {
		SourceSHA string `json:"source_sha256"`
		Spans     []struct {
			Name      string `json:"name"`
			StartLine int    `json:"start_line"`
			EndLine   int    `json:"end_line"`
			StartUS   int64  `json:"start_us"`
			EndUS     int64  `json:"end_us"`
			WallUS    int64  `json:"inclusive_wall_us"`
		} `json:"spans"`
	}
	rawOracle, err := os.ReadFile(oraclePath)
	if err != nil || json.Unmarshal(rawOracle, &oracle) != nil || oracle.SourceSHA != before || len(oracle.Spans) != 2611 {
		t.Fatal("independent whole-source marker oracle unavailable")
	}
	ctx, results := hmc346QueryContext(t, path, 57874, 236627.820748, 236628.490099)
	projection := hmc218CausalProjection(t, ctx)
	tables := hmc347MemberTables(ctx)
	_, messages := finalizerReaderFieldMessages(t, ctx)
	checked, verifyMembers, verifyOverlapUS := 0, 0, int64(0)
	for _, table := range tables {
		if !table.HasNativeMeasurementFacts() || table.MemberSet != nil || table.DisplayPopulation != nil || !strings.Contains(messages, table.ObservationID) {
			t.Fatal("actual finalizer lost source-bound member facts or gained population authority")
		}
		for _, row := range table.Rows {
			if row[0] != ".ugc.aweme.lite (57874)" && !strings.Contains(row[0], "57874") {
				continue // Other-thread native records are not this target oracle.
			}
			matched := false
			for _, expected := range oracle.Spans {
				if strconv.Itoa(expected.StartLine) != row[8] || strconv.Itoa(expected.EndLine) != row[9] {
					continue
				}
				s, e, ms := row[2], row[3], row[4]
				us := func(v string) int64 {
					f, err := strconv.ParseFloat(v, 64)
					if err != nil {
						t.Fatal(err)
					}
					return int64(math.Round(f * 1e6))
				}
				f, err := strconv.ParseFloat(ms, 64)
				if err != nil || expected.Name != row[1] || us(s) != expected.StartUS || us(e) != expected.EndUS || int64(math.Round(f*1000)) != expected.WallUS ||
					row[2] != row[5] || row[3] != row[6] {
					t.Fatalf("native member differs from independent original B/E pair: %v / %+v", row, expected)
				}
				if strings.HasPrefix(expected.Name, "VerifyClass ") {
					verifyMembers++
					// Previous real failure used the family envelope to infer
					// overlap with this separately observed dex interval.
					if d := min(expected.EndUS, int64(236627959747)) - max(expected.StartUS, int64(236627871880)); d > 0 {
						verifyOverlapUS += d
					}
				}
				matched, checked = true, checked+1
				break
			}
			if !matched {
				t.Fatalf("native target member missing from independent original source: %v", row)
			}
		}
	}
	if len(tables) == 0 || checked < 11 || verifyMembers != 11 || verifyOverlapUS != 0 || !bytes.Equal(projection, hmc218CausalProjection(t, ctx)) {
		t.Fatalf("full native interval handoff failed: groups=%d checked=%d verification=%d overlap_us=%d", len(tables), checked, verifyMembers, verifyOverlapUS)
	}
	for i, result := range results {
		payload := ""
		for _, r := range result.Observations {
			if r.SourceRef.PayloadRef != "" {
				payload = r.SourceRef.PayloadRef
				break
			}
		}
		for name, file := range map[string]string{"native": payload, "raw": result.RawRef} {
			b, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(out, string(rune('A'+i))+"-"+name+".json"), b, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}
	for name, value := range map[string]any{"tables.json": tables, "receipt.json": map[string]any{"source": path, "source_sha256": before, "source_bytes": size, "model_requests": 0, "native_queries": 2, "native_member_groups": len(tables), "independently_checked_target_members": checked, "class_work_members": verifyMembers, "member_dex_overlap_us": verifyOverlapUS, "context_bytes": len(messages), "causal_projection_unchanged": true, "capture_completeness_proven": false}} {
		b, _ := json.MarshalIndent(value, "", "  ")
		if err := os.WriteFile(filepath.Join(out, name), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(out, "actual-finalizer.txt"), []byte(messages), 0600); err != nil {
		t.Fatal(err)
	}
	after, afterSize := hmc333FullSourceHash(t, path)
	if after != before || afterSize != size {
		t.Fatal("whole source mutated")
	}
}
