package hitraceconv

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type conversionAuthoritySpy struct {
	info            os.FileInfo
	name            string
	order           *[]string
	validates       int
	removes         int
	closes          int
	removePath      string
	removeError     error
	removeErrorOnce bool
	closeError      error
}

func descriptorFileInfo(t testing.TB, path string) os.FileInfo {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	info, statErr := file.Stat()
	closeErr := file.Close()
	if statErr != nil || closeErr != nil {
		t.Fatalf("capture descriptor-derived file info: stat=%v close=%v", statErr, closeErr)
	}
	return info
}

func (spy *conversionAuthoritySpy) Validate() (os.FileInfo, error) {
	spy.validates++
	return spy.info, nil
}

func (spy *conversionAuthoritySpy) Remove() error {
	spy.removes++
	if spy.order != nil {
		*spy.order = append(*spy.order, "remove:"+spy.name)
	}
	if spy.removeError != nil {
		err := spy.removeError
		if spy.removeErrorOnce {
			spy.removeError = nil
		}
		return err
	}
	if spy.removePath != "" {
		return os.Remove(spy.removePath)
	}
	return nil
}

func (spy *conversionAuthoritySpy) Close() error {
	spy.closes++
	if spy.order != nil {
		*spy.order = append(*spy.order, "close:"+spy.name)
	}
	return spy.closeError
}

// Cancellation after an output has been fully written and sealed still means
// the transaction did not commit. The creator ledger must remove that exact
// output while preserving the protected input.
func TestReleaseConversionFileTransactionCancellationRollsBackSealedOutput(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "input.perf")
	output := filepath.Join(dir, "output.perftrace")
	inputBody := []byte("protected input")
	if err := os.WriteFile(input, inputBody, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	err := runConversionFileTransaction(ctx, input, func(ledger *conversionFileLedger) error {
		out, err := openOwnedConversionFile(output, ledger)
		if err != nil {
			return err
		}
		body := []byte("sealed output")
		if _, err := out.Write(body); err != nil {
			return err
		}
		if _, err := finishOwnedConversionFile(output, out, ledger, true); err != nil {
			return err
		}
		cancel()
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("post-seal cancellation identity was lost: %T %v", err, err)
	}
	if _, err := os.Lstat(output); !os.IsNotExist(err) {
		t.Fatalf("sealed output survived canceled transaction: %v", err)
	}
	got, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(inputBody) {
		t.Fatalf("canceled transaction modified protected input: got=%q want=%q", got, inputBody)
	}
}

// Auto fallback may remove a failed provider's output and publish a new file at
// the same pathname. The ledger must retain the removed generation for audit
// while making the replacement creator identity authoritative.
func TestReleaseConversionFileLedgerRegistersReplacementGeneration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fallback.systrace")
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	first, err := openOwnedConversionFile(path, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.Write([]byte("first provider")); err != nil {
		t.Fatal(err)
	}
	if _, err := finishOwnedConversionFile(path, first, ledger, true); err != nil {
		t.Fatal(err)
	}
	if err := ledger.removeOwnedPath(path); err != nil {
		t.Fatal(err)
	}
	second, err := openOwnedConversionFile(path, ledger)
	if err != nil {
		t.Fatalf("replacement generation was rejected: %v", err)
	}
	if _, err := second.Write([]byte("fallback provider")); err != nil {
		t.Fatal(err)
	}
	if _, err := finishOwnedConversionFile(path, second, ledger, true); err != nil {
		t.Fatal(err)
	}
	if len(ledger.created) != 2 || !ledger.created[0].removed || ledger.created[1].removed {
		t.Fatalf("replacement generation ledger malformed: %+v", ledger.created)
	}
	if err := ledger.validateOwnedPaths(); err != nil {
		t.Fatalf("replacement generation did not validate: %v", err)
	}
	if err := ledger.cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("replacement generation survived creator cleanup: %v", err)
	}
}

