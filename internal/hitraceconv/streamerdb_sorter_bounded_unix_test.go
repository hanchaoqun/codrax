//go:build unix

package hitraceconv

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"testing"
)

func TestTraceDBRowSorterProductFanInUnderLowRLimit(t *testing.T) {
	const childEnv = "CODRAX_ROW_SORT_RLIMIT_CHILD"
	if os.Getenv(childEnv) != "1" {
		command := exec.Command(os.Args[0], "-test.run=^TestTraceDBRowSorterProductFanInUnderLowRLimit$", "-test.v")
		command.Env = append(os.Environ(), childEnv+"=1")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("low-RLIMIT sorter subprocess failed: %v\n%s", err, output)
		}
		return
	}

	var limit syscall.Rlimit
	if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		t.Fatal(err)
	}
	target := uint64(64)
	if limit.Max < target {
		target = limit.Max
	}
	if target < uint64(defaultTraceDBRowMergeFanIn+8) {
		t.Skipf("RLIMIT_NOFILE hard limit %d is too low for product fan-in proof", limit.Max)
	}
	limit.Cur = target
	if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
		t.Fatal(err)
	}

	const rowCount = defaultTraceDBRowMergeFanIn*defaultTraceDBRowMergeFanIn + 1
	sink, err := newTraceDBRowSink(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = sink.cleanup() }()
	for index := 0; index < rowCount; index++ {
		if err := sink.add(renderedRow{tsNS: uint64(rowCount - index), seq: index, line: fmt.Sprintf("rlimit-%04d", index)}); err != nil {
			t.Fatal(err)
		}
	}
	var output bytes.Buffer
	stats, err := sink.prepareAndWriteForTest(context.Background(), &output)
	if err != nil {
		t.Fatal(err)
	}
	if stats.RowsWritten != rowCount || stats.SpillChunks != rowCount || stats.MergePasses != 3 ||
		stats.PeakOpenRunFDs > defaultTraceDBRowMergeFanIn+1 {
		t.Fatalf("product fan-in escaped low-RLIMIT proof: %+v", stats)
	}
}
