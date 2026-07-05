# Deterministic profiles — real_traces fixtures (campaign ground truth)

Date: 2026-07-05. Companion to `README.md` (provenance) and
`docs/design/real_trace_campaign_20260705.md` (dimension matrix, per-case
oracle rationale, probe source appendix).

Method: every number below was produced either by

- **[shell]** — the plain shell command shown inline (run from
  `eval/fixtures/real_traces/`), or
- **[probe]** — a parser-parity Go probe that builds the index through
  `internal/tracequery.BuildIndex` and reads
  `ThreadTimeline`/`Event` fields exactly as the product does. The probe was
  temporary (deleted after use); its full source is archived verbatim in the
  campaign ledger appendix (`docs/design/real_trace_campaign_20260705.md`
  §A), so every [probe] number is re-derivable by pasting it back under a
  scratch `package main` dir and running the command recorded there.

Probe invocations used:

```
go run ./tmp_probe_trace_profile -pids 59566,59891 \
  -windows "jank:34579.472865:34579.475857:v;wide10:34579.472865:34579.502785;post30:34579.475857:34579.505857;legacy115:34579.472865:34579.587805" \
  eval/fixtures/real_traces/donghu_tieba_frame.systrace

go run ./tmp_probe_trace_profile eval/fixtures/real_traces/donghu_short_excerpt.systrace
```

Fixture files are read-only (README red line): all numbers are observations,
never edits.

---

## 1. donghu_tieba_frame.systrace (15,623 lines, 1.9 MB)

### 1.1 Identity & time range

| fact | value | derivation |
|---|---|---|
| lines / parsed events | 15,623 / 15,623 (0 unparsed, 0 parse panics, 0 clock regressions) | [probe] header block; cross-check `wc -l donghu_tieba_frame.systrace` |
| first ts | 34579.450627 s | [shell] `head -1 donghu_tieba_frame.systrace \| grep -oE '[0-9]+\.[0-9]{6}'` |
| last ts | 34579.595184 s | [shell] `tail -1 donghu_tieba_frame.systrace \| grep -oE '[0-9]+\.[0-9]{6}' \| head -1` (monotonic: probe clock_regressions=0) |
| span | 144.557 ms | last − first |
| flavor | harmony_hitrace, confidence 0.98 (signals: event_binder_label, event_ffrt, event_harmony_irq, event_harmony_render, raw_harmony_marker) | [probe] |
| header lines | none — file starts directly at an event row | [shell] `grep -c '^#' donghu_tieba_frame.systrace` → 0 |

### 1.2 Event kind counts (tracequery typed)

[probe]; raw-name cross-check: `grep -oE '[0-9]+\.[0-9]{6}: [a-zA-Z_]+:' donghu_tieba_frame.systrace | awk '{print $2}' | sort | uniq -c | sort -rn`

| typed kind | count | raw names |
|---|---|---|
| irq | 4090 | irq_handler_entry 2045 + irq_handler_exit 2045 |
| sched_switch | 3517 | |
| sched_wakeup | 2130 | sched_wakeup 2122 + sched_wakeup_new 8; **sched_waking: 0** |
| trace_mark | 1680 | print |
| cpu_idle | 1278 | |
| softirq | 526 | entry 263 + exit 263 |
| memory | 463 | mm_filemap_add 439 + delete 24 |
| sched_blocked_reason | 441 | |
| clock_set_rate | 323 | |
| block_rq_complete / block_rq_issue / block_bio_remap | 276 / 276 / 269 | |
| workqueue | 106 | execute_start 53 + end 53 |
| cpu_frequency | 90 | |
| binder_transaction / binder_transaction_received | 79 / 79 | |
| cpu_frequency_limits | **0** | [shell] `grep -c 'cpu_frequency_limits' …` → 0 |

### 1.3 CPUs & cluster shape

- CPU brackets observed: **6 CPUs, ids 0–5**. [shell]
  `grep -oE '\[[0-9]{3}\] [^ ]{4} [0-9]+\.[0-9]{6}:' donghu_tieba_frame.systrace | awk '{print $1}' | sort | uniq -c`
  → 000:3431, 001:2775, 002:2170, 003:2719, 004:2833, 005:1695 (matches [probe] exactly).
  (Naive `grep -oE '\[[0-9]{3}\]'` also catches a payload artifact `[960]` from
  audio span text — do not use the unanchored form.)
