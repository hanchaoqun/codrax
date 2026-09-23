package types

// RecordFileTotalLines stores the total line count for a file as
// observed in typed read coverage. Subsequent observations win when
// they are larger (defensive against partial totals mid-pagination).
// Zero is an explicitly observed empty file; callers must omit unknown totals.
// Negative inputs are dropped. Accumulated positive observations are retained:
// an empty observation cannot erase previously read positive lines.
func (c *EvidenceClosure) RecordFileTotalLines(file string, total int) {
	if c == nil || total < 0 {
		return
	}
	file = c.canonicalize(file)
	if file == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if total == 0 && len(c.readRanges[file]) > 0 {
		return
	}
	if c.fileTotalLines == nil {
		c.fileTotalLines = make(map[string]int)
	}
	if cur, known := c.fileTotalLines[file]; known && cur >= total {
		return
	}
	c.fileTotalLines[file] = total
}

// SetFileTotalLines atomically replaces the entire totals map.
// Mirrors SetReadRanges: explorer.ParseOutput refreshes the snapshot
// per dispatch from the latest extractFileCoverage walk.
func (c *EvidenceClosure) SetFileTotalLines(totals map[string]int) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.fileTotalLines = make(map[string]int, len(totals))
	for file, total := range totals {
		if total < 0 {
			continue
		}
		file = c.canonicalize(file)
		if file == "" {
			continue
		}
		if total == 0 && len(c.readRanges[file]) > 0 {
			continue
		}
		// Keep largest value when canonicalisation collapses two keys.
		if cur, known := c.fileTotalLines[file]; known && cur >= total {
			continue
		}
		c.fileTotalLines[file] = total
	}
}

// FileTotalLines returns the recorded total line count for a file,
// or 0 for an empty file or an unknown total. Exact coverage methods retain
// map presence to distinguish those two states.
func (c *EvidenceClosure) FileTotalLines(file string) int {
	total, _ := c.knownFileTotalLines(file)
	return total
}

// A present zero is a producer-observed empty file; absent is unknown. Keep
// the public integer getter compatible while exact coverage retains presence.
func (c *EvidenceClosure) knownFileTotalLines(file string) (int, bool) {
	if c == nil || file == "" {
		return 0, false
	}
	file = c.canonicalize(file)
	if file == "" {
		return 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	total, known := c.fileTotalLines[file]
	return total, known
}

// Cumulative reads describe observed bytes, not a file-version replacement.
// A later positive range with unknown total disproves an earlier empty-file
// total, but does not supply a new whole-file denominator.
func (c *EvidenceClosure) forgetEmptyTotalWithObservedLinesLocked(file string) {
	if total, known := c.fileTotalLines[file]; known && total == 0 && len(c.readRanges[file]) > 0 {
		delete(c.fileTotalLines, file)
	}
}

// MergedReadLines returns the sum of (end - start + 1) over the
// merged read ranges for a file. Zero when the file is unread or
// has no recorded ranges.
func (c *EvidenceClosure) MergedReadLines(file string) int {
	if c == nil || file == "" {
		return 0
	}
	file = c.canonicalize(file)
	if file == "" {
		return 0
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	sum := 0
	for _, r := range c.readRanges[file] {
		if r.End >= r.Start {
			sum += r.End - r.Start + 1
		}
	}
	return sum
}

// CoverageRatio returns merged_read_lines / total_lines for a file,
// in [0, 1]. Returns -1 when the total is unknown so callers can
// distinguish "ratio too low" from "ratio cannot be computed". A
// ratio >= 1 (e.g. ranges merged to cover all lines) is clamped to
// 1.0 — the gate semantics are "fully covered or not", not "over
// covered".
func (c *EvidenceClosure) CoverageRatio(file string) float64 {
	total, known := c.knownFileTotalLines(file)
	if !known {
		return -1
	}
	if total == 0 {
		if c.HasRead(file) {
			return 1
		}
		return 0
	}
	read := c.MergedReadLines(file)
	if read <= 0 {
		return 0
	}
	r := float64(read) / float64(total)
	if r > 1.0 {
		return 1.0
	}
	return r
}

// HasFullyRead reports whether merged ranges cover the entire file.
// Exact coverage contract:
//
//  1. total known (FileTotalLines > 0): true iff merged_read_lines
//     >= total_lines (the typed read ranges already merge to that).
//  2. total unknown: false. Without a denominator we cannot prove
//     "fully read"; conservative answer prevents the parity gate from
//     short-circuiting on an undersampled file.
//  3. known empty: true only when the file was actually read; no positive
//     line becomes covered by that file-level observation.
//
// Used by the multi-path symbol-anchored verification check (and any
// future per-file coverage gate) to bypass the comparison entirely
// when a file is already fully covered — historically also the fix
// for the 113-line file being asked to cover 205 lines bug, before
// the per-file-ratio floor itself was retired in favour of symbol-
// region verification.
func (c *EvidenceClosure) HasFullyRead(file string) bool {
	total, known := c.knownFileTotalLines(file)
	if !known {
		return false
	}
	if total == 0 {
		return c.HasRead(file)
	}
	return c.MergedReadLines(file) >= total
}
