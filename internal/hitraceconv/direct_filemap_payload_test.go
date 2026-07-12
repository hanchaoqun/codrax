package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type directFilemapTestFixture struct {
	format  eventFormat
	content []byte
}

func TestDirectFilemapCanonicalProfileMatrix(t *testing.T) {
	tests := []struct {
		name    string
		fixture directFilemapTestFixture
		want    string
	}{
		{name: "page add linux 5.10 32 bit", fixture: directPageCacheFixture("mm_filemap_add_to_page_cache", 4, false), want: "dev 12:48 ino 0x1234 pfn=77 ofs=4096"},
		{name: "page delete linux 5.10 32 bit", fixture: directPageCacheFixture("mm_filemap_delete_from_page_cache", 4, false), want: "dev 12:48 ino 0x1234 pfn=77 ofs=4096"},
		{name: "page add linux 5.10 64 bit", fixture: directPageCacheFixture("mm_filemap_add_to_page_cache", 8, false), want: "dev 12:48 ino 0x1234 pfn=77 ofs=4096"},
		{name: "page delete linux 5.10 64 bit", fixture: directPageCacheFixture("mm_filemap_delete_from_page_cache", 8, false), want: "dev 12:48 ino 0x1234 pfn=77 ofs=4096"},
		{name: "page add linux 6.6 32 bit order zero", fixture: directPageCacheFixtureWithOrder("mm_filemap_add_to_page_cache", 4, 0), want: "dev 12:48 ino 0x1234 pfn=77 ofs=4096"},
		{name: "page delete linux 6.6 64 bit order max", fixture: directPageCacheFixtureWithOrder("mm_filemap_delete_from_page_cache", 8, math.MaxUint8), want: "dev 12:48 ino 0x1234 pfn=77 ofs=4096"},
		{name: "writeback set 32 bit", fixture: directFilemapSetFixture(4), want: "dev=12:48 ino=0x1234 errseq=0xabcdef01"},
		{name: "writeback set 64 bit", fixture: directFilemapSetFixture(8), want: "dev=12:48 ino=0x1234 errseq=0xabcdef01"},
		{name: "writeback advance 32 bit", fixture: directFilemapAdvanceFixture(4, 0xfedcba98), want: "file=0xfedcba98 dev=12:48 ino=0x1234 old=0x1020304 new=0xa0b0c0d"},
		{name: "writeback advance 64 bit", fixture: directFilemapAdvanceFixture(8, 0xfedcba9876543210), want: "file=0xfedcba9876543210 dev=12:48 ino=0x1234 old=0x1020304 new=0xa0b0c0d"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			body := directFilemapAdmittedBody(t, test.fixture)
			if body != test.want {
				t.Fatalf("canonical filemap body=%q want=%q", body, test.want)
			}
			if strings.Contains(body, "page=") || strings.Contains(body, "order=") {
				t.Fatalf("canonical page body exposed an unavailable/audit-only dimension: %q", body)
			}
			line, admission, reason, envelopeOK := renderEventLineDecision(
				renderContext{cmdlines: map[int]string{100: "filemap-worker"}, tgids: map[int]int{100: 100}},
				1_000_000, 2, test.fixture.format, test.fixture.content,
			)
			if !envelopeOK || admission != bodyAdmitted || reason != "" ||
				!strings.HasSuffix(line, test.fixture.format.Name+": "+test.want) {
				t.Fatalf("wrapped filemap drifted: envelope=%v admission=%d reason=%q line=%q", envelopeOK, admission, reason, line)
			}
			if len(line) > maxTraceDBSystraceLineBytes || !traceDBSinglePhysicalLine(line, false) {
				t.Fatalf("admitted filemap row escaped publication gate: bytes=%d line=%q", len(line), line)
			}
		})
	}
}

