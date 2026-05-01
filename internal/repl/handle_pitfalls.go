package repl

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// FailureTaxonomyReader is the REPL-side interface to the
// stage-3 Failure Taxonomy cache. The REPL only inspects
// (list / show / clear); the orchestrator owns Append at
// reflector-emit time. internal/orchestrator's
// FailureTaxonomyStore satisfies this interface.
type FailureTaxonomyReader interface {
	// All returns a snapshot of every persisted pattern,
	// newest LastSeen first. Empty when the cache is fresh
	// or the feature is disabled.
	All() []types.FailurePattern

	// Clear wipes the cache file + zeroes the in-memory
	// copy. Idempotent — missing file is success.
	Clear() error
}

// handlePitfallsCmd services the /pitfalls slash command
// family (stage 3 inspection). Recognised forms:
//
//	/pitfalls               — alias of /pitfalls list
//	/pitfalls list          — enumerate every persisted pattern (newest first)
//	/pitfalls show <id>     — show full description + trigger + consequence
//	/pitfalls clear         — wipe the per-repo cache (interactive y/N)
//
// Read-only by default; clear requires interactive confirm
// since the operator typed the destructive verb.
func (r *REPL) handlePitfallsCmd(line string) {
	if r.failureTaxonomy == nil {
		r.info("/pitfalls: failure taxonomy is disabled (cache_dir not set, or pipeline_failure_taxonomy_enabled=false)\n")
		return
	}
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/pitfalls"))
	if rest == "" {
		rest = "list"
	}
	if showID := strings.TrimSpace(strings.TrimPrefix(rest, "show ")); showID != rest && showID != "" {
		r.pitfallsShow(showID)
		return
	}
	switch rest {
	case "list":
		r.pitfallsList()
	case "clear":
		r.pitfallsClear()
	default:
		r.warn("unknown /pitfalls subcommand %q. expected: list / show <id> / clear\n", rest)
	}
}

// pitfallsList enumerates every cached pattern (newest first).
// Each row shows id + name + hits + confidence so the operator
// can spot at-a-glance which patterns are recurring vs one-offs.
func (r *REPL) pitfallsList() {
	all := r.failureTaxonomy.All()
	if len(all) == 0 {
		r.info("/pitfalls list: no patterns recorded for this repo yet\n")
		return
	}
	fmt.Fprintf(r.out, "  %d pattern(s) recorded for this repo (newest first):\n", len(all))
	for _, p := range all {
		fmt.Fprintf(r.out, "    %s  hits=%d  confidence=%.2f  last_seen=%s\n",
			p.ID, p.HitCount, p.Confidence, p.LastSeen.Format("2006-01-02"))
		fmt.Fprintf(r.out, "      %s\n", oneLine(p.Name))
	}
}

// pitfallsShow renders the full description + trigger +
// consequence + scope for one pattern by ID. Useful when
// /pitfalls list shows a name that isn't self-explanatory.
func (r *REPL) pitfallsShow(id string) {
	all := r.failureTaxonomy.All()
	for _, p := range all {
		if p.ID == id {
			fmt.Fprintf(r.out, "  id: %s\n", p.ID)
			fmt.Fprintf(r.out, "  name: %s\n", oneLine(p.Name))
			fmt.Fprintf(r.out, "  description: %s\n", oneLine(p.Description))
			fmt.Fprintf(r.out, "  trigger: %s\n", oneLine(p.Trigger))
			if p.Consequence != "" {
				fmt.Fprintf(r.out, "  consequence: %s\n", oneLine(p.Consequence))
			}
			fmt.Fprintf(r.out, "  hits: %d  confidence: %.2f\n", p.HitCount, p.Confidence)
			fmt.Fprintf(r.out, "  first_seen: %s  last_seen: %s\n",
				p.FirstSeen.Format("2006-01-02 15:04"),
				p.LastSeen.Format("2006-01-02 15:04"))
			if p.ExampleFile != "" {
				fmt.Fprintf(r.out, "  example: %s", p.ExampleFile)
				if p.ExampleLine > 0 {
					fmt.Fprintf(r.out, ":%d", p.ExampleLine)
				}
				fmt.Fprintln(r.out)
			}
			if len(p.AppliesToKinds) > 0 {
				fmt.Fprintf(r.out, "  applies_to_kinds: %s\n", strings.Join(p.AppliesToKinds, ", "))
			}
			if len(p.AppliesToPaths) > 0 {
				fmt.Fprintf(r.out, "  applies_to_paths: %s\n", strings.Join(p.AppliesToPaths, ", "))
			}
			return
		}
	}
	r.warn("/pitfalls show: no pattern with id %q\n", id)
}

// pitfallsClear wipes the cache. The destructive verb the
// operator typed is itself the confirmation; no extra prompt
// to keep the call site terse (mirrors /baseline clear which
// also wipes without re-prompt).
func (r *REPL) pitfallsClear() {
	if err := r.failureTaxonomy.Clear(); err != nil {
		r.errorf("/pitfalls clear: %v\n", err)
		return
	}
	r.success("/pitfalls clear: cache wiped\n")
}
