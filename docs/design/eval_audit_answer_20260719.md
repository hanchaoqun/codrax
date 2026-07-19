# 答案审计 — eval 20260719 批(10 案 ×2 run)

审计口径:逐 run 读 post-━━━ 答案体(与 verdict 同源字节),trace 数值对 fixture/PROFILE.md 复算,读/写案对源码与 applied ref 复算。禁信 PASS。

## 1. grounding 手算抽验

### c2 (real_trace_c2_dstate_iowait) — 真值:PROFILE §1.8 = 恰 3 条 sched_blocked_reason(pid=59566),**全部 iowait=1**,全部 caller=sync_buffer_read_wi,ts 451840/453081/471723;raw grep 复核一致(fixture 行 118/250/2533)
- **run-1 值错(P1)**:答案称「3 次 D 状态,其中 2 次为 IO 等待,1 次无内核 blocked_reason 记录」+「第3次…无 blocked_reason 记录」——第三条 blocked_reason(34579.471723, iowait=1, sync_buffer_read_wi)在 trace 里明确存在,模型在 explore 第 6 轮 think 里**亲眼看到 line 2533/2534 又把它丢弃**。D 总量称 1.656ms(把 thread_timeline 的 running 段 450627..451701 = 1.074ms 误记为 D 段),io_wait 总量称 0.285ms;引擎口径 0.635ms、probe 口径 0.635ms(d_sleep 0.488+io 0.147)。另有无据引申「sync_buffer_read_wi 对应 fscache 页面缓存同步读」(trace 无此关联;fscache_page_wait_o 是另一 caller 族)。
- **run-2 全对**:3 次全 iowait=1、0.138/0.147/0.35、Σ=0.635ms、caller 一致;唤醒计数 34×CookieMonsterCl/8×T7@Zeus/3×IRQ/总 48 —— raw grep 逐项复核全中。
- 同案两 run 结论互相矛盾(run-1「2+1 无记录」vs run-2「3 次全 IO」),EXPECT 两 run 都盖章 PASS。

### a3 (whole_trace_overview) — 真值:§1.7 full 行 running 50.524 / runnable 5.529 / s_sleep 85.915 / d 0.488 / io 0.147(Σ=142.603,窗 144.557,缺口 1.954ms)
- 两 run 答案主线程 running=52.478ms = 50.524+1.954:引擎四态守恒把 **1.954ms 未覆盖缺口整段折进 running**。案头 oracle band「50–51ms」实际未命中(52.478 出 band)。
- run-2 额外错句:「running 52.478ms,占其总 window(58.025ms)的约 90%」——58.025 是 process_cpu_load 的 top_thread 值,被 LLM 硬说成「总 window」,编造比值关系(窗是 144.557)。
- top 线程值 sysevent_store 60.009/hilogd.pst 54.308 vs probe 59.108/53.421 —— 同一 padding 家族口径漂移,叙事未披露口径。

### 短窗 runnable (donghu_real_short_runnable) — 真值:§1.7 jank 行 + raw
- sleep 2.978 / runnable 0.014 / running 0 ✓;CookieMonsterCl 474032 被唤醒→475693 切入 = runnable 1.661ms ✓(raw 逐行复核);475843 唤醒边 ✓。引擎 running=0.483 = 0.333+(475693..475843 边前 0.150) —— 边即凭证裁边正确(ONCHAIN 语义落地实证)。
- run-1 叙事把 0.483 说成「随后才 running 0.483ms 最终发出唤醒」(实际 0.333ms 在 runnable 段之前)——次序小误,数值忠于引擎。两 run 全部关键值有据。

### frame_multicausal — 真值:§1.7 legacy115 行 + raw
- 四态 running 26.946(=24.992+1.954 同 padding)/runnable 3.636 ✓/sleep 84.358 ✓;blocked_reason 17×fscache_page_wait_o + 1×hmfs_get_dnode + 1×hmfs_read(raw grep=19 条)与答案逐条吻合;E20 同源二分 0.445=0.370⛓+0.075◇ 与 §29.139 账本手算值逐字一致(新凭证机制真板复现)。
- run-2 错句(P2):「runnable 态等待 1.743ms」——1.743 是引擎某单段观测(cpu=1 段 summary),窗口 runnable 实为 3.636(引擎席行 3.615+0.021);LLM 拿段值冒充窗口总量。

