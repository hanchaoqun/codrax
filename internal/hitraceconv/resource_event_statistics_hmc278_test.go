package hitraceconv

import (
	"context"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestHMC278ResourceStatisticsSQLitePublicPath(t *testing.T) {
	out, idx := resourceSQLiteFixture(t,
		`DELETE FROM native_hook; DELETE FROM native_hook_frame; DELETE FROM data_dict;
		INSERT INTO data_dict VALUES (0,''),(8,'缓存|纹理'),(9,NULL),(10,'first'),(10,'second');
		INSERT INTO native_hook VALUES
		(1,10000000000,10090000000,'AllocEvent',999,1,1,0,NULL,0,0),
		(2,10010000000,NULL,'AllocEvent',999,1,1,NULL,NULL,0,0),
		(3,10020000000,NULL,'FreeEvent',999,1,1,20,NULL,0,8),
		(4,10030000000,NULL,'MmapEvent',999,1,1,30,NULL,0,9),
		(5,10040000000,NULL,'FD_Open_Event',999,1,1,1,NULL,0,10),
		(6,10050000000,NULL,'THREAD_Create_Event',999,1,1,1,NULL,0,99),
		(7,10060000000,NULL,'AllocEvent',999,2,2,-1,NULL,0,NULL),
		(8,10070000000,NULL,'AllocEvent',999,1,1,'bad',NULL,0,-1),
		(9,10080000000,NULL,'FreeEvent',999,1,1,99,NULL,0,8);
		CREATE TABLE native_hook_statistic (ipid,type,apply_count,release_count,apply_size,release_size);
		INSERT INTO native_hook_statistic VALUES (1,0,999999,888888,777777,666666);`)
	q := tracequery.Query{View: tracequery.ViewResourceStack, TimeStart: 10, TimeEnd: 10.08, Limit: 1}
	p := tracequery.Run(idx, q).ResourceStack
	if !tracequery.ValidResourceStack(*p) || p.MatchedEvents != 8 || p.EventStatistics.EventCount != 8 || len(p.EventStatistics.Groups) != 7 || p.OmittedEvents != 7 {
		t.Fatalf("bad source census %+v", p)
	}
	if p.Events[0].Source.Subtype == nil || p.Events[0].Source.Subtype.ID.Value != "0" || p.Events[0].Source.Subtype.Name.Status != "known" || p.Events[0].Source.Subtype.Name.Value != "" || p.Events[0].Source.Process != "Gallery" {
		t.Fatalf("zero/empty/owner lost %+v", p.Events[0])
	}
	groups := map[string]tracequery.ResourceEventStatistic{}
	for _, g := range p.EventStatistics.Groups {
		groups[g.Operation+":"+g.Subtype.ID.Status+":"+g.Subtype.ID.Value] = g
	}
	zero := groups["AllocEvent:known:0"]
	if zero.EventCount != 2 || zero.KnownSizeEvents != 1 || zero.NullSizeEvents != 1 || zero.KnownSizeTotal != "0" {
		t.Fatalf("NULL/0 lost %+v", zero)
	}
	free := groups["FreeEvent:known:8"]
	if free.EventCount != 1 || free.KnownSizeTotal != "20" || free.Subtype.Name.Value != "缓存|纹理" || free.SizeUnit != "bytes" {
		t.Fatalf("release/right edge/subtype lost %+v", free)
	}
	for key, status := range map[string]string{"MmapEvent:known:9": "null", "FD_Open_Event:known:10": "ambiguous", "THREAD_Create_Event:known:99": "unavailable", "AllocEvent:known:-1": "invalid"} {
		if g := groups[key]; g.Subtype.Name.Status != status {
			t.Fatalf("dictionary state %s: %+v", key, g)
		}
	}
	if g := groups["AllocEvent:null:"]; g.PID != 202 || g.IPID != 2 || g.NegativeSizeEvents != 1 || g.KnownSizeTotal != "" {
		t.Fatalf("owner/negative lost %+v", g)
	}
	if g := groups["FD_Open_Event:known:10"]; g.SizeUnit != "unspecified" {
		t.Fatalf("FD fabricated bytes %+v", g)
	}
	streamed, err := tracequery.StreamResourceStack(context.Background(), out, q)
	if err != nil || !reflect.DeepEqual(streamed.ResourceStack, p) {
		t.Fatalf("public streaming mismatch %v", err)
	}
}

func TestHMC278ResourceStatisticsAbsentOptionalColumns(t *testing.T) {
	_, idx := resourceSQLiteFixture(t, `ALTER TABLE native_hook DROP COLUMN heap_size; ALTER TABLE native_hook DROP COLUMN sub_type_id;`)
	p := tracequery.Run(idx, tracequery.Query{View: tracequery.ViewResourceStack, PID: 101, TimeStart: 10, TimeEnd: 10.05}).ResourceStack
	if !tracequery.ValidResourceStack(*p) || p.EventStatistics.EventCount != 3 {
		t.Fatalf("optional columns blocked valid events %+v", p)
	}
	for _, g := range p.EventStatistics.Groups {
		if g.UnavailableSizeEvents != g.EventCount || g.KnownSizeTotal != "" || g.Subtype.ID.Status != "unavailable" || g.Subtype.Name.Status != "unavailable" {
			t.Fatalf("absence turned zero %+v", g)
		}
	}
}
