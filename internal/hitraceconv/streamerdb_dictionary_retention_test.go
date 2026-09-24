package hitraceconv

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestTraceDBDictionaryRetentionFollowsConsumerReferences(t *testing.T) {
	for _, count := range []int{0, 5000} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			tdb := sharedDictionaryFixture(t,
				"CREATE TABLE data_dict (id, data)",
				"INSERT INTO data_dict VALUES (0, 'ZERO'), (1, 'name'), (2, 'value')",
				fmt.Sprintf("WITH RECURSIVE n(x) AS (SELECT 10 UNION ALL SELECT x+1 FROM n WHERE x<%d) INSERT INTO data_dict SELECT x, printf('unused-%%d',x) FROM n WHERE x<%d", count+10, count+10),
				"INSERT INTO data_dict VALUES ('bad-id', 'unused'), (900001, NULL), (900002, 'same'), (900002, 'same')",
				"CREATE TABLE app_startup (start_name)", "INSERT INTO app_startup VALUES (0), ('1'), (1.0), (NULL), (X'32')",
				"CREATE TABLE args (argset, key, datatype, value)", "INSERT INTO args VALUES (0, 1, 1, 2)")
			dict, coverage, err := tdb.loadDataDict(context.Background())
			if err != nil || len(dict) != 1 || dict[0] != "ZERO" {
				t.Fatalf("unreferenced or non-INTEGER consumer names retained: unused=%d retained=%d zero=%q error=%v", count, len(dict), dict[0], err)
			}
			if coverage.RowsEmitted != count+3 || coverage.Metrics["dictionary_references"] != 1 || coverage.Metrics["dictionary_entries_retained"] != 1 || !strings.Contains(coverage.Skipped, "invalid_id=1") || !strings.Contains(coverage.Skipped, "invalid_value=1") || !strings.Contains(coverage.Skipped, "duplicate_id=1") {
				t.Fatalf("bounded retention changed global diagnostics: %+v", coverage)
			}
			args, cov, err := tdb.loadArgsets(context.Background())
			if err != nil || args.Sets[0]["name"].Text != "value" || cov[1].RowsEmitted != count+3 || cov[1].Metrics["dictionary_entries_retained"] != 2 || cov[1].Metrics["dictionary_references"] != 2 {
				t.Fatalf("args key/value references did not retain exactly their names: args=%+v coverage=%+v error=%v", args, cov, err)
			}
		})
	}
}

func TestTraceDBDictionaryRetentionNeverTruncatesReferencedPopulation(t *testing.T) {
	for _, schema := range []string{"CREATE TABLE data_dict (id, data)", "CREATE TABLE data_dict (id INTEGER PRIMARY KEY, data TEXT) WITHOUT ROWID"} {
		t.Run(schema, func(t *testing.T) {
			tdb := sharedDictionaryFixture(t, schema,
				"WITH RECURSIVE n(x) AS (SELECT 0 UNION ALL SELECT x+1 FROM n WHERE x<4097) INSERT INTO data_dict SELECT x, printf('NAME_%d',x) FROM n ORDER BY x DESC",
				"CREATE TABLE app_startup (start_name)", "INSERT INTO app_startup SELECT id FROM data_dict",
				"CREATE TABLE args (argset, key, datatype, value)", "INSERT INTO args SELECT id, id, 0, id FROM data_dict")
			dict, coverage, err := tdb.loadDataDict(t.Context())
			if err != nil || len(dict) != 4098 || dict[4097] != "NAME_4097" || coverage.Metrics["dictionary_entries_retained"] != 4098 {
				t.Fatalf("referenced dictionary silently truncated: entries=%d coverage=%+v err=%v", len(dict), coverage, err)
			}
			args, cov, err := tdb.loadArgsets(t.Context())
			// Keys are intentionally noncanonical. Resolving them still matters:
			// poison must localize to their canonical key, not the whole argset.
			if err != nil || cov[1].Metrics["dictionary_entries_retained"] != 4098 || !args.InvalidKeys[4097]["name_4097"] || args.Invalid[4097] {
				t.Fatalf("late referenced arg key did not resolve: coverage=%+v err=%v", cov, err)
			}
		})
	}
}

func TestTraceDBDictionaryRetentionAuditsZeroReferencesAndGlobalDuplicates(t *testing.T) {
	for _, referenced := range []bool{false, true} {
		for _, values := range []struct {
			sql      string
			rejected int
		}{
			{"(7,NULL),(7,'good')", 2}, {"(7,'good'),(7,X'78')", 1}, {"(7,'good'),(7,'good')", 1},
			{"(7,'good'),(7,'good'),(7,'third')", 2},
		} {
			t.Run(fmt.Sprintf("referenced=%t/%s", referenced, values.sql), func(t *testing.T) {
				statements := []string{"CREATE TABLE data_dict (id, data)", "INSERT INTO data_dict VALUES " + values.sql,
					"INSERT INTO data_dict VALUES (9,'valid')", "CREATE TABLE app_startup (start_name)", "CREATE TABLE args (argset,key,datatype,value)"}
				if referenced {
					statements = append(statements, "INSERT INTO app_startup VALUES (7)", "INSERT INTO args VALUES (0,7,0,42)")
				}
				tdb := sharedDictionaryFixture(t, statements...)
				dict, coverage, err := tdb.loadDataDict(t.Context())
				if err != nil || len(dict) != 0 || coverage.RowsEmitted != 1 || !strings.Contains(coverage.Skipped, "duplicate_id=") || coverage.Metrics["dictionary_entries_retained"] != 0 {
					t.Fatalf("global dictionary duplicates hidden/rescued: dict=%v coverage=%+v err=%v", dict, coverage, err)
				}
				args, cov, err := tdb.loadArgsets(t.Context())
				if err != nil || cov[1].RowsEmitted != 1 || cov[1].Skipped != fmt.Sprintf("%d data_dict row(s) rejected: invalid or duplicate typed identity", values.rejected) || cov[1].Metrics["dictionary_entries_retained"] != 0 || referenced && !args.Invalid[0] {
					t.Fatalf("argument global audit or referenced poison changed: args=%+v coverage=%+v err=%v", args, cov, err)
				}
			})
		}
	}
}
