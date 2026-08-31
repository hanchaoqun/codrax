# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T15:10:05Z
- sweep_start_ts: 20260831-081004
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | patch_java_typo | PASS | eval/results/patch_java_typo-20260831-081005 | write_plan,write_patch_oracle | none | 47s | 27 | read=1,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 精确规划 `Main.java:16 retrun -> return`，保持 `pending_approval`，未修改源码；写模式守卫无回归。 |
| 1 | qf_sequence_analyzer_gate | PASS | eval/results/qf_sequence_analyzer_gate-20260831-081005 | answer_regex,answer_contains | none | 333s | 33 | read=5,repo_map=1,list=1,trace=0,source_lens=0 | midloop=12,inv=3/2,fin_reject=3,unavail=0,prune=0 | pass | 正常结构化出厂；明确无连续有向路径，只画两条已证汇聚边，Mermaid 在关键函数清单之前，无别名/occurrence 循环或恢复稿说明。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

1. `patch_java_typo` 人工通过：改动范围、单行 patch、审批状态和零源码副作用均符合 oracle。
2. `qf_sequence_analyzer_gate` 人工通过。最终答案明确说明 `buildAnalysisIR` 与 `gate.Run` 之间没有连续有向调用路径，只展示
   `buildAnalysisIR -> RunWith` 与 `gate.Run -> RunWith` 两条已证局部关系；Mermaid 语法有效，随后列出 13 个有当前源码引用的关键函数。
   页面无“最终重试失败”、降级或恢复稿说明。
3. B1492 获得生产正证：最终模型块顺序为 `summary -> diagram -> boundary path -> key-function roster`，图位于另行要求的关键函数清单之前。
   顺序来自模型提交的结构化块次序，系统没有移动或改写块内容。
4. B1493 获得生产正证。初稿使用已有声明 `n1 as agent.buildAnalysisIR / n2 as RunWith / n3 as gate.Run`；第二次局部补丁能直接用
   `n1/n2/n3` 绑定 typed additions 并被执行器接受。r976 的 `not a typed carrier`、技术 ID/可见 alias 相互矛盾和重复 participant 均未复发。
5. B1494 本轮没有自然命中 normalized-view `body_occurrence=2` 分支，日志中也没有 `exceeds 1 visible edge`。因此只能维持 executor-level
   regression positive，不能用本轮 2/2 PASS 宣称 production branch closed；后续自然命中时再收生产证据。
6. 仍有 3 次 finalizer reject、4 次 patch。第一次是模型漏 hidden anchors/未证边界；第二次模型把原子 add 与整块 diagram replacement 混用，
   被 preserve-unlisted-edges 合同正确拒绝，下一轮只用原子 alias 后通过；最后补边界与 `member_set` facet。它没有形成矛盾合同或降级，暂按模型
   一次过度修补观察，不增加基于答案文字、标签或单 case 的硬门。
7. 本批没有 Trace case，不能据此更新 Trace 生产状态；下一批必须恰好 2 路包含显式窗 Trace，继续守住因果投影、自动补齐、typed 链上根因、
   实际占时/规则可消双账户与邻近背景隔离。