func TestDirectFilemapExactNamesRejectNearAndPrefixCompatibility(t *testing.T) {
	for _, name := range []string{
		"mm_filemap_add_to_page_cache", "mm_filemap_delete_from_page_cache",
		"filemap_set_wb_err", "file_check_and_advance_wb_err",
	} {
		if !directFilemapNameGoverned(name) {
			t.Fatalf("exact filemap name missing from governed registry: %q", name)
		}
	}
	for _, name := range []string{
		"mm_filemap_fault", "mm_filemap_add_to_page_cache_start", "filemap_set_wb_err_done",
		"file_check_and_advance_wb_err_vendor", "Filemap_set_wb_err", "filemap_set_wb_err ",
	} {
		if directFilemapNameGoverned(name) {
			t.Fatalf("near-name entered governed filemap registry: %q", name)
		}
	}

	tests := []struct {
		name    string
		fixture directFilemapTestFixture
	}{
		{name: "mm_filemap_add_to_page_cache_start", fixture: directPageCacheFixture("mm_filemap_add_to_page_cache", 8, false)},
		{name: "mm_filemap_delete_from_page_cache_vendor", fixture: directPageCacheFixture("mm_filemap_delete_from_page_cache", 8, false)},
		{name: "MM_filemap_add_to_page_cache", fixture: directPageCacheFixture("mm_filemap_add_to_page_cache", 8, false)},
		{name: "mm_filemap_add_to_page_cache ", fixture: directPageCacheFixture("mm_filemap_add_to_page_cache", 8, false)},
		{name: "filemap_set_wb_err_start", fixture: directFilemapSetFixture(8)},
		{name: "filemap_set_wb_err_vendor", fixture: directFilemapSetFixture(8)},
		{name: "file_check_and_advance_wb_err_done", fixture: directFilemapAdvanceFixture(8, 1)},
		{name: "file_check_and_advance_wb_err_vendor", fixture: directFilemapAdvanceFixture(8, 1)},
		{name: "mm_filemap_fault", fixture: directPageCacheFixture("mm_filemap_add_to_page_cache", 8, false)},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := cloneDirectFilemapFixture(test.fixture)
			fixture.format.Name = test.name
			body, admission, reason := renderEventBodyDecision(
				coreDecodeContext{}, decodeEvent(fixture.format, fixture.content), fixture.content, 0,
			)
			if admission != bodyUnsupported || reason != "" {
				t.Fatalf("near-name entered filemap authority: admission=%d reason=%q body=%q", admission, reason, body)
			}
			if line, known := renderEventLine(
				renderContext{cmdlines: map[int]string{100: "filemap-worker"}, tgids: map[int]int{100: 100}},
				1_000_000, 2, fixture.format, fixture.content,
			); known {
				t.Fatalf("near-name acquired a public canonical row: line=%q body=%q", line, body)
			}
		})
	}
}

func TestDirectFilemapRejectsCrossAliasesAndGenericIOInjection(t *testing.T) {
	tests := []struct {
		name        string
		field       string
		replacement string
	}{
		{name: "device alias", field: "s_dev", replacement: "dev"},
		{name: "inode alias", field: "i_ino", replacement: "ino"},
		{name: "offset alias", field: "index", replacement: "offset"},
		{name: "ofs alias", field: "index", replacement: "ofs"},
		{name: "position alias", field: "index", replacement: "pos"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := directPageCacheFixture("mm_filemap_add_to_page_cache", 8, false)
			filemapFixtureField(t, &fixture, test.field).Name = test.replacement
			directFilemapAssertRejected(t, fixture)
		})
	}

	for _, name := range []string{"bytes", "entry_name"} {
		t.Run(name+" injection", func(t *testing.T) {
			fixture := directPageCacheFixture("mm_filemap_delete_from_page_cache", 8, false)
			fixture.format.Fields = append(fixture.format.Fields, eventField{
				Type: "unsigned long", Name: name, Offset: len(fixture.content), Size: 8,
			})
			fixture.content = append(fixture.content, make([]byte, 8)...)
			directFilemapAssertRejected(t, fixture)
		})
	}
}

