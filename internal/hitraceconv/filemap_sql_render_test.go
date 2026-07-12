package hitraceconv

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type traceDBFilemapTestArg struct {
	key      string
	value    int64
	datatype int64
	text     string
}

type traceDBFilemapTestRow struct {
	name string
	args []traceDBFilemapTestArg
}

func traceDBFilemapInt(key string, value int64) traceDBFilemapTestArg {
	return traceDBFilemapTestArg{key: key, value: value}
}

func traceDBFilemapText(key, value string) traceDBFilemapTestArg {
	return traceDBFilemapTestArg{key: key, datatype: 1, text: value}
}

func traceDBFilemapPageArgs(dev, ino, index, pfn int64) []traceDBFilemapTestArg {
	return []traceDBFilemapTestArg{
		traceDBFilemapInt("s_dev", dev),
		traceDBFilemapInt("i_ino", ino),
		traceDBFilemapInt("index", index),
		traceDBFilemapInt("pfn", pfn),
	}
}

func traceDBFilemapSetArgs(dev, ino, errseq int64) []traceDBFilemapTestArg {
	return []traceDBFilemapTestArg{
		traceDBFilemapInt("s_dev", dev),
		traceDBFilemapInt("i_ino", ino),
		traceDBFilemapInt("errseq", errseq),
	}
}

func cloneTraceDBFilemapArgs(args []traceDBFilemapTestArg) []traceDBFilemapTestArg {
	return append([]traceDBFilemapTestArg(nil), args...)
}

func traceDBFilemapTestAuthority() traceDBSchedulerAuthority {
	index := newTraceDBThreadIndex(0, true)
	index.Processes[1] = traceDBProcess{IPID: 1, PID: 100, Name: "app"}
	index.ByITID[1] = traceDBThread{ITID: 1, TID: 10, IPID: 1, Name: "worker"}
	buildTraceDBThreadSecondaryIndexes(&index)
	return traceDBTestCompleteSchedulerAuthority(index)
}

func exportTraceDBFilemapRows(t *testing.T, rows []traceDBFilemapTestRow) ([]TraceDBCoverage, string) {
	t.Helper()
	dict := map[string]int{}
	var dictStatements, argStatements, rawStatements []string
	nextDict := 1
	dictID := func(value string) int {
		if id, ok := dict[value]; ok {
			return id
		}
		id := nextDict
		nextDict++
		dict[value] = id
		escaped := strings.ReplaceAll(value, "'", "''")
		dictStatements = append(dictStatements, fmt.Sprintf("INSERT INTO data_dict VALUES (%d, '%s')", id, escaped))
		return id
	}
	for i, row := range rows {
		argset := i + 1
		for _, arg := range row.args {
			value := arg.value
			if arg.datatype == 1 {
				value = int64(dictID(arg.text))
			}
			argStatements = append(argStatements, fmt.Sprintf("INSERT INTO args VALUES (%d, %d, %d, %d)",
				argset, dictID(arg.key), arg.datatype, value))
		}
		escapedName := strings.ReplaceAll(row.name, "'", "''")
		rawStatements = append(rawStatements, fmt.Sprintf("INSERT INTO raw VALUES (%d, %d, '%s', 2, 1, %d)",
			i+1, 1000+i, escapedName, argset))
	}
	statements := []string{
		"CREATE TABLE data_dict (id, data)",
		"CREATE TABLE args (argset, key, datatype, value)",
		"CREATE TABLE raw (id, ts, name, cpu, itid, argsetid)",
	}
	statements = append(statements, dictStatements...)
	statements = append(statements, argStatements...)
	statements = append(statements, rawStatements...)
	return exportTraceDBRawB3BFixture(t, statements, traceDBFilemapTestAuthority(),
		traceDBSchedulerRunningIndex{initialized: true})
}