### semantic_span(合成) — 手算:app sleep 5.0/runnable 0.8/running 1.2,span 5.0004..5.0054,唤醒边 5.005
- 四态 1.2+0.8+5.0=7.0 ✓;span 席值 5.000、有效归因 4.600=边前份(5.0004..5.005) ✓ 边即凭证算术正确。
- **run-2 叙事曲解(P2)**:标题句「丢帧的根因是优先级反转」——引擎主根因=类校验 4.6ms(修向 自身工作量),树内无任何优先级反转席;app 只在 runnable 0.8ms 里受调度影响,worker 在不同 CPU 上、app 处于 sleep。run-1 同案正确论证了「优先级倒置未形成实际阻塞,真正原因是 5ms 类校验」。run-2 与自己贴出的投影自相矛盾。
- run-1 首句小瑕:「直到 5.005800 才被 worker-200 唤醒」(唤醒在 5.005000,5.0058 是调度切入;正文列表项自己写对了)。

### 读 3 案
- gson:两 run diff 均为基于 value 的 equals/hashCode(与上游语义一致),verify report_passed=true,PASS 实至名归。
- data_multifile run-1:17,0,5 手算 ✓(GroupA=10+7,GroupX=0,GroupC=5)。
- zod:见 §4(两 FAIL 均归因到机制,一真一假)。

## 2. 新词面产线表现表(§29.124-§29.139 首次真 LLM 链实战)

| 词面 | 实渲证据 | 判定 |
|---|---|---|
| ◎ 方向分组(节头/「方向间收益不可相加」/守恒尾行) | c2 run-2:139-154(四方向节+守恒 pass 行);multicausal ×2;short_runnable ×2(「小计 1.675ms(区间互斥)」=L1 档,1.661+0.014 恒等 ✓);semantic_span ×2 | **实渲全正确**:节序=最大可消降序 ✓、L1/L2 小计档位按重叠切换 ✓、◇ 块头「不入方向守恒」✓、守恒尾行恒发 ✓。**LLM 叙事零消费**(6 个大答案 narrative 里 最大可消/修复方向/方向词 0 次出现)——没曲解也没转述,新面目前是纯 engine 区显示 |
| SPANTOP top3 子行+余行 | 全 20 run **零实渲**(所有窗口只有单一 VerifyClass span,无 ≥2 成员 span 族席) | **未被本轮案单覆盖**(coverage absence,非缺陷) |
| ∩ 互指句/「同段收益不叠加」 | 零实渲(∩ 只出现在图例文字里;本批各板无 typed 跨方向重叠对) | 未覆盖(AXIOM-V2 活体 witness 在 17267 板,不在本案单) |
| 凭证词(边锚定/同源二分/合计还原全窗账/无链上凭证整席降道) | multicausal run-1 E19/E20/E24(边锚定+最晚边 34579.496810 逐字);E20 同源二分 0.445=0.370+0.075(§29.139 账本同值);◇ E25-E27「无链上凭证(整席降道)」 | 实渲正确+数值恒等式手算成立;LLM 叙事未转述(未消费,无失真) |
| 修向词(闭集 7 向) | 全部 ⛓ 席行佩「修向 X」,词全在闭集(锁与优先级/IO与依赖/调度供给/频率与热治理/自身工作量);◇ 行「·方向=X」转录 ✓ | 实渲正确;叙事零消费 |
| 账目关系/不可相加指针 | c2 run-2 E9↔E10、E16↔E11、E12↔E14;multicausal E18↔E28(「物理时间不相交·账目关系」) | 实渲正确、双向互指齐全 |

## 3. PASS 掩盖发现(按严重度)

1. **a3 oracle 假阳(P1, oracle 面)**:EXPECT_MATCHES_REGEX 第二行 `(5[01]\.[0-9]+|5[01] ?(ms|毫秒))` 两 run 全靠「Trace 指标快照」里 udk-irq-2-64 的 **`runnable 0.350ms` 中的子串 "50ms"** 命中(复放验证:全文无 50.x/51.x)。主线程 running 答案值 52.478 实际**出了案定 50-51 band**,oracle 无声放行。EVALFIX h5 的数字边界守卫只装在 EXPECT_CONTAINS 通道,EXPECT_MATCHES_REGEX 通道无守卫。
2. **c2 run-1 值错三连未被 EXPECT 盖住(P1, 答案面)**:「第3次无 blocked_reason」「D 总 1.656ms」「io 0.285ms」全错(§1 详),EXPECT 只查 caller 词+时间戳前缀+「0.x ms 量级小」,三错全漏。量级仍诚实(小),但「内核记录的等待原因」这一问答错了 1/3。
3. **multicausal 代表窗时间戳靠机器区凑数(P3, oracle 面)**:EXPECT 的 34579.525/546/576 两 run 均只在证据索引/明细 dump 行命中;LLM 的「代表性时间窗」小节选了别的窗。oracle 意图(叙事含代表窗)被 typed dump 面代偿。
4. **semantic_span run-2 根因词错但 PASS(P2)**:EXPECT 第二行允许 `(worker).*(唤醒|阻塞)` 型共现,不检验机制词;「根因是优先级反转」曲解通过。
5. a3 run-2「58.025=总 window」编造比值、multicausal run-2「runnable 1.743ms」段值冒充窗口值 —— EXPECT 均无感。

