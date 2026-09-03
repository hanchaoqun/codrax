package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// Pins for colleague_merge_audit §40.13 (V6-2): every visibility carrier row
// publishes under the ONE reserved event name; the wrapped record's original
// name lives only in the typed payload. The class being closed is "a metadata
// carrier wearing the identity name of what it carries" — so the pins are
// parametrised over wrapped families and bound to the producer registry, not
// to the HMFS fixture the carrier was first built for.

// TestTraceDBSourceRawVisibilityPublishesUnderReservedEventName (PIN 1): for
// scheduler, interrupt and filesystem-looking wrapped families alike, the
// header is the reserved name, the body starts with the wire token and the
// original name round-trips through event_name_b64.
func TestTraceDBSourceRawVisibilityPublishesUnderReservedEventName(t *testing.T) {
	for _, name := range []string{"sched_migrate_task", "irq_handler_entry", "hmfs_writepage"} {
		t.Run(name, func(t *testing.T) {
			capture, _ := traceDBRawVisibilityCaptureNamed(t, name, false)
			path := filepath.Join(t.TempDir(), name+".sys")
			if err := os.WriteFile(path, capture, 0o600); err != nil {
				t.Fatal(err)
			}
			authority, err := openConversionInputAuthority(path)
			if err != nil {
				t.Fatal(err)
			}
			defer authority.Close()
			inventory, err := scanTraceDBSourceNameInventory(context.Background(), authority)
			if err != nil {
				t.Fatal(err)
			}
			sink, err := newTraceDBInactiveOrdinaryRowSink(t.TempDir(), 8)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			coverage, err := publishTraceDBSourceRawVisibility(context.Background(), &inventory, sink)
			if err != nil {
				t.Fatal(err)
			}
			if coverage.RowsEmitted != 2 || len(sink.rows) != 2 {
				t.Fatalf("visibility publication mismatch for %s: coverage=%+v rows=%d", name, coverage, len(sink.rows))
			}
			for index, row := range sink.rows {
				body := row.line[strings.Index(row.line, ": ")+2:]
				if !strings.HasPrefix(body, tracequery.SourceRawVisibilityEventName+": "+tracequery.SourceRawVisibilityWire+" ") {
					t.Fatalf("carrier for %s not published under the reserved event name: %s", name, row.line)
				}
				if strings.Contains(row.line, name+": ") {
					t.Fatalf("carrier for %s still wears the wrapped record's name in its header: %s", name, row.line)
				}
				event, ok := tracequery.ParseLine(index+1, row.line, nil)
				if !ok || event.Type != tracequery.EventSourceRawVisibility ||
					event.Name != tracequery.SourceRawVisibilityEventName || event.SubsystemKind != "" {
					t.Fatalf("carrier for %s parsed outside the reserved advisory lane: %+v ok=%t", name, event, ok)
				}
				if got, ok := traceDBSourceRawVisibilityOriginalEventName(row.line); !ok || got != name {
					t.Fatalf("original name %q not recoverable from carrier payload: got=%q ok=%t", name, got, ok)
				}
			}
			if got := coverage.Metadata["published_format_names_witness"]; got != name {
				t.Fatalf("diagnostic format-name witness for %s drifted: %q", name, got)
			}
			if keys := TraceDBCoverageDiagnosticWitnessKeys(coverage.Metadata); !containsExact(keys, "published_format_names_witness") {
				t.Fatalf("format-name witness is not admitted into the diagnostic sideband: %v", keys)
			}
		})
	}
}

// traceDBRawVisibilitySchedMigrateFormat is a realistic OpenHarmony
// sched_migrate_task descriptor: it is NOT a strict target family, so every
// record becomes a visibility carrier.
func traceDBRawVisibilitySchedMigrateFormat() eventFormat {
	return eventFormat{
		ID: 1201, Name: "sched_migrate_task",
		Fields: []eventField{
			{Type: "unsigned short", Name: "common_type", Offset: 0, Size: 2},
			{Type: "unsigned char", Name: "common_flags", Offset: 2, Size: 1},
			{Type: "unsigned char", Name: "common_preempt_count", Offset: 3, Size: 1},
			{Type: "int", Name: "common_pid", Offset: 4, Size: 4, Signed: true},
			{Type: "char", Name: "comm[16]", Offset: 8, Size: 16, Signed: true},
			{Type: "pid_t", Name: "pid", Offset: 24, Size: 4, Signed: true},
			{Type: "int", Name: "prio", Offset: 28, Size: 4, Signed: true},
			{Type: "int", Name: "orig_cpu", Offset: 32, Size: 4, Signed: true},
			{Type: "int", Name: "dest_cpu", Offset: 36, Size: 4, Signed: true},
		},
		PrintFmt: `"comm=%s pid=%d prio=%d orig_cpu=%d dest_cpu=%d", REC->comm, REC->pid, REC->prio, REC->orig_cpu, REC->dest_cpu`,
	}
}

