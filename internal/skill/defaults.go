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
		// OutputFormat is shape-by-example, not shape-by-rule. The previous
		// abstract description ("Direct answer ... plus file:line citations")
		// failed to bind the LLM away from its training-distribution default
		// of cataloger sections. A single concrete example fixed that, but
		// over-fit in two ways: (1) the example was the trigger query's
		// answer verbatim, contaminating the very query it was supposed to
		// validate, and (2) it pinned the answer length at "one or two
		// sentences", which compresses any genuine multi-paragraph
		// explanation request into a degenerate two-sentence summary plus
		// file list.
		//
		// The fix is three concrete examples covering three answer scales
		// (lookup, count, multi-paragraph explanation), all using content
		// completely unrelated to this codebase. The principle "scale the
		// answer to the question" is stated above the examples in plain
		// English. The examples teach format flexibility (the Answer block
		// scales) without teaching specific content (the LLM can't copy any
		// of these because none of them are about this repo).
		OutputFormat: `The shape of your answer must match the shape of the question. Lead with the direct answer at the right level of detail — one sentence for a count, name, or yes/no question; multiple paragraphs for an explanation, walkthrough, or comparison. Match the answer's depth to the question's depth: do not pad a one-sentence answer into prose, and do not compress a multi-paragraph explanation into two sentences. Always ground load-bearing claims in a file:line citation.

Use this shape — Answer first, then Evidence:

Answer: <direct answer at the right depth>
Evidence:
- <file:line> — <what this establishes>
- <file:line> — <what this establishes>

If part of the question genuinely cannot be answered from the code you read, add one short sentence naming the missing piece — do not substitute "further analysis required" for an answer the evidence supports.

Below are three illustrative examples drawn from a hypothetical, unrelated codebase. They show how the Answer block scales — DO NOT copy their content, only their shape:

Example A — lookup question (one short sentence is the right depth):
Answer: The project is written in Go 1.22.
Evidence:
- go.mod:3 — go 1.22.5

Example B — count question (one sentence + several pieces of evidence):
Answer: There are 4 HTTP handlers registered on the public router.
Evidence:
- cmd/server/router.go:18-21 — Mount calls for /health, /api/v1/users, /api/v1/orders, /metrics
- cmd/server/router.go:42 — no Mount calls after line 21

Example C — explanation question (multi-paragraph is the right depth):
Answer: The cache is a write-through layer between the API handlers and the database. On every write the handler updates the database first and then invalidates the corresponding cache key; the next read repopulates the cache from the database. There is no read-through-on-miss path: a cache miss is served directly from the database without writing back, which is intentional so that stale data from a misbehaving writer cannot persist beyond one request.

The cache key format is "<resource>:<id>" and the default TTL is 5 minutes. Eviction is purely LRU; there is no manual flush API.

Evidence:
- internal/cache/cache.go:42-58 — Set() invalidates the key after the DB write returns
- internal/cache/cache.go:78-91 — Get() falls through to the DB on miss but does not write back
- internal/cache/cache.go:14 — DefaultTTL = 5*time.Minute

Example D — architecture/flow question (use a Mermaid diagram when a visual clarifies relationships or sequences):
Answer: A write request flows through three layers before reaching the database:

` + "```mermaid" + `
sequenceDiagram
    participant C as Client
    participant H as Handler
    participant CA as Cache
    participant DB as Database
    C->>H: PUT /api/v1/users/42
    H->>DB: UPDATE users SET ...
    DB-->>H: ok
    H->>CA: invalidate("users:42")
    H-->>C: 200 OK
` + "```" + `

The handler always writes to the database first. Only after the DB confirms success does it invalidate the cache key.

Evidence:
- internal/handler/user.go:87-102 — UpdateUser calls repo.Save then cache.Invalidate
- internal/cache/cache.go:42-58 — Invalidate deletes the key, next Get repopulates from DB`,
		Prohibitions: []string{
			"do not modify any files",
			"do not make assumptions without evidence",
			"do not stop at 'the answer would require checking X' — go check X yourself",
			"do not write about what would be done next or what the user should do — answer only what was asked",
			"do not treat repo_map output as evidence — it is a cached navigation index that tells you where to look, not what is true. After consulting repo_map, always read_file or grep the actual source files to establish facts. list_files is fine as evidence since it reads the real directory",
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
		OutputFormat: "Markdown with summary, changes, and instructions. " +
			"Use tables for structured data (file lists, config comparisons). " +
			"Use Mermaid diagrams (```mermaid) when a visual would clarify architecture, " +
			"data flow, or sequence of operations.",
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
		Goal: "Answer the user's question directly using the evidence the prior stages collected, at the right level of detail for what was asked.",
		Workflow: []string{
			"read the user's original question and identify whether it asks for a fact (count/name/yes-no), an explanation (how/why/walkthrough), or a comparison",
			"read the prior stage findings (especially the explorer's stage report) and identify the specific answer the evidence supports",
			"state that answer at the right depth — one sentence for a fact, multiple paragraphs for an explanation",
			"ground load-bearing claims in file:line citations from the prior stages",
			"if part of the question is unanswered, say which part and why — do not substitute templated boilerplate for an answer the evidence supports",
		},
		ToolSuggestions: []string{},
		// OutputFormat shares the same shape-by-example design as
		// repo-explore-skill above. See the comment there for the rationale
		// and the over-fitting failure mode that motivated three examples
		// instead of one. The two skills use the same Answer/Evidence
		// shape so that the explorer's stage_report can flow through the
		// finalizer with minimal rewriting.
		OutputFormat: `The shape of your answer must match the shape of the question. Lead with the direct answer at the right level of detail — one sentence for a count, name, or yes/no question; multiple paragraphs for an explanation, walkthrough, or comparison. Match the answer's depth to the question's depth: do not pad a one-sentence answer into prose, and do not compress a multi-paragraph explanation into two sentences. Always ground load-bearing claims in a file:line citation.

Use this shape — Answer first, then Evidence:

**Answer:** <direct answer at the right depth>

**Evidence:**
- <file:line> — <what this establishes>
- <file:line> — <what this establishes>

(Optional) **Caveat:** <only if part of the question genuinely cannot be answered from the gathered evidence — name the missing piece in one short sentence, do not pad>

Below are three illustrative examples drawn from a hypothetical, unrelated codebase. They show how the Answer block scales — DO NOT copy their content, only their shape:

Example A — lookup question (one short sentence is the right depth):
**Answer:** The project is written in Go 1.22.
**Evidence:**
- go.mod:3 — go 1.22.5

Example B — count question (one sentence + several pieces of evidence):
**Answer:** There are 4 HTTP handlers registered on the public router.
**Evidence:**
- cmd/server/router.go:18-21 — Mount calls for /health, /api/v1/users, /api/v1/orders, /metrics
- cmd/server/router.go:42 — no Mount calls after line 21

Example C — explanation question (multi-paragraph is the right depth):
**Answer:** The cache is a write-through layer between the API handlers and the database. On every write the handler updates the database first and then invalidates the corresponding cache key; the next read repopulates the cache from the database. There is no read-through-on-miss path: a cache miss is served directly from the database without writing back, which is intentional so that stale data from a misbehaving writer cannot persist beyond one request.

The cache key format is "<resource>:<id>" and the default TTL is 5 minutes. Eviction is purely LRU; there is no manual flush API.

**Evidence:**
- internal/cache/cache.go:42-58 — Set() invalidates the key after the DB write returns
- internal/cache/cache.go:78-91 — Get() falls through to the DB on miss but does not write back
- internal/cache/cache.go:14 — DefaultTTL = 5*time.Minute

Example D — architecture/flow question (use a Mermaid diagram when a visual clarifies relationships or sequences):
**Answer:** A write request flows through three layers before reaching the database:

` + "```mermaid" + `
sequenceDiagram
    participant C as Client
    participant H as Handler
    participant CA as Cache
    participant DB as Database
    C->>H: PUT /api/v1/users/42
    H->>DB: UPDATE users SET ...
    DB-->>H: ok
    H->>CA: invalidate("users:42")
    H-->>C: 200 OK
` + "```" + `

The handler always writes to the database first. Only after the DB confirms success does it invalidate the cache key. This guarantees that a crash between the two steps leaves stale-but-safe data in the cache rather than new data in the cache with an uncommitted DB row.

**Evidence:**
- internal/handler/user.go:87-102 — UpdateUser calls repo.Save then cache.Invalidate
- internal/cache/cache.go:42-58 — Invalidate deletes the key, next Get repopulates from DB`,
		Prohibitions: []string{
			"do not invent next steps the user did not ask for",
			"do not write 'usage instructions' or 'action steps' for a question that asked for an answer rather than for changes",
			"do not substitute 'further investigation needed' for an answer the prior stages already established",
		},
	})
}
