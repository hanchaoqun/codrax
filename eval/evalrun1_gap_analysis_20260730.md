# EVALRUN-1 多场景 eval 批跑 gap 分析（2026-07-30）

## 批跑结果
- **通用 12 例 / 6 场景族**（trace/logtri/runtime+源码组合/qf 基本盘/github_issue/data，PARALLEL=2）：11 PASS / 1 FAIL。
- **真机 real_traces 10 例 / 8 族**（a–h，PARALLEL=2）：8 PASS / 2 FAIL（均为答案词面正则不中，非硬失败，待逐例定性——见候办）。
- 11 个 PASS 全部做了过程级审计（PASS≠绿）：1 例 clean，其余带过程 gap。

## 已根修并验证（EVALFIX-1，谓词同源/typed 逃逸族）
1. **Gap A（数据工作流 reconcile 活锁，FAIL 根因之一）**：`StageReconcileArtifacts` 唯一允许动作反复失败时无 typed 逃逸（§1.6 红线违背；与 2026-07-05 `AnswerRepairActionContracts` 注释里点名的同标本同类）。修复=facts 增加 `ReconcileFailureStreak`（typed 连击计数，成功归零、他类拒绝不隐藏），阈值 3 到达即在单源提供器 `AllowedNextActionContractsForFacts` 并入重算车道。
2. **Gap B（answer 准入谓词分叉，FAIL 根因之二）**：`HasAnswer` 门用 typed 合同谓词而 reconcile 校验器只过噪声摘要识别器——模型早期脚本发出的描述性 answer 毒化比对且永不可能通过。修复=`reconcileComparableAnswer` 复用 `ValidateAnswer` 同一合同谓词（CSP#63 同源镜像纪律）。
3. **端到端判决**：`data_json_strict_ids` 重跑 **FAIL→PASS（230s 活锁死亡 → 44s 完成）**。
4. **Gap C（分析器实体误判假阴性，qf_config_precedence 双臂）**：①一致性硬门把磁盘上真实存在的 `codrax.yaml.example` 判为幻觉（repomap 源码索引结构性不含非源码文件）→ 全额浪费一次分析器重派发；修复=文件形实体加文件系统 stat 臂（精确地面真值；raw-request verbatim 臂按在案裁定 `ProductionGateDoesNotReadRawRequest` **考虑后放弃**，typed 载体纪律维持）。②`validateExactTargets` 把「逐字在请求里但被同解析键去重吞掉」的目标误报为"非逐字"并丢弃（假诊断+丢用户意图）→ 修复=直接 raw 子串复核，逐字目标一律保留。

## 已定性、记档候办的机制族（按泛化价值排序）
- **F5 write_analyzer 精确等值合同虚构**（2/2 github_issue 例，各烧 ~20s 一轮）：门的 reject hint 已教学、初始 prompt 未教——prompt 教学前移（需过 prompt 红线 checklist），候办。
- **F4 trace 多主题重复查询**（state_churn 例：同窗口 4 次相同 root_cause_rank + 3 次同参 window_stats）：trace_query 是纯确定性函数→per-run 参数归一化 memo 缓存；伴生显示重复行 [E1][E2] 与图例合并承诺矛盾。候办（双件）。
- **F7 数值比较方向自相矛盾出厂**（blocked_reason 例："80ms 未超过 16.67ms 预算"渲染两次）：数字是精确信号→finalize 侧确定性方向校验（soft violation）可机械拦截此类；L4 BODY-vs-evidence 盲点（logtri_go nil-receiver 叙事）维持已知观察。候办。
- **F11 data 幽灵组键**（multifile 例：组键=字段名 "canonical_label" 值 47 通过 reconcile）：expected 未钉组时 reconcile 判别力为零——需查 compute_contributions 分组来源后修。候办。
- **F10 perf_triage 预处理长尾**（111s：42.5s 零工具散文轮 + 100 字节尾段独占 LLM 轮）：退化段大小地板可免 LLM 派发。候办。
- **F3 emit_analysis 字段名近失**（required_↔requested_，2/12 例各烧一轮）：strict-decode did-you-mean 一轮重试是**在案设计**（remap 明示不改错误值）——记录观测成本，prompt 侧规范名 nudge 为可选候办。
- **F8 静默降级车道**（richness facet_softened / 17 处引文重写等）：fail-open 是设计，但零 transcript 披露与披露文化有张力——候办：硬转软时一行披露。
- **F9 预算未尽的诚实欠探索**（blocked_reason：grounding floor 点名钻取、模型带披露收尾）：完成门权属模型裁定内合法——维持观察，只考虑 skill prompt 软引导。
- **真机 2 FAIL 定性**（c2_dstate_iowait 的"3 次 iowait"计数断言、e2 的短摘录无采样披露断言）：答案正文 vs 案例 bar 的逐例审读，候办（禁降 bar）。

## 过程结论
eval 面的 gap 与代码面同构：**三个已修根因全部是"谓词分叉/硬门吃噪声信号/无 typed 逃逸"三类红线的实例**——红线体系在 eval 证据下再次自证。