// syntheticRawMultiPageEvents lays records out over as many 4 KiB pages as
// they need, each page with an increasing base timestamp and a round-robin
// CPU, so a thousands-of-rows carrier family can be replayed page by page.
func syntheticRawMultiPageEvents(events []syntheticRawEvent, baseTS uint64, cpuNum int) ([]byte, int) {
	var out bytes.Buffer
	pages := 0
	for len(events) > 0 {
		page := make([]byte, tracePageSize)
		binary.LittleEndian.PutUint64(page[0:8], baseTS+uint64(pages)*1_000_000)
		page[16] = byte(pages % cpuNum)
		off := pageHeaderSize
		consumed := 0
		for _, ev := range events {
			content := append([]byte(nil), ev.Content...)
			binary.LittleEndian.PutUint16(content[0:2], ev.EventID)
			aligned := int((uint32(len(content)) + 3) &^ 3)
			if off+eventHeaderSize+aligned > len(page) {
				break
			}
			binary.LittleEndian.PutUint32(page[off:off+4], ev.OffsetNS)
			binary.LittleEndian.PutUint16(page[off+4:off+6], uint16(len(content)))
			copy(page[off+eventHeaderSize:], content)
			off += eventHeaderSize + aligned
			consumed++
		}
		if consumed == 0 {
			panic("synthetic raw event does not fit a page")
		}
		binary.LittleEndian.PutUint64(page[8:16], uint64(off-pageHeaderSize))
		out.Write(page)
		events = events[consumed:]
		pages++
	}
	return out.Bytes(), pages
}

func traceDBRawVisibilitySchedMigrateCapture(t *testing.T, records int) []byte {
	t.Helper()
	format := traceDBRawVisibilitySchedMigrateFormat()
	events := make([]syntheticRawEvent, 0, records)
	for i := 0; i < records; i++ {
		content := make([]byte, 40)
		content[2] = 0x04
		content[3] = 0x02
		binary.LittleEndian.PutUint32(content[4:8], 25827)
		copy(content[8:24], "com.tencent.mm")
		binary.LittleEndian.PutUint32(content[24:28], uint32(25827+i%7))
		binary.LittleEndian.PutUint32(content[28:32], 120)
		binary.LittleEndian.PutUint32(content[32:36], uint32(i%4))
		binary.LittleEndian.PutUint32(content[36:40], uint32((i+1)%4))
		events = append(events, syntheticRawEvent{EventID: uint16(format.ID), OffsetNS: uint32(1000 * (i + 1)), Content: content})
	}
	raw, _ := syntheticRawMultiPageEvents(events, 2942124416000, 4)
	var capture bytes.Buffer
	writeFileHeader(&capture, 4)
	header := capture.Bytes()
	binary.LittleEndian.PutUint16(header[0:2], traceStreamerRawTraceMagic)
	header[2] = harmonyRMQFileType
	capture.Reset()
	capture.Write(header)
	writeSegment(&capture, segmentCmdlines, []byte("25827 com.tencent.mm\n"))
	writeSegment(&capture, segmentEventsFormat, []byte(directPairFormatBlock(format.ID, format)))
	writeSegment(&capture, segmentRawTrace, raw)
	return capture.Bytes()
}

