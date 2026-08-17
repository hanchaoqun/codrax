# Selected Eval Manual Audit Scaffold

- date: 2026-08-17T14:49:52Z
- sweep_start_ts: 20260817-074950
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260817-074952 | write_apply,answer_regex | none | 147s | 25 | read=7,repo_map=2,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | partial | 两处目标改动均正确落地，`make check` exit=0；但该 fixture 的检查器只是 Python 源码形状检查，两条 TypeScript 生产路径都被诚实标为 `capability=source_static`。本机没有可执行目标行为的 Node/project-native runner，controller 将模型的 `all_verified` 规范化为 `accept_unverified` 是正确 fail-closed，不应靠降低验证杆修绿。 |
| 1 | qf_logic_view_read_pipeline | PASS | eval/results/qf_logic_view_read_pipeline-20260817-074952 | answer_regex,answer_contains,mermaid_edge_count | none | 404s | 41 | read=16,repo_map=3,list=0,trace=0,source_lens=1 | midloop=11,inv=6/1,fin_reject=2,unavail=0,prune=1 | fail | Runner 仅看阶段名和边数而假阳性。关系补采连续导航到 `cgec_enforcers.go` 与 `emit_investigation_complete.go` 的无关 BusContext 使用，真正的 `dispatchStage/BuildAgentContext` 载体交接未被选择；终稿把内部自检函数画进主架构，并把用户明确要求的 BusContext/Mutable 数据流降成未证断点。6 次 completion、2 次 finalizer reject、404s 是同一软导航排序 gap 的级联。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Audit conclusion

- `B983-CROSSPARTICIPANTCARRIERARGNAV1`：confirmed/implemented。载体补采候选原先只计算声明 binding、call endpoint 与 callable owner 是否命中请求参与者，不计算同一调用的其他完整实参；通用 dispatcher 把阶段身份作为 enum/constant 实参传入时，真正交接点因此与任意 BusContext helper 同分并按文件序落后。
- 最优修向只提升软导航：由已有语言无关调用参数解析器枚举完整 sibling arguments，并将其用于“一个操作触达几个独立请求参与者”的排序。它不产生 EvidenceItem、不关闭关系门、不扫描用户/模型文本、不把系统判断写入答案。
- 写案例没有新增代码 gap：没有真实目标执行能力时保持 `unverified` 是必要边界；后续在具备 Node 或项目原生 runner 的环境复放即可，不为 runner 绿灯伪造行为证明。
