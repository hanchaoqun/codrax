package traceinput

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/attachment"
	"github.com/hanchaoqun/codrax/internal/pcapinput"
)

func TestHMC307PreparationTeachingReservesWholeUnits(t *testing.T) {
	required := "# codrax-source: /来源/😀.capture\nquery {\"view\":\"native\"}\n"
	optional := []string{"第一条说明完整保留。\n", "Second independent teaching unit.\n", "最后一条。\n"}
	prefixes := map[string]bool{required: true}
	full := required
	for _, unit := range optional {
		full += unit
		prefixes[full] = true
	}
	for budget := 1; budget <= len(full)+1; budget++ {
		got, err := preparationPreviewWithTeaching(required, optional, budget)
		if budget < len(required) {
			if err == nil || got != "" || !strings.Contains(err.Error(), fmt.Sprintf("required_bytes=%d available_bytes=%d", len(required), budget)) {
				t.Fatalf("budget %d published incomplete mandatory facts: %q %v", budget, got, err)
			}
			continue
		}
		if err != nil || !prefixes[got] || !utf8.ValidString(got) || len(got) > budget {
			t.Fatalf("budget %d split a semantic unit/UTF-8: %q %v", budget, got, err)
		}
	}
	// A future long explanation cannot enlarge the mandatory admission floor,
	// and a later short explanation cannot reorder the teaching priority.
	grown := append([]string{strings.Repeat("future guidance ", 10000)}, optional...)
	for _, budget := range []int{len(required), len(required) + len(full)} {
		got, err := preparationPreviewWithTeaching(required, grown, budget)
		if err != nil || got != required {
			t.Fatal("optional teaching changed mandatory admission or skipped priority", err)
		}
	}
}

func hmc307PacketBytes() []byte {
	b := make([]byte, 44)
	binary.LittleEndian.PutUint32(b[0:4], 0xa1b2c3d4)
	binary.LittleEndian.PutUint16(b[4:6], 2)
	binary.LittleEndian.PutUint16(b[6:8], 4)
	binary.LittleEndian.PutUint32(b[16:20], 65535)
	binary.LittleEndian.PutUint32(b[20:24], 147)
	binary.LittleEndian.PutUint32(b[24:28], 7)
	binary.LittleEndian.PutUint32(b[28:32], 1)
	binary.LittleEndian.PutUint32(b[32:36], 4)
	binary.LittleEndian.PutUint32(b[36:40], 12)
	copy(b[40:], []byte{1, 2, 3, 4})
	return b
}

func hmc307PacketPath(t *testing.T, name string, body []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func hmc307CheckPacketPreview(t *testing.T, m *attachment.TraceMaterial, source string, size int64, limit int) {
	t.Helper()
	preview := m.Preview()
	if len(preview) > limit || !utf8.ValidString(preview) || !strings.HasPrefix(preview, "# codrax-source: "+source+"\n") || m.SourcePath() != source || m.QueryPath() != source || m.QueryDomain() != attachment.TraceQueryDomainPacketCapture || m.SelfContainedText() {
		t.Fatalf("source/query/preview authority drift: %q", preview)
	}
	for _, fact := range []string{"Classic PCAP 2.4", fmt.Sprintf("source_bytes=%d", size), "link_type=147", "timestamp_resolution=microsecond", "Query material: original binary.", "global header validated; packet records not queried.", "native epoch seconds; no Trace-clock alignment."} {
		if !strings.Contains(preview, fact) {
			t.Fatalf("mandatory preparation fact omitted: %q\n%s", fact, preview)
		}
	}
	queries := map[string]bool{}
	for _, line := range strings.Split(preview, "\n") {
		if !strings.HasPrefix(line, "trace_query ") {
			continue
		}
		var q map[string]string
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "trace_query ")), &q); err != nil || q["source"] != "attached_trace" || len(q) != 2 {
			t.Fatalf("query navigation is not complete, valid tool JSON: %q %v", line, err)
		}
		queries[q["view"]] = true
	}
	if len(queries) != 2 || !queries["packet_inventory"] || !queries["packet_flows"] {
		t.Fatalf("packet query navigation lost: %v", queries)
	}
	if err := ValidateQueryMaterial(t.Context(), m, preview); err != nil {
		t.Fatal(err)
	}
	metadata, err := m.DisplayMetadata(t.Context(), preview)
	if err != nil || metadata.SourceBytes != size || metadata.QueryBytes != size || metadata.PreviewBytes != int64(len(preview)) || metadata.SourceFormat != "pcap" || metadata.QueryFormat != "pcap" {
		t.Fatalf("metadata confused original/query bytes with envelope: %+v %v", metadata, err)
	}
}

