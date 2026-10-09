package hitraceconv

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func exportMeasureFixture(t *testing.T, sql []string) []tracewire.MeasureInterval {
	t.Helper()
	path := createTraceDBFixture(t, sql)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "measure.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, out)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("source mutated", err)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var rows []tracewire.MeasureInterval
	for _, line := range strings.Split(string(body), "\n") {
		if r, ok := tracewire.ParseMeasureInterval(line); ok {
			rows = append(rows, r)
		}
	}
	c := requireTraceDBCoverage(t, result.Coverage, "measurement", "measure")
	if c.RowsEmitted != len(rows) {
		t.Fatal("coverage mismatch", c, len(rows))
	}
	return rows
}

func TestMeasureExportLosslessStorageAndStrictRegistry(t *testing.T) {
	rows := exportMeasureFixture(t, []string{
		"CREATE TABLE measure(type,ts,dur,value,filter_id)",
		"CREATE TABLE measure_filter(id,name,type,source_arg_set_id)",
		"INSERT INTO measure_filter VALUES (-1,'gpufreq',7,9007199254740993),(2,'same',2.5,NULL),(2,'conflict','x',1),(3,'good','x',8),('3','poison','x',9)",
		"INSERT INTO measure VALUES (7,-5,15,9007199254740993,-1),(2.5,0,0,2.5,-1),(X'00ff',1,NULL,'42',2),(NULL,2,-1,X'00ff',3),(CAST(X'80FF0031' AS TEXT),3,1,CAST(X'80FF0031' AS TEXT),999),(NULL,NULL,NULL,NULL,NULL)",
	})
	if len(rows) != 6 {
		t.Fatal(rows)
	}
	by := map[int64]tracewire.MeasureInterval{}
	for _, r := range rows {
		by[r.RowID] = r
	}
	if r := by[1]; r.StartNS.Value != "-5" || r.Value.Value != "9007199254740993" || r.FilterStatus != "observed_unique" || r.Filter.ID.Value != "-1" || r.Filter.SourceArgSetID.Value != "9007199254740993" || r.MeasureType.StorageClass != "integer" {
		t.Fatal(r)
	}
	if by[2].MeasureType.StorageClass != "real" || by[3].MeasureType.StorageClass != "blob" || by[3].FilterStatus != "ambiguous" || by[4].FilterStatus != "ambiguous" {
		t.Fatal(by)
	}
	if r := by[5]; r.Value.Encoding != "base64" || r.Value.StorageClass != "text" || r.Value.Value != "gP8AMQ" || r.MeasureType != r.Value {
		t.Fatal("invalid UTF8 lost", r)
	}
	if by[6].StartNS.StorageClass != "null" {
		t.Fatal(by[6])
	}
}

func TestMeasureExportMissingOptionalAndUnknownFilter(t *testing.T) {
	rows := exportMeasureFixture(t, []string{"CREATE TABLE measure(ts,value,filter_id)", "INSERT INTO measure VALUES(1,7,4)"})
	if len(rows) != 1 || rows[0].DurationNS.StorageClass != "absent" || rows[0].MeasureType.StorageClass != "absent" || rows[0].FilterStatus != "unknown" {
		t.Fatal(rows)
	}
}

func TestMeasureFilterRetainsOnlyReferencedMetadata(t *testing.T) {
	path := createTraceDBFixture(t, []string{"CREATE TABLE measure(ts,value,filter_id)", "INSERT INTO measure VALUES(1,7,-4)", "CREATE TABLE measure_filter(id,name,type,source_arg_set_id)", "INSERT INTO measure_filter VALUES(-4,'wanted','raw',9),(80,zeroblob(1000000),'unreferenced',10),(-4,'duplicate','raw',11)"})
	tdb, err := openTraceDB(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer tdb.close()
	ids, err := referencedMeasureFilters(context.Background(), tdb)
	if err != nil {
		t.Fatal(err)
	}
	filters, bad, err := loadMeasureFilters(context.Background(), tdb, ids)
	if err != nil || len(ids) != 1 || len(filters) != 0 || !bad[-4] || len(bad) != 1 {
		t.Fatal(ids, filters, bad, err)
	}
}
