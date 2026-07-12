package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrintkFormatCatalogAuthority(t *testing.T) {
	const clean = "0xc0aff2da : \"Rescheduling interrupts\"\n"
	catalog := parsePrintkFormats([]byte(clean + clean +
		"0xc0aff2bf : \"Timer broadcast interrupts\"\n" +
		"0xc0aff2da : \"Conflicting reason\"\n" + clean +
		"0xc0aff333 : missing-quotes\n" +
		"not-an-address : \"ignored\"\n"))
	if _, ok := catalog.Formats[0xc0aff2da]; ok || !catalog.Poisoned[0xc0aff2da] {
		t.Fatalf("conflicting address was rescued: %+v", catalog)
	}
	if _, ok := catalog.Formats[0xc0aff333]; ok || !catalog.Poisoned[0xc0aff333] {
		t.Fatalf("addressed malformed row was not quarantined: %+v", catalog)
	}
	if got := catalog.Formats[0xc0aff2bf]; got != "Timer broadcast interrupts" {
		t.Fatalf("unrelated mapping lost: %+v", catalog)
	}
	if catalog.Malformed != 1 {
		t.Fatalf("unattributed malformed count=%d want=1", catalog.Malformed)
	}

	destination := parsePrintkFormats([]byte(clean + "0xc0aff2bf : \"Timer broadcast interrupts\"\n"))
	conflict := parsePrintkFormats([]byte("0xc0aff2da : \"Other\"\n"))
	cleanAgain := parsePrintkFormats([]byte(clean))
	mergePrintkFormatCatalog(&destination, conflict)
	mergePrintkFormatCatalog(&destination, cleanAgain)
	if _, ok := destination.Formats[0xc0aff2da]; ok || !destination.Poisoned[0xc0aff2da] {
		t.Fatalf("cross-segment conflict was rescued: %+v", destination)
	}
	if destination.Formats[0xc0aff2bf] != "Timer broadcast interrupts" {
		t.Fatalf("cross-segment conflict damaged sibling: %+v", destination)
	}

	escaped := parsePrintkFormats([]byte(`0xc0aff999 : "line\nwith\"escapes"` + "\n"))
	if escaped.Formats[0xc0aff999] != `line\nwith\"escapes` {
		t.Fatalf("kernel escape spelling was guessed or damaged: %+v", escaped)
	}
	if validCoreIPIReason(escaped.Formats[0xc0aff999]) {
		t.Fatal("ambiguous printk escape spelling became an IPI reason")
	}

	uppercase := parsePrintkFormats([]byte(clean + "0xC0AFF2DA : \"malformed case\"\n" + clean))
	if _, ok := uppercase.Formats[0xc0aff2da]; ok || !uppercase.Poisoned[0xc0aff2da] {
		t.Fatalf("noncanonical but attributable address escaped quarantine: %+v", uppercase)
	}

	overwideZero := parsePrintkFormats([]byte(clean + "0x00000000000000000c0aff2da : \"malformed width\"\n" + clean))
	if _, ok := overwideZero.Formats[0xc0aff2da]; ok || !overwideZero.Poisoned[0xc0aff2da] {
		t.Fatalf("overwide zero-padded address escaped quarantine: %+v", overwideZero)
	}

	internalQuote := parsePrintkFormats([]byte(clean + "0xc0aff2da : \"bad\"quote\"\n" + clean))
	if _, ok := internalQuote.Formats[0xc0aff2da]; ok || !internalQuote.Poisoned[0xc0aff2da] {
		t.Fatalf("unescaped internal quote escaped quarantine: %+v", internalQuote)
	}

	overlong := clean + "0xc0aff2da : \"" + strings.Repeat("x", maxPrintkFormatLineBytes+1) + "\"\n" + clean
	longCatalog := parsePrintkFormats([]byte(overlong))
	if _, ok := longCatalog.Formats[0xc0aff2da]; ok || !longCatalog.Poisoned[0xc0aff2da] {
		t.Fatalf("overlong addressed row was rescued by a later clean mapping: %+v", longCatalog)
	}
}