// TestTraceStreamerConversionSchedMigrateCarriersAuditClean (PIN 2, the
// ruling's acceptance shape): an OpenHarmony-style capture whose thousands of
// sched_migrate_task records ride the visibility carrier converts, and the
// windowed tracequery index / streaming state cluster over the converted
// artifact mint ZERO scheduler/cpu-input/duration integrity witnesses; the
// original event name stays recoverable for the diagnostic report.
func TestTraceStreamerConversionSchedMigrateCarriersAuditClean(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer fixture uses a POSIX shell")
	}
	const records = 3000
	dir := t.TempDir()
	input := filepath.Join(dir, "sched_migrate.sys")
	output := filepath.Join(dir, "sched_migrate.systrace")
	if err := os.WriteFile(input, traceDBRawVisibilitySchedMigrateCapture(t, records), 0o600); err != nil {
		t.Fatal(err)
	}
	fixtureDB := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
	traceStreamer := writeFakeTraceStreamer(t, dir, 0)
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)
	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: output,
		TraceEngine: traceEngineTraceStreamer, TraceStreamerPath: traceStreamer,
	})
	if err != nil {
		t.Fatal(err)
	}
	var visibility *TraceDBCoverage
	for index := range result.TraceDBCoverage {
		if result.TraceDBCoverage[index].Family == traceDBSourceRawVisibilityFamily {
			visibility = &result.TraceDBCoverage[index]
		}
	}
	if visibility == nil || visibility.RowsEmitted != records ||
		visibility.Metadata["publication_state"] != "published_complete_visibility_only_source_census" {
		t.Fatalf("sched_migrate_task carriers were not published completely: %+v", visibility)
	}
	// The ruling's acceptance criterion first: zero malformed audits on the
	// windowed lane. Wire-shape and diagnostic recoverability follow.
	window := tracequery.Query{TimeStart: 0, TimeEnd: 4000}
	idx, err := tracequery.BuildIndexWithOptions(context.Background(), output, tracequery.BuildOptions{
		TimeStart: window.TimeStart, TimeStartSet: true,
		TimeEnd: window.TimeEnd, TimeEndSet: true,
		AllowWindowedParse: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !idx.Windowed {
		t.Fatalf("acceptance index must exercise the windowed pre-parse audit lane")
	}
	stats := tracequery.ComputeWindowStats(idx, window)
	for _, caveat := range stats.Caveats {
		if strings.Contains(caveat, "scheduler_row_parse_incomplete") ||
			strings.Contains(caveat, "endpoint_parse_incomplete") ||
			strings.Contains(caveat, "cpu_input") {
			t.Fatalf("carrier rows poisoned a windowed audit face: %v", stats.Caveats)
		}
	}
	if len(stats.TopRunning) == 0 {
		t.Fatalf("windowed scheduler duration face is empty although only carriers were added: %+v", stats)
	}
	timeline := tracequery.ThreadTimeline(idx, tracequery.Query{PID: 201, TimeStart: window.TimeStart, TimeEnd: window.TimeEnd})
	if timeline.IntegrityFailure != "" {
		t.Fatalf("thread timeline failed closed on carrier rows: %+v", timeline)
	}
	// The converted artifact is a tracebundle, which the streaming state
	// cluster refuses by design (indexed path preserves provenance); the
	// streaming lane's zero-witness pin lives in tracequery on a plain text
	// fixture (TestSourceRawVisibilityReservedNameCarriersAuditZero).

	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "sched_migrate_task: ") {
		t.Fatalf("a carrier still wears the sched_migrate_task header name")
	}
	if got := strings.Count(string(body), traceDBSourceRawVisibilityEventName+": "+traceDBSourceRawVisibilityWire+" "); got != records {
		t.Fatalf("reserved-name carrier rows=%d want %d", got, records)
	}
	if got := visibility.Metadata["published_format_names_witness"]; got != "sched_migrate_task" {
		t.Fatalf("diagnostic report lost the wrapped format name: %q", got)
	}
}

