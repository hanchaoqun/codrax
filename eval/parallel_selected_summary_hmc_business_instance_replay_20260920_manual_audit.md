# Business instance / IO distribution replay — manual audit

日期：2026-09-20。固定构建 `17df3e7d9fcf`，2 并行 × 1，每例 1800 秒外层预算；结果根 `eval/results/hmc_business_instance_replay_20260920`，sweep `20260920-005136`。未改 case/oracle、未启动第三例追绿。机器 1/2 PASS，人工 **0/2 PASS**；不覆盖先前两批人工 FAIL。后续 `d0e69c2cd` 的精确 memo 策略仅有确定性验收，本次 live 不为它签收。

| case | 机器 / 人工 | 秒 / 上下文 | 工具 | 过程 |
|---|---|---|---|---|
| trace_query_business_marker_io_chain | FAIL / **FAIL** | 332 / 41% | trace=13，read=0 | 分析发射5次；完成接受2/拒绝0；midloop=1；成文拒绝1、patch1；不可用工具0 |
| trace_query_io_request_latency_distribution | PASS / **FAIL** | 371 / 33% | trace=6，read=0 | 完成接受2/拒绝0；midloop=0；成文首次接受；不可用工具1 |

两例均为 Trace 读模式，不代表 mixed-source 读、写模式或 Mermaid 验收；未因活跃流短时无正文而超时降级，600/300/600 秒请求等待配置未改。以下行号对应各例 `run-1.logs.all.log` 或原输出 Markdown；私人完整运行日志不加入版本库。

## 1. 业务响应：原子查询能力已提供，实例焦点与答案验收仍未闭环

输出 `.codrax/output/20260920-005706.199-89131.md`（同名 HTML / root-causes.json）。原生事实：OpenDocument `1.000..1.050`，50ms 内运行5ms、runnable 1ms、睡眠44ms；LoadDocumentIndex `1.0045..1.0445` 为40ms。worker 请求驻留35ms，completion-closed S态阻塞31ms，随后 runnable 1ms；后台47ms不具已证唤醒链资格。

- **仍有跨窗量纲错误**：答案15行把6+1+44=51ms账户用于50ms响应；29–37行虽说明它来自51ms查询窗，不能修复头部的错误归属。38行又把31ms左端说成 issue，实际左端为 `1.009010` 的S态切入，不是 `1.005` 的issue。35ms请求驻留不能替代31ms链上阻塞量。
- **实例焦点仍未绑定**：1418–1420行正确查出两个业务span，1507–1509行后续仍使用不同探索窗。最终分类恢复 `causal_diagnosis`、工作关系true，但补齐2391–2392行因 `no_typed_target` 跳过；本例未执行root-cause-rank。新 `business_span_ref` 没有被后续调用消费，不能据首片查询接口宣称完成accepted焦点/补齐接线。
- **分析过程不能误报成系统合同矛盾**：5次发射在因果与有限效果间变动，真实修补要求是保留已请求角色及消除 `no_named_target` 与目标列表冲突。模型把附件范围 `.999..1.051` 作为显式时间范围，但用户没有给出这对数字；不应以关键词扫描问题来硬改结果。已有组合意图教学已验证到达。
- **Trace 因果投影存在，机器计数是假阴性**：答案75行标题为 `## Trace 因果投影（补充查询范围）`，98行有 `text trace-causal-projection` 围栏，worker/storage/main文本投影仍在；3387行有系统物化记录。`eval/runner_lib.sh::eval_count_trace_query_final_projection_blocks` 只认无括注或破折号后缀，故记0。原机器verdict保留，登记评测缺口；即使计数修好，本例仍因上述语义错误及将context-only关系说成直达根因而人工FAIL，不借此提为PASS。
- 成文修补要求提供缺失facet；模型用未支持的 `field=facet_ids` 而非已教的 `add_facet_id`，3477–3479行被拒，保留前版草稿。未确认必带且必拒；不开放任意patch字段，也不增重试。答案没有消失。
- `.root-causes.json` 为schema2，`status=unavailable`、`reason_code=no_selectable_typed_on_chain_candidates`、空数组。文件生成正确，但不能以空旁路代表业务诊断成功。

