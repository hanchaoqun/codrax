package hitraceconv

import (
	"context"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestTraceDBIRQHappyFamiliesRoundTrip(t *testing.T) {
	statements := traceDBIRQFixtureSchema()
	statements = append(statements,
		"INSERT INTO args VALUES (1, 1, 0, 32, 100)",
		"INSERT INTO args VALUES (2, 2, 1, 10, 100)",
		"INSERT INTO args VALUES (3, 1, 0, 4294967295, 101)",
		"INSERT INTO args VALUES (4, 2, 1, 11, 101)",
		"INSERT INTO args VALUES (5, 3, 0, 0, 200)",
		"INSERT INTO args VALUES (6, 2, 1, 20, 200)",
		"INSERT INTO args VALUES (7, 3, 0, 5, 205)",
		"INSERT INTO args VALUES (8, 2, 1, 25, 205)",
		"INSERT INTO args VALUES (9, 3, 0, 9, 209)",
		"INSERT INTO args VALUES (10, 2, 1, 29, 209)",
		"INSERT INTO irq VALUES (1, 1000000, 100000, 0, 'irq', 'uart', 100, '')",
		"INSERT INTO irq VALUES (2, 1200000, 100000, 4095, 'irq', 'arch_timer', 101, '')",
		"INSERT INTO irq VALUES (3, 1400000, 100000, 1, 'softirq', 'HI', 200, '')",
		"INSERT INTO irq VALUES (4, 1600000, 100000, 2, 'softirq', 'BLOCK_IOPOLL', 205, '')",
		"INSERT INTO irq VALUES (5, 1800000, 100000, 3, 'softirq', 'RCU', 209, '')",
		"INSERT INTO irq VALUES (6, 2000000, 100000, 4, 'ipi', 'Rescheduling interrupts', 'ignored', '1')",
		"INSERT INTO irq VALUES (7, 2200000, 0, 5, 'ipi', '(Function call interrupts)', NULL, '1')",
	)
	body, coverage, index := exportTraceDBIRQFixture(t, statements)
	if coverage.RowsRead != 7 || coverage.RowsEmitted != 14 || coverage.Skipped != "" {
		t.Fatalf("IRQ happy coverage mismatch: %+v", coverage)
	}
	for _, want := range []string{
		"[000] ....     0.001000: irq_handler_entry: irq=32 name=uart",
		"[4095] ....     0.001200: irq_handler_entry: irq=4294967295 name=arch_timer",
		"softirq_entry: vec=0 [action=HI]",
		"softirq_entry: vec=5 [action=BLOCK_IOPOLL]",
		"softirq_exit: vec=9 [action=RCU]",
		"ipi_entry: (Rescheduling interrupts)",
		"ipi_exit: (Rescheduling interrupts)",
		"ipi_entry: (Function call interrupts)",
		"ipi_exit: (Function call interrupts)",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("IRQ happy output missing %q:\n%s", want, body)
		}
	}
	counts := map[tracequery.EventType]int{}
	for _, event := range index.Events {
		counts[event.Type]++
	}
	if counts[tracequery.EventIRQ] != 4 || counts[tracequery.EventSoftIRQ] != 6 || counts[tracequery.EventIPI] != 4 {
		t.Fatalf("IRQ families lost in tracequery round trip: %v events=%+v", counts, index.Events)
	}
	stats := tracequery.ComputeWindowStats(index, tracequery.Query{TimeStart: 0.0009, TimeEnd: 0.0023})
	if !traceDBInterruptActivityHas(stats.IRQActivity, 0, 32, 0.1) ||
		!traceDBInterruptActivityHas(stats.IRQActivity, 4095, int(math.MaxUint32), 0.1) ||
		!traceDBInterruptActivityHas(stats.SoftIRQActivity, 2, 5, 0.1) ||
		!traceDBInterruptActivityNameHas(stats.IPIActivity, 4, "Rescheduling interrupts", 0.1) {
		t.Fatalf("IRQ duration/CPU round trip mismatch: irq=%+v soft=%+v ipi=%+v", stats.IRQActivity, stats.SoftIRQActivity, stats.IPIActivity)
	}
	for key, want := range map[string]string{
		"cpu":             "irq.callid strict SQLite INTEGER in range 0..4095",
		"ipi.exit_reason": "irq.name interval projection; upstream IPI exit reason is not retained",
		"softirq.action":  "canonical tuple irq.name==args.irq_ret==action(args.vec)",
	} {
		if coverage.FieldSources[key] != want {
			t.Fatalf("IRQ provenance %s=%q, want %q: %+v", key, coverage.FieldSources[key], want, coverage)
		}
	}
}

