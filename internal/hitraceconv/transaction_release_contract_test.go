package hitraceconv

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

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
