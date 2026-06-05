package orchestrator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// dirIsEffectivelyEmpty reports whether path contains no files the
// pipeline could analyse. "Effectively empty" means: directory is
// missing, unreadable, or its contents (after filtering out the
// canonical `.git` directory + the codrax-managed `.codrax/` runtime
// directory) carry no regular files at any depth.
//
// Cross-platform: os.ReadDir + filepath.Walk both use the OS-native
// path machinery (Windows / macOS / Linux). The hidden-file test is
// the dotfile convention shared by all three POSIX-like surfaces;
// Windows hides via attribute, but the dotfile prefix is still the
// only stable surface from a Go program without syscall packages.
//
// Walk depth caps: stops descending after 256 entries TOTAL across
// the whole walk so a deeply nested non-empty tree returns false
// quickly without scanning every file.
func dirIsEffectivelyEmpty(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return false
	}
	const maxScan = 256
	scanned := 0
	var walk func(string) bool
	walk = func(dir string) bool {
		es, err := os.ReadDir(dir)
		if err != nil {
			return true // unreadable subtree counts as "no usable files"
		}
		for _, e := range es {
			scanned++
			if scanned > maxScan {
				return false
			}
			name := e.Name()
			if e.IsDir() {
				if name == ".git" || name == ".codrax" {
					continue
				}
				if !walk(filepath.Join(dir, name)) {
					return false
				}
				continue
			}
			return false
		}
		return true
	}
	for _, e := range entries {
		scanned++
		if scanned > maxScan {
			return false
		}
		name := e.Name()
		if e.IsDir() {
			if name == ".git" || name == ".codrax" {
				continue
			}
			if !walk(filepath.Join(path, name)) {
				return false
			}
			continue
		}
		return false
	}
	return true
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
