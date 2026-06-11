package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/worktree"
)

// dirIsEffectivelyEmpty forwards to the canonical probe in the
// worktree package. The REPL's bare-dir consent prompt and the stage
// hooks' scaffold-tier gate MUST agree on emptiness, so there is
// exactly one implementation (worktree.DirIsEffectivelyEmpty); this
// alias keeps the orchestrator-internal call sites and their tests
// unchanged.
func dirIsEffectivelyEmpty(path string) bool {
	return worktree.DirIsEffectivelyEmpty(path)
}

// emptyRepoReadIntro builds the user-visible terminal message
// shown when the target directory has no source files to analyse.
// Plain language only — no internal stage names, no project brand,
// no implementation jargon. The user gets a one-line summary of
// what was observed plus the concrete next actions.
//
// Bilingual surface: zh when language is empty (default) or starts
// with "zh", en otherwise. SetResultPlain renders the result
// verbatim so the line breaks survive intact.
func emptyRepoReadIntro(language, repoRoot string) string {
	zh := language == "" || strings.HasPrefix(strings.ToLower(language), "zh")
	if zh {
		return fmt.Sprintf(
			"这个目录(%s)里没有可以分析的源代码文件。\n\n"+
				"接下来你可以:\n"+
				"  - 想看已有代码:把目录换成放着源代码的那个目录\n"+
				"  - 想从零搭一个新项目:同时加上 --auto-init-repo 和 --allow-scaffold,然后用 /mode write 提需求 (前者允许把目录变成 git 仓库,后者允许凭空生成文件)\n"+
				"  - 只是想随便聊聊:用 /chat <消息>",
			repoRoot)
	}
	return fmt.Sprintf(
		"this directory (%s) has no source files to look at.\n\n"+
			"Next you can:\n"+
			"  - look at existing code: point the tool at the directory that actually holds the source\n"+
			"  - start a new project from scratch: pass BOTH --auto-init-repo and --allow-scaffold, then use /mode write with your request (the first allows turning the directory into a git repo, the second allows generating files from nothing)\n"+
			"  - just chat: use /chat <message>",
		repoRoot)
}
