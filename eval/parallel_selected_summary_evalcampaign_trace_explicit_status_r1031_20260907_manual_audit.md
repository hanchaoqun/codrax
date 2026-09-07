# r1031 Trace 双路人工审计

- date: 2026-09-07T09:08:04Z
- sweep_start_ts: 20260907-020803
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

生产源码固定为 `e93028b287a4`，干净构建时间 `2026-09-07T09:07:57Z`，runner 单次快照后严格并行 2 例。机器原判 0/2 保留；下文区分能力回归、模型错误与评测口径陈旧，不改旧答案或 oracle 求绿。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h2_dstate_dma_fence_triform | FAIL | eval/results/real_trace_h2_dstate_dma_fence_triform-20260907-020804 | log_regex,typed_trace_projection_count,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 126s | 32 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | partial | 11 段 D-state/36.757ms 与全部明细正确；调用点不冒充资源对象，有限状态查询零投影。正文将已归账 231.794ms 当总窗，准确上下文已披露缺口 1.396ms。缺另一口径 12/39.157 及旧词形要求，不能等同 D-state 计算失败。 |
| 1 | real_trace_h8_semantic_edge_anchor_sentinel | FAIL | eval/results/real_trace_h8_semantic_edge_anchor_sentinel-20260907-020804 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 326s | 51 | read=0,repo_map=0,list=0,trace=11,source_lens=0 | midloop=1,inv=2/2,fin_reject=0,unavail=0,prune=0 | partial | 10ms 投影、六名链上候选、业务线索与两轴/旁路保留；语义边前 .285ms 符合最新裁定，旧 oracle 要求退役措辞。模型局部列表却把唤醒方向说反并将多段包络说成连续状态；系统次级显示面另有榜域/占时口径待核问题。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## H2：状态查询主体保留；模型忽略了已给出的覆盖边界

原始 `eval/fixtures/real_traces/donghu.ftrace` 与不可变附件逐段核对；原文件前置来源头使附件行号整体 +1，不能混用引用行号。

- 请求窗 13762.791708–13763.024898，长度 **233.190ms**；11 个可完整配对 D 段合计 **36.757ms**，区间与时长全部正确、均 `iowait=0`。
- 独立 12 条 `sched_blocked_reason pid=2955` 的 delay 合计 **39.157ms**，其中第一条结束于窗头附近但没有窗内起始 D 切换；这是内核记录数/内核 delay 口径，不是 12 个已证窗内 D 区间，也不能拿 39.157 替换 36.757。模型未列这一补充口径，保留机器缺项结果。
- 第一个可归账调度状态起于 13762.793104，故状态账合计 231.794ms、未归账 1.396ms。finalizer 日志 1716/1863 已逐字提供“部分覆盖，仍有未计入时间”；模型在 1898 的原始提交却将 231.794ms 称作总窗。属于准确上下文下的模型口径误读，不能系统改正文或加原文数字扫描硬门。
- 模型明确说明 `dma_fence_default_w` 是调用点，不是已知的具体 GPU 命令槽或硬件 fence 对象；没有把 D-state 当 IO 等待。最终事实附注为“内核等待调用点=”，oracle 仍只接受“内核调用点=”，是词形不等价检查过窄，不能判调用点信息缺失。
- 三次模型 trace_query（一次 event_search、两次 timeline），系统按有限问题补跑 window_stats。全部 11 条等待明细经完整 typed 账进入成文，未因摘要仅显示 8 条而丢掉其余 3 条。没有根因视图硬义务、因果投影 0，符合当前请求。分析阶段 3 次 emit 才收敛：先缺 completeness_obligation，后 root_cause 与 bounded_fact_set 自报冲突；修复指引一致，非“同一声明同时必带必拒”。

显示债另列：系统明细仍暴露 `d_sleep`/`caller`/`iowait=` 等内部词；event_search 的 `enumeration_complete=true` 是完整计数，不是全部返回（40/1639），真正 Compactions/枚举权限诚实为 incomplete。应优化两种“完整”的解释，不改原 JSON 字段含义、不将返回截断误报为范围未扫描。

## H8：最新语义边前计价有效，主投影未消失；人审仍非全通过

最终工件 `.codrax/output/20260907-021328.790-27748.{md,html,root-causes.json}`。main `e93028b287a4` 下，实际因果投影块 1，程序化旁路含 6 个模型选中根因；没有把邻近/背景根因加入这 6 名。

最新裁定 `real_trace_campaign` §29.88.1/.2、同事审计 §40.28 恢复语义工作的已证宿主唤醒边前份计价；`rank_semantic_edge_anchor_r3.go` 当前实现相符。VerifyClass 在 tid61839 的 34579.495841–34579.496126，占 0.285ms；该线程在 .496810 直接唤醒目标，整个 span 在边前，当前规则值 0.285ms 合法。它不等于已证明“工作完成触发唤醒/目标等待该工作/造成丢帧”；最终关系判断明确保留三项未证。case 仍要求旧“仅关系凭证”“优化项,非根因”，属于陈旧 oracle，不得为了通过把合法值清零或删除链上语义工作。

主榜最大项 NetworkService-60595 原链上量 6.406ms、规则量 5.950875ms（runnable 5.930 + running 供给折算 0.020875）；其后 .285、.121、.11555、.105、.050697。投影按链上规则量排序并保留帧因果未证、跨方向不可加、目标全窗 1.156/0.183/8.661ms 三态；业务锁 span 保留为链上业务排查线索，后台 CPU/IO 不升格主因。

人审真实错误：模型原始 emit 日志约2702已含将 NetworkService→CookieMonster 反说成“Network 被 Cookie 唤醒”、将 Cookie→主线程反说成“Cookie 被主线程唤醒”；同一正文另外两处方向正确。实际入模有正确有向边（约2008/2013/2157），不是系统替换导致。模型还将多个 runnable 小段总量 .105ms 的 .494869–.496442 包络写成连续 runnable、把供给折算称作独立修向而未充分区分收益独立性。先列模型质量观察，不扫正文重写/硬拒。

JSON 过程：负观察首次缺 origin/target/scope，工具给同一套完整结构后重报；成文一次模型 patch 补精确 runtime_work_relation 等元数据，原正文所有权保持，无 Mermaid 图（文本树不是语法失败），没有为无图强加图义务。机器 inv=2/2、fin_reject=0、patch=1 如实保留，不由指标名猜两次永久闭环失败。

系统显示残余另在统一台账单列：占时表中的 effective/raw 选择、代表性时间窗把邻近行放在“链上项目”标题下、事实对照名次缺链上/邻近榜域，均需读取实际 typed producer 后处理；不能用本轮主投影正确就宣称全页已闭环。

## 后续优先级

先闭环确定性接线/显示权限问题；不重跑 H8 直到随机命中旧词形。下个两路轮换优先 `sr_java_annotation_route`（久未跑的反射注入/注册与请求期、接口无体边界）和 `github_issue_dateutil_relativedelta_float_symptom`（本机 Python 原生 unittest，检查正式逐义务凭证）；C/C++ 原生 assertion receipt 仍是比重复同题求绿更有价值的设计债。保持原预算，只有日志证明预算不足再调整。