- cpu_frequency samples exist **only for cpu_id 3, 4, 5** — 30 samples each,
  identical 11-step ladder 807000..2189000 kHz
  (807000, 965000, 1090000, 1224000, 1484000, 1618000, 1748000, 1882000,
  1997000, 2093000, 2189000) → cpus 3–5 form one governed cluster face;
  cpus 0–2 have **no frequency samples in this capture** (absence of samples,
  not proof of no DVFS). [probe]; [shell] `grep -oE 'cpu_frequency: state=[0-9]+ cpu_id=[0-9]+' … | sort | uniq -c`
- Frequency sample timeline: first at 34579.468951 (1090000, cpu3), a jump to
  2189000 at 34579.470347, last at 34579.590078 (1224000). [shell]
  `grep -n 'cpu_frequency:' donghu_tieba_frame.systrace | sed -n '1p;$p'`
- cpu_idle events on all 6 cpus (0:46, 1:10, 2:22, 3:492, 4:380, 5:328). [probe]
- Idle (pid 0) attributed running time across all cpus: 269.833 ms of the
  6×144.557=867 ms cpu-time budget → large idle headroom in-window. [probe]

### 1.4 Threads (241 distinct pids) — top by attributed running time

[probe] per-cpu sched_switch attribution (interval between switch-in and next
switch on same cpu). Top 10:

| comm | pid | tgid | busy ms | switch-ins |
|---|---|---|---|---|
| sysevent_store | 47924 | 47895 | 59.108 | 137 |
| hilogd.pst | 474 | 459 | 53.421 | 133 |
| com.baidu.tieba | 59566 | 59566 | 51.462 | 99 |
| os.FusionSearch | 8661 | 8091 | 35.637 | 74 |
| T7@ZeusThreadPo | 61839 | 59566 | 30.287 | 22 |
| hilogd.rd_kmsg | 503 | 459 | 29.667 | 258 |
| os.FusionSearch | 8091 | 8091 | 27.712 | 63 |
| TurboNet | 61522 | 60885 | 16.271 | 48 |
| dh-irq-bind-0 | 75 | 59 | 15.337 | 155 |
| hisi_rxdata | 919 | 59 | 15.148 | 27 |

Process tgid=59566 (com.baidu.tieba) has 39 threads in this capture
([shell] `grep -oE '^[^(]+-[0-9]+ *\(59566\)' … | sort -u | wc -l`); top by
busy: main 51.462 ms, T7@ZeusThreadPo-61839 30.287 ms, NetworkService-60595
13.135 ms, CookieMonsterCl-59843 8.494 ms, BdAsyncTask #8-59953 8.006 ms,
ThreadPoolForeg-60555 7.738 ms, …, RenderThread-59891 only 1.077 ms. [probe]

Thread pid=99999 does **not** exist: [shell] `grep -c '99999' donghu_tieba_frame.systrace` → 0.

### 1.5 Priority distribution (Harmony semantics: larger numeric = higher; 1–40 CFS, 41–139 RT)

[shell] `grep -oE 'next_prio=[0-9]+' donghu_tieba_frame.systrace | sort | uniq -c | sort -rn | head`

| next_prio | switch-ins |
|---|---|
| 20 | 1294 |
| 65534 (idle-class) | 649 |
| 142 | 453 |
| 40 | 376 |
| 51 | 198 |
| 52 | 131 |
| 10 | 107 |

Target prios: main thread 59566 always prio **52** (RT band); RenderThread
59891 prio **53**; CookieMonsterCl-59843 / NetworkService-60595 /
ThreadPoolForeg prio **20** (CFS band) — the chain the main thread waits on
runs at *lower* Harmony priority than the main thread. [probe]

### 1.6 Wakeup edges

- Totals: 2130 typed sched_wakeup (incl. 8 wakeup_new), 0 sched_waking. [probe]
- Wakeups **into main thread 59566: 48** ([shell]
  `grep -c 'sched_wakeup: comm=com.baidu.tieba pid=59566' donghu_tieba_frame.systrace`),
  of which **34 from CookieMonsterCl-59843** ([shell] append
  `… | grep -c 'CookieMonsterCl-59843'` on the matching lines), 8 from
  T7@ZeusThreadPo-61839, 3 from udk-irq (irq threads), 1 each from
  Binder:43397_19, Chrome_IOThread-60560, RenderThread-59891. [probe] full list.
- Top pairs global (count): dh-irq-bind-0→wlan_bus_rx/sdi 67,
  dh-irq-bind-0→hisi_hcc 49, sysevent_store→dh-irq-bind-0 47,
  hilogd.pst→hilogd.rd_kmsg 44, **CookieMonsterCl→com.baidu.tieba 34**,
  **CookieMonsterCl→NetworkService 32**, **NetworkService→CookieMonsterCl 31**,
  **com.baidu.tieba→CookieMonsterCl 31**. [probe]
