package skill

// RegisterDefaults registers all built-in skill configurations.
func RegisterDefaults(r *Registry) {
	r.Register(&Config{
		Name: "task-analysis-skill",
		Goal: "Structurize and classify the user task into actionable items.",
		Workflow: []string{
			"read user input",
			"identify intent type",
			"classify task type",
			"generate objective",
			"decompose into subtasks",
			"extract constraints",
			"identify missing pieces",
		},
		ToolSuggestions: []string{},
		OutputFormat:    "JSON with task_type, objective, task_list, constraints, missing_piece",
		Prohibitions: []string{
			"do not make assumptions about code structure",
			"do not start implementation",
		},
	})

	r.Register(&Config{
		Name: "repo-explore-skill",
		Goal: "Build a trusted factual foundation about the codebase.",
		Workflow: []string{
			"find entry points",
			"grep for key functions",
			"build module map",
			"analyze call chains",
			"identify relevant files",
			"document findings as RepoFacts",
		},
		ToolSuggestions: []string{
			"grep",
			"read_file",
			"list_files",
			"repo_map",
			"exec_command",
		},
		OutputFormat: "JSON with repo_facts, entrypoints, call_chains, relevant_files",
		Prohibitions: []string{
			"do not modify any files",
			"do not make assumptions without evidence",
		},
	})

	r.Register(&Config{
		Name: "cli-analysis-skill",
		Goal: "Analyze CLI structure and commands.",
		Workflow: []string{
			"identify CLI framework",
			"map commands and subcommands",
			"trace command handlers",
			"document flags and options",
		},
		ToolSuggestions: []string{
			"grep",
			"read_file",
			"exec_command",
		},
		OutputFormat: "JSON with cli_structure, commands, handlers",
		Prohibitions: []string{
			"do not execute destructive commands",
		},
	})

	r.Register(&Config{
		Name: "implementation-plan-skill",
		Goal: "Design a step-by-step implementation plan.",
		Workflow: []string{
			"review objective and facts",
			"identify files to modify",
			"design change sequence",
			"assess risks",
			"define verification approach",
			"estimate impact scope",
		},
		ToolSuggestions: []string{
			"read_file",
			"grep",
		},
		OutputFormat: "JSON with plan containing files_to_modify, steps, risks, validation",
		Prohibitions: []string{
			"do not write code",
			"do not modify files",
		},
	})

	r.Register(&Config{
		Name: "code-implement-skill",
		Goal: "Write or modify code to implement the plan.",
		Workflow: []string{
			"review plan steps",
			"implement changes file by file",
			"generate patches",
			"verify syntax",
			"add necessary imports",
			"handle edge cases",
		},
		ToolSuggestions: []string{
			"read_file",
			"write_file",
			"exec_command",
			"grep",
		},
		OutputFormat: "JSON with patch, modified_files, implementation_notes",
		Prohibitions: []string{
			"do not deviate from the plan without justification",
			"do not delete unrelated code",
		},
	})

	r.Register(&Config{
		Name: "design-review-skill",
		Goal: "Review plan feasibility, architecture impact, and risks.",
		Workflow: []string{
			"check requirement alignment",
			"verify architecture integrity",
			"evaluate edge cases",
			"assess security and performance risks",
			"check for missing steps",
			"provide recommendations",
		},
		ToolSuggestions: []string{
			"read_file",
			"grep",
		},
		OutputFormat: "JSON with review_result (pass/fail), issues, must_fix, suggestions",
		Prohibitions: []string{
			"do not modify the plan",
			"do not implement fixes",
		},
	})

	r.Register(&Config{
		Name: "code-review-skill",
		Goal: "Review code correctness, bugs, style, and side effects.",
		Workflow: []string{
			"check plan conformance",
			"verify logic correctness",
			"check boundary conditions",
			"review error handling",
			"assess code style",
			"check for side effects",
			"verify backwards compatibility",
		},
		ToolSuggestions: []string{
			"read_file",
			"grep",
			"git_diff",
		},
		OutputFormat: "JSON with review_result (pass/fail), code_issues, must_fix",
		Prohibitions: []string{
			"do not modify code",
			"do not auto-fix issues",
		},
	})

	r.Register(&Config{
		Name: "verification-skill",
		Goal: "Verify implementation correctness through testing.",
		Workflow: []string{
			"compile the project",
			"run unit tests",
			"run linter",
			"execute smoke tests",
			"verify runtime behavior",
			"report results",
		},
		ToolSuggestions: []string{
			"exec_command",
			"run_tests",
			"read_file",
		},
		OutputFormat: "JSON with verification_result (pass/fail), logs, errors, next_action",
		Prohibitions: []string{
			"do not modify source code",
			"do not skip failing tests",
		},
	})

	r.Register(&Config{
		Name: "final-answer-skill",
		Goal: "Produce the final user-facing output.",
		Workflow: []string{
			"summarize all changes",
			"compile patch information",
			"write usage instructions",
			"list action steps",
			"mark tasks complete",
		},
		ToolSuggestions: []string{},
		OutputFormat:    "Markdown with summary, changes, instructions",
		Prohibitions: []string{
			"do not introduce new changes",
			"do not omit failures or warnings",
		},
	})
}
