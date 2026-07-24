package hitraceconv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracebundle"
)

type traceBundleFaultWriter struct {
	file    *os.File
	writeFn func([]byte) (int, error)
	syncFn  func() error
	closeFn func() error
}

func (writer *traceBundleFaultWriter) Write(buffer []byte) (int, error) {
	if writer.writeFn != nil {
		return writer.writeFn(buffer)
	}
	return writer.file.Write(buffer)
}

func (writer *traceBundleFaultWriter) Sync() error {
	if writer.syncFn != nil {
		return writer.syncFn()
	}
	return writer.file.Sync()
}

func (writer *traceBundleFaultWriter) Close() error {
	if writer.closeFn != nil {
		return writer.closeFn()
	}
	return writer.file.Close()
}

func TestTraceBundleAtomicPublicationFirstVisibleBodyIsComplete(t *testing.T) {
	requireTraceBundleAtomicPublisher(t)
	dir := t.TempDir()
	input := writeTraceBundleAtomicInput(t, dir)
	finalPath := traceSidecarBase(input, "") + ".tracebundle.json"
	beforePublish := make(chan struct{})
	continuePublish := make(chan struct{})
	afterPublish := make(chan struct{})
	continueCommit := make(chan struct{})
	type result struct {
		artifact Artifact
		err      error
	}
	resultCh := make(chan result, 1)
	go func() {
		artifact, err := writeTraceBundleAtomicFixture(context.Background(), input, []string{"atomic visibility"}, traceBundlePublicationOps{
			checkpoint: func(phase traceBundlePublicationPhase) error {
				switch phase {
				case traceBundlePublicationBeforePublish:
					close(beforePublish)
					<-continuePublish
				case traceBundlePublicationAfterPublish:
					close(afterPublish)
					<-continueCommit
				}
				return nil
			},
		})
		resultCh <- result{artifact: artifact, err: err}
	}()

	awaitTraceBundleAtomicSignal(t, beforePublish, "before publish")
	if _, err := os.Lstat(finalPath); !os.IsNotExist(err) {
		t.Errorf("tracebundle final path became visible before atomic publication: %v", err)
	}
	close(continuePublish)
	awaitTraceBundleAtomicSignal(t, afterPublish, "after publish")
	body, readErr := os.ReadFile(finalPath)
	if readErr != nil {
		t.Errorf("read first-visible tracebundle body: %v", readErr)
	} else {
		if err := tracebundle.ValidateManifestBytes(context.Background(), body); err != nil {
			t.Errorf("first-visible tracebundle body is not consumer-acceptable: %v", err)
		}
		var manifest traceBundleMetadata
		if err := json.Unmarshal(body, &manifest); err != nil {
			t.Errorf("first-visible tracebundle body is partial JSON: %v\n%s", err, body)
		} else if manifest.Schema != tracebundle.SchemaV2 || manifest.CaptureID == "" {
			t.Errorf("first-visible tracebundle body lost V2 identity: %+v", manifest)
		}
	}
	close(continueCommit)
	completed := awaitTraceBundleAtomicResult(t, resultCh)
	if completed.err != nil {
		t.Fatal(completed.err)
	}
	if completed.artifact.Path != finalPath || completed.artifact.Bytes != int64(len(body)) {
		t.Fatalf("atomic tracebundle artifact mismatch: %+v body=%d", completed.artifact, len(body))
	}
	assertNoTraceBundlePrivateStaging(t, dir)
}