- Chain shape for case design: main ⇄ CookieMonsterCl ⇄ NetworkService is a
  repeated bidirectional wake relay; NetworkService sits one hop upstream of
  CookieMonsterCl on the main thread's wait chain.
- All 48 wakeups of the main thread target cpu 0 (`target_cpu=000`). [probe]

### 1.7 Main-thread (59566) state breakdowns per window

[probe] = product-parity `tracequery.ThreadTimeline(idx, Query{PID:59566,
TimeStart/End set})`, per-state DurationMs sums:

| window | bounds (s) | width | running | runnable | s_sleep | d_sleep | io_wait |
|---|---|---|---|---|---|---|---|
| full | 34579.450627–34579.595184 | 144.557 ms | 50.524 (99 frag) | 5.529 (96) | 85.915 (45) | 0.488 (2) | 0.147 (1) |
| jank | 34579.472865–34579.475857 | 2.992 ms | 0.000 | 0.014 (1) | 2.978 (1) | 0 | 0 |
| wide10 (10× jank) | 34579.472865–34579.502785 | 29.920 ms | 2.964 (20) | 0.794 (19) | 26.162 (10) | 0 | 0 |
| post30 | 34579.475857–34579.505857 | 30.000 ms | 3.414 (19) | 0.780 (19) | 25.806 (10) | 0 | 0 |
| legacy115 | 34579.472865–34579.587805 | 114.940 ms | 24.992 (67) | 3.636 (67) | 84.358 (36) | 0 | 0 |

Ratios worth pinning: wide10 → s_sleep 87.4%, running 9.9%, runnable 2.7%.
post30 → running 11.4%. jank → running 0.000 ms (the two boundary "running"
intervals clamp to zero width), s_sleep 99.5%; the window ends exactly at the
CookieMonsterCl wakeup: sched_wakeup at 34579.475843 (line 3213), runnable
0.014 ms, switch-in at 34579.475857. legacy115 covers ~1.954 ms of
out-of-interval gap (covered_total 112.986 of 114.940 ms).

RenderThread 59891 for the same windows: full = running 1.077 / runnable
0.080 / s_sleep 143.330 ms; **jank, wide10, post30, legacy115 = 100% s_sleep**
(RenderThread never runs between 34579.4728 and 34579.5878; its only activity
is at trace tail — woken by main thread at 34579.590882 and 34579.593245). [probe]

### 1.8 D-state / io_wait evidence for main thread

Exactly 3 sched_blocked_reason rows for pid 59566, all `iowait=1`, all caller
`sync_buffer_read_wi+0x60/0x11c[sysmgr.elf]`, at ts 34579.451840 (line 118),
34579.453081 (line 250), 34579.471723 (line 2533) — i.e. all **before** the
jank window. Full-window D totals are small: d_sleep 0.488 ms + io_wait
0.147 ms. [probe]; [shell] `grep -n 'sched_blocked_reason: pid=59566' donghu_tieba_frame.systrace`

### 1.9 VSync / periodic-source signals

- 59 lines mention vsync case-insensitively. [shell] `grep -ci vsync donghu_tieba_frame.systrace`
- Thread VSyncGenerator-1682 (tgid 1252 = render_service) emits 32 event
  lines. Marker spans include `B|onVSyncEvent` (3), counter `C|VSYNC-app` (3),
  `H:VSyncReceiver … period:16552213`, `H:GenerateVsyncCount:1, period:16552213`,
  two `H:VSync now:… period:16552213` pulses (34579.468x and ~16.55 ms later),
  DVSync management spans. [probe]
- Period ground truth: **16552213 ns ≈ 16.55 ms ≈ 60 Hz** (also
  `vsyncRefreshRate(60)`, `rate:60` appear verbatim). [shell] `grep -o 'period:16552213' … | head`
- Density caveat for case design: only ~2 VSync pulses fall inside the
  144.6 ms capture window — presence/period questions are supported; a full
  multi-cycle cadence accounting (VS-1 style) is NOT supported by sample count.

### 1.10 Binder

79 transactions + 79 received (delivery pairs), first at 34579.457104. Sample:
`transaction=21639505 dest_node=0 dest_proc=59566 dest_thread=61425 reply=1` —
binder traffic exists both toward system services and into tgid 59566. [probe]

### 1.11 Supported / not-supported summary (for the dimension matrix)