func TestDirectFilemapDescriptorMutationMatrix(t *testing.T) {
	var fixtures []directFilemapTestFixture
	for _, name := range []string{"mm_filemap_add_to_page_cache", "mm_filemap_delete_from_page_cache"} {
		for _, width := range []int{4, 8} {
			fixtures = append(fixtures,
				directPageCacheFixture(name, width, false),
				directPageCacheFixture(name, width, true),
			)
		}
	}
	for _, width := range []int{4, 8} {
		fixtures = append(fixtures, directFilemapSetFixture(width), directFilemapAdvanceFixture(width, 1))
	}

	for _, base := range fixtures {
		base := base
		for fieldIndex := 4; fieldIndex < len(base.format.Fields); fieldIndex++ {
			fieldIndex := fieldIndex
			field := base.format.Fields[fieldIndex]
			prefix := base.format.Name + "/word" + filemapTestItoa(baseWordWidth(base)) + "/" + cleanFieldName(field.Name)

			t.Run(prefix+"/missing", func(t *testing.T) {
				fixture := cloneDirectFilemapFixture(base)
				fixture.format.Fields = append(fixture.format.Fields[:fieldIndex], fixture.format.Fields[fieldIndex+1:]...)
				if cleanFieldName(field.Name) == "order" {
					// The 5.10 descriptor is byte-for-byte the 6.6 descriptor
					// without order. Absence therefore selects the independently
					// pinned 5.10 profile; it is not a malformed 6.6 row.
					directFilemapAdmittedBody(t, fixture)
					return
				}
				directFilemapAssertRejected(t, fixture)
			})
			t.Run(prefix+"/wrong_type", func(t *testing.T) {
				fixture := cloneDirectFilemapFixture(base)
				fixture.format.Fields[fieldIndex].Type = "char *"
				directFilemapAssertRejected(t, fixture)
			})
			t.Run(prefix+"/wrong_sign", func(t *testing.T) {
				fixture := cloneDirectFilemapFixture(base)
				fixture.format.Fields[fieldIndex].Signed = true
				directFilemapAssertRejected(t, fixture)
			})
			t.Run(prefix+"/wrong_width", func(t *testing.T) {
				fixture := cloneDirectFilemapFixture(base)
				if fixture.format.Fields[fieldIndex].Size == 1 {
					fixture.format.Fields[fieldIndex].Size = 2
				} else {
					fixture.format.Fields[fieldIndex].Size = 1
				}
				directFilemapAssertRejected(t, fixture)
			})
			t.Run(prefix+"/wrong_offset", func(t *testing.T) {
				fixture := cloneDirectFilemapFixture(base)
				fixture.format.Fields[fieldIndex].Offset++
				directFilemapAssertRejected(t, fixture)
			})
			t.Run(prefix+"/truncated", func(t *testing.T) {
				fixture := cloneDirectFilemapFixture(base)
				fixture.content = fixture.content[:field.Offset+field.Size-1]
				directFilemapAssertRejected(t, fixture)
			})
			for _, conflict := range []bool{false, true} {
				conflict := conflict
				label := "same_duplicate"
				if conflict {
					label = "conflicting_duplicate"
				}
				t.Run(prefix+"/"+label, func(t *testing.T) {
					fixture := cloneDirectFilemapFixture(base)
					duplicateDirectFilemapField(&fixture, fieldIndex, conflict)
					directFilemapAssertRejected(t, fixture)
				})
			}
			t.Run(prefix+"/alias", func(t *testing.T) {
				fixture := cloneDirectFilemapFixture(base)
				fixture.format.Fields[fieldIndex].Name = cleanFieldName(field.Name) + "_alias"
				directFilemapAssertRejected(t, fixture)
			})
		}

		t.Run(base.format.Name+"/word"+filemapTestItoa(baseWordWidth(base))+"/extra", func(t *testing.T) {
			fixture := cloneDirectFilemapFixture(base)
			fixture.format.Fields = append(fixture.format.Fields, eventField{
				Type: "unsigned int", Name: "vendor_extra", Offset: len(fixture.content), Size: 4,
			})
			fixture.content = append(fixture.content, 0, 0, 0, 0)
			directFilemapAssertRejected(t, fixture)
		})
		t.Run(base.format.Name+"/word"+filemapTestItoa(baseWordWidth(base))+"/overlap", func(t *testing.T) {
			fixture := cloneDirectFilemapFixture(base)
			last := len(fixture.format.Fields) - 1
			fixture.format.Fields[last].Offset = fixture.format.Fields[last-1].Offset
			directFilemapAssertRejected(t, fixture)
		})
	}
}

func TestDirectFilemapCommonEnvelopeIsExactAndClosed(t *testing.T) {
	base := directPageCacheFixture("mm_filemap_add_to_page_cache", 8, false)
	for fieldIndex := 0; fieldIndex < 4; fieldIndex++ {
		fieldIndex := fieldIndex
		field := base.format.Fields[fieldIndex]
		name := cleanFieldName(field.Name)
		t.Run(name+"/missing", func(t *testing.T) {
			fixture := cloneDirectFilemapFixture(base)
			fixture.format.Fields = append(fixture.format.Fields[:fieldIndex], fixture.format.Fields[fieldIndex+1:]...)
			directFilemapAssertRejected(t, fixture)
		})
		t.Run(name+"/wrong_type", func(t *testing.T) {
			fixture := cloneDirectFilemapFixture(base)
			fixture.format.Fields[fieldIndex].Type = "unsigned long"
			directFilemapAssertRejected(t, fixture)
		})
		t.Run(name+"/wrong_sign", func(t *testing.T) {
			fixture := cloneDirectFilemapFixture(base)
			fixture.format.Fields[fieldIndex].Signed = !fixture.format.Fields[fieldIndex].Signed
			directFilemapAssertRejected(t, fixture)
		})
		t.Run(name+"/wrong_width", func(t *testing.T) {
			fixture := cloneDirectFilemapFixture(base)
			fixture.format.Fields[fieldIndex].Size++
			directFilemapAssertRejected(t, fixture)
		})
		t.Run(name+"/wrong_offset", func(t *testing.T) {
			fixture := cloneDirectFilemapFixture(base)
			fixture.format.Fields[fieldIndex].Offset++
			directFilemapAssertRejected(t, fixture)
		})
		t.Run(name+"/duplicate", func(t *testing.T) {
			fixture := cloneDirectFilemapFixture(base)
			duplicateDirectFilemapField(&fixture, fieldIndex, false)
			directFilemapAssertRejected(t, fixture)
		})
	}

	fixture := cloneDirectFilemapFixture(base)
	fixture.format.Fields = append(fixture.format.Fields, eventField{
		Type: "unsigned int", Name: "common_vendor", Offset: len(fixture.content), Size: 4,
	})
	fixture.content = append(fixture.content, 0, 0, 0, 0)
	directFilemapAssertRejected(t, fixture)
}

