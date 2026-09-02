# r1014 Python 关系与 Trace 全谱人工审计

- date: 2026-09-02T01:45:17Z
- sweep_start_ts: 20260901-184515
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | sr_py_registry_dispatch | PASS | eval/results/sr_py_registry_dispatch-20260901-184517 | answer_regex,answer_contains | none | 140s | 36 | read=2,repo_map=1,list=0,trace=0,source_lens=0 | midloop=5,inv=1/0,fin_reject=3,unavail=0,prune=0 | pass (core); partial (presentation/process) | 核心解析、注册、回调分离，未丢关系；三次成文拒绝均有可行修复路径。内部化 receipt 文案仍待观察。 |
| 2 | real_trace_h7_self_seat_full_spectrum | PASS | eval/results/real_trace_h7_self_seat_full_spectrum-20260901-184517 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 220s | 50 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | fail | 模型有自行求和/优先级方向/资源推断残余；另确认系统占用表统计口径 B1549，已单独施工，不混同模型问题。 |

## Python 逐轮与关系

- 工件 `.codrax/output/20260901-184735.457-87506.md`。
- 查看全部 fixture 源码：kind=json 查 REGISTRY 得到 JsonPlugin 实例；装饰器导入时注册；executor 接收 handle 回调。终稿保留对应 call/register/callback 三种关系，模型选择有向列表而不是 Mermaid，不能把“未画图”当作图丢失。
- 第 1 次拒绝：模型自选 call_edge/registration_edge 而未填 edge_anchors。第 2 次拒绝：模型伪造第三个 addition_ref；正确拒绝，不能猜测其意图修引用。第 3 次拒绝：更正 callback_handoff 后仍漏该边。随后窄补完成，未删正文或关系。
- 回调证据明确为 loop.run_in_executor→handle，未强制将其说成 run_pipeline 的直接调用。不存在本例已证证据却被消息参数 resolve(json) 污染的旧矛盾。
- 尾部“概念目标核对”是模型选择 BasePlugin.handle→dict 和 supported receipt 后的绑定披露；系统未代选结论。该叶操作不能独立证明整段处理成功，措辞偏内部化，记为表达/选择质量观察；不因抽样一次就删此通用事实边界合同。

## H7 逐轮与双维

- 工件 `.codrax/output/20260901-184855.385-87505.md`；同名根因 JSON status=unavailable/reason=valid_model_root_cause_selection_unavailable，合法空数组按 B1548 必交付。没有以 JSON 空值删去 Trace 因果投影。
- 三次查询为 window_stats、wakeup_chain、root_cause_rank，均绑定相同用户窗/线程。一次成文拒绝是模型将仅属于 summary 的 trace_causal_claim_caliber 放入 section；纠正后继续，无无限重试或活跃流时间降级。
- 实际占时 74.915ms 与规则折算 65.912ms 分账，D 等待 36.757ms/IO0 保留，完整等待清单 11 次；业务 span、链上微贡献、未计价占用、邻近49.623ms的二分与未完整枚举均在。
- 模型残余：自行合计“反转约10ms”；41 对 52 的 OHOS 优先级方向错述；把所有归因口径统称最大可消；从 dma_fence 名称外推资源类别；仍输出内部术语。对应正确上下文均已送达，不新增模型正文硬门或系统改答案。
- **确定系统缺陷 B1549**：系统占用表将 74.915ms 累计运行写成单次最大74.915ms/次数1；D聚合组写成次数4，与11个物理区间不同。代码直接在缺省时用 cumulative fallback，属确定性口径伪造。

## B1549 修复验证

- 先红：单行累计、family 缺最大、merge 缺最大、表格一行即一次四臂实测失败。
- 根修只影响占用表的统计单元：不从累计值补单次；已有合并数据标记为统计记录/记录最大，未提供为 —；业务 span Count/MaxMS 继续是逐个 span。中文/英文同构。
- 泛化矩阵和真实东湖 query→projection→tree→occupancy 确定性回放通过：running累计74.915ms仍在，最长/次数为未提供；D累计36.757ms仍在，4条统计记录不冒充4次；业务11次/11.393ms等原值不变。根因排名、65.912ms定价和模型正文未变。
- 全仓回归结果与提交状态见统一账 §123.1622；此处保留修复前生产工件，不把历史答案伪装为已修复。

## 审计原则

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.
