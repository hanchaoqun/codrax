package hitraceconv

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// TestOwnedTraceValidationTraceDBRecordSequenceWitnessNamesTheRightRow
// (§40.43 F-carrier-2 G): the record-sequence refusal is split precisely —
// an ordinary ftrace row published after the typed trace_db suffix began is
// witnessed as trace_db_record_sequence_foreign_row naming THAT row, while a
// typed record carrier that breaks the chunk/ordinal/digest contract keeps
// trace_db_record_sequence and names the carrier with the producer bytes
// after its `# <wire> ` prefix (the comment form never carries an
// "<eventName>: " marker, so the old ftrace-only body derivation showed "").
func TestOwnedTraceValidationTraceDBRecordSequenceWitnessNamesTheRightRow(t *testing.T) {
	headerLines := strings.Count(systraceHeader, "\n")
	known := traceDBPostvalidationKnownLine(t, 1_000_000)
	later := traceDBPostvalidationKnownLine(t, 2_000_000)
	schema := traceDBPostvalidationTypedRecordLines("schema", 1, 0, []byte("schema"), 211)
	receipt := traceDBPostvalidationTypedRecordLines("receipt", 1, 0, []byte("receipt"), 211)
	// A single-chunk forgery is refused by the parser itself (unparsed row);
	// only a multi-chunk logical record reaches the validator's whole-record
	// digest, which refuses at the LAST chunk — that carrier is the named row.
	multiChunk := traceDBPostvalidationTypedRecordLines("schema", 1, 0, []byte(strings.Repeat("schema", 100)), 211)
	if len(multiChunk) < 2 {
		t.Fatalf("fixture is not multi-chunk: %d", len(multiChunk))
	}
	parts := strings.Fields(multiChunk[0])
	validHash := strings.TrimPrefix(parts[len(parts)-1], "record_sha256=")
	forgedSchema := strings.ReplaceAll(strings.Join(multiChunk, ""), "record_sha256="+validHash, "record_sha256="+strings.Repeat("0", sha256.Size*2))
	if forgedSchema == strings.Join(multiChunk, "") {
		t.Fatal("fixture did not forge the record hash")
	}
	forgedLast := strings.SplitAfter(forgedSchema, "\n")[len(multiChunk)-1]
	forgedBody := strings.TrimSuffix(strings.TrimPrefix(forgedLast, tracequery.TraceDBTextRecordPrefix+" "), "\n")
	for _, tc := range []struct {
		name           string
		body           string
		expectedRows   int
		typedPreserved int
		wantKind       TraceEventInvalidKind
		wantLine       int
		wantEventName  string
		wantEventType  tracequery.EventType
		wantBodyPrefix string
	}{
		{
			name:           "ordinary_row_after_typed_suffix_is_the_foreign_row",
			body:           systraceHeader + known + schema[0] + receipt[0] + later,
			expectedRows:   4,
			typedPreserved: 2,
			wantKind:       TraceEventInvalidTraceDBRecordSequenceForeignRow,
			wantLine:       headerLines + 4,
			wantEventName:  "sched_wakeup",
			wantEventType:  tracequery.EventSchedWakeup,
			wantBodyPrefix: "comm=app pid=20 prio=53 target_cpu=2",
		},
		{
			name:           "record_contract_break_names_the_carrier_with_its_bytes",
			body:           systraceHeader + known + forgedSchema + receipt[0],
			expectedRows:   1 + len(multiChunk) + 1,
			typedPreserved: len(multiChunk) + 1,
			wantKind:       TraceEventInvalidTraceDBRecordSequence,
			wantLine:       headerLines + 1 + len(multiChunk),
			wantEventName:  "codrax_trace_db_record",
			wantEventType:  tracequery.EventTraceDBRecord,
			wantBodyPrefix: traceEventInvalidWitnessExcerpt(forgedBody),
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.HasPrefix(tc.wantBodyPrefix, fmt.Sprintf("kind=schema table_id=1 row_ordinal=0 chunk=%d chunks=%d", len(multiChunk), len(multiChunk))) &&
				!strings.HasPrefix(tc.wantBodyPrefix, "comm=") {
				t.Fatalf("expected body prefix is not producer bytes: %q", tc.wantBodyPrefix)
			}
			target, sealed := adoptTraceDBPostvalidationFixture(t, []byte(tc.body))
			_, coverage, err := validateSealedSystraceWithTraceQueryReceipt(
				t.Context(), sealed, target.FinalPath, tc.expectedRows, tc.typedPreserved)
			reason, typed := traceDBOutputInvariantReason(err)
			if !typed || reason != traceDBPostvalidationEventInvalid || coverage.Error != traceDBPostvalidationEventInvalid {
				t.Fatalf("sequence violation escaped: reason=%q coverage=%+v err=%v", reason, coverage, err)
			}
			var witness *TraceEventInvalidWitnessError
			if !errors.As(err, &witness) {
				t.Fatalf("no typed witness on the error graph: %v", err)
			}
			if witness.Kind != tc.wantKind || witness.Line != tc.wantLine || witness.EventName != tc.wantEventName ||
				witness.EventType != tc.wantEventType || witness.BodyPrefix != tc.wantBodyPrefix {
				t.Fatalf("witness names the wrong row or kind:\n got=%+v\nwant kind=%s line=%d event_name=%s event_type=%s body_prefix=%q",
					witness, tc.wantKind, tc.wantLine, tc.wantEventName, tc.wantEventType, tc.wantBodyPrefix)
			}
			if witness.BodyPrefix == "" || !utf8.ValidString(witness.BodyPrefix) ||
				len(witness.BodyPrefix) > maxTraceEventInvalidWitnessBodyBytes {
				t.Fatalf("body prefix is empty, unbounded or invalid UTF-8: %q", witness.BodyPrefix)
			}
			columns := strings.Join(coverage.ColumnsPresent, "\n")
			for _, want := range []string{
				"event_invalid_kind=" + string(tc.wantKind),
				fmt.Sprintf("event_invalid_line=%d", tc.wantLine),
				"event_invalid_event_name=" + tc.wantEventName,
				fmt.Sprintf("event_invalid_body_prefix=%q", tc.wantBodyPrefix),
			} {
				if !strings.Contains(columns, want) {
					t.Fatalf("coverage detail lacks %q:\n%s", want, columns)
				}
			}
		})
	}
}