func TestTraceBundleAtomicPublicationCollisionPreservesCompetitor(t *testing.T) {
	requireTraceBundleAtomicPublisher(t)
	dir := t.TempDir()
	input := writeTraceBundleAtomicInput(t, dir)
	finalPath := traceSidecarBase(input, "") + ".tracebundle.json"
	competitor := []byte("external owner\n")
	artifact, err := writeTraceBundleAtomicFixture(context.Background(), input, []string{"collision"}, traceBundlePublicationOps{
		checkpoint: func(phase traceBundlePublicationPhase) error {
			if phase != traceBundlePublicationBeforePublish {
				return nil
			}
			return os.WriteFile(finalPath, competitor, 0o600)
		},
	})
	if err == nil || !reflect.DeepEqual(artifact, Artifact{}) {
		t.Fatalf("publication collision returned artifact=%+v err=%v", artifact, err)
	}
	got, readErr := os.ReadFile(finalPath)
	if readErr != nil || !bytes.Equal(got, competitor) {
		t.Fatalf("atomic collision changed competitor: got=%q err=%v", got, readErr)
	}
	assertNoTraceBundlePrivateStaging(t, dir)
}

func TestTraceBundleAtomicPublicationRejectsProducerEnvelopeOverflowBeforeStaging(t *testing.T) {
	tests := []struct {
		name    string
		caveats []string
		want    error
	}{
		{
			name:    "top-level structural array",
			caveats: traceBundleAtomicCaveats(1025, 1),
			want:    tracebundle.ErrInvalidManifest,
		},
		{
			name:    "final body including LF exceeds 4 MiB",
			caveats: traceBundleAtomicCaveats(1024, 5000),
			want:    tracebundle.ErrTooLarge,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			input := writeTraceBundleAtomicInput(t, dir)
			opened := false
			artifact, err := writeTraceBundleAtomicFixture(context.Background(), input, test.caveats, traceBundlePublicationOps{
				openStaging: func(string) (traceBundleStagingWriter, error) {
					opened = true
					return nil, errors.New("staging must not be reached")
				},
			})
			if !errors.Is(err, test.want) || !reflect.DeepEqual(artifact, Artifact{}) {
				t.Fatalf("producer envelope error artifact=%+v err=%v want=%v", artifact, err, test.want)
			}
			if opened {
				t.Fatal("producer created staging before rejecting its own manifest envelope")
			}
			assertTraceBundleFinalAbsent(t, input)
			assertNoTraceBundlePrivateStaging(t, dir)
		})
	}
}

func TestTraceBundleAtomicPublicationPrivateIOFailuresLeaveNoPublication(t *testing.T) {
	openErr := errors.New("injected tracebundle open failure")
	syncErr := errors.New("injected tracebundle sync failure")
	closeErr := errors.New("injected tracebundle close failure")
	tests := []struct {
		name string
		want error
		open func(string) (traceBundleStagingWriter, error)
	}{
		{
			name: "open",
			want: openErr,
			open: func(string) (traceBundleStagingWriter, error) { return nil, openErr },
		},
		{
			name: "short write",
			want: io.ErrShortWrite,
			open: func(path string) (traceBundleStagingWriter, error) {
				file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
				if err != nil {
					return nil, err
				}
				writer := &traceBundleFaultWriter{file: file}
				writer.writeFn = func(buffer []byte) (int, error) {
					return file.Write(buffer[:len(buffer)/2])
				}
				return writer, nil
			},
		},
		{
			name: "sync",
			want: syncErr,
			open: func(path string) (traceBundleStagingWriter, error) {
				file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
				return &traceBundleFaultWriter{file: file, syncFn: func() error { return syncErr }}, err
			},
		},
		{
			name: "close",
			want: closeErr,
			open: func(path string) (traceBundleStagingWriter, error) {
				file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
				return &traceBundleFaultWriter{file: file, closeFn: func() error {
					return traceDBJoinPreservingSingle(file.Close(), closeErr)
				}}, err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			input := writeTraceBundleAtomicInput(t, dir)
			artifact, err := writeTraceBundleAtomicFixture(context.Background(), input, []string{"I/O fault"}, traceBundlePublicationOps{openStaging: test.open})
			if !errors.Is(err, test.want) || !reflect.DeepEqual(artifact, Artifact{}) {
				t.Fatalf("private I/O fault artifact=%+v err=%v want=%v", artifact, err, test.want)
			}
			assertTraceBundleFinalAbsent(t, input)
			assertNoTraceBundlePrivateStaging(t, dir)
		})
	}
}

