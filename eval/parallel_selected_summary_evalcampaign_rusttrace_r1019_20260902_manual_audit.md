# r1019 人工审计：旁路长度回归与图修补残余

- date: 2026-09-02T07:51:24Z
- sweep_start_ts: 20260902-005122
- total cases: 2
- parallel: 2
- timeout: 1800s per case
- results_root: eval/results

基线 `b3b122619960`，严格 2 路。已读最终 Markdown、真实输入/代码、成文与修补载荷、返回错误和最终模型上下文；机器结果不覆盖人工判定。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | real_trace_h11_cross_direction_overlap | FAIL | eval/results/real_trace_h11_cross_direction_overlap-20260902-005124 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 220s | 45 | read=0,repo_map=0,list=0,trace=4,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | fail | B1558 旁路证据超长回归；B1559 旧 oracle；正文多处口径误解，投影和业务线索在场 |
| 1 | sr_rust_cross_module_chain | PASS | eval/results/sr_rust_cross_module_chain-20260902-005124 | answer_regex | none | 301s | 49 | read=3,repo_map=2,list=0,trace=0,source_lens=0 | midloop=7,inv=1/0,fin_reject=5,unavail=0,prune=0 | partial | 主调用路径保留，但 5 次拒绝/6 次 patch；内部节点 ID 出现在图中，匹配返回箭头由模型删除 |

## Trace：系统回归与模型误解分账

- 输出 `.codrax/output/20260902-005502.157-78634.md`：`Trace 因果投影`、占用/可消除两轴、业务 traversal/measure/doFrame、链上小项和邻近/背景隔离均在。
- **B1558 / P1**：模型已选合法 candidate_id，full emit 已完成外层版本字段精确搬运；随后运行时自身拼接的 evidence[0] 超过 240 字符，整份 optional report 被拒，默认旁路只剩 139 字节 unavailable。B1557 新增长口径说明引入回归，不是模型没有选择、JSON 畸形或长答案消失。
- **B1559 / P2**：机器 FAIL 原因是未找到旧 `running x io_latency overlap 0.114ms`。当前真实 JSON 的自身 IO 家族为 47 段 / 12.658ms / resource_completion_closure=true / sum_disjoint；其 S/D 阻塞区间与自身 Running 不相交。既有 `rank_direction_axiom_live_pin_test.go` 已因 IO-CAL-1/IO-WAKE 修正这一口径，live case 未同步。不能为了旧 oracle 恢复请求驻留时间作为响应阻塞的旧错误。
- 正文错误留档，不硬改：Running 157.248ms 明显大于 sleep 70.338ms，却声称主要时间在等待；把互不重叠的 7.405+4.710=12.115ms 解释成“消除任一项覆盖全部”；12.115 排在 12.658 前；称优先级方向全为墙钟但其组成含折算；频率 558/2100/2075 写成 kHz 而非上下文的 MHz；暴露 lock_priority/typed_pairwise_disjoint/unresolved 等枚举。上下文已有数值、非重叠可加解释、不同尺限制和不代表方向独立的明确提示，尚未证明新的事实供给缺口。
- 分析阶段一次 fact_families 与 causal_diagnosis 自冲突后由模型重发修复；无新增合同矛盾证据。最终上下文约 89k/200k，未达到预算极限。

## Rust：路径正确不代表图体验闭环

- 输出 `.codrax/output/20260902-005623.564-78633.md`；已读 fixture 的 main.rs/matcher.rs/walker.rs。main→run→collect_files→walk、run→index_file→Matcher.is_match 路径与逐行工作保留。
- 首次图的真实节点是 `Matcher`，模型 metadata 却指向 `Matcher.is_match`，缺少同节点锚；并非再次证明 `m.is_match(line)` 消息解析器失效。该操作解析函数能够解析 is_match，不能把旧类型问题直接套到这次。
- 第 1 次 patch 只改列表且缺 from_identity/to_identity；第 2 次尝试整块删图被现有局部修补协议拒；后续模型采用修补候选，另一个列表调用锚身份未证又产生重试。模型最终删除匹配返回边；系统没有自动删边/代写结论。
- **B1560 / P1 待细审**：局部修补提供 `MatcherIs_match_6e635e0881767d6f` 技术节点，模型照抄但未提供可选 to_node_visible_label，现有声明适配器以节点 ID 回退显示标签，导致内部编号可见并留下未使用的 Matcher participant。B1553 仅修非图载体，这一图载体教学/显示入口仍需同类审计；修复应降低 ID 与读者标签混淆，不能系统替模型生成业务措辞或改动已写图关系。
- 5 次成文拒绝、6 次 patch（包含后续精确凭证修补）；全过程有持续流活动，未因 4ms/4min 无可见答案降级。长期成本仍需关注，但本次未证明所有拒绝都是系统错误。

## 优先级与边界

1. B1558 先修：无损分行、引用完整、真实 full→patch 入口 pin，保持 v2 上限；无法装入的极端数据仍诚实失败，不截断。
2. B1559 更新过时的 eval 口径，保留旧 FAIL 历史和独立正/负重叠回归。
3. B1560 对齐图修补标签教学；B1554 泛型调用提取矩阵继续开放，不混成一个错误。
4. 下一对优先 Trace 旁路复放 + 写模式异构用例；不因本轮模型的措辞/算术错误反复硬拟合同一问。
