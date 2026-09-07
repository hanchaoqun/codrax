# r1035 Trace 睡眠库存与 Python 跨仓写入人工审计

- date: 2026-09-07T13:45:59Z
- sweep_start_ts: 20260907-064559
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

冻结二进制：main `6a17d2f42e5e`，干净构建；严格恰好两路。后续 B1607a 补采计数、B1614/B1615 提示修复未进入本轮二进制，不算本轮生产正证。机器原始结果不改；人工审计明确区分能力命中、答案正确性和最终交付验证权限。下表sec沿用外层批次summary；Python子进程run-1.wall为309s，外层311s，二者不混为单次模型调用时长。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | real_trace_h1_binder_true_false_attribution | PASS | eval/results/real_trace_h1_binder_true_false_attribution-20260907-064559 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 141s | 47 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail/partial | B1607a完整状态统计命中；完整Binder总账仍缺；帧间空闲/waker错并与capacity误译 |
| 2 | github_issue_tokenizers_newline_run_multirepo_py | FAIL | eval/results/github_issue_tokenizers_newline_run_multirepo_py-20260907-064559 | log_regex,write_apply,answer_regex,answer_contains | none | 311s | 28 | read=7,repo_map=1,list=2,trace=0,source_lens=0 | midloop=1,inv=0/0,fin_reject=0,unavail=0,prune=0 | fail | 原生2测试过，但单LF边界独立探针失败；最终诚实unverified，未签绿 |

## Trace：统计到睡眠不等于完成 Binder 归因

产物 `.codrax/output/20260907-064818.288-25439.md`、同名HTML及root-causes JSON均存在。HTML约3.2MB，正文、表格、Trace因果投影完整；本例图为成对text fence，不是Mermaid故不冒称Mermaid回归。sidecar为有效schema2、空root_causes、status=unavailable、reason=valid_model_root_cause_selection_unavailable：模型没有提交根因选择，系统没有替它选；必选旁路文件没有丢。

实际trace_query 5次、均同显式窗/目标；无源码工具。analyzer有2次结构拒绝后修正，成文1轮0拒绝。新库存明确给65段S、union70.338ms、32/65返回、闭合/Binder/因果未评估，explorer准确复述65/70.338后继续辨Binder，B1607a生产命中。不能据此声称完整Binder账已实现。补采入口仍执行，但已有families_present而跳过engine实采；本轮只证明入口保留，不是实际补采命中。

最终保留1.409ms下界而非全量的说明，但仍据这个下界说Binder不是主要瓶颈；并把‘查询结果容量裁剪’误译成trace capture被截断、泄漏coverage枚举。主值提示只说‘覆盖被截断’，没有截断主体，成文后系统附注才写‘结果达到容量上限’。这是typed信息入模丢域（B1615）与模型误述叠加，不新增答案prose门/不改原文。

人工原trace核验有5条目标S→同peer reply→sched_wakeup闭环，非重叠共3.094ms，后4条均小于1ms，仍属于B1607b独立于链预算的完整Binder账残余。下表是**人工源码级见证**，不是本轮系统已发布的总量；没有回写历史答案/JSON/机器PASS。行号以仓库fixture为准，附件副本行号可能+1。

| 事务 | S开始→唤醒（秒） | ms | 对端 | fixture关键行（send/receive/reply/S/wake） |
|---|---|---:|---|---|
| 12145859 | 13762.835861→13762.837270 | 1.409 | binder:496_9-10961 | 4353/4369/4442/4356/4443 |
| 12145963 | 13762.894496→13762.895420 | 0.924 | binder:496_9-10961 | 12560/12565/12616/12562/12618 |
| 12146012 | 13762.937527→13762.937595 | 0.068 | binder:227_4-10625 | 16942/16949/16975/16946/16976 |
| 12146033 | 13762.950638→13762.950758 | 0.120 | binder:496_9-10961 | 18520/18530/18548/18524/18549 |
| 12146036 | 13762.951138→13762.951711 | 0.573 | binder:496_F-11354 | 18608/18615/18666/18611/18668 |

模型探索曾把前4段预览当全部、声称无十几ms长睡眠，最终已部分纠正；仍把15.758ms（app-9511唤醒，typed pacing）与14.302ms（13763.009537..023839，DetectViewRect-17679唤醒，仅s_sleep/advisory_nearest）合成‘两段系统明确帧间空闲’，并混淆首段waker。模型质量残差记录，不给后一段凭空补typed pacing或因果证据。