func TestBuiltinSysKernelIPIUsesPrintkAndDataLocAuthorities(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "kernel-ipi.sys")
	output := filepath.Join(dir, "kernel-ipi.systrace")
	format := strings.Join(syntheticFormatBlock("ipi_raise", 70, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("__data_loc unsigned long[]", "target_cpus", 8, 4, false),
		syntheticField("const char *", "reason", 16, 8, false),
	}), "\n")
	content := make([]byte, 32)
	binary.LittleEndian.PutUint16(content[:2], 70)
	binary.LittleEndian.PutUint32(content[4:8], 100)
	binary.LittleEndian.PutUint32(content[8:12], uint32((8<<16)|24))
	binary.LittleEndian.PutUint64(content[16:24], 0xc0aff2da)
	binary.LittleEndian.PutUint64(content[24:32], 0x10)

	var capture bytes.Buffer
	writeFileHeader(&capture, 1)
	// CPUNum=31 makes legacy raw-shard IDs overlap metadata IDs 30..32.
	// segmentPrintk must remain metadata authority, never an RMQ page.
	binary.LittleEndian.PutUint32(capture.Bytes()[8:12], 31<<1)
	writeSegment(&capture, segmentEventsFormat, []byte(format))
	writeSegment(&capture, segmentPrintk, []byte("0xc0aff2da : \"Rescheduling interrupts\"\n"))
	writeSegment(&capture, segmentCmdlines, []byte("100 irq-worker\n"))
	writeSegment(&capture, segmentTGIDs, []byte("100 100\n"))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents([]syntheticRawEvent{{EventID: 70, Content: content}}))
	if err := os.WriteFile(input, capture.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if result.EventsWritten != 1 || result.UnknownEventCount != 0 || result.MissingFormatCount != 0 ||
		!strings.Contains(string(body), "ipi_raise: target_mask=16 (Rescheduling interrupts)") {
		t.Fatalf("kernel IPI authorities not rendered: result=%+v\n%s", result, body)
	}
}

func TestReservedMetadataSegmentsNeverBecomeRawCPUShards(t *testing.T) {
	for _, typ := range []uint32{segmentHeaderPage, segmentPrintk, segmentKallsyms} {
		if isRawTraceSegment(typ, 31) {
			t.Fatalf("reserved metadata segment %d became a raw CPU shard", typ)
		}
	}
	for _, typ := range []uint32{segmentRawTrace, 5, 29, 33, 35} {
		if !isRawTraceSegment(typ, 31) {
			t.Fatalf("valid raw CPU shard %d was lost", typ)
		}
	}
}

func TestBuiltinSysPrintkConflictRejectsOnlyAffectedIPIRow(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "kernel-ipi-conflict.sys")
	output := filepath.Join(dir, "kernel-ipi-conflict.systrace")
	format := strings.Join(syntheticFormatBlock("ipi_entry", 71, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("const char *", "reason", 8, 8, false),
	}), "\n")
	makeContent := func(address uint64) []byte {
		content := make([]byte, 16)
		binary.LittleEndian.PutUint16(content[:2], 71)
		binary.LittleEndian.PutUint32(content[4:8], 100)
		binary.LittleEndian.PutUint64(content[8:16], address)
		return content
	}
	var capture bytes.Buffer
	writeFileHeader(&capture, 1)
	writeSegment(&capture, segmentEventsFormat, []byte(format))
	writeSegment(&capture, segmentPrintk, []byte(strings.Join([]string{
		`0xc0aff2da : "Rescheduling interrupts"`,
		`0xC0AFF2DA : "malformed duplicate"`,
		`0xc0aff2bf : "Timer broadcast interrupts"`,
		"",
	}, "\n")))
	writeSegment(&capture, segmentCmdlines, []byte("100 irq-worker\n"))
	writeSegment(&capture, segmentTGIDs, []byte("100 100\n"))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents([]syntheticRawEvent{
		{EventID: 71, OffsetNS: 0, Content: makeContent(0xc0aff2da)},
		{EventID: 71, OffsetNS: 1_000, Content: makeContent(0xc0aff2bf)},
	}))
	if err := os.WriteFile(input, capture.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	if result.EventsWritten != 1 || result.UnknownEventCount != 0 || strings.Count(text, "ipi_entry:") != 1 ||
		!strings.Contains(text, "ipi_entry: (Timer broadcast interrupts)") || strings.Contains(text, "Rescheduling interrupts") {
		t.Fatalf("printk quarantine was not address-local: result=%+v\n%s", result, text)
	}
	caveats := strings.Join(result.Caveats, "\n")
	if !strings.Contains(caveats, "1 conflicting or malformed printk address mapping") ||
		!strings.Contains(caveats, "ipi_entry_missing_or_invalid_reason=1") {
		t.Fatalf("printk/body rejection disclosure missing: %s", caveats)
	}
}

