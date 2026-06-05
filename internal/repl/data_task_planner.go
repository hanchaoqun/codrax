package repl

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/dataquery"
	"github.com/hanchaoqun/codrax/internal/llm"
)

type DataTaskPlanner interface {
	PlanDataTask(ctx context.Context, userLine, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile) (dataquery.TaskPlan, error)
}

type llmDataTaskPlanner struct {
	adapter   llm.Adapter
	lastTrace replLLMCallTrace
}

func NewDataTaskPlanner(adapter llm.Adapter) DataTaskPlanner {
	return &llmDataTaskPlanner{adapter: adapter}
}

var dataTaskPlanTool = llm.ToolSchema{
	Name:        "emit_data_task_plan",
	Description: "Emit a typed read-only data-task plan. This tool never executes code.",
	Parameters: json.RawMessage(`{
  "type": "object",
  "properties": {
    "status": {
      "type": "string",
      "enum": ["ready", "needs_clarification", "blocked"],
      "description": "ready when input paths, output contract, and script are concrete; needs_clarification only for user-owned missing inputs; blocked when safe read-only data calculation cannot be attempted."
    },
    "input_paths": {
      "type": "array",
      "items": {"type": "string"},
      "description": "Repo/workspace-relative local data files or directories to copy into the isolated data workspace. Use paths from the candidate list when possible."
    },
    "output_contract": {
      "type": "object",
      "properties": {
        "format": {"type": "string", "enum": ["plain_single_line", "csv_line", "json_only", "markdown_table", "markdown", "file_path", "freeform"]},
        "explanation_allowed": {"type": "boolean"},
        "delimiter": {"type": "string"}
      },
      "required": ["format", "explanation_allowed"],
      "description": "Typed final-output contract derived from the user's requested shape. If the user says only output JSON/CSV/a single line, set explanation_allowed=false."
    },
    "script": {
      "type": "string",
      "description": "Python calculation body executed by a restricted read-only helper. Prefer csv_rows(path), tsv_rows(path), json_load(path), jsonl_rows(path), read_text(path), parse_money(value), Decimal, re, and emit(obj). You may import only common data standard libraries such as csv/json/decimal/re/math/statistics/collections/datetime/itertools/functools/operator, and open(path) only reads declared input files. Do not access os/sys/pathlib/shutil, run shell commands, use network libraries, dynamic eval/exec, or write files. The script must call emit({\"answer\": string, \"output_contract\": object, \"audit_summary\": string, \"rows\": [...]}) or set result to that object."
    },
    "questions": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "id": {"type":"string"},
          "question": {"type":"string"},
          "suggestions": {"type":"array", "items":{"type":"string"}}
        }
      },
      "description": "Short user questions when status=needs_clarification."
    },
    "block_reason": {
      "type": "string",
      "description": "Reason when status=blocked."
    }
  },
  "required": ["status", "output_contract"]
}`),
}

const dataTaskPlannerSystemPrompt = `You are a read-only data task planner.

Convert the user's request into a typed data task plan for deterministic local data processing.

Hard rules:
- Emit only emit_data_task_plan. Do not write prose.
- This lane is for structured/semi-structured data computation: CSV/TSV/JSON/JSONL/text cleaning, filtering, joining, aggregation, row-level decisions, and strict data-shaped output.
- This is not source-code analysis, log/trace diagnosis, write-mode code editing, or computer operation.
- Use candidate files when possible. If no relevant local data file is available or the user's business rule is genuinely missing, ask a short clarification question.
- Do not make the model hand-calculate the final answer. The script must compute it.
- The final result object must include answer as a string and output_contract as an object.
- If the user requests JSON-only, CSV-only, a single line, only a file path, only a code block, or a Markdown table, encode that in output_contract. Do not hard-code one output style.
- If explanation_allowed=false, answer must be exactly the requested final payload, with no explanatory prefix or suffix.
- Script sandbox: prefer the provided helpers:
  csv_rows(path), tsv_rows(path), json_load(path), jsonl_rows(path), read_text(path), parse_money(value), Decimal, re, emit(obj).
- You may also import common data standard libraries only: csv, json, decimal, re, math, statistics, collections, datetime, itertools, functools, operator.
- open(path) is read-only and works only for files listed in input_paths. Never write files, run shell commands, use network/process libraries, access os/sys/pathlib/shutil, or use dynamic eval/exec/compile.
- input_paths must list every path the script reads. The runner copies only those paths into an isolated temporary workspace.
- Emit row-level audit in rows when the task includes filtering/joining/aggregation decisions. Do not misuse member_set/count semantics; numeric totals belong in answer/metrics/audit, not in member count fields.
`