## 4. FAIL 归因(全部到机制,零 LLM-flake)

### zod ×2 write_report_failed — 两层确定性机制,一真败一假败
- **共同凶手(引擎 bug,P0)**:第二轮 verify 时 `run_tests` 走「inherited scoped verify target from typed verify-failure handoff: runner=make framework= working_dir=. suite="make"」→ 把 **runner 名当 make target** 执行 `make make` → "No rule to make target 'make'" → 判 UNAVAILABLE/parser_error(正确目标是 `make check`,首轮就是这么跑的)。日志:run-1 all.log:4695-4696,run-2:3770-3772。
- **答案面谎(P1)**:终答「本地验证环境缺少测试运行器或依赖…没有断言验证过」——环境完好,首轮 `make check` 真跑过且**真拒绝**(run-1 首轮输出 "missing regression test…"),把断言失败/自伤当环境缺失报给用户,并引导 /merge 未验证改动。
- **run-1 独有(P0)**:首个 plan(核心修复 to-json-schema.ts)checkpoint commit 失败——「git add: pathspec 'check_prefault_schema.py' did not match any files」(计划含从未创建的幽灵路径,git add 整批 abort,仅 WARN,log:2607)→ 核心修复**永远没进 durable ref**;最终 applied ref 5680b1c(parent=seed)只有 21 行测试。材料化复放:`make check` → **"implementation still uses truthiness"** FAIL。用户照终答 cherry-pick 会得到「加了会失败的测试、没修 bug」。
- **run-2 假败**:两 plan 链式 ref(af4f6ea 核心修复→cd2c0a2 测试),材料化复放 `make check` → **"Zod falsy prefault regression checks passed"**。代码完全正确,仅因 `make make` bug 报告 report_passed=false。另:终答只给最后一个 ref 的 cherry-pick 指令,单 commit cherry-pick 只带测试改动——多 plan 会话落地指引不完整(P2)。
- 复放工件:scratchpad/zod_r1(FAIL 复现)、scratchpad/zod_r2(PASS 复现)。

### data_multifile run-2 — 数据工作流阶段门 vs 台账契约互锁(P1)
- validate 要求 `coverage_contract.entity_resolution_required=true → result.entity_resolutions 非空`;`assemble_answer` 不产 entity_resolutions;能产的 `normalize_entities` 在 next_stage=emit_output_contract_answer 已被 allowed_next_actions=[reconcile_artifacts, assemble_answer] 拒收。repair hint 还在喊「Emit the required generic ledger」——**修复指示与阶段门互相矛盾**,13 批+6 修后 terminal failed(诚实 fail-loud,零错误答案出厂)。run-1 早批就带出 ledger 所以 PASS——同案掷币形,DL 家族新亚种(阶段门吞掉唯一能清偿义务的动作)。

## 5. GAP 清单

