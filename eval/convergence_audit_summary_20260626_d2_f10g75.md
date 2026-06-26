# D2-F10g75 Representative Eval Audit

Date: 2026-06-26
Scope: representative read/write convergence batch, 2-way parallel, 6 cases.
Baseline: after D1-F10g.74 analyzer bounded source-inventory prescan cutover.

## Result

Overall: 6/6 PASS.

| Case | Result | Wall | Key Tools / Signals | Manual Audit |
| --- | --- | ---: | --- | --- |
| `qf_relation_subagent_registry-20260626-111409` | PASS | 109s | `repo_map=1`, `read_file=7`, `finalizer_rejects=0`, `max_context_tokens_est=65816` | Answer is grounded in agent/subagent registry and runtime entry evidence. No hard-gate loop observed. |
| `arkts_repomap-20260626-111409` | PASS | 130s | `repo_map=1`, `read_file=7`, `list_files=1`, `unavailable_tool_attempts=1`, `answer_contract_violations=1` | Functionally acceptable, but extractor attempted unavailable `read_file`; record as residual tool-boundary/handoff gap, not a case-specific patch. |
| `cangjie_repomap-20260626-111640` | PASS | 208s | `repo_map=3`, `read_file=14`, `list_files=0`, `finalizer_rejects=0`, `max_context_tokens_est=58087` | Correctness improved, but log proved broad analyzer/source-inventory navigation candidates leaked into generic forced-read and caused unrelated Go file reads after `.cj` evidence closure. Tracked and fixed as D1-G120 / D1-F10g.75. |
| `trace_query_openharmony_bytrace_thread-20260626-111640` | PASS | 160s | `trace_query=4`, `repo_map=0`, `read_file=1` | Trace-first path preserved; no repo_map/source-localizer hard gate regression in this run. |
| `read_combo_log_current_source_explanation-20260626-112022` | PASS | 188s | `read_file=11`, `repo_map=1`, `answer_contract_violations=3`, `max_context_tokens_est=72209` | Functional, but high context and residual contract warnings remain a separate read/log-current-source quality gap. |
| `patch_go_typo-20260626-112022` | PASS | 95s | `repo_map=1`, `read_file=2`, authoritative `go test -json ./...` passed | Write mode produced the intended one-line patch (`retrun` -> `return`) and official local verification passed. |

## Architecture Findings

### Closed in D1-F10g.75

`source_inventory` navigation and ranker candidates could still become generic forced-read debt. In the Cangjie run, the explorer had already grounded the requested `.cj` declarations and emitted enough principal evidence, but the system later forced reads of broad Go implementation/support files inherited from analyzer/source-inventory navigation. This is a system class gap: navigation candidates are not proof obligations.

Fix direction:
- Producer side: active source-inventory requests keep only explicit file-shaped mentions/exact targets and precise runtime/log files in `EvidencePlan.RequiredFiles`.
- Consumer side: `phase1_unread` only queues source-inventory ranker files when a completed strict source-inventory scope exists.
- Completion side: generic forced-read boundary accepts precise source-inventory requested/exact universe member-set closure.

### Residuals for Later Batches

- ArkTS extract-stage unavailable tool attempt: extractor/finalizer prompt/tool-boundary should not expose or suggest explorer-only tools. This should be fixed through typed tool-surface projection or repair, not by matching the `read_file` text.
- Read combo contract warnings/high context: keep auditing answer-contract warning families and context handoff pruning for log/current-source mixed lanes.
- Representative eval should be rerun after the D1-F10g.75 patch to confirm Cangjie no longer performs unrelated Go support forced reads and that trace/write stable paths remain unchanged.