func TestReleaseConversionFileLedgerAuthorityCommitAndRollbackLifecycle(t *testing.T) {
	dir := t.TempDir()
	newSpy := func(name string, order *[]string) (string, *conversionAuthoritySpy) {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
		info := descriptorFileInfo(t, path)
		return path, &conversionAuthoritySpy{info: info, name: name, order: order}
	}

	commitLedger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	commitPath, commitSpy := newSpy("commit.db", nil)
	if err := commitLedger.recordSealedAuthority(commitPath, commitSpy.info.Size(), commitSpy); err != nil {
		t.Fatal(err)
	}
	if err := commitLedger.validateOwnedPaths(); err != nil {
		t.Fatal(err)
	}
	if err := commitLedger.releaseOwnedAuthorities(); err != nil {
		t.Fatal(err)
	}
	if commitSpy.validates < 2 || commitSpy.removes != 0 || commitSpy.closes != 1 {
		t.Fatalf("commit authority lifecycle malformed: %+v", commitSpy)
	}
	if _, err := os.Lstat(commitPath); err != nil {
		t.Fatalf("committed authority file was removed: %v", err)
	}

	rollbackLedger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	order := []string{}
	companionPath, companion := newSpy("pair.db.ohos.ts", &order)
	dbPath, db := newSpy("pair.db", &order)
	if err := rollbackLedger.recordSealedAuthority(companionPath, companion.info.Size(), companion); err != nil {
		t.Fatal(err)
	}
	if err := rollbackLedger.recordSealedAuthority(dbPath, db.info.Size(), db); err != nil {
		t.Fatal(err)
	}
	if err := rollbackLedger.cleanup(); err != nil {
		t.Fatal(err)
	}
	want := []string{"remove:pair.db", "close:pair.db", "remove:pair.db.ohos.ts", "close:pair.db.ohos.ts"}
	if strings.Join(order, "|") != strings.Join(want, "|") {
		t.Fatalf("authority rollback order=%v want=%v", order, want)
	}
}

func TestReleaseConversionFileLedgerAuthorityCloseFailureIsPostCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "close-error.db")
	if err := os.WriteFile(path, []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	info := descriptorFileInfo(t, path)
	want := errors.New("injected authority close failure")
	spy := &conversionAuthoritySpy{info: info, closeError: want}
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.recordSealedAuthority(path, info.Size(), spy); err != nil {
		t.Fatal(err)
	}
	if err := ledger.releaseOwnedAuthorities(); !errors.Is(err, want) {
		t.Fatalf("authority close failure was lost: %v", err)
	}
	var releaseFailure *conversionPublicationReleaseError
	if !errors.As(ledger.releaseErr, &releaseFailure) || releaseFailure.Published != 1 ||
		len(ledger.created) != 1 || ledger.created[0].authority != spy || ledger.created[0].removed ||
		!ledger.releaseStarted || spy.closes != 1 {
		t.Fatalf("close-failed authority was discarded before rollback: record=%+v spy=%+v", ledger.created, spy)
	}
	if err := ledger.cleanup(); err != nil {
		t.Fatalf("irreversible release point attempted rollback cleanup: %v", err)
	}
	if ledger.created[0].authority != spy || ledger.created[0].removed || spy.removes != 0 || spy.closes != 1 {
		t.Fatalf("close-failed committed authority was rolled back or discarded: record=%+v spy=%+v", ledger.created[0], spy)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "db" {
		t.Fatalf("close-failed authority generation was deleted or changed: got=%q err=%v", got, err)
	}
}

func TestReleaseConversionFileLedgerAuthorityReleaseIsIrreversibleCommitPoint(t *testing.T) {
	release := sourceGenerationFunctionBody(t, "transaction.go", "releaseOwnedAuthorities")
	assertSourceGenerationOrder(t, release,
		"if l.releaseStarted",
		"l.releaseStarted = true",
		"record.authority.Close()",
		"conversionPublicationReleaseError{Published: published, Cause: result}",
	)
	cleanup := sourceGenerationFunctionBody(t, "transaction.go", "cleanup")
	releaseGate := strings.Index(cleanup, "if l.releaseStarted")
	authorityRemove := strings.Index(cleanup, "record.authority.Remove()")
	if releaseGate < 0 || authorityRemove <= releaseGate ||
		!strings.Contains(cleanup[releaseGate:authorityRemove], "return nil") {
		t.Fatalf("cleanup can rollback after authority release began: release=%d remove=%d\n%s",
			releaseGate, authorityRemove, cleanup)
	}
	remove := sourceGenerationFunctionBody(t, "transaction.go", "removeOwnedPath")
	if !strings.Contains(remove, "if l.releaseStarted") ||
		strings.Index(remove, "if l.releaseStarted") > strings.Index(remove, "record.authority.Remove()") {
		t.Fatalf("explicit rollback can cross the authority-release commit point:\n%s", remove)
	}
}

func TestReleaseConversionFileLedgerPartialAuthorityCloseNeverRollsBackCommittedSiblings(t *testing.T) {
	dir := t.TempDir()
	newAuthority := func(name string, closeErr error) (string, *conversionAuthoritySpy) {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(name), 0o600); err != nil {
			t.Fatal(err)
		}
		info := descriptorFileInfo(t, path)
		return path, &conversionAuthoritySpy{info: info, name: name, closeError: closeErr}
	}
	want := errors.New("injected first authority close failure")
	failingPath, failing := newAuthority("first.db", want)
	successPath, success := newAuthority("second.db", nil)
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	// Reverse release order closes the successful sibling first, then fails.
	if err := ledger.recordSealedAuthority(failingPath, failing.info.Size(), failing); err != nil {
		t.Fatal(err)
	}
	if err := ledger.recordSealedAuthority(successPath, success.info.Size(), success); err != nil {
		t.Fatal(err)
	}
	if err := ledger.releaseOwnedAuthorities(); !errors.Is(err, want) {
		t.Fatalf("partial authority release failure was lost: %v", err)
	}
	if ledger.created[0].authority != failing || ledger.created[1].authority != nil ||
		failing.closes != 1 || success.closes != 1 || failing.removes != 0 || success.removes != 0 {
		t.Fatalf("partial release state drifted: ledger=%+v failing=%+v success=%+v", ledger.created, failing, success)
	}
	if err := ledger.cleanup(); err != nil {
		t.Fatalf("partial release crossed the irreversible commit boundary: %v", err)
	}
	for _, path := range []string{failingPath, successPath} {
		if body, err := os.ReadFile(path); err != nil || len(body) == 0 {
			t.Fatalf("committed sibling was removed after partial release %q: bytes=%d err=%v", path, len(body), err)
		}
	}
	if failing.removes != 0 || success.removes != 0 {
		t.Fatalf("partial release attempted impossible rollback: failing=%+v success=%+v", failing, success)
	}
}

