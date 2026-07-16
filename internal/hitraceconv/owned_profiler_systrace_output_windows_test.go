//go:build windows

package hitraceconv

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReleaseOwnedProfilerSystraceWindowsCommitReopensHeldGeneration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiler-windows-commit.systrace")
	sink := newProfilerWriterTestSink(t, 8, []profilerWriterTestRow{{
		tsNS: 1_000_000, seq: 1, body: profilerWriterKnownBody(71), publisher: profilerPairPublisherExactFtrace,
	}})
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	publication, err := writeValidatedOwnedProfilerSystraceWithLedger(
		context.Background(), path, sink, profilerContainerExtraction{}, profilerWriterTerminal(1), ledger,
	)
	if err != nil {
		t.Fatalf("publish Profiler generation while Windows held authority is live: %v", err)
	}
	if publication.Artifact.Path != path || publication.Artifact.Trace == nil ||
		!publication.Artifact.Trace.TraceQueryReady {
		t.Fatalf("Windows Profiler receipt drifted: %+v", publication)
	}
	if err := ledger.validateOwnedPaths(); err != nil {
		t.Fatalf("revalidate Windows Profiler public generation: %v", err)
	}
	if err := ledger.releaseOwnedAuthorities(); err != nil {
		t.Fatalf("release Windows Profiler publication authority: %v", err)
	}
	if len(ledger.created) != 1 || ledger.created[0].authority != nil || ledger.created[0].removed {
		t.Fatalf("Windows Profiler commit ledger drifted: %+v", ledger.created)
	}
	body, err := os.ReadFile(path)
	if err != nil || len(body) == 0 {
		t.Fatalf("committed Windows Profiler generation is unreadable: bytes=%d err=%v", len(body), err)
	}
}

func TestReleaseOwnedProfilerSystraceWindowsRollbackUsesHeldGeneration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profiler-windows-rollback.systrace")
	sink := newProfilerWriterTestSink(t, 8, []profilerWriterTestRow{{
		tsNS: 1_000_000, seq: 1, body: profilerWriterKnownBody(72), publisher: profilerPairPublisherExactFtrace,
	}})
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := writeValidatedOwnedProfilerSystraceWithLedger(
		context.Background(), path, sink, profilerContainerExtraction{}, profilerWriterTerminal(1), ledger,
	); err != nil {
		t.Fatalf("publish Windows Profiler rollback fixture: %v", err)
	}
	if err := ledger.removeOwnedPath(path); err != nil {
		t.Fatalf("rollback Windows Profiler generation through held authority: %v", err)
	}
	if len(ledger.created) != 1 || !ledger.created[0].removed || ledger.created[0].authority != nil {
		t.Fatalf("Windows Profiler rollback ledger drifted: %+v", ledger.created)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("Windows Profiler generation survived rollback: %v", err)
	}
	assertNoProfilerStagingResidue(t, dir)
}

func TestReleaseConvertFileProfilerWindowsBundleMeasuresHeldChild(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "profiler-windows-bundle.htrace")
	output := filepath.Join(dir, "profiler-windows-bundle.systrace")
	if err := os.WriteFile(input, profilerAuthorityFixture("trace_file"), 0o640); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin,
	})
	if err != nil {
		t.Fatalf("convert Windows Profiler fixture through bundle commit: %v", err)
	}
	if result.OutputPath != output || result.BundlePath == "" {
		t.Fatalf("Windows Profiler bundle result drifted: %+v", result)
	}
	for _, path := range []string{result.OutputPath, result.BundlePath} {
		body, readErr := os.ReadFile(path)
		if readErr != nil || len(body) == 0 {
			t.Fatalf("Windows Profiler committed artifact %q is unreadable: bytes=%d err=%v", path, len(body), readErr)
		}
	}
}
