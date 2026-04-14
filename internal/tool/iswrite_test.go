package tool

import "testing"

// TestBuiltinIsWriteClassification locks in the read/write classification
// for every builtin tool. Adding a new tool that doesn't compile here is
// the intended way to force a deliberate read/write decision.
//
// After the 2026-04-14 simplification the codrax pipeline is read-only:
// apply_patch and run_tests were deleted, so no builtin tool is classified
// as a filesystem write anymore.
func TestBuiltinIsWriteClassification(t *testing.T) {
	cases := []struct {
		name    string
		tool    Tool
		isWrite bool
	}{
		{"exec_command", &ExecCommand{}, false},
		{"grep", &GrepTool{}, false},
		{"read_file", &ReadFile{}, false},
		{"list_files", &ListFiles{}, false},
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

// TestBuiltinConfidenceClassification locks in the confidence classification
// for every builtin tool. Adding a new tool that doesn't compile here forces
// a deliberate evidence/navigation/non-evidence decision.
func TestBuiltinConfidenceClassification(t *testing.T) {
	cases := []struct {
		name       string
		tool       Tool
		confidence float64
	}{
		{"exec_command", &ExecCommand{}, 0.8},
		{"grep", &GrepTool{}, 0.8},
		{"read_file", &ReadFile{}, 0.8},
		{"list_files", &ListFiles{}, 0.8},
		{"git_diff", &GitDiff{}, 0.8},
		{"git_log", &GitLog{}, 0.8},
		{"propose_sub_agents", &ProposeSubAgents{}, 0.0},
		{"todo_write", &TodoWrite{}, 0.0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.tool.Confidence(); got != c.confidence {
				t.Errorf("%s.Confidence() = %v, want %v", c.name, got, c.confidence)
			}
		})
	}
}

func TestRegistryIsWrite(t *testing.T) {
	r := NewRegistry()
	RegisterDefaults(r)

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