// TestOwnedTraceValidationRejectsCarrierUnderForeignEventName (PIN 3): the
// owned-output postvalidation is the one throat every emitted row passes, so
// it is the runtime emission census — a carrier whose header is not the
// reserved name fails the artifact closed as an invalid event.
func TestOwnedTraceValidationRejectsCarrierUnderForeignEventName(t *testing.T) {
	format := traceDBRawVisibilityFormat("sched_migrate_task")
	payload, digest, err := traceDBSourceRawVisibilitySchemaFor(format)
	if err != nil {
		t.Fatal(err)
	}
	carrierBody, err := traceDBSourceRawVisibilityBody(format, traceDBRawVisibilityContent(format),
		&traceDBSourceRawVisibilitySchemaWire{payload: payload, digest: digest})
	if err != nil {
		t.Fatal(err)
	}
	known := traceDBPostvalidationKnownLine(t, 1_000_000)
	for _, tc := range []struct {
		name   string
		header string
		reason string
		body   string
	}{
		{name: "reserved_name_passes", header: tracequery.SourceRawVisibilityEventName, reason: ""},
		{name: "foreign_name_fails_closed", header: "sched_migrate_task", reason: traceDBPostvalidationEventInvalid},
		// 复核: a FUTURE body family built the same way (wire token at the
		// body start) under a semantic header is refused by the body-signature
		// half of the census even though no parser lane classifies it yet.
		{name: "future_family_under_semantic_header_fails_closed", header: "hmfs_writepage", reason: traceDBPostvalidationEventInvalid,
			body: "codrax_zz_future_family/v1 semantic_authority=none payload_b64=AAEC"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.body
			if body == "" {
				body = carrierBody
			}
			row, err := prepareTraceDBRenderedRowEnvelope(2_000_000, 1, "worker", 25827, 25827, 1, 4, 2, true,
				tc.header+": "+body)
			if err != nil {
				t.Fatal(err)
			}
			bytesBody := []byte(systraceHeader + known + row.line + "\n")
			target, sealed := adoptTraceDBPostvalidationFixture(t, bytesBody)
			_, coverage, err := validateSealedSystraceWithTraceQueryReceiptAndWire(
				context.Background(), sealed, target.FinalPath, 2, 0, 1, ownedTraceTestWireDigest(t, bytesBody))
			if tc.reason == "" {
				if err != nil {
					t.Fatalf("reserved-name carrier rejected by postvalidation: %v coverage=%+v", err, coverage)
				}
				return
			}
			var invariant *traceDBOutputInvariantError
			if !errors.As(err, &invariant) || invariant.Reason != tc.reason || coverage.Error != tc.reason {
				t.Fatalf("carrier under foreign header %q escaped the emission census: coverage=%+v err=%v",
					tc.header, coverage, err)
			}
		})
	}
}

func TestTraceDBSourceRawVisibilityFormatNamesWitnessBounded(t *testing.T) {
	names := make([]string, 0, 40)
	for i := 0; i < 40; i++ {
		names = append(names, fmt.Sprintf("family_%02d", 39-i))
	}
	got := traceDBSourceRawVisibilityFormatNamesWitness(names)
	parts := strings.Split(got, ";")
	if len(parts) != maxTraceDBSourceRawVisibilityFormatNameWitnesses+1 ||
		parts[0] != "family_00" || parts[len(parts)-1] != "+8 more" {
		t.Fatalf("witness is not sorted/bounded: %q", got)
	}
	if got := traceDBSourceRawVisibilityFormatNamesWitness([]string{"odd name!"}); !strings.HasPrefix(got, "name_sha256_") {
		t.Fatalf("unsafe name leaked verbatim into the witness: %q", got)
	}
	long := strings.Repeat("x", 100)
	got = traceDBSourceRawVisibilityFormatNamesWitness([]string{long + "a", long + "b", long + "c", long + "d", long + "e", long + "f"})
	if len(got) > maxTraceDBSourceRawVisibilityFormatNameWitnessBytes+len(";+9 more") || !strings.HasSuffix(got, " more") {
		t.Fatalf("witness byte cap not honoured: len=%d %q", len(got), got)
	}
}

// ---- PIN 4: structural registry + emission-site census ----

var traceDBCarrierWirePattern = regexp.MustCompile(`^codrax_[a-z_]+/v[0-9]+`)