func TestTraceDBIRQStrictCPUAndTimeGates(t *testing.T) {
	t.Run("CPU dynamic type and range", func(t *testing.T) {
		statements := traceDBIRQFixtureSchema()
		statements = append(statements,
			"INSERT INTO irq VALUES (1, 100, 10, NULL, 'irq', 'uart', 100, '')",
			"INSERT INTO irq VALUES (2, 110, 10, -1, 'irq', 'uart', 100, '')",
			"INSERT INTO irq VALUES (3, 120, 10, 4096, 'irq', 'uart', 100, '')",
			"INSERT INTO irq VALUES (4, 130, 10, 'cpu-x', 'irq', 'uart', 100, '')",
			"INSERT INTO irq VALUES (5, 140, 10, 1.5, 'irq', 'uart', 100, '')",
		)
		body, coverage, _ := exportTraceDBIRQFixture(t, statements)
		if coverage.RowsRead != 5 || coverage.RowsEmitted != 0 || !strings.Contains(coverage.Skipped, "invalid_cpu=5") || traceDBIRQEndpointCount(body) != 0 {
			t.Fatalf("invalid CPU rows escaped: %+v\n%s", coverage, body)
		}
	})

	t.Run("timestamp duration and overflow", func(t *testing.T) {
		statements := traceDBIRQFixtureSchema()
		statements = append(statements,
			"INSERT INTO irq VALUES (1, NULL, 10, 1, 'irq', 'uart', 100, '')",
			"INSERT INTO irq VALUES (2, 'ts-x', 10, 1, 'irq', 'uart', 100, '')",
			"INSERT INTO irq VALUES (3, -1, 10, 1, 'irq', 'uart', 100, '')",
			"INSERT INTO irq VALUES (4, 100, NULL, 1, 'irq', 'uart', 100, '')",
			"INSERT INTO irq VALUES (5, 110, 'dur-x', 1, 'irq', 'uart', 100, '')",
			"INSERT INTO irq VALUES (6, 120, -1, 1, 'irq', 'uart', 100, '')",
			"INSERT INTO irq VALUES (7, "+strconv.FormatInt(math.MaxInt64-5, 10)+", 10, 1, 'irq', 'uart', 100, '')",
		)
		body, coverage, _ := exportTraceDBIRQFixture(t, statements)
		for _, want := range []string{"invalid_timestamp=3", "invalid_duration=3", "interval_end_overflow=1"} {
			if !strings.Contains(coverage.Skipped, want) {
				t.Fatalf("time coverage missing %q: %+v", want, coverage)
			}
		}
		if coverage.RowsRead != 7 || coverage.RowsEmitted != 0 || traceDBIRQEndpointCount(body) != 0 {
			t.Fatalf("invalid time rows escaped: %+v\n%s", coverage, body)
		}
	})
}

