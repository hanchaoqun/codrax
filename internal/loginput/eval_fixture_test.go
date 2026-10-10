package loginput

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSharedLogEvalFixtureOracle(t *testing.T) {
	root := filepath.Join("..", "..", "eval", "fixtures", "hmosperf_log_sources")
	app := filepath.Join(root, "app", "session.log")
	kernel := filepath.Join(root, "kernel", "session.log.gz")
	beforeApp, err := os.ReadFile(app)
	if err != nil {
		t.Fatal(err)
	}
	beforeKernel, err := os.ReadFile(kernel)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := Prepare(context.Background(), []Input{{Path: app}, {Path: kernel}}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	result, err := catalog.Query(context.Background(), Query{})
	if err != nil || !result.Complete || result.Matched != 9 || result.Returned != 9 || len(result.Sources) != 2 {
		t.Fatalf("%v %+v", err, result)
	}
	if result.Sources[0].PhysicalLines != 7 || result.Sources[0].ParsedRecords != 3 || result.Sources[0].MalformedRecords != 1 || result.Sources[0].OrphanContinuations != 1 || result.Sources[1].PhysicalLines != 4 || result.Sources[1].Compression != "gzip" {
		t.Fatalf("source oracle drift: %+v", result.Sources)
	}
	if result.Records[1].FirstLine != 2 || result.Records[1].LastLine != 3 || result.Records[3].PID != nil || result.Records[6].BootTimestampNS != "9007199254740993" || result.Records[7].PID != nil || result.Records[7].TID != nil {
		t.Fatalf("record oracle drift: %+v", result.Records)
	}
	afterApp, _ := os.ReadFile(app)
	afterKernel, _ := os.ReadFile(kernel)
	if string(beforeApp) != string(afterApp) || string(beforeKernel) != string(afterKernel) {
		t.Fatal("query changed attached sources")
	}
}