func buildTraceDBFilemapIndex(t *testing.T, body string) *tracequery.Index {
	t.Helper()
	path := filepath.Join(t.TempDir(), "filemap-sql.ftrace")
	if err := os.WriteFile(path, []byte("# tracer: nop\n#\n"+body+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatalf("tracequery parse SQL filemap output: %v\n%s", err, body)
	}
	return idx
}

func TestTraceDBFilemapExactProfilesCanonicalAndRoundTrip(t *testing.T) {
	dev := int64(syntheticDev(260, 136))
	maxIndex := int64(math.MaxInt64 >> 12)
	maxPage := append(traceDBFilemapPageArgs(math.MaxUint32, math.MaxInt64, maxIndex, math.MaxInt64),
		traceDBFilemapInt("order", 255))
	rows := []traceDBFilemapTestRow{
		{name: "mm_filemap_add_to_page_cache", args: traceDBFilemapPageArgs(dev, 12345, 1, 3062260)},
		{name: "mm_filemap_delete_from_page_cache", args: append(traceDBFilemapPageArgs(dev, 12345, 1, 3062260), traceDBFilemapInt("order", 0))},
		{name: "mm_filemap_add_to_page_cache", args: maxPage},
		{name: "filemap_set_wb_err", args: traceDBFilemapSetArgs(dev, 12345, 0x12)},
		{name: "filemap_set_wb_err", args: traceDBFilemapSetArgs(math.MaxUint32, math.MaxInt64, math.MaxUint32)},
		{name: "file_check_and_advance_wb_err", args: []traceDBFilemapTestArg{
			traceDBFilemapInt("file", 0x1000), traceDBFilemapInt("s_dev", dev), traceDBFilemapInt("i_ino", 12345),
			traceDBFilemapInt("old", 1), traceDBFilemapInt("new", 2),
		}},
	}
	coverage, body := exportTraceDBFilemapRows(t, rows)
	for _, want := range []string{
		"mm_filemap_add_to_page_cache: dev 260:136 ino 0x3039 pfn=3062260 ofs=4096",
		"mm_filemap_delete_from_page_cache: dev 260:136 ino 0x3039 pfn=3062260 ofs=4096",
		"mm_filemap_add_to_page_cache: dev 4095:1048575 ino 0x7fffffffffffffff pfn=9223372036854775807 ofs=9223372036854771712",
		"filemap_set_wb_err: dev=260:136 ino=0x3039 errseq=0x12",
		"filemap_set_wb_err: dev=4095:1048575 ino=0x7fffffffffffffff errseq=0xffffffff",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("SQL filemap canonical output missing %q:\n%s\ncoverage=%+v", want, body, coverage)
		}
	}
	for _, forbidden := range []string{"page=", " dev=260:136 ino=12345", "file_check_and_advance_wb_err:"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("SQL filemap output escaped its source boundary via %q:\n%s", forbidden, body)
		}
	}
	if strings.Count(body, "mm_filemap_add_to_page_cache:") != 2 ||
		strings.Count(body, "mm_filemap_delete_from_page_cache:") != 1 ||
		strings.Count(body, "filemap_set_wb_err:") != 2 {
		t.Fatalf("SQL exact filemap profile row counts changed:\n%s", body)
	}

	idx := buildTraceDBFilemapIndex(t, body)
	stats := tracequery.ComputeWindowStats(idx, tracequery.Query{})
	foundNormal := false
	for _, item := range stats.PageCacheByInode {
		if item.Dev == "260:136" && item.Inode == "0x3039" {
			foundNormal = item.Adds == 1 && item.Deletes == 1 && item.Churn == 2 && item.MinOffset == 4096 && item.MaxOffset == 4096
		}
	}
	if !foundNormal {
		t.Fatalf("SQL canonical page rows did not round-trip as one add/delete mutation: %+v", stats.PageCacheByInode)
	}
	writebackEvents := 0
	for _, ev := range idx.Events {
		if ev.Name == "filemap_set_wb_err" {
			writebackEvents++
			if ev.Type != tracequery.EventFilesystem || ev.SubsystemKind != "writeback" {
				t.Fatalf("SQL writeback row lost its exact searchable classification: %+v", ev)
			}
		}
	}
	if writebackEvents != 2 {
		t.Fatalf("SQL writeback rows did not round-trip as exact observations: events=%+v", idx.Events)
	}

	var writebackLines []string
	for _, line := range strings.Split(body, "\n") {
		if strings.Contains(line, "filemap_set_wb_err:") {
			writebackLines = append(writebackLines, line)
		}
	}
	writebackStats := tracequery.ComputeWindowStats(buildTraceDBFilemapIndex(t, strings.Join(writebackLines, "\n")), tracequery.Query{})
	if len(writebackStats.FilesystemResources) != 0 || len(writebackStats.FileIOByInode) != 0 ||
		len(writebackStats.PageCacheByInode) != 0 || writebackStats.TopIOInodes != nil ||
		len(writebackStats.StorageLatencyByLayer) != 0 || writebackStats.IOPressureSummary != nil ||
		len(writebackStats.IOBurstEpisodes) != 0 || len(writebackStats.BlockIOByInode) != 0 {
		t.Fatalf("writeback observations leaked into IO/resource/rank inputs: %+v", writebackStats)
	}
}