// TestTraceEventInvalidWitnessExcerptBoundedEscapedProducerIdentifying
// (§40.43 F-carrier-2 H): the witness excerpt cuts at the 64-byte rune
// boundary through the shared single source and escapes invalid bytes, so an
// invalid first byte keeps the producer token, an emoji straddling the bound
// is not split, a short body with a stray byte keeps every byte readable,
// and a non-empty body never collapses to "".
func TestTraceEventInvalidWitnessExcerptBoundedEscapedProducerIdentifying(t *testing.T) {
	const budget = maxTraceEventInvalidWitnessBodyBytes
	long := "codrax_agent/v2 started " + strings.Repeat("payload ", 12)
	if len(long) < 90 {
		t.Fatalf("fixture too short: %d", len(long))
	}
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{name: "empty", body: "", want: ""},
		{name: "short_verbatim", body: "comm=app pid=20", want: "comm=app pid=20"},
		{name: "ascii_cut_at_budget", body: long, want: long[:budget]},
		{name: "invalid_first_byte_keeps_producer_token", body: "\xff" + long, want: `\xff` + long[:budget-1]},
		{name: "invalid_byte_inside_budget_does_not_collapse", body: long[:12] + "\xff" + long[12:], want: long[:12] + `\xff` + long[12:budget-1]},
		{name: "short_body_invalid_byte_escaped", body: "comm=app\xffpid=1", want: `comm=app\xffpid=1`},
		{name: "emoji_straddling_the_bound_is_not_split", body: strings.Repeat("a", budget-2) + "😀tail", want: strings.Repeat("a", budget-2)},
		{name: "cjk_cut_on_rune_boundary", body: strings.Repeat("中", 30), want: strings.Repeat("中", budget/3)},
		{name: "continuation_run_longer_than_budget", body: strings.Repeat("\x80", budget+6), want: strings.Repeat(`\x80`, budget)},
		{name: "replacement_char_is_a_rune_not_an_invalid_byte", body: "x\uFFFDy", want: "x\uFFFDy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := traceEventInvalidWitnessExcerpt(tc.body)
			if got != tc.want {
				t.Fatalf("excerpt=%q want %q", got, tc.want)
			}
			if !utf8.ValidString(got) || (tc.body != "" && got == "") {
				t.Fatalf("excerpt is empty or invalid UTF-8: %q", got)
			}
		})
	}
	// Row-body derivation feeding the excerpt: ftrace marker form, comment
	// carrier form (bytes after `# <wire> `), and a comment without a wire.
	row, err := prepareTraceDBRenderedRow(1_000_000, 0, "waker", 10, 10, 1, "sched_wakeup: comm=app pid=20 prio=53 target_cpu=2")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name      string
		text      string
		eventName string
		want      string
	}{
		{name: "ftrace_row", text: row.line, eventName: "sched_wakeup", want: "comm=app pid=20 prio=53 target_cpu=2"},
		{name: "comment_carrier", text: tracequery.TraceDBTextRecordPrefix + " kind=schema table_id=1", eventName: "codrax_trace_db_record", want: "kind=schema table_id=1"},
		{name: "comment_without_wire", text: "# plain comment", eventName: "", want: "plain comment"},
		{name: "marker_missing", text: "no marker here", eventName: "sched_wakeup", want: ""},
	} {
		if got := traceEventInvalidWitnessBodyPrefix(tc.text, tc.eventName); got != tc.want {
			t.Fatalf("%s: body prefix=%q want %q", tc.name, got, tc.want)
		}
	}
}
