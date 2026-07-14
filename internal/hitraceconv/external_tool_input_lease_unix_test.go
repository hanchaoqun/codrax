//go:build unix

package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestReleaseExternalToolInputLeaseRejectsTransientReplacementAndUsesSnapshotAfterABA(t *testing.T) {
	parent := t.TempDir()
	inputPath := filepath.Join(parent, "input.trace")
	originalPath := filepath.Join(parent, "original.trace")
	decoyPath := filepath.Join(parent, "decoy.trace")
	want := []byte("original-generation\n")
	if err := os.WriteFile(originalPath, want, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(decoyPath, []byte("decoy-generation---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(originalPath, inputPath); err != nil {
		t.Skipf("symlink ABA fixture unavailable: %v", err)
	}
	authority, err := openConversionInputAuthority(inputPath)
	if unavailableConversionInputAuthority(t, err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	staging, err := newPrivateConversionDir(parent, ".external-tool-lease-*")
	if err != nil {
		t.Fatal(err)
	}
	defer staging.FinalizeCleanup()
	lease, err := newExternalToolInputLease(
		context.Background(), authority, staging, "input.snapshot", externalToolInputSnapshotOnly,
	)
	if err != nil {
		t.Fatal(err)
	}

	replaceInputSymlink := func(target string) {
		t.Helper()
		next := inputPath + ".next"
		if err := os.Symlink(target, next); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(next, inputPath); err != nil {
			t.Fatal(err)
		}
	}
	replaceInputSymlink(decoyPath)
	if _, err := lease.Command(context.Background(), "/bin/cat", nil, nil); conversionInputErrorCode(err) != ConversionInputCodeGenerationChanged {
		t.Fatalf("public replacement reached command construction: %v", err)
	}
	replaceInputSymlink(originalPath)
	cmd, err := lease.Command(context.Background(), "/bin/cat", nil, nil)
	if err != nil {
		t.Fatalf("restored namespace ABA did not recover through the exact snapshot: %v", err)
	}
	var got bytes.Buffer
	cmd.Stdout = &got
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.Bytes(), want) {
		t.Fatalf("external tool consumed namespace decoy: got=%q want=%q", got.Bytes(), want)
	}
	if err := finishExternalToolCommand(context.Background(), lease, staging, nil); err != nil {
		t.Fatalf("restored namespace ABA failed exact snapshot boundary: %v", err)
	}
}

func TestReleaseExternalToolInputLeaseRejectsPublicPathOutsideOwnedSlot(t *testing.T) {
	parent := t.TempDir()
	inputPath := filepath.Join(parent, "input.trace")
	if err := os.WriteFile(inputPath, []byte("trace\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	authority, err := openConversionInputAuthority(inputPath)
	if unavailableConversionInputAuthority(t, err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	staging, err := newPrivateConversionDir(parent, ".external-tool-argv-*")
	if err != nil {
		t.Fatal(err)
	}
	defer staging.FinalizeCleanup()
	lease, err := newExternalToolInputLease(
		context.Background(), authority, staging, "input.snapshot", externalToolInputSnapshotOnly,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if _, err := lease.Command(context.Background(), "/bin/cat", []string{inputPath}, nil); conversionInputErrorCode(err) != ConversionInputCodeInternalContract {
		t.Fatalf("public input path bypass was not rejected: %v", err)
	}
	cmd, err := lease.Command(context.Background(), "/bin/cat", nil, nil)
	if err != nil {
		t.Fatalf("rejected argv consumed the one-command capability: %v", err)
	}
	if len(cmd.Args) != 2 || sameConversionCanonicalPath(cmd.Args[1], inputPath) || cmd.Args[1] != lease.snapshot.path {
		t.Fatalf("command did not receive the lease-owned snapshot: %#v", cmd.Args)
	}
	if _, err := lease.Command(context.Background(), "/bin/cat", nil, nil); err == nil {
		t.Fatal("one lease built more than one command")
	}
	var got bytes.Buffer
	cmd.Stdout = &got
	if err := cmd.Run(); err != nil {
		t.Fatalf("snapshot reader failed: %v", err)
	}
	if string(got.Bytes()) != "trace\n" {
		t.Fatalf("external tool did not read the exact private snapshot: %q", got.Bytes())
	}
	if err := finishExternalToolCommand(context.Background(), lease, staging, nil); err != nil {
		t.Fatalf("stable external tool transaction failed: %v", err)
	}
}

func TestReleaseExternalToolInputLeasePostCommandGatesSourceAndSnapshotMutation(t *testing.T) {
	for _, mutateSnapshot := range []bool{false, true} {
		name := "source"
		if mutateSnapshot {
			name = "snapshot"
		}
		t.Run(name, func(t *testing.T) {
			parent := t.TempDir()
			inputPath := filepath.Join(parent, "input.trace")
			body := []byte("first-generation\n")
			if err := os.WriteFile(inputPath, body, 0o600); err != nil {
				t.Fatal(err)
			}
			authority, err := openConversionInputAuthority(inputPath)
			if unavailableConversionInputAuthority(t, err) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer authority.Close()
			staging, err := newPrivateConversionDir(parent, ".external-tool-mutation-*")
			if err != nil {
				t.Fatal(err)
			}
			defer staging.FinalizeCleanup()
			lease, err := newExternalToolInputLease(
				context.Background(), authority, staging, "input.snapshot", externalToolInputSnapshotOnly,
			)
			if err != nil {
				t.Fatal(err)
			}
			cmd, err := lease.Command(context.Background(), "/bin/cat", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			mutationPath := inputPath
			if mutateSnapshot {
				mutationPath = lease.snapshot.path
			}
			before, err := os.Stat(mutationPath)
			if err != nil {
				t.Fatal(err)
			}
			time.Sleep(2 * time.Millisecond)
			if err := os.WriteFile(mutationPath, []byte("other-generation\n"), before.Mode().Perm()); err != nil {
				t.Fatal(err)
			}
			if err := os.Chtimes(mutationPath, before.ModTime(), before.ModTime()); err != nil {
				t.Fatal(err)
			}
			runErr := cmd.Run()
			if runErr != nil {
				t.Fatalf("external tool fixture failed: %v", runErr)
			}
			err = finishExternalToolCommand(context.Background(), lease, staging, nil)
			if !externalToolInputLeaseHasGenerationError(err) {
				t.Fatalf("post-command %s mutation was not typed/fail-closed: %v", name, err)
			}
		})
	}
}

func TestReleaseExternalToolInputLeaseBoundaryErrorPolicy(t *testing.T) {
	parent := t.TempDir()
	inputPath := filepath.Join(parent, "input.trace")
	if err := os.WriteFile(inputPath, []byte("trace\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	newLease := func(t *testing.T) (*conversionInputAuthority, *privateConversionDir, *externalToolInputLease) {
		t.Helper()
		authority, err := openConversionInputAuthority(inputPath)
		if err != nil {
			t.Fatal(err)
		}
		staging, err := newPrivateConversionDir(parent, ".external-tool-boundary-*")
		if err != nil {
			_ = authority.Close()
			t.Fatal(err)
		}
		lease, err := newExternalToolInputLease(
			context.Background(), authority, staging, "input.snapshot", externalToolInputSnapshotOnly,
		)
		if err != nil {
			_ = staging.FinalizeCleanup()
			_ = authority.Close()
			t.Fatal(err)
		}
		return authority, staging, lease
	}

	authority, staging, lease := newLease(t)
	childErr := errors.New("adapter exit 23")
	if err := finishExternalToolCommand(context.Background(), lease, staging, childErr); err != nil {
		t.Fatalf("child-only failure did not stay on the provider fallback lane: %v", err)
	}
	if err := staging.FinalizeCleanup(); err != nil {
		t.Fatal(err)
	}
	if err := authority.Close(); err != nil {
		t.Fatal(err)
	}

	authority, staging, lease = newLease(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := finishExternalToolCommand(ctx, lease, staging, childErr)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, childErr) {
		t.Fatalf("hard boundary lost cancellation or child evidence: %v", err)
	}
	if err := staging.FinalizeCleanup(); err != nil {
		t.Fatal(err)
	}
	if err := authority.Close(); err != nil {
		t.Fatal(err)
	}

	authority, staging, lease = newLease(t)
	if err := os.Chmod(staging.Path(), 0o755); err != nil {
		t.Fatal(err)
	}
	err = finishExternalToolCommand(context.Background(), lease, staging, childErr)
	if !errors.Is(err, errPrivateConversionDirSecurityInvalid) || !errors.Is(err, childErr) {
		t.Fatalf("hard boundary lost staging-security or child evidence: %v", err)
	}
	if err := staging.FinalizeCleanup(); err != nil {
		t.Fatalf("security-drift cleanup failed: %v", err)
	}
	if err := authority.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseExternalToolInputLeaseCancellationAndCopyGenerationGate(t *testing.T) {
	for _, changed := range []bool{false, true} {
		name := "cancel"
		if changed {
			name = "generation"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			view := &externalToolGateTestView{
				body:    bytes.Repeat([]byte("x"), 2*64*1024),
				cancel:  cancel,
				changed: changed,
			}
			staging, err := newPrivateConversionDir(t.TempDir(), ".external-tool-copy-*")
			if err != nil {
				t.Fatal(err)
			}
			lease, err := newExternalToolInputLease(ctx, view, staging, "input.snapshot", externalToolInputSnapshotOnly)
			if lease != nil {
				t.Fatal("failed copy returned a live lease")
			}
			if changed {
				if !externalToolInputLeaseHasGenerationError(err) {
					t.Fatalf("copy-time source mutation lost typed generation error: %v", err)
				}
			} else if !errors.Is(err, context.Canceled) {
				t.Fatalf("copy-time cancellation was lost: %v", err)
			}
			if err := staging.FinalizeCleanup(); err != nil {
				t.Fatalf("failed copy leaked an open staging child: %v", err)
			}
		})
	}
}

type externalToolGateTestView struct {
	body    []byte
	cancel  context.CancelFunc
	changed bool
	read    bool
}

func (view *externalToolGateTestView) ReadAt(buffer []byte, offset int64) (int, error) {
	if view.changed && view.read {
		return 0, io.ErrUnexpectedEOF
	}
	if offset >= int64(len(view.body)) {
		return 0, io.EOF
	}
	n := copy(buffer, view.body[offset:])
	if !view.read {
		view.read = true
		if view.changed {
			// Validate observes the precise generation transition at the common
			// all-exits gate instead of leaking a copy/EOF surrogate.
		} else {
			view.cancel()
		}
	}
	if n < len(buffer) {
		return n, io.EOF
	}
	return n, nil
}

func (view *externalToolGateTestView) Size() int64         { return int64(len(view.body)) }
func (view *externalToolGateTestView) DisplayPath() string { return "fixture://external-tool-input" }
func (view *externalToolGateTestView) Validate(stage conversionInputStage) error {
	if view.changed && view.read {
		return conversionInputFailure(
			ConversionInputCodeGenerationChanged,
			stage,
			view.DisplayPath(),
			errors.New("fixture generation changed"),
		)
	}
	return nil
}

func externalToolInputLeaseHasGenerationError(err error) bool {
	var typed *ConversionInputError
	return errors.As(err, &typed) && typed.Code == ConversionInputCodeGenerationChanged &&
		typed.Stage == conversionInputStageExternalTool.String()
}
