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
		ToolSuggestions: []string{
			"todo_write",
		},
		OutputFormat: `Call the todo_write tool with the decomposed task list. For each task set: ` +
			`title (short user-facing label), writing (true if the task may modify files, ` +
			`false for read-only / question-answering work), and high_risk (true only for ` +
			`writing tasks that touch security-sensitive code, schemas, or irreversible ops). ` +
			`After todo_write succeeds, briefly explain your classification in plain text.`,
		Prohibitions: []string{
			"do not make assumptions about code structure",
			"do not start implementation",
		},
	})

	r.Register(&Config{
		Name: "repo-explore-skill",
		Goal: "Investigate the user's question and answer it directly using evidence from the code.",
		Workflow: []string{
			"form a hypothesis about where the answer lives",
			"read the code or run the commands needed to verify it",
			"if you surface a name that looks load-bearing (a function, type, symbol, config key), open it before drawing conclusions — a name is a hypothesis to verify, not an answer",
			"iterate until you have a specific answer, not a description of where someone else could find it",
			"record the answer alongside the evidence (file:line) that supports it",
		},
		ToolSuggestions: []string{
			"grep",
			"read_file",
			"list_files",
			"repo_map",
			"exec_command",
			"todo_write",
		},
		// Few-shot OutputFormat. The previous abstract description ("Direct
		// answer ... plus file:line citations") was insufficient to bind the
		// LLM away from its training-distribution default of "Key Components
		// / Files Involved / Usage Instructions" cataloger sections. A
		// concrete example is much stickier than an abstract requirement.
		OutputFormat: `Output the answer in this exact shape (no Key Components / Files Involved / Usage Instructions sections):

Answer: <one or two sentences directly answering the question>
Evidence:
- <file:line> — <what this line establishes>
- <file:line> — <what this line establishes>

If part of the question genuinely cannot be answered from the code you read, name the missing piece in one extra sentence — do not substitute "further analysis required" for an answer.

Example:
Answer: There is one SubAgent implementation in the project, SubExplorer.
Evidence:
- internal/agent/subagent.go:63-66 — RegisterDefaultSubAgents adds NewSubExplorer(deps) to the registry
- internal/agent/sub_explorer.go:14 — SubExplorer type definition`,
		Prohibitions: []string{
			"do not modify any files",
			"do not make assumptions without evidence",
			"do not stop at 'the answer would require checking X' — go check X yourself",
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
			"apply_patch",
			"exec_command",
			"grep",
			"todo_write",
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
		Goal: "Produce the final user-facing output for an implementation task.",
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

	// analysis-final-answer-skill is the finalize-stage skill used when the
	// task is read-only / question-answering (Writing == false). The default
	// final-answer-skill is shaped for implementation tasks: its workflow is
	// "summarize all changes", "compile patch information", "write usage
	// instructions", "list action steps", "mark tasks complete" — verbs that
	// presuppose a code change actually happened. Forcing an analysis answer
	// through that template produces verbose templated prose with invented
	// "Action Steps" and "Tasks Completion" sections, and it dilutes precise
	// quantitative answers ("1 SubExplorer") into mushy ones ("several
	// components facilitate subagent management"). The orchestrator's
	// dispatchStage routes to this skill when determineActivePolicy()
	// returns "analysis".
	r.Register(&Config{
		Name: "analysis-final-answer-skill",
		Goal: "Answer the user's question directly using the evidence the prior stages collected.",
		Workflow: []string{
			"read the user's original question",
			"read the prior stage findings (especially the explorer's stage report) and identify the specific answer the evidence supports",
			"state that answer in one sentence",
			"cite the evidence (file:line where applicable) that establishes it",
			"if part of the question is unanswered, say which part and why — do not substitute templated 'further analysis required' boilerplate for an answer",
		},
		ToolSuggestions: []string{},
		// This is a few-shot OutputFormat, not an abstract description. LLMs
		// bind much more tightly to a concrete example than to a list of
		// requirements, and the failure mode this is fixing is precisely the
		// LLM defaulting to its training-distribution template instead of
		// the requested shape.
		OutputFormat: `Output exactly the following structure (no extra sections, no Summary/Changes/Conclusion/Action Steps headers):

**Answer:** <one or two sentences directly answering the user's question>

**Evidence:**
- <file:line> — <what this evidence establishes>
- <file:line> — <what this evidence establishes>

(Optional) **Caveat:** <only if part of the question genuinely cannot be answered from the gathered evidence — name the missing piece, do not pad>

Example:
**Answer:** There is one SubAgent implementation in the project, ` + "`SubExplorer`" + `, registered as the default subagent.

**Evidence:**
- internal/agent/subagent.go:63-66 — RegisterDefaultSubAgents adds NewSubExplorer(deps) to the registry
- internal/agent/sub_explorer.go:14 — SubExplorer type definition`,
		Prohibitions: []string{
			"do not produce a Summary / Changes / Conclusion / Action Steps / Instructions template",
			"do not invent next steps the user did not ask for",
			"do not list project components when the question asked for a count or a name",
			"do not substitute 'further investigation needed' for an answer the prior stages already established",
		},
	})
}