Supported: windowed state decomposition (all bands), wide/degenerate/dual
windows, whole-trace no-window profile, out-of-range window honesty (data ends
at 34579.595184), named/tid-only/process-level/multi subjects, missing-thread
honesty (99999), D/io_wait with verbatim kernel caller token, wake-chain via
NetworkService, priority-band contrast (52 RT target vs 20 CFS dependencies),
demand-vs-supply wording (sleep-dominated + 269.8 ms idle headroom + high fmax),
same-trace dual-window normalized comparison, cross-trace comparison vs
excerpt, freq-cluster evidence (cpus 3–5 only), vsync presence/period.
Not supported: cpu_frequency_limits lane (0 events), sched_waking lane,
multi-cycle periodic cadence accounting, truncation-disclosure shapes (15,623
events is far below any index/event budget — no honest way to trigger).

---

## 2. donghu_short_excerpt.systrace (100 lines, 11.7 KB)

### 2.1 Identity & time range

| fact | value | derivation |
|---|---|---|
| lines | 100; 92 parsed events + **8 unparsed** | [probe]; the 8 = the `#` comment header block ([shell] `grep -c '^#' donghu_short_excerpt.systrace` → 8), including the `TASK-PID TGID CPU#` legend — header/format lane IS present here (unlike the frame fixture) |
| first ts | 2942.244845 s | [shell] `sed -n 9p donghu_short_excerpt.systrace \| grep -oE '[0-9]+\.[0-9]{6}'` |
| last ts | 2942.245401 s | [shell] `tail -1 … \| grep -oE '[0-9]+\.[0-9]{6}' \| head -1` |
| span | **0.556 ms** | last − first |
| flavor | harmony_hitrace, confidence 0.98 | [probe] |
| timebase | 2942.x s — a **different capture/boot epoch** than the frame fixture's 34579.x; windows are not cross-comparable on a shared clock | inspection |

### 2.2 Event kinds

[probe]: irq 22, sched_switch 17, sched_wakeup 12, trace_mark 12, cpu_idle 8
(all on cpu_id 5), binder_transaction 7, binder_transaction_received 6,
sched_blocked_reason 3, memory 2, block_bio_remap/issue/complete 1 each.
**Absent lanes: cpu_frequency (0), cpu_frequency_limits (0), vsync (0
case-insensitive), clock_set_rate, workqueue, softirq.** [shell]
`grep -ci vsync donghu_short_excerpt.systrace` → 0; `grep -c 'cpu_frequency' …` → 0.

### 2.3 Threads & CPUs

18 threads; 10 CPU brackets seen (000,001,003,004,005,007,008,009,010,011 —
sparse sampling of a ≥12-cpu device, no cpu 2/6). [shell]
`grep -oE '\[[0-9]{3}\] [^ ]{4} [0-9]+\.[0-9]{6}:' donghu_short_excerpt.systrace | awk '{print $1}' | sort | uniq -c`

Top attributed running [probe]: [GT]ColdPool#6-36644 (tgid 36379) 0.367 ms,
binder:486_1-10803 (tgid 10756) 0.280 ms, wc_srvinit_3-37253 0.115 ms,
CronetInit-37179 0.110 ms, wc_srvinit_0-37254 0.105 ms.

Wakeup pairs (12 total): CronetInit→binder:486_1 ×3, binder:486_1→CronetInit
×2, mars::31816→mars::comm ×2, others ×1. Binder round-trips
(CronetInit ⇄ binder:486_1, reply=1 transactions 4407138–4407142) are the
excerpt's only causal story. [probe]

sched_blocked_reason rows: pid=36644 iowait=1
`sync_buffer_read_wi+0x74/0x160[sysmgr.elf]` (ts 2942.245313, line 69);
pid=37253 iowait=0 procmgr_prctl; pid=37254 iowait=0 do_native_map_range.
[shell] `grep -n 'sched_blocked_reason' donghu_short_excerpt.systrace`

### 2.4 What the 100-line excerpt can support (case-design verdict)

- **Header/format lane**: 8-line `#` header incl. TASK-PID TGID legend parses
  as unparsed-without-panic; flavor still detected at 0.98.
- **Degenerate-window shapes**: any humanly-plausible window (even 100 ms)
  exceeds the 0.556 ms coverage → honest-coverage disclosure faces.
- **Binder micro-slice**: 7+6 binder events with reply pairing.
- **Cross-trace asymmetry face**: no vsync / no cpu_frequency at all →
  single-side-unsampled honesty when compared against the frame fixture;
  disjoint 2942.x vs 34579.x timebases.
- NOT supported: state-duration accounting of any depth (sub-ms coverage),
  periodic sources, freq/supply reasoning, priority-inversion arcs (only 4
  distinct prios over 17 switches), whole-frame jank analysis.
