package hitraceconv

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestTraceDBSharedDictionaryInvalidKeysAreLocal(t *testing.T) {
	for _, raw := range []string{"NULL", "'not-an-id'", "'7'", "7.0", "7.5", "X'37'"} {
		t.Run(raw, func(t *testing.T) {
			tdb := sharedDictionaryFixture(t, "CREATE TABLE data_dict (id, data)",
				"INSERT INTO data_dict VALUES (7, 'seven'), (9, 'nine')",
				"INSERT INTO data_dict VALUES ("+raw+", 'unrelated')")
			got, coverage, err := tdb.loadReferencedDataDict(context.Background(), sharedDictionaryTestReferences())
			if err != nil {
				t.Fatalf("one malformed dictionary key stopped unrelated identities: %v", err)
			}
			if want := map[int64]string{7: "seven", 9: "nine"}; !reflect.DeepEqual(got, want) {
				t.Errorf("non-INTEGER identity was coerced or damaged another key: got=%v want=%v", got, want)
			}
			if coverage.Skipped == "" || coverage.Error != "" {
				t.Errorf("row rejection must be diagnosed without a database failure: %+v", coverage)
			}
		})
	}
}

func TestTraceDBSharedDictionaryNonTextValuesAreNotNames(t *testing.T) {
	for _, raw := range []string{"NULL", "X'616263'", "123", "1.5"} {
		t.Run(raw, func(t *testing.T) {
			tdb := sharedDictionaryFixture(t, "CREATE TABLE data_dict (id, data)",
				"INSERT INTO data_dict VALUES (7, "+raw+"), (9, 'nine')")
			got, coverage, err := tdb.loadReferencedDataDict(context.Background(), sharedDictionaryTestReferences())
			if err != nil {
				t.Fatal(err)
			}
			if want := map[int64]string{9: "nine"}; !reflect.DeepEqual(got, want) {
				t.Errorf("non-TEXT storage class fabricated a name: got=%v want=%v", got, want)
			}
			if coverage.Skipped == "" {
				t.Error("rejected dictionary value was not diagnosed")
			}
		})
	}
}

func TestTraceDBSharedDictionaryDuplicateIdentityCannotBeRescued(t *testing.T) {
	for _, values := range [][]string{
		{"'alpha'", "'beta'"}, {"'same'", "'same'"}, {"''", "'name'"},
		{"NULL", "'name'"}, {"X'616263'", "'name'"}, {"123", "'name'"},
		{"'alpha'", "'beta'", "'alpha'"},
	} {
		var forwardSkipped string
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", strings.Join(values, ","), reverse), func(t *testing.T) {
				statements := []string{"CREATE TABLE data_dict (id, data)", "INSERT INTO data_dict VALUES (9, 'nine')"}
				for i := range values {
					index := i
					if reverse {
						index = len(values) - i - 1
					}
					statements = append(statements, "INSERT INTO data_dict VALUES (7, "+values[index]+")")
				}
				tdb := sharedDictionaryFixture(t, statements...)
				got, coverage, err := tdb.loadReferencedDataDict(context.Background(), sharedDictionaryTestReferences())
				if err != nil {
					t.Fatal(err)
				}
				if want := map[int64]string{9: "nine"}; !reflect.DeepEqual(got, want) {
					t.Errorf("duplicate identity selected/rescued by row order: got=%v want=%v", got, want)
				}
				if coverage.Skipped == "" {
					t.Error("ambiguous dictionary identity was not diagnosed")
				}
				if reverse && coverage.Skipped != forwardSkipped {
					t.Errorf("row order changed rejection accounting: forward=%q reverse=%q", forwardSkipped, coverage.Skipped)
				}
				forwardSkipped = coverage.Skipped
			})
		}
	}
}

func TestTraceDBSharedDictionaryRetainsIntegerAndTextCompatibility(t *testing.T) {
	// This shared loader is not the native_hook uint32 subtype resolver. It must
	// not introduce that upper bound or an argument-name length/control policy.
	want := map[int64]string{
		math.MinInt64: "negative carrier", -1: "minus one", 0: "",
		1:       "中文|quoted\"\n\t" + strings.Repeat("x", 5000),
		1 << 32: "above uint32", math.MaxInt64: "maximum SQLite INTEGER",
	}
	for _, schema := range []string{"CREATE TABLE data_dict (id, data)", "CREATE TABLE data_dict (id INTEGER PRIMARY KEY, data TEXT) WITHOUT ROWID"} {
		t.Run(schema, func(t *testing.T) {
			statements := []string{schema}
			for key, value := range want {
				statements = append(statements, fmt.Sprintf("INSERT INTO data_dict VALUES (%d, '%s')", key, strings.ReplaceAll(value, "'", "''")))
			}
			tdb := sharedDictionaryFixture(t, statements...)
			got, coverage, err := tdb.loadReferencedDataDict(context.Background(), sharedDictionaryTestReferences())
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("legal INTEGER/TEXT payload changed: got=%v err=%v", got, err)
			}
			if coverage.Skipped != "" || coverage.Error != "" {
				t.Errorf("valid dictionary was reported rejected: %+v", coverage)
			}
		})
	}
	t.Run("affinity_is_stored_class_not_SQL_literal", func(t *testing.T) {
		tdb := sharedDictionaryFixture(t, "CREATE TABLE data_dict (id INTEGER, data TEXT)",
			"INSERT INTO data_dict VALUES ('7', 123)")
		got, _, err := tdb.loadReferencedDataDict(context.Background(), sharedDictionaryTestReferences())
		if err != nil || !reflect.DeepEqual(got, map[int64]string{7: "123"}) {
			t.Fatalf("SQLite's actual INTEGER/TEXT storage was rejected: got=%v err=%v", got, err)
		}
	})
}

