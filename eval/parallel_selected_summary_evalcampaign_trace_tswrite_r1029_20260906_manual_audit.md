# Selected Eval Manual Audit Scaffold

- date: 2026-09-07T06:47:19Z
- sweep_start_ts: 20260906-234718
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_memoclaw_text_search_multirepo_ts | FAIL | eval/results/github_issue_memoclaw_text_search_multirepo_ts-20260906-234719 | log_regex,write_apply,write_patch_oracle | none | 134s | 28 | read=7,repo_map=2,list=1,trace=0,source_lens=1 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | 补丁行为通过；正式证明不足 | 仅client.ts+6/-7；make check只静态检查，6缺证义务诚实weak；独立Node7/7不回填；B1589否定语义被软化丢失 |
| 1 | real_trace_h7_self_seat_full_spectrum | FAIL | eval/results/real_trace_h7_self_seat_full_spectrum-20260906-234719 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 218s | 52 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=1,inv=1/0,fin_reject=0,unavail=1,prune=0 | 核心能力通过；结论部分不准确 | 两轴/投影/6项JSON保留；合法参数变化触发旧数值oracle；B1590a教学冲突、b查询榜混排；模型D/IO、枚举/合计表述待改 |

## 人工审计（冻结22299b53ed34，原结果/报告/答案不改）

### TypeScript跨仓写

仅focus SDK src/client.ts +6/-7；工作树提交cbfe635fd3c1757d27849ab1d53ac976b9f02f52，原仓HEAD仍9925c3148af7a0cb07b0fb7784af7edf70c757e9。API reference、tests、Makefile、package.json哈希与预检相同，两兄弟仓及最终工作树干净。POST /v1/search + JSON保留default limit/namespace/异步返回，无额外main合并。

正式make check45ms执行Python源码字符串检查，只产生aggregate make-test；check_search_client声明没有独立assertion receipt。三条必需合同形成6缺证义务，9项账本covered2/advisory1/missing6；系统保留unverified/proof_weak，未按模型all_verified签绿。稳定静态且缺直接运行时后停止重复验证属已有诚实出口，不是B1578触发或B1575成功probe。

独立检查用已安装Node v24.19.0导入真实保留工作树、mock fetch、零网络：默认limit+特殊query、namespace+显式limit、limit=0、空query/空namespace、namespace+默认limit及fetch/json异常分别传播7/7通过；精确URL/method/header/body与返回对象均核验。这不是tsc类型检查，也不是产品执行凭证，未写回report。

新B1589：all.log多处`id=no-query-string operator=satisfies expected=GET with URLSearchParams for search params polarity=expected planning_only=true`；模型原为not_contains，无grounded ref，被write_analysis_quality语义改写。补丁此回仍正确，不将该风险冒称本次weak之因；修向是保留否定operator、仅降required权限。

### H7 Trace

答案`.codrax/output/20260906-235054.086-55965.md`和同名root-causes.json；4个query均目标2955、显式13762.791708..13763.024898。完整投影/自身运行74.915和供给折算65.912/非IO D36.757/自身runnable/业务span/未计价/小贡献幸存，JSON6项status=available，3反转候选带lower_priority_dependency_candidate与分解，D类别sleep_blocking且明确非IO。49.638仅邻近，未冒充链上根因；无关JIT明确未绑定。

补采1222–1225按families_present跳过重复引擎；不是能力消失。系统枚举保留12/52、12/56、12/67及critical20/85各自范围，未入榜⛓13/未计价19诚实。模型原4blocks经自身一次replace patch保留，初稿columns被模型漏发，最终中性列标题，非系统删表头；JSON别名安全迁移不改可见值，0hardreject。

默认query=0a9b0ae9/board2609fdd5二分49.656=0.033+49.623，max_chain_nodes48 query=87eefda8/board5301603e原始结果二分0.018+49.638。最终树E44标后者域，故固定默认oracle FAIL不足以判引擎回归。真实B1590b是context丢board参数分组，混同#5/#6与2.029双行且称单一排序。B1590a是同prompt旧max-only与精确方向subtotal5.324矛盾。

模型最终仍把running称最大墙钟（S118.586更大），D/IO混称、未完整枚举称全、跨席合计；对已有精确信息的部分留模型波动，不扫正文硬门或替写。单次活跃流45.126/35.701s自然完成，无超时/降级，不能当>4分钟live证据。旧unavail指标需按日志拒绝归因，不等于trace不可用；无读代码绕开trace-only范围。

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
