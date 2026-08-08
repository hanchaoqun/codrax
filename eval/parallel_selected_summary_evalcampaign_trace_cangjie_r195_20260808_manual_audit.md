# Selected Eval Manual Audit Scaffold

- date: 2026-08-08T07:13:47Z
- sweep_start_ts: 20260808-001345
- total cases: 2
- parallel: 2
- timeout: 1200s per case
- results_root: eval/results

This scaffold is for human review. The runner records typed metrics and declared oracle surfaces only; it does not decide whether a PASS theoretically solves the real user requirement.

| # | case | verdict | result_dir | declared_oracles | runtime_authority | sec | ctx% | tools | churn | human_correctness | audit_notes |
|--:|------|---------|------------|------------------|-------------------|----:|-----:|-------|-------|-------------------|-------------|
| 2 | cangjie_repomap | PASS | eval/results/cangjie_repomap-20260808-001347 | typed_inventory_rowset,dimension_substring,answer_contains | none | 133s | 22 | read=0,repo_map=2,list=0,trace=0,source_lens=2 | midloop=1,inv=1/0,fin_reject=0,unavail=0,prune=0 | FAIL | 2 extend、2 foreign func、8 public class 的 typed 清单与可见路径都正确；但第二个同名 `native_add` 的模型原始 citation_ref 正确指向 corpus 文件，pre-emit 弱 aggregate repair 将其改到 Bridge，随后错误 row_id 自洽并裁掉正确 citation。属于系统改错模型正确引用，记 B332。 |
| 1 | trace_query_wakeup_background_demotion | FAIL | eval/results/trace_query_wakeup_background_demotion-20260808-001347 | log_regex,trace_attachment,answer_regex,answer_contains | perf_triage+trace_query | 201s | 37 | read=0,repo_map=0,list=0,trace=3,source_lens=0 | midloop=0,inv=1/0,fin_reject=0,unavail=0,prune=0 | PASS | runner 仅因单行 prose regex 未跨行匹配而报 FAIL；人工确认正文与系统投影均给出 `threadpool→network→cookie→app`、链上 11ms 主根因，并把 19.5ms logger 明确降为无依赖背景。但关键指标表仍给 background logger 显示 7ms“有效归因”，与列定义/不参与排序自相矛盾，记 B331；内核 wait point 被模型扩写成“磁盘读取未完成”记 B333(P2)。 |

## Human Audit Checklist

- Mark human_correctness as pass/fail/uncertain only after reading the final answer, relevant logs, and any applied patch/diff.
- For inventory cases, prefer typed_inventory_rowset or row/origin evidence over broad answer_contains or dimension_substring oracles.
- For runtime/log/trace cases, interpret trace_query absence together with runtime_authority; pre-stage log_triage/perf_triage can be authoritative.
- Record prompt/tool noise, repeated completion/form repair, unavailable tool attempts, context pressure, and any answer supplement that changes the main conclusion.

## Human findings

### `trace_query_wakeup_background_demotion`

- 三次 `trace_query` 的 typed authority 完整：`wakeup_chain` 证明 `threadpool-400 -> network-300 -> cookie-200 -> app-100`，`root_cause_rank` 仅把链上 `threadpool-400` 的 11ms IO wait 加冕；`logger-900` 明确为 `background` 且不进入 root ranking。
- 模型正文与系统 Trace 因果投影都遵守“根因只能来自已证链”的新不变量。runner 的正则把“唤醒链”与线程名要求在同一行，无法识别自然分段答案，是 oracle false negative，不应以放宽生产硬门修它。
- 真实产品 gap 在结构化表面：关键指标列把 `有效归因` 定义成“计入根因排序的影响时长”，却给 background logger 发射 `7.000ms`。这会让非链上背景重新获得类似根因的数值外观，即使同一行另一列写着“不参与根因排序”。B331 应仅按 typed row kind / chain relevance 把 adjacent/background 在该列显示为 `—`；背景规模仍留在背景/条件上界表面，不删除信息。
- 模型把 `fscache_page_wait_on_page_bit` 从“已证内核等待点”扩写为“页面尚未从磁盘读取完成”，超出当前 trace 凭证；后文 caveat 又承认对象/持有者未知。B333 只需给模型 typed caliber 软提示，不能扫描答案词面硬拒，也不能由系统改写结论。

### `cangjie_repomap`

- typed inventory 的 12 行、计数、family、文件、行号与 package 均正确；runner PASS 只证明清单存在，不能发现 citation 与可见路径相互矛盾。
- 模型首次 `emit_answer_document` 给两个同名 `native_add` 分别使用 Bridge 与 corpus 的正确引用。系统随后记录 `bound 1 item citation_ref ... aggregate member support`，把 corpus 行改成 Bridge；再按被改错的 citation 绑定 row_id，并把真正 corpus citation 当 unused pool entry 删除。
- 根因是 authority 顺序：压缩 aggregate 的单一同名 support 弱于 source-inventory 的 exact label + file + line，却先破坏了文件轴。B332 的通用修复必须在任何弱 citation repair 前捕获精确 typed row identity，并在末尾从 row_id 恢复 citation；适用于所有语言、family 和同名声明，不得按 Cangjie/native_add 特判。

## Ruling

- runner：1 PASS / 1 FAIL；人工：1 PASS / 1 FAIL。runner 两个方向都与人工相反，进一步证明必须审计最终答案与归一化日志。
- `EVAL-B331-TRACEBACKGROUNDATTRIBUTIONDISPLAY1=P1-confirmed`：非链上 row 的根因归因列必须 typed-dash，背景信息保留在非根因表面。
- `EVAL-B332-DUPLICATESYMBOLCITATIONFILEAXIS1=P0-confirmed`：系统不可用弱 aggregate support 覆盖模型已选中的精确 source-inventory 文件/行。
- `EVAL-B333-TRACEKERNELCALLSITECLAIMCALIBER1=P2-filed`：wait point 只能软提示为已证等待点，具体介质/对象/持有者仍未证。
- 本批无 malformed JSON、无 JSON repair、无 finalizer reject、无“成文校验未通过”；显式时间窗、Trace 自动补齐、链上根因排序、唤醒链与窗内可消除量均保留，系统未替换模型主结论。