func TestHMC307PacketPreparation640KeepsFactsAndQueryIdentity(t *testing.T) {
	for _, name := range []string{"normal.capture", strings.Repeat("档", 18) + "😀.capture"} {
		t.Run(name, func(t *testing.T) {
			body := hmc307PacketBytes()
			path := hmc307PacketPath(t, name, body)
			m, err := Prepare(t.Context(), Options{InputPath: path, PreviewBytes: 640})
			if err != nil {
				t.Fatal(err)
			}
			hmc307CheckPacketPreview(t, m, path, int64(len(body)), 640)
			current, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(current, body) {
				t.Fatal("preview preparation changed the binary original", err)
			}
			if err := ValidateQueryMaterial(t.Context(), m, m.Preview()+"changed"); err == nil {
				t.Fatal("modified preview borrowed the receipt")
			}
		})
	}
}

func TestHMC307PacketPreparationExactBoundaryAndStaleGeneration(t *testing.T) {
	body := hmc307PacketBytes()
	path := hmc307PacketPath(t, "精确边界.capture", body)
	h, err := pcapinput.ParseHeader(body)
	if err != nil {
		t.Fatal(err)
	}
	full, err := packetPreparationPreview(path, int64(len(body)), h, 4096)
	if err != nil {
		t.Fatal(err)
	}
	minimum := strings.Index(full, "Inventory:")
	if minimum <= 0 || !strings.HasSuffix(full[:minimum], "}\n") {
		t.Fatal("optional teaching boundary does not follow complete query JSON")
	}
	for _, budget := range []int{minimum - 1, minimum, minimum + len("Inventory:") - 1} {
		m, err := Prepare(t.Context(), Options{InputPath: path, PreviewBytes: budget})
		if budget < minimum {
			var preparationErr *Error
			if m != nil || !errors.As(err, &preparationErr) || preparationErr.Code != "packet_capture_preview_budget" || !strings.Contains(err.Error(), fmt.Sprintf("required_bytes=%d available_bytes=%d", minimum, budget)) {
				t.Fatalf("genuinely insufficient budget admitted: %v %v", m, err)
			}
			continue
		}
		if err != nil || m.Preview() != full[:minimum] {
			t.Fatal("exact mandatory envelope was truncated or could not fit", err)
		}
		hmc307CheckPacketPreview(t, m, path, int64(len(body)), budget)
	}
	pending, err := Begin(t.Context(), Options{InputPath: path, PreviewBytes: minimum})
	if err != nil {
		t.Fatal(err)
	}
	defer pending.Discard()
	replacement := hmc307PacketPath(t, "replacement.capture", body)
	if err := os.Rename(replacement, path); err != nil {
		t.Fatal(err)
	}
	if material, err := pending.Commit(t.Context()); material != nil || err == nil {
		t.Fatal("same-byte source replacement renewed the prepared generation")
	}
	current, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(current, body) {
		t.Fatal("failure cleanup removed or modified the replacement source", err)
	}
}

func TestHMC307PacketStreamPreviewBindsEOFAndRollsBackTooSmall(t *testing.T) {
	body := hmc307PacketBytes()
	anchor := t.TempDir()
	pending, err := BeginStream(t.Context(), io.NopCloser(bytes.NewReader(body)), StreamOptions{Options: Options{RuntimeAnchor: anchor, PreviewBytes: 640}, MaxBytes: int64(len(body))})
	if err != nil {
		t.Fatal(err)
	}
	defer pending.Discard()
	root := pending.owned.path
	m, err := pending.Commit(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	hmc307CheckPacketPreview(t, m, m.SourcePath(), int64(len(body)), 640)
	if err := os.WriteFile(filepath.Join(root, "input-stream.json"), []byte("changed EOF receipt"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateQueryMaterial(t.Context(), m, m.Preview()); err == nil {
		t.Fatal("compact preview dropped the stream EOF provenance binding")
	}
	failedAnchor := t.TempDir()
	if material, err := PrepareStream(t.Context(), io.NopCloser(bytes.NewReader(body)), StreamOptions{Options: Options{RuntimeAnchor: failedAnchor, PreviewBytes: 1}, MaxBytes: int64(len(body))}); material != nil || err == nil {
		t.Fatal("too-small stream envelope published incomplete source/navigation")
	}
	entries, err := os.ReadDir(failedAnchor)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed preview leaked uncommitted stream material: %v %v", entries, err)
	}
}

func TestHMC307PacketPreparationNeverClaimsRecordQueryCompletion(t *testing.T) {
	for _, tc := range []struct {
		name, status string
		body         []byte
		count        uint64
	}{
		{"empty", "complete", hmc307PacketBytes()[:24], 0},
		{"record", "complete", hmc307PacketBytes(), 1},
		{"bad_tail", "partial", append(hmc307PacketBytes(), 1), 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := hmc307PacketPath(t, "ordinary.capture", tc.body)
			m, err := Prepare(t.Context(), Options{InputPath: path, PreviewBytes: 640})
			if err != nil {
				t.Fatal(err)
			}
			hmc307CheckPacketPreview(t, m, path, int64(len(tc.body)), 640)
			query, err := pcapinput.Stream(t.Context(), m.QueryPath(), 1)
			if err != nil || query.Status != tc.status || query.PacketCount != tc.count {
				t.Fatalf("query outcome was replaced by header-only preparation: %+v %v", query, err)
			}
		})
	}
}
