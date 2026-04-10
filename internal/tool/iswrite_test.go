package tool

import "testing"

// TestBuiltinIsWriteClassification locks in the read/write classification
// for every builtin tool. Adding a new tool that doesn't compile here is
// the intended way to force a deliberate read/write decision.
func TestBuiltinIsWriteClassification(t *testing.T) {
	cases := []struct {
		name    string
		tool    Tool
		isWrite bool
	}{
		{"apply_patch", &ApplyPatch{}, true},
		{"exec_command", &ExecCommand{}, false},
		{"grep", &GrepTool{}, false},
		{"read_file", &ReadFile{}, false},
		{"list_files", &ListFiles{}, false},
		// repo_map moved to internal/tool/repomap/ (tree-sitter powered, registered from main.go)
		{"run_tests", &RunTests{}, false},
		{"git_diff", &GitDiff{}, false},
		{"git_log", &GitLog{}, false},
		{"propose_sub_agents", &ProposeSubAgents{}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.tool.IsWrite(); got != c.isWrite {
				t.Errorf("%s.IsWrite() = %v, want %v", c.name, got, c.isWrite)
			}
		})
	}
}

func TestRegistryIsWrite(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

	t.Run("write tool reports true", func(t *testing.T) {
		if !r.IsWrite("apply_patch") {
			t.Error("apply_patch should be classified as write")
		}
	})

	t.Run("read-only tool reports false", func(t *testing.T) {
		if r.IsWrite("read_file") {
			t.Error("read_file should be classified as read-only")
		}
		if r.IsWrite("grep") {
			t.Error("grep should be classified as read-only")
		}
		if r.IsWrite("exec_command") {
			t.Error("exec_command is currently classified as read-only")
		}
	})

	t.Run("unknown tool reports false", func(t *testing.T) {
		if r.IsWrite("nonexistent_tool") {
			t.Error("unknown tool should report false")
		}
	})
}
