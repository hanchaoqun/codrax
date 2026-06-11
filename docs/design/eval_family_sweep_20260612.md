# 家族变体扫尾(2026-06-12)— u/s 族 39 案 + 两个系统修复

## 1. 结果

- **u 族 27 案全 PASS**(u1b/u3b/u4b/u5a/u5b/u6a + u7b..u7p 14 个 + u8a/u8b/u9b/u10a/u10b/u11a)。
- cond_resolve_inrequest_retry_attempts PASS。
- s 族 8 案:5 首跑 PASS;s3a/s11a 修后 PASS;s11b 残余(见 §4)。
- (工程注:zsh 不分词,`set -- $pair` 整串当 $1 导致一波误判 NO_RESULT;循环改显式成对。)

## 2. s3a — eval 基础设施污染源码(已修)

`glossary.go` 的 `ProjectSpecificIdentifierBlocklist` 把 probe-only 配置键以**可 grep 字面量 + "s3a eval case"注释**写进 shipped 源码。模型 grep 该键唯一命中即此 eval 标记,把 meta 信息当代码事实,答"键只存在于 blocklist"并停止,不去追真正的 explore_* 组机制。证据分类层防御(`RejectsPromptSupportConfigMention`)存在但被散文绕过。**修**:字面量改运行时拼接(`"explore_mid_loop" + "_hint_budget"`,lint 测试不受影响),注释去 eval 案引用改为通用"probe-only"措辞;非测试源码 0 命中。重跑 PASS。

## 3. s11a/s11b — 名近机制未验证消费点(软引导,部分修)

两案同形态:答案点名**名近但不在所问路径上消费**的符号(s11a 引 analysis_limits.go"白名单"实为 MaxPrescanRounds 文档,真门是 validateAnalyzerPrescanToolCall;s11b 引 TaskGraph 字段 RetryBudget,真预算是 orchestrator.go:2423 消费的 MaxRetriesPerStage)。**修**(skill 软引导,通用零项目词):MECHANISM WIRING 规则——问"哪个参数/门控制行为 X"时必须确认候选符号在所问路径上被消费(grep 用法点),仅 meta 表面(linter/blocklist/test fixture/docs)的命中不构成 runtime 表面证据。s11a 修后 PASS。

## 4. s11b 残余 — exact-target lock-in(下一 session 取证点)

修后重跑:答案进步(emit_analysis 命中)但以"精确目标已命中:transientRetryBudget"开头,随后**自我否定**"此机制并非 emit_analysis 未调用时触发",却仍以锁定目标组织答案。"精确目标已命中"横幅指向 exact-resolution lane(ExactResolutionContract 族)早期锁定错误目标后,模型的纠正分析无法推翻锁定。**待查**:exact-target 锁定的建立时机与可否被模型 typed 信号(答案侧自我否定)解锁——§1.6 形态。

## 5. 累计

批 1-11 + 家族扫尾:**102/106 first-or-fixed PASS**;残余 4:data-planner 路径专项 3 + s11b exact lock-in 1。read_combo 族 22 案 + trace_query 变体 6 案待扫。
