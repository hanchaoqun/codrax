package ground

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

type activeSetAliasGater struct {
	aliases map[string]string
}

func (g activeSetAliasGater) ResolveActiveSetPath(_ *types.BusContext, _, llmPath string, _ func(string) bool) types.ActiveSetGateResult {
	if dst, ok := g.aliases[llmPath]; ok {
		return types.ActiveSetGateResult{
			Allowed:        true,
			ResolvedPath:   dst,
			SubRepoRootRel: "opencode",
			AutoPrefixed:   true,
		}
	}
	return types.ActiveSetGateResult{Allowed: true, ResolvedPath: llmPath}
}

func (g activeSetAliasGater) ResolveActiveSetCommand(_ *types.BusContext, _, _ string) types.ActiveSetGateResult {
	return types.ActiveSetGateResult{Allowed: true}
}

func TestGroundItem_MultiRepoActiveSetAliasMatchesReadHistory(t *testing.T) {
	ctx := &types.BusContext{
		MultiGraph: activeSetAliasGater{aliases: map[string]string{
			"packages/opencode/src/tool/read.ts": "opencode/packages/opencode/src/tool/read.ts",
		}},
		ToolResults: []types.ToolResult{{
			ToolName: "read_file",
			Success:  true,
			Summary: "[opencode/packages/opencode/src/tool/read.ts: showing lines 37-37 of 338 total]\n" +
				"    37│ export const ReadTool = Tool.define(\"read\", {\n",
		}},
	}
	gc := BuildContext(ctx)
	item := types.EvidenceItem{
		Kind:         types.EvidenceMechanism,
		Source:       "packages/opencode/src/tool/read.ts",
		LineStart:    37,
		AnchorKind:   types.AnchorDefinition,
		AnchorSymbol: "ReadTool",
		Scope:        types.ScopeLine,
	}

	rep := GroundItem(&item, gc)
	if rep.Status != types.GroundingGrounded {
		t.Fatalf("expected active-set alias to ground against read_file history, status=%s note=%q", rep.Status, rep.Note)
	}
	if item.Source != "opencode/packages/opencode/src/tool/read.ts" {
		t.Fatalf("item source should be rewritten to the read_file history key, got %q", item.Source)
	}
}

func TestGroundCitation_MultiRepoActiveSetAliasMatchesReadHistory(t *testing.T) {
	ctx := &types.BusContext{
		MultiGraph: activeSetAliasGater{aliases: map[string]string{
			"packages/opencode/src/tool/read.ts": "opencode/packages/opencode/src/tool/read.ts",
		}},
		ToolResults: []types.ToolResult{{
			ToolName: "read_file",
			Success:  true,
			Summary: "[opencode/packages/opencode/src/tool/read.ts: showing lines 37-37 of 338 total]\n" +
				"    37│ export const ReadTool = Tool.define(\"read\", {\n",
		}},
	}
	gc := BuildContext(ctx)

	rep := GroundCitation(types.Citation{
		File: "packages/opencode/src/tool/read.ts",
		Line: 37,
	}, gc)
	if !rep.Valid || rep.Tier != types.TierLineText {
		t.Fatalf("expected active-set alias citation to hit line_text, got valid=%t tier=%s reason=%q", rep.Valid, rep.Tier, rep.Reason)
	}
}
