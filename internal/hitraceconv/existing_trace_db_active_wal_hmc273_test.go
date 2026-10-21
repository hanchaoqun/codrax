package hitraceconv

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

// This is a passing safety regression and an explicit capability-gap witness,
// not an assertion that active-WAL snapshots have been delivered. The current
// immutable-input contract must keep rejecting a real commit during capture.
func TestHMC273ActiveWALDefaultPreparationFailsClosed(t *testing.T) {
	path, writer := existingWALFixture(t)
	opts := existingTraceDBOptions(t, path)
	beforeMain, err := filegeneration.FromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	var afterCommit map[string][]byte
	var afterCommitGenerations map[string]filegeneration.Identity
	var sourceNames []string
	committed := false
	opts.Progress = func(event ProgressEvent) {
		if committed || event.Stage != "existing_trace_db_snapshot" || event.Status != ProgressStatusComplete {
			return
		}
		// Deterministically interleave a real SQLite commit while the
		// public preparation still owns its input transaction. No source
		// permissions, owners, bytes, or timestamps are patched by the test.
		if _, err := writer.Exec("UPDATE process SET name='active-writer-next-commit'"); err != nil {
			t.Fatal(err)
		}
		committed = true
		afterCommit = walSourceBytes(t, path)
		afterCommitGenerations = make(map[string]filegeneration.Identity)
		for suffix := range afterCommit {
			id, err := filegeneration.FromPath(path + suffix)
			if err != nil {
				t.Fatal(err)
			}
			afterCommitGenerations[suffix] = id
		}
		entries, err := os.ReadDir(filepath.Dir(path))
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range entries {
			sourceNames = append(sourceNames, item.Name())
		}
	}
	result, err := PrepareExistingTraceDB(t.Context(), opts)
	if !committed {
		t.Fatal("active writer interleaving was not triggered")
	}
	assertExistingTraceDBFailureClean(t, opts, result, err)
	var sourceErr *ConversionInputError
	if !errors.As(err, &sourceErr) || sourceErr.Code != ConversionInputCodeGenerationChanged {
		t.Fatalf("active commit must retain precise generation failure, got %v", err)
	}
	if !beforeMain.SameVersion(afterCommitGenerations[""]) {
		t.Fatal("fixture changed main DB: this witness requires a WAL-only commit")
	}
	for suffix, expected := range afterCommit {
		got, err := os.ReadFile(path + suffix)
		if err != nil || !bytes.Equal(expected, got) {
			t.Fatalf("preparation changed source %q after the writer commit: %v", suffix, err)
		}
		id, err := filegeneration.FromPath(path + suffix)
		if err != nil || !id.SameVersion(afterCommitGenerations[suffix]) {
			t.Fatalf("preparation changed source generation/permissions %q: %v", suffix, err)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	var afterNames []string
	for _, item := range entries {
		afterNames = append(afterNames, item.Name())
	}
	if !reflect.DeepEqual(sourceNames, afterNames) {
		t.Fatalf("source sidecars changed: before=%v after=%v", sourceNames, afterNames)
	}
	t.Logf("active-WAL capability remains OPEN; public preparation safely rejected WAL-only commit: code=%s stage=%s; no publication or source mutation by preparation", sourceErr.Code, sourceErr.Stage)
}
