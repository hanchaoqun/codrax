# r1071 人工审计：Trace 统计口径与 C++ 图身份恢复

- date: 2026-09-14T11:55:23Z
- sweep_start_ts: 20260914-045523
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

清洁47ae1ed5079b（B1684/B1685已交付）构建11:54:58Z，二进制不可变快照exact2各一次。当前用例均read15步，上批已轮转写模式；CAP5对这两单仓无影响。未改oracle/case/fixture/原MD/HTML/root-causes，未追加第三例或追PASS。

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 1 | trace_query_donghu_real_frame_multicausal | PASS | eval/results/trace_query_donghu_real_frame_multicausal-20260914-045523 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 213s | 54 | read=4,repo_map=0,list=0,trace=1,source_lens=0 | midloop=2,inv=1/0,fin_reject=1,unavail=0,prune=0 | FAIL | 投影/两轴/旁路保留；正文跨席借caller、错误加总与同核口径；系统包络错称单次另修B1688 |
| 2 | sr_cpp_virtual_chain | PASS | eval/results/sr_cpp_virtual_chain-20260914-045523 | answer_regex,answer_contains | none | 273s | 36 | read=4,repo_map=2,list=0,trace=0,source_lens=0 | midloop=8,inv=3/0,fin_reject=6,unavail=0,prune=0 | FAIL | 原图可解析但删至零消息边；纯拓扑补身份误配另一工厂调用，B1687公共复现 |

## 1. Trace：数值供给与模型说明分开验收

原产物`.codrax/output/20260914-045854.090-39837.{md,html,root-causes.json}`；日志本结果目录`run-1.logs/codrax-20260914-045525-000-39837.log`。以固定原trace独立核对，不沿用r943旧PASS为golden；旧r943外层1800s不同于本轮1200s，不声称加速比例。

- 显式34579.472865–34579.587805/114.940ms、目标59566、36次唤醒（Cookie34/Binder1/T7 1）、60555→60595→59843→59566方向保留。目标running26.946/runnable3.636/sleep84.358/D与IO0正确。Trace投影、两轴、链上IO/供给、VerifyClass LacUtils .285ms和frame未证限定在，自动补采bundle+vsync2次814ms（log1409–1422）。三个text fence、无模型Mermaid，非语法失败。
- Explorer只1次bundle、4次派生JSON read及2次grep，无源码读取或emit_evidence；1次完成接受。Explorer误称无compute缺口（1321），finalizer得到同源10.331ms卡片（2809/2877）后正文改回，证明该供给有效。首次analysis因causal范围夹带fact_families退回后自纠；exclude所需quote未填，但schema902–909和当轮log106已说明，不加原文关键词硬门补它。
- 正文实质错误：md15/54从S态推断非同步/非内核阻塞；37/47/75把未知reason10.433ms接成同线程fscache/hmfs；60把26.738/20.342全Runnable当同核重叠（独立真overlap1.847/.683）；71/73将总片段2.978/7.235说成Runnable（真2.377/6.754）；79把T7 IO3.550移至.526–.587（真.474223–.478112）；81声称明显重叠各窗不重叠并合计114.940。log1939–1941已明确未知caller不能借同线程；不能据单轮错误宣称“模型波动已证”，也不以扫描答案改写/否决来修复。
- sidecar v2/status=available/4项、rank1–4、同请求窗、impact_seconds=.024710/.019041/.010433/.010331、frame_unproven及前两项低优先级候选组成正确，JSON非空且可解析。但模型description第3项仍错误借fscache/单次最长，第2项将唤醒下游Cookie称上游；typed evidence没有借caller。接口消费者应区分冻结测量字段与模型自由说明，不能把文件存在/可解析当说明准确性绿签。
- B1688/P1：系统md369–371“代表性时间窗”表拿聚合node.StartTs/EndTs作“一处代表性发生片段”。真实rank1至少5个不连续occurrence，却展示.472865–.572194包络。生产`answer_document_mutation_runtime.go`使用的node没有独立occurrence凭证；只纠正为起止包络/可能重叠/不代表持续或单次，不猜补片段。后续真正多片段供给另列。

## 2. C++：原图语法通过，关系与答案未过

原产物`.codrax/output/20260914-045955.013-39847.{md,html}`；日志`run-1.logs/codrax-20260914-045525-000-39847.log`。实际4次read覆盖logger/registry/console/sink，未读旧README。分析阶段猜sink_->log，探索读源码后改回write，不用早期猜测直接立系统gap。9探索轮、7成文轮、6拒/6patch；指标inv=3/0，但工具成功返回中的DOWNGRADED不等于完成通过（log2110–2122）。

- 正确部分：日志拼接→virtual write→Console fputs/fputc、三种工厂分支、未知nullptr。错误/遗漏：正文声称Logger构造器调用make_sink→create，但源码构造器只接unique_ptr并move；并无装配调用站点。Error条件flush及Console继承的空flush未解释。可见列表仍泄漏生成身份`SinkRegistryCreate_af1c9eb7c200871d`。
- 最终sequence只剩L/C两参与者和两Note，零消息边；本例未强制图，不增“必须有图”门，但它不能作为完整关系的合格可视化。原MD/HTML分别用仓内Mermaid实际parse+render成功（SVG20255/20075字节），原HTML整页执行SVG20588字节、page_errors=[]，网络HTTP(S)全阻，原SHA不变；`.codrax/tmp/20260914-r1071-cpp-mermaid-render.json`。语法绿与语义绿不同。
- B1687/P1确证：原full log3439的C=ConsoleSink::write、F=std::fputs，C→F空identity。patch1/2只删L/S错边和孤立S（3560/3596），没改C/F身份。系统3598–3601两次正规化按完整component拓扑唯一补成make_sink→SinkRegistry.create，3628再报identity_conflict。单边真局部片段被误配到另一单边组件；图形唯一不代表模型选择了该语义。公共Emit+Patch RED已8臂1.210s；修复需明确节点选择凭据，不能只扩子图搜索来拟合本例。

## 3. 否证与交付顺序

1. Trace optional patch未立冲突：提示及共享教学明确“若当轮发布add_facet_id才用，否则完整replace_blocks”。模型自行说schema已发布并发未发布分支；当前ParametersFor与执行同源限制。顶层schema_version被隔离不是拒因。一次可选建议失败后保accepted首稿，无流超时降级。日志未保存完整第二轮schema wire，不冒充逐字网络对照。
2. B1686/P1初始Trace手读分流教学残余：explorer3837让手读raw trace用emit_evidence，实际raw与query blob均外部soft-skip并要求reason/aggregate_facts。本轮未走此分支，不能说导致此次成文失败；公共Read/Emit/BuildInitialInstruction八臂RED1.094s后复用既有共享提示，权限/窗口/因果和事实查询分流不改。
3. 优先顺序：B1687错误身份自铸；同批B1688系统时间范围口径与B1686旧教学。模型正文/旁路description准确性、精确代表片段供给、主数据流导航供给仍留账，不加散文硬门，不替模型下结论。
4. 原始产物SHA在`20260914-r1071-original-artifacts.sha`；case/fixture前后核对通过。B1684/B1685此次不宣称自然触发后的生产闭环或提速。代码修复后先确定性验收，不重跑此对追绿；后继异构读/写exact2另排。
