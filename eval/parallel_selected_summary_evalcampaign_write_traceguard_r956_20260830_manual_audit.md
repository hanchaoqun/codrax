# Selected Eval Manual Audit Scaffold

- date: 2026-08-31T05:30:21Z
- sweep_start_ts: 20260830-223019
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | patch_go_typo | PASS | eval/results/patch_go_typo-20260830-223021 | write_apply,write_patch_oracle,answer_contains | none | 89s | 27 | read=3,repo_map=0,list=1,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass | 计划为单文件单行 kind=patch；隔离 worktree 实际仅 1 增 1 删，apply checkpoint、风险决策、patch effect 和 go test 验证回执齐全，主仓未修改。 |
| 2 | real_trace_h8_semantic_edge_anchor_sentinel | PASS | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260830-223021 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 150s | 39 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | pass | 显式 10ms 窗、3 次 typed query、最终 Trace 因果投影、链上 NetworkService 第一席、类校验业务线索、实际占时/规则可消双账户和背景隔离均完整。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Manual Findings

1. write 人工通过。`run-1.plan.json` 只有 `main.go` 一个 target，`kind=patch` 的唯一 hunk 把第 25 行 `retrun` 改为 `return`；
   applied-tree 与原 fixture 的 diff 只有 1 行删除、1 行新增。风险门按 medium/auto-safe 记录，apply commit、recovery ref、effect fingerprint、
   path coverage 和 clean worktree 均有结构化回执；验证实际执行 `go test -json ./...` 且退出码 0，覆盖 `main.go`。没有碰主仓、跳过 worktree、
   跳过风险/验证门或由系统拼造可见补丁。
2. write 的 read-classifier 仍把明确修改请求标成 `intent=explain/question_kind=mechanism`，但后续独立 write analyzer 精确收敛到 micro/low-risk，
   controller 依次走 plan→apply→verify→finish，未造成路由、范围或权限错误。本批只记低优先上下文噪声，不据一次样本新增硬分类器或关键词门。
3. Trace 人工通过。显式 `34579.490..34579.500s` 窗、3 次 typed query 和最终 `Trace 因果投影` 完整；已证
   `NetworkService-60595 -> CookieMonsterCl-59843 -> com.baidu.tieba-59566` 链、5.951ms 第一席、目标四态账、实际占时/规则可消双账户、
   `VerifyClass` 类校验业务线索与邻近/背景隔离均在。非链 D/IO 保持背景，未升为主因；无 4ms、4m、轮次、上下文比例或活动流年龄降级。
4. r956 没有复放 source-inventory，因此 `B1470` 仍是 deterministic prompt/test positive，不能以本批误记 production positive。下一批应回放一个
   typed family inventory 与显式窗 Trace，检查模型是否直接消费精确 family counts，同时继续守住 Trace 红线。
