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

func exportProcessMeasureFixture(t *testing.T, extra []string) (string, []tracewire.ProcessMeasureInterval, traceDBSystraceExport) {
	t.Helper()
	sql := []string{
		"CREATE TABLE process (ipid, pid, name)",
		"INSERT INTO process VALUES (1,101,'same'),(2,202,'same')",
		"CREATE TABLE thread (itid, tid, ipid, name)",
		"CREATE TABLE process_measure_filter (id, name, ipid)",
		"INSERT INTO process_measure_filter VALUES (1,'RSS',1),(2,'RSS',2)",
		"CREATE TABLE process_measure (type,ts,dur,value,filter_id)",
	}
	path := createTraceDBFixture(t, append(sql, extra...))
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "process.systrace")
	result, err := exportTraceDBToSystrace(context.Background(), path, out)
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("source database mutated", err)
	}
	body, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	var records []tracewire.ProcessMeasureInterval
	for _, line := range strings.Split(string(body), "\n") {
		if r, ok := tracewire.ParseProcessMeasureInterval(line); ok {
			records = append(records, r)
		}
	}
	if strings.Contains(string(body), "C|101|RSS|") {
		t.Fatal("SQL process intervals still exported as synthetic ftrace point counters")
	}
	return out, records, result
}

func TestProcessMeasureExportPreservesRawIntervalsAndUnknown(t *testing.T) {
	_, records, result := exportProcessMeasureFixture(t, []string{
		"INSERT INTO process_measure VALUES ('memory',-5,15,9223372036854775807,1),('memory',0,0,0,1),('memory',10,NULL,NULL,2),('memory',20,5,'42',2),('memory',30,5,x'00ff',2),('memory',40,5,2.5,2)",
	})
	if len(records) != 6 {
		t.Fatalf("lossless process interval records: got %d, want 6", len(records))
	}
	if records[0].StartNS.Value != "-5" || records[0].Value.Value != "9223372036854775807" || records[0].DurationNS.Value != "15" {
		t.Fatalf("signed/exact integer source lost: %+v", records[0])
	}
	if records[1].Value.Status != "known" || records[1].Value.Value != "0" || records[2].Value.Status != "null" || records[2].DurationNS.Status != "null" {
		t.Fatal("null/zero distinction lost", records)
	}
	for _, i := range []int{3, 4, 5} {
		if records[i].Value.Status != "invalid_storage" {
			t.Fatalf("wrong storage assigned integer authority: %+v", records[i])
		}
	}
	if records[0].PID == nil || *records[0].PID != 101 || records[2].PID == nil || *records[2].PID != 202 {
		t.Fatalf("same-name process identities collapsed: %+v", records)
	}
	c := requireTraceDBCoverage(t, result.Coverage, "counter", "process_measure")
	if c.RowsRead != 6 || c.RowsEmitted != 6 {
		t.Fatalf("coverage dishonest: %+v", c)
	}
}

func TestProcessMeasureExportRejectsAmbiguousOwnersWithoutLosingRows(t *testing.T) {
	_, records, _ := exportProcessMeasureFixture(t, []string{
		"INSERT INTO process_measure_filter VALUES (1,'identical_or_not_duplicate',1),(3,'missing',999),(4,'invalid_owner','1'),(5,'coerced_filter',1),('5','coercion_conflict',2)",
		"INSERT INTO process_measure VALUES ('x',0,10,1,1),('x',0,10,2,3),('x',0,10,3,4),('x',0,10,4,5),('x',0,10,5,'2'),('x',0,10,6,NULL),('x',0,10,7,2)",
	})
	if len(records) != 7 {
		t.Fatal("JOIN fanout or unknown-owner loss", len(records))
	}
	for i, r := range records {
		if i == 6 {
			if r.PID == nil || *r.PID != 202 {
				t.Fatal("healthy owner lost", r)
			}
		} else if r.PID != nil || r.OwnerStatus == "known" {
			t.Fatalf("unknown/coercible owner granted: %d %+v", i, r)
		}
	}
	if records[0].OwnerStatus != "ambiguous" || records[3].OwnerStatus != "ambiguous" || records[2].IPID.StorageClass != "text" || records[4].FilterID.StorageClass != "text" {
		t.Fatal("owner defects hidden", records)
	}
}

func TestProcessMeasureExportRawStorageAndMissingDuration(t *testing.T) {
	_, records, _ := exportProcessMeasureFixture(t, []string{
		"INSERT INTO process_measure VALUES ('x',0,-1,'',1),('x',1,0,x'',1),('x',2,2,1e999,1),('x','3',3,3,1),('x',4,'4',4,1),('x',9223372036854775807,1,5,1)",
	})
	if len(records) != 6 {
		t.Fatal(records)
	}
	byID := map[int64]tracewire.ProcessMeasureInterval{}
	for _, record := range records {
		byID[record.RowID] = record
	}
	for i := range records {
		records[i] = byID[int64(i+1)]
	}
	if records[0].Value.StorageClass != "text" || records[0].Value.Value != "" || records[1].Value.StorageClass != "blob" || records[1].Value.Value != "" || records[2].Value.Value != "+Inf" {
		t.Fatal("raw storage loss", records)
	}
	for _, i := range []int{0, 3, 4, 5} {
		if _, ok := records[i].EndNS(); ok {
			t.Fatal("invalid interval upgraded", records[i])
		}
	}
	path := createTraceDBFixture(t, []string{"CREATE TABLE process_measure(ts,value,filter_id)", "INSERT INTO process_measure VALUES (0,NULL,1)"})
	out := filepath.Join(t.TempDir(), "missing.systrace")
	_, err := exportTraceDBToSystrace(context.Background(), path, out)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(out)
	found := false
	for _, line := range strings.Split(string(body), "\n") {
		if r, ok := tracewire.ParseProcessMeasureInterval(line); ok {
			found = true
			if r.DurationNS.Status != "unavailable" || r.Value.Status != "null" || r.OwnerStatus != "unknown" {
				t.Fatal("missing dependency dropped raw evidence", r)
			}
		}
	}
	if !found {
		t.Fatal("missing registry lost measurement")
	}
}
