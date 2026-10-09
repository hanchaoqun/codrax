package tool

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func measurementIndependentSQLiteFixture(t *testing.T, schema string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "capture.data")
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE trace_range(start_ts,end_ts); INSERT INTO trace_range VALUES (0,3000000000);` + schema); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

func measurementIndependentSQLiteQuery(t *testing.T, path string, limit int) *tracequery.MeasurementsResult {
	t.Helper()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, _, _ := hmc17NamedPathContext(t)
	r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": path, "view": "measurements", "time_start": 1, "time_end": 2, "limit": limit})
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("native read changed source database")
	}
	p := hmc17NamedPayload(t, r)
	if p.Measurements == nil {
		t.Fatalf("no typed measurements: %s", r.Summary)
	}
	return p.Measurements
}

func TestMeasurementsSQLitePublicIndependentStorageIdentity(t *testing.T) {
	p := measurementIndependentSQLiteFixture(t, `
CREATE TABLE measure_filter(id,name,type,source_arg_set_id);
INSERT INTO measure_filter VALUES (1,'gpufreq','measure_filter',7),(2,'gpufreq','measure_filter',7),(-3,'negative-ref','vendor',-9),(4,'integer-ref','vendor',1),('4','text-decoy','vendor',1);
CREATE TABLE cpu_measure_filter(id,name,cpu); INSERT INTO cpu_measure_filter VALUES (1,'cpu_frequency',5);
CREATE TABLE measure(rowid TEXT,ts,dur,value,filter_id,type);
INSERT INTO measure VALUES
 ('fake',900000000,300000000,334200000.5,1,'measure'),
 ('fake',1200000000,100000000,9007199254740993,2,'measure'),
 ('fake',1300000000,NULL,'1',-3,'vendor'),
 ('fake',1400000000,0,X'0031',4,'vendor'),
 ('fake',1500000000,-1,NULL,404,'vendor'),
 ('fake',1600000000,9223372036854775807,1.25,1,'measure'),
 ('fake',1700000000,10.0,0,1,'measure'),
 ('fake',1800000000,10,7,'1','measure'),
 ('fake',1900000000,10,8,1.0,'measure'),
 ('fake',2000000000,10,99,1,'measure'),
 ('fake',1100000000.0,10,100,1,'measure'),
 ('fake','1100000000',10,101,1,'measure'),
 ('fake',NULL,10,102,1,'measure'),
 ('fake',-100000000,1200000000,9,1,'measure');