| # | 形 | witness | 归因面 | 严重度 | 修向候选 |
|---|---|---|---|---|---|
| G1 | verify 重试把 runner 名当 make target(`make make`),真断言失败洗成 UNAVAILABLE | zod run-1 log:4695;run-2 log:3770 | engine(scoped verify-target handoff 的 suite 字段污染) | **P0** | handoff 填 make_target(=check)而非 runner 名;或 make runner 忽略 suite、重读 surface.json 的 make_target |
| G2 | checkpoint commit 因幽灵计划路径整批 abort(仅 WARN),核心修复丢出 durable ref | zod run-1 log:2607;5680b1c parent=seed;复放 make check FAIL | engine(CommitChangesForPaths 全有全无 + planner 幽灵路径) | **P0** | git add 逐路径容缺(存在的照提交)+幽灵路径升 typed 失败;apply 后 ref 缺席应拦 finish |
| G3 | 「未验证」终答把断言失败/自伤谎报为「环境缺少测试运行器」,并给出只含最后 plan 的 cherry-pick 指令 | zod run-1/2 终答「未验证」节 | engine 词面(verify_failure_summary 直通用户)+多 plan 落地指引 | P1 | UNAVAILABLE 与 FAILED 分词面;多 plan 会话列全部 applied refs |
| G4 | c2 run-1:第 3 条 blocked_reason 被丢弃→「1 次无记录」;D 总 1.656(running 段冒充 D 段);io 0.285 | run-1.out 53-57 vs fixture 行 2533 | LLM 消费(explore 看到 witness 后自我说服丢弃)+prompt 教学(D 段=switch-out→wakeup 未教) | P1 | extract/finalize 对 blocked_reason↔D 段配对给 typed 车道,而非让模型手推 |
| G5 | a3 EXPECT 正则被 "0.3**50ms**" 子串假命中;52.478 出 50-51 band 无声 PASS | 复放 grep(全文无 5[01]\.);case 正则第 2 行 | oracle 面(EXPECT_MATCHES_REGEX 无数字边界守卫) | P1 | h5 数字边界守卫扩到 REGEX 通道慎改;先修 a3 case 正则为 `5[0-2]\.[0-9]+ *ms` 带前界 |
| G6 | 四态守恒把 1.954ms 未覆盖缺口静默折进 running(50.524→52.478;24.992→26.946),叙事无口径披露 | a3/c2/multicausal 四态行 vs PROFILE §1.7 | engine 词面(coverage gap 折算未标注) | P2 | running 行加「含未覆盖段 Xms」注记,或缺口独立第五项;PROFILE 侧补当前口径 pin |
| G7 | semantic_span run-2 叙事「根因是优先级反转」与自贴投影(主根因=类校验,无反转席)矛盾 | run-2.out 首段 vs 投影区 | LLM 消费 | P2 | finalize prompt:根因机制词须与投影主根因席 typed 类别一致(软引导+答案侧对照) |
| G8 | 同 sleep 双车道两行逐字同形零互指(E1 wakeup_causal_impact / E2 target_self_state,均「☾ 自身·sleep 5.000ms 窗口内主要处于等待唤醒」) | semantic_span ×2 run 树区 72-73 行 | engine 显示(W-A 双账互指句未覆盖 self-sleep 症状对) | P2 | 双车道 self 症状行合并或补账目关系互指句 |
| G9 | 段值冒充窗口总量:「runnable 态等待 1.743ms」(窗口实为 3.636);「占其总 window(58.025ms)」编造 | multicausal run-2、a3 run-2 | LLM 消费(引擎观测 summary 的单段值被当总量) | P2 | 观测 summary 段值带「(单段)」词;finalize 教学:窗口总量只认四态行 |
| G10 | data 工作流:entity_resolution 义务只能由已被阶段门禁掉的动作清偿,repair hint 自相矛盾,6 修全烧 | data run-2 terminal json+尾日志 | engine(阶段门 allowed_actions 与 coverage_contract 不求交) | P1 | 门放行能清偿未清义务的动作,或 assemble_answer 允许携带 entity_resolutions 投影(闸门滤法同 DL 修) |
| G11 | 「合计(共N段)」N=成员组数非物理段数(E4:共2段,成员实为 3 段:0.350+0.285(2段)) | c2 run-2 E4 席行 | engine 显示(小) | P3 | N 取合并后物理段数,与图例「墙钟段求和」对齐 |
| G12 | 代表窗 EXPECT 靠证据索引 dump 命中,叙事代表窗与案意图脱钩 | multicausal ×2 | oracle 面 | P3 | 案改 EXPECT_MATCHES_TEXT_REGEX 限叙事区,或接受 typed 面代偿并落注释 |
| G13 | SPANTOP top3 子行/∩ 互指句零覆盖 | 全 20 run grep | eval 案单 coverage | P3 | EVALCASE-DH 补一案:多 span 族窗口+已知跨方向重叠板(17267 形) |

## 6. 总评
- 16 PASS 里真水位:14/16 站得住;c2 run-1(值错三连)与 semantic_span run-2(根因词曲解)是 PASS 面下的实质缺陷。
- 4 FAIL 全部归因到确定性机制:zod=G1+G2+G3(其中 run-2 是**假 FAIL,代码正确**);data run-2=G10 互锁(诚实 fail-loud)。
- 本窗新词面(◎ 分组/凭证词/修向/守恒/小计双档/同源二分)实渲零缺陷、恒等式手算全过;但 LLM 叙事层对新面**零消费**,SPANTOP 与 ∩ 面零覆盖。
