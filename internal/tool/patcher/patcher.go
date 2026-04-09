// Package patcher provides safe, targeted file modifications bound to a
// workspace root. It supports four ops — write, edit, insert_before,
// insert_after — with EOL preservation, idempotency detection, binary-file
// protection, atomic writes, and unified diff previews.
package patcher

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Op names one of the supported patch operations.
type Op string

const (
	OpWrite        Op = "write"         // replace entire file content
	OpEdit         Op = "edit"          // replace exact-match substring
	OpInsertBefore Op = "insert_before" // insert before an anchor substring
	OpInsertAfter  Op = "insert_after"  // insert after an anchor substring
)

// Status describes the outcome of an Apply call.
type Status string

const (
	StatusApplied        Status = "applied"
	StatusAlreadyApplied Status = "already_applied"
	StatusNoMatch        Status = "no_match"
	StatusAmbiguous      Status = "ambiguous_match"
	StatusUnchanged      Status = "unchanged"
)

// Request describes a single patch to apply.
type Request struct {
	Path      string // absolute or relative to Patcher root
	Op        Op
	OldString string // anchor for edit/insert_*
	NewString string // replacement (edit) or inserted content (insert_*)
	Content   string // full content for write
	DryRun    bool   // compute result but do not write
}

// Result carries the outcome plus enough context for callers to report or
// preview the change.
type Result struct {
	Path         string
	Op           Op
	Status       Status
	Message      string
	MatchesFound int
	Before       string // file content before change (empty if file didn't exist)
	After        string // file content after change (what's on disk, or would be)
	DiffPreview  string // unified-style preview with context lines
	EOL          string // detected line ending ("\n" or "\r\n")
}

// Patcher applies patches to files confined within a workspace root.
// All paths in requests are resolved relative to root and validated to
// ensure they cannot escape it.
type Patcher struct {
	root string // absolute, cleaned
}

// New constructs a Patcher for the given workspace root.
func New(root string) (*Patcher, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("workspace root is required")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve root: %w", err)
	}
	return &Patcher{root: filepath.Clean(abs)}, nil
}

// Root returns the absolute workspace root.
func (p *Patcher) Root() string { return p.root }

// Apply performs the requested patch and returns a Result describing what
// happened (or would happen, in dry-run mode). It returns an error only for
// truly exceptional conditions: invalid inputs, missing files where
// required, binary files, I/O failures, or path-safety violations. Logical
// non-applies such as no_match and ambiguous_match are carried in the
// Result's Status field, not returned as errors.
func (p *Patcher) Apply(ctx context.Context, req Request) (*Result, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if err := validateOp(req.Op); err != nil {
		return nil, err
	}

	absPath, err := p.resolvePath(req.Path)
	if err != nil {
		return nil, err
	}

	rawBefore, exists, mode, err := readFileIfExists(absPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", req.Path, err)
	}

	if req.Op != OpWrite && !exists {
		return nil, fmt.Errorf("file not found: %s", req.Path)
	}
	if exists && req.Op != OpWrite && isBinary(rawBefore) {
		return nil, fmt.Errorf("refusing to edit binary file: %s", req.Path)
	}

	// Work on \n-normalized text and restore original EOL on write.
	eol := detectEOL(rawBefore)
	normBefore := normalizeEOL(rawBefore, eol)

	normOld := normalizeEOL(req.OldString, detectEOL(req.OldString))
	normNew := normalizeEOL(req.NewString, detectEOL(req.NewString))
	normContent := normalizeEOL(req.Content, detectEOL(req.Content))

	var (
		normAfter string
		status    Status
		message   string
		matches   int
	)

	switch req.Op {
	case OpWrite:
		normAfter, status, message = applyWrite(normBefore, exists, normContent)
	case OpEdit:
		normAfter, status, message, matches, err = applyEdit(normBefore, normOld, normNew)
	case OpInsertBefore:
		normAfter, status, message, matches, err = applyInsert(normBefore, normOld, normNew, true)
	case OpInsertAfter:
		normAfter, status, message, matches, err = applyInsert(normBefore, normOld, normNew, false)
	}
	if err != nil {
		return nil, err
	}

	rawAfter := denormalizeEOL(normAfter, eol)

	result := &Result{
		Path:         req.Path,
		Op:           req.Op,
		Status:       status,
		Message:      message,
		MatchesFound: matches,
		Before:       rawBefore,
		After:        rawAfter,
		EOL:          eol,
	}

	if status == StatusApplied && !req.DryRun {
		if err := atomicWriteFile(absPath, rawAfter, mode); err != nil {
			return nil, fmt.Errorf("write %s: %w", req.Path, err)
		}
	}

	if status == StatusApplied || status == StatusUnchanged {
		result.DiffPreview = generateDiff(normBefore, normAfter, 3)
	}

	return result, nil
}

func validateOp(op Op) error {
	switch op {
	case OpWrite, OpEdit, OpInsertBefore, OpInsertAfter:
		return nil
	default:
		return fmt.Errorf("unsupported op: %q", op)
	}
}

// resolvePath turns req.Path into an absolute path and verifies it stays
// within the workspace root. Both relative and absolute inputs are accepted.
func (p *Patcher) resolvePath(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", errors.New("path is required")
	}

	var abs string
	if filepath.IsAbs(path) {
		abs = filepath.Clean(path)
	} else {
		abs = filepath.Clean(filepath.Join(p.root, path))
	}

	rel, err := filepath.Rel(p.root, abs)
	if err != nil {
		return "", fmt.Errorf("resolve path %q: %w", path, err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path outside workspace root: %s", path)
	}

	return abs, nil
}
