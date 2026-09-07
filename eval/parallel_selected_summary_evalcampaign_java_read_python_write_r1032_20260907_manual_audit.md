# r1032 Java 读模式 / Python 写模式人工审计

- date: 2026-09-07T09:42:42Z
- sweep_start_ts: 20260907-024240
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

干净源码 `43cbded5e0ef` 构建于 `2026-09-07T09:42:24Z`，runner 于02:42:42同时启动两例，零第三路。机器2/2通过，人工核对最终答案、真实源码、上下文、持久补丁及正式验证报告；不修改旧答案、oracle或报告求绿。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | github_issue_dateutil_relativedelta_float_symptom | PASS | eval/results/github_issue_dateutil_relativedelta_float_symptom-20260907-024242 | write_apply,write_patch_oracle | none | 142s | 28 | read=5,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | pass (有界) | years/months修复及持久交付正确，原测试字节未改；正式执行1个行为probe覆盖5行为，并非运行原unittest。报告诚实suite_skipped，人工另跑原4测试通过，不回填正式证明。 |
| 1 | sr_java_annotation_route | PASS | eval/results/sr_java_annotation_route-20260907-024242 | answer_regex,answer_contains | none | 217s | 29 | read=25,repo_map=2,list=0,trace=0,source_lens=0 | midloop=10,inv=8/0,fin_reject=0,unavail=0,prune=0 | pass (核心三问) | 处理类、注册先注入后入表、反射赋值均正确，成文零拒绝；无图符合题目。探索结构补报及重复读仍有成本，声明行伪call已在入池前降为普通引用，不能误报为图边权威漏洞。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Java：核心回答正确，无图不是关系丢失

最终回答正确说明 `/stats` 由 `StatsHandler` 处理，`Router.register` 获取 `@Route.path()`，先 `injectFields` 后 `putIfAbsent`；私有 `AuditLog` 字段由 `Field.set(handler, auditLog)` 赋值。不是Spring或class-path扫描、不是注解自己执行注入，也没有把注册画成运行调用。源码中 `Main` 显式创建共享AuditLog并注册三个实例；题目未强制图，最终无Mermaid、无结构降级或答案删除，不能以无图判能力失败。

准确上下文足以支持更精确措辞：`getDeclaredFields`只本类声明字段，类型检查是`isAssignableFrom(AuditLog.class)`而非任意字段可注入；“共享实例”比“单例”更不易被理解为Singleton生命周期保证。正文未展开Main初始化、可赋值性检查及dispatch查表，但核心三问已答；这些列为模型表达/引用精度观察，不新增prose硬门或系统替换结论。引用部分落在方法/类声明行，完整源码已入模，不冒称每条声明锚独自证明其方法全部行为。

JSON/完成过程区分：分析器首次`predicate_axis=call`但缺必需endpoint载体被拒，schema已说明空端点与exact形；兼容默认只服务非call的relation场景，本例不是同一字段同时必带必拒。主探测和三个子题共25次read，8次complete；有成员数组8/6/6不齐、维度operation索引及support-ref不匹配补报，工具使用真实已读/已发引用提示修正。统计`inv=8/0`未计入所有DOWNGRADED补报，所以保留机器指标且在此单列过程，不能把0直接读成毫无修补。成文1轮、0硬拒/0patch、29%上下文，没有证明预算不足；不涨预算绕过正确结构要求。

曾怀疑声明行被包装成调用：日志1218将Router.java:22归一化成`Router.register calls register`。继续查到实际入池3221/4391以及成文5136/5484，同一`ev-9923294504a3a3a4`已经是`text_reference_fact`、空关系端点、`standalone_fact`。`stabilizeUnprovenCallAnchorAuthority`在缓存/发射前拒绝声明伪调用并降为普通文本引用，23条最终typed关系和capsule中无该self-call，正文亦未使用。预归一化日志最多属可观测性噪声，不立虚假的P1权威污染案。

