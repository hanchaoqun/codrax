# r1039 双域人工审计（2026-09-08）

源码4077ce397a2e已推送，clean make；严格两路并发，timeout=1200s，不改oracle、不跑第三例或反复单题求绿。从243个case按显式窗/完整链上与自身多机制、原生写模式、距上次回放间隔选择H7与fmt。

| case | 自动判定 | 人工判定 |
|---|---|---|
| H7全谱Trace | PASS，214s（主流程212s） | partial：核心值/投影/D明细在，模型小席位误分类，类别教学及系统显示仍有gap |
| fmt C++年份溢出write | FAIL，164s（主流程162s） | 源码和原生测试通过；正式逐项证明未闭环，unverified披露正确 |

## H7：过程、上下文与答案

工件：.codrax/output/20260908-025927.305-77376.{md,html,root-causes.json}；过程日志：eval/results/real_trace_h7_self_seat_full_spectrum-20260908-025555/run-1.logs.all.log。

- 两次模型查询与critical补采均为13762.791708..13763.024898（233.190ms），新范围进入上下文、状态、占时、投影与附注。这是精确同窗生产正证，不是r1038的20/21ms扩窗反例重放；后者有确定性入口回归。
- running74.915ms/折算65.912ms、D11段/36.757ms、blocked_reason12条/39.157ms分尺保留。dma_fence_default_w只是调用点，不铸造持有者/设备身份。真实工具结果logd全窗49.656=.033锚定+49.623邻近，本次旧oracle仍成立。
- 两轴与业务线索在：ProcessComposerFrame108.005ms、Repaint98.485ms、CommitAndGetReleaseFence69.337ms；业务占时不自动计价。JIT2.388ms保留，但下述链上措辞错误。
- 系统文本因果投影存在，unknown-parent不补造确定父边。本例没有Mermaid，不能用它验收Mermaid修复；没有活跃流短时间无正文降级。
- 模型只列8条链上根因，将#9 surfaceflinger、#10 aweme IO、#11自身runnable、#12 OS_IPC_16误归背景。日志2550–2553/2568–2569已有12席及显示上限不改资格，系统投影也仍标链上。暂记模型使用事实失误，不由系统改写表格或加正文硬门。
- B1621/P1：日志2597–2598的类别教学宣称“已观测到的链上语义工作”，但本轮JIT是adjacent。全局类别说明跨多个角色池、按type去重，不能赋予某条记录入链资格；priority类别本轮也覆盖邻近项。最小修向是类别语义中性化+逐行链路凭证，覆盖所有语义工作族及混池/调序，不能只修JIT或按第一条role定谳。
- B1619-P1a/P2：4条path同时显示已知观测窗口和“实际查询范围未明确”。该族producer的window明确来自chain.Window，但新的消费者只接受selected_window；应在明确的query producer补同源字段，不给全局resolver启用Span/普通window猜补。
- B1622/P2：可消量概览等仍把4个CPU统计组写“共4段/4次合计”，成员次数实际5+3+2+1=11。占时表已正确称4条统计记录、完整等待表正确11次；剩余消费面混淆统计组与物理次数，未修。
- B1623/P2：系统CPU摘要称最高线程CompThread_0-2955=26.444ms，但同结果线程跨CPU合计74.915ms。cpu_occupancy.go进程累加器直接从逐CPU running项取最大，没有先按线程合计；可能低报并选错最高线程。已对照代码确认，未修。
- 根因JSON稳定生成139字节，schema2、root_causes=[]、status=unavailable、reason_code=valid_model_root_cause_selection_unavailable。12个候选已给，但三次emit/patch均无模型选择；不是漏文件，不能由系统代选填满。继续观察选择教学和接口对unavailable的消费。
- 重试按原日志而非仅计数：2次analyzer结构拒绝、1次真正exploration完成拒绝（metrics的2次含一次ok=true warning）；成文首稿把caveat/id/kind/text嵌入claim_uses而JSON畸形。教学已明确claim_uses及块级facet_ids，未见同一字段必带/必拒。随后完整10块accepted，再一块metadata patch accepted，最终不是旧草稿降级；一次模型构造波动不新增硬门。
- 上下文峰值55%，无工具不可用/剪枝，不靠提预算求绿。重复范围披露的可读性可按精确query组去重，不能删除边界。

## C++：真实交付与正式证明

目录：eval/results/github_issue_fmt_tm_year_overflow_symptom-20260908-025555；交付树run-1.applied-tree。

- diff仅include/tmfmt.hpp两行：render_year参数拓为long long，tm_year在加1900前转换。README、Makefile、测试逐字未改；原121→2021、INT_MAX→2147485547断言保留。
- 正式两次make check均exit0（日志2678/3056）。审计者在最终交付树独立运行原生make check，exit0、约0.455s；不是仅用源码regex推定正确。
- 3次计划结构拒绝（多行缺end_line、空patch等）及1次protected-test重规划。首计划只是改测试名称并加边界，未降低原期望；不能描述为企图绕过测试。修补提示有精确范围，未见同一声明必带/必拒。
- 正式receipt只有make-test/check总结果，模型声明main/expect_year/large_positive缺逐断言实测身份绑定，一个model obs还错引用large_positive。因此工作流complete而completion=unverified/proof=weak是诚实结果，非修复代码失败，也非B1616b跨计划载体遗失；归并既有B1561。整套测试PASS不能自动变成每条断言已证明。
- run-1.out:266起明确“未完全验证”，交付只留隔离工作树/持久导出。上下文峰值28%，无工具不可用或剪枝，本仓main未被eval改动。

## 排队与红线

先收B1621类别解释不授入链资格、B1619-P1a已知path查询窗小批；随后B1618-P2a精确state/wait匹配、B1620字段级事实/模型说明分层。B1622组数口径、B1623最高线程跨CPU汇总、B1616b/B1561仍开放。新修复的生产回放另记，不改此次历史答案。

模型保留正文、图与根因选择；系统提供精确事实和边界。根因只限已证链上，非链上只作背景；合法小项证据不丢。不扫描原文做硬门，不因单次模型波动拟合。无默认4ms或旧4分钟无正文降级；明确取消/截止、真实字节空闲仍有效。