func TestTraceDBIRQCategoryAndNameFailClosed(t *testing.T) {
	statements := traceDBIRQFixtureSchema()
	statements = append(statements,
		"INSERT INTO irq VALUES (1, 100, 10, 1, NULL, 'uart', 100, '')",
		"INSERT INTO irq VALUES (2, 110, 10, 1, '', 'uart', 100, '')",
		"INSERT INTO irq VALUES (3, 120, 10, 1, 'unknown', 'uart', 100, '')",
		"INSERT INTO irq VALUES (4, 130, 10, 1, 'IRQ', 'uart', 100, '')",
		"INSERT INTO irq VALUES (5, 140, 10, 1, 'irq', NULL, 100, '')",
		"INSERT INTO irq VALUES (6, 150, 10, 1, 'irq', '', 100, '')",
		"INSERT INTO irq VALUES (7, 160, 10, 1, 'irq', '   ', 100, '')",
		"INSERT INTO irq VALUES (8, 170, 10, 1, 'irq', 'uart' || char(10) || 'bad', 100, '')",
	)
	body, coverage, _ := exportTraceDBIRQFixture(t, statements)
	if coverage.RowsRead != 8 || coverage.RowsEmitted != 0 ||
		!strings.Contains(coverage.Skipped, "invalid_category=4") ||
		!strings.Contains(coverage.Skipped, "invalid_name=4") || traceDBIRQEndpointCount(body) != 0 {
		t.Fatalf("category/name fallback minted IRQ rows: %+v\n%s", coverage, body)
	}
}

func TestTraceDBIRQHardArgsFailClosed(t *testing.T) {
	statements := traceDBIRQFixtureSchema()
	statements = append(statements,
		"INSERT INTO args VALUES (1, 2, 1, 10, 110)",
		"INSERT INTO args VALUES (2, 1, 0, 1, 111)",
		"INSERT INTO args VALUES (3, 1, 1, 10, 112)",
		"INSERT INTO args VALUES (4, 2, 1, 10, 112)",
		"INSERT INTO args VALUES (5, 1, 0, 1, 113)",
		"INSERT INTO args VALUES (6, 2, 0, 1, 113)",
		"INSERT INTO args VALUES (7, 1, 0, 1, 114)",
		"INSERT INTO args VALUES (8, 1, 0, 2, 114)",
		"INSERT INTO args VALUES (9, 2, 1, 10, 114)",
		"INSERT INTO args VALUES (10, 1, 0, 1, 115)",
		"INSERT INTO args VALUES (11, 2, 1, 10, 115)",
		"INSERT INTO args VALUES (12, 2, 1, 11, 115)",
		"INSERT INTO args VALUES (13, 4, 0, 1, 116)",
		"INSERT INTO args VALUES (14, 2, 1, 10, 116)",
		"INSERT INTO args VALUES (15, 1, 0, -1, 117)",
		"INSERT INTO args VALUES (16, 2, 1, 10, 117)",
		"INSERT INTO args VALUES (17, 1, 0, 4294967296, 118)",
		"INSERT INTO args VALUES (18, 2, 1, 10, 118)",
		"INSERT INTO args VALUES (19, 1, 0, 1, 119)",
		"INSERT INTO args VALUES (20, 2, 1, 12, 119)",
	)
	for id, argset := range []int{999, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119} {
		statements = append(statements, "INSERT INTO irq VALUES ("+strconv.Itoa(100+id)+", "+strconv.Itoa(1000+id)+", 10, 1, 'irq', 'uart', "+strconv.Itoa(argset)+", '')")
	}
	body, coverage, _ := exportTraceDBIRQFixture(t, statements)
	for _, want := range []string{
		"missing_required_arg=4",
		"duplicate_required_arg=2",
		"invalid_irq_arg=3",
		"invalid_irq_ret_arg=2",
	} {
		if !strings.Contains(coverage.Skipped, want) {
			t.Fatalf("hard IRQ args coverage missing %q: %+v", want, coverage)
		}
	}
	if coverage.RowsRead != 11 || coverage.RowsEmitted != 0 || traceDBIRQEndpointCount(body) != 0 {
		t.Fatalf("invalid hard IRQ args minted endpoints: %+v\n%s", coverage, body)
	}
}

