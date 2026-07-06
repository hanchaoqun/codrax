# Real-device trace fixtures (customer-derived, provenance pinned)

Both files are byte-identical copies of real HarmonyOS device trace
excerpts previously kept outside the repo at `../../customlogs/`
(user directive 2026-07-05: adopt into the repo as multi-dimension
eval material).

| fixture | original name | lines | sha256 (first 16) | content |
|---|---|---|---|---|
| donghu_short_excerpt.systrace | a.systrace | 100 | 2a488c7ab59393de | tiny excerpt: header + a short sched slice; useful for header/format lanes and degenerate-window shapes |
| donghu_tieba_frame.systrace | xxx_all.systrace | 15,623 | f5d85dd9723d75c9 | 1.9MB real capture around a com.baidu.tieba (pid 59566) main-thread frame stall at ~34579.472865s-34579.475857s; drives the donghu_real_* eval family |
| customer_carved_trace_mark.systrace | q7_line140711 neighborhood | 31 | (curated) | curated excerpt of a real capture whose print-mark converter (trace_streamer suspect) dropped the leading action character (B\|/E\|/C\|) and left a `0x0:` address residue, so marks arrive as `print: 0x0: <pid>[\|<name>][\|<tag>]`; drives the §16 T-span carved-mark restoration pins. Rows = the real neighborhood plus: the closing E for the com.xs.fm.lite bindApplication span (same thread's later close; span_window pairs on the ftrace ROW pid, the payload container-ns pid only names the span subject), M0538-tag rows from undamaged reference captures, and two carved counter rows from real donghu payload shapes (5-field value-mid + 4-field) that must restore to C\| and never forge a span. |

Rules:
- These are REAL customer-derived captures: never edit in place, never
  "fix" their contents to make a test pass (eval-bar red line). New
  synthetic shapes belong in synthetic fixtures, not here.
- Existing eval cases that reference `../../customlogs/xxx_all.systrace`
  (path-semantics cases assert on the OUT-OF-REPO path form itself) are
  intentionally left pointing outside; new cases should reference these
  in-repo copies.