func TestTraceBundleAtomicPublicationRejectsPrivateGenerationAndDigestChanges(t *testing.T) {
	tests := []struct {
		name       string
		phase      traceBundlePublicationPhase
		mutateBody bool
		mutate     func(string) error
	}{
		{
			name:  "adopt missing",
			phase: traceBundlePublicationStagingWritten,
			mutate: func(path string) error {
				return os.Rename(path, path+".displaced")
			},
		},
		{
			name:       "digest differs from marshaled body",
			mutateBody: true,
		},
		{
			name:  "same inode same size restored mtime after adopt",
			phase: traceBundlePublicationStagingAdopted,
			mutate: func(path string) error {
				info, err := os.Stat(path)
				if err != nil {
					return err
				}
				body, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				body[len(body)/2] ^= 1
				if err := os.WriteFile(path, body, info.Mode().Perm()); err != nil {
					return err
				}
				return os.Chtimes(path, info.ModTime(), info.ModTime())
			},
		},
		{
			name:  "atomic staging replacement after adopt",
			phase: traceBundlePublicationStagingAdopted,
			mutate: func(path string) error {
				body, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if err := os.Rename(path, path+".old"); err != nil {
					return err
				}
				return os.WriteFile(path, body, 0o644)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			input := writeTraceBundleAtomicInput(t, dir)
			var stagingPath string
			ops := traceBundlePublicationOps{
				openStaging: func(path string) (traceBundleStagingWriter, error) {
					stagingPath = path
					file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
					if err != nil {
						return nil, err
					}
					writer := &traceBundleFaultWriter{file: file}
					if test.mutateBody {
						writer.writeFn = func(buffer []byte) (int, error) {
							changed := append([]byte(nil), buffer...)
							changed[len(changed)/2] ^= 1
							return file.Write(changed)
						}
					}
					return writer, nil
				},
				checkpoint: func(phase traceBundlePublicationPhase) error {
					if phase != test.phase || test.mutate == nil {
						return nil
					}
					return test.mutate(stagingPath)
				},
			}
			artifact, err := writeTraceBundleAtomicFixture(context.Background(), input, []string{"generation fault"}, ops)
			if err == nil || !reflect.DeepEqual(artifact, Artifact{}) {
				t.Fatalf("private generation fault artifact=%+v err=%v", artifact, err)
			}
			assertTraceBundleFinalAbsent(t, input)
			assertNoTraceBundlePrivateStaging(t, dir)
		})
	}
}

func TestTraceBundleAtomicPublicationCancellationAtEveryCheckpointRollsBack(t *testing.T) {
	phases := []traceBundlePublicationPhase{
		traceBundlePublicationTargetPrepared,
		traceBundlePublicationStagingWritten,
		traceBundlePublicationStagingAdopted,
		traceBundlePublicationBodyAttested,
		traceBundlePublicationBeforePublish,
		traceBundlePublicationAfterPublish,
		traceBundlePublicationBeforeCommit,
	}
	for _, phase := range phases {
		t.Run(string(phase), func(t *testing.T) {
			requireTraceBundleAtomicPublisher(t)
			dir := t.TempDir()
			input := writeTraceBundleAtomicInput(t, dir)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			artifact, err := writeTraceBundleAtomicFixture(ctx, input, []string{"cancel"}, traceBundlePublicationOps{
				checkpoint: func(got traceBundlePublicationPhase) error {
					if got == phase {
						cancel()
					}
					return nil
				},
			})
			if !errors.Is(err, context.Canceled) || !reflect.DeepEqual(artifact, Artifact{}) {
				t.Fatalf("checkpoint cancellation phase=%s artifact=%+v err=%v", phase, artifact, err)
			}
			assertTraceBundleFinalAbsent(t, input)
			assertNoTraceBundlePrivateStaging(t, dir)
		})
	}

	dir := t.TempDir()
	input := writeTraceBundleAtomicInput(t, dir)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	checkpointCalled := false
	artifact, err := writeTraceBundleAtomicFixture(ctx, input, []string{"pre-cancel"}, traceBundlePublicationOps{
		checkpoint: func(traceBundlePublicationPhase) error { checkpointCalled = true; return nil },
	})
	if !errors.Is(err, context.Canceled) || checkpointCalled || !reflect.DeepEqual(artifact, Artifact{}) {
		t.Fatalf("pre-cancel artifact=%+v checkpoint=%v err=%v", artifact, checkpointCalled, err)
	}
	assertTraceBundleFinalAbsent(t, input)
}

