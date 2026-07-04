package dataquery

// action_artifact_read_bounds.go — artifact-file ranking + oversize refusal
// context for the typed data-action lane, split out of action_runner.go
// (DQA LOC ratchet: the god-file may only shrink; new bounded-read concern
// code lives here).

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tool/width"
)

// annotateInternalArtifactOversize adds lane context when an oversize
// refusal names a system-generated artifact file: the action input alias
// resolved through r.artifactFiles to a runner-written temp path the user
// never typed, so without this note the refusal message reads like a
// complaint about an oversized user input file that does not exist in the
// workspace (DQA F4). Only the oversize refusal is annotated — every other
// error passes through untouched — and the wrap uses %w so the typed
// *width.ErrSourceReadOversized stays visible to errors.As
// (classifyExecutionFailureLeaf → source_oversized).
func (r ActionRunner) annotateInternalArtifactOversize(err error, alias, abs string) error {
	if err == nil || r.artifactFiles == nil {
		return err
	}
	var oversize *width.ErrSourceReadOversized
	if !errors.As(err, &oversize) {
		return err
	}
	if strings.TrimSpace(r.artifactFiles[alias]) != strings.TrimSpace(abs) {
		return err
	}
	return fmt.Errorf("internal artifact %q (system-generated intermediate; the oversized path below is codrax's own temp artifact file, not a user-provided input): %w", alias, err)
}

func actionArtifactFilePreference(abs string, maxBytes int64) int {
	abs = strings.TrimSpace(abs)
	if abs == "" {
		return 0
	}
	// Bounded whole-file read (DQA O1). This is a soft ranking heuristic:
	// unreadable already meant preference 0, so oversize degrades the same
	// way instead of slurping. Artifact files are runner-written, but the
	// alias map can also be seeded, so the bound stays on. maxBytes is the
	// caller-resolved data-lane bound (DQA F4: callers pass the runner-level
	// override through EffectiveMaxFileBytes, consistent with every other
	// read point in the action lane — a runner configured for larger
	// materials must rank its own larger artifacts, not degrade them to 0).
	raw, err := width.ReadFileBounded(abs, maxBytes)
	if err != nil {
		return 0
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return 2
	}
	return jsonActionRecordPayloadPreference(value)
}

func jsonActionRecordPayloadPreference(value any) int {
	switch typed := value.(type) {
	case []any:
		return 4
	case map[string]any:
		for _, key := range []string{"records", "rows", "contributions", "entity_resolutions", "rule_coverage", "groups", "items", "data", "values"} {
			if arr, ok := typed[key].([]any); ok {
				if len(arr) > 0 {
					return 4
				}
				return 3
			}
		}
		if _, hasChildren := typed["children"]; hasChildren {
			if _, hasSummary := typed["summary"]; hasSummary {
				return 1
			}
		}
		return 2
	default:
		return 2
	}
}