func TestTraceDBIRQSoftCanonicalTupleFailClosed(t *testing.T) {
	statements := traceDBIRQFixtureSchema()
	statements = append(statements,
		"INSERT INTO args VALUES (1, 3, 0, 10, 210)",
		"INSERT INTO args VALUES (2, 2, 1, 29, 210)",
		"INSERT INTO args VALUES (3, 3, 1, 25, 211)",
		"INSERT INTO args VALUES (4, 2, 1, 25, 211)",
		"INSERT INTO args VALUES (5, 3, 0, 5, 212)",
		"INSERT INTO args VALUES (6, 2, 0, 5, 212)",
		"INSERT INTO args VALUES (7, 3, 0, 5, 213)",
		"INSERT INTO args VALUES (8, 2, 1, 25, 213)",
		"INSERT INTO args VALUES (9, 3, 0, 5, 214)",
		"INSERT INTO args VALUES (10, 2, 1, 24, 214)",
		"INSERT INTO irq VALUES (1, 100, 10, 1, 'softirq', 'RCU', 210, '')",
		"INSERT INTO irq VALUES (2, 120, 10, 1, 'softirq', 'BLOCK_IOPOLL', 211, '')",
		"INSERT INTO irq VALUES (3, 140, 10, 1, 'softirq', 'BLOCK_IOPOLL', 212, '')",
		"INSERT INTO irq VALUES (4, 160, 10, 1, 'softirq', 'BLOCK', 213, '')",
		"INSERT INTO irq VALUES (5, 180, 10, 1, 'softirq', 'BLOCK_IOPOLL', 214, '')",
	)
	body, coverage, _ := exportTraceDBIRQFixture(t, statements)
	for _, want := range []string{"invalid_softirq_vec_arg=2", "invalid_softirq_ret_arg=1", "softirq_canonical_mismatch=2"} {
		if !strings.Contains(coverage.Skipped, want) {
			t.Fatalf("softirq tuple coverage missing %q: %+v", want, coverage)
		}
	}
	if coverage.RowsRead != 5 || coverage.RowsEmitted != 0 || traceDBIRQEndpointCount(body) != 0 {
		t.Fatalf("invalid softirq tuple minted endpoints: %+v\n%s", coverage, body)
	}
}

func TestTraceDBIRQIPICompletionReasonAndStableOrder(t *testing.T) {
	statements := traceDBIRQFixtureSchema()
	statements = append(statements,
		"INSERT INTO irq VALUES (1, 1000, 0, 1, 'ipi', '', NULL, '1')",
		"INSERT INTO irq VALUES (2, 1100, 10, 1, 'ipi', 'orphan', NULL, '')",
		"INSERT INTO irq VALUES (3, 1200, 10, 1, 'ipi', 'null flag', NULL, NULL)",
		"INSERT INTO irq VALUES (4, 1300, 10, 1, 'ipi', 'zero flag', NULL, '0')",
		"INSERT INTO irq VALUES (5, 1400, 10, 1, 'ipi', 'integer flag', NULL, 1)",
		"INSERT INTO irq VALUES (6, 2000, 0, 2, 'ipi', '(Inherited args ignored)', 'not-an-argset', '1')",
		"INSERT INTO irq VALUES (10, 3000, 0, 3, 'ipi', 'First', NULL, '1')",
		"INSERT INTO irq VALUES (11, 3000, 0, 3, 'ipi', '(Second)', NULL, '1')",
	)
	body, coverage, index := exportTraceDBIRQFixture(t, statements)
	if coverage.RowsRead != 8 || coverage.RowsEmitted != 8 ||
		!strings.Contains(coverage.Skipped, "invalid_name=1") ||
		!strings.Contains(coverage.Skipped, "invalid_ipi_flag=3") {
		t.Fatalf("IPI completeness coverage mismatch: %+v\n%s", coverage, body)
	}
	for _, reason := range []string{"Inherited args ignored", "First", "Second"} {
		if strings.Count(body, "ipi_entry: ("+reason+")") != 1 || strings.Count(body, "ipi_exit: ("+reason+")") != 1 {
			t.Fatalf("IPI endpoints do not share projected reason %q:\n%s", reason, body)
		}
	}
	var sameTS []string
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "0.000003:") && strings.Contains(line, "ipi_") {
			sameTS = append(sameTS, line)
		}
	}
	if len(sameTS) != 4 || !strings.Contains(sameTS[0], "ipi_entry: (First)") ||
		!strings.Contains(sameTS[1], "ipi_exit: (First)") ||
		!strings.Contains(sameTS[2], "ipi_entry: (Second)") ||
		!strings.Contains(sameTS[3], "ipi_exit: (Second)") {
		t.Fatalf("equal-ts id order or endpoint atomicity drifted: %q", sameTS)
	}
	ipiEvents := 0
	for _, event := range index.Events {
		if event.Type == tracequery.EventIPI {
			ipiEvents++
		}
	}
	if ipiEvents != 8 {
		t.Fatalf("tracequery lost zero-duration IPI endpoints: %+v", index.Events)
	}
}

