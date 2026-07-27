package hitraceconv

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxTraceDBInventoryTables           = 1024
	maxTraceDBInventorySurfacedNonempty = 128
	maxTraceDBInventoryTableNameBytes   = 512
)

// inspectTraceDBUnhandledTableInventory closes the silent whole-table loss
// gap. Existing family coverage is the classification authority: if a real
// SQLite table has no exact coverage table, Codrax has neither an exporter nor
// a resolver classification for it. Nonempty unclassified tables remain a
// soft diagnostic and never acquire source-admission authority.
func inspectTraceDBUnhandledTableInventory(ctx context.Context, tdb *traceDB,
	classifiedCoverage []TraceDBCoverage,
) ([]TraceDBCoverage, error) {
	summary := TraceDBCoverage{
		Family: "conversion_inventory",
		Table:  "__table_inventory__",
		Role:   "diagnostic_inventory",
		Found:  true,
		FieldSources: map[string]string{
			"table_roster":     "bounded sqlite_master type=table roster excluding sqlite_% internals; exact ASCII case-folded coverage Table plus typed SourceTables lineage classify handled tables",
			"nonempty_witness": "SELECT 1 FROM the safely quoted unclassified table LIMIT 1; RowsRead on per-table findings is a presence witness, not a total row count",
			"effect":           "advisory conversion-loss disclosure only; never changes exporter admission or blocks positive rows",
		},
	}
	classified := make(map[string]bool, len(classifiedCoverage))
	classify := func(table string) {
		if table == "" || strings.HasPrefix(table, "__") ||
			!validTraceDBInventoryTableName(table) {
			return
		}
		classified[sqliteASCIIIdentifierFold(table)] = true
	}
	for _, item := range classifiedCoverage {
		classify(item.Table)
		for _, sourceTable := range item.SourceTables {
			classify(sourceTable)
		}
	}

	rows, err := tdb.db.QueryContext(ctx, `
		SELECT CASE
			WHEN typeof(name)='text'
			 AND length(CAST(name AS BLOB)) BETWEEN 1 AND ?
			THEN name
		END
		FROM sqlite_master
		WHERE type='table' AND name NOT LIKE 'sqlite_%'
		ORDER BY rowid
		LIMIT ?`, maxTraceDBInventoryTableNameBytes, maxTraceDBInventoryTables+1)
	if err != nil {
		summary.Error = err.Error()
		return []TraceDBCoverage{summary}, err
	}
	defer rows.Close()
	var names []string
	invalidNames := 0
	tableCount := 0
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			summary.Error = err.Error()
			return []TraceDBCoverage{summary}, err
		}
		var raw any
		if err := rows.Scan(&raw); err != nil {
			summary.Error = err.Error()
			return []TraceDBCoverage{summary}, err
		}
		tableCount++
		if tableCount > maxTraceDBInventoryTables {
			continue
		}
		name, ok := raw.(string)
		if !ok || !validTraceDBInventoryTableName(name) {
			invalidNames++
			continue
		}
		names = append(names, name)
	}
	if err := rows.Err(); err != nil {
		summary.Error = err.Error()
		return []TraceDBCoverage{summary}, err
	}
	if tableCount > maxTraceDBInventoryTables {
		tableCount = maxTraceDBInventoryTables
		traceDBAddCoverageMetric(&summary, "inventory_truncated", 1)
	}
	sort.Strings(names)
	traceDBAddCoverageMetric(&summary, "tables_in_inventory", int64(tableCount))

	var findings []TraceDBCoverage
	var nonemptyNames []string
	unclassified := invalidNames
	empty := 0
	uninspectable := invalidNames
	classifiedCount := 0
	for _, name := range names {
		if classified[sqliteASCIIIdentifierFold(name)] {
			classifiedCount++
			continue
		}
		unclassified++
		var one int
		err := tdb.db.QueryRowContext(ctx, "SELECT 1 FROM "+quoteSQLiteIdent(name)+" LIMIT 1").Scan(&one)
		if err == sql.ErrNoRows {
			empty++
			continue
		}
		if err != nil {
			summary.Error = err.Error()
			return append([]TraceDBCoverage{summary}, findings...), err
		}
		nonemptyNames = append(nonemptyNames, name)
		if len(findings) >= maxTraceDBInventorySurfacedNonempty {
			continue
		}
		findings = append(findings, TraceDBCoverage{
			Family:   "conversion_inventory",
			Table:    name,
			Role:     "unsupported_input",
			Found:    true,
			RowsRead: 1,
			FieldSources: map[string]string{
				"classification": "absent from the exact table roster of every completed exporter/resolver coverage record",
				"row_witness":    "bounded nonempty presence witness only; table contents were not decoded",
			},
			Skipped: "nonempty trace_streamer table has no Codrax exporter/resolver classification; its rows were not converted",
		})
	}
	sort.Strings(nonemptyNames)
	traceDBAddCoverageMetric(&summary, "classified_tables", int64(classifiedCount))
	traceDBAddCoverageMetric(&summary, "unclassified_tables", int64(unclassified))
	traceDBAddCoverageMetric(&summary, "unclassified_empty_tables", int64(empty))
	traceDBAddCoverageMetric(&summary, "unclassified_nonempty_tables", int64(len(nonemptyNames)))
	traceDBAddCoverageMetric(&summary, "unclassified_uninspectable_tables", int64(uninspectable))
	if len(nonemptyNames) > 0 {
		roster := nonemptyNames
		if len(roster) > maxTraceDBInventorySurfacedNonempty {
			roster = roster[:maxTraceDBInventorySurfacedNonempty]
		}
		quoted := make([]string, 0, len(roster))
		for _, name := range roster {
			quoted = append(quoted, strconv.Quote(name))
		}
		summary.Skipped = fmt.Sprintf(
			"unclassified_nonempty_tables=%d roster=%s",
			len(nonemptyNames), strings.Join(quoted, ","))
		if omitted := len(nonemptyNames) - len(roster); omitted > 0 {
			summary.Skipped += fmt.Sprintf(" omitted=%d", omitted)
		}
	}
	if summary.Metrics["inventory_truncated"] > 0 {
		traceDBAppendCoverageSkipped(&summary,
			fmt.Sprintf("table inventory truncated at %d entries", maxTraceDBInventoryTables))
	}
	if uninspectable > 0 {
		traceDBAppendCoverageSkipped(&summary,
			fmt.Sprintf("unclassified_uninspectable_tables=%d", uninspectable))
	}
	return append([]TraceDBCoverage{summary}, findings...), nil
}

func validTraceDBInventoryTableName(name string) bool {
	if name == "" || len(name) > maxTraceDBInventoryTableNameBytes || !utf8.ValidString(name) {
		return false
	}
	for _, r := range name {
		if unicode.IsControl(r) || unicode.Is(unicode.Zl, r) || unicode.Is(unicode.Zp, r) {
			return false
		}
	}
	return true
}