func TestDirectCoreAdmissionStructurePinned(t *testing.T) {
	coreSource, err := os.ReadFile("core_payload.go")
	if err != nil {
		t.Fatal(err)
	}
	coreText := string(coreSource)
	for _, forbidden := range []string{
		"intByCleanName(", "intField(", "firstNonZero(", "renderKV(", "protoUint(", "protoString(",
		"numericFieldTypeAllowed(", "uniqueFieldByCleanNames(", "cleanFieldName(",
	} {
		if strings.Contains(coreText, forbidden) {
			t.Fatalf("typed core authority calls a permissive legacy reader %q", forbidden)
		}
	}
	for _, name := range []string{
		"sched_wakeup", "sched_wakeup_new", "sched_waking", "sched_blocked_reason",
		"cpu_frequency", "cpu_frequency_limits", "cpu_idle",
		"binder_transaction", "binder_transaction_received",
		"irq_handler_entry", "irq_handler_exit", "softirq_entry", "softirq_exit", "softirq_raise",
		"ipi_entry", "ipi_exit", "ipi_raise",
	} {
		if !strings.Contains(coreText, `"`+name+`"`) {
			t.Fatalf("closed core registry missing %s", name)
		}
	}
	for _, path := range []string{"render.go", "official_render.go"} {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		text := string(body)
		for _, oldName := range []string{
			"sched_wakeup", "sched_wakeup_new", "sched_waking", "sched_blocked_reason",
			"cpu_frequency", "cpu_frequency_limits", "cpu_idle",
			"binder_transaction", "binder_transaction_received",
			"irq_handler_entry", "irq_handler_exit", "softirq_entry", "softirq_exit", "softirq_raise",
			"ipi_entry", "ipi_exit", "ipi_raise",
		} {
			if strings.Contains(text, `"`+oldName+`"`) {
				t.Fatalf("legacy second core renderer remains in %s: %s", path, oldName)
			}
		}
		for _, dormantAuthority := range []string{"func renderBinderTransaction(", "func blockedCaller(", "func irqName("} {
			if strings.Contains(text, dormantAuthority) {
				t.Fatalf("dormant second core authority remains in %s: %s", path, dormantAuthority)
			}
		}
	}
	convertSource, err := os.ReadFile("convert.go")
	if err != nil {
		t.Fatal(err)
	}
	convertText := string(convertSource)
	rejected := strings.Index(convertText, "if admission == bodyRejected")
	if rejected < 0 {
		t.Fatal("production bodyRejected branch is missing")
	}
	headerFallback := strings.Index(convertText[rejected:], "if admission == bodyUnsupported")
	if headerFallback < 0 || !strings.Contains(convertText[rejected:rejected+headerFallback], "continue") {
		t.Fatal("production rejected body can still reach the unsupported header-only fallback")
	}
}