func TestTraceBundleAtomicPublicationChildMutationBeforeAndAfterPublishFailsClosed(t *testing.T) {
	for _, phase := range []traceBundlePublicationPhase{traceBundlePublicationBeforePublish, traceBundlePublicationAfterPublish} {
		t.Run(string(phase), func(t *testing.T) {
			requireTraceBundleAtomicPublisher(t)
			dir := t.TempDir()
			input := writeTraceBundleAtomicInput(t, dir)
			ledger, err := newConversionFileLedger(input)
			if err != nil {
				t.Fatal(err)
			}
			childArtifact, childDecision, childCoverage := validatedResultBuiltinSystraceFixture(
				t, ledger, input, filepath.Join(dir, "capture.systrace"),
				[]renderedRow{builtinWriterKnownRow(1_000_000, 0)},
			)
			child := childArtifact.Path
			checkpointReached := false
			artifact, err := writeTraceBundleWithAllCoverageAndGatesAndLedgerOps(
				context.Background(), input, child,
				[]Artifact{childArtifact}, nil, nil, []TraceProviderDecision{childDecision}, nil,
				[]TraceDBCoverage{childCoverage}, nil, ledger,
				traceBundlePublicationOps{checkpoint: func(got traceBundlePublicationPhase) error {
					if got != phase {
						return nil
					}
					checkpointReached = true
					return os.WriteFile(child, []byte("# tracer: bad\n"), 0o644)
				}},
			)
			if err == nil || !reflect.DeepEqual(artifact, Artifact{}) || !checkpointReached {
				t.Fatalf("causal child mutation phase=%s artifact=%+v err=%v", phase, artifact, err)
			}
			// Deliberate public-generation mutation is expected to surface again
			// while the ledger closes its held authority.
			_ = ledger.cleanup()
			bundlePath := traceSidecarBase(input, child) + ".tracebundle.json"
			if _, statErr := os.Lstat(bundlePath); !os.IsNotExist(statErr) {
				t.Fatalf("child mutation left tracebundle publication: %v", statErr)
			}
			assertNoTraceBundlePrivateStaging(t, dir)
		})
	}
}

func TestTraceBundleAtomicPublicationRollbackPreservesExternalReplacement(t *testing.T) {
	requireTraceBundleAtomicPublisher(t)
	dir := t.TempDir()
	input := writeTraceBundleAtomicInput(t, dir)
	finalPath := traceSidecarBase(input, "") + ".tracebundle.json"
	displaced := finalPath + ".published-generation"
	competitor := []byte("external replacement\n")
	artifact, err := writeTraceBundleAtomicFixture(context.Background(), input, []string{"replacement"}, traceBundlePublicationOps{
		checkpoint: func(phase traceBundlePublicationPhase) error {
			if phase != traceBundlePublicationAfterPublish {
				return nil
			}
			if err := os.Rename(finalPath, displaced); err != nil {
				return err
			}
			return os.WriteFile(finalPath, competitor, 0o600)
		},
	})
	if err == nil || !reflect.DeepEqual(artifact, Artifact{}) {
		t.Fatalf("external replacement rollback artifact=%+v err=%v", artifact, err)
	}
	got, readErr := os.ReadFile(finalPath)
	if readErr != nil || !bytes.Equal(got, competitor) {
		t.Fatalf("rollback removed external replacement: got=%q err=%v", got, readErr)
	}
	if _, statErr := os.Lstat(displaced); statErr != nil {
		t.Fatalf("rollback lost displaced creator generation without its binding: %v", statErr)
	}
	assertNoTraceBundlePrivateStaging(t, dir)
}