// traceDBCarrierFamilyCensus is the structural tripwire behind the registry:
//
//	R1 every registered family publishes under the reserved event name;
//	R2 every ftrace-body wire literal (`codrax_<family>/v<N>` at the start of a
//	   string literal, comment carriers `# codrax_*` excluded) in the producer
//	   package belongs to a registered family — a new family cannot exist
//	   unregistered;
//	R3 in each family's emitter file, every rendered-row body argument is
//	   `<reserved-name const> + ": " + <body>` and at least one such site
//	   exists — a registered family cannot emit under any other header;
//	R4 the family's body builder is only called from its emitter file.
func traceDBCarrierFamilyCensus(sources map[string][]byte, families []traceDBCarrierFamily) []string {
	var violations []string
	registered := map[string]bool{}
	for _, family := range families {
		registered[family.Wire] = true
		switch family.Kind {
		case traceDBCarrierKindFtraceBody:
			if family.EventName != tracequery.SourceRawVisibilityEventName {
				violations = append(violations, fmt.Sprintf("R1 family %s publishes under %q, not the reserved event name", family.Wire, family.EventName))
			}
		case traceDBCarrierKindComment:
			// A comment carrier's whole contract is its `# <wire>` prefix
			// constant in the emitter file; it never wears an event name.
			if family.EventName != "" || family.BodyBuilder != "" {
				violations = append(violations, fmt.Sprintf("R1 comment family %s must carry neither an event name nor a body builder", family.Wire))
			}
			if src, ok := sources[family.EmitterFile]; ok && !strings.Contains(string(src), "\"# "+family.Wire+"\"") {
				violations = append(violations, fmt.Sprintf("R1 comment family %s: emitter %s declares no `# %s` prefix", family.Wire, family.EmitterFile, family.Wire))
			}
		default:
			violations = append(violations, fmt.Sprintf("R1 family %s has no kind", family.Wire))
		}
		if !traceDBCarrierWirePattern.MatchString(family.Wire) {
			violations = append(violations, fmt.Sprintf("R1 family wire %q is not a versioned codrax wire token", family.Wire))
		}
		if _, ok := sources[family.EmitterFile]; !ok {
			violations = append(violations, fmt.Sprintf("R1 family %s emitter file %s is missing", family.Wire, family.EmitterFile))
		}
	}
	fset := token.NewFileSet()
	parsed := map[string]*ast.File{}
	for name, src := range sources {
		file, err := parser.ParseFile(fset, name, src, 0)
		if err != nil {
			violations = append(violations, fmt.Sprintf("parse %s: %v", name, err))
			continue
		}
		parsed[name] = file
	}
	for name, file := range parsed {
		ast.Inspect(file, func(node ast.Node) bool {
			lit, ok := node.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(lit.Value)
			if err != nil || strings.HasPrefix(value, "# ") {
				return true
			}
			match := traceDBCarrierWirePattern.FindString(value)
			if match == "" || (len(value) > len(match) && value[len(match)] != ' ') {
				return true
			}
			if !registered[match] {
				violations = append(violations, fmt.Sprintf("R2 %s:%d wire literal %q is not a registered carrier family", name, fset.Position(lit.Pos()).Line, match))
			}
			return true
		})
	}
	for _, family := range families {
		if family.Kind != traceDBCarrierKindFtraceBody {
			continue // R3/R4 are ftrace-row emission rules
		}
		file := parsed[family.EmitterFile]
		if file == nil {
			continue
		}
		reservedIdents := map[string]bool{}
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.CONST {
				continue
			}
			for _, spec := range gen.Specs {
				value, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for index, ident := range value.Names {
					if index >= len(value.Values) {
						continue
					}
					switch expr := value.Values[index].(type) {
					case *ast.SelectorExpr:
						if pkg, ok := expr.X.(*ast.Ident); ok && pkg.Name == "tracequery" && expr.Sel.Name == "SourceRawVisibilityEventName" {
							reservedIdents[ident.Name] = true
						}
					case *ast.BasicLit:
						if lit, err := strconv.Unquote(expr.Value); err == nil && lit == tracequery.SourceRawVisibilityEventName {
							reservedIdents[ident.Name] = true
						}
					}
				}
			}
		}
		sites := 0
		builderDeclared := false
		ast.Inspect(file, func(node ast.Node) bool {
			if fn, ok := node.(*ast.FuncDecl); ok && fn.Name.Name == family.BodyBuilder {
				builderDeclared = true
			}
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			callee := ""
			switch fun := call.Fun.(type) {
			case *ast.Ident:
				callee = fun.Name
			case *ast.SelectorExpr:
				callee = fun.Sel.Name
			}
			if !strings.HasPrefix(callee, "prepareTraceDBRenderedRow") || len(call.Args) == 0 {
				return true
			}
			sites++
			bodyArg := call.Args[len(call.Args)-1]
			leftmost, hasHeaderSeparator := traceDBCarrierBodyArgShape(bodyArg)
			if leftmost == "" || !hasHeaderSeparator || !reservedIdents[leftmost] {
				violations = append(violations, fmt.Sprintf("R3 %s:%d rendered-row body is not `<reserved event name const> + \": \" + body` (leftmost=%q)",
					family.EmitterFile, fset.Position(call.Pos()).Line, leftmost))
			}
			return true
		})
		if sites == 0 {
			violations = append(violations, fmt.Sprintf("R3 %s has no rendered-row emission site for family %s", family.EmitterFile, family.Wire))
		}
		if !builderDeclared {
			violations = append(violations, fmt.Sprintf("R1 family %s body builder %s is not declared in %s", family.Wire, family.BodyBuilder, family.EmitterFile))
		}
		for name, other := range parsed {
			if name == family.EmitterFile {
				continue
			}
			ast.Inspect(other, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == family.BodyBuilder {
					violations = append(violations, fmt.Sprintf("R4 %s:%d calls %s outside its emitter file", name, fset.Position(call.Pos()).Line, family.BodyBuilder))
				}
				return true
			})
		}
	}
	return violations
}