func TestReleaseConversionFileLedgerAuthorityRemoveFailureRemainsRetryable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "remove-retry.db")
	if err := os.WriteFile(path, []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	info := descriptorFileInfo(t, path)
	want := errors.New("injected authority remove failure")
	spy := &conversionAuthoritySpy{info: info, removePath: path, removeError: want}
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.recordSealedAuthority(path, info.Size(), spy); err != nil {
		t.Fatal(err)
	}
	if err := ledger.removeOwnedPath(path); !errors.Is(err, want) {
		t.Fatalf("authority remove failure was lost: %v", err)
	}
	if ledger.created[0].authority != spy || ledger.created[0].removed || spy.removes != 1 || spy.closes != 0 {
		t.Fatalf("remove-failed authority was closed or discarded: record=%+v spy=%+v", ledger.created[0], spy)
	}
	spy.removeError = nil
	if err := ledger.cleanup(); err != nil {
		t.Fatalf("retry held-authority rollback: %v", err)
	}
	if !ledger.created[0].removed || ledger.created[0].authority != nil || spy.removes != 2 || spy.closes != 1 {
		t.Fatalf("retried authority rollback did not close: record=%+v spy=%+v", ledger.created[0], spy)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("retried authority rollback left public generation: %v", err)
	}
}

func TestReleaseConversionFileLedgerCleanupRetriesTransientAuthorityRemove(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cleanup-remove-retry.db")
	if err := os.WriteFile(path, []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	info := descriptorFileInfo(t, path)
	spy := &conversionAuthoritySpy{
		info: info, removePath: path,
		removeError: errors.New("injected transient remove failure"), removeErrorOnce: true,
	}
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	if err := ledger.recordSealedAuthority(path, info.Size(), spy); err != nil {
		t.Fatal(err)
	}
	if err := ledger.cleanup(); err != nil {
		t.Fatalf("transient authority rollback was not recovered: %v", err)
	}
	if !ledger.created[0].removed || ledger.created[0].authority != nil || spy.removes != 2 || spy.closes != 1 {
		t.Fatalf("transient authority rollback lifecycle drifted: record=%+v spy=%+v", ledger.created[0], spy)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("transient authority rollback left public generation: %v", err)
	}
}
