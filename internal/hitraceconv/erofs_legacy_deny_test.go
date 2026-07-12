package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

var erofsLegacyNames = []string{
	"erofs_read_enter",
	"erofs_read_exit",
	"erofs_read_iter_enter",
	"erofs_read_iter_exit",
	"erofs_readdir",
	"erofs_lookup_start",
	"erofs_lookup_end",
	"erofs_getattr_enter",
	"erofs_getattr_exit",
	"erofs_listxattr_enter",
	"erofs_listxattr_exit",
	"erofs_raw_access_readpages_start",
	"erofs_raw_access_readpages_end",
	"erofs_read_raw_page_start",
	"erofs_read_raw_page_end",
	"z_erofs_vle_normalaccess_readpage_start",
	"z_erofs_vle_normalaccess_readpage_end",
	"z_erofs_vle_normalaccess_readpages_start",
	"z_erofs_vle_normalaccess_readpages_end",
}

const (
	erofsIndexLure = uint64(0x1111222233334444)
	erofsNIDLure   = uint64(0x2222333344445555)
)

func TestDirectEROFSLegacyNamesRemainBodyUnsupported(t *testing.T) {
	for i, exact := range erofsLegacyNames {
		variants := []struct {
			label string
			name  string
		}{
			{label: "exact", name: exact},
			{label: "case", name: strings.ToUpper(exact)},
			{label: "near", name: erofsNearName(exact)},
			{label: "suffix", name: exact + "_vendor_extension"},
		}
		for _, variant := range variants {
			t.Run(exact+"/"+variant.label, func(t *testing.T) {
				format := erofsLegacyFormat(variant.name, 320+i)
				content := erofsLegacyContent(uint16(format.ID))
				line, admission, reason, envelopeOK := renderEventLineDecision(
					renderContext{cmdlines: map[int]string{4242: "erofs-worker"}},
					1_000_000, 3, format, content,
				)
				if !envelopeOK {
					t.Fatal("valid common ftrace envelope was rejected")
				}
				if admission != bodyUnsupported || reason != "" {
					t.Fatalf("legacy EROFS name acquired semantic body authority: admission=%v reason=%q line=%q", admission, reason, line)
				}
				marker := variant.name + ": "
				at := strings.LastIndex(line, marker)
				if at < 0 || strings.TrimSpace(line[at+len(marker):]) != "" {
					t.Fatalf("unsupported EROFS inventory row fabricated a body: %q", line)
				}
			})
		}
	}
}

func TestDirectEROFSNameGateUsesSourceNeutralAuthority(t *testing.T) {
	for _, name := range []string{
		"erofs_lookup_start",
		" EROFS_LOOKUP_START ",
		"z_erofs_map_blocks_iter_enter",
		"sched_blocked_reason",
		"caller=z_erofs_readpage",
	} {
		if got, want := directEROFSNameCandidate(name), tracequery.EROFSCoverageOnlyNameCandidate(name); got != want {
			t.Fatalf("converter/consumer EROFS gate drifted for %q: direct=%t consumer=%t", name, got, want)
		}
	}
}

func TestConvertFileKeepsAllLegacyEROFSRowsHeaderOnly(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "erofs-legacy.htrace")
	output := filepath.Join(dir, "erofs-legacy.systrace")

	var formatLines []string
	events := make([]syntheticRawEvent, 0, len(erofsLegacyNames))
	for i, name := range erofsLegacyNames {
		id := 320 + i
		formatLines = append(formatLines, erofsLegacyFormatBlock(name, id)...)
		events = append(events, syntheticRawEvent{
			EventID:  uint16(id),
			OffsetNS: uint32(i * 1_000),
			Content:  erofsLegacyContent(uint16(id)),
		})
	}

	var capture bytes.Buffer
	writeFileHeader(&capture, 1)
	writeSegment(&capture, segmentEventsFormat, []byte(strings.Join(formatLines, "\n")))
	writeSegment(&capture, segmentCmdlines, []byte("4242 erofs-worker\n"))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents(events))
	if err := os.WriteFile(input, capture.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ConvertFile(context.Background(), Options{
		InputPath:   input,
		OutputPath:  output,
		TraceEngine: traceEngineBuiltin,
	})
	if err != nil {
		t.Fatalf("convert legacy EROFS capture: %v", err)
	}
	if result.EventsWritten != len(erofsLegacyNames) || result.MissingFormatCount != 0 || result.UnknownEventCount != len(erofsLegacyNames) {
		t.Fatalf("legacy EROFS rows must remain known-format header-only inventory: %+v", result)
	}
	if !strings.Contains(strings.Join(result.Caveats, " "), "header-only") {
		t.Fatalf("header-only EROFS degradation was not disclosed: %+v", result.Caveats)
	}

	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, forbidden := range append(append([]string(nil), erofsLegacyNames...),
		"index:", "nid:", "nr_pages:", "entry_name:", "dev:",
		strconv.FormatUint(erofsIndexLure, 10), strconv.FormatUint(erofsNIDLure, 10)) {
		if strings.Contains(text, forbidden) {
			t.Fatalf("header-only EROFS output leaked semantic body token %q:\n%s", forbidden, text)
		}
	}
	if got := strings.Count(text, "erofs-worker"); got != len(erofsLegacyNames) {
		t.Fatalf("expected %d preserved header rows, got %d:\n%s", len(erofsLegacyNames), got, text)
	}
}