// traceDBCarrierBodyArgShape walks a `a + b + c` chain: returns the leftmost
// identifier and whether a literal ": " header separator is concatenated.
func traceDBCarrierBodyArgShape(expr ast.Expr) (string, bool) {
	separator := false
	var walk func(ast.Expr) string
	walk = func(e ast.Expr) string {
		switch v := e.(type) {
		case *ast.ParenExpr:
			return walk(v.X)
		case *ast.BinaryExpr:
			if v.Op != token.ADD {
				return ""
			}
			if lit, ok := v.Y.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if value, err := strconv.Unquote(lit.Value); err == nil && value == ": " {
					separator = true
				}
			}
			return walk(v.X)
		case *ast.Ident:
			return v.Name
		}
		return ""
	}
	return walk(expr), separator
}

func traceDBCarrierCensusSources(t *testing.T) map[string][]byte {
	t.Helper()
	sources := map[string][]byte{}
	// The producer package AND the parser package: after §40.13 the live wire
	// literals are declared in internal/tracequery and referenced from here by
	// selector, so a census over this directory alone would be vacuous (复核).
	for _, dir := range []string{".", "../tracequery"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			src, err := os.ReadFile(dir + "/" + name)
			if err != nil {
				t.Fatal(err)
			}
			key := name
			if dir != "." {
				key = "tracequery/" + name
			}
			sources[key] = src
		}
	}
	if len(sources) < 80 {
		t.Fatalf("carrier census read only %d producer/parser files; package layout drifted", len(sources))
	}
	return sources
}

// TestTraceDBReservedCarrierFamilyRegistry (PIN 4) runs the census over the
// live producer package and proves the census itself bites (self-red).
func TestTraceDBReservedCarrierFamilyRegistry(t *testing.T) {
	sources := traceDBCarrierCensusSources(t)
	if len(traceDBReservedCarrierFamilies) == 0 {
		t.Fatal("carrier family registry is empty")
	}
	if violations := traceDBCarrierFamilyCensus(sources, traceDBReservedCarrierFamilies); len(violations) != 0 {
		t.Fatalf("carrier family census violations:\n%s", strings.Join(violations, "\n"))
	}
	if tracequery.SourceRawVisibilityEventName != "codrax_source_raw_event" ||
		traceDBReservedCarrierFamilies[0].Wire != tracequery.SourceRawVisibilityWire {
		t.Fatalf("reserved wire/name constants drifted from the parser-side single source: %+v", traceDBReservedCarrierFamilies)
	}

	t.Run("self_red_original_name_header", func(t *testing.T) {
		emitter := traceDBReservedCarrierFamilies[0].EmitterFile
		mutated := map[string][]byte{}
		for name, src := range sources {
			mutated[name] = src
		}
		before := string(sources[emitter])
		after := strings.Replace(before, "traceDBSourceRawVisibilityEventName+\": \"+visibilityBody", "format.Name+\": \"+visibilityBody", 1)
		if after == before {
			t.Fatalf("self-red mutation did not apply; emission site shape drifted")
		}
		mutated[emitter] = []byte(after)
		violations := traceDBCarrierFamilyCensus(mutated, traceDBReservedCarrierFamilies)
		if len(violations) == 0 || !strings.Contains(strings.Join(violations, "\n"), "R3 "+emitter) {
			t.Fatalf("census did not report a carrier emitted under the wrapped record's name: %v", violations)
		}
	})
	t.Run("self_red_unregistered_wire_literal", func(t *testing.T) {
		mutated := map[string][]byte{}
		for name, src := range sources {
			mutated[name] = src
		}
		mutated["zz_future_family.go"] = []byte("package hitraceconv\n\nconst futureWire = \"codrax_future_family/v1\"\n")
		violations := traceDBCarrierFamilyCensus(mutated, traceDBReservedCarrierFamilies)
		if !strings.Contains(strings.Join(violations, "\n"), "R2 zz_future_family.go") {
			t.Fatalf("census did not report an unregistered carrier wire literal: %v", violations)
		}
	})
	t.Run("self_red_family_off_reserved_name", func(t *testing.T) {
		families := []traceDBCarrierFamily{traceDBReservedCarrierFamilies[0]}
		families[0].EventName = "sched_migrate_task"
		violations := traceDBCarrierFamilyCensus(sources, families)
		if !strings.Contains(strings.Join(violations, "\n"), "R1 family") {
			t.Fatalf("census accepted a registry entry off the reserved name: %v", violations)
		}
	})
}
