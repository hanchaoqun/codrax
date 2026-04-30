package repl

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// handleBaselineCmd services the /baseline subcommands. Operator
// surface for the baseline disk cache (commit 2 P0 A1 + commit 11
// race-skip): inspect what's cached, clear stale entries, show
// the report content for one SHA.
//
// Recognised forms:
//
//	/baseline               — alias of /baseline list
//	/baseline list          — enumerate cached SHAs (newest mtime first)
//	/baseline show <sha>    — print the cached ChangeReport for the
//	                          named SHA (8-char prefix or full)
//	/baseline clear         — wipe every cached entry (no confirmation
//	                          because the cache is regenerable)
//
// All file I/O goes against <RuntimeAnchor>/baselines/ — same dir
// the BaselineCache writes to. Doesn't take a lock because the
// commands are read-mostly and single-process; the rare collision
// with an in-flight Store collapses to "I see a stale snapshot"
// which is acceptable for an admin command.
func (r *REPL) handleBaselineCmd(line string) {
	if r.runtimeAnchor == "" {
		r.warn("/baseline disabled: no runtime anchor configured\n")
		return
	}
	dir := filepath.Join(r.runtimeAnchor, "baselines")
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/baseline"))
	if rest == "" {
		rest = "list"
	}

	if showSHA := strings.TrimSpace(strings.TrimPrefix(rest, "show ")); showSHA != rest && showSHA != "" {
		r.baselineShow(dir, showSHA)
		return
	}

	switch rest {
	case "list":
		r.baselineList(dir)
	case "clear":
		r.baselineClear(dir)
	default:
		r.warn("unknown /baseline subcommand %q. expected: list / show <sha> / clear\n", rest)
	}
}

// baselineList enumerates cache entries sorted newest-first.
func (r *REPL) baselineList(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			r.info("/baseline list: cache dir does not exist yet (no captures recorded)\n")
			return
		}
		r.errorf("/baseline list: read %s: %v\n", dir, err)
		return
	}
	type row struct {
		name  string
		size  int64
		mtime time.Time
	}
	rows := make([]row, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		rows = append(rows, row{name: e.Name(), size: info.Size(), mtime: info.ModTime()})
	}
	if len(rows) == 0 {
		r.info(fmt.Sprintf("/baseline list: 0 cached entries in %s\n", dir))
		return
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].mtime.After(rows[j].mtime) })
	fmt.Fprintf(r.out, "  %d cached baseline(s) in %s (newest first):\n", len(rows), dir)
	for _, row := range rows {
		shaPrefix := strings.TrimSuffix(row.name, ".json")
		ts := row.mtime.Format("2006-01-02 15:04:05")
		fmt.Fprintf(r.out, "    - [%s] sha=%s  (%d bytes)\n", ts, shaPrefix, row.size)
	}
}

// baselineShow prints the cached ChangeReport for a SHA prefix.
// Accepts both 8-char prefixes (typical "first 8 of git rev-parse")
// and the 16-char on-disk filename prefix (the cache's actual key).
func (r *REPL) baselineShow(dir, sha string) {
	sha = strings.TrimSpace(sha)
	if sha == "" {
		r.warn("/baseline show: missing SHA argument\n")
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		r.errorf("/baseline show: read %s: %v\n", dir, err)
		return
	}
	var match string
	for _, e := range entries {
		name := strings.TrimSuffix(e.Name(), ".json")
		if strings.HasPrefix(name, sha) || strings.HasPrefix(sha, name) {
			match = e.Name()
			break
		}
	}
	if match == "" {
		r.info(fmt.Sprintf("/baseline show: no cached entry matches sha=%s\n", sha))
		return
	}
	data, err := os.ReadFile(filepath.Join(dir, match))
	if err != nil {
		r.errorf("/baseline show: read %s: %v\n", match, err)
		return
	}
	var report types.ChangeReport
	if err := json.Unmarshal(data, &report); err != nil {
		r.errorf("/baseline show: parse %s: %v (file likely corrupt; consider /baseline clear)\n", match, err)
		return
	}
	fmt.Fprintf(r.out, "  cache entry: %s\n", match)
	fmt.Fprintf(r.out, "    plan_id:     %s\n", report.PlanID)
	fmt.Fprintf(r.out, "    passed:      %v\n", report.Passed)
	fmt.Fprintf(r.out, "    test_count:  %d\n", len(report.TestResults))
	if report.FailureSummary != "" {
		fmt.Fprintf(r.out, "    failure_summary: %s\n", oneLine(report.FailureSummary))
	}
	if len(report.NoTestsRunners) > 0 {
		fmt.Fprintf(r.out, "    no_tests_runners: %s\n", strings.Join(report.NoTestsRunners, ", "))
	}
}

// baselineClear wipes every entry. No interactive confirmation
// because the cache is fully regenerable — worst case is one
// extra full test-suite run on the next apply, no lasting data
// loss.
func (r *REPL) baselineClear(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			r.info("/baseline clear: cache dir does not exist (nothing to clear)\n")
			return
		}
		r.errorf("/baseline clear: read %s: %v\n", dir, err)
		return
	}
	removed := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		if err := os.Remove(path); err != nil {
			r.warn("/baseline clear: remove %s: %v\n", e.Name(), err)
			continue
		}
		removed++
	}
	r.success(fmt.Sprintf("/baseline clear: removed %d cached entrie(s)\n", removed))
}
