package hitraceconv

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	traceDBIRQArgTypeInt    int64 = 0
	traceDBIRQArgTypeString int64 = 1
)

var traceDBSoftIRQActionByVec = [...]string{
	"HI",
	"TIMER",
	"NET_TX",
	"NET_RX",
	"BLOCK",
	"BLOCK_IOPOLL",
	"TASKLET",
	"SCHED",
	"HRTIMER",
	"RCU",
}

type traceDBIRQArg struct {
	DataType int64
	Int      int64
	Text     string
	Valid    bool
}

type traceDBIRQArgset struct {
	Values     map[string]traceDBIRQArg
	Duplicates map[string]bool
}

type traceDBIRQInterval struct {
	TS        int64
	Dur       int64
	CPU       int64
	Category  string
	Name      string
	RawArgSet any
	RawFlag   any
}

func exportTraceDBIRQ(ctx context.Context, tdb *traceDB, sink *traceDBRowSink) (TraceDBCoverage, error) {
	coverage, err := tdb.inspectCoverage(ctx, "irq", "irq", []string{"ts", "dur", "callid", "cat", "name", "argsetid"})
	coverage.FieldSources = map[string]string{
		"category":         "irq.cat exact closed set irq|softirq|ipi",
		"cpu":              "irq.callid strict SQLite INTEGER in range 0..4095",
		"hard_irq.irq":     "args.irq strict INT uint32",
		"hard_irq.ret":     "args.irq_ret strict STRING handled|unhandled",
		"interval":         "irq.ts+irq.dur; strict nonnegative SQLite INTEGER with checked addition",
		"ipi.completion":   "irq.flag exact TEXT 1",
		"ipi.entry_reason": "irq.name with at most one outer parenthesis layer normalized",
		"ipi.exit_reason":  "irq.name interval projection; upstream IPI exit reason is not retained",
		"ordering":         "irq.ts then irq.id when present, otherwise SQLite rowid",
		"ownership":        "CPU-only pseudo header; no raw thread ownership projection",
		"softirq.action":   "canonical tuple irq.name==args.irq_ret==action(args.vec)",
		"softirq.vec":      "args.vec strict INT in range 0..9",
	}
	if err != nil || !coverage.Found || len(coverage.ColumnsMissing) > 0 {
		return coverage, err
	}

	argsets, argsReady, err := loadTraceDBIRQArgsets(ctx, tdb)
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	hasFlag, err := tdb.columnExists(ctx, "irq", "flag")
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	hasID, err := tdb.columnExists(ctx, "irq", "id")
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	flagExpr := "NULL"
	if hasFlag {
		flagExpr = quoteSQLiteIdent("flag")
	}
	orderIdentity := "rowid"
	if hasID {
		orderIdentity = quoteSQLiteIdent("id")
	}
	rows, err := tdb.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT ts, dur, callid, cat, name, argsetid, %s
		FROM irq
		ORDER BY ts, %s
	`, flagExpr, orderIdentity))
	if err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	defer rows.Close()
	coverage.RowsRead = 0
	skipped := map[string]int{}
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return coverage, err
		}
		coverage.RowsRead++
		interval, reason, err := scanTraceDBIRQInterval(rows)
		if err != nil {
			coverage.Error = err.Error()
			return coverage, err
		}
		if reason != "" {
			skipped[reason]++
			continue
		}
		entry, exit, reason := renderTraceDBIRQInterval(interval, argsets, argsReady)
		if reason != "" {
			skipped[reason]++
			continue
		}
		// Both endpoint payloads are fully validated before either endpoint is
		// handed to the sorter. Any sorter error aborts the whole conversion, so
		// no successful artifact can expose a half interval.
		if err := addTraceDBInstantRow(sink, interval.TS, entry.task, 0, 0, interval.CPU, entry.body); err != nil {
			return coverage, err
		}
		if err := addTraceDBInstantRow(sink, interval.TS+interval.Dur, exit.task, 0, 0, interval.CPU, exit.body); err != nil {
			return coverage, err
		}
		coverage.RowsEmitted += 2
	}
	if err := rows.Err(); err != nil {
		coverage.Error = err.Error()
		return coverage, err
	}
	coverage.Skipped = traceDBIRQSkipSummary(skipped)
	return coverage, nil
}

type traceDBIRQEndpoint struct {
	task string
	body string
}

func scanTraceDBIRQInterval(rows *sql.Rows) (traceDBIRQInterval, string, error) {
	var rawTS, rawDur, rawCPU, rawCategory, rawName, rawArgSet, rawFlag any
	if err := rows.Scan(&rawTS, &rawDur, &rawCPU, &rawCategory, &rawName, &rawArgSet, &rawFlag); err != nil {
		return traceDBIRQInterval{}, "", err
	}
	var interval traceDBIRQInterval
	ts, ok := traceDBStrictSQLiteInt(rawTS)
	if !ok || ts < 0 {
		return interval, "invalid_timestamp", nil
	}
	dur, ok := traceDBStrictSQLiteInt(rawDur)
	if !ok || dur < 0 {
		return interval, "invalid_duration", nil
	}
	if ts > math.MaxInt64-dur {
		return interval, "interval_end_overflow", nil
	}
	cpu, ok := traceDBStrictSQLiteInt(rawCPU)
	if !ok || !validTraceDBCPUIndex(cpu) {
		return interval, "invalid_cpu", nil
	}
	category, ok := rawCategory.(string)
	if !ok || (category != "irq" && category != "softirq" && category != "ipi") {
		return interval, "invalid_category", nil
	}
	name, ok := rawName.(string)
	if !ok || strings.TrimSpace(name) == "" || traceDBIRQHasControl(name) {
		return interval, "invalid_name", nil
	}
	interval = traceDBIRQInterval{
		TS: ts, Dur: dur, CPU: cpu, Category: category, Name: name,
		RawArgSet: rawArgSet, RawFlag: rawFlag,
	}
	return interval, "", nil
}

func renderTraceDBIRQInterval(interval traceDBIRQInterval, argsets map[int64]traceDBIRQArgset, argsReady bool) (traceDBIRQEndpoint, traceDBIRQEndpoint, string) {
	switch interval.Category {
	case "irq":
		args, reason := traceDBIRQRequiredArgset(interval.RawArgSet, argsets, argsReady, "irq", "irq_ret")
		if reason != "" {
			return traceDBIRQEndpoint{}, traceDBIRQEndpoint{}, reason
		}
		irqArg := args.Values["irq"]
		if !irqArg.Valid || irqArg.DataType != traceDBIRQArgTypeInt || irqArg.Int < 0 || uint64(irqArg.Int) > math.MaxUint32 {
			return traceDBIRQEndpoint{}, traceDBIRQEndpoint{}, "invalid_irq_arg"
		}
		retArg := args.Values["irq_ret"]
		if !retArg.Valid || retArg.DataType != traceDBIRQArgTypeString || (retArg.Text != "handled" && retArg.Text != "unhandled") {
			return traceDBIRQEndpoint{}, traceDBIRQEndpoint{}, "invalid_irq_ret_arg"
		}
		return traceDBIRQEndpoint{task: "<irq>", body: fmt.Sprintf("irq_handler_entry: irq=%d name=%s", irqArg.Int, interval.Name)},
			traceDBIRQEndpoint{task: "<irq>", body: fmt.Sprintf("irq_handler_exit: irq=%d ret=%s", irqArg.Int, retArg.Text)}, ""
	case "softirq":
		args, reason := traceDBIRQRequiredArgset(interval.RawArgSet, argsets, argsReady, "vec", "irq_ret")
		if reason != "" {
			return traceDBIRQEndpoint{}, traceDBIRQEndpoint{}, reason
		}
		vecArg := args.Values["vec"]
		if !vecArg.Valid || vecArg.DataType != traceDBIRQArgTypeInt || vecArg.Int < 0 || vecArg.Int >= int64(len(traceDBSoftIRQActionByVec)) {
			return traceDBIRQEndpoint{}, traceDBIRQEndpoint{}, "invalid_softirq_vec_arg"
		}
		retArg := args.Values["irq_ret"]
		if !retArg.Valid || retArg.DataType != traceDBIRQArgTypeString {
			return traceDBIRQEndpoint{}, traceDBIRQEndpoint{}, "invalid_softirq_ret_arg"
		}
		action := traceDBSoftIRQActionByVec[vecArg.Int]
		if interval.Name != action || retArg.Text != action {
			return traceDBIRQEndpoint{}, traceDBIRQEndpoint{}, "softirq_canonical_mismatch"
		}
		bodyEntry := fmt.Sprintf("softirq_entry: vec=%d [action=%s]", vecArg.Int, action)
		bodyExit := fmt.Sprintf("softirq_exit: vec=%d [action=%s]", vecArg.Int, action)
		return traceDBIRQEndpoint{task: "<softirq>", body: bodyEntry}, traceDBIRQEndpoint{task: "<softirq>", body: bodyExit}, ""
	case "ipi":
		flag, ok := interval.RawFlag.(string)
		if !ok || flag != "1" {
			return traceDBIRQEndpoint{}, traceDBIRQEndpoint{}, "invalid_ipi_flag"
		}
		reason, ok := normalizeTraceDBIPIReason(interval.Name)
		if !ok {
			return traceDBIRQEndpoint{}, traceDBIRQEndpoint{}, "invalid_ipi_reason"
		}
		body := "(" + reason + ")"
		return traceDBIRQEndpoint{task: "<ipi>", body: "ipi_entry: " + body},
			traceDBIRQEndpoint{task: "<ipi>", body: "ipi_exit: " + body}, ""
	default:
		return traceDBIRQEndpoint{}, traceDBIRQEndpoint{}, "invalid_category"
	}
}

func traceDBIRQRequiredArgset(rawArgSet any, argsets map[int64]traceDBIRQArgset, argsReady bool, required ...string) (traceDBIRQArgset, string) {
	if !argsReady {
		return traceDBIRQArgset{}, "args_schema_unavailable"
	}
	argSetID, ok := traceDBStrictSQLiteInt(rawArgSet)
	if !ok || argSetID < 0 {
		return traceDBIRQArgset{}, "invalid_argset_id"
	}
	args, found := argsets[argSetID]
	if !found {
		return traceDBIRQArgset{}, "missing_required_arg"
	}
	for _, key := range required {
		if args.Duplicates[key] {
			return traceDBIRQArgset{}, "duplicate_required_arg"
		}
		if _, ok := args.Values[key]; !ok {
			return traceDBIRQArgset{}, "missing_required_arg"
		}
	}
	return args, ""
}

func loadTraceDBIRQArgsets(ctx context.Context, tdb *traceDB) (map[int64]traceDBIRQArgset, bool, error) {
	out := map[int64]traceDBIRQArgset{}
	argsCoverage, err := tdb.inspectCoverage(ctx, "irq.args.schema", "args", []string{"argset", "key", "datatype", "value"})
	if err != nil {
		return out, false, err
	}
	dictCoverage, err := tdb.inspectCoverage(ctx, "irq.args.schema", "data_dict", []string{"id", "data"})
	if err != nil {
		return out, false, err
	}
	if !argsCoverage.Found || !dictCoverage.Found || len(argsCoverage.ColumnsMissing) > 0 || len(dictCoverage.ColumnsMissing) > 0 {
		return out, false, nil
	}
	hasID, err := tdb.columnExists(ctx, "args", "id")
	if err != nil {
		return out, false, err
	}
	orderIdentity := "a.rowid"
	if hasID {
		orderIdentity = "a." + quoteSQLiteIdent("id")
	}
	rows, err := tdb.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT a.argset, key_dict.data, a.datatype, a.value, value_dict.data
		FROM args a
		LEFT JOIN data_dict key_dict ON key_dict.id = a.key
		LEFT JOIN data_dict value_dict ON value_dict.id = a.value
		WHERE key_dict.data IN ('irq', 'irq_ret', 'vec')
		ORDER BY a.argset, %s
	`, orderIdentity))
	if err != nil {
		return out, false, err
	}
	defer rows.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return out, false, err
		}
		var rawArgSet, rawKey, rawDataType, rawValue, rawText any
		if err := rows.Scan(&rawArgSet, &rawKey, &rawDataType, &rawValue, &rawText); err != nil {
			return out, false, err
		}
		argSetID, argSetOK := traceDBStrictSQLiteInt(rawArgSet)
		key, keyOK := rawKey.(string)
		if !argSetOK || argSetID < 0 || !keyOK || (key != "irq" && key != "irq_ret" && key != "vec") {
			continue
		}
		set := out[argSetID]
		if set.Values == nil {
			set.Values = map[string]traceDBIRQArg{}
			set.Duplicates = map[string]bool{}
		}
		if _, exists := set.Values[key]; exists {
			set.Duplicates[key] = true
			out[argSetID] = set
			continue
		}
		arg := traceDBIRQArg{DataType: -1}
		if dataType, ok := traceDBStrictSQLiteInt(rawDataType); ok {
			arg.DataType = dataType
			switch dataType {
			case traceDBIRQArgTypeInt:
				if value, ok := traceDBStrictSQLiteInt(rawValue); ok {
					arg.Int = value
					arg.Valid = true
				}
			case traceDBIRQArgTypeString:
				valueID, valueIDOK := traceDBStrictSQLiteInt(rawValue)
				text, textOK := rawText.(string)
				if valueIDOK && valueID >= 0 && textOK {
					arg.Text = text
					arg.Valid = true
				}
			}
		}
		set.Values[key] = arg
		out[argSetID] = set
	}
	if err := rows.Err(); err != nil {
		return out, false, err
	}
	return out, true, nil
}

func normalizeTraceDBIPIReason(raw string) (string, bool) {
	reason := strings.TrimSpace(raw)
	if reason == "" || traceDBIRQHasControl(reason) {
		return "", false
	}
	if len(reason) >= 2 && reason[0] == '(' && reason[len(reason)-1] == ')' {
		reason = strings.TrimSpace(reason[1 : len(reason)-1])
	}
	if reason == "" || traceDBIRQHasControl(reason) {
		return "", false
	}
	return reason, true
}

func traceDBIRQHasControl(value string) bool {
	if !utf8.ValidString(value) {
		return true
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func traceDBIRQSkipSummary(skipped map[string]int) string {
	keys := make([]string, 0, len(skipped))
	total := 0
	for key, count := range skipped {
		if count <= 0 {
			continue
		}
		keys = append(keys, key)
		total += count
	}
	if total == 0 {
		return ""
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, skipped[key]))
	}
	return fmt.Sprintf("%d irq interval row(s) skipped: %s", total, strings.Join(parts, ","))
}