确认P2系统残余B1599：初始探测完成时唯一未读建议是AuditLog（主日志约1465）；后续02:44:35及02:45:13已成功完整读取（约2502/2635、2695/2828、3839/3883），最终typed已有该类grounded定义，最终仍说“建议文件未读取”。`emit_investigation_complete.go:4713..4718`只追加无文件身份的ForcedReadCoverage标志，`termination_disclosure.go:80`据旧标志发布固定警示，没有后续同源消解。最小根修应保留同仓/同文件/范围的缺口身份，在真实读取或当前完整coverage下逐项消解；不能看到任意read就全局清空，不改模型正文、不把建议升级必读硬门。证据持久快照`.codrax/plans/read_runs/trace-1788774172699099000.json`与最终`.codrax/output/20260907-024616.641-42904.md`均保留。

其余教学观察：通用Required字段总表可与详细endpoint规则共源，减少首轮漏空载体。重复多主题读取先复核已有逐任务绑定设计，不能凭全局完成状态或问句关键词跳过独立子题。声明伪调用已由既有B255-CALLANCHORAUTH1（统一台账§123.239）挡住，不重复立案。

## Python：正式证明是一项行为检查，不是原套件已跑

持久提交 `df8d7fa43f4d8b3ec9a1bc90325784b046e19ab4` 只改`relativedelta.py`，+10/-2：构造器共用normalize，同时处理years/months；整数值float转int，非整数float抛ValueError，整数输入保留。真实故障是构造器保留float、日历运算后将float传给`date.replace`，不是`divmod`本身拒绝float。原`test_relativedelta.py`字节不变，scratch主干不被自动改动。

正式报告 `plan-1788774268718448000-42901.report.json` 的 `post_apply_verify`、最终plan/report ID与持久交付一致，completion=verified；只执行1个与当前改动/必需合同绑定的Python probe，内部覆盖whole-float月份/年份、整数月份、fractional月份/年份5项行为。项目unittest发现成功但明确标`suite_skipped`，没有project_test_observations，不能把该轮称作逐项目assertion→contract映射正控。嵌套probe规范化到顶层时原代码/引用保留，无replan/正式拒绝。

skip符合当前既裁：无累计旧失败、无精确impact-test强制续跑、无changed-path或required-ref遗漏、未改测试、改动符号已绑定；保留测试文件约束不等于“必须执行全部测试”的结构义务。1项required及soft/fallback合同有probe，7项仅供规划清单不凭自然语言变造硬证明。模型过程曾说“7个planning_only合同已满足”，而入模上下文准确标仅供规划；最终系统交付只称1个测试通过，并说明自然语言清单不代表逐项执行证明。保留该模型概括观察，不加正文扫描硬门。

根代理在已提取的同一持久字节 `run-1.applied-tree` 独立运行`python3 -B -m unittest discover -v`，原4测试全部通过，日志`.codrax/tmp/20260907-r1032-dateutil-independent-unittest.log`。这是人工补验，不回填产品report/metrics，也不冒称产品先红后绿。覆盖不等于所有数值/日期边界已穷尽；fixture本身注释含上游修向，因此属于本地化/验证正控，不是完全盲推理。

## 排队与非回归范围

未确认新的P1合同自冲突；B1599旧覆盖警示未消解是有明确生产见证的新P2，下一显示修复批优先处理。并保留B1598等待字段读者词面与检索三层计数、显示折叠的全成员原始测量凭证；C/C++原生逐断言验证能力B1561仍为独立P1设计。Python“1个测试”可后续明确区分“行为检查/项目套件未执行”，不得伪造执行记录或为一句自然语言改变既裁调度。

本轮无Trace输入，不用它验证显式窗/因果投影/自动补采；这些已有r1031原始生产见证及B1596/B1597真实Trace回归。两案均无单次超过4分钟活跃流正证，不挪用总回放时长宣称长流通过。本批不增加原文关键词门、不替模型改结论，不修改历史oracle或答案求绿。