func TestTraceBundleAtomicPublicationJoinsCleanupFailureAndPreservesReplacements(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("directory-binding replacement is exercised natively on POSIX; Windows has a separate held-delete release gate")
	}
	dir := t.TempDir()
	input := writeTraceBundleAtomicInput(t, dir)
	finalPath := traceSidecarBase(input, "") + ".tracebundle.json"
	displacedFinal := finalPath + ".creator-generation"
	competitor := []byte("external final owner\n")
	var stagingRoot string
	var displacedStaging string
	var mutationErr error
	artifact, err := writeTraceBundleAtomicFixture(context.Background(), input, []string{"cleanup double error"}, traceBundlePublicationOps{
		openStaging: func(path string) (traceBundleStagingWriter, error) {
			stagingRoot = filepath.Dir(path)
			displacedStaging = stagingRoot + ".creator-generation"
			return os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		},
		checkpoint: func(phase traceBundlePublicationPhase) error {
			if phase != traceBundlePublicationAfterPublish {
				return nil
			}
			for _, operation := range []func() error{
				func() error { return os.Rename(finalPath, displacedFinal) },
				func() error { return os.WriteFile(finalPath, competitor, 0o600) },
				func() error { return os.Rename(stagingRoot, displacedStaging) },
				func() error { return os.Mkdir(stagingRoot, 0o700) },
			} {
				if operationErr := operation(); operationErr != nil {
					mutationErr = operationErr
					return operationErr
				}
			}
			return nil
		},
	})
	if mutationErr != nil {
		t.Fatalf("prepare replacement/cleanup double-error fixture: %v", mutationErr)
	}
	if err == nil || !reflect.DeepEqual(artifact, Artifact{}) {
		t.Fatalf("cleanup double error artifact=%+v err=%v", artifact, err)
	}
	for _, want := range []string{"revalidate tracebundle publication", "private conversion"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("cleanup double error lost %q: %v", want, err)
		}
	}
	got, readErr := os.ReadFile(finalPath)
	if readErr != nil || !bytes.Equal(got, competitor) {
		t.Fatalf("cleanup double error removed external final replacement: got=%q err=%v", got, readErr)
	}
	if _, statErr := os.Lstat(displacedFinal); statErr != nil {
		t.Fatalf("cleanup double error lost displaced creator final generation: %v", statErr)
	}
	for _, path := range []string{stagingRoot, displacedStaging} {
		if removeErr := os.RemoveAll(path); removeErr != nil {
			t.Fatalf("remove deliberate private-directory replacement %s: %v", path, removeErr)
		}
	}
	assertNoTraceBundlePrivateStaging(t, dir)
}