func TestTraceDBIRQMissingArgsDependencyKeepsDirectIPI(t *testing.T) {
	for _, test := range []struct {
		name       string
		dependency []string
	}{
		{name: "missing args", dependency: []string{"CREATE TABLE data_dict (id INT, data TEXT)"}},
		{name: "missing data_dict", dependency: []string{"CREATE TABLE args (id INT, key INT, datatype INT, value INT, argset INT)"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			statements := []string{
				"CREATE TABLE irq (id INT, ts INT, dur INT, callid INT, cat TEXT, name TEXT, argsetid INT, flag TEXT)",
			}
			statements = append(statements, test.dependency...)
			statements = append(statements,
				"INSERT INTO irq VALUES (1, 100, 10, 1, 'irq', 'uart', 100, '')",
				"INSERT INTO irq VALUES (2, 120, 10, 1, 'softirq', 'RCU', 200, '')",
				"INSERT INTO irq VALUES (3, 140, 10, 1, 'ipi', 'Direct IPI', NULL, '1')",
			)
			body, coverage, _ := exportTraceDBIRQFixture(t, statements)
			if coverage.RowsRead != 3 || coverage.RowsEmitted != 2 ||
				!strings.Contains(coverage.Skipped, "args_schema_unavailable=2") ||
				strings.Count(body, "ipi_entry: (Direct IPI)") != 1 || strings.Count(body, "ipi_exit: (Direct IPI)") != 1 {
				t.Fatalf("direct IPI should survive missing args dependency: %+v\n%s", coverage, body)
			}
		})
	}
}

func TestTraceDBIRQRawEndpointNamesRemainUnsupportedNoDuplicate(t *testing.T) {
	for _, name := range []string{"irq_handler_entry", "irq_handler_exit", "softirq_entry", "softirq_exit", "ipi_entry", "ipi_exit"} {
		if class := traceDBRawFtraceClass(name); class != "" {
			t.Fatalf("raw SQL exporter claimed IRQ endpoint %q as class %q; irq interval export would duplicate it", name, class)
		}
	}
	statements := traceDBIRQFixtureSchema()
	statements = append(statements,
		"INSERT INTO args VALUES (1, 1, 0, 32, 100)",
		"INSERT INTO args VALUES (2, 2, 1, 10, 100)",
		"INSERT INTO irq VALUES (1, 1000, 100, 1, 'irq', 'uart', 100, '')",
		"CREATE TABLE raw (id INT, ts INT, name TEXT, cpu INT, itid INT, argset INT)",
		"INSERT INTO raw VALUES (1, 1000, 'irq_handler_entry', 1, 0, 100)",
		"INSERT INTO raw VALUES (2, 1100, 'irq_handler_exit', 1, 0, 100)",
	)
	path := createTraceDBFixture(t, statements)
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	irqCoverage, err := exportTraceDBIRQ(context.Background(), tdb, sink)
	if err != nil {
		t.Fatal(err)
	}
	rawCoverage, err := exportTraceDBRawFtraceFamilies(context.Background(), tdb, sink,
		traceDBSchedulerAuthority{}, traceDBSchedulerRunningIndex{}, filepath.Join(t.TempDir(), "irq-raw.ftrace"))
	if err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(t.TempDir(), "irq-raw-dedup.systrace")
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := sink.prepareAndWriteForTest(context.Background(), out)
	closeErr := out.Close()
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if irqCoverage.RowsEmitted != 2 || traceDBIRQEndpointCount(string(bodyBytes)) != 2 {
		t.Fatalf("raw duplicate escaped SQL IRQ ownership boundary: irq=%+v raw=%+v\n%s", irqCoverage, rawCoverage, bodyBytes)
	}
	foundUnsupported := false
	for _, item := range rawCoverage {
		if item.Family == "raw_ftrace" && item.Table == "unsupported" && item.RowsRead == 2 {
			foundUnsupported = true
		}
	}
	if !foundUnsupported {
		t.Fatalf("raw IRQ endpoints were not accounted as unsupported inventory: %+v", rawCoverage)
	}
}