`)
	r := measurementIndependentSQLiteQuery(t, p, 64)
	if r.Status != "available" || r.TotalRows != 10 || len(r.Rows) != 10 || r.OmittedRows != 0 || r.UnpositionedRows != 3 {
		t.Fatalf("exact population lost: %+v", r)
	}
	by := map[int64]tracequery.MeasurementRow{}
	for _, row := range r.Rows {
		if row.Unit != "unknown" || row.SourcePath == "" || row.SourceLine <= 0 || row.Line <= 0 {
			t.Fatalf("invented protocol/lost coordinate: %+v", row)
		}
		if _, dup := by[row.Record.RowID]; dup {
			t.Fatalf("JOIN fanout/physical identity collision: %+v", row)
		}
		by[row.Record.RowID] = row
	}
	if _, ok := by[10]; ok {
		t.Fatal("included right-boundary record")
	}
	for _, id := range []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 14} {
		if _, ok := by[id]; !ok {
			t.Fatalf("lost physical row %d (must not use shadow rowid)", id)
		}
	}
	a, b := by[1], by[2]
	if a.Record.Filter == nil || b.Record.Filter == nil || a.Record.Filter.ID.Value != "1" || b.Record.Filter.ID.Value != "2" || a.Record.Filter.Name.Value != "gpufreq" || a.Record.Filter.SourceArgSetID.Value != "7" || b.Record.Filter.SourceArgSetID.Value != "7" {
		t.Fatalf("same name/reference merged sequences or borrowed CPU registry: %+v %+v", a, b)
	}
	if a.Record.Value.StorageClass != "real" || a.Record.Value.Value != "3.342000005e+08" || b.Record.Value.Value != "9007199254740993" {
		t.Fatalf("numeric raw precision lost: %+v %+v", a.Record.Value, b.Record.Value)
	}
	if a.ClippedStartNS == nil || *a.ClippedStartNS != 1000000000 || a.ClippedEndNS == nil || *a.ClippedEndNS != 1200000000 {
		t.Fatalf("carry-in lost: %+v", a)
	}
	neg := by[3]
	if neg.Record.Filter == nil || neg.Record.FilterStatus != "observed_unique" || neg.Record.Filter.ID.Value != "-3" || neg.Record.Filter.SourceArgSetID.Value != "-9" || neg.Record.Value.StorageClass != "text" || neg.Record.Value.Value != "1" {
		t.Fatalf("signed metadata/text lost: %+v", neg)
	}
	if by[4].Record.FilterStatus != "ambiguous" || by[4].Record.Filter != nil || by[4].Record.Value.StorageClass != "blob" || by[4].Record.Value.Value != base64.RawStdEncoding.EncodeToString([]byte{0, '1'}) {
		t.Fatalf("strict duplicate registry/bytes lost: %+v", by[4])
	}
	for _, id := range []int64{5, 8, 9} {
		if by[id].Record.FilterStatus != "unknown" || by[id].Record.Filter != nil {
			t.Fatalf("invalid filter type gained reference authority: %+v", by[id])
		}
	}
	for _, id := range []int64{3, 5, 6, 7} {
		if by[id].Selection != "unknown_duration" || by[id].ClippedEndNS != nil {
			t.Fatalf("duration invented: %+v", by[id])
		}
	}
	if by[14].Record.StartNS.Value != "-100000000" || by[14].ClippedStartNS == nil || *by[14].ClippedStartNS != 1000000000 || by[14].ClippedEndNS == nil || *by[14].ClippedEndNS != 1100000000 {
		t.Fatalf("signed source interval relabelled: %+v", by[14])
	}
}

func TestMeasurementsSQLitePublicIndependentLimitAndAbsentMetadata(t *testing.T) {
	var sqlText strings.Builder
	sqlText.WriteString(`CREATE TABLE measure(ts,value,filter_id);`)
	for i := 0; i < 70; i++ {
		fmt.Fprintf(&sqlText, "INSERT INTO measure VALUES (%d,%d,1);", 1000000000+i, i)
	}
	p := measurementIndependentSQLiteFixture(t, sqlText.String())
	r := measurementIndependentSQLiteQuery(t, p, 2)
	if r.Status != "available" || r.TotalRows != 70 || len(r.Rows) != 2 || r.OmittedRows != 68 || r.UnpositionedRows != 0 {
		t.Fatalf("limit changed population: %+v", r)
	}
	for _, row := range r.Rows {
		if row.Record.DurationNS.StorageClass != "absent" || row.Record.MeasureType.StorageClass != "absent" || row.Record.FilterStatus != "unknown" || row.ClippedEndNS != nil {
			t.Fatalf("absent metadata converted to known/null/end: %+v", row)
		}
	}
}

func TestMeasurementsSQLitePublicIndependentAllRowidAliasesShadowed(t *testing.T) {
	p := transactionSQLitePublicFixture(t, `CREATE TABLE measure(rowid,_rowid_,oid,ts,dur,value,filter_id); INSERT INTO measure VALUES(1,2,3,1000000000,10,7,1); CREATE TABLE measure_filter(id,name); INSERT INTO measure_filter VALUES(1,'vendor');`)
	before, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	ctx, _, _ := hmc17NamedPathContext(t)
	r := hmc17NamedQuery(t, ctx, map[string]any{"source": "path", "path": p, "view": "measurements", "time_start": 1, "time_end": 2})
	if r.Success || len(r.Observations) != 0 || r.TraceEvidenceAuthority != nil || r.RawRef != "" || r.TraceQuerySourceRead.Path() != "" || !strings.Contains(r.Summary, "trace_db_text_fidelity_rowid_alias_shadowed") {
		t.Fatalf("user columns impersonated physical identity: %+v", r)
	}
	after, err := os.ReadFile(p)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("rejected preparation mutated source")
	}
}

func TestMeasurementsSQLitePublicIndependentInvalidUTF8Text(t *testing.T) {
	p := measurementIndependentSQLiteFixture(t, `CREATE TABLE measure(ts,dur,value,filter_id); INSERT INTO measure VALUES(1000000000,10000000,CAST(X'80FF0031' AS TEXT),1); CREATE TABLE measure_filter(id,name,type,source_arg_set_id); INSERT INTO measure_filter VALUES(1,'vendor','measure_filter',1);`)
	r := measurementIndependentSQLiteQuery(t, p, 64)
	if r.Status != "available" || len(r.Rows) != 1 || r.Rows[0].Record.Value.StorageClass != "text" {
		t.Fatalf("legal SQL TEXT lost raw storage: %+v", r)
	}
	encoded, _ := json.Marshal(r.Rows[0].Record.Value)
	var scalar map[string]any
	if err := json.Unmarshal(encoded, &scalar); err != nil {
		t.Fatal(err)
	}
	if scalar["encoding"] != "base64" || scalar["value"] != base64.RawStdEncoding.EncodeToString([]byte{0x80, 0xff, 0, '1'}) {
		t.Fatalf("invalid UTF8 bytes silently changed: %s", encoded)
	}
}
