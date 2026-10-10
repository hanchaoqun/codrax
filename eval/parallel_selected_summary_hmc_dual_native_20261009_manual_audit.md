# HMC §220 双侧量测与原生事实展示人工审计

- date: 2026-10-10T06:38:22Z
- sweep_start_ts: 20261009-233821
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results/hmc_dual_native_20261009

冻结代码 `bb9cd89f7`，构建后固定两例并行、各一次；机器 1/2，完整人工 0/2。机器判据、原始答案和日志保持不变。两名独立审计者与主代理对照实际源文件、最终报告和过程日志核验，不把底层确定性测试代签端到端答案。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_dual_measurement_records | FAIL | eval/results/hmc_dual_native_20261009/trace_dual_measurement_records-20261009-233823 | log_regex | none | 109s | 0 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | FAIL | 错入 data；未取量测，inspect 占位摘要却标 complete |
| 2 | log_shared_sources | PASS | eval/results/hmc_dual_native_20261009/log_shared_sources-20261009-233823 | log_regex,log_attachment | log_triage+log_query | 335s | 30 | read=0,repo_map=0,list=0,trace=0,source_lens=0 | midloop=1,inv=2/0,fin_reject=1,unavail=0,prune=0 | FAIL | 9/9 记录已到场，混合问题未启用原生默认表，手写仍有错误 |

## 双侧量测：入口能力与完成状态缺口

两个实际 SQLite 文件存在且 SHA 未变，题目只有自然需求、来源及独立窗口，不提示内部 view 或约束。预期完整原值是 baseline 13 行、current 8 行，另应保独立来源/覆盖状态。本次未进入 read analyzer 或 trace_query，不能评价 comparison 在 live 的效果。

- `run-1.logs.all.log:46–47`：首轮 `route=data`，confidence=0.90。分类器只获 CLI 附件提示，命名文件的内容能力在更晚 read preflight 才出现。
- 同日志 `99–136`：候选名册未命中，被生成 `kind=unknown` 占位且计入 consumed；`1105–1128` 真实读取因工作区路径边界被拒。保留路径保护，不把放开任意外部路径作为修复。
- 同日志 `2653–2702`、`2797–2801`：后续抽取的是占位 JSON/路径，源内容仍未读取。
- 同日志 `7868–7871`：模型最终准确返回 `partial_answer_possible` 并指出未提取量测；`8096` 系统终态仍为 `complete`。`run-1.out:86–87` 最终只有两行 `inspected 1 material(s)`，没有量测表。

归因：当前输入内容能力向首轮路由缺失；检查元数据与真实数据消费混同；partial 终态未准确交接。是已证跨流程缺陷，不是 pair schema 错误，也没有依据称模型偶发波动。挂 10.1/17.7/18.4，失败不销账。

## 多源日志：完整供给已通过，完整答案仍未通过

最终 `run-1.primary.md` 保留 app 6 条、kernel 3 条，坏日期、孤立续行及 unknown 均在；匿名 kernel L3 未继承邻行身份；observer 未归入 rx17；共享请求标识不证明因果、源码位置未验证的限制正确。原生事实供给通过。

- 过程日志 `1053` 拒绝 bounded 分类，`1085` 接受 causal scope；`2669` 向 finalizer 提供完整原生 9 行；`2863` emit 仅 5 个模型块，无原生 selector。整题 scope 的 display guard 抑制了混合问题中独立事件清单的默认表，故本例未 live 验收自动展示。
- 最终主报告第 1 行跨来源混排并写“内核层随后”，与第 3/23 行无跨时钟校准的结论冲突；第 23 行 `.002/.003` 示例误写“1秒”；第 27 行把未知进程名比较成“均不同”，混淆 `kmsg` 格式与 tag。
- `2904` patch 将普通日志记录 ID 当关系凭证，`2906` 正确拒绝；最终保留旧稿，不冒称 patch 成功。没有图，不计图语法失败。

下一修复优先按独立请求维度复用原生表，并保留关系、因果与源码义务；不是添加日志关键词提示、重写模型正文或放宽关系凭证。挂 16.4/18.4，旧 §219 FAIL 不倒签。
