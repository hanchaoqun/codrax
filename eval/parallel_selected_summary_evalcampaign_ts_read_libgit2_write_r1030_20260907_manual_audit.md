# Selected Eval Manual Audit Scaffold

- date: 2026-09-07T08:24:23Z
- sweep_start_ts: 20260907-012419
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_ts_workspace_chain | PASS | eval/results/sr_ts_workspace_chain-20260907-012423 | answer_regex,answer_contains | none | 263s | 33 | read=13,repo_map=2,list=0,trace=0,source_lens=0 | midloop=8,inv=6/0,fin_reject=3,unavail=0,prune=0 | fail | 主链与paths解释大体正确，但图把CLI/API连接改成cli/api，造成额外隐式节点及断链；当前实例FixedDelay(200,3)被误称ExponentialBackoff。另有接口签名被硬要实现体的确定性过程缺陷，不能以机器PASS收账。 |
| 2 | github_issue_libgit2_foreach_worktree_symptom | FAIL | eval/results/github_issue_libgit2_foreach_worktree_symptom-20260907-012423 | write_apply,write_patch_oracle | none | 331s | 28 | read=7,repo_map=3,list=1,trace=0,source_lens=0 | midloop=3,inv=0/0,fin_reject=0,unavail=4,prune=0 | pass (patch; proof incomplete) | durable diff仅repository.c两行括号；原测试和配置未改，make check真实通过，独立原生7格补验通过。正式执行仅有aggregate而无2项逐assertion凭证，unverified诚实，人工补验不回填。已读test_repository.c的保留约束被命名门连拒3次是独立系统gap。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## 本轮人工证据与任务边界

- 二进制源码固定在已推送`6cb87aad9`，buildTime `2026-09-07T08:23:58Z`；恰好两路，不改oracle、预算或旧答案。TS 263s，C runner 331s（内部stage用时不同不能混作同一耗时）。
- TS真值：`ApiClient`构造器明确注入`new FixedDelay(200,3)`；`send`在status<500时直接返回，仅当仍有下次attempt才sleep。模型收到完整源码仍把本实例描述为指数退避，暂列模型质量观察，不能为此加答案关键词门或系统代写。最终图缺少loop/alt也列语义完整性观察；不强制每个答案都画图。
- B1592 / P1：真实parser把接口`Transport.send`签名9..9与类的真实方法同列为method。completion连续索要接口不存在的body；随后同一行definition_fact又被当作body通过，既误拒又假闭合。实际parser→preComplete双红日志`.codrax/tmp/b1592-body-probe/red.log`。根修需parser-owned三态body presence，不能把全部definition_site_only豁免或自行绑定接口实现。
- B1593 / P1：原图显式`CLI`/`API`，最初body使用正确大小写，但关系metadata误写`run -> API`；系统repair addition `ra1-47d66781f5c7a954e5454dfc`却给出`from_node_ids=[cli]`、`to_node_ids=[api,...]`，模型照教学修成小写边，后续把大写CLI视为孤立。最终语法可解析不代表调用图正确；需审计查询归一化键与源码可见node ID混用，不能由系统随意改图/改关系。
- B1594 / P1：write_analyzer已成功读取根目录`test_repository.c`，`preserve_regression_test`却因共享命名helper仅认Python的`test_`前缀而拒绝三次；模型最终删保护约束。最小修向为保护基线接受本dispatch精确已读文件，不把保护标记升级为测试身份/执行证明，不扩大共享source-role命名规则。
- C错误`src/repository.c`、`tests/repository/...`路径及`ret ||`实现猜测由模型引入，系统最初比较优先级合同正确；有一次工具预算真实耗尽，另一次已grep准确路径仍误用。记录交接精度债，不能把所有模型错写归为系统fabrication，也不先加预算掩盖合同冲突。
- C正式applied ref=`2fef2848ba6e`，scratch HEAD保留seed=`1ba26d4108c1`。回调任意非零原值返回、lookup仅负值失败，成功0；未扩大lookup正数为错误。除repository.c括号两行外，test_repository.c/Makefile/README未改。真实make check两次exit0但仅aggregate，正式callback_negative/lookup_negative仍0/2，没有本轮动态verification_probes。
- 独立原生补验存`.codrax/tmp/r1030-c-independent-955P9h`：原四格(-42,0)、(17,0)、(0,-7)、(0,0)，加lookup正数应0、callback正/负优先于lookup错误，共7/7。只用于人工判断补丁正确，不反写report/机器FAIL或铸造执行器不存在的assertion receipt。
- 下一批轮换H8显式窗投影正臂与H2有限状态查询投影负臂，继续每批两路；本批read/write没有提供新的Trace或单次>4分钟活跃流正证。
