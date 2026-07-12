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

func TestEventFormatCatalogDuplicateIDAuthority(t *testing.T) {
	base := strings.Join(syntheticFormatBlock("sched_wakeup", 10, matrixWakeupFields()), "\n")
	other := strings.Join(syntheticFormatBlock("sched_wakeup", 11, matrixWakeupFields()), "\n")
	conflict := strings.Join(syntheticFormatBlock("sched_blocked_reason", 10, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("int", "pid", 8, 4, true),
		syntheticField("unsigned long", "caller", 12, 8, false),
	}), "\n")
	catalog, err := parseEventFormats([]byte(strings.Join([]string{base, base, other, conflict, base}, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := catalog.Formats[10]; ok || !catalog.Poisoned[10] {
		t.Fatalf("conflicting ID was rescued by a later duplicate: %+v", catalog)
	}
	if got, ok := catalog.Formats[11]; !ok || got.Name != "sched_wakeup" || catalog.Poisoned[11] {
		t.Fatalf("unrelated descriptor was damaged: %+v", catalog)
	}

	destination, err := parseEventFormats([]byte(base + "\n" + other))
	if err != nil {
		t.Fatal(err)
	}
	cleanAgain, err := parseEventFormats([]byte(base))
	if err != nil {
		t.Fatal(err)
	}
	mergeEventFormatCatalog(&destination, cleanAgain)
	if _, ok := destination.Formats[10]; !ok || destination.Poisoned[10] {
		t.Fatalf("identical cross-segment descriptor was not idempotent: %+v", destination)
	}
	conflictingSegment, err := parseEventFormats([]byte(conflict))
	if err != nil {
		t.Fatal(err)
	}
	mergeEventFormatCatalog(&destination, conflictingSegment)
	mergeEventFormatCatalog(&destination, cleanAgain)
	if _, ok := destination.Formats[10]; ok || !destination.Poisoned[10] {
		t.Fatalf("cross-segment conflict was rescued: %+v", destination)
	}
	if _, ok := destination.Formats[11]; !ok {
		t.Fatalf("cross-segment conflict escaped its exact ID: %+v", destination)
	}

	fieldConflictFields := matrixWakeupFields()
	fieldConflictFields[len(fieldConflictFields)-1] = syntheticField("int", "target_cpu", 40, 4, true)
	fieldConflict := strings.Join(syntheticFormatBlock("sched_wakeup", 12, fieldConflictFields), "\n")
	fieldBase := strings.Join(syntheticFormatBlock("sched_wakeup", 12, matrixWakeupFields()), "\n")
	fieldCatalog, err := parseEventFormats([]byte(fieldBase + "\n" + fieldConflict))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := fieldCatalog.Formats[12]; ok || !fieldCatalog.Poisoned[12] {
		t.Fatalf("same-name layout conflict was not quarantined: %+v", fieldCatalog)
	}

	duplicateID := strings.Replace(base, "ID: 10", "ID: 10\nID: 13", 1)
	duplicateIDCatalog, err := parseEventFormats([]byte(duplicateID + "\n" + strings.Join(syntheticFormatBlock("sched_wakeup", 13, matrixWakeupFields()), "\n")))
	if err != nil {
		t.Fatal(err)
	}
	if len(duplicateIDCatalog.Formats) != 0 || !duplicateIDCatalog.Poisoned[10] || !duplicateIDCatalog.Poisoned[13] {
		t.Fatalf("duplicate IDs inside one descriptor were not fail-closed for every ambiguous ID: %+v", duplicateIDCatalog)
	}

	duplicatePrintFmt := base + "\n" + `print fmt: "late override"`
	duplicatePrintCatalog, err := parseEventFormats([]byte(duplicatePrintFmt))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := duplicatePrintCatalog.Formats[10]; ok || !duplicatePrintCatalog.Poisoned[10] {
		t.Fatalf("duplicate print fmt inside one descriptor was not fail-closed: %+v", duplicatePrintCatalog)
	}

	duplicateField := strings.Replace(base, `print fmt: "synthetic"`, syntheticField("unsigned long", "target_cpu", 48, 8, false)+"\n"+`print fmt: "synthetic"`, 1)
	malformedField := strings.Replace(base, `print fmt: "synthetic"`, "field: malformed\n"+`print fmt: "synthetic"`, 1)
	fieldGrammarInjection := strings.Replace(base,
		syntheticField("char", "comm[16]", 8, 16, false),
		"\tfield:char comm[16];\toffset:8;\tsize:16;\tsigned:0;\toffset:48;\tsize:4;\tsigned:0;", 1)
	invalidSigned := strings.Replace(base, "signed:1;", "signed:2;", 1)
	for name, body := range map[string]string{
		"duplicate clean field authority": duplicateField,
		"malformed field declaration":     malformedField,
		"field grammar injection":         fieldGrammarInjection,
		"invalid signed declaration":      invalidSigned,
	} {
		t.Run(name, func(t *testing.T) {
			candidate, err := parseEventFormats([]byte(body + "\n" + base + "\n" + other))
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := candidate.Formats[10]; ok || !candidate.Poisoned[10] {
				t.Fatalf("malformed descriptor was rescued by a later clean block: %+v", candidate)
			}
			if _, ok := candidate.Formats[11]; !ok || candidate.Poisoned[11] {
				t.Fatalf("malformed descriptor poisoned an unrelated ID: %+v", candidate)
			}
		})
	}

	for name, nameless := range map[string]string{
		"missing name": strings.Replace(base, "name: sched_wakeup\n", "", 1),
		"empty name":   strings.Replace(base, "name: sched_wakeup", "name: ", 1),
	} {
		t.Run(name, func(t *testing.T) {
			destination, err := parseEventFormats([]byte(base + "\n" + other))
			if err != nil {
				t.Fatal(err)
			}
			bad, err := parseEventFormats([]byte(nameless))
			if err != nil {
				t.Fatal(err)
			}
			clean, err := parseEventFormats([]byte(base))
			if err != nil {
				t.Fatal(err)
			}
			mergeEventFormatCatalog(&destination, bad)
			mergeEventFormatCatalog(&destination, clean)
			if _, ok := destination.Formats[10]; ok || !destination.Poisoned[10] {
				t.Fatalf("nameless descriptor failed to quarantine its ID permanently: %+v", destination)
			}
			if _, ok := destination.Formats[11]; !ok || destination.Poisoned[11] {
				t.Fatalf("nameless descriptor poisoned an unrelated ID: %+v", destination)
			}
		})
	}

	for name, normalized := range map[string]string{
		"no-space ID": strings.Replace(conflict, "ID: 10", "ID:10", 1),
		"tab ID":      strings.Replace(conflict, "ID: 10", "ID:\t10", 1),
		"tab name":    strings.Replace(conflict, "name: sched_blocked_reason", "name:\tsched_blocked_reason", 1),
		"tab print":   strings.Replace(base, `print fmt: "synthetic"`, "print fmt:\t\"different\"", 1),
	} {
		t.Run(name, func(t *testing.T) {
			destination, err := parseEventFormats([]byte(base + "\n" + other))
			if err != nil {
				t.Fatal(err)
			}
			candidate, err := parseEventFormats([]byte(normalized))
			if err != nil {
				t.Fatal(err)
			}
			clean, err := parseEventFormats([]byte(base))
			if err != nil {
				t.Fatal(err)
			}
			mergeEventFormatCatalog(&destination, candidate)
			mergeEventFormatCatalog(&destination, clean)
			if _, ok := destination.Formats[10]; ok || !destination.Poisoned[10] {
				t.Fatalf("reserved-key whitespace variant bypassed permanent quarantine: %+v", destination)
			}
			if _, ok := destination.Formats[11]; !ok || destination.Poisoned[11] {
				t.Fatalf("reserved-key whitespace variant damaged an unrelated ID: %+v", destination)
			}
		})
	}
}

func TestBuiltinSysConflictingDescriptorIDIsCoverageOnlyAndSiblingSurvives(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "descriptor-conflict.sys")
	output := filepath.Join(dir, "descriptor-conflict.systrace")
	wakeup10 := strings.Join(syntheticFormatBlock("sched_wakeup", 10, matrixWakeupFields()), "\n")
	wakeup11 := strings.Join(syntheticFormatBlock("sched_wakeup", 11, matrixWakeupFields()), "\n")
	conflict10 := strings.Join(syntheticFormatBlock("sched_blocked_reason", 10, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("int", "pid", 8, 4, true),
		syntheticField("unsigned long", "caller", 12, 8, false),
		syntheticField("unsigned int", "iowait", 20, 4, false),
		syntheticField("unsigned long", "delay", 24, 8, false),
	}), "\n")

	first := syntheticWakeupContent(10)
	second := syntheticWakeupContent(11)
	binary.LittleEndian.PutUint16(second[0:2], 11)
	page := syntheticRawPageEvents([]syntheticRawEvent{
		{EventID: 10, OffsetNS: 0, Content: first},
		{EventID: 11, OffsetNS: 1_000, Content: second},
	})
	var capture bytes.Buffer
	writeFileHeader(&capture, 1)
	writeSegment(&capture, segmentEventsFormat, []byte(wakeup10+"\n"+wakeup11))
	writeSegment(&capture, segmentEventsFormat, []byte(conflict10))
	writeSegment(&capture, segmentEventsFormat, []byte(wakeup10))
	writeSegment(&capture, segmentCmdlines, []byte("36379 app\n"))
	writeSegment(&capture, segmentTGIDs, []byte("36379 36379\n"))
	writeSegment(&capture, segmentRawTrace, page)
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
	if strings.Count(text, "sched_wakeup:") != 1 || strings.Contains(text, "sched_blocked_reason:") {
		t.Fatalf("conflicting descriptor decoded or clean sibling lost:\n%s", text)
	}
	if result.EventsWritten != 1 || result.MissingFormatCount != 0 {
		t.Fatalf("descriptor conflict was misclassified: %+v", result)
	}
	caveats := strings.Join(result.Caveats, "\n")
	if !strings.Contains(caveats, "1 conflicting or malformed raw ftrace event descriptor ID") ||
		!strings.Contains(caveats, "1 raw ftrace event row(s) referenced a conflicting or malformed descriptor ID") {
		t.Fatalf("descriptor conflict disclosure missing: %s", caveats)
	}
}

