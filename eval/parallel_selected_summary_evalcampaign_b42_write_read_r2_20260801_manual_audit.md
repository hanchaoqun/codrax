# Selected Eval Manual Audit Scaffold

- date: 2026-08-02T13:14:56Z
- sweep_start_ts: 20260802-061455
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | github_issue_napi_force_wasi_env_symptom | PASS | eval/results/github_issue_napi_force_wasi_env_symptom-20260802-061456 | write_apply,answer_regex | none | 185s | 19 | read=9,repo_map=1,list=1,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 生成 loader 内直接写入 true/error 严格比较，无跨模板变量；新 generated-output oracle 对应用树 exit 0，原始 fixture 与 r1 坏补丁均 exit 1。 |
| 2 | read_combo_log_current_source_explanation | FAIL | eval/results/read_combo_log_current_source_explanation-20260802-061456 | log_attachment,answer_regex | log_triage | 187s | 23 | read=0,repo_map=1,list=0,trace=0,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | 独立机制措辞生效但证据未闭合：零 read_file，仅以 timeout 两锚构造两机制 grouped_count，把 read-mode contract failure 错接到 writeRetryBudget/autoRepair。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human audit details

### github_issue_napi_force_wasi_env_symptom

- patch 仅改一行，严格比较位于 `renderNativeBinding` 返回模板内，没有 r1 的跨词法域变量。
- 当前主仓新 oracle 对 r2 applied-tree 输出
  `napi-rs generated-loader force-wasi scope and behavior checks passed`。
- `false/0` 只是不强制 WASI；`!nativeBinding` 的正常 fallback 保留，ground truth 与上游一致。
- human PASS，`EVAL-B42-GENART1=covered`。

### read_combo_log_current_source_explanation

- answer 已不再把 `IsStreamLevelRetryable=false` 直接等价成内容校验失败，说明第一版
  shared directive 对措辞有作用。
- 但 explorer 只执行 grep/repo_map，`read_file=0`；两个 current-source evidence 都是
  timeout 路径的 `IsStreamLevelRetryable` 与 `transientRetryBudget`，没有读取
  contract check、violation filtering、fallback 或 `requeueToStage`。
- principal grouped_count 的 dimensions 声称第二路径为
  `finalizer contract check / writeRetryBudget / autoRepair`，members/support_refs 却仍只有
  timeout 路径两锚。这里 `writeRetryBudget` 是写模式 verify→plan 预算，不是 read-mode
  成文合同失败的主重试预算。
- runner FAIL 的第一条 regex 还要求 runtime 与 source 词必须同一行；这是显示形 overfit，
  与机制正确性无关，已拆成独立 runtime/source/contrast 三个 oracle。
- human FAIL，`EVAL-B42-CONTRAST1=partial`，新增结构根因
  `EVAL-B42-MECHCARRIER1/P1`。

## Second-batch decision

- 复用现有 `aggregate_facts.member_set` 的成员—说明—引用索引映射，不新增答案接管：
  每个被比较机制一个 member、一个 member_note、一个 support_ref。
- explore 端明确 grep/repo_map 仅定位，每一侧必须打开 load-bearing control path；finalizer
  只能保留独立支持的成员和已证明 join。
- schema 明确禁止把第二机制仅塞进无 member-specific proof 的 grouped_count dimensions。
- 仍然只做 soft guidance + 既有 member_set grounding，不扫描用户/答案原文，不新增
  case/type hard gate，不影响 Trace 因果投影与自动补齐。
