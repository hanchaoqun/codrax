//go:build windows

package repl

// raiseSelfSIGINT: no direct self-SIGINT on Windows; the caller falls
// back to invoking the cancel surface directly (single-tap semantics
// only — disclosed limitation in the design ledger).
func raiseSelfSIGINT() bool { return false }