func traceDBIRQFixtureSchema() []string {
	return []string{
		"CREATE TABLE irq (id INT, ts INT, dur INT, callid INT, cat TEXT, name TEXT, argsetid INT, flag TEXT)",
		"CREATE TABLE data_dict (id INT, data TEXT)",
		"INSERT INTO data_dict VALUES (1, 'irq')",
		"INSERT INTO data_dict VALUES (2, 'irq_ret')",
		"INSERT INTO data_dict VALUES (3, 'vec')",
		"INSERT INTO data_dict VALUES (4, 'irq_id')",
		"INSERT INTO data_dict VALUES (10, 'handled')",
		"INSERT INTO data_dict VALUES (11, 'unhandled')",
		"INSERT INTO data_dict VALUES (12, 'bad-ret')",
		"INSERT INTO data_dict VALUES (20, 'HI')",
		"INSERT INTO data_dict VALUES (21, 'TIMER')",
		"INSERT INTO data_dict VALUES (22, 'NET_TX')",
		"INSERT INTO data_dict VALUES (23, 'NET_RX')",
		"INSERT INTO data_dict VALUES (24, 'BLOCK')",
		"INSERT INTO data_dict VALUES (25, 'BLOCK_IOPOLL')",
		"INSERT INTO data_dict VALUES (26, 'TASKLET')",
		"INSERT INTO data_dict VALUES (27, 'SCHED')",
		"INSERT INTO data_dict VALUES (28, 'HRTIMER')",
		"INSERT INTO data_dict VALUES (29, 'RCU')",
		"CREATE TABLE args (id INT, key INT, datatype INT, value INT, argset INT)",
	}
}

func exportTraceDBIRQFixture(t *testing.T, statements []string) (string, TraceDBCoverage, *tracequery.Index) {
	t.Helper()
	path := createTraceDBFixture(t, statements)
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	sink, err := newTraceDBRowSink(t.TempDir(), 8)
	if err != nil {
		t.Fatal(err)
	}
	coverage, err := exportTraceDBIRQ(context.Background(), tdb, sink)
	if err != nil {
		t.Fatalf("export IRQ fixture: %v", err)
	}
	outPath := filepath.Join(t.TempDir(), "irq.systrace")
	out, err := os.OpenFile(outPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, writeErr := sink.prepareAndWriteForTest(context.Background(), out)
	closeErr := out.Close()
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	if closeErr != nil {
		t.Fatal(closeErr)
	}
	bodyBytes, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	index, err := tracequery.BuildIndex(context.Background(), outPath)
	if err != nil {
		t.Fatalf("tracequery IRQ fixture: %v", err)
	}
	return string(bodyBytes), coverage, index
}

func traceDBIRQEndpointCount(body string) int {
	count := 0
	for _, name := range []string{"irq_handler_entry:", "irq_handler_exit:", "softirq_entry:", "softirq_exit:", "ipi_entry:", "ipi_exit:"} {
		count += strings.Count(body, name)
	}
	return count
}

func traceDBInterruptActivityHas(items []tracequery.InterruptActivity, cpu, vector int, activeMs float64) bool {
	for _, item := range items {
		if item.CPU == cpu && item.Vector == vector && math.Abs(item.ActiveMs-activeMs) < 0.0001 {
			return true
		}
	}
	return false
}

func traceDBInterruptActivityNameHas(items []tracequery.InterruptActivity, cpu int, name string, activeMs float64) bool {
	for _, item := range items {
		if item.CPU == cpu && item.Name == name && math.Abs(item.ActiveMs-activeMs) < 0.0001 {
			return true
		}
	}
	return false
}