func TestBuiltinSysAllDescriptorsConflictingStillPublishesCoverage(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "all-descriptors-conflict.sys")
	output := filepath.Join(dir, "all-descriptors-conflict.systrace")
	wakeup := strings.Join(syntheticFormatBlock("sched_wakeup", 10, matrixWakeupFields()), "\n")
	conflict := strings.Join(syntheticFormatBlock("sched_blocked_reason", 10, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("int", "pid", 8, 4, true),
		syntheticField("unsigned long", "caller", 12, 8, false),
	}), "\n")

	var capture bytes.Buffer
	writeFileHeader(&capture, 1)
	writeSegment(&capture, segmentEventsFormat, []byte(wakeup))
	writeSegment(&capture, segmentEventsFormat, []byte(conflict))
	writeSegment(&capture, segmentRawTrace, syntheticRawPageEvents([]syntheticRawEvent{{
		EventID:  10,
		OffsetNS: 0,
		Content:  syntheticWakeupContent(10),
	}}))
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
	if result.EventsWritten != 0 || result.MissingFormatCount != 0 || result.UnknownEventCount != 0 {
		t.Fatalf("all-conflicting capture counts were misclassified: %+v", result)
	}
	if strings.Contains(string(body), "sched_wakeup:") || !strings.HasPrefix(string(body), "# tracer: nop") {
		t.Fatalf("all-conflicting capture did not remain header-only compatible coverage:\n%s", body)
	}
	caveats := strings.Join(result.Caveats, "\n")
	if !strings.Contains(caveats, "1 conflicting or malformed raw ftrace event descriptor ID") ||
		!strings.Contains(caveats, "1 raw ftrace event row(s) referenced a conflicting or malformed descriptor ID") {
		t.Fatalf("all-conflicting capture coverage disclosure missing: %s", caveats)
	}
}