func (p *llmDataTaskPlanner) PlanDataTask(ctx context.Context, userLine, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile) (dataquery.TaskPlan, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if p == nil || p.adapter == nil {
		return dataquery.TaskPlan{}, fmt.Errorf("data task planner is not configured")
	}
	req := dataTaskPlannerPrompt(userLine, repoRoot, policy, candidates)
	resp, err := p.adapter.Chat(ctx,
		[]llm.Message{
			{Role: "system", Content: dataTaskPlannerSystemPrompt},
			{Role: "user", Content: req},
		},
		[]llm.ToolSchema{dataTaskPlanTool},
		llm.ChatOptions{ToolChoice: "required"},
	)
	p.lastTrace = traceFromLLMResponse("data_task_planner", resp)
	if err != nil {
		return dataquery.TaskPlan{}, err
	}
	if len(resp.ToolCalls) == 0 {
		return dataquery.TaskPlan{}, fmt.Errorf("data task planner returned no tool_call")
	}
	call := resp.ToolCalls[0]
	if call.Name != dataTaskPlanTool.Name {
		return dataquery.TaskPlan{}, fmt.Errorf("data task planner returned unexpected tool %q", call.Name)
	}
	var parsed dataTaskPlanDraft
	if err := unmarshalReplStructuredToolParams(dataTaskPlanTool, call.Params, &parsed, "data task planner"); err != nil {
		return dataquery.TaskPlan{}, err
	}
	return parsed.toPlan(), nil
}

func (p *llmDataTaskPlanner) LastReplLLMTrace() replLLMCallTrace {
	if p == nil {
		return replLLMCallTrace{}
	}
	return p.lastTrace
}

func dataTaskPlannerPrompt(userLine, repoRoot string, policy TurnPolicy, candidates []dataquery.CandidateFile) string {
	var b strings.Builder
	fmt.Fprintf(&b, "## user_request\n%s\n\n", strings.TrimSpace(userLine))
	fmt.Fprintf(&b, "## workspace_root\n%s\n\n", strings.TrimSpace(repoRoot))
	fmt.Fprintf(&b, "## route_policy\nroute=%s data_task_kind=%s operation=%s source=%s confidence=%.2f\n\n",
		policy.Route, policy.DataTaskKind, policy.Operation, policy.Source, policy.Confidence)
	b.WriteString("## candidate_data_files\n")
	if len(candidates) == 0 {
		b.WriteString("(none discovered)\n")
	} else {
		limit := len(candidates)
		if limit > 120 {
			limit = 120
		}
		for i := 0; i < limit; i++ {
			f := candidates[i]
			fmt.Fprintf(&b, "- path=%s kind=%s size=%d\n", f.Path, f.Kind, f.Size)
		}
		if len(candidates) > limit {
			fmt.Fprintf(&b, "- ... %d more candidate file(s) omitted\n", len(candidates)-limit)
		}
	}
	return strings.TrimSpace(b.String())
}

type dataTaskPlanDraft struct {
	Status         flexiblePolicyString     `json:"status"`
	InputPaths     flexiblePolicyStringList `json:"input_paths"`
	OutputContract dataTaskOutputDraft      `json:"output_contract"`
	Script         flexiblePolicyString     `json:"script"`
	Questions      []dataTaskQuestionDraft  `json:"questions"`
	BlockReason    flexiblePolicyString     `json:"block_reason"`
}

type dataTaskOutputDraft struct {
	Format             flexiblePolicyString `json:"format"`
	ExplanationAllowed flexiblePolicyBool   `json:"explanation_allowed"`
	Delimiter          flexiblePolicyString `json:"delimiter"`
}

type dataTaskQuestionDraft struct {
	ID          flexiblePolicyString     `json:"id"`
	Question    flexiblePolicyString     `json:"question"`
	Suggestions flexiblePolicyStringList `json:"suggestions"`
}

func (d dataTaskPlanDraft) toPlan() dataquery.TaskPlan {
	qs := make([]dataquery.Question, 0, len(d.Questions))
	for _, q := range d.Questions {
		if strings.TrimSpace(string(q.Question)) == "" {
			continue
		}
		qs = append(qs, dataquery.Question{
			ID:          strings.TrimSpace(string(q.ID)),
			Question:    strings.TrimSpace(string(q.Question)),
			Suggestions: []string(q.Suggestions),
		})
	}
	return dataquery.TaskPlan{
		Status:     strings.ToLower(strings.TrimSpace(string(d.Status))),
		InputPaths: cleanPolicyStringList([]string(d.InputPaths)),
		OutputContract: dataquery.OutputContract{
			Format:             dataquery.OutputFormat(strings.TrimSpace(string(d.OutputContract.Format))),
			ExplanationAllowed: bool(d.OutputContract.ExplanationAllowed),
			Delimiter:          strings.TrimSpace(string(d.OutputContract.Delimiter)),
		}.Normalize(),
		Script:      strings.TrimSpace(string(d.Script)),
		Questions:   qs,
		BlockReason: strings.TrimSpace(string(d.BlockReason)),
	}
}
