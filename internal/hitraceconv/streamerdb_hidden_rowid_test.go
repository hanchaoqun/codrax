package hitraceconv

import (
	"context"
	"testing"
)

func TestTraceDBHiddenRowIDGeneratedAliasesCannotGrantIdentity(t *testing.T) {
	for _, tc := range []struct{ name, schema, want string }{
		{"virtual", "CREATE TABLE source(value, rowid INTEGER GENERATED ALWAYS AS(7) VIRTUAL)", "_rowid_"},
		{"stored_uppercase", "CREATE TABLE source(value, ROWID INTEGER GENERATED ALWAYS AS(7) STORED)", "_rowid_"},
		{"two_aliases", "CREATE TABLE source(value, rowid INTEGER GENERATED ALWAYS AS(7) VIRTUAL, _ROWID_ INTEGER GENERATED ALWAYS AS(8) STORED)", "oid"},
		{"all_aliases", "CREATE TABLE source(value, rowid INTEGER GENERATED ALWAYS AS(7) VIRTUAL, _ROWID_ INTEGER GENERATED ALWAYS AS(8) STORED, OID INTEGER GENERATED ALWAYS AS(9) VIRTUAL)", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := createTraceDBFixture(t, []string{tc.schema, "INSERT INTO source(value) VALUES ('same'),('same')"})
			tdb, err := openTraceDB(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			defer tdb.close()
			expr, source, err := traceDBHiddenRowIDExpr(context.Background(), tdb.db, "source")
			if tc.want == "" {
				if err == nil || expr != "" || source != "" {
					t.Fatalf("generated integer aliases granted physical identity: %q %q %v", expr, source, err)
				}
				return
			}
			if err != nil || expr != tc.want || source != "source.hidden_"+tc.want {
				t.Fatalf("incomplete shadow discovery: %q %q %v", expr, source, err)
			}
		})
	}
}
