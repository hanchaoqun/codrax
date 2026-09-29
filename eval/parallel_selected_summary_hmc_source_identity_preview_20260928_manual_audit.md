# HMC §188：授权测试身份与Trace预览人工审计

- date: 2026-09-29T04:00:06Z
- sweep_start_ts: 20260928-210004
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

固定7969两例并行×1，正式exit0。生产快照`02770242fba3`，build time `2026-09-29T03:59:30Z`；自然问题及oracle不改，没有第三次追跑。机器1/2、完整用户结果人工1/2。新只读登记分支本次未live命中，与写例用户结果PASS分开记；旧§187失败不改签。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | empty_python_module_apply | PASS | eval/results/empty_python_module_apply-20260928-210006 | write_apply,write_patch_oracle,answer_contains | none | 128s | 28 | read=6,repo_map=1,list=0,trace=0,source_lens=0 | midloop=0,inv=0/0,fin_reject=0,unavail=0,prune=0 | PASS（用户结果）；新登记未命中 | 只改实现、4原生测试PASS；规划身份仍错，弱化后的合同无强补证义务 |
| 2 | trace_existing_sqlite_dictionary | FAIL | eval/results/trace_existing_sqlite_dictionary-20260928-210006 | log_regex,trace_attachment,primary_answer | perf_triage+trace_query | 217s | 42 | read=0,repo_map=0,list=0,trace=5,source_lens=0 | midloop=1,inv=2/0,fin_reject=0,unavail=0,prune=0 | FAIL | 解码预览/合法事件族实际命中；漏系统事件、未知业务名误命名、线程冒充进程 |

## 1. 写模式：用户结果通过，不代签补证身份新分支

实际交付仅`totals.py`增加`total(values): return sum(values)`及说明字符串，使用Python整数求和、空输入和单次迭代语义；两份既有测试文件与fixture逐字节cmp相同，delivery仅持有该实现路径，未改配置或依赖。报告`plan-1790654497715402000-98063.report.json`记录真实语法预检exit0和`python3 -m unittest "tests/test_totals.py" -v`exit0，四条assertion均PASS；不是把接受清单当执行结果。最终输出“4条验证结果通过”与实际一致，明确自然语言清单不代表每项独立执行证明。

但本轮写前分析产生的5条fallback合同全部为`quality_repaired:planning_only_ungrounded`、无required，因此没有进入verification_proof_followup，新`Retained-source native test identities`并未投递。原计划PTO仍错填pytest风格，而真实suite为`tests.test_totals.TotalTest`。证明账没有将这些错误声明变成required行为见证，也没有偷偷匹配身份；当前用户结果可PASS，但不能声称§187登记失败已获真实模型重验或原生补证能力完全验收。新分支仅有公开真实controller/原生runner及恢复矩阵验证，继续挂18.5验收。

源文件存在但空时首次create被拒，模型修为insert_at_eof；此轮多次规划与分类弱合同属既存系统/教学残余，记录在01.2/01.3/16.4/18.5，不根据一次规划输出调整硬门或追加本案例专属提示。后续宜用公开预置强合同恢复场景验证登记分支，避免靠自然分类恰好选中或增加用户心智；不修改本次原判。

## 2. Trace：预览能力命中，完整答案仍失败

真实日志193–201（后续消息715、1155等同样）已有5行解码预览。1010000000ns明确为1.010000000秒，查询族明确`trace_mark`/`hi_sysevent`，stage-1不再自行从编码字符串猜1.012。正文查询全有时间窗，trace_query从§187的14次降到本批5次（样本量仅一对，不能当稳定性能结论），read_file/repo_map/list/source_lens均0。正确族查询实际命中，没有上一批`hisysevent`拼写漏查；一次扩到0.950秒的span查询保留合法补充能力，最终正确排除窗外阶段。

最终四段起止和8/4/4/5ms均正确，机器8/5ms正则不接受表头“耗时（毫秒）”与数字单元格分开的格式，仍属oracle误报，原机器FAIL保留。不过人工有独立、实质错误：

- 全窗口5条系统事件只给4条，漏`APP_LAUNCH / BOOTSTRAP / stage=bootstrap status=ready`。最终日志2892–2896明确全窗13个端点/事件已完整供给：8启动端点+5系统事件，row-6含被漏行，handoff_rows_omitted=0。另三份带PID的库存分别8/3/11条，不能替代无主体过滤的全窗集合。
- 对NULL与无法唯一解析的启动名称仍写`startup（第1/2次）`，未说明是合成标签而业务名未知；NULL域写“无对应域”也把未知变成不存在。typed字段在最终输入中完整可见，不能再归咎源字段丢失。
- `DocumentApp`来自线程名，尚无业务进程名称凭证，却称“DocumentApp（PID27599）”。内部载体名`codrax_trace_mark_exact`直接进入用户答案；这与业务化表达目标仍有差距。
- 请求窗是1.000–1.080秒；coverage的scope_time及matched_time同为1.010–1.058秒。最终以匹配包络宣称前10ms“无任何事件”，没有界定可查询/未解析人口和采集覆盖。精确query字段仍在，但scope描述的语义歧义应系统性拆分，不要求用户自行解释。
- explorer已经给出错误的4条member_set，finalizer又误称“13条只展示10条”，实际13条共享展示成员齐全。是已接受模型汇总与确定性库存相互竞争的接缝；不扫正文修答案，也不因新完整数据在场便宣称纯模型波动。

Markdown与HTML均输出两张表，无关系图；核对内容/结构，不冒称截图视觉验收。根因旁路schema2、空列表、`trace_root_cause_contract_not_active`，未把系统背景事件升级链上根因。

## 3. 原始证据SHA-256

| 文件（结果目录见表） | SHA-256 |
| --- | --- |
| write `run-1.out` | `2000213232da759ad77917cfdf372ee93bc2e16582a8380d63196ff78f8e0579` |
| write `run-1.logs.all.log` | `5540f52022f42a8440b4807042394a790a455e72a682f02ac487f25f4b162730` |
| write `run-1.plan-2.json` | `c7b47f8ab77ea082abf5ce941b4efe68a9378eeddb7bdec5db5b61e6fd4e112c` |
| write `plan-1790654497715402000-98063.final.json` | `7f8dccbb7350bc512dda5fd70a4d98b476d63b95d4e05b6448e0b8708397d385` |
| Trace `run-1.primary.md` | `dc47a65962641f66f1a3af25a510e4222a5c8eb8db791dab2deade9f6a02e443` |
| Trace `run-1.logs.all.log` | `4d1cc22943fb1c56cebc3517ad9f1a493317b525127e8e91cde5f1bb27ec6eb2` |
| `.codrax/output/20260928-210339.807-98047.root-causes.json` | `5f8063f5882d6f6c5da9ef59d8ba0282f0fb2ef8c728bdcc77ef0249d2287e39` |

## 4. 下一批：不追单例措辞，保完整能力主线

普通viewer兼容/启动实例/主体/首帧、WAL一致快照仍是04.3/17.7未交付范围；新登记真实模型验收留18.5。高影响缺陷优先检查统计作用域与成员集的类型化绑定：请求窗、源观察包络、匹配包络、对象过滤和模型汇总分开传递；用有时间/对象变化的公开反例验证，不删除扩窗支持或用文字扫描硬门。旧5个稳定验收父项和全部历史FAIL留账，当前总数仍63开放。
