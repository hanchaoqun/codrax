package worktree

import (
	"os/exec"
	"os/user"
	"strings"
)

// OperatorIdentity resolves WHO write-mode decisions are recorded for:
// the repo's git identity ("Name <email>"), falling back to the OS
// user name when git identity is absent. Deterministic capture — no
// flags, no prompts — so approval records always carry an operator
// even on minimally-configured machines.
func OperatorIdentity(repoRoot string) string {
	name := gitConfigValue(repoRoot, "user.name")
	email := gitConfigValue(repoRoot, "user.email")
	switch {
	case name != "" && email != "":
		return name + " <" + email + ">"
	case name != "":
		return name
	case email != "":
		return email
	}
	if u, err := user.Current(); err == nil && strings.TrimSpace(u.Username) != "" {
		return strings.TrimSpace(u.Username)
	}
	return ""
}

func gitConfigValue(repoRoot, key string) string {
	cmd := exec.Command("git", "-C", repoRoot, "config", "--get", key)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
