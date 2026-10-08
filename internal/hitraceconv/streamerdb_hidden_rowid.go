package hitraceconv

import (
	"context"
	"database/sql"
)

// table_info omits generated and virtual-table hidden columns. Such declared
// columns still shadow SQLite's rowid aliases, even when their value happens
// to be INTEGER. The complete schema, not a sample scalar's shape, decides
// whether a candidate token can denote physical row identity.
func traceDBHiddenRowIDColumnNames(ctx context.Context, queryer traceDBQueryer, table string) (out []string, err error) {
	rows, err := queryer.QueryContext(ctx, `PRAGMA table_xinfo(`+quoteSQLiteIdent(table)+`)`)
	if err != nil {
		return nil, err
	}
	defer func() { err = traceDBJoinPreservingSingle(err, rows.Close()) }()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var cid, notNull, pk, hidden int
		var name, typ string
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk, &hidden); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}
