# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T13:57:26Z
- sweep_start_ts: 20260831-065725
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_java_typo | PASS | eval/results/patch_java_typo-20260831-065726 | write_plan,write_patch_oracle | none | 55s | 27 | read=2,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确规划 `Main.java:16 retrun -> return`，状态 `pending_approval`，未修改源码；写模式守卫正常。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260831-065726 | answer_regex,answer_contains | none | 363s | 33 | read=7,repo_map=1,list=0,trace=0,source_lens=1 | midloop=11,inv=4/0,fin_reject=3,unavail=0,prune=0 | partial | B1490/B1491 均获生产正证：不存在直达路径的结论、两条真实边、22 项关键函数、引用和 Mermaid 均正确，零降级；但 typed 维度要求图在前、清单在后，最终清单块仍位于图块之前，暴露 B1492：顺序仅教学、无结构化局部修补协议。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. `patch_java_typo` 人工通过：改动范围、单行 patch、审批状态和零源码副作用均符合 oracle。
2. `qf_sequence_analyzer_gate` 的事实与关系人工通过：回答明确说明 `buildAnalysisIR` 与 `gate.Run` 无连续有向路径，只保留已证的
   `buildAnalysisIR -> gate.RunWith`、`gate.Run -> gate.RunWith`；图语法有效，22 项中间函数均有当前源码引用；无恢复稿或降级说明。
3. B1491 生产回放闭环：模型提交 producer 允许的技术端点后，执行器能归一到现有显式 participant，不再出现
   “not a typed carrier” 确定性循环。前三次 reject 分别来自初稿关系 identity/participant coverage、模型在原子 lease 下尝试整块改图、再次整块替换；
   第四轮原子 attach 成功，第五轮补 `member_set` 成功。
4. 人工总判仍为 `partial`：分析器的 schema-valid 维度顺序为 `#1 diagram, #2 member_set`，初始教学也明确“先图、图后清单”，但最终模型块顺序是
   `summary -> principal path -> member roster -> diagram`。既有覆盖器只确认两个 typed carrier 都存在，不核对其相对顺序；既有 patch 协议的
   replace/unchanged 保持原位，remove+add 同 ID 又归一为原位 replace，因此模型没有可执行的局部重排动作。登记
   `B1492-TYPEDDIMENSIONORDERPATCH1`，按 typed index + 唯一模型块所有权根修，禁止扫描用户/答案文字或让系统自行移动内容。
