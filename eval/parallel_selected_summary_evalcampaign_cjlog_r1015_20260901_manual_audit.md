# r1015 仓颉清单与混合语言日志人工审计

- date: 2026-09-02T02:31:57Z
- sweep_start_ts: 20260901-193157
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | cangjie_repomap_fixture | PASS | eval/results/cangjie_repomap_fixture-20260901-193157 | dimension_substring,answer_contains | none | 90s | 27 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=2,inv=3/0,fin_reject=1,unavail=0,prune=0 | PASS | 五个声明、类别、文件坐标和 package 均与源码一致，窄补 summary 后原表保留。 |
| 2 | hilog_mixed_arkts_cangjie | PASS | eval/results/hilog_mixed_arkts_cangjie-20260901-193157 | log_attachment,answer_contains | log_triage | 98s | 27 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=2/0,fin_reject=0,unavail=0,prune=0 | FAIL | 两种语言首帧及 caller 正确，但自添跨栈桥接/传播结论；另确认主体/二级交接解释回流 B1550。 |

## 仓颉清单

- 终稿 `.codrax/output/20260901-193325.530-99658.md`，逐字对照 fixture 的三个 `.cj` 文件。
- `extend Cart` 位于 `cart/Cart.cj:30`，`foreign func native_add` 位于 `bridge/Bridge.cj:6`；public class 为 Bridge/Cart/App，分别为 bridge/Bridge.cj:15、cart/Cart.cj:14、main.cj:11。
- 包路径来自 `package demo.bridge/demo.cart/demo.app`，不是目录推导；Item 是 struct，未误列 public class；Cart 的 class 和 extend 未混为一条。
- 原草稿把导语放在 table.text、未发 summary block，触发一次结构拒绝；模型保留原表并追加 summary。机械引用绑定只恢复 exact row-id 对应的路径/quote，不替模型选择成员。没有未知 row、跨行身份或无限修补故障。
- 最后导语重复属于模型组织/格式服从性观察，本批不为该样例增加自动删文或文本 hard gate。

## 混合日志

- 终稿 `.codrax/output/20260901-193333.364-99647.md`。ArkTS 首帧 NativeBridge.invokeOhSum:33:11、caller HomePage.computeTotal:54:7；仓颉首帧 demo.bridge.ohSum:18、caller demo.bridge.checkout:42，均正确保留。
- 预处理器首轮将邻接错误放入 Cause，因没有明确 cause marker 被拒；第二轮改为 peers。Analyzer 首轮同时声明 root_cause 和 bounded_fact_set，收到双向可行修复说明后自行改 explain/generic，保留 bounded scope。不是系统必带/必拒的矛盾合同。
- 仓库没有这些客户路径，系统未强迫检索无关 Codrax 文件，也未制造源码引用。
- 人工 FAIL：最终添加了日志未证的 NativeBridge→checkout 桥接、“同一崩溃”“包装/向上传播”结论。日志只证明各自堆栈与消息，未证明两个错误属于一条跨语言传播链；尾注反称调用关系来自日志，同样不成立。
- 明确正确帧序和跨错误未证边界已送达 finalizer，仍有模型越权残余，不能承诺仅修一个字段便消除全部问题。

## 确认的系统 B1550 与修复边界

- 虽然 B808/B809 隔离了观察 Summary，模型另填的 Subject=`Cangjie 层根本原因` 仍进入 structured context、ClaimBinding target、ObservationLedger 的 direct_observation principal claim 和 subject candidates；external seed 还以 Summary 作 Raw、Subject 作 Func。
- 用无语言/业务依赖的模拟错误先红验证 5 个消费面，既有解释确实绕过了摘要隔离；空 Evidence 还会以摘要铸 observed fact。
- 共享只读投影统一五面：多顶层错误的 unbound Subject 不作身份，Summary 只取保留的 Evidence；没有 Evidence 的观察不进入事实交接。原始 bundle 不修改，原始解释仍在审计记录；单错误、显式 Cause 子树、typed operational semantic 和 Trace 通道不变。
- 泛化回归包含：含 cause 字样的真实 Evidence 原样保留、空 Evidence、单错误/嵌套 Cause、thread snapshot、输入字节无修改，以及 analyze/explore/finalize 实际 BuildPromptContext 的二级交接一致性。
- 本批没有扫描或改写最终正文，也没有添加 Mermaid/关系内容硬门；代码状态与全仓验证结果见统一台账 §123.1624。历史失败工件原样保留。

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