func TestTraceDBFilemapStrictNamesCarriersBoundsAndSiblingLocality(t *testing.T) {
	dev := int64(syntheticDev(260, 136))
	valid := traceDBFilemapPageArgs(dev, 12345, 0, 3062260)
	validSet := traceDBFilemapSetArgs(dev, 12345, 0x12)
	invalid := []traceDBFilemapTestRow{
		{name: "MM_FILEMAP_ADD_TO_PAGE_CACHE", args: valid},
		{name: "mm_filemap_add_to_page_cache_start", args: valid},
		{name: "mm_filemap_add_to_page_cache ", args: valid},
		{name: "mm_filemap_fault", args: valid},
		{name: "file_check_and_advance_wb_err", args: []traceDBFilemapTestArg{
			traceDBFilemapInt("file", 0x1000), traceDBFilemapInt("s_dev", dev), traceDBFilemapInt("i_ino", 12345),
			traceDBFilemapInt("old", 1), traceDBFilemapInt("new", 2),
		}},
	}
	for i := range valid {
		missing := cloneTraceDBFilemapArgs(valid)
		missing = append(missing[:i], missing[i+1:]...)
		invalid = append(invalid, traceDBFilemapTestRow{name: "mm_filemap_add_to_page_cache", args: missing})
	}
	for i := range validSet {
		missing := cloneTraceDBFilemapArgs(validSet)
		missing = append(missing[:i], missing[i+1:]...)
		invalid = append(invalid, traceDBFilemapTestRow{name: "filemap_set_wb_err", args: missing})
	}
	for _, replacement := range []traceDBFilemapTestArg{
		traceDBFilemapInt("dev", dev), traceDBFilemapInt("ino", 12345), traceDBFilemapInt("offset", 0),
		traceDBFilemapInt("ofs", 0), traceDBFilemapInt("pos", 0), traceDBFilemapInt("bytes", 4096),
		traceDBFilemapText("entry_name", "secret.db"), traceDBFilemapInt("page", 0),
		traceDBFilemapInt("pg", 0), traceDBFilemapInt("unknown", 1),
	} {
		args := cloneTraceDBFilemapArgs(valid)
		args = append(args, replacement)
		invalid = append(invalid, traceDBFilemapTestRow{name: "mm_filemap_add_to_page_cache", args: args})
	}
	for _, args := range [][]traceDBFilemapTestArg{
		append(cloneTraceDBFilemapArgs(valid), traceDBFilemapInt("s_dev", dev)),
		append(cloneTraceDBFilemapArgs(valid), traceDBFilemapInt("s_dev", dev+1)),
		append(cloneTraceDBFilemapArgs(valid), traceDBFilemapInt("order", 1), traceDBFilemapInt("order", 1)),
		append(cloneTraceDBFilemapArgs(valid), traceDBFilemapInt("order", 1), traceDBFilemapInt("order", 2)),
	} {
		invalid = append(invalid, traceDBFilemapTestRow{name: "mm_filemap_add_to_page_cache", args: args})
	}
	for _, args := range [][]traceDBFilemapTestArg{
		traceDBFilemapPageArgs(-1, 12345, 0, 1),
		traceDBFilemapPageArgs(int64(math.MaxUint32)+1, 12345, 0, 1),
		traceDBFilemapPageArgs(dev, -1, 0, 1),
		traceDBFilemapPageArgs(dev, 12345, -1, 1),
		traceDBFilemapPageArgs(dev, 12345, int64(math.MaxInt64>>12)+1, 1),
		traceDBFilemapPageArgs(dev, 12345, 0, -1),
		append(traceDBFilemapPageArgs(dev, 12345, 0, 1), traceDBFilemapInt("order", -1)),
		append(traceDBFilemapPageArgs(dev, 12345, 0, 1), traceDBFilemapInt("order", 256)),
		append(traceDBFilemapPageArgs(dev, 12345, 0, 1), traceDBFilemapText("order", "1")),
	} {
		invalid = append(invalid, traceDBFilemapTestRow{name: "mm_filemap_add_to_page_cache", args: args})
	}
	for _, args := range [][]traceDBFilemapTestArg{
		traceDBFilemapSetArgs(-1, 12345, 1),
		traceDBFilemapSetArgs(int64(math.MaxUint32)+1, 12345, 1),
		traceDBFilemapSetArgs(dev, -1, 1),
		traceDBFilemapSetArgs(dev, 12345, -1),
		traceDBFilemapSetArgs(dev, 12345, int64(math.MaxUint32)+1),
		append(traceDBFilemapSetArgs(dev, 12345, 1), traceDBFilemapInt("bytes", 4096)),
		append(traceDBFilemapSetArgs(dev, 12345, 1), traceDBFilemapText("entry_name", "secret.db")),
		append(traceDBFilemapSetArgs(dev, 12345, 1), traceDBFilemapInt("ino", 12345)),
		append(traceDBFilemapSetArgs(dev, 12345, 1), traceDBFilemapInt("errseq", 1)),
		append(traceDBFilemapSetArgs(dev, 12345, 1), traceDBFilemapInt("errseq", 2)),
		{traceDBFilemapText("s_dev", fmt.Sprint(dev)), traceDBFilemapInt("i_ino", 12345), traceDBFilemapInt("errseq", 1)},
		{traceDBFilemapInt("s_dev", dev), traceDBFilemapText("i_ino", "12345"), traceDBFilemapInt("errseq", 1)},
		{traceDBFilemapInt("s_dev", dev), traceDBFilemapInt("i_ino", 12345), traceDBFilemapText("errseq", "1")},
	} {
		invalid = append(invalid, traceDBFilemapTestRow{name: "filemap_set_wb_err", args: args})
	}
	rows := []traceDBFilemapTestRow{{name: "mm_filemap_add_to_page_cache", args: valid}}
	rows = append(rows, invalid...)
	rows = append(rows,
		traceDBFilemapTestRow{name: "mm_filemap_delete_from_page_cache", args: valid},
		traceDBFilemapTestRow{name: "filemap_set_wb_err", args: validSet})
	coverage, body := exportTraceDBFilemapRows(t, rows)
	if strings.Count(body, "mm_filemap_add_to_page_cache:") != 1 ||
		strings.Count(body, "mm_filemap_delete_from_page_cache:") != 1 ||
		strings.Count(body, "filemap_set_wb_err:") != 1 {
		t.Fatalf("invalid SQL filemap rows escaped or poisoned valid siblings:\n%s\ncoverage=%+v", body, coverage)
	}
	for _, forbidden := range []string{
		"MM_FILEMAP_ADD_TO_PAGE_CACHE:", "mm_filemap_add_to_page_cache_start:",
		"mm_filemap_fault:", "file_check_and_advance_wb_err:", "secret.db", "bytes=4096",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("SQL filemap strict profile published forbidden %q:\n%s", forbidden, body)
		}
	}
	stats := tracequery.ComputeWindowStats(buildTraceDBFilemapIndex(t, body), tracequery.Query{})
	if len(stats.PageCacheByInode) != 1 || stats.PageCacheByInode[0].Adds != 1 ||
		stats.PageCacheByInode[0].Deletes != 1 || stats.PageCacheByInode[0].Churn != 2 {
		t.Fatalf("invalid SQL rows changed page-cache mutations: %+v", stats.PageCacheByInode)
	}
}
