package agent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// This scans the complete unchanged user capture. Retained native member rows
// are checked against the original B/E lines, not another production table.
// It is opt-in, model-free, and never supplies an oracle to a live answer.
func TestHMC348FullNetEaseMemberEndpointHandoff(t *testing.T) {
	path, out := os.Getenv("CODRAX_HMC348_REAL_TRACE"), os.Getenv("CODRAX_HMC348_REAL_OUT")
	if path == "" {
		t.Skip("whole user capture not supplied")
	}
	if !filepath.IsAbs(path) || !filepath.IsAbs(out) {
		t.Fatal("absolute source and fresh output required")
	}
	if err := os.Mkdir(out, 0700); err != nil {
		t.Fatal(err)
	}
	before, size := hmc333FullSourceHash(t, path)
	if size != 82838676 || before != "12349db5bbdc5a4343360d9633c7d6c36f18c8be5f39fb64e78b53c9e345bfe4" {
		t.Fatal("not the whole pinned NetEase capture")
	}
	ctx, results := hmc346QueryContext(t, path, 36434, 5969.994941, 5970.622586)
	projection := hmc218CausalProjection(t, ctx)
	tables := hmc347MemberTables(ctx)
	_, messages := finalizerReaderFieldMessages(t, ctx)
	wanted := map[int]bool{}
	for _, table := range tables {
		basis, owned := table.NativeSourceTitleBasis()
		if !owned || basis.QueryWindow != "[5969.994941,5970.622586) s" || !table.HasNativeMeasurementFacts() || table.HasNativeRawFacts() || table.MemberSet != nil || table.DisplayPopulation != nil || !strings.Contains(messages, table.ObservationID) {
			t.Fatal("full-source endpoint or authority boundary lost")
		}
		for _, row := range table.Rows {
			if len(row) != 12 {
				t.Fatal("native row shape changed without review")
			}
			for _, col := range []int{8, 9} {
				line, err := strconv.Atoi(row[col])
				if err != nil || line <= 0 {
					t.Fatal("invalid source line")
				}
				wanted[line] = true
			}
		}
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64<<10), 8<<20)
	lines := map[int]string{}
	count := 0
	for scanner.Scan() {
		count++
		if wanted[count] {
			lines[count] = scanner.Text()
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 559592 || len(lines) != len(wanted) {
		t.Fatal("complete source scan or member endpoints missing")
	}
	marker := regexp.MustCompile(`^\s*(.*?)-(\d+)\s+\(\s*(\d+)\)\s+\[\d+\]\s+\S+\s+(\d+)\.(\d{1,6}):\s+tracing_mark_write:\s+([BE])\|(\d+)(?:\|(.*))?\s*$`)
	parse := func(line int) []string {
		m := marker.FindStringSubmatch(lines[line])
		if m == nil {
			t.Fatalf("not an original B/E marker at %d", line)
		}
		return m
	}
	us := func(s string) int64 {
		v, err := strconv.ParseFloat(s, 64)
		if err != nil {
			t.Fatal(err)
		}
		return int64(math.Round(v * 1e6))
	}
	stamp := func(m []string) int64 {
		sec, _ := strconv.ParseInt(m[4], 10, 64)
		frac, _ := strconv.ParseInt(m[5]+strings.Repeat("0", 6-len(m[5])), 10, 64)
		return sec*1e6 + frac
	}
	checked := 0
	for _, table := range tables {
		for _, row := range table.Rows {
			sl, _ := strconv.Atoi(row[8])
			el, _ := strconv.Atoi(row[9])
			s, e := parse(sl), parse(el)
			if s[6] != "B" || e[6] != "E" || s[2] != e[2] || s[7] != e[7] || s[8] != row[1] || !strings.HasSuffix(row[0], "-"+s[2]) || s[3] != row[10] || s[7] != row[11] || stamp(s) != us(row[5]) || stamp(e) != us(row[6]) || us(row[2]) < stamp(s) || us(row[3]) > stamp(e) {
				t.Fatalf("original member identity/coordinates changed: %v", row)
			}
			ms, err := strconv.ParseFloat(row[7], 64)
			if err != nil || int64(math.Round(ms*1000)) != stamp(e)-stamp(s) {
				t.Fatal("complete-pair ruler differs from original markers")
			}
			checked++
		}
	}
	if len(tables) == 0 || checked == 0 || !bytes.Equal(projection, hmc218CausalProjection(t, ctx)) {
		t.Fatal("no real members or causal projection changed")
	}
	after, afterSize := hmc333FullSourceHash(t, path)
	if before != after || size != afterSize {
		t.Fatal("original source changed")
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
	write("actual-member-tables.json", tables)
	write("actual-tool-results.json", results)
	write("independent-source-line-check.json", map[string]any{"source": path, "source_sha256": before, "source_bytes": size, "source_lines": count, "retained_groups": len(tables), "member_checks": checked, "exact_original_lines": lines, "causal_projection_unchanged": true, "model_calls": 0, "scope": "Original retained member coordinates only; no claim of all semantic members, full capture, scheduler semantics, or causal roots."})
	if err := os.WriteFile(filepath.Join(out, "actual-finalizer-context.txt"), []byte(messages), 0600); err != nil {
		t.Fatal(err)
	}
}