上下文边界：原生span数据确实到达；工具日志仅预览2000字节，新引用在实际两轮消息回归中位于3474字节消息的3141偏移，因此不可由日志搜不到token推断某条live模型没收到。真实两轮 `Explorer.Execute→adapter` 回归保护的是公开传输接缝，不是该live的完整请求录制。

## 2. IO分布：三组统计完整，正文解释仍错

输出 `.codrax/output/20260920-005745.293-89132.md`（同名 HTML / root-causes.json）。完整三组八项统计进入成文上下文3516–3518及3533–3535行，答案21/29/37与79–81行数值表正确：

| 组 | 配对数 | min / max / mean | P50 / P90 / P95 / P99（ms） |
|---|---:|---|---|
| RQ读 | 11 | 1 / 11 / 6 | 6 / 10 / 10.5 / 10.9 |
| RQ写 | 3 | 5 / 25 / 15 | 15 / 23 / 24 / 24.8 |
| BIO读 | 3 | 2 / 6 / 4 | 4 / 5.6 / 5.8 / 5.96 |

17是合格请求数量清单，不是混合三组后重新计算的总体分布。未新增全层聚合能力。

- 19/47行把 `unpaired_start=1` 解释为窗口左侧缺发起、仅完成落窗。实际fixture43行是13.900发起、全文件无完成；缺的是完成端，不是发起端。19行还混用请求数和歧义组数：3个未合格请求为1个缺完成＋2个歧义抑制，`ambiguous_cohorts=1`是这2个请求所属的1组，不是额外第4个请求。
- 35/57/59行把BIO起点称为issue，宣称含排队/磁盘/硬件IRQ，且推论RQ≤BIO。原引擎只测RQ issue→complete与BIO queue→complete；没有跨层请求同一性或硬件阶段拆分证据。
- 71/83行“P90及以上等于最大值”等解释与同页23/24/24.8<25矛盾。工具的线性分位数正确，不能改统计内核拟合文字错误。
- 17/79行将RQ/R 14个起点归到reader40，但实际也含reader41；上下文3534行明确 `issuers=all`。独立复核同时发现3516行首层摘要仍标 `thread=reader-40`，代表线程标签与总体范围存在歧义，因此不能把单线程归属完全归为模型凭空编造，组身份展示也需修复。67/73行称reader40为用户线程，仍无响应目标证据。
- 首次调用 `emit_evidence` 不在当轮工具集合（1467–1469行）。之后两次completion均接受，第二路是继续补多主题证据，不是五轮拒绝循环；2867行保留非胜出分支2份已完成工具结果。不得把模型误写数值全部归因于丢上下文或系统早收敛。
- 本例纯事实请求，无需强行补因果链；2892–2893行 `no_typed_target` 与旁路 `trace_root_cause_contract_not_active` / schema2空数组均符合权限边界。答案数字保住不等于整份答案已正确。

## 3. 分批余项（仍开放，不倒签）

1. **HMC-02.4**：仅在已接受completion中显式选注册的业务实例，按原子源/TID/窗接入补齐；取消败方、旧代次、不完整端点和未选择不授焦点，用户显式范围最高优先。请求驻留/实际阻塞/调度及后台分别保护。
2. **HMC-08.1 / 16.4**：把引擎已知RQ/BIO端点口径作为聚合组的紧凑typed元数据交付，避免依赖Top8明细展示；组总体的首层标签不能沿用代表线程冒充范围。复核上一批finalizer确实缺BIO queue端点，但本批三组数值、issuer范围notes已齐；二者不能混作“统计全丢”。`Count`还可能含孤立完成，不能泛称issue数或由Count-Paired直接推缺完成。
3. **HMC-18.5**：投影标题限定语计数已另以 `3d7f9a690` 修复；同一原报告计数0→1，20个生产标题正例、代码围栏/伪标题反例和完整runner脚本通过。原生产结果不重写，不把机器计数修正等同于人工通过；持续跨模式验收任务仍开放。
4. mixed-source读 + Python写apply两例顺延，不取消。先收本批可复现接缝，不靠重复刷这两个样例代替跨模式验证。