算力58.320ms、IO12.658ms、反转7.405/4.710ms、调度3.956ms均仍在链上榜；真实运行157.248ms、业务span占用/新修向线索、邻近背景隔离也保留。模型业务修向总结偏弱是观察项，不通过系统代写结论补齐。

## Python：原测试绿不代表行为边界完整

持久交付`run-1.applied-tree`对应commit `a3fd0aeef22fc427569381f9b5987593e8bcfd50`；只改fastlex/tokenizer.py +19行。与git show源码SHA256一致：`325f0cbf7b580dd67ff43ad319467cbe2e61425854a35c5b9d90f2fd92fc8925`。原五LF测试、Makefile、pyproject、__init__未改；bindings-py主HEAD及其余4子仓均仍原seed/clean，两恢复ref存在，无越权合入主干。

原生Python3.9.6/_HAVE_NATIVE=False；在持久树执行原unittest两项全绿，不能把它称Rust扩展验证。独立三组unittest为2过1败：2..6LF、折叠后继续普通BPE、hi、aaaaa普通自合并、无规则和空输入均过；单LF在有(10,10,300)规则时错误返回[300]而不是[10]。根代理复算日志：`.codrax/tmp/20260907-r1035-python-native-tests.log`与`20260907-r1035-python-independent-boundaries.log`（测试脚本r1035_native_boundary_check.py）；未修改交付工件。

首轮write_analyzer把‘奇数run留尾10’写进planning-only expected_outcomes，与自身合同/原五LF测试矛盾；实际教学已明确该提议不得覆盖原请求和受保护测试。项目真实测试成功拦下首版，重规划改过五LF，但漏单LF。现有通用below-activation/min-trigger提示已在模型输入，不能因本例失败新增LF专用产品硬门。历史20260820-000947工件今天独立检查另有aaaaa错误[400]；本轮普通aaaaa已正确，不把两个缺陷混同、不改旧PASS。

系统最终`report_passed=true`而`final_verdict=unverified`、`verify_authoritative=false`，正确未签绿。累计证明另有独立系统缺口B1616：6未闭合全为首计划impact/patch_review，各3条，旧计划在controller声明的cumulative source-plan中，目标fastlex/tokenizer.py仍保留，新旧三个正式合同完整相同。verification_proof_profile.go的resolver只消解missing，不能承接unverified；patch_review只投影EvidenceRef，没有ContractRef，兄弟behavior_contract记录却已有covered。该证明投影不一致已对照真实最终JSON和代码确认，但不能把本例6行无条件改绿：实际新probe只验证五LF，原测试也没有逐项执行全部合同的行为边界。

同次审计还确认必须先补的身份边界：两代nl-collapse-probe代码不同（旧期待五LF→[300,10]，新期待[300]），ExecutedCommand却都展示`python -c <verification_probe:nl-collapse-probe>`；verificationProofCommandIdentity仅取runner/framework/cwd/suite/command，最终旧probe能力被标为superseded_by_terminal_exact_command_pass。这个占位命令相等不能证明“同一探测重跑成功”。B1616先修实际探测定义/执行代次身份，再统一同计划与controller准许的跨计划履约投影；新旧错误历史保留。普通项目测试真实重跑与旧非权威探测被原测试否定是两种不同来源，不混用同一reason。

当前目录说明确有系统缺口B1614：typed已选child正确进入上下文，但旧readmulti提示被抑制后没有替代child-relative导航，造成重复bindings-py前缀。已另批修soft上下文，权限门/用户请求不改。本轮无JSON形状失败；真正测试失败允许重规划，pytest不可用后实际unittest接替。没有因流活跃而按默认4ms/旧4分钟降级；此轮时长也不能冒充单连接超过4分钟的live证据。

## 后续优先级

1. P1 B1607b：完整Binder关联/闭合账独立于chain预算，新增5闭环真源见证；普通S、oneway、无reply/假关联维持非根因。
2. P1 B1616累计验证生命周期：先修同ID异代码probe不能当exact rerun；再以source-plan、完整typed合同、仍存目标域和最终树实际执行凭证统一三载体承接。保留失败历史，不按同ref或“测试全过”一键签绿；具体施工针见统一台账§123.1677。
3. B1614已选child上下文、B1615查询裁剪域提示：共用typed输入的soft教学，各自单元验收并推送；等待异构新回放。
4. 交付评测异构域+原生边界证据：现10个未绑定case不机械迁移；‘原回归保留’/‘实现文本’/‘原生行为’分开验收。模型单LF/帧节拍误述继续观察，不加样例关键词门。
