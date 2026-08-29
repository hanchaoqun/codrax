# Selected Eval Manual Audit Scaffold

- date: 2026-08-29T05:06:47Z
- sweep_start_ts: 20260828-220646
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | logtri_oversized | PASS | eval/results/logtri_oversized-20260828-220647 | log_attachment | log_triage | 128s | 27 | read=2,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 附件事实保持主答案：精确定位 `main.crashy()` 与调用者；当前仓无匹配只作为证据边界，不再升级为总体“无法判断”。 |
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260828-220647 | typed_inventory_rowset,dimension_substring,answer_contains | none | 321s | 28 | read=0,repo_map=3,list=0,trace=0,source_lens=3 | midloop=4,inv=5/1,fin_reject=1,unavail=7,prune=0 | pass | 2/2/8 三组 principal 全量保留，成员、完整路径、行号与 package 正确；无 `out_of_requested_universe` 错降级或内部 row-set 术语泄漏。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 1. `logtri_oversized`

- 人工 PASS。最终答案直接回答 panic 从 `main.crashy()`（`internal/agent/analyzer.go:100`）发出，`main.main()`
  （`:200`）是调用者；该结论只使用附件运行时栈，不伪装为当前源码证明。
- 当前 checkout 无对应文件只作为末尾证据边界披露，不再生成 `current_status_verdict`，也没有全局
  “证据不足—无法判断”。`finalizer_rejects=0`，无工具不可用或结构修补。
- B1438 获得生产正证：external-observation + optional current source 的附件事实不会被孤立的当前版本旗标摘掉主答案；
  typed required/current-source profile 的保留臂仍由单测覆盖。

## 2. `cangjie_repomap`

- 人工 PASS。最终答案完整列出 2 个 extend、2 个 foreign func、8 个 public class；每项均包含符号、完整路径、
  精确行号和 package，且三组都保持 principal 身份。
- B1437 获得生产正证：短 support ref 已安全绑定唯一 typed row；不再把 public class 错标为
  `out_of_requested_universe`，最终答案也未泄漏 `source_inventory_row_id` / principal row-set 等内部术语。
- 仍有一次成文拒绝：首稿把清单写在 section 文本中，却没有使用要求的 structured item label/hidden carrier；精确修复提示后模型原位改成结构化 items，
  第二次补丁只补 `member_set` facet。它没有改变成员、分组或结论，归为模型结构遵循/投影心智成本观察，不据此新增正文扫描、系统代写或固定列表格式硬门。
- `unavailable_tool_attempts=7` 来自模型在限定调查窗仍尝试 grep/read 等未发布工具；typed source inventory 本身已经完备，最终事实无损。
  后续异构枚举批继续观察，只有跨语言重复复现为同一确定性合同摩擦时再立通用 schema/教学修项。

## 3. 本批闭环

- runner 2/2 PASS，人工 2/2 PASS；B1437、B1438 与格式中立 eval oracle 均获得真实回放正证。
- 本批没有修改 Trace 查询、显式时间窗、链上根因、双账户、因果投影/自动补齐、JSON/Mermaid、自愈或活跃流时限。
- 没有扫描用户原文、模型 thinking/答案原文作硬门；系统没有创建、删除或改写模型结论、成员、分组或可见措辞。