func TestOfficialRendererHasNoLegacyEROFSSemanticAuthority(t *testing.T) {
	body, err := os.ReadFile("official_render.go")
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(body))
	if strings.Contains(lower, "erofs") {
		t.Fatal("official compatibility renderer still contains unpinned EROFS semantic authority")
	}
}

func TestDirectEROFSCoverageGateDominatesLegacyRenderer(t *testing.T) {
	body, err := os.ReadFile("render.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(body)
	gate := strings.Index(source, "if directEROFSNameCandidate(ev.format.Name)")
	legacy := strings.Index(source, "body, known := renderLegacyEventBody(ev, content, cpu)")
	if gate < 0 || legacy < 0 || gate >= legacy {
		t.Fatalf("EROFS coverage-only gate must dominate the generic legacy renderer: gate=%d legacy=%d", gate, legacy)
	}
}

func erofsNearName(name string) string {
	prefix := "erofs_"
	if strings.HasPrefix(name, "z_erofs_") {
		prefix = "z_erofs_"
	}
	tail := strings.TrimPrefix(name, prefix)
	if strings.Contains(tail, "_") {
		tail = strings.Replace(tail, "_", "", 1)
	} else {
		tail += "x"
	}
	return prefix + tail
}

func erofsLegacyFormat(name string, id int) eventFormat {
	return eventFormat{
		ID:   id,
		Name: name,
		Fields: []eventField{
			{Type: "unsigned char", Name: "common_flags", Offset: 2, Size: 1},
			{Type: "unsigned char", Name: "common_preempt_count", Offset: 3, Size: 1},
			{Type: "int", Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Type: "unsigned long", Name: "dev", Offset: 8, Size: 8},
			{Type: "unsigned long", Name: "ino", Offset: 16, Size: 8},
			{Type: "unsigned long", Name: "index", Offset: 24, Size: 8},
			{Type: "unsigned long", Name: "nid", Offset: 32, Size: 8},
			{Type: "unsigned long", Name: "off", Offset: 40, Size: 8},
			{Type: "unsigned long", Name: "size", Offset: 48, Size: 8},
			{Type: "long", Name: "res", Offset: 56, Size: 8, Signed: true},
			{Type: "unsigned int", Name: "nr_pages", Offset: 64, Size: 4},
			{Type: "unsigned int", Name: "mode", Offset: 68, Size: 4},
			{Type: "unsigned long", Name: "start_pos", Offset: 72, Size: 8},
			{Type: "unsigned long", Name: "end_pos", Offset: 80, Size: 8},
			{Type: "unsigned long", Name: "cino", Offset: 88, Size: 8},
			{Type: "unsigned long", Name: "blocks", Offset: 96, Size: 8},
			{Type: "unsigned int", Name: "nlink", Offset: 104, Size: 4},
			{Type: "unsigned int", Name: "xattr_nid", Offset: 108, Size: 4},
			{Type: "char", Name: "name[16]", Offset: 112, Size: 16},
		},
	}
}

func erofsLegacyFormatBlock(name string, id int) []string {
	format := erofsLegacyFormat(name, id)
	fields := make([]string, 0, len(format.Fields))
	for _, field := range format.Fields {
		fields = append(fields, syntheticField(field.Type, field.Name, field.Offset, field.Size, field.Signed))
	}
	return syntheticFormatBlock(name, id, fields)
}

func erofsLegacyContent(eventID uint16) []byte {
	content := make([]byte, 128)
	binary.LittleEndian.PutUint16(content[0:2], eventID)
	binary.LittleEndian.PutUint32(content[4:8], 4242)
	binary.LittleEndian.PutUint64(content[8:16], syntheticDev(8, 1))
	binary.LittleEndian.PutUint64(content[16:24], 0x12345678)
	binary.LittleEndian.PutUint64(content[24:32], erofsIndexLure)
	binary.LittleEndian.PutUint64(content[32:40], erofsNIDLure)
	binary.LittleEndian.PutUint64(content[40:48], 4096)
	binary.LittleEndian.PutUint64(content[48:56], 8192)
	binary.LittleEndian.PutUint64(content[56:64], 7)
	binary.LittleEndian.PutUint32(content[64:68], 3)
	binary.LittleEndian.PutUint32(content[68:72], 0o100644)
	binary.LittleEndian.PutUint64(content[72:80], 11)
	binary.LittleEndian.PutUint64(content[80:88], 22)
	binary.LittleEndian.PutUint64(content[88:96], 0xabcdef)
	binary.LittleEndian.PutUint64(content[96:104], 5)
	binary.LittleEndian.PutUint32(content[104:108], 2)
	binary.LittleEndian.PutUint32(content[108:112], 0x33334444)
	copy(content[112:128], []byte("payload-lure"))
	return content
}