func TestTraceDBSharedDictionaryAbsentAndDatabaseFailureBoundaries(t *testing.T) {
	for _, schema := range []string{"CREATE TABLE unrelated (id)", "CREATE TABLE data_dict (id, data)", "CREATE TABLE data_dict (id)"} {
		t.Run(schema, func(t *testing.T) {
			tdb := sharedDictionaryFixture(t, schema)
			got, _, err := tdb.loadReferencedDataDict(context.Background(), sharedDictionaryTestReferences())
			if err != nil || len(got) != 0 {
				t.Fatalf("optional dictionary absence changed: got=%v err=%v", got, err)
			}
		})
	}
	t.Run("canceled_context", func(t *testing.T) {
		tdb := sharedDictionaryFixture(t, "CREATE TABLE data_dict (id, data)")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, _, err := tdb.loadReferencedDataDict(ctx, sharedDictionaryTestReferences()); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation was swallowed: %v", err)
		}
	})
	t.Run("closed_database_with_cached_metadata", func(t *testing.T) {
		tdb := sharedDictionaryFixture(t, "CREATE TABLE data_dict (id, data)")
		if _, _, err := tdb.loadReferencedDataDict(context.Background(), sharedDictionaryTestReferences()); err != nil {
			t.Fatal(err)
		}
		if err := tdb.db.Close(); err != nil {
			t.Fatal(err)
		}
		if _, coverage, err := tdb.loadReferencedDataDict(context.Background(), sharedDictionaryTestReferences()); err == nil || coverage.Error == "" {
			t.Fatalf("database failure was mistaken for a row rejection: err=%v coverage=%+v", err, coverage)
		}
	})
}

// These legacy admission tests request all their original test identities;
// reference discovery/retention is exercised separately through the consumers.
func sharedDictionaryTestReferences() map[int64]bool {
	return map[int64]bool{math.MinInt64: true, -1: true, 0: true, 1: true, 7: true, 9: true, 1 << 32: true, math.MaxInt64: true}
}

func sharedDictionaryFixture(t *testing.T, statements ...string) *traceDB {
	t.Helper()
	tdb, err := openTraceDB(context.Background(), createTraceDBFixture(t, statements))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tdb.close() })
	return tdb
}

func TestTraceDBSharedDictionaryDriverFailuresRemainFatal(t *testing.T) {
	// Fault injection tests error propagation, not SQLite type admission. The
	// storage-class cases above use a real SQLite DB through openTraceDB.
	for _, mode := range []string{"query", "scan", "iteration", "close"} {
		t.Run(mode, func(t *testing.T) {
			db := sql.OpenDB(sharedDictionaryFaultConnector{mode: mode})
			t.Cleanup(func() { _ = db.Close() })
			tdb := &traceDB{db: db,
				tableExistsKnown: map[string]bool{"data_dict": true}, tableExistsCache: map[string]bool{"data_dict": true},
				columnNamesCache: map[string][]string{"data_dict": {"id", "data"}}, rowCountCache: map[string]int{"data_dict": 1},
			}
			if _, _, err := tdb.loadReferencedDataDict(context.Background(), sharedDictionaryTestReferences()); err == nil {
				t.Fatalf("%s error was swallowed as an invalid dictionary row", mode)
			}
		})
	}
}

type sharedDictionaryFaultConnector struct{ mode string }

func (c sharedDictionaryFaultConnector) Connect(context.Context) (driver.Conn, error) {
	return &sharedDictionaryFaultConn{mode: c.mode}, nil
}
func (c sharedDictionaryFaultConnector) Driver() driver.Driver { return sharedDictionaryFaultDriver{} }

type sharedDictionaryFaultDriver struct{}

func (sharedDictionaryFaultDriver) Open(string) (driver.Conn, error) {
	return nil, errors.New("connector must be used")
}

type sharedDictionaryFaultConn struct{ mode string }

func (*sharedDictionaryFaultConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unexpected prepare")
}
func (*sharedDictionaryFaultConn) Close() error { return nil }
func (*sharedDictionaryFaultConn) Begin() (driver.Tx, error) {
	return nil, errors.New("unexpected transaction")
}
func (c *sharedDictionaryFaultConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	// Reach the actual payload scan/iteration/close fault after the new narrow
	// population query. Do not accidentally pass every mode on a count cast.
	if strings.Contains(query, "SUM(n-1)") {
		return &sharedDictionaryFaultRows{population: true}, nil
	}
	if c.mode == "query" {
		return nil, errors.New("injected query failure")
	}
	return &sharedDictionaryFaultRows{mode: c.mode}, nil
}

type sharedDictionaryFaultRows struct {
	mode       string
	read       bool
	population bool
}

func (r *sharedDictionaryFaultRows) Columns() []string {
	if r.mode == "scan" {
		return []string{"id", "data", "unexpected"}
	}
	return []string{"id", "data"}
}
func (r *sharedDictionaryFaultRows) Close() error {
	if r.mode == "close" {
		return errors.New("injected close failure")
	}
	return nil
}
func (r *sharedDictionaryFaultRows) Next(values []driver.Value) error {
	if r.read {
		if r.mode == "iteration" {
			return errors.New("injected iteration failure")
		}
		return io.EOF
	}
	r.read = true
	if r.population {
		values[0], values[1] = int64(1), int64(0)
		return nil
	}
	values[0], values[1] = int64(1), "one"
	return nil
}
