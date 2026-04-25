package tool

import (
	"path/filepath"
	"strings"
)

// WiringAnchor declares the relationship "files created under <Dir>
// MUST also have a corresponding modify entry in <WiringFile> in the
// same ChangePlan." Used by emit_change_plan's V3 wiring-closure
// validator to reject plans that create dead code (a new MCP server,
// skill, or built-in tool that nothing registers).
//
// This table is the single source of truth for "what gets registered
// where" — kept here (in the tool package, alongside the validator
// that reads it) rather than scattered across each subsystem because
// the validator is the only place that needs the cross-cutting view.
// When a new pluggable subsystem is added (e.g. a future "transports"
// package), append one entry here and the validator picks it up.
//
// Reason field is mandatory and surfaces in the rejection message so
// the planner LLM sees WHY it must include the wiring change, not just
// that it must.
type WiringAnchor struct {
	// Dir is the repo-relative directory whose new files trigger this
	// anchor. Matched as prefix on the change's Path (so a new file
	// at "internal/mcp/foo.go" matches Dir="internal/mcp").
	Dir string

	// WiringFile is the repo-relative path of the file that MUST also
	// appear in changes[] (with kind=modify or kind=patch) when a new
	// file lands under Dir. The planner is expected to add registration
	// boilerplate here.
	WiringFile string

	// Reason is the human-readable explanation surfaced to the planner
	// when V3 rejects. Should answer "why does the new file need to be
	// wired in this specific file?" in one sentence.
	Reason string
}

// wiringAnchors is the static table of "dir → wiring file" pairs.
// Order matters: V3 returns the FIRST matching anchor, so put more
// specific dirs (e.g. "internal/mcp/sub") before parents.
var wiringAnchors = []WiringAnchor{
	{
		Dir:        "internal/mcp",
		WiringFile: "cmd/root.go",
		Reason:     "MCP servers are registered in cmd/root.go via mcp.Registry.Register; a new internal/mcp/*.go file with no corresponding cmd/root.go modify entry is dead code that the runtime never instantiates",
	},
	{
		Dir:        "internal/skill",
		WiringFile: "internal/skill/defaults.go",
		Reason:     "Skill configs are registered in internal/skill/defaults.go via skill.Registry.Register; a new internal/skill/*.go file with no defaults.go modify entry will not appear in the agent dispatch table",
	},
	{
		Dir:        "internal/tool",
		WiringFile: "cmd/root.go",
		Reason:     "Built-in tools are wired into the global registry from cmd/root.go (search for tool.Register calls); a new internal/tool/*.go file without the corresponding cmd/root.go modify entry will be unreachable from any agent",
	},
	{
		Dir:        "internal/agent",
		WiringFile: "cmd/root.go",
		Reason:     "Agents are constructed and registered from cmd/root.go via agent.Registry.Register; a new internal/agent/*.go file declaring an Agent without the corresponding cmd/root.go modify entry will not be dispatched by the orchestrator",
	},
}

// MatchWiringAnchor returns the first WiringAnchor whose Dir is a
// prefix of the given repo-relative path, or nil if no anchor matches.
// Case-sensitive; trailing-slash agnostic. Returns nil for paths that
// are themselves the wiring file (so a modify of cmd/root.go for some
// other reason does not trigger spurious self-matches).
//
// Used by V3 wiring-closure validator and exported so tests in this
// package can assert the table contents.
func MatchWiringAnchor(repoRelPath string) *WiringAnchor {
	clean := filepath.ToSlash(strings.TrimSpace(repoRelPath))
	if clean == "" {
		return nil
	}
	// Skip non-Go files outright — wiring is a Go-runtime concern
	// (registries are Go maps); a new YAML/MD file under internal/mcp
	// does not need registration.
	if !strings.HasSuffix(clean, ".go") {
		return nil
	}
	// Skip _test.go — tests don't need to be wired into the runtime
	// registries; they construct their own subjects locally.
	if strings.HasSuffix(clean, "_test.go") {
		return nil
	}
	for i := range wiringAnchors {
		anchor := &wiringAnchors[i]
		// Self-match guard: a modify of the wiring file itself does
		// not need wiring (it IS the wiring).
		if clean == anchor.WiringFile {
			continue
		}
		dir := strings.TrimRight(anchor.Dir, "/")
		if strings.HasPrefix(clean, dir+"/") {
			return anchor
		}
	}
	return nil
}
