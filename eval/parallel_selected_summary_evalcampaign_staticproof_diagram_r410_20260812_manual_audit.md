# Selected Eval Manual Audit Scaffold

- date: 2026-08-12T23:31:35Z
- sweep_start_ts: 20260812-163134
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | mr_poly_binding_chain | PASS | eval/results/mr_poly_binding_chain-20260812-163136 | answer_regex | none | 185s | 24 | read=2,repo_map=3,list=0,trace=0,source_lens=1 | midloop=4,inv=3/1,fin_reject=1,unavail=0,prune=0 | partial | 正文完整说明 Python→PyO3→Rust 核心、`best_merge` 和纯 Python 回退。第一稿的自绘图含无证/语义错误边，被正确拒绝；系统随后提供 5 条 typed 真边的 copy-ready skeleton，模型逐条确认这些边有 evidence 后却又自相矛盾地称前两条无记录，并主动删掉整图。精确关系 carrier 已到 finalizer，故本轮是模型修补波动，不是新的关系提取缺口；不能据此强制留图或由系统代画。 |
| 1 | github_issue_zod_prefault | FAIL | eval/results/github_issue_zod_prefault-20260812-163136 | write_apply,answer_regex | none | 388s | 24 | read=9,repo_map=2,list=0,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=3,prune=0 | fail | 初始 TypeScript 修复和首批回归测试正确。B679 路由修复使累计 source-static 缺口进入 planner，但纯 proof 批越权追加 TypeScript tests，并新增复制 `finalizeDefault` 逻辑的 Python 自证脚本；后者通过也不能证明 TypeScript 生产实现。JavaScript probe 又用 child_process 包装 npx 且 runner unavailable，最终诚实 unverified。确认 B681：纯证据批的硬边界只拒生产路径，却放行辅助/测试修改。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Findings and next batch

1. B679 的单构造器路由获得生产正证：累计 source-static 队列进入了
   `verification_proof_followup` 计划车道，不再重复同一 verify-only 静态命令。
2. 新确认 B681：proof-only 的 typed 合同要求 `changes=[]`，但 B680 只阻止生产路径。
   于是模型可修改 test/fixture/doc 或新增一份复制生产算法的脚本，再让该脚本为自己签绿；
   这是一类自证证据污染，不是 Zod 专属问题。
3. 根修把共同 emitter 早门和 scheduler 后备门统一为：exact
   `verification_proof_followup` 且无 same-batch `VerifyFailureHandoff` 时，任何
   `changes[]` 都拒绝；生产、测试、fixture、文档及其他辅助路径一视同仁。只有 typed
   probe failure 才可重开修复；`impact_and_verification_proof_followup` 不受误封。
4. 计划教学同步明确 proof-only 不得通过新增/修改辅助文件制造 oracle。判断只读 durable
   batch purpose、progress authorization、handoff batch ID 和结构化 changes；不扫描用户请求、
   patch/probe code、模型 thinking 或答案原文。
5. 本轮 JavaScript/Python probe 都是外部命令 wrapper，现有 schema 虽已明确禁止，但硬校验
   只有 runtime-family×target-path 兼容，尚无 parser-owned“直接 import/execute”证明载体。
   记为 B682（P1 设计项）：应新增 typed direct-target carrier/执行证明，而不是按
   `child_process`、`subprocess`、`npx` 等模型代码关键词硬拒。B681 先消除文件自证污染；
   B682 后续独立设计并覆盖所有已支持 probe runtime。
6. Poly 图退化不新立确定性代码 gap：finalizer 已交付五条 typed 真边，模型修补回合自行
   矛盾并删 optional 图。保留为波动观察；不得强制留图、系统代画或扫描答案词面。
7. active-stream 专审保持：adapter 声明 first-byte/byte-stall owner 时，BaseAgent 不把
   4ms terminal emit-only budget 安装成流式 deadline；SSE 任意字节均刷新活跃性。活跃链路
   即使 4ms 尚无完整答案也不会降级。精确退出/有界恢复只有 caller cancel/deadline、
   无首字节、byte stall、transport/decode failure；系统不得用 evidence 自铸替代答案。