func TestDirectPageCacheRejectsSyntheticPagePointerAndOrderOverflow(t *testing.T) {
	for _, width := range []int{4, 8} {
		for _, alias := range []string{"page", "pg"} {
			t.Run("word"+filemapTestItoa(width)+"/"+alias, func(t *testing.T) {
				fixture := directPageCacheFixture("mm_filemap_add_to_page_cache", width, false)
				fixture.format.Fields = append(fixture.format.Fields, eventField{
					Type: "struct page *", Name: alias, Offset: len(fixture.content), Size: width,
				})
				fixture.content = append(fixture.content, make([]byte, width)...)
				directFilemapAssertRejected(t, fixture)
			})
		}
	}

	fixture := directPageCacheFixtureWithOrder("mm_filemap_add_to_page_cache", 8, math.MaxUint8)
	if body := directFilemapAdmittedBody(t, fixture); strings.Contains(body, "order=") {
		t.Fatalf("audit-only order leaked into canonical page wire: %q", body)
	}
	overflow := cloneDirectFilemapFixture(fixture)
	order := filemapFixtureField(t, &overflow, "order")
	order.Size = 2
	overflow.content = append(overflow.content, 0)
	binary.LittleEndian.PutUint16(overflow.content[order.Offset:order.Offset+2], 256)
	directFilemapAssertRejected(t, overflow)
}

func TestDirectFilemapNumericBoundaries(t *testing.T) {
	zeroPage := directPageCacheFixtureWithOrder("mm_filemap_add_to_page_cache", 8, 0)
	for _, name := range []string{"pfn", "i_ino", "index", "s_dev"} {
		putDirectFilemapUnsigned(t, &zeroPage, name, 0)
	}
	if body := directFilemapAdmittedBody(t, zeroPage); body != "dev 0:0 ino 0x0 pfn=0 ofs=0" {
		t.Fatalf("page zero boundary=%q", body)
	}
	zeroSet := directFilemapSetFixture(8)
	for _, name := range []string{"i_ino", "s_dev", "errseq"} {
		putDirectFilemapUnsigned(t, &zeroSet, name, 0)
	}
	if body := directFilemapAdmittedBody(t, zeroSet); body != "dev=0:0 ino=0x0 errseq=0x0" {
		t.Fatalf("writeback set zero boundary=%q", body)
	}
	zeroAdvance := directFilemapAdvanceFixture(8, 1)
	for _, name := range []string{"i_ino", "s_dev", "old", "new"} {
		putDirectFilemapUnsigned(t, &zeroAdvance, name, 0)
	}
	if body := directFilemapAdmittedBody(t, zeroAdvance); body != "file=0x1 dev=0:0 ino=0x0 old=0x0 new=0x0" {
		t.Fatalf("writeback advance zero scalar boundary=%q", body)
	}

	page := directPageCacheFixtureWithOrder("mm_filemap_add_to_page_cache", 8, math.MaxUint8)
	putDirectFilemapUnsigned(t, &page, "pfn", math.MaxUint64)
	putDirectFilemapUnsigned(t, &page, "i_ino", math.MaxUint64)
	putDirectFilemapUnsigned(t, &page, "index", uint64(math.MaxInt64)>>12)
	putDirectFilemapUnsigned(t, &page, "s_dev", math.MaxUint32)
	wantOffset := (uint64(math.MaxInt64) >> 12) << 12
	wantPage := "dev 4095:1048575 ino 0xffffffffffffffff pfn=18446744073709551615 ofs=" + filemapTestUint(wantOffset)
	if body := directFilemapAdmittedBody(t, page); body != wantPage {
		t.Fatalf("page upper boundary=%q want=%q", body, wantPage)
	}
	overflowIndex := cloneDirectFilemapFixture(page)
	putDirectFilemapUnsigned(t, &overflowIndex, "index", (uint64(math.MaxInt64)>>12)+1)
	directFilemapAssertRejected(t, overflowIndex)

	page32 := directPageCacheFixture("mm_filemap_delete_from_page_cache", 4, false)
	for _, name := range []string{"pfn", "i_ino", "index", "s_dev"} {
		putDirectFilemapUnsigned(t, &page32, name, math.MaxUint32)
	}
	directFilemapAdmittedBody(t, page32)

	set := directFilemapSetFixture(8)
	putDirectFilemapUnsigned(t, &set, "i_ino", math.MaxUint64)
	putDirectFilemapUnsigned(t, &set, "s_dev", math.MaxUint32)
	putDirectFilemapUnsigned(t, &set, "errseq", math.MaxUint32)
	if body := directFilemapAdmittedBody(t, set); body != "dev=4095:1048575 ino=0xffffffffffffffff errseq=0xffffffff" {
		t.Fatalf("writeback set upper boundary=%q", body)
	}

	advance := directFilemapAdvanceFixture(8, math.MaxUint64)
	putDirectFilemapUnsigned(t, &advance, "i_ino", math.MaxUint64)
	putDirectFilemapUnsigned(t, &advance, "s_dev", math.MaxUint32)
	putDirectFilemapUnsigned(t, &advance, "old", math.MaxUint32)
	putDirectFilemapUnsigned(t, &advance, "new", math.MaxUint32)
	if body := directFilemapAdmittedBody(t, advance); body != "file=0xffffffffffffffff dev=4095:1048575 ino=0xffffffffffffffff old=0xffffffff new=0xffffffff" {
		t.Fatalf("writeback advance upper boundary=%q", body)
	}
	zeroFile := cloneDirectFilemapFixture(advance)
	putDirectFilemapUnsigned(t, &zeroFile, "file", 0)
	directFilemapAssertRejected(t, zeroFile)
}

