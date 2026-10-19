package traceinput

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hanchaoqun/codrax/internal/filegeneration"
)

// A named input needs search permission, not parent enumeration permission.
// Only a native directory-entry spelling witness may replace enumeration; a
// lexical fallback or inode-only cache key would let case aliases bypass a
// stale receipt or merge independent hardlink names.
func preparationSearchOnlyDirectoryEntry(ctx context.Context, parent, name string, wanted os.FileInfo) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	parentInfo, err := os.Stat(parent)
	if err != nil {
		return "", err
	}
	requested := filepath.Join(parent, name)
	entry, err := preparationNativeEntryName(requested, wanted)
	if err != nil {
		return "", fmt.Errorf("trace source identity cannot resolve search-only parent %q: %w", parent, err)
	}
	if entry == "" || entry == "." || entry == ".." || filepath.Base(entry) != entry || strings.ContainsRune(entry, '\x00') {
		return "", fmt.Errorf("trace native entry name is invalid for %q", requested)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	currentParent, err := os.Stat(parent)
	if err != nil {
		return "", err
	}
	current, err := os.Lstat(filepath.Join(parent, entry))
	if err != nil {
		return "", err
	}
	selected, err := os.Lstat(requested)
	if err != nil {
		return "", err
	}
	version := filegeneration.FromInfo(wanted)
	if !version.Strong() || !os.SameFile(parentInfo, currentParent) || !version.MatchesInfo(current) || !version.MatchesInfo(selected) {
		return "", fmt.Errorf("trace directory entry changed during search-only path resolution: %q", requested)
	}
	return entry, nil
}