func TestReleaseTraceBundleManifestUsesSingleAtomicPublicationThroat(t *testing.T) {
	wrapper := sourceGenerationFunctionBody(t, "standalone.go", "writeTraceBundleWithAllCoverageAndGatesAndLedger")
	if strings.Count(wrapper, "writeTraceBundleWithAllCoverageAndGatesAndLedgerOps(") != 1 {
		t.Fatalf("tracebundle wrapper regained a second publication implementation:\n%s", wrapper)
	}
	core := sourceGenerationFunctionBody(t, "standalone.go", "writeTraceBundleWithAllCoverageAndGatesAndLedgerOps")
	for _, required := range []string{
		"tracebundle.ValidateManifestBytes(ctx, body)",
		"prepareSealedConversionPublicationTargetWithLedger(path, \".codrax-tracebundle-*\", ledger)",
		"stageAndValidateTraceBundleManifest(ctx, target, body, publicationOps)",
		"publishSealedConversionFileNoReplace(ctx, target, sealedManifest, ledger)",
		"ledger.validateSealedOwnedPath(ctx, path)",
	} {
		if strings.Count(core, required) != 1 {
			t.Fatalf("tracebundle atomic publication missing singleton %q:\n%s", required, core)
		}
	}
	for _, forbidden := range []string{
		"os.OpenFile(path", "os.WriteFile(path", "ledger.recordOpenFile(path", "ledger.sealOwnedPath(path",
		"os.Rename(path", "publishConversionFileNoReplace(",
	} {
		if strings.Contains(core, forbidden) {
			t.Fatalf("tracebundle atomic publication reopened weak final-path primitive %q:\n%s", forbidden, core)
		}
	}
	assertSourceGenerationOrder(t, core,
		"tracebundle.ValidateManifestBytes(ctx, body)",
		"prepareSealedConversionPublicationTargetWithLedger(path, \".codrax-tracebundle-*\", ledger)",
		"stageAndValidateTraceBundleManifest(ctx, target, body, publicationOps)",
		"publishSealedConversionFileNoReplace(ctx, target, sealedManifest, ledger)",
		"if cleanupErr := targetCleanup(); cleanupErr != nil",
		"heldCloseErr := closeHeldSealedOwnedFiles(heldChildren)",
		"return Artifact{Type: ArtifactTraceBundle",
	)

	staging := sourceGenerationFunctionBody(t, "standalone.go", "stageAndValidateTraceBundleManifest")
	if strings.Count(staging, "publicationOps.openPrivateStaging(target.StagingPath)") != 1 ||
		strings.Contains(staging, "target.FinalPath") || strings.Contains(staging, "os.WriteFile(") {
		t.Fatalf("tracebundle staging helper is not private-path-only:\n%s", staging)
	}
	attestation := sourceGenerationFunctionBody(t, "standalone.go", "validateSealedTraceBundleManifestBody")
	for _, required := range []string{"sha256.Sum256(body)", "tracebundle.MeasureFile(ctx, file)", "sealed.identity.SameVersion(measuredIdentity)"} {
		if strings.Count(attestation, required) != 1 {
			t.Fatalf("tracebundle body attestation missing singleton %q:\n%s", required, attestation)
		}
	}
}

func writeTraceBundleAtomicFixture(ctx context.Context, input string, caveats []string, ops traceBundlePublicationOps) (Artifact, error) {
	return writeTraceBundleWithAllCoverageAndGatesAndLedgerOps(
		ctx, input, "", nil, caveats, nil, nil, nil, nil, nil, nil, ops,
	)
}

func writeTraceBundleAtomicInput(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "capture.sys")
	if err := os.WriteFile(path, []byte("input"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func traceBundleAtomicCaveats(count, payloadBytes int) []string {
	result := make([]string, count)
	for index := range result {
		prefix := fmt.Sprintf("%06d:", index)
		payload := payloadBytes - len(prefix)
		if payload < 0 {
			payload = 0
		}
		result[index] = prefix + strings.Repeat("x", payload)
	}
	return result
}

func assertTraceBundleFinalAbsent(t *testing.T, input string) {
	t.Helper()
	path := traceSidecarBase(input, "") + ".tracebundle.json"
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("failed tracebundle operation left a final publication: %v", err)
	}
}

func assertNoTraceBundlePrivateStaging(t *testing.T, dir string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, ".codrax-tracebundle-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("private tracebundle staging survived terminal cleanup: %v", matches)
	}
}

func requireTraceBundleAtomicPublisher(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("exact tracebundle publication is intentionally fail-closed on this platform")
	}
}

func awaitTraceBundleAtomicSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for tracebundle publication phase %s", name)
	}
}

func awaitTraceBundleAtomicResult[T any](t *testing.T, result <-chan T) T {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for tracebundle publication result")
		var zero T
		return zero
	}
}