func TestDirectFilemapMalformedRowIsLocalAndSameIDSiblingsSurvive(t *testing.T) {
	fixture := directPageCacheFixture("mm_filemap_add_to_page_cache", 8, false)
	fixture.format.ID = 680
	good := append([]byte(nil), fixture.content...)
	bad := good[:len(good)-1]
	formatText := directFilemapSyntheticFormat(fixture.format)

	var capture bytes.Buffer
	writeFileHeader(&capture, 1)
	writeSegment(&capture, segmentEventsFormat, []byte(formatText))
	writeSegment(&capture, segmentCmdlines, []byte("100 filemap-worker\n"))
	writeSegment(&capture, segmentTGIDs, []byte("100 100\n"))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents([]syntheticRawEvent{
		{EventID: 680, OffsetNS: 0, Content: good},
		{EventID: 680, OffsetNS: 1_000, Content: bad},
		{EventID: 680, OffsetNS: 2_000, Content: good},
	}))

	dir := t.TempDir()
	input := filepath.Join(dir, "direct-filemap.sys")
	output := filepath.Join(dir, "direct-filemap.ftrace")
	if err := os.WriteFile(input, capture.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatalf("convert direct filemap capture: %v", err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if result.EventsWritten != 2 || strings.Count(text, "mm_filemap_add_to_page_cache: dev 12:48 ino 0x1234 pfn=77 ofs=4096") != 2 {
		t.Fatalf("malformed row escaped or poisoned same-ID siblings: result=%+v\n%s", result, text)
	}
	if strings.Count(text, "mm_filemap_add_to_page_cache:") != 2 {
		t.Fatalf("rejected governed row degraded into a public header/generic body:\n%s", text)
	}
}

func TestDirectFilemapPublicationAuthorityIsSingleAndLineGated(t *testing.T) {
	legacy, err := os.ReadFile("official_render.go")
	if err != nil {
		t.Fatal(err)
	}
	legacyText := string(legacy)
	for _, token := range []string{
		`strings.HasPrefix(name, "mm_filemap_`,
		`strings.HasPrefix(name, "filemap_set_wb_err`,
		`strings.HasPrefix(name, "file_check_and_advance_wb_err`,
	} {
		if strings.Contains(legacyText, token) {
			t.Fatalf("legacy official renderer regained broad filemap authority %q", token)
		}
	}

	bodyDecision := conversionProductionFunction(t, "render.go", "renderEventBodyDecisionWithPair")
	filemapAt := strings.Index(bodyDecision, "decodeDirectFilemapPayload(ev)")
	legacyAt := strings.Index(bodyDecision, "renderLegacyEventBody(ev, content, cpu)")
	if filemapAt < 0 || legacyAt < 0 || filemapAt >= legacyAt {
		t.Fatalf("typed filemap decoder must precede legacy renderer: filemap=%d legacy=%d\n%s", filemapAt, legacyAt, bodyDecision)
	}
	lineDecision := conversionProductionFunction(t, "render.go", "renderEventLineDecisionWithPairAudit")
	gateAt := strings.Index(lineDecision, "directFilemapNameGoverned(format.Name)")
	lineAt := strings.Index(lineDecision, "traceDBSinglePhysicalLine(line, false)")
	if gateAt < 0 || lineAt < 0 || gateAt >= lineAt || maxTraceDBSystraceLineBytes != 1<<20 {
		t.Fatalf("governed filemap rows lost shared single-line/1MiB final gate: gate=%d line=%d cap=%d\n%s",
			gateAt, lineAt, maxTraceDBSystraceLineBytes, lineDecision)
	}
}

func TestStructuredPageCacheOrderAuditAndWritebackRemainUnsupported(t *testing.T) {
	base := protoPayload(
		protoVarint(1, 77),
		protoVarint(2, 0x1234),
		protoVarint(3, 1),
		protoVarint(4, uint64((12<<20)|48)),
	)
	tests := []struct {
		name      string
		extra     []byte
		admitted  bool
		wantAudit bool
	}{
		{name: "field5 absent is proto3 zero", admitted: true},
		{name: "field5 explicit zero", extra: protoVarint(5, 0), admitted: true},
		{name: "field5 maximum", extra: protoVarint(5, math.MaxUint8), admitted: true},
		{name: "field5 overflow", extra: protoVarint(5, math.MaxUint8+1), wantAudit: true},
		{name: "field5 wrong wire", extra: protoBytes(5, []byte{1}), wantAudit: true},
		{name: "field5 same duplicate", extra: append(protoVarint(5, 1), protoVarint(5, 1)...), wantAudit: true},
		{name: "field5 conflicting duplicate", extra: append(protoVarint(5, 1), protoVarint(5, 2)...), wantAudit: true},
	}

	for _, field := range []int{1000, 1001} {
		for _, test := range tests {
			t.Run(filemapTestItoa(field)+"/"+test.name, func(t *testing.T) {
				payload := append(append([]byte(nil), base...), test.extra...)
				name, body, known, degradations := renderProfilerFtraceEventBodyWithAudit(
					profilerFtraceEventRecord{Field: field, Payload: payload},
				)
				if test.admitted {
					if !known || name == "" || body != "dev 12:48 ino 0x1234 pfn=77 ofs=4096" || len(degradations) != 0 {
						t.Fatalf("structured page rejected/drifted: known=%v name=%q body=%q degradations=%v", known, name, body, degradations)
					}
					if strings.Contains(body, "page=") || strings.Contains(body, "order=") {
						t.Fatalf("structured page exposed unavailable/audit-only dimension: %q", body)
					}
					return
				}
				if known || name != "" || body != "" || !test.wantAudit || len(degradations) == 0 {
					t.Fatalf("invalid structured order escaped: known=%v name=%q body=%q degradations=%v", known, name, body, degradations)
				}
			})
		}
		upper := protoPayload(
			protoVarint(1, math.MaxUint64),
			protoVarint(2, math.MaxUint64),
			protoVarint(3, uint64(math.MaxInt64)>>12),
			protoVarint(4, math.MaxUint32),
			protoVarint(5, math.MaxUint8),
		)
		name, body, known, degradations := renderProfilerFtraceEventBodyWithAudit(
			profilerFtraceEventRecord{Field: field, Payload: upper},
		)
		wantOffset := (uint64(math.MaxInt64) >> 12) << 12
		want := "dev 4095:1048575 ino 0xffffffffffffffff pfn=18446744073709551615 ofs=" + filemapTestUint(wantOffset)
		if !known || name == "" || body != want || len(degradations) != 0 {
			t.Fatalf("structured page upper boundary drifted: known=%v name=%q body=%q want=%q degradations=%v", known, name, body, want, degradations)
		}
	}

	for _, field := range []int{4013, 4014} {
		if descriptor, ok := profilerFtraceEventDescriptors[field]; ok {
			t.Fatalf("structured writeback field %d unexpectedly gained D-batch authority: %+v", field, descriptor)
		}
		name, body, known, degradations := renderProfilerFtraceEventBodyWithAudit(
			profilerFtraceEventRecord{Field: field, Payload: base},
		)
		if known || name != "" || body != "" || len(degradations) != 0 {
			t.Fatalf("unsupported structured writeback field %d published: known=%v name=%q body=%q degradations=%v", field, known, name, body, degradations)
		}
	}
}

func directPageCacheFixture(name string, width int, withOrder bool) directFilemapTestFixture {
	order := uint8(0)
	if withOrder {
		return directPageCacheFixtureWithOrder(name, width, order)
	}
	return directPageCacheFixtureValues(name, width, nil)
}

func directPageCacheFixtureWithOrder(name string, width int, order uint8) directFilemapTestFixture {
	return directPageCacheFixtureValues(name, width, &order)
}

func directPageCacheFixtureValues(name string, width int, order *uint8) directFilemapTestFixture {
	fields := directFilemapCommonFields()
	offset := 8
	for _, fieldName := range []string{"pfn", "i_ino", "index"} {
		fields = append(fields, eventField{Type: "unsigned long", Name: fieldName, Offset: offset, Size: width})
		offset += width
	}
	fields = append(fields, eventField{Type: "dev_t", Name: "s_dev", Offset: offset, Size: 4})
	offset += 4
	if order != nil {
		fields = append(fields, eventField{Type: "unsigned char", Name: "order", Offset: offset, Size: 1})
		offset++
	}
	fixture := directFilemapTestFixture{
		format:  eventFormat{ID: 680, Name: name, Fields: fields},
		content: directFilemapEnvelope(680, offset),
	}
	putDirectFilemapUnsigned(nil, &fixture, "pfn", 77)
	putDirectFilemapUnsigned(nil, &fixture, "i_ino", 0x1234)
	putDirectFilemapUnsigned(nil, &fixture, "index", 1)
	putDirectFilemapUnsigned(nil, &fixture, "s_dev", (12<<20)|48)
	if order != nil {
		putDirectFilemapUnsigned(nil, &fixture, "order", uint64(*order))
	}
	return fixture
}

func directFilemapSetFixture(width int) directFilemapTestFixture {
	offset := 8
	fields := directFilemapCommonFields()
	fields = append(fields, eventField{Type: "unsigned long", Name: "i_ino", Offset: offset, Size: width})
	offset += width
	fields = append(fields,
		eventField{Type: "dev_t", Name: "s_dev", Offset: offset, Size: 4},
		eventField{Type: "errseq_t", Name: "errseq", Offset: offset + 4, Size: 4},
	)
	fixture := directFilemapTestFixture{
		format:  eventFormat{ID: 681, Name: "filemap_set_wb_err", Fields: fields},
		content: directFilemapEnvelope(681, offset+8),
	}
	putDirectFilemapUnsigned(nil, &fixture, "i_ino", 0x1234)
	putDirectFilemapUnsigned(nil, &fixture, "s_dev", (12<<20)|48)
	putDirectFilemapUnsigned(nil, &fixture, "errseq", 0xabcdef01)
	return fixture
}

func directFilemapAdvanceFixture(width int, file uint64) directFilemapTestFixture {
	offset := 8
	fields := directFilemapCommonFields()
	fields = append(fields, eventField{Type: "struct file *", Name: "file", Offset: offset, Size: width})
	offset += width
	fields = append(fields, eventField{Type: "unsigned long", Name: "i_ino", Offset: offset, Size: width})
	offset += width
	fields = append(fields,
		eventField{Type: "dev_t", Name: "s_dev", Offset: offset, Size: 4},
		eventField{Type: "errseq_t", Name: "old", Offset: offset + 4, Size: 4},
		eventField{Type: "errseq_t", Name: "new", Offset: offset + 8, Size: 4},
	)
	fixture := directFilemapTestFixture{
		format:  eventFormat{ID: 682, Name: "file_check_and_advance_wb_err", Fields: fields},
		content: directFilemapEnvelope(682, offset+12),
	}
	putDirectFilemapUnsigned(nil, &fixture, "file", file)
	putDirectFilemapUnsigned(nil, &fixture, "i_ino", 0x1234)
	putDirectFilemapUnsigned(nil, &fixture, "s_dev", (12<<20)|48)
	putDirectFilemapUnsigned(nil, &fixture, "old", 0x01020304)
	putDirectFilemapUnsigned(nil, &fixture, "new", 0x0a0b0c0d)
	return fixture
}

func directFilemapCommonFields() []eventField {
	return []eventField{
		{Type: "unsigned short", Name: "common_type", Offset: 0, Size: 2},
		{Type: "unsigned char", Name: "common_flags", Offset: 2, Size: 1},
		{Type: "unsigned char", Name: "common_preempt_count", Offset: 3, Size: 1},
		{Type: "int", Name: "common_pid", Offset: 4, Size: 4, Signed: true},
	}
}

func directFilemapEnvelope(eventID int, size int) []byte {
	content := make([]byte, size)
	binary.LittleEndian.PutUint16(content[0:2], uint16(eventID))
	binary.LittleEndian.PutUint32(content[4:8], 100)
	return content
}

func directFilemapAdmittedBody(t *testing.T, fixture directFilemapTestFixture) string {
	t.Helper()
	body, admission, reason := renderEventBodyDecision(
		coreDecodeContext{}, decodeEvent(fixture.format, fixture.content), fixture.content, 0,
	)
	if admission != bodyAdmitted || reason != "" || body == "" {
		t.Fatalf("filemap fixture rejected: event=%s admission=%d reason=%q body=%q", fixture.format.Name, admission, reason, body)
	}
	return body
}

func directFilemapAssertRejected(t *testing.T, fixture directFilemapTestFixture) {
	t.Helper()
	body, admission, reason := renderEventBodyDecision(
		coreDecodeContext{}, decodeEvent(fixture.format, fixture.content), fixture.content, 0,
	)
	if admission != bodyRejected || reason == "" || body != "" {
		t.Fatalf("malformed filemap fixture escaped: event=%s admission=%d reason=%q body=%q fields=%+v", fixture.format.Name, admission, reason, body, fixture.format.Fields)
	}
}

func cloneDirectFilemapFixture(fixture directFilemapTestFixture) directFilemapTestFixture {
	clone := fixture
	clone.format.Fields = append([]eventField(nil), fixture.format.Fields...)
	clone.content = append([]byte(nil), fixture.content...)
	return clone
}

func filemapFixtureField(t *testing.T, fixture *directFilemapTestFixture, name string) *eventField {
	if t != nil {
		t.Helper()
	}
	for index := range fixture.format.Fields {
		if cleanFieldName(fixture.format.Fields[index].Name) == name {
			return &fixture.format.Fields[index]
		}
	}
	if t != nil {
		t.Fatalf("field %q not found in %s", name, fixture.format.Name)
	}
	panic("filemap fixture field not found: " + name)
}

func putDirectFilemapUnsigned(t *testing.T, fixture *directFilemapTestFixture, name string, value uint64) {
	if t != nil {
		t.Helper()
	}
	field := filemapFixtureField(t, fixture, name)
	if field.Offset < 0 || field.Size <= 0 || field.Offset+field.Size > len(fixture.content) {
		if t != nil {
			t.Fatalf("field %q outside fixture content", name)
		}
		panic("filemap fixture field outside content: " + name)
	}
	switch field.Size {
	case 1:
		fixture.content[field.Offset] = byte(value)
	case 2:
		binary.LittleEndian.PutUint16(fixture.content[field.Offset:field.Offset+2], uint16(value))
	case 4:
		binary.LittleEndian.PutUint32(fixture.content[field.Offset:field.Offset+4], uint32(value))
	case 8:
		binary.LittleEndian.PutUint64(fixture.content[field.Offset:field.Offset+8], value)
	default:
		if t != nil {
			t.Fatalf("unsupported fixture width %d for %q", field.Size, name)
		}
		panic("unsupported filemap fixture width")
	}
}

func duplicateDirectFilemapField(fixture *directFilemapTestFixture, fieldIndex int, conflict bool) {
	duplicate := fixture.format.Fields[fieldIndex]
	if conflict {
		duplicate.Offset = len(fixture.content)
		fixture.content = append(fixture.content, make([]byte, duplicate.Size)...)
		fixture.content[duplicate.Offset] = fixture.content[fixture.format.Fields[fieldIndex].Offset] ^ 0xff
	}
	fixture.format.Fields = append(fixture.format.Fields, duplicate)
}

func baseWordWidth(fixture directFilemapTestFixture) int {
	for _, field := range fixture.format.Fields {
		if cleanFieldName(field.Name) == "i_ino" {
			return field.Size
		}
	}
	return 0
}

func directFilemapSyntheticFormat(format eventFormat) string {
	fields := make([]string, 0, len(format.Fields))
	for _, field := range format.Fields {
		fields = append(fields, syntheticField(field.Type, field.Name, field.Offset, field.Size, field.Signed))
	}
	return strings.Join(syntheticFormatBlock(format.Name, format.ID, fields), "\n")
}

func filemapTestItoa(value int) string {
	if value == 0 {
		return "0"
	}
	var buf [20]byte
	index := len(buf)
	for value > 0 {
		index--
		buf[index] = byte('0' + value%10)
		value /= 10
	}
	return string(buf[index:])
}

func filemapTestUint(value uint64) string {
	if value == 0 {
		return "0"
	}
	var buf [20]byte
	index := len(buf)
	for value > 0 {
		index--
		buf[index] = byte('0' + value%10)
		value /= 10
	}
	return string(buf[index:])
}
